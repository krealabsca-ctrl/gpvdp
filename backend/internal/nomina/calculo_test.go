package nomina

// Tests GOLDEN del motor: los casos y valores esperados vienen de los empleados demo de
// la MAQUETA APROBADA (maqueta-rrhh.html, motor calcEmp/rentaSalario) — el backend debe
// reproducirlos colón por colón.

import (
	"testing"

	"github.com/shopspring/decimal"
)

func calcParams(t *testing.T) ParametrosCalc {
	t.Helper()
	p, err := parametrosACalc(ParametrosDefault2026(2026))
	if err != nil {
		t.Fatalf("parametrosACalc(defaults): %v", err)
	}
	return p
}

func ingresoSalarial(nombre, monto string) IngresoCalc {
	return IngresoCalc{Nombre: nombre, Monto: dec(monto), AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true}
}

func exigir(t *testing.T, nombre string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(dec(want)) {
		t.Errorf("%s = %s, quiere %s", nombre, got.String(), want)
	}
}

// María José Ramírez (maqueta #1): base 480k + comisión 420k, 2 hijos, préstamo 45k, ahorro 25k.
// La comisión es salario → base CCSS 900k (guardarraíl). Bajo el tramo exento de renta.
func TestGoldenMariaJose(t *testing.T) {
	p := calcParams(t)
	r := CalcularLiquidacion(EmpleadoCalc{
		Hijos: 2, SalarioBase: dec("480000"),
		Ingresos: []IngresoCalc{
			ingresoSalarial("Salario ordinario", "480000"),
			ingresoSalarial("Comisiones", "420000"),
		},
		Deducciones: []DeduccionCalc{
			{ID: "d1", Etiqueta: "Préstamo Asociación", Cuota: dec("45000"), SaldoRestante: decPtr("315000"), Prioridad: 100},
			{ID: "d2", Etiqueta: "Ahorro", Cuota: dec("25000"), Prioridad: 100},
		},
		AdelantoPagado: dec("240000"),
	}, p)

	exigir(t, "bruto", r.Bruto, "900000")
	exigir(t, "baseCCSS", r.BaseCCSS, "900000")
	exigir(t, "ccssObrero", r.CCSSObrero, "97470") // SEM 49500 + IVM 38970 + BP 9000
	exigir(t, "renta", r.Renta, "0")               // 900k < 918k exento
	exigir(t, "deducciones", r.Deducciones, "70000")
	exigir(t, "adelanto", r.Adelanto, "240000")
	exigir(t, "neto", r.Neto, "492530")
	exigir(t, "patronal", r.Patronal, "241470") // 25.83% + INS 1% sobre 900k
	exigir(t, "provAguinaldo", r.ProvAguinaldo, "74970")
	exigir(t, "provVacaciones", r.ProvVacaciones, "37440")
	exigir(t, "provCesantia", r.ProvCesantia, "13500")
}

// Marielos Álvarez (maqueta #6): base 1 150k + bono 120k, 2 hijos, ahorro 60k.
// Cae en el tramo del 10%: (1 270 000 − 918 000) × 10% − 2×1 710 = 31 780.
func TestGoldenMarielos(t *testing.T) {
	p := calcParams(t)
	r := CalcularLiquidacion(EmpleadoCalc{
		Hijos: 2, SalarioBase: dec("1150000"),
		Ingresos: []IngresoCalc{
			ingresoSalarial("Salario ordinario", "1150000"),
			ingresoSalarial("Bonificación habitual", "120000"),
		},
		Deducciones:    []DeduccionCalc{{ID: "d1", Etiqueta: "Ahorro", Cuota: dec("60000"), Prioridad: 100}},
		AdelantoPagado: dec("575000"),
	}, p)

	exigir(t, "ccssObrero", r.CCSSObrero, "137541") // 69850 + 54991 + 12700
	exigir(t, "renta", r.Renta, "31780")
	exigir(t, "neto", r.Neto, "465679") // 1 270 000 − 137 541 − 31 780 − 60 000 − 575 000
}

// Carlos Fernández (maqueta #2): base 690k + bono 60k + extras 85k + viático 40k, 1 hijo,
// cónyuge, ahorro 30k. El viático se PAGA pero no cotiza CCSS ni renta (base legal).
func TestGoldenCarlosViatico(t *testing.T) {
	p := calcParams(t)
	r := CalcularLiquidacion(EmpleadoCalc{
		Hijos: 1, Conyuge: true, SalarioBase: dec("690000"),
		Ingresos: []IngresoCalc{
			ingresoSalarial("Salario ordinario", "690000"),
			ingresoSalarial("Bonificación habitual", "60000"),
			ingresoSalarial("Horas extra", "85000"),
			{Nombre: "Viáticos liquidados", Monto: dec("40000")}, // sin banderas: no afecta nada
		},
		Deducciones:    []DeduccionCalc{{ID: "d1", Etiqueta: "Ahorro", Cuota: dec("30000"), Prioridad: 100}},
		AdelantoPagado: dec("345000"),
	}, p)

	exigir(t, "bruto", r.Bruto, "875000")       // incluye viático
	exigir(t, "baseCCSS", r.BaseCCSS, "835000") // sin viático
	exigir(t, "baseRenta", r.BaseRenta, "835000")
	exigir(t, "ccssObrero", r.CCSSObrero, "90431") // 45925 + 36156 (36155.5↑) + 8350
	exigir(t, "renta", r.Renta, "0")
	exigir(t, "neto", r.Neto, "409569")
}

// Adelanto: % configurable sobre el salario base, sin deducciones (maqueta: 50%).
func TestGoldenAdelanto(t *testing.T) {
	p := calcParams(t)
	r := CalcularAdelanto(EmpleadoCalc{SalarioBase: dec("480000")}, p)
	exigir(t, "adelanto bruto", r.Bruto, "240000")
	exigir(t, "adelanto neto", r.Neto, "240000")
	if len(r.Detalle) != 1 || r.Detalle[0].Tipo != "INGRESO" {
		t.Errorf("detalle del adelanto inesperado: %+v", r.Detalle)
	}
}

// Prelación: la pensión alimentaria (prioridad 1) cobra completa; el préstamo se aplica
// PARCIAL hasta agotar el neto; el ahorro (última prioridad) queda pospuesto en 0.
func TestPrelacionNetoInsuficiente(t *testing.T) {
	p := calcParams(t)
	r := CalcularLiquidacion(EmpleadoCalc{
		SalarioBase: dec("400000"),
		Ingresos:    []IngresoCalc{ingresoSalarial("Salario ordinario", "400000")},
		Deducciones: []DeduccionCalc{
			{ID: "ahorro", Etiqueta: "Ahorro", Cuota: dec("50000"), Prioridad: 200},
			{ID: "pension", Etiqueta: "Pensión alimentaria", Cuota: dec("120000"), Prioridad: 1},
			{ID: "prestamo", Etiqueta: "Préstamo", Cuota: dec("60000"), Prioridad: 100},
		},
		AdelantoPagado: dec("200000"),
	}, p)
	// Disponible tras CCSS y adelanto: 400 000 − 43 320 − 200 000 = 156 680.
	// Pensión 120 000 → quedan 36 680; préstamo aplica parcial 36 680; ahorro 0.
	exigir(t, "deducciones", r.Deducciones, "156680")
	exigir(t, "neto", r.Neto, "0")
	var prestamo, ahorro string
	for _, d := range r.Detalle {
		if d.DeduccionID == "prestamo" {
			prestamo = d.Monto
		}
		if d.DeduccionID == "ahorro" {
			ahorro = d.Monto
		}
	}
	if prestamo != "36680.00" {
		t.Errorf("préstamo parcial = %q, quiere 36680.00", prestamo)
	}
	if ahorro != "" {
		t.Errorf("ahorro debió quedar pospuesto (sin renglón), tiene %q", ahorro)
	}
}

// Corte automático por saldo: si el saldo restante es menor a la cuota, aplica el saldo.
func TestCortePorSaldo(t *testing.T) {
	p := calcParams(t)
	r := CalcularLiquidacion(EmpleadoCalc{
		SalarioBase: dec("500000"),
		Ingresos:    []IngresoCalc{ingresoSalarial("Salario ordinario", "500000")},
		Deducciones: []DeduccionCalc{
			{ID: "d1", Etiqueta: "Préstamo Soda", Cuota: dec("18000"), SaldoRestante: decPtr("7500"), Prioridad: 100},
		},
	}, p)
	exigir(t, "deducciones (última cuota parcial)", r.Deducciones, "7500")
}

// Excepción INA (<5 empleados): al desactivarla, el costo patronal baja exactamente 1.50%.
func TestPatronalSinINA(t *testing.T) {
	p := calcParams(t)
	base := EmpleadoCalc{SalarioBase: dec("900000"), Ingresos: []IngresoCalc{ingresoSalarial("Salario ordinario", "900000")}}
	con := CalcularLiquidacion(base, p)
	p.AplicaINA = false
	sin := CalcularLiquidacion(base, p)
	exigir(t, "diferencia INA", con.Patronal.Sub(sin.Patronal), "13500") // 1.5% de 900k
}

// Los créditos familiares nunca vuelven negativa la renta (piso 0).
func TestRentaPisoCero(t *testing.T) {
	p := calcParams(t)
	renta := calcularRenta(dec("920000"), 10, true, p) // impuesto 200 − créditos 19 690 → 0
	exigir(t, "renta piso", renta, "0")
}

func decPtr(s string) *decimal.Decimal {
	d := dec(s)
	return &d
}

// ─────────── Pago QUINCENAL real (maqueta aprobada 2026-07-29) ───────────
// Ana: ₡1 000 000 al mes, 1 hijo, préstamo ₡40 000 cada quincena, ahorro ₡20 000 solo 2ª,
// y ₡200 000 de comisiones que se pagan en la segunda quincena.

// 1ª quincena: media base, CCSS sobre ella y MITAD del impuesto mensual estimado.
func TestQuincena1(t *testing.T) {
	p := calcParams(t)
	r := CalcularLiquidacion(EmpleadoCalc{
		Hijos: 1, SalarioBase: dec("1000000"),
		Ingresos: []IngresoCalc{ingresoSalarial("Salario de quincena", "500000")},
		Deducciones: []DeduccionCalc{
			{ID: "d1", Etiqueta: "Préstamo Asociación", Cuota: dec("40000"), SaldoRestante: decPtr("315000"), Prioridad: 100},
		},
		// Los tramos son mensuales: se estima sobre el salario del mes y se retiene la mitad.
		Renta: RentaPeriodo{BaseMensual: dec("1000000"), Fraccion: dec("0.5")},
	}, p)

	exigir(t, "bruto", r.Bruto, "500000")
	exigir(t, "CCSS obrero", r.CCSSObrero, "54150") // 27500 + 21650 + 5000
	exigir(t, "renta mitad estimada", r.Renta, "3245")
	exigir(t, "deducciones", r.Deducciones, "40000")
	exigir(t, "neto", r.Neto, "402605")
}

// 2ª quincena: media base + comisiones del mes; la renta se recalcula sobre el mes REAL
// y cobra la diferencia contra lo retenido el día 15.
func TestQuincena2ConAjusteDeRenta(t *testing.T) {
	p := calcParams(t)
	r := CalcularLiquidacion(EmpleadoCalc{
		Hijos: 1, SalarioBase: dec("1000000"),
		Ingresos: []IngresoCalc{
			ingresoSalarial("Salario de quincena", "500000"),
			ingresoSalarial("Comisiones", "200000"),
		},
		Deducciones: []DeduccionCalc{
			{ID: "d1", Etiqueta: "Préstamo Asociación", Cuota: dec("40000"), SaldoRestante: decPtr("275000"), Prioridad: 100},
			{ID: "d2", Etiqueta: "Ahorro navideño", Cuota: dec("20000"), Prioridad: 100},
		},
		Renta: RentaPeriodo{BaseMensual: dec("1200000"), YaRetenida: dec("3245")},
	}, p)

	exigir(t, "bruto", r.Bruto, "700000")
	exigir(t, "CCSS obrero", r.CCSSObrero, "75810") // 38500 + 30310 + 7000
	exigir(t, "renta ajuste", r.Renta, "23245")     // 26490 del mes − 3245 ya retenido
	exigir(t, "deducciones", r.Deducciones, "60000")
	exigir(t, "neto", r.Neto, "540945")
}

// El mes CUADRA EXACTO: la suma de las dos quincenas es lo que corresponde al mes,
// ni un colón de menos para la CCSS ni para Hacienda (partir el pago no reduce nada).
func TestCuadreMensualQuincenal(t *testing.T) {
	p := calcParams(t)
	q1 := CalcularLiquidacion(EmpleadoCalc{
		Hijos: 1, SalarioBase: dec("1000000"),
		Ingresos: []IngresoCalc{ingresoSalarial("Salario de quincena", "500000")},
		Renta:    RentaPeriodo{BaseMensual: dec("1000000"), Fraccion: dec("0.5")},
	}, p)
	q2 := CalcularLiquidacion(EmpleadoCalc{
		Hijos: 1, SalarioBase: dec("1000000"),
		Ingresos: []IngresoCalc{
			ingresoSalarial("Salario de quincena", "500000"),
			ingresoSalarial("Comisiones", "200000"),
		},
		Renta: RentaPeriodo{BaseMensual: dec("1200000"), YaRetenida: q1.Renta},
	}, p)
	// Referencia: el mismo mes pagado de una sola vez (jornada mensual).
	mes := CalcularLiquidacion(EmpleadoCalc{
		Hijos: 1, SalarioBase: dec("1000000"),
		Ingresos: []IngresoCalc{
			ingresoSalarial("Salario ordinario", "1000000"),
			ingresoSalarial("Comisiones", "200000"),
		},
	}, p)

	exigir(t, "base CCSS del mes", q1.BaseCCSS.Add(q2.BaseCCSS), mes.BaseCCSS.String())
	exigir(t, "CCSS obrero del mes", q1.CCSSObrero.Add(q2.CCSSObrero), mes.CCSSObrero.String())
	exigir(t, "renta del mes", q1.Renta.Add(q2.Renta), mes.Renta.String())
	exigir(t, "neto del mes", q1.Neto.Add(q2.Neto), mes.Neto.String())
}

// Frecuencia de cobro: en jornada quincenal manda la frecuencia; en jornada mensual
// (un solo pago) la cuota se cobra una vez, cualquiera sea la frecuencia configurada.
func TestFrecuenciaDeduccion(t *testing.T) {
	casos := []struct {
		frecuencia       string
		primera, segunda bool
	}{
		{FrecAmbas, true, true},
		{FrecPrimera, true, false},
		{FrecSegunda, false, true},
		{FrecMensual, false, true},
	}
	for _, c := range casos {
		if got := CobraEn(c.frecuencia, true, true); got != c.primera {
			t.Errorf("CobraEn(%s, 1ª quincena) = %v, quiere %v", c.frecuencia, got, c.primera)
		}
		if got := CobraEn(c.frecuencia, false, true); got != c.segunda {
			t.Errorf("CobraEn(%s, 2ª quincena) = %v, quiere %v", c.frecuencia, got, c.segunda)
		}
		if got := CobraEn(c.frecuencia, false, false); !got {
			t.Errorf("CobraEn(%s, jornada mensual) = false, quiere true", c.frecuencia)
		}
	}
}

// Si en la 2ª quincena el mes real resulta MENOR que lo estimado (p. ej. una novedad que
// no se dio), la retención del período es 0 — nunca negativa.
func TestAjusteRentaNoNegativo(t *testing.T) {
	p := calcParams(t)
	r := CalcularLiquidacion(EmpleadoCalc{
		SalarioBase: dec("1000000"),
		Ingresos:    []IngresoCalc{ingresoSalarial("Salario de quincena", "500000")},
		Renta:       RentaPeriodo{BaseMensual: dec("1000000"), YaRetenida: dec("50000")},
	}, p)
	exigir(t, "renta piso 0", r.Renta, "0")
}
