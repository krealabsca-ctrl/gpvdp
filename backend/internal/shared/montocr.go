package shared

// Montos escritos para que los lea una PERSONA, en el formato de Costa Rica.
//
// EL PROBLEMA (Contabilidad, 19-oct-2026): el comprobante que se le manda al proveedor decía
// «₡56500.00» — el decimal crudo con el símbolo pegado adelante. Un monto así se lee mal y se
// transcribe peor: sin separador de miles, ₡56500 y ₡565000 se distinguen contando dígitos.
//
// Esto NO existía en el backend. El formateo vivía solo en el frontend (`src/lib/format.ts`), así
// que todo lo que el servidor escribe para una persona —correos, PDF, WhatsApp— salía crudo. El
// formato de acá iguala el de esa función: separador de miles con ESPACIO DURO y coma decimal, que
// es lo que produce `Intl.NumberFormat("es-CR")`.

import (
	"strings"

	"github.com/shopspring/decimal"
)

// espacioDuro es el separador de miles del es-CR (U+00A0), el mismo que emite Intl en el cliente.
//
// Duro y no normal: con un espacio común, un monto partido al final de un renglón queda «₡56» en
// una línea y «500,00» en la siguiente, y así se transcribe mal.
const espacioDuro = " "

// MontoCR escribe un decimal con separador de miles y dos decimales: «56 500,00».
//
// Recibe el monto como string porque así viaja el dinero en este sistema (numeric → decimal →
// string). Un valor ilegible se devuelve tal cual en vez de convertirse en «0,00»: inventar un
// cero en un comprobante de pago es peor que mostrar algo raro que se note.
func MontoCR(monto string) string {
	d, err := decimal.NewFromString(strings.TrimSpace(monto))
	if err != nil {
		return monto
	}
	s := d.Round(2).StringFixed(2) // «-56500.00»
	negativo := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	partes := strings.SplitN(s, ".", 2)
	entera, dec := partes[0], "00"
	if len(partes) == 2 {
		dec = partes[1]
	}
	// Los miles se agrupan de a tres desde la derecha.
	var b strings.Builder
	for i, r := range entera {
		if i > 0 && (len(entera)-i)%3 == 0 {
			b.WriteString(espacioDuro)
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + dec
	if negativo {
		return "-" + out
	}
	return out
}

// MontoConMoneda antepone el símbolo que corresponde: «₡56 500,00» o «USD 1 234,56».
//
// El símbolo sale de la moneda de la FACTURA, no de una constante: decirle «₡» a un proveedor que
// facturó en dólares le cambia el monto por 500.
func MontoConMoneda(monto, moneda string) string {
	n := MontoCR(monto)
	if strings.ToUpper(strings.TrimSpace(moneda)) == "USD" {
		return "USD " + n
	}
	return "₡" + n
}
