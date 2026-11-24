package conta

// Importar el plan de cuentas desde el Excel del contador.
//
// EL ARCHIVO REAL que esto tiene que leer es `CATALOGO DF.1.xlsx`: 258 cuentas, código de cinco
// segmentos, y DOS columnas de catálogo —la propuesta y la de Softland— porque el Director
// Financiero está migrando de una a otra. De ahí salen las decisiones de este lector:
//
//   · Se leen las columnas por su ENCABEZADO, no por su posición. El archivo tiene una fila de
//     título antes de los encabezados y una columna vacía de separación; contar posiciones a mano
//     es lo que rompe el día que alguien agregue una columna.
//   · El código de Softland se guarda aparte. Es el puente a los saldos iniciales (Fase 4) y
//     perderlo obliga a rehacer el cruce a mano sobre 258 cuentas.

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// FilaImportada es una cuenta leída del archivo, antes de armar el árbol.
type FilaImportada struct {
	Fila           int    `json:"fila"`
	Codigo         string `json:"codigo"`
	Nombre         string `json:"nombre"`
	CodigoSoftland string `json:"codigo_softland"`
	// Problema explica por qué esta fila no se va a importar. Vacío = entra.
	Problema string `json:"problema"`
}

// ResultadoImportacion es lo que pasó.
type ResultadoImportacion struct {
	Creadas      int `json:"creadas"`
	Actualizadas int `json:"actualizadas"`
	// DeMovimiento son las que quedaron aceptando asientos: las hojas del árbol.
	DeMovimiento int             `json:"de_movimiento"`
	Descartadas  []FilaImportada `json:"descartadas"`
}

// PrevisualizacionImportacion es lo que se muestra ANTES de escribir nada.
//
// Existe por la misma razón que en el importador de bancos: nadie firma sobre una lista que no
// vio. Un plan de cuentas mal cargado se arrastra a todos los asientos que vengan después.
type PrevisualizacionImportacion struct {
	Filas []FilaImportada `json:"filas"`
	// Totales de lo que SE VA a hacer si se confirma.
	ACrear       int `json:"a_crear"`
	AActualizar  int `json:"a_actualizar"`
	DeMovimiento int `json:"de_movimiento"`
	ConProblema  int `json:"con_problema"`
	// PorClase dice cuántas cuentas de cada clase trae el archivo. Es el control de cordura más
	// barato: un plan sin ingresos o sin patrimonio salta a la vista.
	PorClase map[string]int `json:"por_clase"`
}

// encabezadosPlan reconoce las columnas por su nombre, normalizado.
//
// Se aceptan las grafías que la gente escribe de verdad. El del archivo del Director Financiero
// dice «Cuenta» y «Descripción»; otros planes dicen «Código» y «Nombre».
var encabezadosPlan = map[string]string{
	"cuenta":          "codigo",
	"codigo":          "codigo",
	"código":          "codigo",
	"cuenta contable": "codigo",
	"descripcion":     "nombre",
	"descripción":     "nombre",
	"nombre":          "nombre",
	"detalle":         "nombre",
}

func norm(s string) string {
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n")
	return r.Replace(strings.ToLower(strings.TrimSpace(s)))
}

// LeerPlanDesdeExcel saca las cuentas del archivo.
//
// Devuelve TODAS las filas con contenido, cada una con su problema si lo tiene, para que la
// previsualización pueda mostrar lo que entra y lo que no. Solo falla si el archivo no se puede
// abrir o si no se encuentra el encabezado: eso no es una fila mala, es otro archivo.
func LeerPlanDesdeExcel(archivo []byte) ([]FilaImportada, error) {
	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return nil, fmt.Errorf("conta: no se pudo abrir el archivo: %w", err)
	}
	defer func() { _ = f.Close() }()

	hojas := f.GetSheetList()
	if len(hojas) == 0 {
		return nil, fmt.Errorf("conta: el archivo no tiene hojas")
	}
	filas, err := f.GetRows(hojas[0])
	if err != nil {
		return nil, fmt.Errorf("conta: no se pudieron leer las filas: %w", err)
	}

	// La fila de encabezado es la primera que nombra a la vez el código y el nombre. Buscarla en
	// vez de asumir la 1 es lo que tolera los títulos de arriba que traen estos archivos.
	colCodigo, colNombre, filaEnc := -1, -1, -1
	for i := 0; i < len(filas) && i < 25; i++ {
		c, n := -1, -1
		for j, celda := range filas[i] {
			switch encabezadosPlan[norm(celda)] {
			case "codigo":
				if c < 0 {
					c = j
				}
			case "nombre":
				if n < 0 {
					n = j
				}
			}
		}
		if c >= 0 && n >= 0 {
			colCodigo, colNombre, filaEnc = c, n, i
			break
		}
	}
	if filaEnc < 0 {
		return nil, fmt.Errorf("conta: no se encontró el encabezado; hace falta una columna de código (p. ej. «Cuenta») y una de nombre (p. ej. «Descripción»)")
	}

	// La SEGUNDA pareja de columnas, si existe, es el catálogo de Softland: el archivo del
	// Director Financiero pone la propuesta a la izquierda y Softland a la derecha.
	colSoftland := -1
	for j, celda := range filas[filaEnc] {
		if j > colCodigo && encabezadosPlan[norm(celda)] == "codigo" {
			colSoftland = j
			break
		}
	}
	// Si el encabezado de Softland no está rotulado —el archivo real lo rotula una fila más
	// arriba— se toma la primera columna a la derecha del nombre que traiga códigos.
	if colSoftland < 0 {
		colSoftland = columnaConCodigos(filas, filaEnc+1, colNombre+1)
	}

	out := make([]FilaImportada, 0, len(filas))
	for i := filaEnc + 1; i < len(filas); i++ {
		cod := strings.TrimSpace(celda(filas[i], colCodigo))
		nom := strings.TrimSpace(celda(filas[i], colNombre))
		if cod == "" && nom == "" {
			continue
		}
		fi := FilaImportada{
			Fila: i + 1, Codigo: cod, Nombre: nom,
			CodigoSoftland: strings.TrimSpace(celda(filas[i], colSoftland)),
		}
		switch {
		case cod == "":
			fi.Problema = "sin código"
		case nom == "":
			fi.Problema = "sin nombre"
		case !strings.Contains(cod, "-"):
			// Una celda sin guiones en la columna del código es un subtítulo del archivo, no una
			// cuenta. Se descarta con su motivo en vez de intentar importarla.
			fi.Problema = "no parece un código de cuenta"
		default:
			if _, ok := claseDelCodigo(cod); !ok {
				fi.Problema = "el código no empieza por una clase contable (1 a 7)"
			}
		}
		out = append(out, fi)
	}
	return out, nil
}

// columnaConCodigos busca, a la derecha de `desde`, la primera columna cuyas celdas parezcan
// códigos de cuenta. Devuelve -1 si no hay ninguna.
func columnaConCodigos(filas [][]string, desdeFila, desdeCol int) int {
	mejor, mejorN := -1, 0
	for j := desdeCol; j < 40; j++ {
		n := 0
		for i := desdeFila; i < len(filas) && i < desdeFila+30; i++ {
			c := strings.TrimSpace(celda(filas[i], j))
			if c != "" && strings.Contains(c, "-") {
				if _, ok := claseDelCodigo(c); ok {
					n++
				}
			}
		}
		if n > mejorN {
			mejor, mejorN = j, n
		}
	}
	// Al menos tres aciertos: una columna con un código suelto es casualidad.
	if mejorN < 3 {
		return -1
	}
	return mejor
}

func celda(fila []string, i int) string {
	if i < 0 || i >= len(fila) {
		return ""
	}
	return fila[i]
}

// ArmarPlan convierte las filas leídas en cuentas con su árbol resuelto.
//
// LAS DOS DECISIONES QUE TOMA, y que son el corazón del importador:
//
//  1. QUIÉN ES LA MADRE. Se deriva del código poniendo en ceros el último segmento significativo
//     («1-01-01-001-002» → «1-01-01-001-000»). Si esa cuenta no viene en el archivo, se sube un
//     escalón más, y así hasta la raíz. Eso tolera los planes que saltan niveles, que son casi
//     todos.
//  2. QUIÉN ACEPTA MOVIMIENTO. Las HOJAS: las que nadie tiene como madre. Es la regla del plan de
//     cuentas y la única forma de que el árbol totalice sin contar dos veces el mismo colón.
//
// Las cuentas salen ORDENADAS POR NIVEL, de la raíz hacia abajo. No es cosmético: la madre tiene
// que existir en la base antes que la hija, o la llave foránea la rechaza.
func ArmarPlan(filas []FilaImportada) []Cuenta {
	// Índice por código de lo que SÍ entra.
	porCodigo := map[string]*Cuenta{}
	orden := make([]*Cuenta, 0, len(filas))
	for _, f := range filas {
		if f.Problema != "" {
			continue
		}
		clase, _ := claseDelCodigo(f.Codigo)
		c := &Cuenta{
			Codigo: f.Codigo, Nombre: f.Nombre, CodigoSoftland: f.CodigoSoftland,
			Clase: clase, Naturaleza: naturalezaDeLaClase(clase),
			Nivel: nivelDelCodigo(f.Codigo), Activo: true,
		}
		// Un código repetido dentro del mismo archivo se ignora: la primera gana. Insertar las dos
		// reventaría contra el índice único a mitad de la importación.
		if _, ya := porCodigo[c.Codigo]; ya {
			continue
		}
		porCodigo[c.Codigo] = c
		orden = append(orden, c)
	}

	// Quién tiene madre, y quién es madre de alguien.
	esMadre := map[string]bool{}
	padreDe := map[string]string{}
	for _, c := range orden {
		p := codigoDelPadre(c.Codigo)
		for p != "" {
			if _, hay := porCodigo[p]; hay {
				padreDe[c.Codigo] = p
				esMadre[p] = true
				break
			}
			p = codigoDelPadre(p)
		}
	}

	// Hoja = nadie la tiene como madre. Esas y solo esas aceptan movimiento.
	for _, c := range orden {
		c.AceptaMovimiento = !esMadre[c.Codigo]
	}

	// De la raíz hacia abajo, para que la madre exista antes que la hija.
	sort.SliceStable(orden, func(i, j int) bool {
		if orden[i].Nivel != orden[j].Nivel {
			return orden[i].Nivel < orden[j].Nivel
		}
		return orden[i].Codigo < orden[j].Codigo
	})

	out := make([]Cuenta, 0, len(orden))
	for _, c := range orden {
		cc := *c
		if p, hay := padreDe[c.Codigo]; hay {
			// El id real lo resuelve el repositorio dentro de la transacción; acá viaja el CÓDIGO
			// de la madre, que es lo único que el archivo conoce.
			codigo := p
			cc.PadreID = &codigo
		}
		out = append(out, cc)
	}
	return out
}

// Previsualizar lee el archivo y dice qué pasaría, sin tocar la base.
func (s *Service) Previsualizar(ctx context.Context, empresaID string, archivo []byte) (PrevisualizacionImportacion, error) {
	filas, err := LeerPlanDesdeExcel(archivo)
	if err != nil {
		return PrevisualizacionImportacion{}, err
	}
	plan := ArmarPlan(filas)

	existentes, err := s.repo.Cuentas(ctx, empresaID, false)
	if err != nil {
		return PrevisualizacionImportacion{}, err
	}
	ya := make(map[string]bool, len(existentes))
	for _, c := range existentes {
		ya[c.Codigo] = true
	}

	p := PrevisualizacionImportacion{Filas: filas, PorClase: map[string]int{}}
	for _, f := range filas {
		if f.Problema != "" {
			p.ConProblema++
		}
	}
	for _, c := range plan {
		p.PorClase[c.Clase]++
		if c.AceptaMovimiento {
			p.DeMovimiento++
		}
		if ya[c.Codigo] {
			p.AActualizar++
		} else {
			p.ACrear++
		}
	}
	return p, nil
}

// Importar escribe el plan. `alGrupo` decide de quién es: del grupo o de la empresa activa.
//
// LA DECISIÓN DE `alGrupo` ES LA QUE EL MANDATO LLAMA D1 («¿plan único o uno por empresa?»), y
// acá se toma al cargar, no al diseñar: la misma pantalla sirve para las dos cosas y cambiar de
// opinión es volver a importar, no migrar el modelo.
func (s *Service) Importar(ctx context.Context, empresaID string, archivo []byte, alGrupo bool, usuarioID string) (ResultadoImportacion, error) {
	filas, err := LeerPlanDesdeExcel(archivo)
	if err != nil {
		return ResultadoImportacion{}, err
	}
	plan := ArmarPlan(filas)

	var destino *string
	if !alGrupo {
		emp := empresaID
		destino = &emp
	}
	res, err := s.repo.ImportarCuentas(ctx, destino, plan, usuarioID)
	if err != nil {
		return ResultadoImportacion{}, err
	}
	for _, f := range filas {
		if f.Problema != "" {
			res.Descartadas = append(res.Descartadas, f)
		}
	}
	s.auditar(ctx, empresaID, "", "IMPORTAR_PLAN", usuarioID, map[string]any{
		"creadas": res.Creadas, "actualizadas": res.Actualizadas,
		"de_movimiento": res.DeMovimiento, "descartadas": len(res.Descartadas),
		"al_grupo": alGrupo,
	})
	return res, nil
}
