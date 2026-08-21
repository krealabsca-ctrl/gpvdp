package bancos

import "testing"

func TestIbanMismatch(t *testing.T) {
	casos := []struct {
		nombre  string
		cuenta  string
		archivo string
		want    bool
	}{
		{"iguales", "CR123456789", "CR123456789", false},
		{"normaliza espacios/guiones/mayúsculas", "CR12 3456-789", "cr123456789", false},
		{"distintos bloquea", "CR111111111", "CR999999999", true},
		{"cuenta sin iban memorizado no bloquea", "", "CR123456789", false},
		{"archivo sin iban (p. ej. BN) no bloquea", "CR123456789", "", false},
		{"ambos vacíos", "", "", false},
	}
	for _, c := range casos {
		if got := ibanMismatch(c.cuenta, c.archivo); got != c.want {
			t.Errorf("%s: ibanMismatch(%q, %q) = %v, want %v", c.nombre, c.cuenta, c.archivo, got, c.want)
		}
	}
}
