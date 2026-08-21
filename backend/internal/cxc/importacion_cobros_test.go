package cxc

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestLeerCobrosDelArchivoReal(t *testing.T) {
	t.Parallel()
	g := gridDeTestdata(t, "pagos_muestra.csv")
	// 1 encabezado + 11 pagos.
	if len(g) != 12 {
		t.Fatalf("filas del grid = %d, se esperaban 12", len(g))
	}
	filas, err := LeerCobros(g, ReglasCobros{CobroMaximo: decimal.NewFromInt(1000000)})
	if err != nil {
		t.Fatalf("LeerCobros: %v", err)
	}
	if len(filas) != 11 {
		t.Fatalf("cobros = %d, se esperaban 11", len(filas))
	}

	porConsecutivo := map[string]FilaCobro{}
	for _, f := range filas {
		porConsecutivo[f.Consecutivo] = f
	}

	t.Run("las tres fechas se leen distintas y con su significado", func(t *testing.T) {
		f := porConsecutivo["3946983"]
		if f.FechaPago != "2026-07-01" {
			t.Errorf("fecha de pago (el período) = %q, se esperaba 2026-07-01", f.FechaPago)
		}
		if f.FechaBanco != "2026-07-30" {
			t.Errorf("fecha bancaria (cuándo entró la plata) = %q, se esperaba 2026-07-30", f.FechaBanco)
		}
		if f.FechaCreado != "2026-07-31" {
			t.Errorf("fecha de registro = %q, se esperaba 2026-07-31", f.FechaCreado)
		}
	})

	t.Run("la fecha bancaria doble toma la primera", func(t *testing.T) {
		// El dato real: «08/07/2026|11/07/2026» — la planilla llegó en dos transferencias.
		f := porConsecutivo["3258181"]
		if f.FechaBanco != "2026-07-08" {
			t.Errorf("fecha bancaria = %q, se esperaba 2026-07-08 (la primera de las dos)", f.FechaBanco)
		}
	})

	t.Run("el canal y la asociación se leen", func(t *testing.T) {
		f := porConsecutivo["3946983"]
		if f.FormaPago != "Descuento por Asociación Solidarista" {
			t.Errorf("forma de pago = %q", f.FormaPago)
		}
		if f.Asociacion != "REGION HUETAR BRUNCA" {
			t.Errorf("asociación = %q", f.Asociacion)
		}
		if f.Contrato != "CO83436" || f.Documento != "109150628" {
			t.Errorf("contrato/documento = %q / %q", f.Contrato, f.Documento)
		}
		if !f.Monto.Equal(decimal.RequireFromString("5000")) {
			t.Errorf("monto = %s", f.Monto)
		}
	})

	t.Run("el Concepto revela el PERÍODO que pagó cada cobro", func(t *testing.T) {
		casos := map[string]string{
			"3946983": "2026-07",    // M/JULIO
			"3929581": "2026-09-2Q", // 2Q/SEPTIEMBRE — pagado en julio, ADELANTADO
			"3145430": "2026-03-2Q", // 2Q/MARZO — pagado en julio, 4 meses tarde
			"3344392": "2026-08",    // M/AGOSTO — adelantado
		}
		for cons, periodo := range casos {
			f := porConsecutivo[cons]
			if len(f.Aplicaciones) != 1 {
				t.Errorf("%s: aplicaciones = %d (%+v)", cons, len(f.Aplicaciones), f.Aplicaciones)
				continue
			}
			if f.Aplicaciones[0].Periodo != periodo {
				t.Errorf("%s: período = %q, se esperaba %q", cons, f.Aplicaciones[0].Periodo, periodo)
			}
			if !f.Aplicaciones[0].Monto.Equal(f.Monto) {
				t.Errorf("%s: el detalle (%s) no suma el valor (%s)", cons, f.Aplicaciones[0].Monto, f.Monto)
			}
		}
	})

	t.Run("EL CASO CLAVE: un cobro partido en dos períodos, uno parcial", func(t *testing.T) {
		// «1Q/JULIO - Adepsa Zafiro2500.00, PAGO PARCIAL - 2Q/JULIO - … cuota250.00»
		f := porConsecutivo["3308788"]
		if len(f.Aplicaciones) != 2 {
			t.Fatalf("aplicaciones = %d, se esperaban 2: %+v", len(f.Aplicaciones), f.Aplicaciones)
		}
		a, b := f.Aplicaciones[0], f.Aplicaciones[1]
		if a.Periodo != "2026-07-1Q" || !a.Monto.Equal(decimal.RequireFromString("2500")) || a.Parcial {
			t.Errorf("primera aplicación = %+v", a)
		}
		if b.Periodo != "2026-07-2Q" || !b.Monto.Equal(decimal.RequireFromString("250")) || !b.Parcial {
			t.Errorf("segunda aplicación = %+v (se esperaba 2026-07-2Q · 250 · parcial)", b)
		}
		// Y las dos suman el valor del cobro: 2500 + 250 = 2750.
		if !a.Monto.Add(b.Monto).Equal(f.Monto) {
			t.Errorf("las aplicaciones suman %s y el valor es %s", a.Monto.Add(b.Monto), f.Monto)
		}
		if f.EnCuarentena() {
			t.Errorf("no debería estar en cuarentena: %v", f.Motivos)
		}
	})

	t.Run("ninguna fila del archivo real queda en cuarentena", func(t *testing.T) {
		for _, f := range filas {
			if f.EnCuarentena() {
				t.Errorf("consecutivo %s en cuarentena por %v", f.Consecutivo, f.Motivos)
			}
		}
	})

	t.Run("todos vienen Activos, no anulados", func(t *testing.T) {
		for _, f := range filas {
			if f.Anulado() {
				t.Errorf("consecutivo %s vino anulado", f.Consecutivo)
			}
		}
	})
}

func TestLeerAplicacionesDelConcepto(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre   string
		concepto string
		anio     int
		quiero   []AplicacionLeida
	}{
		{
			"mensual simple",
			"M/JULIO - Ice Corporacion Brunca Zafiro5000.00", 2026,
			[]AplicacionLeida{{Periodo: "2026-07", Monto: decimal.RequireFromString("5000")}},
		},
		{
			"quincena adelantada",
			"2Q/SEPTIEMBRE - Adepsa SPFC Plan Gardenia2300.00", 2026,
			[]AplicacionLeida{{Periodo: "2026-09-2Q", Monto: decimal.RequireFromString("2300")}},
		},
		{
			"dos períodos con parcial",
			"1Q/JULIO - Adepsa Zafiro2500.00, PAGO PARCIAL - 2Q/JULIO - Adepsa Zafiro. Se realizo un adelanto de cuota250.00", 2026,
			[]AplicacionLeida{
				{Periodo: "2026-07-1Q", Monto: decimal.RequireFromString("2500")},
				{Periodo: "2026-07-2Q", Monto: decimal.RequireFromString("250"), Parcial: true},
			},
		},
		{"sin año no adivina", "M/JULIO - algo5000.00", 0, nil},
		{"sin período no inventa", "PAGO VARIOS 5000.00", 2026, nil},
		{"vacío", "", 2026, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := LeerAplicacionesDelConcepto(c.concepto, c.anio)
			if len(got) != len(c.quiero) {
				t.Fatalf("aplicaciones = %d, se esperaban %d: %+v", len(got), len(c.quiero), got)
			}
			for i := range c.quiero {
				if got[i].Periodo != c.quiero[i].Periodo || !got[i].Monto.Equal(c.quiero[i].Monto) ||
					got[i].Parcial != c.quiero[i].Parcial {
					t.Errorf("[%d] = %+v, se esperaba %+v", i, got[i], c.quiero[i])
				}
			}
		})
	}
}

func TestLeerCobrosCuarentena(t *testing.T) {
	t.Parallel()
	g := Grid{
		{"Contrato", "Consecutivo", "Valor", "Fecha de Pago", "Concepto", "Estado"},
		{"CO1", "1", "999999999.00", "1/7/2026", "M/JULIO - x5000.00", "Activo"}, // monto absurdo
		{"CO2", "2", "0.00", "1/7/2026", "", "Activo"},                           // cero
		{"CO3", "3", "no es monto", "1/7/2026", "", "Activo"},                    // ilegible
		{"", "4", "5000.00", "1/7/2026", "", "Activo"},                           // sin contrato
		{"CO5", "5", "5000.00", "", "", "Activo"},                                // sin fecha
		{"CO6", "6", "5000.00", "1/7/2026", "M/JULIO - x2000.00", "Activo"},      // el detalle no cuadra
		{"CO7", "7", "5000.00", "1/7/2026", "M/JULIO - x5000.00", "Activo"},      // bueno
	}
	filas, err := LeerCobros(g, ReglasCobros{CobroMaximo: decimal.NewFromInt(1000000)})
	if err != nil {
		t.Fatalf("LeerCobros: %v", err)
	}
	if len(filas) != 7 {
		t.Fatalf("filas = %d, se esperaban 7 (ninguna se descarta en silencio)", len(filas))
	}
	quiero := map[string]bool{"1": true, "2": true, "3": true, "4": true, "5": true, "6": true, "7": false}
	for _, f := range filas {
		if f.EnCuarentena() != quiero[f.Consecutivo] {
			t.Errorf("consecutivo %s: cuarentena=%v (motivos %v), se esperaba %v",
				f.Consecutivo, f.EnCuarentena(), f.Motivos, quiero[f.Consecutivo])
		}
	}
}

func TestMontoAlFinal(t *testing.T) {
	t.Parallel()
	casos := map[string]string{
		"Zafiro5000.00":                         "5000",
		"Plan Plus2750.00":                      "2750",
		"Se realizo un adelanto de cuota250.00": "250",
		"SD Majestic12000.00":                   "12000",
		"algo1.084.200,50":                      "1084200.5",
	}
	for in, quiero := range casos {
		got, ok := montoAlFinal(in)
		if !ok || !got.Equal(decimal.RequireFromString(quiero)) {
			t.Errorf("montoAlFinal(%q) = (%s,%v), se esperaba %s", in, got, ok, quiero)
		}
	}
	// Ambiguo: dos separadores que no forman un monto válido. Debe RECHAZARSE, no adivinar.
	for _, in := range []string{"sin numero", "", "Plan Gardenia", "Zafiro2.5000.00", "cuota."} {
		if _, ok := montoAlFinal(in); ok {
			t.Errorf("montoAlFinal(%q) debería fallar", in)
		}
	}
}
