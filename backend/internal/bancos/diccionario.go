package bancos

// Diccionario del catálogo: exportar e importar Concepto › Clasificación con sus palabras clave.
//
// Es el «diccionario palabra_clave → (concepto, clasificación) exportable/importable» que pide la
// spec del motor. Sirve para tres cosas reales:
//   · llevarse el criterio de clasificación de una empresa a otra sin re-teclearlo,
//   · armar el catálogo en Excel (donde la gente trabaja) y subirlo de una vez,
//   · y que un concepto traiga sus palabras clave, o sea que se vuelva REGLA al importarlo.
//
// Importar no cambia el criterio del motor: la palabra clave se le entrega a la misma CrearRegla
// que ya retro-aplica sobre lo que está sin clasificar y sigue vigente el ≥90%-o-nada.

import (
	"errors"
	"strconv"
	"strings"
)

var (
	// ErrDiccionarioVacio indica que el archivo no trae ninguna fila utilizable.
	ErrDiccionarioVacio = errors.New("bancos: el archivo no trae filas con concepto")
	// ErrDiccionarioSinEncabezado indica que falta la columna Concepto.
	ErrDiccionarioSinEncabezado = errors.New("bancos: no se encontró el encabezado (se espera una columna «Concepto»)")
)

// separadorPalabras es `;` y NO la coma: las descripciones bancarias están llenas de comas y una
// palabra clave puede contenerlas («trf a cr17010402842201520116, davivienda»).
const separadorPalabras = ";"

// prioridadDiccionario es la prioridad con la que nacen las reglas del diccionario cuando el
// archivo no la indica. Los pasos de prioridad del motor son de 10.
const prioridadDiccionario = 100

// FilaDiccionario es una línea del archivo, ya normalizada.
type FilaDiccionario struct {
	Linea         int    `json:"linea"`
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
	VisibleCxP    bool   `json:"visible_cxp"`
	// Naturaleza: INGRESO, GASTO o NEUTRO si la fila la declara; vacío = la fila no dice nada y no
	// se toca lo que haya. Vacío NO significa NEUTRO: por eso el export deja la celda en blanco
	// cuando nadie declaró la naturaleza, y así la ida y vuelta no inventa una decisión.
	Naturaleza string   `json:"naturaleza"`
	Palabras   []string `json:"palabras"`
	AplicaA    string   `json:"aplica_a"`
	Prioridad  int      `json:"prioridad"`
	// Problema explica por qué la fila no se puede aplicar (vacío = está bien).
	Problema string `json:"problema"`
}

// AccionDiccionario es lo que se hará con una fila al aplicar el diccionario.
type AccionDiccionario struct {
	Linea         int    `json:"linea"`
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
	// CrearConcepto / CrearClasificacion / CrearRegla: qué falta y se va a crear.
	CrearConcepto      bool `json:"crear_concepto"`
	CrearClasificacion bool `json:"crear_clasificacion"`
	CrearRegla         bool `json:"crear_regla"`
	// DeclararNaturaleza: el concepto no tenía naturaleza declarada y la fila la declara.
	DeclararNaturaleza bool   `json:"declarar_naturaleza"`
	Naturaleza         string `json:"naturaleza"`
	// AvisoNaturaleza: la fila trae una naturaleza distinta de la ya declarada. NO se cambia —el
	// diccionario solo agrega— y acá se dice, porque cambiarla movería el EBITDA de todos los meses.
	AvisoNaturaleza string `json:"aviso_naturaleza"`
	Palabras        string `json:"palabras"`
	AplicaA         string `json:"aplica_a"`
	// Problema: la fila se omite y acá dice por qué.
	Problema string `json:"problema"`
}

// PlanDiccionario es el resultado de leer el archivo: qué se haría (o qué se hizo).
type PlanDiccionario struct {
	Filas    int                 `json:"filas"`
	Acciones []AccionDiccionario `json:"acciones"`
	// Resumen para decidir de un vistazo.
	ConceptosNuevos        int `json:"conceptos_nuevos"`
	ClasificacionesNuevas  int `json:"clasificaciones_nuevas"`
	ReglasNuevas           int `json:"reglas_nuevas"`
	NaturalezasDeclaradas  int `json:"naturalezas_declaradas"`
	NaturalezasEnConflicto int `json:"naturalezas_en_conflicto"`
	SinCambios             int `json:"sin_cambios"`
	Omitidas               int `json:"omitidas"`
	// Clasificados: movimientos que las reglas creadas dejaron clasificados (solo al aplicar).
	Clasificados int `json:"clasificados"`
	// Aplicado: false = fue una previsualización.
	Aplicado bool `json:"aplicado"`
}

// encabezadosDiccionario mapea el nombre de columna (normalizado) a su papel. Se aceptan varias
// grafías porque el archivo lo edita gente en Excel.
var encabezadosDiccionario = map[string]string{
	"concepto":         "concepto",
	"clasificacion":    "clasificacion",
	"clasificación":    "clasificacion",
	"subclasificacion": "clasificacion",
	"visible en cxp":   "visible_cxp",
	"visible cxp":      "visible_cxp",
	"palabras clave":   "palabras",
	"palabras":         "palabras",
	"palabra clave":    "palabras",
	"naturaleza":       "naturaleza",
	"en el ebitda":     "naturaleza",
	"ebitda":           "naturaleza",
	"aplica a":         "aplica_a",
	"aplica":           "aplica_a",
	"prioridad":        "prioridad",
}

// esSi acepta las formas en que la gente escribe «sí» en una hoja de cálculo.
func esSi(v string) bool {
	switch norm(v) {
	case "si", "sí", "s", "x", "true", "1", "verdadero", "yes":
		return true
	}
	return false
}

// aplicaANormalizado acepta DEBITO/CREDITO/MIXTO en cualquier grafía. Vacío o desconocido → MIXTO,
// que es el valor neutro que el motor ya usa por defecto.
func aplicaANormalizado(v string) string {
	switch norm(v) {
	case "debito", "débito", "d", "salida", "gasto":
		return "DEBITO"
	case "credito", "crédito", "c", "entrada", "ingreso":
		return "CREDITO"
	default:
		return "MIXTO"
	}
}

// naturalezaDeCelda lee la naturaleza escrita a mano en la hoja. Devuelve "" cuando la celda está
// vacía o no se entiende, y eso significa «la fila no declara nada»: NO significa NEUTRO.
//
// La diferencia importa. Si una celda vacía se leyera como NEUTRO, importar un catálogo declararía
// «no entra al EBITDA» sobre conceptos que nadie decidió — justo lo que la migración 0064 vino a
// separar. Acepta las formas en que la gente lo escribe, incluidas las etiquetas de la pantalla
// («↑ Ingreso», «— No entra»).
func naturalezaDeCelda(v string) string {
	switch norm(strings.TrimLeft(strings.TrimSpace(v), "↑↓—-– ")) {
	case "ingreso", "ingresos", "i":
		return NaturalezaIngreso
	case "gasto", "gastos", "g":
		return NaturalezaGasto
	case "neutro", "no entra", "no", "ninguno", "n", "no entra al ebitda":
		return NaturalezaNeutro
	default:
		return ""
	}
}

// palabrasDeCelda parte la celda por `;`, limpia y descarta las que no sirven como palabra clave.
func palabrasDeCelda(v string) []string {
	out := make([]string, 0, 4)
	for _, p := range strings.Split(v, separadorPalabras) {
		p = strings.TrimSpace(p)
		if len([]rune(norm(p))) < 3 {
			// Menos de 3 letras calzaría con demasiadas descripciones.
			continue
		}
		out = append(out, p)
	}
	return out
}

// LeerDiccionario interpreta la grilla del archivo (encabezado en cualquier fila de las primeras).
// Función pura: no toca base de datos.
func LeerDiccionario(g Grid) ([]FilaDiccionario, error) {
	col, filaEnc := columnasDiccionario(g)
	if col == nil {
		return nil, ErrDiccionarioSinEncabezado
	}
	filas := make([]FilaDiccionario, 0, 32)
	for i := filaEnc + 1; i < len(g); i++ {
		celda := func(papel string) string {
			idx, ok := col[papel]
			if !ok {
				return ""
			}
			return strings.TrimSpace(cell(g[i], idx))
		}
		concepto := celda("concepto")
		clasif := celda("clasificacion")
		palabras := palabrasDeCelda(celda("palabras"))
		if concepto == "" && clasif == "" && len(palabras) == 0 {
			continue // fila en blanco
		}
		f := FilaDiccionario{
			Linea:         i + 1,
			Concepto:      concepto,
			Clasificacion: clasif,
			VisibleCxP:    esSi(celda("visible_cxp")),
			Naturaleza:    naturalezaDeCelda(celda("naturaleza")),
			Palabras:      palabras,
			AplicaA:       aplicaANormalizado(celda("aplica_a")),
			Prioridad:     prioridadDiccionario,
		}
		if p, err := strconv.Atoi(strings.TrimSpace(celda("prioridad"))); err == nil && p > 0 {
			f.Prioridad = p
		}
		switch {
		case f.Concepto == "":
			f.Problema = "sin concepto"
		case len(f.Palabras) > 0 && f.Clasificacion == "":
			// Una regla necesita destino completo: el motor asigna Concepto Y Clasificación.
			f.Problema = "trae palabras clave pero no clasificación: la regla necesita el destino completo"
		}
		filas = append(filas, f)
	}
	if len(filas) == 0 {
		return nil, ErrDiccionarioVacio
	}
	return filas, nil
}

// columnasDiccionario busca el encabezado en las primeras filas y devuelve papel→índice.
func columnasDiccionario(g Grid) (map[string]int, int) {
	for i := 0; i < len(g) && i < 10; i++ {
		col := map[string]int{}
		for j, c := range g[i] {
			if papel, ok := encabezadosDiccionario[norm(c)]; ok {
				if _, repetido := col[papel]; !repetido {
					col[papel] = j
				}
			}
		}
		if _, hay := col["concepto"]; hay {
			return col, i
		}
	}
	return nil, 0
}
