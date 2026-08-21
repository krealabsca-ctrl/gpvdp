package cxp

import "testing"

func TestRequisitoAprobacion(t *testing.T) {
	casos := []struct {
		monto            string
		requeridos       int
		requiereGerencia bool
	}{
		{"500000", 1, false},  // ≤ 1M
		{"1000000", 1, false}, // 1M exacto
		{"1000001", 2, false}, // > 1M
		{"5000000", 2, false}, // 5M exacto
		{"5000001", 2, true},  // > 5M -> Gerencia
		{"20000000", 2, true},
	}
	for _, c := range casos {
		req, ger := requisitoAprobacion(dec(c.monto))
		if req != c.requeridos || ger != c.requiereGerencia {
			t.Errorf("monto %s => (%d,%v), quería (%d,%v)", c.monto, req, ger, c.requeridos, c.requiereGerencia)
		}
	}
}

func TestAprobacionCompleta(t *testing.T) {
	casos := []struct {
		nombre string
		monto  string
		roles  []string
		want   bool
	}{
		{"1M con 1 supervisor", "800000", []string{"SUPERVISOR_FINANCIERO"}, true},
		{"1M sin aprobaciones", "800000", nil, false},
		{"3M con 1 aprobador", "3000000", []string{"DIRECTOR_FINANCIERO"}, false},
		{"3M con 2 aprobadores", "3000000", []string{"DIRECTOR_FINANCIERO", "SUPERVISOR_FINANCIERO"}, true},
		{"10M con 2 sin gerencia", "10000000", []string{"DIRECTOR_FINANCIERO", "SUPERVISOR_FINANCIERO"}, false},
		{"10M con gerencia + 1", "10000000", []string{"DIRECTOR_FINANCIERO", "GERENCIA_GENERAL"}, true},
	}
	for _, c := range casos {
		if got := aprobacionCompleta(dec(c.monto), c.roles); got != c.want {
			t.Errorf("%s: got %v, quería %v", c.nombre, got, c.want)
		}
	}
}
