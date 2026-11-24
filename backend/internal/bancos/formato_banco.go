package bancos

// Formatos de banco configurables (mig 0108).
//
// Un formato guardado en la base se convierte acá en un `bankSpec` idéntico a los seis que viven
// en `specs.go`: el MISMO tipo, el mismo `Parsea`, los mismos helpers de fecha y monto. No hay un
// segundo lector con sus propias reglas —esa sería la forma segura de que un día lean distinto el
// mismo archivo—; lo único nuevo es de dónde salen los números.

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// FormatoBanco es un formato tal como se configura desde la pantalla.
type FormatoBanco struct {
	ID                 string            `json:"id"`
	Nombre             string            `json:"nombre"`
	Activo             bool              `json:"activo"`
	TextoIdentificador string            `json:"texto_identificador"`
	ColFecha           int               `json:"col_fecha"`
	ColDocumento       int               `json:"col_documento"`
	ColDescripcion     int               `json:"col_descripcion"`
	ColDebito          int               `json:"col_debito"`
	ColCredito         int               `json:"col_credito"`
	FormatoFecha       string            `json:"formato_fecha"`
	Senales            []SenalEncabezado `json:"senales"`
	CreadoEn           time.Time         `json:"creado_en"`
}

// SenalEncabezado es una condición sobre la fila de encabezado: «la columna N contiene este texto».
//
// El texto se compara NORMALIZADO y por contenido, no exacto: los encabezados reales traen
// acentos inconsistentes, mayúsculas cambiantes y sufijos («Débitos (DR)»). Exigir igualdad
// obligaría a transcribir la celda carácter por carácter, que es justo lo que nadie acierta.
type SenalEncabezado struct {
	Columna int    `json:"columna"`
	Texto   string `json:"texto"`
}

// ── Formatos de fecha ──────────────────────────────────────────────────────────────────────
//
// Lista CERRADA, con nombre humano. No se guarda un layout de Go («01-02-06») porque pedirle eso
// a quien configura el sistema es pedirle que adivine una convención interna: el 1 es el mes y el
// 2 es el día, y equivocarse produce un archivo que se lee entero con las fechas cambiadas —el
// mismo defecto que ya costó caro con DateStyle en las claves de Hacienda—.
const (
	FechaDDMMAAAA  = "DD/MM/AAAA"
	FechaMMDDAAAA  = "MM/DD/AAAA"
	FechaDDMMAA    = "DD-MM-AA"
	FechaMMDDAA    = "MM-DD-AA"
	FechaISO       = "AAAA-MM-DD"
	FechaDDMesAAAA = "DD MES AAAA"
)

// FormatosDeFecha son los que la pantalla ofrece, con un ejemplo de cómo se ve cada uno.
//
// Los seis bancos de código usan tres de estos; los otros tres están porque son los que aparecen
// cuando un portal exporta en otro idioma o con otra configuración regional.
var FormatosDeFecha = []struct {
	Clave   string `json:"clave"`
	Ejemplo string `json:"ejemplo"`
}{
	{FechaDDMMAAAA, "22/10/2026"},
	{FechaMMDDAAAA, "10/22/2026"},
	{FechaDDMMAA, "22-10-26"},
	{FechaMMDDAA, "10-22-26"},
	{FechaISO, "2026-10-22"},
	{FechaDDMesAAAA, "22 OCT 2026"},
}

// parserDeFecha traduce la clave al parser real. Los mismos helpers que usan los seis de código.
func parserDeFecha(clave string) (func(string) (time.Time, error), bool) {
	switch clave {
	case FechaDDMMAAAA:
		return fechaLayout("02/01/2006"), true
	case FechaMMDDAAAA:
		return fechaLayout("01/02/2006"), true
	case FechaDDMMAA:
		return fechaLayout("02-01-06"), true
	case FechaMMDDAA:
		return fechaLayout("01-02-06"), true
	case FechaISO:
		return fechaLayout("2006-01-02"), true
	case FechaDDMesAAAA:
		// El mismo parser del Banco Popular: acepta el mes en español y en inglés, y tolera la
		// hora al final. Ver mesesAbrev.
		return fechaBP, true
	}
	return nil, false
}

// ── Validación ─────────────────────────────────────────────────────────────────────────────

var (
	// ErrFormatoSinNombre: el nombre es lo que queda escrito en cada importación.
	ErrFormatoSinNombre = errors.New("bancos: el formato necesita un nombre (es el que queda en cada carga)")
	// ErrFormatoFechaDesconocida: la clave tiene que ser una de la lista cerrada.
	ErrFormatoFechaDesconocida = errors.New("bancos: elegí uno de los formatos de fecha de la lista")
	// ErrFormatoColumnaInvalida: una columna negativa (salvo el documento) no existe.
	ErrFormatoColumnaInvalida = errors.New("bancos: la fecha, la descripción, el débito y el crédito necesitan una columna")
	// ErrFormatoDebilParaDetectar: el formato calzaría con demasiados archivos. Ver abajo.
	ErrFormatoDebilParaDetectar = errors.New("bancos: hacen falta al menos dos señales del encabezado, o una señal más el texto que identifica al banco; si no, este formato va a reclamar archivos de otros bancos")
	// ErrFormatoSenalVacia: una señal sin texto se cumple siempre.
	ErrFormatoSenalVacia = errors.New("bancos: cada señal necesita el texto que aparece en esa columna del encabezado")
)

// topeSenales acota la lista. No es una regla de negocio: tres o cuatro señales identifican
// cualquier encabezado real, y una lista sin tope es una forma de guardar un documento entero en
// una columna de configuración.
const topeSenales = 10

// Validar comprueba que el formato pueda reconocer un archivo sin reclamar los de otros.
//
// LA REGLA QUE IMPORTA es la última: un formato con una sola señal y sin texto identificador
// calza con media docena de bancos —«la columna 0 dice fecha» lo cumplen todos—, y como los
// configurados se prueban ANTES que los de código, ese formato le robaría los archivos a los seis
// que hoy funcionan. Dos condiciones no es burocracia: es lo que lo vuelve específico.
func (f *FormatoBanco) Validar() error {
	f.Nombre = strings.TrimSpace(f.Nombre)
	f.TextoIdentificador = strings.TrimSpace(f.TextoIdentificador)
	if f.Nombre == "" {
		return ErrFormatoSinNombre
	}
	if _, ok := parserDeFecha(f.FormatoFecha); !ok {
		return ErrFormatoFechaDesconocida
	}
	// El documento admite -1: hay bancos que no publican ninguna referencia, y en esos la celda
	// queda vacía (ver `cell`, que devuelve "" fuera de rango). Los otros cuatro son obligatorios.
	if f.ColFecha < 0 || f.ColDescripcion < 0 || f.ColDebito < 0 || f.ColCredito < 0 {
		return ErrFormatoColumnaInvalida
	}
	if len(f.Senales) > topeSenales {
		f.Senales = f.Senales[:topeSenales]
	}
	for i := range f.Senales {
		f.Senales[i].Texto = strings.TrimSpace(f.Senales[i].Texto)
		if f.Senales[i].Texto == "" || f.Senales[i].Columna < 0 {
			return ErrFormatoSenalVacia
		}
	}
	if len(f.Senales) < 2 && !(len(f.Senales) == 1 && f.TextoIdentificador != "") {
		return ErrFormatoDebilParaDetectar
	}
	return nil
}

// ── De configuración a adaptador ───────────────────────────────────────────────────────────

// Adaptador arma el lector a partir del formato guardado.
//
// Devuelve el MISMO `bankSpec` que usan los seis de código, así que el archivo se parsea con
// exactamente el mismo recorrido: encabezado, filas, descarte de las que no traen fecha, conteo
// de las ilegibles y aviso si no quedó ninguna.
func (f FormatoBanco) Adaptador() (Adapter, error) {
	parser, ok := parserDeFecha(f.FormatoFecha)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrFormatoFechaDesconocida, f.FormatoFecha)
	}
	// Las señales se normalizan UNA vez, al construir, y no en cada fila: `detect` las evalúa
	// contra las primeras 25 filas de cada archivo.
	senales := make([]SenalEncabezado, 0, len(f.Senales))
	for _, s := range f.Senales {
		senales = append(senales, SenalEncabezado{Columna: s.Columna, Texto: norm(s.Texto)})
	}
	return &bankSpec{
		banco:      Banco(f.Nombre),
		sigToken:   norm(f.TextoIdentificador),
		colFecha:   f.ColFecha,
		colDoc:     f.ColDocumento,
		colDesc:    f.ColDescripcion,
		colDebito:  f.ColDebito,
		colCredito: f.ColCredito,
		fecha:      parser,
		isHeader: func(c []string) bool {
			// Sin señales no se reconoce NADA. Devolver true sería un formato que reclama la
			// primera fila de cualquier archivo; `Validar` ya lo impide, y esto lo sostiene
			// aunque alguien escriba la fila directo en la base.
			if len(senales) == 0 {
				return false
			}
			for _, s := range senales {
				if !cellHas(c, s.Columna, s.Texto) {
					return false
				}
			}
			return true
		},
	}, nil
}

// AdaptadoresDe convierte los formatos ACTIVOS en adaptadores, descartando los que no se pueden
// construir.
//
// Un formato roto se SALTA en vez de tumbar la importación: lo que está en juego es que alguien
// pueda cargar su estado de cuenta, y un formato mal guardado no puede impedírselo cuando los
// seis de siempre siguen ahí. Se devuelve aparte cuántos se saltaron para poder decirlo.
func AdaptadoresDe(formatos []FormatoBanco) ([]Adapter, int) {
	out := make([]Adapter, 0, len(formatos))
	rotos := 0
	for _, f := range formatos {
		if !f.Activo {
			continue
		}
		a, err := f.Adaptador()
		if err != nil {
			rotos++
			continue
		}
		out = append(out, a)
	}
	return out, rotos
}

// SenalQueNoCalza es una señal que no se cumple, con lo que de verdad hay en esa columna.
//
// Es lo que convierte «no se reconoció el archivo» en algo accionable: quien configura ve que
// pidió «Descripcion» en la columna 2 y que ahí dice «Documento», y lo corrige en diez segundos.
type SenalQueNoCalza struct {
	Columna     int    `json:"columna"`
	Pedido      string `json:"pedido"`
	Encontrado  string `json:"encontrado"`
	FilaTitulos int    `json:"fila_titulos"`
}

// SenalesQueNoCalzan compara las señales del formato contra la fila que más parece el encabezado.
//
// POR QUÉ HACE FALTA. Las señales se unen con Y: una sola que no se cumpla hace que el formato no
// reconozca nada, y el sistema sabía perfectamente cuál era y no lo decía. El caso que lo motivó
// (23-oct-2026): el estado de cuenta de Scotiabank trae los títulos corridos respecto de los
// datos —la columna titulada «Descripcion» contiene el número de documento—, así que quien
// configuró las señales mirando los DATOS las escribió cruzadas. Las señales se comparan contra
// los TÍTULOS, y eso la pantalla no lo decía en ninguna parte.
//
// Devuelve la lista vacía si no hay una fila de títulos reconocible: entonces el problema no son
// las señales sino que se está mirando la hoja o el tramo equivocado, y señalar señales sería
// mandar a corregir lo que está bien.
func SenalesQueNoCalzan(f FormatoBanco, g Grid) []SenalQueNoCalza {
	mejor := mejorCandidata(filasConContenido(g))
	if mejor == nil {
		return nil
	}
	titulos := g[mejor.Numero-1]
	out := []SenalQueNoCalza{}
	for _, s := range f.Senales {
		if cellHas(titulos, s.Columna, norm(s.Texto)) {
			continue
		}
		out = append(out, SenalQueNoCalza{
			Columna:     s.Columna,
			Pedido:      s.Texto,
			Encontrado:  strings.TrimSpace(cell(titulos, s.Columna)),
			FilaTitulos: mejor.Numero,
		})
	}
	return out
}

// DetectarCon prueba primero los formatos configurados y después los seis del código.
//
// EL ORDEN NO ES ARBITRARIO. El caso que viene es un banco que cambia su exportación: entonces el
// formato de código deja de calzar —o peor, calza a medias— y hace falta poner uno nuevo delante
// sin esperar un despliegue. Al revés, el de código seguiría ganando y configurar el arreglo no
// serviría de nada.
//
// Con `extras` vacío es exactamente `Detectar`: ese es el estado de toda instalación hasta que
// alguien configure el primero.
func DetectarCon(g Grid, extras []Adapter) (Adapter, error) {
	for _, a := range extras {
		if a.Detecta(g) {
			return a, nil
		}
	}
	return Detectar(g)
}
