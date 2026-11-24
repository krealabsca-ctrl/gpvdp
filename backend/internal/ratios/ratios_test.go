package ratios

// La matemática del DSO, el DPO y el ciclo de efectivo.
//
// Un ratio equivocado no se ve equivocado: sale un número de dos cifras que alguien lleva a una
// junta o a un banco. Por eso lo que se prueba acá es el CRITERIO —que el agotamiento no promedie,
// que un resultado incompleto se declare como piso y no como medición— y no solo que la división
// dé bien.

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// mesesParejos: tres meses iguales de 30 días, para los casos donde el promedio sí sirve.
func mesesParejos(monto string) []MesDeBase {
	return []MesDeBase{
		{Periodo: "2026-10", Monto: d(monto), Dias: 30},
		{Periodo: "2026-09", Monto: d(monto), Dias: 30},
		{Periodo: "2026-08", Monto: d(monto), Dias: 30},
	}
}

// EL CASO BASE: una cartera que equivale a un mes y medio de facturación.
func TestElAgotamientoCuentaDiasReales(t *testing.T) {
	// 150 de cartera contra meses de 100: se come el primero entero (30 días) y medio del
	// segundo (15) = 45 días.
	r := CalcularRatio(d("150"), mesesParejos("100"), "facturación")
	if !r.Calculable {
		t.Fatalf("no se pudo calcular: %+v", r)
	}
	if r.Dias != "45.0" {
		t.Errorf("días por agotamiento = %q, se esperaba 45.0", r.Dias)
	}
	if r.MesesUsados != 2 {
		t.Errorf("meses usados = %d, se esperaban 2", r.MesesUsados)
	}
	// Con meses parejos el simple tiene que coincidir: es el control de que las dos fórmulas
	// miden lo mismo cuando no hay dispersión.
	if r.DiasSimple != "45.0" {
		t.Errorf("días simple = %q; con meses iguales tiene que dar lo mismo", r.DiasSimple)
	}
}

// EL AGOTAMIENTO NO PROMEDIA, y es toda su razón de ser.
//
// Con meses muy dispares —medido en la empresa: de ₡73 a ₡187 millones— el promedio da un número
// que no corresponde a ningún mes real. El countback usa los meses como son.
func TestConMesesDisparesElAgotamientoYElPromedioSeSeparan(t *testing.T) {
	// El mes más reciente es grande; los anteriores, chicos.
	meses := []MesDeBase{
		{Periodo: "2026-10", Monto: d("900"), Dias: 30},
		{Periodo: "2026-09", Monto: d("100"), Dias: 30},
		{Periodo: "2026-08", Monto: d("100"), Dias: 30},
	}
	// Cartera de 450: el countback la agota dentro del mes grande → 450/900 × 30 = 15 días.
	r := CalcularRatio(d("450"), meses, "facturación")
	if r.Dias != "15.0" {
		t.Errorf("agotamiento = %q, se esperaba 15.0", r.Dias)
	}
	// El simple promedia (900+100+100)/3 = 366,67 → 450/366,67 × 30 ≈ 36,8 días: más del doble.
	if r.DiasSimple == r.Dias {
		t.Error("el método simple dio igual que el agotamiento; con meses dispares tienen que separarse")
	}
	simple := d(r.DiasSimple)
	if !simple.GreaterThan(d("30")) {
		t.Errorf("días simple = %q; con el último mes grande el promedio tiene que exagerar", r.DiasSimple)
	}
}

// UN MES DE FEBRERO NO SON 30 DÍAS. Prorratear contra un mes fijo mete un error de hasta un 10 %
// en un número que se compara contra el trimestre anterior.
func TestElProrrateoUsaLosDiasDelMesQueAgota(t *testing.T) {
	meses := []MesDeBase{{Periodo: "2026-02", Monto: d("100"), Dias: 28}}
	// Media cartera del mes: la mitad de 28 días, no de 30.
	r := CalcularRatio(d("50"), meses, "facturación")
	if r.Dias != "14.0" {
		t.Errorf("días = %q, se esperaba 14.0 (medio febrero)", r.Dias)
	}
}

// UNA CARTERA MAYOR QUE TODA LA VENTANA ES UN PISO, NO UNA MEDICIÓN.
//
// Es el caso más grave —la cartera equivale a más de lo que se registró en un año— y es justo
// donde devolver el acumulado como si fuera el valor mentiría por lo bajo.
func TestUnaCarteraMayorQueLaVentanaSeDeclaraComoMinimo(t *testing.T) {
	r := CalcularRatio(d("10000"), mesesParejos("100"), "facturación")
	if !r.Calculable {
		t.Fatal("debería calcularse igual, como piso")
	}
	// Se comió los tres meses: 90 días, y no hay con qué seguir.
	if r.Dias != "90.0" {
		t.Errorf("días = %q, se esperaba el tope de la ventana (90.0)", r.Dias)
	}
	if r.Aviso == "" || !strings.Contains(r.Aviso, "MÍNIMO") {
		t.Errorf("no se declaró como mínimo: %q", r.Aviso)
	}
}

// SIN BASE NO HAY RATIO. Devolver cero diría «cobro el mismo día», que es lo contrario de «no sé».
func TestSinBaseElRatioNoSeCalcula(t *testing.T) {
	casos := []struct {
		nombre string
		meses  []MesDeBase
	}{
		{"ventana vacía", nil},
		{"meses todos en cero", mesesParejos("0")},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			r := CalcularRatio(d("5000"), c.meses, "facturación")
			if r.Calculable {
				t.Errorf("se calculó sin base: %+v", r)
			}
			if r.Dias != "" {
				t.Errorf("devolvió días (%q) sin con qué medir", r.Dias)
			}
			if r.Aviso == "" {
				t.Error("no explicó por qué no se puede")
			}
		})
	}
}

// Sin cartera pendiente, el ratio es CERO y es calculable: no deber nada es una respuesta, no una
// falta de datos.
func TestSinCarteraElRatioEsCero(t *testing.T) {
	r := CalcularRatio(decimal.Zero, mesesParejos("100"), "facturación")
	if !r.Calculable || r.Dias != "0.0" {
		t.Errorf("sin cartera: calculable=%v días=%q", r.Calculable, r.Dias)
	}
	if r.Aviso != "" {
		t.Errorf("avisó algo que no hacía falta: %q", r.Aviso)
	}
}

// Los meses SIN facturación no consumen días: contarlos como mes completo inflaría el ratio con
// períodos en los que no pasó nada.
func TestLosMesesVaciosNoCuentanDias(t *testing.T) {
	meses := []MesDeBase{
		{Periodo: "2026-10", Monto: d("0"), Dias: 31},
		{Periodo: "2026-09", Monto: d("100"), Dias: 30},
	}
	// Los 100 de cartera se agotan en el mes que sí tuvo facturación: 30 días, no 61.
	r := CalcularRatio(d("100"), meses, "facturación")
	if r.Dias != "30.0" {
		t.Errorf("días = %q, se esperaba 30.0 (el mes vacío no cuenta)", r.Dias)
	}
	if r.MesesUsados != 1 {
		t.Errorf("meses usados = %d, se esperaba 1", r.MesesUsados)
	}
}

// EL SIGNO DEL CICLO ES LA LECTURA: positivo = financiás vos; negativo = te financian.
func TestElCicloSumaCobroEInventarioYRestaPago(t *testing.T) {
	dso := CalcularRatio(d("150"), mesesParejos("100"), "facturación") // 45 días
	dpo := CalcularRatio(d("300"), mesesParejos("100"), "compras")     // 90 días
	dio := CalcularRatio(d("30"), mesesParejos("100"), "costo")        // 9 días

	ciclo, ok := CalcularCiclo(dso, dpo, dio)
	if !ok {
		t.Fatal("no se calculó el ciclo")
	}
	// 45 + 9 − 90 = −36: los proveedores financian la operación 36 días.
	if ciclo != "-36.0" {
		t.Errorf("ciclo = %q, se esperaba -36.0", ciclo)
	}
}

// Si falta uno de los dos ratios que de verdad importan, NO hay ciclo. Calcularlo con un hueco
// daría un número que se lee como completo.
func TestSinDSOoDPONoHayCiclo(t *testing.T) {
	bueno := CalcularRatio(d("150"), mesesParejos("100"), "facturación")
	vacio := CalcularRatio(d("150"), nil, "compras")

	if _, ok := CalcularCiclo(vacio, bueno, Ratio{}); ok {
		t.Error("se calculó el ciclo sin DSO")
	}
	if _, ok := CalcularCiclo(bueno, vacio, Ratio{}); ok {
		t.Error("se calculó el ciclo sin DPO")
	}
}

// El DIO ausente entra como cero y el ciclo se calcula igual: sin inventario cargado, el ciclo es
// DSO − DPO, que ya es la lectura principal. Lo que no se puede es callarlo, y de eso se encarga
// el aviso del servicio.
func TestElCicloSeCalculaConDIOAusente(t *testing.T) {
	dso := CalcularRatio(d("150"), mesesParejos("100"), "facturación") // 45
	dpo := CalcularRatio(d("300"), mesesParejos("100"), "compras")     // 90
	ciclo, ok := CalcularCiclo(dso, dpo, Ratio{})
	if !ok {
		t.Fatal("no se calculó el ciclo sin inventario")
	}
	if ciclo != "-45.0" {
		t.Errorf("ciclo = %q, se esperaba -45.0", ciclo)
	}
}
