package bancos

import "testing"

func TestPeriodosHasta(t *testing.T) {
	casos := []struct {
		hasta   string
		n       int
		primero string
		ultimo  string
	}{
		{"2026-07", 12, "2025-08", "2026-07"}, // cruza el año
		{"2026-07", 1, "2026-07", "2026-07"},
		{"2026-01", 3, "2025-11", "2026-01"}, // retrocede a noviembre
		{"2026-12", 12, "2026-01", "2026-12"},
	}
	for _, c := range casos {
		t.Run(c.hasta, func(t *testing.T) {
			got := periodosHasta(c.hasta, c.n)
			if len(got) != c.n {
				t.Fatalf("len = %d, quiere %d", len(got), c.n)
			}
			if got[0] != c.primero || got[len(got)-1] != c.ultimo {
				t.Errorf("rango = %s..%s, quiere %s..%s", got[0], got[len(got)-1], c.primero, c.ultimo)
			}
		})
	}
}

func TestRellenarSerie(t *testing.T) {
	periodos := []string{"2026-05", "2026-06", "2026-07"}
	puntos := []SerieMensualPunto{
		{Periodo: "2026-07", Ingresos: "100", Gastos: "40", EBITDA: "60", Movimientos: 9},
	}
	out := rellenarSerie(puntos, periodos)
	if len(out) != 3 {
		t.Fatalf("len = %d, quiere 3", len(out))
	}
	if out[0].Periodo != "2026-05" || out[0].Ingresos != "0" || out[0].Movimientos != 0 {
		t.Errorf("mes sin datos debe salir en cero: %+v", out[0])
	}
	if out[2].Ingresos != "100" || out[2].Movimientos != 9 {
		t.Errorf("mes con datos debe conservarse: %+v", out[2])
	}
}

func TestOrdenSQL(t *testing.T) {
	casos := map[string]string{
		"":           "m.fecha DESC, m.id",
		"fecha_asc":  "m.fecha ASC, m.id",
		"monto_desc": "m.monto_crc DESC, m.id",
		"monto_asc":  "m.monto_crc ASC, m.id",
		// valores desconocidos caen al orden por defecto (whitelist, sin inyección)
		"nombre; DROP TABLE x": "m.fecha DESC, m.id",
	}
	for in, quiere := range casos {
		if got := ordenSQL(in); got != quiere {
			t.Errorf("ordenSQL(%q) = %q, quiere %q", in, got, quiere)
		}
	}
}

func TestEsPeriodoValido(t *testing.T) {
	validos := []string{"2026-07", "2000-01", "2100-12"}
	invalidos := []string{"", "2026-13", "2026-00", "26-07", "2026/07", "abcd-ef", "1999-05"}
	for _, p := range validos {
		if !esPeriodoValido(p) {
			t.Errorf("esPeriodoValido(%q) = false, quiere true", p)
		}
	}
	for _, p := range invalidos {
		if esPeriodoValido(p) {
			t.Errorf("esPeriodoValido(%q) = true, quiere false", p)
		}
	}
}
