package precios

// Pruebas del simulador.
//
// La prueba central reproduce el cálculo de Gardenia de la propuesta: cuota ₡4.950, permanencia
// 36 meses, servicio ₡1.779.566, indirecto ₡1.195/mes. La frecuencia de equilibrio tiene que dar
// 2,53 %. Si ese número se mueve, cambió algo en el motor y hay que saberlo.

import (
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func planDirecto(precio string) Plan {
	return Plan{Nombre: "Oro", Modalidad: ModalidadDirecto, PrecioVenta: &precio}
}

func planSuscripcion(cuota string, meses int) Plan {
	m := meses
	return Plan{Nombre: "Gardenia", Modalidad: ModalidadSuscripcion, CuotaMensual: &cuota, PermanenciaMeses: &m}
}

// ── La cifra que decide si un plan de suscripción se sostiene ───────────────────────────────

func TestLaFrecuenciaDeEquilibrioDeGardenia(t *testing.T) {
	t.Parallel()
	// Indirecto de 36 meses a ₡1.195 el mes = ₡43.020.
	indirecto := dec("43020")
	in := InsumosDeSimulacion{
		Plan:                planSuscripcion("4950", 36),
		CostoVariable:       dec("1779566"),
		CosteoCompleto:      true,
		IndirectoPorPeriodo: &indirecto,
		IndirectoCompleto:   true,
	}
	frec := "5.05"
	r, err := Simular(in, Escenario{FrecuenciaAnualPct: &frec, SinComision: true})
	if err != nil {
		t.Fatalf("Simular: %v", err)
	}

	// Ingreso: 4.950 × 36 = 178.200
	if r.Ingreso != "178200.00" {
		t.Errorf("ingreso = %q, se esperaba 178200.00 (cuota × permanencia)", r.Ingreso)
	}
	// Equilibrio: (178.200 − 43.020) ÷ 1.779.566 ÷ 3 años × 100 = 2,53 %
	if r.FrecuenciaEquilibrio == nil || *r.FrecuenciaEquilibrio != "2.53" {
		t.Errorf("frecuencia de equilibrio = %v, se esperaba 2.53 — el número de la propuesta",
			r.FrecuenciaEquilibrio)
	}
	// Con la frecuencia observada (5,05 %) el plan pierde plata.
	if r.Margen[0] != '-' {
		t.Errorf("margen = %q: con 5,05 %% de frecuencia Gardenia debería perder", r.Margen)
	}
}

// La frecuencia es la variable que decide, así que moverla tiene que cambiar el signo.
func TestLaFrecuenciaCambiaElSignoDelResultado(t *testing.T) {
	t.Parallel()
	indirecto := dec("43020")
	in := InsumosDeSimulacion{
		Plan: planSuscripcion("4950", 36), CostoVariable: dec("1779566"),
		CosteoCompleto: true, IndirectoPorPeriodo: &indirecto, IndirectoCompleto: true,
	}

	bajo := "0.47" // la lectura «por contrato»
	rb, _ := Simular(in, Escenario{FrecuenciaAnualPct: &bajo, SinComision: true})
	if rb.Margen[0] == '-' {
		t.Errorf("con 0,47 %% el plan debería ganar; margen = %q", rb.Margen)
	}

	alto := "5.05" // la lectura «por beneficiario»
	ra, _ := Simular(in, Escenario{FrecuenciaAnualPct: &alto, SinComision: true})
	if ra.Margen[0] != '-' {
		t.Errorf("con 5,05 %% el plan debería perder; margen = %q", ra.Margen)
	}
}

// En suscripción el costo esperado NO es el del servicio: es el del servicio por la probabilidad
// de usarlo. Sin eso, comparar una cuota mensual contra un funeral completo no significa nada.
func TestEnSuscripcionElCostoEsperadoEsCostoPorFrecuencia(t *testing.T) {
	t.Parallel()
	in := InsumosDeSimulacion{
		Plan: planSuscripcion("10000", 36), CostoVariable: dec("1800000"), CosteoCompleto: true,
	}
	frec := "5"
	r, err := Simular(in, Escenario{FrecuenciaAnualPct: &frec, SinComision: true, SinIndirecto: true})
	if err != nil {
		t.Fatalf("Simular: %v", err)
	}
	// 1.800.000 × 5 % × 3 años = 270.000
	if r.CostoEsperado != "270000.00" {
		t.Errorf("costo esperado = %q, se esperaba 270000.00", r.CostoEsperado)
	}
	if r.CostoServicio != "1800000.00" {
		t.Errorf("el costo del servicio se muestra entero: %q", r.CostoServicio)
	}
}

// En un plan directo no hay probabilidad: el servicio se presta siempre.
func TestEnDirectoElCostoEsperadoEsElCostoEntero(t *testing.T) {
	t.Parallel()
	in := InsumosDeSimulacion{Plan: planDirecto("1350000"), CostoVariable: dec("936225"), CosteoCompleto: true}
	r, err := Simular(in, Escenario{SinComision: true, SinIndirecto: true})
	if err != nil {
		t.Fatalf("Simular: %v", err)
	}
	if r.CostoEsperado != r.CostoServicio {
		t.Errorf("en directo esperado (%q) y servicio (%q) deben coincidir", r.CostoEsperado, r.CostoServicio)
	}
	if r.FrecuenciaEquilibrio != nil {
		t.Errorf("un plan directo no tiene frecuencia de equilibrio: %v", r.FrecuenciaEquilibrio)
	}
	// 1.350.000 − 936.225 = 413.775, que es la rentabilidad del archivo de costeo.
	if r.Margen != "413775.00" {
		t.Errorf("margen = %q, se esperaba 413775.00", r.Margen)
	}
}

// ── Los escenarios que el usuario quiere probar ─────────────────────────────────────────────

func TestSiLosInsumosSubenElMargenBaja(t *testing.T) {
	t.Parallel()
	in := InsumosDeSimulacion{Plan: planDirecto("1350000"), CostoVariable: dec("936225"), CosteoCompleto: true}

	base, _ := Simular(in, Escenario{SinComision: true, SinIndirecto: true})
	doce := "12"
	sube, _ := Simular(in, Escenario{AjusteCostosPct: &doce, SinComision: true, SinIndirecto: true})

	// 936.225 × 1,12 = 1.048.572
	if sube.CostoServicio != "1048572.00" {
		t.Errorf("costo con +12 %% = %q, se esperaba 1048572.00", sube.CostoServicio)
	}
	if !(dec(sube.Margen).LessThan(dec(base.Margen))) {
		t.Errorf("el margen tenía que bajar: base %q, con ajuste %q", base.Margen, sube.Margen)
	}
}

func TestCambiarLaCuotaMueveElEquilibrio(t *testing.T) {
	t.Parallel()
	in := InsumosDeSimulacion{
		Plan: planSuscripcion("4950", 36), CostoVariable: dec("1779566"), CosteoCompleto: true,
	}
	frec := "5.05"

	base, _ := Simular(in, Escenario{FrecuenciaAnualPct: &frec, SinComision: true, SinIndirecto: true})
	nueva := "8700"
	sube, _ := Simular(in, Escenario{CuotaMensual: &nueva, FrecuenciaAnualPct: &frec, SinComision: true, SinIndirecto: true})

	if !(dec(*sube.FrecuenciaEquilibrio).GreaterThan(dec(*base.FrecuenciaEquilibrio))) {
		t.Errorf("subir la cuota tiene que subir el equilibrio: base %s, nueva %s",
			*base.FrecuenciaEquilibrio, *sube.FrecuenciaEquilibrio)
	}
	if !(dec(sube.Margen).GreaterThan(dec(base.Margen))) {
		t.Errorf("subir la cuota tiene que mejorar el margen")
	}
}

// ── Lo que el simulador se niega a hacer ────────────────────────────────────────────────────

// Un resultado al que le faltan piezas se muestra, pero MARCADO. Publicarlo como completo es el
// defecto que el módulo entero existe para evitar.
func TestUnResultadoIncompletoSeMarcaYNombraLoQueFalta(t *testing.T) {
	t.Parallel()
	in := InsumosDeSimulacion{
		Plan:           planDirecto("1350000"),
		CostoVariable:  dec("400000"),
		CosteoCompleto: false,
		Faltantes:      []string{"Cremación de adulto"},
	}
	r, err := Simular(in, Escenario{})
	if err != nil {
		t.Fatalf("Simular: %v", err)
	}
	if r.Confiable {
		t.Error("con un insumo sin costo el resultado no puede darse por confiable")
	}
	hay := map[string]bool{}
	for _, f := range r.Faltantes {
		hay[f] = true
	}
	if !hay["sin costo: Cremación de adulto"] {
		t.Errorf("no nombró el insumo que falta: %v", r.Faltantes)
	}
	// Y también avisa de la comisión y el indirecto, que no se pidieron excluir.
	if len(r.Faltantes) < 3 {
		t.Errorf("faltan avisos: %v", r.Faltantes)
	}
	// Pero el número igual se calcula: lo que no se puede es leerlo como completo.
	if r.Margen == "" {
		t.Error("el margen parcial se muestra igual")
	}
}

// Sin frecuencia, un plan de suscripción no tiene costo esperado. Suponer cero diría que nadie
// usa el servicio y el plan parecería rentabilísimo.
func TestSinFrecuenciaLaSuscripcionUsaElPeorCasoYLoAvisa(t *testing.T) {
	t.Parallel()
	in := InsumosDeSimulacion{
		Plan: planSuscripcion("4950", 36), CostoVariable: dec("1779566"), CosteoCompleto: true,
	}
	r, err := Simular(in, Escenario{SinComision: true, SinIndirecto: true})
	if err != nil {
		t.Fatalf("Simular: %v", err)
	}
	if r.Confiable {
		t.Error("sin frecuencia el resultado no es confiable")
	}
	if r.CostoEsperado != r.CostoServicio {
		t.Errorf("sin frecuencia se usa el peor caso (costo entero): esperado %q, servicio %q",
			r.CostoEsperado, r.CostoServicio)
	}
	if r.FrecuenciaUsada != nil {
		t.Errorf("no puede declarar una frecuencia que nadie dio: %v", r.FrecuenciaUsada)
	}
}

func TestUnPlanSinPrecioNoSePuedeSimular(t *testing.T) {
	t.Parallel()
	in := InsumosDeSimulacion{Plan: Plan{Nombre: "X", Modalidad: ModalidadDirecto}, CosteoCompleto: true}
	if _, err := Simular(in, Escenario{}); err != ErrSinPrecioParaSimular {
		t.Errorf("error = %v, se esperaba ErrSinPrecioParaSimular", err)
	}
}
