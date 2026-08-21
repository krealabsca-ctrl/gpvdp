package cxp

import "testing"

func TestExtraerHuella(t *testing.T) {
	casos := []struct {
		desc   string
		quiero string
		ok     bool
	}{
		{"TRANSFERENCIA CXP-A1B2C3D4E5F6 PAGO PROVEEDOR", "CXP-A1B2C3D4E5F6", true},
		{"pago cxp-a1b2c3d4e5f6 minusculas", "CXP-A1B2C3D4E5F6", true}, // se normaliza a mayúsculas
		{"SINPE MOVIL sin huella", "", false},
		{"CXP-SHORT no calza (menos de 12)", "", false},
	}
	for _, c := range casos {
		got, ok := extraerHuella(c.desc)
		if ok != c.ok || got != c.quiero {
			t.Errorf("extraerHuella(%q) = (%q,%v), quería (%q,%v)", c.desc, got, ok, c.quiero, c.ok)
		}
	}
}
