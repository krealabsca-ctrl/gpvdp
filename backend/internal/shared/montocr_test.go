package shared

// El comprobante que se le manda al proveedor decía «₡56500.00» (Contabilidad, 19-oct-2026).

import "testing"

func TestMontoCR(t *testing.T) {
	t.Parallel()
	casos := map[string]string{
		// El caso del correo real.
		"56500.00": "56 500,00",
		// Los cortes de cada grupo de miles.
		"999.00":        "999,00",
		"1000.00":       "1 000,00",
		"999999.99":     "999 999,99",
		"1000000.00":    "1 000 000,00",
		"56500":         "56 500,00", // sin decimales en el origen
		"0":             "0,00",
		"1234.5":        "1 234,50",
		"-56500.00":     "-56 500,00", // el signo va antes del símbolo, no entre medio
		"1234567890.12": "1 234 567 890,12",
	}
	for entra, quiere := range casos {
		if got := MontoCR(entra); got != quiere {
			t.Errorf("MontoCR(%q) = %q, quiere %q", entra, got, quiere)
		}
	}
}

// Un monto ilegible NO se convierte en «0,00». Un cero inventado en un comprobante de pago se lee
// como un pago de cero, que es peor que un texto raro: el texto raro se nota y se corrige.
func TestUnMontoIlegibleNoSeVuelveCero(t *testing.T) {
	t.Parallel()
	for _, basura := range []string{"", "ocho mil", "N/D"} {
		if got := MontoCR(basura); got == "0,00" {
			t.Errorf("MontoCR(%q) devolvió un cero inventado", basura)
		}
	}
}

// El símbolo sale de la moneda de la FACTURA. Decirle «₡» a un proveedor que facturó en dólares le
// cambia el monto por 500.
func TestMontoConMonedaUsaLaMonedaDeLaFactura(t *testing.T) {
	t.Parallel()
	if got := MontoConMoneda("56500.00", "CRC"); got != "₡56 500,00" {
		t.Errorf("CRC = %q", got)
	}
	if got := MontoConMoneda("1234.56", "USD"); got != "USD 1 234,56" {
		t.Errorf("USD = %q", got)
	}
	// Minúsculas y espacios no son otra moneda.
	if got := MontoConMoneda("1234.56", " usd "); got != "USD 1 234,56" {
		t.Errorf("usd con espacios = %q", got)
	}
	// Sin moneda declarada se asume colones: así nacieron las 4.542 facturas que existen.
	if got := MontoConMoneda("100.00", ""); got != "₡100,00" {
		t.Errorf("sin moneda = %q", got)
	}
}

// El separador es el espacio DURO. Con uno normal, un monto partido al final de un renglón queda
// «₡56» en una línea y «500,00» en la siguiente, y así se transcribe mal.
func TestElSeparadorDeMilesEsEspacioDuro(t *testing.T) {
	t.Parallel()
	got := MontoCR("1000.00")
	if got == "1 000,00" {
		t.Error("usó espacio normal: un monto partido en dos líneas se transcribe mal")
	}
	if got != "1 000,00" {
		t.Errorf("= %q, quiere 1<nbsp>000,00", got)
	}
}
