package precios

// Importador de PLANES y sus RECETAS desde la hoja de costeo.
//
// ── LA FORMA DEL ARCHIVO ────────────────────────────────────────────────────────────────────
//
// La hoja «Costos» es una matriz, no una lista: cada fila es un insumo y cada columna un plan.
// La celda donde se cruzan es la CANTIDAD que ese plan lleva de ese insumo.
//
//            │ Jade │ Bronce │ Plata │ Oro │ …        ← fila de encabezado con los planes
//   ─────────┼──────┼────────┼───────┼─────┤
//   Cofre #24│      │        │       │  1  │          ← una línea de receta por celda con número
//   Flores   │  2   │   2    │   3   │  4  │
//
// Debajo de los insumos, una fila dice el PRECIO DE VENTA de cada plan. De ahí salen los planes
// con su precio en la misma pasada.
//
// ── LO QUE EL IMPORTADOR NO DECIDE SOLO ─────────────────────────────────────────────────────
//
// Si un plan se cobra de una vez o por cuota mensual. Lo PROPONE mirando el monto —un servicio
// funerario completo no cuesta ₡4.950, así que ese número solo puede ser una cuota— y lo muestra
// antes de escribir, porque de esa distinción depende contra qué se compara el costo: el precio
// de una venta, o la cuota por los meses que dura el contrato.

import (
	"bytes"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// umbralCuotaMensual separa un precio de venta de una cuota. El plan directo más barato del
// archivo vale ₡500.000 y la cuota más cara ₡10.000: cualquier corte entre esos dos valores
// clasifica igual. Se elige ₡50.000 por quedar lejos de ambos extremos.
var umbralCuotaMensual = decimal.NewFromInt(50000)

// PlanDetectado es un plan encontrado en el encabezado de la matriz.
type PlanDetectado struct {
	Nombre    string  `json:"nombre"`
	Codigo    string  `json:"codigo"`
	Familia   string  `json:"familia"`
	Columna   string  `json:"columna"`
	Modalidad string  `json:"modalidad"`
	Precio    *string `json:"precio"`
	Lineas    int     `json:"lineas"`
}

// AnalisisRecetas es lo que se muestra antes de escribir.
type AnalisisRecetas struct {
	Hoja            string          `json:"hoja"`
	HojasDelLibro   []string        `json:"hojas_del_libro"`
	FilaEncabezado  int             `json:"fila_encabezado"`
	FilaPrecio      int             `json:"fila_precio"`
	Planes          []PlanDetectado `json:"planes"`
	Insumos         int             `json:"insumos"`
	InsumosConCosto int             `json:"insumos_con_costo"`
	LineasDeReceta  int             `json:"lineas_de_receta"`
	Muestra         []string        `json:"muestra"`
}

// nombresDePlanConocidos son las etiquetas que marcan una columna de plan en el encabezado. No es
// una lista de los planes de la empresa —esos salen del archivo—, sino de las palabras que NO son
// un plan y hay que saltar cuando aparecen en la misma fila.
var noSonPlan = map[string]bool{
	"productos": true, "costo": true, "precio incluye iva": true, "proveedor": true,
	"consumo": true, "costeo por servicio": true, "": true,
}

// filaDe devuelve una fila del grid o nil.
func filaDe(rows [][]string, n int) []string {
	if n-1 < 0 || n-1 >= len(rows) {
		return nil
	}
	return rows[n-1]
}

func celda(fila []string, i int) string {
	if i < 0 || i >= len(fila) {
		return ""
	}
	return strings.TrimSpace(fila[i])
}

// buscarFila encuentra la primera fila cuya columna A contenga el texto dado.
func buscarFila(rows [][]string, texto string) int {
	t := strings.ToLower(texto)
	for i, r := range rows {
		if strings.Contains(strings.ToLower(celda(r, 0)), t) {
			return i + 1
		}
	}
	return 0
}

// buscarEncabezadoDePlanes busca la fila que lista los planes: es la que tiene «PRODUCTOS» en la
// columna A. Se busca por contenido y no por número fijo porque una fila insertada arriba
// correría todo y el importador leería el archivo equivocado sin avisar.
func buscarEncabezadoDePlanes(rows [][]string) int {
	for i, r := range rows {
		if strings.EqualFold(celda(r, 0), "PRODUCTOS") {
			return i + 1
		}
	}
	return 0
}

// AnalizarRecetas lee la matriz y mide qué planes y qué líneas de receta traería.
func AnalizarRecetas(archivo []byte, hoja string) (AnalisisRecetas, error) {
	hojas, err := LeerLibro(archivo)
	if err != nil {
		return AnalisisRecetas{}, err
	}
	if hoja == "" {
		// Por defecto, la hoja que se llame «Costos»; si no está, la primera.
		hoja = hojas[0]
		for _, h := range hojas {
			if strings.EqualFold(h, "Costos") {
				hoja = h
			}
		}
	}
	existe := false
	for _, h := range hojas {
		if h == hoja {
			existe = true
		}
	}
	if !existe {
		return AnalisisRecetas{}, ErrHojaNoEncontrada
	}

	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return AnalisisRecetas{}, ErrArchivoIlegible
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(hoja)
	if err != nil {
		return AnalisisRecetas{}, ErrArchivoIlegible
	}

	filaEnc := buscarEncabezadoDePlanes(rows)
	if filaEnc == 0 {
		return AnalisisRecetas{}, ErrEncabezadoDePlanesNoHallado
	}
	enc := filaDe(rows, filaEnc)
	familias := filaDe(rows, filaEnc-1)
	filaPrecio := buscarFila(rows, "precio de venta")

	// Las columnas de CONSUMO son las que están antes del segundo bloque. El segundo bloque
	// («COSTEO POR SERVICIO») repite los mismos nombres de plan, así que sin este corte cada plan
	// aparecería dos veces y las cantidades se leerían del bloque equivocado.
	corte := len(enc)
	if cab := filaDe(rows, filaEnc-3); cab != nil {
		for i, c := range cab {
			if strings.Contains(strings.ToUpper(strings.TrimSpace(c)), "COSTEO") {
				corte = i
				break
			}
		}
	}

	a := AnalisisRecetas{
		Hoja: hoja, HojasDelLibro: hojas,
		FilaEncabezado: filaEnc, FilaPrecio: filaPrecio,
		Planes: []PlanDetectado{}, Muestra: []string{},
	}

	familiaActual := ""
	for i := 1; i < corte && i < len(enc); i++ {
		if fam := celda(familias, i); fam != "" {
			familiaActual = fam
		}
		nombre := celda(enc, i)
		if noSonPlan[strings.ToLower(nombre)] {
			continue
		}
		col, _ := excelize.ColumnNumberToName(i + 1)
		p := PlanDetectado{
			Nombre: nombre, Codigo: codigoDesde(nombre, ""),
			Familia: familiaActual, Columna: col, Modalidad: ModalidadDirecto,
		}
		if filaPrecio > 0 {
			// El precio vive en el bloque de COSTEO, no en el de consumo: se busca por nombre de
			// plan en el encabezado de ese bloque.
			if monto := precioDelPlan(rows, filaEnc, filaPrecio, nombre, corte); monto != nil {
				s := monto.StringFixed(2)
				p.Precio = &s
				if monto.LessThan(umbralCuotaMensual) {
					p.Modalidad = ModalidadSuscripcion
				}
			}
		}
		a.Planes = append(a.Planes, p)
	}
	if len(a.Planes) == 0 {
		return AnalisisRecetas{}, ErrEncabezadoDePlanesNoHallado
	}

	// Las filas de insumo: desde la siguiente al encabezado hasta la del precio (o el final).
	fin := len(rows)
	if filaPrecio > 0 {
		fin = filaPrecio - 1
	}
	for n := filaEnc + 1; n <= fin && n <= len(rows); n++ {
		r := filaDe(rows, n)
		nombre := celda(r, 0)
		if nombre == "" || strings.EqualFold(nombre, "COSTO POR SERVICIO") {
			continue
		}
		a.Insumos++
		if normalizarMonto(celda(r, 1)) != nil {
			a.InsumosConCosto++
		}
		if len(a.Muestra) < 10 {
			a.Muestra = append(a.Muestra, nombre)
		}
		for idx := range a.Planes {
			i, _ := excelize.ColumnNameToNumber(a.Planes[idx].Columna)
			if q := normalizarMonto(celda(r, i-1)); q != nil {
				a.Planes[idx].Lineas++
				a.LineasDeReceta++
			}
		}
	}
	return a, nil
}

// precioDelPlan busca el precio de un plan en la fila de precios, usando el encabezado del bloque
// de costeo (el que está después del corte).
func precioDelPlan(rows [][]string, filaEnc, filaPrecio int, plan string, corte int) *decimal.Decimal {
	enc := filaDe(rows, filaEnc)
	fp := filaDe(rows, filaPrecio)
	// Primero en el bloque de costeo (después del corte), que es donde el archivo pone los montos.
	for i := corte; i < len(enc); i++ {
		if strings.EqualFold(celda(enc, i), plan) {
			if v := normalizarMonto(celda(fp, i)); v != nil {
				return v
			}
		}
	}
	// Algunos archivos repiten el encabezado justo arriba de la fila de precios.
	if enc2 := filaDe(rows, filaPrecio-2); enc2 != nil {
		for i := range enc2 {
			if strings.EqualFold(celda(enc2, i), plan) {
				if v := normalizarMonto(celda(fp, i)); v != nil {
					return v
				}
			}
		}
	}
	return nil
}

// LineaDeRecetaAImportar es una celda de la matriz convertida en línea.
type LineaDeRecetaAImportar struct {
	PlanCodigo   string  `json:"plan_codigo"`
	InsumoCodigo string  `json:"insumo_codigo"`
	InsumoNombre string  `json:"insumo_nombre"`
	Cantidad     string  `json:"cantidad"`
	Costo        *string `json:"costo"`
}

// PlanDeRecetas es lo que se va a escribir.
type PlanDeRecetas struct {
	Planes  []PlanDetectado          `json:"planes"`
	Insumos []InsumoAImportar        `json:"insumos"`
	Lineas  []LineaDeRecetaAImportar `json:"lineas"`
}

// PlanificarRecetas arma planes, insumos y líneas listos para escribir.
//
// Crea también los insumos, porque los de esta hoja no son los mismos que los del catálogo de
// precios: son los componentes del servicio. El upsert por código hace que los que ya existan se
// reusen en vez de duplicarse.
func PlanificarRecetas(archivo []byte, hoja string, modalidades map[string]string) (PlanDeRecetas, error) {
	a, err := AnalizarRecetas(archivo, hoja)
	if err != nil {
		return PlanDeRecetas{}, err
	}

	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return PlanDeRecetas{}, ErrArchivoIlegible
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(a.Hoja)
	if err != nil {
		return PlanDeRecetas{}, ErrArchivoIlegible
	}

	p := PlanDeRecetas{Planes: a.Planes, Insumos: []InsumoAImportar{}, Lineas: []LineaDeRecetaAImportar{}}
	// La modalidad que confirme el usuario manda sobre la inferida por monto.
	for i := range p.Planes {
		if m, ok := modalidades[p.Planes[i].Codigo]; ok {
			if m == ModalidadDirecto || m == ModalidadSuscripcion {
				p.Planes[i].Modalidad = m
			}
		}
	}

	fin := len(rows)
	if a.FilaPrecio > 0 {
		fin = a.FilaPrecio - 1
	}
	vistos := map[string]bool{}
	for n := a.FilaEncabezado + 1; n <= fin && n <= len(rows); n++ {
		r := filaDe(rows, n)
		nombre := celda(r, 0)
		if nombre == "" || strings.EqualFold(nombre, "COSTO POR SERVICIO") {
			continue
		}
		cod := codigoDesde(nombre, "")
		costo := normalizarMonto(celda(r, 1))

		if !vistos[cod] {
			vistos[cod] = true
			ins := InsumoAImportar{Codigo: cod, Nombre: nombre}
			if costo != nil {
				s := costo.StringFixed(2)
				ins.Costo = &s
			}
			p.Insumos = append(p.Insumos, ins)
		}

		for _, pl := range a.Planes {
			i, _ := excelize.ColumnNameToNumber(pl.Columna)
			q := normalizarMonto(celda(r, i-1))
			if q == nil {
				continue
			}
			l := LineaDeRecetaAImportar{
				PlanCodigo: pl.Codigo, InsumoCodigo: cod, InsumoNombre: nombre,
				Cantidad: q.String(),
			}
			if costo != nil {
				s := costo.StringFixed(2)
				l.Costo = &s
			}
			p.Lineas = append(p.Lineas, l)
		}
	}
	return p, nil
}
