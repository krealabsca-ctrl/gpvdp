package bancos

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Grid es la hoja del Excel como matriz de celdas ya formateadas a texto.
type Grid [][]string

var errNoFecha = errors.New("sin fecha")

func cell(cells []string, i int) string {
	if i < 0 || i >= len(cells) {
		return ""
	}
	return cells[i]
}

var acentos = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "ü", "u",
	"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ñ", "n",
)

// norm normaliza para comparaciones: minúsculas, sin acentos, sin espacios extremos.
func norm(s string) string {
	return acentos.Replace(strings.ToLower(strings.TrimSpace(s)))
}

func cellHas(cells []string, idx int, subNorm string) bool {
	return strings.Contains(norm(cell(cells, idx)), subNorm)
}

func gridContains(g Grid, tokenNorm string) bool {
	for r := 0; r < len(g) && r < 25; r++ {
		for _, c := range g[r] {
			if strings.Contains(norm(c), tokenNorm) {
				return true
			}
		}
	}
	return false
}

// parseMonto limpia el formato de banco (prefijo de moneda, miles, "-"/vacío) y devuelve decimal.
func parseMonto(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return decimal.Zero, nil
	}
	for _, p := range []string{"CRC", "USD", "₡", "$"} {
		s = strings.ReplaceAll(s, p, "")
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" || s == "-" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("monto inválido %q: %w", s, err)
	}
	return d, nil
}

// fechaLayout devuelve un parser de fecha con un layout de Go (para formatos estándar).
func fechaLayout(layout string) func(string) (time.Time, error) {
	return func(s string) (time.Time, error) {
		s = strings.TrimSpace(s)
		if s == "" {
			return time.Time{}, errNoFecha
		}
		return time.Parse(layout, s)
	}
}

// mesesAbrev acepta las abreviaturas de mes en ESPAÑOL y en INGLÉS.
//
// No es exceso de celo: el portal del Banco Popular exporta los meses en inglés («01 AUG 2026»)
// y el archivo con el que se construyó el adaptador era de JUNIO. Ocho de los doce meses se
// escriben igual en los dos idiomas —FEB MAR MAY JUN JUL SEP OCT NOV—, así que el importador
// funcionó sin problema durante meses y se rompió en AGOSTO, que es el primero que difiere.
// Los otros tres que difieren son ENE/JAN, ABR/APR y DIC/DEC: sin esto volvería a fallar en
// diciembre y en enero.
//
// Aceptar los dos idiomas es lo correcto además porque el idioma del export depende de la
// sesión del portal, no del banco: el mismo usuario puede bajar el archivo en español mañana.
var mesesAbrev = map[string]time.Month{
	// Español (SET es la abreviatura que se usa en Costa Rica).
	"ENE": 1, "FEB": 2, "MAR": 3, "ABR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AGO": 8, "SEP": 9, "SET": 9, "OCT": 10, "NOV": 11, "DIC": 12,
	// Inglés (solo los cuatro que no coinciden con el español).
	"JAN": 1, "APR": 4, "AUG": 8, "DEC": 12,
}

// fechaBP parsea el formato de Banco Popular: "01 JUN 2026 03:23" / "01 AUG 2026 03:37".
func fechaBP(s string) (time.Time, error) {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) < 3 {
		return time.Time{}, errNoFecha
	}
	dd, err := strconv.Atoi(f[0])
	if err != nil {
		return time.Time{}, errNoFecha
	}
	mon, ok := mesesAbrev[strings.ToUpper(f[1])]
	if !ok {
		return time.Time{}, errNoFecha
	}
	yy, err := strconv.Atoi(f[2])
	if err != nil {
		return time.Time{}, errNoFecha
	}
	return time.Date(yy, mon, dd, 0, 0, 0, 0, time.UTC), nil
}

// reIBAN reconoce un IBAN de Costa Rica (CR + 18–22 dígitos), incluso tras el prefijo "CC-" del BCR.
var reIBAN = regexp.MustCompile(`CR\d{18,22}`)

func extraerIBAN(g Grid) string {
	for r := 0; r < len(g) && r < 12; r++ {
		for _, c := range g[r] {
			if m := reIBAN.FindString(strings.ReplaceAll(c, " ", "")); m != "" {
				return m
			}
		}
	}
	return ""
}

func extraerMoneda(g Grid) string {
	for r := 0; r < len(g) && r < 12; r++ {
		for _, c := range g[r] {
			n := norm(c)
			if strings.Contains(n, "dolar") || strings.Contains(n, "usd") {
				return "USD"
			}
			if strings.Contains(n, "colon") || strings.Contains(n, "crc") {
				return "CRC"
			}
		}
	}
	return ""
}
