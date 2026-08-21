package bancos

// Servicio del diccionario del catálogo: previsualizar, aplicar y exportar.
//
// Previsualizar y aplicar recorren EXACTAMENTE el mismo plan; lo único que cambia es si se
// escribe. Así lo que el usuario aprueba en la pantalla es lo que ocurre.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gpvdp/erp/internal/shared"
	"github.com/xuri/excelize/v2"
)

// hojaDiccionario es el nombre de la hoja del archivo (y lo que se busca al importar).
const hojaDiccionario = "Diccionario"

// catalogoActual es la foto del catálogo y las reglas para decidir qué falta crear.
type catalogoActual struct {
	conceptos map[string]Concepto          // clave: nombre normalizado
	clasifs   map[string]ClasificacionItem // clave: conceptoNorm + "\x00" + clasifNorm
	reglas    map[string]bool              // clave: destino + palabras normalizadas
}

// claveClasif arma la clave de una clasificación dentro de su concepto.
func claveClasif(concepto, clasificacion string) string {
	return norm(concepto) + "\x00" + norm(clasificacion)
}

// claveRegla identifica una regla por su destino (por NOMBRE) y su juego de palabras: con eso,
// importar dos veces el mismo diccionario no duplica reglas.
//
// Por nombre y no por id a propósito: al previsualizar, el destino todavía no existe y no tiene
// id. Si la identidad dependiera del id, el plan contaría de más.
func claveRegla(concepto, clasificacion, aplicaA string, palabras []string) string {
	norms := make([]string, 0, len(palabras))
	for _, p := range palabras {
		norms = append(norms, norm(p))
	}
	sort.Strings(norms) // el orden del archivo no debe cambiar la identidad de la regla
	return norm(concepto) + "|" + norm(clasificacion) + "|" + aplicaA + "|" + strings.Join(norms, separadorPalabras)
}

// leerCatalogo arma la foto actual.
func (s *Service) leerCatalogo(ctx context.Context, empresaID string) (catalogoActual, error) {
	cat := catalogoActual{
		conceptos: map[string]Concepto{},
		clasifs:   map[string]ClasificacionItem{},
		reglas:    map[string]bool{},
	}
	conceptos, err := s.repo.ListarConceptos(ctx, empresaID, false)
	if err != nil {
		return cat, err
	}
	for _, c := range conceptos {
		cat.conceptos[norm(c.Nombre)] = c
	}
	clasifs, err := s.repo.ListarClasificaciones(ctx, empresaID, false)
	if err != nil {
		return cat, err
	}
	porID := map[string]ClasificacionItem{}
	for _, cl := range clasifs {
		cat.clasifs[claveClasif(cl.Concepto, cl.Nombre)] = cl
		porID[cl.ID] = cl
	}
	reglas, err := s.repo.ListarReglas(ctx, empresaID)
	if err != nil {
		return cat, err
	}
	for _, r := range reglas {
		// La regla ya trae los nombres al listarse; si faltan, se resuelven por su clasificación.
		concepto, clasificacion := r.Concepto, r.Clasificacion
		if concepto == "" || clasificacion == "" {
			if cl, ok := porID[r.ClasificacionID]; ok {
				concepto, clasificacion = cl.Concepto, cl.Nombre
			}
		}
		cat.reglas[claveRegla(concepto, clasificacion, r.AplicaA, r.Palabras)] = true
	}
	return cat, nil
}

// ImportarDiccionario lee el archivo y, si aplicar es true, crea lo que falta.
//
// Nunca renombra ni borra nada: solo agrega lo que no existe. Un diccionario importado dos veces
// no cambia nada la segunda vez.
func (s *Service) ImportarDiccionario(ctx context.Context, empresaID string, archivo []byte, aplicar bool, usuarioID string) (PlanDiccionario, error) {
	g, err := gridDeArchivo(archivo, hojaDiccionario)
	if err != nil {
		return PlanDiccionario{}, err
	}
	filas, err := LeerDiccionario(g)
	if err != nil {
		return PlanDiccionario{}, err
	}
	return s.aplicarDiccionario(ctx, empresaID, filas, aplicar, usuarioID)
}

// aplicarDiccionario arma el plan de las filas ya leídas y, si aplicar es true, lo ejecuta.
// Separado del parseo para poder probar el criterio sin pasar por un archivo Excel.
func (s *Service) aplicarDiccionario(ctx context.Context, empresaID string, filas []FilaDiccionario, aplicar bool, usuarioID string) (PlanDiccionario, error) {
	cat, err := s.leerCatalogo(ctx, empresaID)
	if err != nil {
		return PlanDiccionario{}, err
	}

	plan := PlanDiccionario{Filas: len(filas), Acciones: make([]AccionDiccionario, 0, len(filas)), Aplicado: aplicar}
	for _, f := range filas {
		acc := AccionDiccionario{
			Linea: f.Linea, Concepto: f.Concepto, Clasificacion: f.Clasificacion,
			Palabras: strings.Join(f.Palabras, separadorPalabras+" "), AplicaA: f.AplicaA,
			Problema: f.Problema,
		}
		if f.Problema != "" {
			plan.Omitidas++
			plan.Acciones = append(plan.Acciones, acc)
			continue
		}

		// ── Concepto ────────────────────────────────────────────────────────
		concepto, existe := cat.conceptos[norm(f.Concepto)]
		if !existe {
			acc.CrearConcepto = true
			plan.ConceptosNuevos++
			if aplicar {
				nuevo, err := s.CrearConcepto(ctx, empresaID, f.Concepto, f.VisibleCxP, usuarioID)
				if err != nil {
					return PlanDiccionario{}, err
				}
				concepto = nuevo
				cat.conceptos[norm(f.Concepto)] = nuevo
			} else {
				// Previsualizando también se anota (con id vacío): varias filas comparten
				// concepto y el plan no puede contarlo dos veces.
				cat.conceptos[norm(f.Concepto)] = Concepto{Nombre: f.Concepto}
			}
		}

		// ── Naturaleza del concepto ─────────────────────────────────────────
		// Solo se DECLARA lo que nadie declaró: el diccionario agrega, no cambia decisiones. Si el
		// archivo dice otra cosa que lo ya declarado, se avisa y no se toca — cambiar la naturaleza
		// mueve el EBITDA de todos los meses y eso no puede pasar por subir un archivo viejo.
		if f.Naturaleza != "" {
			acc.Naturaleza = f.Naturaleza
			switch {
			case !concepto.NaturalezaDeclarada:
				acc.DeclararNaturaleza = true
				plan.NaturalezasDeclaradas++
				if aplicar {
					if err := s.CambiarNaturaleza(ctx, empresaID, concepto.ID, f.Naturaleza, usuarioID); err != nil {
						return PlanDiccionario{}, err
					}
				}
				// Se marca en la foto para que otra fila del mismo concepto no lo cuente de nuevo.
				concepto.Naturaleza, concepto.NaturalezaDeclarada = f.Naturaleza, true
				cat.conceptos[norm(f.Concepto)] = concepto
			case concepto.Naturaleza != f.Naturaleza:
				acc.AvisoNaturaleza = fmt.Sprintf(
					"el archivo dice %s y en el Catálogo está declarado como %s: no se cambia acá",
					nombreNaturaleza(f.Naturaleza), nombreNaturaleza(concepto.Naturaleza))
				plan.NaturalezasEnConflicto++
			}
		}

		// ── Clasificación ───────────────────────────────────────────────────
		var clasif ClasificacionItem
		if f.Clasificacion != "" {
			clave := claveClasif(f.Concepto, f.Clasificacion)
			cl, existe := cat.clasifs[clave]
			if !existe {
				acc.CrearClasificacion = true
				plan.ClasificacionesNuevas++
				if aplicar {
					nueva, err := s.CrearClasificacion(ctx, empresaID, concepto.ID, f.Clasificacion, "", usuarioID)
					if err != nil {
						return PlanDiccionario{}, err
					}
					nueva.Concepto = f.Concepto
					cl = nueva
					cat.clasifs[clave] = nueva
				} else {
					// Igual que con el concepto: dos filas pueden repetir la clasificación
					// (una regla por fila), y el plan no debe contarla dos veces.
					cat.clasifs[clave] = ClasificacionItem{Concepto: f.Concepto, Nombre: f.Clasificacion}
				}
			}
			clasif = cl
		}

		// ── Regla ───────────────────────────────────────────────────────────
		if len(f.Palabras) > 0 {
			clave := claveRegla(f.Concepto, f.Clasificacion, f.AplicaA, f.Palabras)
			if !cat.reglas[clave] {
				acc.CrearRegla = true
				plan.ReglasNuevas++
				cat.reglas[clave] = true // una fila repetida en el archivo no cuenta dos veces
				if aplicar {
					_, clasificados, err := s.CrearRegla(ctx, empresaID, NuevaRegla{
						Nombre:          nombreRegla(f),
						AplicaA:         f.AplicaA,
						ConceptoID:      concepto.ID,
						ClasificacionID: clasif.ID,
						Prioridad:       f.Prioridad,
						Palabras:        f.Palabras,
					}, usuarioID)
					if err != nil {
						return PlanDiccionario{}, err
					}
					plan.Clasificados += clasificados
				}
			}
		}

		if !acc.CrearConcepto && !acc.CrearClasificacion && !acc.CrearRegla && !acc.DeclararNaturaleza {
			plan.SinCambios++
		}
		plan.Acciones = append(plan.Acciones, acc)
	}

	if aplicar {
		s.audit.Registrar(ctx, shared.Evento{
			EmpresaID: &empresaID, Entidad: "catalogo", Accion: "IMPORTAR_DICCIONARIO",
			UsuarioID: &usuarioID,
			ValorNuevo: map[string]int{
				"filas": plan.Filas, "conceptos": plan.ConceptosNuevos,
				"clasificaciones": plan.ClasificacionesNuevas, "reglas": plan.ReglasNuevas,
				"naturalezas":  plan.NaturalezasDeclaradas,
				"clasificados": plan.Clasificados,
			},
		})
	}
	return plan, nil
}

// nombreRegla arma un nombre legible para la regla del diccionario.
func nombreRegla(f FilaDiccionario) string {
	n := f.Palabras[0]
	if len([]rune(n)) > 60 {
		n = string([]rune(n)[:60])
	}
	return n
}

// ExportarDiccionario arma el .xlsx con el catálogo y sus palabras clave. El archivo que sale se
// puede volver a importar tal cual (ida y vuelta sin cambios).
func (s *Service) ExportarDiccionario(ctx context.Context, empresaID string) ([]byte, error) {
	conceptos, err := s.repo.ListarConceptos(ctx, empresaID, false)
	if err != nil {
		return nil, err
	}
	clasifs, err := s.repo.ListarClasificaciones(ctx, empresaID, false)
	if err != nil {
		return nil, err
	}
	reglas, err := s.repo.ListarReglas(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	// Reglas por clasificación destino (una clasificación puede tener varias).
	porClasif := map[string][]Regla{}
	for _, r := range reglas {
		if r.ClasificacionID != "" {
			porClasif[r.ClasificacionID] = append(porClasif[r.ClasificacionID], r)
		}
	}
	visible := map[string]bool{}
	// La naturaleza viaja SOLO si alguien la declaró. Si se exportara «No entra» para lo que nadie
	// decidió, reimportar el archivo declararía por él y la ida y vuelta inventaría una decisión.
	natural := map[string]string{}
	conClasif := map[string]bool{}
	for _, c := range conceptos {
		visible[c.ID] = c.VisibleCxP
		if c.NaturalezaDeclarada {
			natural[c.ID] = nombreNaturaleza(c.Naturaleza)
		}
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	idx, err := f.NewSheet(hojaDiccionario)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	encabezados := []string{"Concepto", "Clasificación", "Visible en CxP", "Naturaleza", "Palabras clave", "Aplica a", "Prioridad"}
	for i, h := range encabezados {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStr(hojaDiccionario, celda, h)
	}
	if estilo, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err == nil {
		_ = f.SetRowStyle(hojaDiccionario, 1, 1, estilo)
	}

	fila := 2
	escribir := func(vals []string) {
		for i, v := range vals {
			celda, _ := excelize.CoordinatesToCellName(i+1, fila)
			_ = f.SetCellStr(hojaDiccionario, celda, v)
		}
		fila++
	}
	siNo := func(b bool) string {
		if b {
			return "Sí"
		}
		return "No"
	}

	for _, cl := range clasifs {
		conClasif[cl.ConceptoID] = true
		rs := porClasif[cl.ID]
		if len(rs) == 0 {
			escribir([]string{cl.Concepto, cl.Nombre, siNo(visible[cl.ConceptoID]), natural[cl.ConceptoID], "", "", ""})
			continue
		}
		// Una fila por regla: así el archivo exportado se puede reimportar sin perder nada.
		for _, r := range rs {
			escribir([]string{cl.Concepto, cl.Nombre, siNo(visible[cl.ConceptoID]), natural[cl.ConceptoID],
				strings.Join(r.Palabras, separadorPalabras+" "), r.AplicaA, strconv.Itoa(r.Prioridad)})
		}
	}
	// Conceptos que todavía no tienen ninguna clasificación: también viajan.
	for _, c := range conceptos {
		if !conClasif[c.ID] {
			escribir([]string{c.Nombre, "", siNo(c.VisibleCxP), natural[c.ID], "", "", ""})
		}
	}

	_ = f.SetColWidth(hojaDiccionario, "A", "B", 28)
	_ = f.SetColWidth(hojaDiccionario, "C", "C", 14)
	_ = f.SetColWidth(hojaDiccionario, "D", "D", 20)
	_ = f.SetColWidth(hojaDiccionario, "E", "E", 52)
	_ = f.SetColWidth(hojaDiccionario, "F", "G", 12)
	_ = f.SetPanes(hojaDiccionario, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
