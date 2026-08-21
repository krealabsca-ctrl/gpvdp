package bancos

import "testing"

func TestConsecutivoLargo(t *testing.T) {
	// 23 chars de prefijo + 25 dígitos = extracción exacta desde la posición 24.
	desc := "PAGO TRANSFERENCIA XYZ 2026040712365478965874123 COLA"
	casos := []struct {
		banco, desc, quiere string
	}{
		{"Davivienda", desc, "2026040712365478965874123"},
		{"DAVIVIENDA", desc, "2026040712365478965874123"}, // insensible a mayúsculas
		{"BN", desc, ""}, // solo aplica a Davivienda
		{"BAC San José", desc, ""},
		{"Davivienda", "corta", ""}, // descripción más corta que la posición 24
	}
	for _, c := range casos {
		if got := ConsecutivoLargo(c.banco, c.desc); got != c.quiere {
			t.Errorf("ConsecutivoLargo(%q, …) = %q, quiere %q", c.banco, got, c.quiere)
		}
	}
	// Longitud: siempre 25 cuando hay material suficiente.
	if got := ConsecutivoLargo("Davivienda", desc); len([]rune(got)) != 25 {
		t.Errorf("longitud = %d, quiere 25", len([]rune(got)))
	}
}
