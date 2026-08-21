package bancos

import (
	"strings"
	"unicode"
)

// stopwordsSugerencia es el vocabulario bancario genérico que NUNCA sirve como palabra
// clave de una regla (aparece en casi cualquier descripción). Normalizado con norm().
var stopwordsSugerencia = map[string]bool{
	// operaciones
	"pago": true, "pagos": true, "factura": true, "facturas": true, "recibo": true,
	"tef": true, "sinpe": true, "movil": true, "linea": true, "trf": true, "transf": true, "transferencia": true,
	"trans": true, "transaccion": true, "deposito": true, "depositos": true,
	"retiro": true, "compra": true, "compras": true, "cheque": true, "cheques": true,
	"nota": true, "debito": true, "credito": true, "abono": true, "cargo": true,
	"comision": true, "comisiones": true, "interes": true, "intereses": true,
	// canales y cuentas
	"cta": true, "cuenta": true, "cuentas": true, "ref": true, "referencia": true,
	"internet": true, "banca": true, "ach": true, "iban": true, "pos": true,
	"atm": true, "caja": true, "sucursal": true, "banco": true, "ahorro": true,
	"ahorros": true, "corriente": true, "electronica": true, "automatico": true,
	// conectores
	"por": true, "para": true, "con": true, "del": true, "los": true, "las": true,
	"desde": true, "hacia": true, "otras": true, "otros": true, "entre": true,
	// sociedades (las de 2 letras caen por longitud)
	"ltda": true, "sociedad": true, "anonima": true,
	// meses (aparecen en descripciones de planillas/servicios)
	"ene": true, "enero": true, "feb": true, "febrero": true, "mar": true, "marzo": true,
	"abr": true, "abril": true, "may": true, "mayo": true, "jun": true, "junio": true,
	"jul": true, "julio": true, "ago": true, "agosto": true, "sep": true, "set": true,
	"septiembre": true, "setiembre": true, "oct": true, "octubre": true,
	"nov": true, "noviembre": true, "dic": true, "diciembre": true,
}

// ExtraerPalabraClave propone la palabra clave más específica de una descripción bancaria:
// el primer token puramente alfabético de ≥3 letras que no sea vocabulario bancario genérico.
// Devuelve el token con la grafía original (el matcher compara con norm(), no le afecta).
// "" significa que la descripción no tiene un candidato útil (solo códigos y genéricos).
func ExtraerPalabraClave(descripcion string) string {
	for _, tok := range strings.Fields(descripcion) {
		tok = strings.Trim(tok, ".,;:()[]{}-*#/\\\"'")
		if tok == "" {
			continue
		}
		n := norm(tok)
		if len([]rune(n)) < 3 || stopwordsSugerencia[n] || !esAlfabetico(n) {
			continue
		}
		return tok
	}
	return ""
}

func esAlfabetico(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// soloActivas filtra las reglas pausadas antes de alimentar el matcher.
// El listado para la UI trae todas (activas y pausadas); el motor solo usa activas.
func soloActivas(reglas []Regla) []Regla {
	out := make([]Regla, 0, len(reglas))
	for _, r := range reglas {
		if r.Activo {
			out = append(out, r)
		}
	}
	return out
}
