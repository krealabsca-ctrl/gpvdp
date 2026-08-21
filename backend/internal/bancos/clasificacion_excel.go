package bancos

// Clasificar movimientos EN BLOQUE desde un Excel — lectura del archivo (función pura).
//
// Por qué existe (pedido del usuario, 2026-08-18): tiene los bancos de 2025 y 2026 en un Excel y la
// mitad ya clasificada a mano. Hasta hoy ese trabajo NO se podía traer: el importador de estados de
// cuenta inserta todo con `estado_clasificacion = 'NO_IDENTIFICADO'` y no lee ninguna columna de
// concepto o clasificación — está escrito en el único INSERT del módulo. La única forma de aprovechar
// meses de clasificación manual era volver a hacerla dentro del sistema.
//
// Qué hace y qué NO hace. Esto **clasifica movimientos que ya están cargados**; no los crea. La
// razón no es pereza: crear movimientos desde un archivo cualquiera saltaría los adaptadores de
// banco, la detección de formato, el chequeo de IBAN y la huella anti-duplicado, y esas cuatro cosas
// son las que hacen que el saldo cuadre. El orden correcto es importar el estado de cuenta y después
// traer la clasificación; cuando una fila no tiene movimiento, se dice así en vez de callarlo.
//
// Cómo encuentra el movimiento: por (cuenta, fecha, débito, crédito, documento) — los mismos campos
// que forman la huella `natural_key` del importador, y a propósito la descripción NO entra (algunos
// bancos la cambian entre descargas). La comparación se hace en SQL con tipos `date` y `numeric`, no
// comparando textos, así que da igual si el Excel escribió «1 234,56» o «1234.56».
//
// Las columnas se resuelven por ENCABEZADO, no por posición: el archivo lo edita gente en Excel y
// mueve columnas. Es el mismo criterio del diccionario del catálogo y del importador de CxC.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var (
	// ErrClasifExcelSinEncabezado indica que no se encontró la fila de encabezado.
	ErrClasifExcelSinEncabezado = errors.New("bancos: no se encontró el encabezado (se esperan al menos las columnas «Fecha» y «Clasificación»)")
	// ErrClasifExcelVacio indica que el archivo no trae filas utilizables.
	ErrClasifExcelVacio = errors.New("bancos: el archivo no trae filas con fecha y clasificación")
	// ErrClasifExcelDemasiadasFilas indica que el archivo pasa el tope y hay que partirlo.
	ErrClasifExcelDemasiadasFilas = errors.New("bancos: el archivo trae más filas de las que se pueden procesar de una vez; partilo por cuenta o por año")
)

// maxFilasClasifExcel es el tope de filas por archivo. Existe para que un archivo enorme falle
// diciéndolo, en vez de tardar diez minutos o quedarse a medias sin avisar.
const maxFilasClasifExcel = 50000

// maxDetalleClasifExcel es cuántas filas de detalle viajan al navegador. Los CONTADORES siempre son
// del total; lo que se recorta es la tabla. Si se recorta, la respuesta lo dice.
const maxDetalleClasifExcel = 400

// hojaMovimientos es la hoja que escribe el reporte de movimientos del sistema, y por eso la que se
// busca primero al importar: el archivo que el ERP exporta es el que el ERP vuelve a leer.
const hojaMovimientos = "Movimientos"

// Estados de una fila del archivo. Están en lenguaje de consecuencia, no de implementación: es lo
// que va a pasar con esa fila.
const (
	ClasifExcelClasifica    = "CLASIFICA"           // estaba sin clasificar y se le asigna la partida
	ClasifExcelReclasifica  = "RECLASIFICA"         // ya tenía otra partida (solo con reclasificar=true)
	ClasifExcelSinCambio    = "SIN_CAMBIO"          // ya tiene exactamente esa partida
	ClasifExcelSinMovim     = "SIN_MOVIMIENTO"      // ese movimiento no está cargado
	ClasifExcelSinPartida   = "PARTIDA_DESCONOCIDA" // el Concepto › Clasificación no existe
	ClasifExcelSinCuenta    = "CUENTA_DESCONOCIDA"  // no se pudo resolver la cuenta bancaria
	ClasifExcelFilaInvalida = "FILA_INVALIDA"       // falta fecha o monto, o débito y crédito juntos
	ClasifExcelAmbiguo      = "AMBIGUO"             // varios movimientos idénticos y no se sabe cuál
	ClasifExcelProtegido    = "YA_CLASIFICADO"      // tiene otra partida y no se pidió reclasificar
	// ClasifExcelSinLlenar es la fila de la plantilla que quedó en blanco. NO es un error: es lo que
	// pasa en el flujo normal (se bajan 5.000 movimientos y se llenan 60). Antes caía en FILA_INVALIDA
	// y el resumen decía «4.940 filas no se pudieron leer» —falso— y esas 4.940 copaban la tabla de
	// detalle, expulsando las poquísimas filas que sí necesitaban atención.
	ClasifExcelSinLlenar = "SIN_LLENAR" // trae el movimiento pero no la partida: no se toca
)

// FilaClasifExcel es una línea del archivo: lo que se leyó y lo que se resolvió.
type FilaClasifExcel struct {
	Linea int `json:"linea"`
	// ── lo que trae el archivo ──
	Cuenta        string `json:"cuenta"`
	Fecha         string `json:"fecha"` // YYYY-MM-DD; vacío = no se entendió
	Documento     string `json:"documento"`
	Debito        string `json:"debito"`
	Credito       string `json:"credito"`
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
	// ── lo que resuelve el servidor ──
	Estado  string `json:"estado"`
	Detalle string `json:"detalle"`
	// Descripcion es la del MOVIMIENTO hallado, no la del archivo: es la prueba de que calzó con el
	// movimiento correcto. Sin ella el usuario tiene que creer que el sistema encontró lo que dice.
	Descripcion string `json:"descripcion"`
	// PartidaActual: qué tenía antes (vacío = estaba sin clasificar).
	PartidaActual string `json:"partida_actual"`

	// campos internos del emparejamiento (no viajan)
	cuentaID  string
	fecha     time.Time
	debito    decimal.Decimal
	credito   decimal.Decimal
	clasifID  string
	conceptID string
}

// PlanClasifExcel es qué va a pasar (o qué pasó) con el archivo entero.
type PlanClasifExcel struct {
	Filas   int               `json:"filas"`
	Detalle []FilaClasifExcel `json:"detalle"`
	// DetalleTruncado avisa que la tabla se recortó; los contadores siguen siendo del total.
	DetalleTruncado bool `json:"detalle_truncado"`
	// Contadores por estado, sobre el total de filas.
	Clasifica     int `json:"clasifica"`
	Reclasifica   int `json:"reclasifica"`
	SinCambio     int `json:"sin_cambio"`
	SinMovimiento int `json:"sin_movimiento"`
	SinPartida    int `json:"sin_partida"`
	SinCuenta     int `json:"sin_cuenta"`
	Invalidas     int `json:"invalidas"`
	Ambiguas      int `json:"ambiguas"`
	Protegidas    int `json:"protegidas"`
	// SinLlenar: filas de la plantilla que quedaron en blanco. No son un problema.
	SinLlenar int `json:"sin_llenar"`
	// Aplicado: false = fue una previsualización y no se escribió nada.
	Aplicado bool `json:"aplicado"`
	// Clasificados: movimientos efectivamente escritos (solo al aplicar).
	Clasificados int `json:"clasificados"`
	// Hoja leída y todas las hojas del libro: se lee UNA sola, y callarlo esconde trabajo.
	Hoja  string   `json:"hoja"`
	Hojas []string `json:"hojas"`
	// Aviso resume en una frase lo que hay que mirar antes de aplicar (vacío = nada que advertir).
	Aviso string `json:"aviso"`
}

// encabezadosClasifExcel mapea nombre de columna normalizado → papel. Se aceptan las grafías que
// escriben las personas y las que exporta el propio sistema.
var encabezadosClasifExcel = map[string]string{
	"fecha":               "fecha",
	"fecha movimiento":    "fecha",
	"fechamovimiento":     "fecha",
	"fecha contable":      "fecha",
	"documento":           "documento",
	"doc":                 "documento",
	"doc.":                "documento",
	"referencia":          "documento",
	"no documento":        "documento",
	"n documento":         "documento",
	"numero de documento": "documento",
	"debito":              "debito",
	"debitos":             "debito",
	"debito (dr)":         "debito",
	"salida":              "debito",
	"credito":             "credito",
	"creditos":            "credito",
	"credito (cr)":        "credito",
	"entrada":             "credito",
	"concepto":            "concepto",
	"clasificacion":       "clasificacion",
	"subclasificacion":    "clasificacion",
	"partida":             "clasificacion",
	"cuenta":              "cuenta",
	"cuenta bancaria":     "cuenta",
	"alias":               "cuenta",
	"iban":                "cuenta",
	"descripcion":         "descripcion",
	"detalle":             "descripcion",
}

// columnasClasifExcel busca el encabezado en las primeras filas y devuelve papel→índice.
//
// El encabezado se reconoce por tener FECHA y (clasificación o concepto): con solo «fecha» se
// confundiría con cualquier tabla, y es justo el error que hace que un archivo se lea al revés.
func columnasClasifExcel(g Grid) (map[string]int, int) {
	for i := 0; i < len(g) && i < 15; i++ {
		col := map[string]int{}
		for j, c := range g[i] {
			if papel, ok := encabezadosClasifExcel[norm(c)]; ok {
				if _, repetido := col[papel]; !repetido {
					col[papel] = j
				}
			}
		}
		_, hayFecha := col["fecha"]
		_, hayClasif := col["clasificacion"]
		_, hayConcepto := col["concepto"]
		if hayFecha && (hayClasif || hayConcepto) {
			return col, i
		}
	}
	return nil, 0
}

// fechaDeCelda entiende las formas en que una fecha llega desde Excel.
//
// `dd/mm/yyyy` va primero porque es la que escribe el reporte del propio sistema (y la convención de
// Costa Rica). También se aceptan los seriales de Excel: cuando la celda es una fecha de verdad y el
// formato no se puede resolver, excelize devuelve el número de días desde 1899-12-30.
func fechaDeCelda(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	// Un serial de Excel es un entero (o con decimales de hora) sin separadores de fecha.
	if !strings.ContainsAny(v, "/-") {
		if n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil && n > 20000 && n < 80000 {
			base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
			return base.AddDate(0, 0, int(n)), true
		}
	}
	for _, layout := range []string{"02/01/2006", "2006-01-02", "02-01-2006", "02/01/06", "2006/01/02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseMontoTolerante lee un monto escrito por una PERSONA en Excel, no por un banco.
//
// No se puede usar el `parseMonto` del importador: ese borra todas las comas porque los archivos de
// los bancos vienen en convención inglesa («1,234.56»), y con eso «1 234,56» —como lo escribe alguien
// en Costa Rica— se convierte en ciento veintitrés mil. El movimiento entonces no calza con nada y la
// fila sale como «no está cargado», que es un mensaje falso sobre un problema que era de formato.
//
// La regla es la misma que ya usa el frontend para los montos que se teclean: manda el ÚLTIMO
// separador, y solo cuenta como decimal si le siguen una o dos cifras. Si le siguen tres, es de miles
// («1,234» = mil doscientos treinta y cuatro), que es la lectura correcta para los dos formatos.
func parseMontoTolerante(s string) (decimal.Decimal, error) {
	limpio := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '-' {
			return r
		}
		return -1
	}, s)
	if limpio == "" || limpio == "-" {
		// Celda vacía (o el guion con que algunos bancos escriben el cero) es cero de verdad. Pero si
		// la celda TENÍA algo y no quedó ni un dígito, es un texto: devolverlo como cero convertiría un
		// error de captura en un monto, y el movimiento no calzaría nunca sin decir por qué.
		if t := strings.TrimSpace(s); t != "" && t != "-" {
			return decimal.Zero, fmt.Errorf("monto inválido %q: no tiene ningún dígito", s)
		}
		return decimal.Zero, nil
	}
	ultimo := strings.LastIndexAny(limpio, ".,")
	if ultimo >= 0 {
		decimales := len(limpio) - ultimo - 1
		// 1 o 2 cifras = decimal; exactamente 3 = separador de miles («1,234» son mil doscientos
		// treinta y cuatro); 4 o más NO puede ser de miles —no existe un grupo de cuatro— y tratarlo
		// como tal multiplicaba el monto por diez mil.
		if decimales != 3 {
			entero := strings.NewReplacer(".", "", ",", "").Replace(limpio[:ultimo])
			limpio = entero + "." + limpio[ultimo+1:]
		} else {
			limpio = strings.NewReplacer(".", "", ",", "").Replace(limpio)
		}
	}
	d, err := decimal.NewFromString(limpio)
	if err != nil {
		return decimal.Zero, fmt.Errorf("monto inválido %q: %w", s, err)
	}
	return d, nil
}

// LeerClasifExcel interpreta la grilla. Función pura: no toca base de datos ni resuelve nada contra
// el catálogo — eso lo hace el servicio, que es quien sabe qué existe.
func LeerClasifExcel(g Grid) ([]FilaClasifExcel, error) {
	col, filaEnc := columnasClasifExcel(g)
	if col == nil {
		return nil, ErrClasifExcelSinEncabezado
	}
	filas := make([]FilaClasifExcel, 0, 256)
	for i := filaEnc + 1; i < len(g); i++ {
		celda := func(papel string) string {
			idx, ok := col[papel]
			if !ok {
				return ""
			}
			return strings.TrimSpace(cell(g[i], idx))
		}
		fechaTxt := celda("fecha")
		clasif := celda("clasificacion")
		concepto := celda("concepto")
		if fechaTxt == "" && clasif == "" && concepto == "" {
			continue // fila en blanco o separador
		}
		if len(filas) >= maxFilasClasifExcel {
			return nil, ErrClasifExcelDemasiadasFilas
		}

		f := FilaClasifExcel{
			Linea:         i + 1,
			Cuenta:        celda("cuenta"),
			Documento:     celda("documento"),
			Concepto:      concepto,
			Clasificacion: clasif,
		}

		t, ok := fechaDeCelda(fechaTxt)
		if !ok {
			f.Estado = ClasifExcelFilaInvalida
			f.Detalle = fmt.Sprintf("no se entiende la fecha %q (se espera dd/mm/aaaa)", fechaTxt)
			filas = append(filas, f)
			continue
		}
		f.fecha = t
		f.Fecha = t.Format("2006-01-02")

		deb, errD := parseMontoTolerante(celda("debito"))
		cre, errC := parseMontoTolerante(celda("credito"))
		switch {
		case errD != nil || errC != nil:
			f.Estado = ClasifExcelFilaInvalida
			f.Detalle = "el débito o el crédito no se entienden como monto"
		case deb.IsNegative() || cre.IsNegative():
			f.Estado = ClasifExcelFilaInvalida
			f.Detalle = "monto negativo: el sentido lo da la columna (débito o crédito), no el signo"
		case deb.IsPositive() && cre.IsPositive():
			f.Estado = ClasifExcelFilaInvalida
			f.Detalle = "débito y crédito a la vez en la misma fila"
		case deb.IsZero() && cre.IsZero():
			f.Estado = ClasifExcelFilaInvalida
			f.Detalle = "la fila no trae monto en débito ni en crédito, y sin monto no se puede encontrar el movimiento"
		case clasif == "" && concepto == "":
			// La plantilla trae la fecha y los montos de TODOS los movimientos y las dos columnas de
			// partida vacías: la fila en blanco es el estado de reposo del archivo, no un error.
			f.Estado = ClasifExcelSinLlenar
			f.Detalle = "sin llenar: no se toca"
		case clasif == "":
			f.Estado = ClasifExcelFilaInvalida
			f.Detalle = "trae el concepto pero no la clasificación: la partida necesita las dos"
		}
		f.debito, f.credito = deb, cre
		f.Debito, f.Credito = deb.StringFixed(2), cre.StringFixed(2)
		filas = append(filas, f)
	}
	if len(filas) == 0 {
		return nil, ErrClasifExcelVacio
	}
	return filas, nil
}

// claveMovimiento identifica el movimiento que busca una fila. Es la misma tupla de la huella
// `natural_key` del importador, sin el índice de ocurrencia: los duplicados legítimos comparten
// clave y por eso se resuelven aparte (ver el emparejamiento en el servicio).
func claveMovimiento(cuentaID string, fecha time.Time, debito, credito decimal.Decimal, documento string) string {
	return strings.Join([]string{
		cuentaID,
		fecha.Format("2006-01-02"),
		debito.StringFixed(2),
		credito.StringFixed(2),
		strings.TrimSpace(documento),
	}, "|")
}
