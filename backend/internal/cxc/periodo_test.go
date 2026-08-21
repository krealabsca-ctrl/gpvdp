package cxc

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal {
	v, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return v
}

func f(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

var (
	mensual     = ModalidadCiclo{Nombre: "Mensual", MesesCiclo: 1}
	trimestral  = ModalidadCiclo{Nombre: "Trimestral", MesesCiclo: 3}
	anual       = ModalidadCiclo{Nombre: "Anual", MesesCiclo: 12}
	quincenalMd = ModalidadCiclo{Nombre: "Quincenal", MesesCiclo: 1, Quincenal: true}
)

func TestPlanDeCargosMensual(t *testing.T) {
	t.Parallel()
	// Contrato real de la muestra: CD-0000000561, primer cobro 3/8/2026, día 3, ₡5 600.
	c := ContratoParaGenerar{Numero: "CD-0000000561", FechaPrimerCobro: f("2026-08-03"), DiaPago: 3, Cuota: d("5600")}

	got, err := PlanDeCargos(c, mensual, f("2026-08-01"), f("2026-10-31"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	quiero := []CargoPlan{
		{Periodo: "2026-08", VenceEn: "2026-08-03", Monto: d("5600")},
		{Periodo: "2026-09", VenceEn: "2026-09-03", Monto: d("5600")},
		{Periodo: "2026-10", VenceEn: "2026-10-03", Monto: d("5600")},
	}
	comparar(t, got, quiero)
}

func TestPlanDeCargosAnualAnclaEnElPrimerCobro(t *testing.T) {
	t.Parallel()
	// Contrato real: CO198456, mantenimiento ANUAL, primer cobro 1/1/2026, cuota ₡2 916,66.
	// Su saldo del origen (₡9 254) equivale a 3,17 cuotas: deuda de varios años.
	c := ContratoParaGenerar{Numero: "CO198456", FechaPrimerCobro: f("2023-01-01"), DiaPago: 30, Cuota: d("2916.66")}

	got, err := PlanDeCargos(c, anual, f("2023-01-01"), f("2026-12-31"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("cargos = %d, se esperaban 4 (2023..2026): %+v", len(got), got)
	}
	// El ciclo se ancla en el primer cobro: enero de cada año, no el mes de arranque
	// de la generación.
	for i, p := range []string{"2023-01", "2024-01", "2025-01", "2026-01"} {
		if got[i].Periodo != p {
			t.Errorf("cargo %d = %s, se esperaba %s", i, got[i].Periodo, p)
		}
	}
}

func TestPlanDeCargosTrimestralNoSeDesalineaConDesde(t *testing.T) {
	t.Parallel()
	// Trimestral que arrancó en FEBRERO: cobra feb, may, ago, nov. Si `desde` recortara
	// el ciclo en vez de saltarse los períodos viejos, quedaría cobrando abr/jul/oct y
	// el contrato entero se correría un mes para siempre.
	c := ContratoParaGenerar{Numero: "CO1", FechaPrimerCobro: f("2026-02-10"), DiaPago: 10, Cuota: d("9000")}

	got, err := PlanDeCargos(c, trimestral, f("2026-04-01"), f("2026-12-31"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	quiero := []string{"2026-05", "2026-08", "2026-11"}
	if len(got) != len(quiero) {
		t.Fatalf("cargos = %d, se esperaban %d: %+v", len(got), len(quiero), got)
	}
	for i, p := range quiero {
		if got[i].Periodo != p {
			t.Errorf("cargo %d = %s, se esperaba %s", i, got[i].Periodo, p)
		}
	}
}

func TestPlanDeCargosQuincenal(t *testing.T) {
	t.Parallel()
	// Los pagos reales traen «1Q/JULIO» y «2Q/JULIO»: dos cargos por mes.
	c := ContratoParaGenerar{Numero: "CO109490", FechaPrimerCobro: f("2026-07-01"), DiaPago: 1, Cuota: d("2500")}

	got, err := PlanDeCargos(c, quincenalMd, f("2026-07-01"), f("2026-07-31"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	quiero := []CargoPlan{
		{Periodo: "2026-07-1Q", VenceEn: "2026-07-01", Monto: d("2500")},
		{Periodo: "2026-07-2Q", VenceEn: "2026-07-16", Monto: d("2500")},
	}
	comparar(t, got, quiero)
}

func TestPlanDeCargosDiaTopadoAlFinDeMes(t *testing.T) {
	t.Parallel()
	// Un contrato que cobra el 31 NO puede vencer el 3 de marzo: se correría de período.
	c := ContratoParaGenerar{Numero: "CO2", FechaPrimerCobro: f("2026-01-31"), DiaPago: 31, Cuota: d("4000")}

	got, err := PlanDeCargos(c, mensual, f("2026-01-01"), f("2026-03-31"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	quiero := []CargoPlan{
		{Periodo: "2026-01", VenceEn: "2026-01-31", Monto: d("4000")},
		{Periodo: "2026-02", VenceEn: "2026-02-28", Monto: d("4000")},
		{Periodo: "2026-03", VenceEn: "2026-03-31", Monto: d("4000")},
	}
	comparar(t, got, quiero)
}

func TestPlanDeCargosEsIdempotenteYDeterminista(t *testing.T) {
	t.Parallel()
	c := ContratoParaGenerar{Numero: "CO3", FechaPrimerCobro: f("2026-03-15"), DiaPago: 15, Cuota: d("6500")}
	a, err := PlanDeCargos(c, mensual, f("2026-03-01"), f("2026-08-31"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	b, _ := PlanDeCargos(c, mensual, f("2026-03-01"), f("2026-08-31"))
	comparar(t, b, a)
	// Y los períodos son únicos: la unicidad (contrato, periodo) de la base nunca debería
	// tener que rechazar nada generado por este plan.
	vistos := map[string]bool{}
	for _, p := range a {
		if vistos[p.Periodo] {
			t.Fatalf("período repetido en el mismo plan: %s", p.Periodo)
		}
		vistos[p.Periodo] = true
	}
}

func TestPlanDeCargosRechazaDatosQueNoAlcanzan(t *testing.T) {
	t.Parallel()
	base := ContratoParaGenerar{Numero: "CO4", FechaPrimerCobro: f("2026-01-01"), DiaPago: 1, Cuota: d("1000")}
	casos := []struct {
		nombre  string
		mutar   func(*ContratoParaGenerar)
		mod     ModalidadCiclo
		esperar error
	}{
		{"sin fecha de primer cobro", func(c *ContratoParaGenerar) { c.FechaPrimerCobro = time.Time{} }, mensual, ErrSinFechaPrimerCobro},
		{"cuota en cero", func(c *ContratoParaGenerar) { c.Cuota = d("0") }, mensual, ErrCuotaInvalida},
		{"cuota negativa", func(c *ContratoParaGenerar) { c.Cuota = d("-500") }, mensual, ErrCuotaInvalida},
		{"modalidad sin ciclo", func(*ContratoParaGenerar) {}, ModalidadCiclo{Nombre: "?", MesesCiclo: 0}, ErrModalidadInvalida},
	}
	for _, cs := range casos {
		t.Run(cs.nombre, func(t *testing.T) {
			c := base
			cs.mutar(&c)
			if _, err := PlanDeCargos(c, cs.mod, f("2026-01-01"), f("2026-06-30")); !errors.Is(err, cs.esperar) {
				t.Errorf("err = %v, se esperaba %v", err, cs.esperar)
			}
		})
	}
	// Rango al revés: se aborta en vez de devolver una lista vacía silenciosa.
	if _, err := PlanDeCargos(base, mensual, f("2026-06-30"), f("2026-01-01")); !errors.Is(err, ErrRangoInvalido) {
		t.Errorf("rango invertido: err = %v", err)
	}
}

func TestPeriodoDesdeConceptoDatosReales(t *testing.T) {
	t.Parallel()
	// Conceptos textuales EXACTOS del archivo de pagos del usuario.
	casos := []struct {
		concepto string
		anio     int
		quiero   string
		ok       bool
	}{
		{"M/JULIO - Ice Corporacion Brunca Zafiro5000.00", 2026, "2026-07", true},
		{"2Q/SEPTIEMBRE - Adepsa SPFC Plan Gardenia2300.00", 2026, "2026-09-2Q", true},
		{"2Q/MARZO - Adepsa SIPROFA Plan Plus2750.00", 2026, "2026-03-2Q", true},
		{"1Q/JUNIO - Adepsa Zafiro2500.00", 2026, "2026-06-1Q", true},
		{"M/AGOSTO - Asesama SIPROFA Plan Plus5000.00", 2026, "2026-08", true},
		// El caso del pago partido en dos períodos: se lee el PRIMERO; el segundo lo
		// extrae el importador de cobros cortando por la coma (fase 2).
		{"1Q/JULIO - Adepsa Zafiro2500.00, PAGO PARCIAL - 2Q/JULIO - …250.00", 2026, "2026-07-1Q", true},
		// Abreviatura de cuatro letras que aparece en «Mes cobro/Año»: sept.
		{"M/SEPT - algo", 2026, "2026-09", true},
		// Basura: no adivina.
		{"", 2026, "", false},
		{"pago varios", 2026, "", false},
		{"X/JULIO - algo", 2026, "", false},
		{"M/MARTES - algo", 2026, "", false},
	}
	for _, cs := range casos {
		got, ok := PeriodoDesdeConcepto(cs.concepto, cs.anio)
		if ok != cs.ok || got != cs.quiero {
			t.Errorf("PeriodoDesdeConcepto(%q) = (%q,%v), se esperaba (%q,%v)", cs.concepto, got, ok, cs.quiero, cs.ok)
		}
	}
}

func TestDiasMoraAdmiteAdelantados(t *testing.T) {
	t.Parallel()
	hoy := f("2026-08-04")
	if got := DiasMora(f("2026-01-01"), hoy); got != 215 {
		// El contrato real CO198456: próximo pago 1/1/2026, 215 días vencidos al 4/8.
		t.Errorf("mora = %d, se esperaban 215", got)
	}
	if got := DiasMora(f("2026-08-30"), hoy); got != -26 {
		// CD-0000000546: vence 30/8, el origen reporta −26 días.
		t.Errorf("adelantado = %d, se esperaban -26", got)
	}
	if got := DiasMora(hoy, hoy); got != 0 {
		t.Errorf("al día = %d, se esperaba 0", got)
	}
}

func comparar(t *testing.T, got, quiero []CargoPlan) {
	t.Helper()
	if len(got) != len(quiero) {
		t.Fatalf("cargos = %d, se esperaban %d: %+v", len(got), len(quiero), got)
	}
	for i := range quiero {
		if got[i].Periodo != quiero[i].Periodo || got[i].VenceEn != quiero[i].VenceEn || !got[i].Monto.Equal(quiero[i].Monto) {
			t.Errorf("cargo %d = %+v, se esperaba %+v", i, got[i], quiero[i])
		}
	}
}
