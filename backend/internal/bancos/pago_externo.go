package bancos

// Los pagos que el sistema ANTERIOR ya tenía aplicados.
//
// Valle de Paz tiene 3.437 débitos sin enlazar por ₡1.831.621.463, y el ERP no puede distinguir
// entre «este pago nunca se registró» y «este ya está aplicado en el otro sistema». Qué se aplicó
// allá no se deduce de ningún dato de acá: hay que traerlo del reporte del otro ERP.
//
// Lo que este archivo NO hace: enlazar el movimiento a una factura. El sello dice «esto ya está
// resuelto en el sistema anterior», que es una afirmación más débil y más honesta que «esto pagó la
// factura X». Inventar el enlace para que el tablero cuadre sería escribir una relación que nadie
// verificó.
//
// Decisión del Director Financiero (6-oct-2026): el movimiento marcado SIGUE VISIBLE con el sello.
// No se esconde — la plata salió del banco igual, y lo que desaparece de la lista no se revisa más.

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var (
	// ErrReporteSinEncabezado: no se encontró una fila que se parezca al encabezado del reporte.
	ErrReporteSinEncabezado = errors.New("bancos: no se reconoció el encabezado del reporte de pagos aplicados")
	// ErrReporteSinFilas: el encabezado está pero debajo no hay nada.
	ErrReporteSinFilas = errors.New("bancos: el reporte no trae ninguna fila de pago")
	// ErrReporteSinLlave: hay encabezado pero ninguna columna sirve para cruzar contra el banco.
	ErrReporteSinLlave = errors.New("bancos: el reporte no trae ni referencia, ni fecha, ni monto: sin alguno de esos no hay forma de cruzarlo contra el banco")
)

// maxFilasReporte acota el archivo. Un reporte de pagos de varios años cabe de sobra; más que esto
// es un archivo equivocado, y vale más decirlo que tragarse medio millón de filas.
const maxFilasReporte = 50000

// ErrReporteFormato: el archivo no es ni Excel ni un texto separado que se pueda leer.
var ErrReporteFormato = errors.New("bancos: el archivo no se pudo abrir: se esperaba un Excel (.xlsx) o un texto separado por comas o punto y coma (.csv)")

// GridDelReporte abre el archivo del reporte, venga como venga.
//
// Excel PRIMERO y CSV de respaldo: un .xlsx es un zip y se reconoce sin ambigüedad, mientras que
// casi cualquier cosa se puede leer como texto. Al revés, un Excel entraría como una sola columna
// de basura binaria y el lector diría «no reconocí el encabezado» en vez de la verdad.
//
// Acepta los dos porque nadie controla cómo exporta el otro ERP, y un 500 por subir el CSV que el
// sistema viejo genera por defecto sería culpar a la persona de una decisión nuestra.
func GridDelReporte(archivo []byte) (Grid, error) {
	if g, err := CargarGrid(bytes.NewReader(archivo)); err == nil {
		return g, nil
	}
	g, err := gridDeTextoSeparado(archivo)
	if err != nil {
		return nil, ErrReporteFormato
	}
	return g, nil
}

// gridDeTextoSeparado lee un CSV adivinando el separador de la primera línea con contenido.
//
// Se prueban coma, punto y coma y tabulación: Excel en español exporta con punto y coma, y un
// lector que solo entienda la coma leería cada fila como UNA columna —sin fallar— y todo el
// archivo entraría vacío.
func gridDeTextoSeparado(archivo []byte) (Grid, error) {
	texto := strings.TrimPrefix(string(archivo), "\xEF\xBB\xBF") // BOM de Excel
	lineas := strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n")

	sep := ','
	for _, l := range lineas {
		if strings.TrimSpace(l) == "" {
			continue
		}
		mejor, cuantos := ',', strings.Count(l, ",")
		for _, cand := range []rune{';', '\t'} {
			if n := strings.Count(l, string(cand)); n > cuantos {
				mejor, cuantos = cand, n
			}
		}
		sep = mejor
		break
	}

	r := csv.NewReader(strings.NewReader(texto))
	r.Comma = sep
	// Las filas de un reporte no siempre traen el mismo número de campos (una columna final vacía,
	// una fila de total más corta). Rechazar el archivo por eso sería perderlo entero.
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	filas, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("bancos: leer el texto separado: %w", err)
	}
	if len(filas) == 0 {
		return nil, ErrReporteSinFilas
	}
	return Grid(filas), nil
}

// aliasReportePago son los encabezados que se reconocen, por campo.
//
// La lista es GENEROSA a propósito y está pensada para crecer: el reporte sale de otro ERP y nadie
// controla cómo nombra sus columnas. Un encabezado que no se reconoce no rompe nada —el campo entra
// vacío— pero sí deja la fila sin poder cruzarse, así que cuando aparezca un nombre nuevo se agrega
// acá en vez de pedirle a alguien que reacomode el archivo a mano.
var aliasReportePago = map[string][]string{
	"referencia": {
		"referencia", "referencia bancaria", "referencia de pago", "pago referencia",
		"ref", "no. referencia", "num referencia", "numero de referencia", "numero de referencia",
		"comprobante", "no. comprobante", "transaccion", "no. transaccion",
	},
	"fecha": {
		"fecha", "fecha de pago", "fecha pago", "fecha aplicacion", "fecha aplicacion",
		"fecha de aplicacion", "fecha de aplicacion", "fecha bancaria", "fecha de pago bancario",
		"fecha movimiento", "fecha de movimiento",
	},
	"monto": {
		"monto", "monto pagado", "valor", "importe", "total", "monto aplicado", "debito", "debito",
		"monto del pago", "monto crc", "monto colones",
	},
	"documento": {
		"documento", "no. documento", "numero de documento", "numero de documento", "factura",
		"no. factura", "numero de factura", "numero de factura", "recibo", "no. recibo",
		"consecutivo", "asiento",
	},
	"tercero": {
		"proveedor", "beneficiario", "cliente", "nombre", "razon social", "razon social",
		"nombre del proveedor", "a nombre de",
	},
	"nota": {"descripcion", "descripcion", "detalle", "concepto", "observaciones", "observacion", "observacion"},
}

// FilaReportePago es un renglón del reporte ya interpretado. Nunca se descarta en silencio: lo que
// no se entiende viaja en `Motivo` y la pantalla lo muestra antes de que alguien confirme nada.
type FilaReportePago struct {
	Linea int `json:"linea"` // línea del archivo, para encontrarla

	Referencia string          `json:"referencia"`
	Documento  string          `json:"documento"`
	Tercero    string          `json:"tercero"`
	Fecha      string          `json:"fecha"` // YYYY-MM-DD; "" si no vino o no se entendió
	Monto      decimal.Decimal `json:"-"`
	MontoTexto string          `json:"monto"`
	Nota       string          `json:"nota"`

	// Motivo: por qué esta fila no se puede cruzar. Vacío = se puede intentar.
	Motivo string `json:"motivo,omitempty"`
}

// SePuedeCruzar dice si la fila trae con qué buscar el movimiento.
//
// Hacen falta DOS datos, no uno: una referencia sola alcanza si es única, pero un monto solo
// calzaría con decenas de débitos y una fecha sola con cientos. Emparejar con uno solo es inventar.
func (f FilaReportePago) SePuedeCruzar() bool {
	if f.Motivo != "" {
		return false
	}
	datos := 0
	if f.Referencia != "" {
		datos++
	}
	if f.Fecha != "" {
		datos++
	}
	if f.Monto.IsPositive() {
		datos++
	}
	return datos >= 2
}

// LeerReportePagos interpreta el reporte del otro ERP. Función pura: no toca la base y no decide
// nada irreversible, igual que el resto de los lectores del proyecto.
func LeerReportePagos(g Grid) ([]FilaReportePago, error) {
	if len(g) == 0 {
		return nil, ErrReporteSinFilas
	}
	cols, encabezado := columnasReportePago(g)
	if cols == nil {
		return nil, ErrReporteSinEncabezado
	}
	// Sin ninguna de las tres llaves, el archivo se lee entero para nada. Se dice ACÁ y no fila por
	// fila: es un problema del archivo, no de los datos.
	if cols["referencia"] < 0 && cols["fecha"] < 0 && cols["monto"] < 0 {
		return nil, ErrReporteSinLlave
	}
	if encabezado+1 >= len(g) {
		return nil, ErrReporteSinFilas
	}

	out := make([]FilaReportePago, 0, len(g)-encabezado-1)
	for i := encabezado + 1; i < len(g); i++ {
		if len(out) >= maxFilasReporte {
			return nil, fmt.Errorf("bancos: el reporte pasa de %d filas: revisá si es el archivo correcto", maxFilasReporte)
		}
		fila := g[i]
		celda := func(campo string) string {
			idx, ok := cols[campo]
			if !ok || idx < 0 {
				return ""
			}
			return strings.TrimSpace(cell(fila, idx))
		}

		f := FilaReportePago{
			Linea:      i + 1,
			Referencia: celda("referencia"),
			Documento:  celda("documento"),
			Tercero:    celda("tercero"),
			Nota:       celda("nota"),
		}
		// Una fila sin NADA es un separador o el total del reporte, no un pago.
		if f.Referencia == "" && f.Documento == "" && celda("monto") == "" && celda("fecha") == "" {
			continue
		}

		if txt := celda("fecha"); txt != "" {
			if fecha, ok := fechaDelReporte(txt); ok {
				f.Fecha = fecha
			} else {
				f.Motivo = fmt.Sprintf("fecha ilegible: %q", txt)
			}
		}
		if txt := celda("monto"); txt != "" {
			// `parseMontoTolerante` y no un parseo propio: ya resuelve la trampa de los separadores
			// («1,234» son mil doscientos treinta y cuatro, «1,23» es uno con veintitrés), que es
			// donde se multiplica un monto por mil sin que nadie lo note.
			monto, err := parseMontoTolerante(txt)
			switch {
			case err != nil:
				f.Motivo = fmt.Sprintf("monto ilegible: %q", txt)
			default:
				// El signo no importa: el reporte puede traer los pagos en negativo y el banco los
				// guarda como débito positivo. Lo que se compara es cuánta plata se movió.
				f.Monto = monto.Abs()
				f.MontoTexto = f.Monto.StringFixed(2)
			}
		}
		if f.Motivo == "" && !f.SePuedeCruzar() {
			f.Motivo = "no trae al menos dos de: referencia, fecha y monto"
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, ErrReporteSinFilas
	}
	return out, nil
}

// columnasReportePago busca el encabezado en las primeras filas (muchos reportes traen título y
// fecha arriba) y resuelve cada campo por alias. Devuelve -1 para los campos que no están.
func columnasReportePago(g Grid) (map[string]int, int) {
	limite := 15
	if len(g) < limite {
		limite = len(g)
	}
	for fila := 0; fila < limite; fila++ {
		cols := map[string]int{}
		for i, c := range g[fila] {
			enc := normalizarEncabezadoReporte(c)
			if enc == "" {
				continue
			}
			for campo, lista := range aliasReportePago {
				if _, ya := cols[campo]; ya {
					continue // la primera columna que calza gana: un reporte puede repetir nombres
				}
				for _, a := range lista {
					if enc == a {
						cols[campo] = i
						break
					}
				}
			}
		}
		// Se acepta como encabezado la fila que traiga AL MENOS UNA de las tres llaves. Exigir las
		// tres dejaría afuera reportes legítimos (hay sistemas que no numeran la referencia).
		if _, hayRef := cols["referencia"]; hayRef {
			return completar(cols), fila
		}
		_, hayFecha := cols["fecha"]
		_, hayMonto := cols["monto"]
		if hayFecha && hayMonto {
			return completar(cols), fila
		}
	}
	return nil, 0
}

// completar pone -1 en los campos que el archivo no trae, para que `celda` no tenga que distinguir
// entre «no está» y «es la columna 0».
func completar(cols map[string]int) map[string]int {
	for campo := range aliasReportePago {
		if _, ok := cols[campo]; !ok {
			cols[campo] = -1
		}
	}
	return cols
}

// normalizarEncabezadoReporte deja el encabezado comparable: sin tildes, sin espacios de más, en
// minúsculas y sin los dos puntos finales que algunos exportadores agregan.
func normalizarEncabezadoReporte(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimRight(s, ": ")
	s = strings.Join(strings.Fields(s), " ")
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'á':
			b.WriteRune('a')
		case 'é':
			b.WriteRune('e')
		case 'í':
			b.WriteRune('i')
		case 'ó':
			b.WriteRune('o')
		case 'ú', 'ü':
			b.WriteRune('u')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// formatosFecha son las formas en que los exportadores escriben una fecha. El orden importa: d/m
// antes que m/d porque los reportes de la empresa vienen en formato de Costa Rica, y para los días
// del 1 al 12 las dos lecturas son válidas —ya nos costó caro una vez (ver la clave de Hacienda)—.
var formatosFecha = []string{
	"2006-01-02", "02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006",
	"20060102", "02/01/06", "2/1/06", "2006/01/02",
}

// fechaDelReporte interpreta la fecha de una celda y la devuelve en YYYY-MM-DD.
func fechaDelReporte(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	// Algunos exportadores agregan la hora: se corta por el primer espacio.
	if i := strings.IndexAny(s, " T"); i > 0 {
		s = s[:i]
	}
	// Los formatos de TEXTO se prueban PRIMERO. Al revés, «20260115» —que es el 15 de enero escrito
	// como yyyymmdd— se tomaba como número de serie de Excel y daba el año 57370.
	for _, f := range formatosFecha {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	// Recién acá, el número de serie: Excel lo deja cuando la celda tiene formato de fecha. Se acota
	// por arriba además de por abajo, para que un número que no es una fecha —un consecutivo, un
	// monto sin decimales— no se convierta en el año 3000.
	if serie, err := decimal.NewFromString(s); err == nil &&
		serie.GreaterThan(decimal.NewFromInt(20000)) && serie.LessThan(decimal.NewFromInt(80000)) {
		// El día 0 de Excel es el 30/12/1899 (su calendario cree que 1900 fue bisiesto).
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).
			AddDate(0, 0, int(serie.IntPart())).Format("2006-01-02"), true
	}
	return "", false
}
