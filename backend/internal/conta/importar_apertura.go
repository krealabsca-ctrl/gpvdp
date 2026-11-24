package conta

// Lector del Excel de saldos iniciales (Fase 4).
//
// ── DE DÓNDE SALE ESTE ARCHIVO ──────────────────────────────────────────────────────────────
//
// Del balance de comprobación del sistema anterior, exportado a Excel. Por eso el lector busca
// el encabezado en vez de asumir la primera fila —esos archivos traen título, empresa y fecha
// arriba— y acepta las dos formas en que ese balance presenta los saldos:
//
//   · DOS COLUMNAS: «Debe» y «Haber», que es como lo exporta un balance de comprobación.
//   · UNA COLUMNA: «Saldo», con el signo decidiendo el lado. Es como queda cuando alguien
//     resume el balance a mano.
//
// Sin la segunda forma, media la gente tendría que reformatear el archivo antes de cargarlo, y
// un paso manual previo es donde se cuelan los errores de tipeo sobre los saldos de apertura.

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// normEncabezado deja el nombre de una columna en letras y dígitos, sin acentos.
//
// NO ALCANZA CON `norm`: esa quita los acentos pero conserva espacios y puntuación, y los nombres
// reales vienen de varias palabras —«Código Cuenta», «No. Cuenta», «Saldo Deudor»—. Con `norm`,
// toda clave compuesta de este mapa era inalcanzable, incluidas «Saldo Deudor» y «Saldo Acreedor»,
// que es justo como un balance de comprobación nombra sus columnas. El archivo más común del
// sistema anterior no se reconocía.
func normEncabezado(s string) string {
	var b strings.Builder
	for _, r := range norm(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// encabezadosApertura reconoce las columnas por su nombre, normalizado con normEncabezado.
var encabezadosApertura = map[string]string{
	"codigo": "codigo", "cuenta": "codigo", "codigocuenta": "codigo",
	"nocuenta": "codigo", "numerodecuenta": "codigo", "cuentacontable": "codigo",

	"debe": "debe", "saldodeudor": "debe", "deudor": "debe",
	"haber": "haber", "saldoacreedor": "haber", "acreedor": "haber",

	"saldo": "saldo", "saldofinal": "saldo", "saldoinicial": "saldo", "monto": "saldo",
}

// LeerAperturaDesdeExcel saca los saldos iniciales del archivo.
func LeerAperturaDesdeExcel(archivo []byte) ([]FilaApertura, error) {
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

	// El encabezado es la primera fila que nombra el código Y al menos una columna de monto.
	colCodigo, colDebe, colHaber, colSaldo, filaEnc := -1, -1, -1, -1, -1
	for i := 0; i < len(filas) && i < 25; i++ {
		c, d, h, s := -1, -1, -1, -1
		for j, celda := range filas[i] {
			switch encabezadosApertura[normEncabezado(celda)] {
			case "codigo":
				if c < 0 {
					c = j
				}
			case "debe":
				if d < 0 {
					d = j
				}
			case "haber":
				if h < 0 {
					h = j
				}
			case "saldo":
				if s < 0 {
					s = j
				}
			}
		}
		if c >= 0 && (d >= 0 || h >= 0 || s >= 0) {
			colCodigo, colDebe, colHaber, colSaldo, filaEnc = c, d, h, s, i
			break
		}
	}
	if filaEnc < 0 {
		return nil, fmt.Errorf(
			"conta: no se encontró el encabezado; hace falta una columna de cuenta y otra de monto («Debe» y «Haber», o una sola de «Saldo»)")
	}

	out := make([]FilaApertura, 0, len(filas))
	for i := filaEnc + 1; i < len(filas); i++ {
		fila := filas[i]
		codigo := strings.TrimSpace(celdaDe(fila, colCodigo))
		if codigo == "" {
			continue
		}
		f := FilaApertura{Fila: i + 1, Codigo: codigo}

		var debe, haber decimal.Decimal
		if colDebe >= 0 || colHaber >= 0 {
			debe = montoDeCelda(celdaDe(fila, colDebe))
			haber = montoDeCelda(celdaDe(fila, colHaber))
		}
		// La columna «Saldo» solo se usa si no hubo debe ni haber en esa fila: cuando el archivo
		// trae las tres, las dos primeras son el dato y la tercera el neto, y sumar las tres
		// duplicaría el saldo.
		if colSaldo >= 0 && debe.IsZero() && haber.IsZero() {
			s := montoDeCelda(celdaDe(fila, colSaldo))
			if s.IsNegative() {
				haber = s.Neg()
			} else {
				debe = s
			}
		}

		// UN SALDO EN LOS DOS LADOS SE NETEA, no se rechaza: un balance exportado a veces trae
		// las dos columnas llenas por un arrastre, y lo que importa es el neto de la cuenta.
		if debe.IsPositive() && haber.IsPositive() {
			neto := debe.Sub(haber)
			if neto.IsNegative() {
				debe, haber = decimal.Zero, neto.Neg()
			} else {
				debe, haber = neto, decimal.Zero
			}
		}

		if debe.IsZero() && haber.IsZero() {
			// Una cuenta en cero no aporta al asiento de apertura y ensuciaría el libro con
			// líneas que no dicen nada. Se descarta en silencio: no es un error del archivo.
			continue
		}
		if debe.IsNegative() || haber.IsNegative() {
			f.Problema = "el saldo quedó negativo en su propia columna"
		}
		f.Debe, f.Haber = debe.StringFixed(2), haber.StringFixed(2)
		out = append(out, f)
	}
	return out, nil
}

// celdaDe devuelve la celda o "" si la columna no existe en esa fila.
//
// Las filas de un .xlsx llegan RECORTADAS en la última celda con contenido: una fila cuyo haber
// está vacío puede venir con menos columnas que el encabezado, y leerla por índice sin esto
// entra en pánico.
func celdaDe(fila []string, col int) string {
	if col < 0 || col >= len(fila) {
		return ""
	}
	return fila[col]
}

// montoDeCelda lee un número que puede venir con separadores, símbolo de moneda o entre
// paréntesis —que en contabilidad es el negativo—.
func montoDeCelda(s string) decimal.Decimal {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero
	}
	negativo := false
	// «(1.234,56)» es −1.234,56: es como un balance escribe los negativos.
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		negativo = true
		s = strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			negativo = true
		case r == ',' || r == '.':
			b.WriteRune(r)
		}
	}
	limpio := normalizarDecimal(b.String())
	if limpio == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(limpio)
	if err != nil {
		return decimal.Zero
	}
	if negativo {
		return d.Neg()
	}
	return d
}

// normalizarDecimal resuelve el punto y la coma.
//
// ── POR QUÉ NO ALCANZA CON «EL ÚLTIMO SEPARADOR MANDA» ──────────────────────────────────────
//
// Esa regla sirve cuando hay de los dos: en «1.234,56» la coma es decimal y en «1,234.56» lo es
// el punto. Pero con un solo tipo repetido —«1.234.567», que es como un Excel de Costa Rica
// escribe un millón doscientos mil sin céntimos— tomar el último dejaba «1.234.567» con dos
// puntos, que no es un número: `decimal` fallaba y el saldo entraba en CERO, en silencio, sobre
// el archivo de apertura. Un saldo inicial en blanco no se nota hasta que el balance no cuadra.
//
// La regla que sí distingue es cuántos dígitos siguen al separador: TRES es de miles, otra
// cantidad es decimal. «1.234» queda en 1234 y «1,5» en 1,5, que es lo que significan.
func normalizarDecimal(s string) string {
	ultimaComa := strings.LastIndex(s, ",")
	ultimoPunto := strings.LastIndex(s, ".")
	if ultimaComa < 0 && ultimoPunto < 0 {
		return s
	}

	// Con los dos tipos presentes, el último es el decimal: no hay ambigüedad.
	if ultimaComa >= 0 && ultimoPunto >= 0 {
		if ultimaComa > ultimoPunto {
			return strings.ReplaceAll(s[:ultimaComa], ".", "") + "." + s[ultimaComa+1:]
		}
		return strings.ReplaceAll(s[:ultimoPunto], ",", "") + "." + s[ultimoPunto+1:]
	}

	// Un solo tipo. Si tras el último separador hay exactamente tres dígitos, es de miles.
	sep := ultimaComa
	cual := ","
	if ultimoPunto >= 0 {
		sep, cual = ultimoPunto, "."
	}
	if len(s)-sep-1 == 3 {
		return strings.ReplaceAll(s, cual, "")
	}
	return strings.ReplaceAll(s[:sep], cual, "") + "." + s[sep+1:]
}
