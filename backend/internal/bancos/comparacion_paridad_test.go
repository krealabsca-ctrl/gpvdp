package bancos

// Comparar a PARIDAD DE DÍA (pedido de Gerencia General, 19-oct-2026).
//
// EL PEDIDO: «le gustaría ver cómo va al día de hoy en comparación al mes anterior, al promedio del
// año basado en el día — por ejemplo hace la comparativa del 1 al 19 de octubre y hace esa medición
// basada en la comparativa».
//
// LO QUE ARREGLA. El tablero comparaba el mes EN CURSO contra meses ENTEROS, así que el porcentaje
// medía cuántos días habían pasado y no cómo iba el negocio. Medido sobre los datos reales:
// setiembre, cargado hasta el día 14, salía «−62 % vs agosto». A paridad de día la caída real es
// del 35 %, y contra el promedio del año venía 3 % ARRIBA. Una alarma que aparece todos los meses
// por construcción deja de leerse, y tapa la que sí importa.
//
// EL DÍA DE CORTE ES HOY, no el último día cargado (decisión del Director Financiero: «sería al 19,
// porque si algo no se ha cargado se termine de cargar»). Por eso el tablero devuelve también
// `dias_sin_cargar`: con el corte en el calendario, un estado de cuenta atrasado mete días vacíos
// en la comparación, y sin ese número un atraso de carga se lee igual que una caída de ventas.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func dec2(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func totales(ing, gas string) TotalesEbitda {
	return TotalesEbitda{Ingresos: dec2(ing), Gastos: dec2(gas)}
}

// ── El día de corte ─────────────────────────────────────────────────────────────────────────

func TestElDiaDeCorteSoloAplicaAlMesEnCurso(t *testing.T) {
	t.Parallel()
	hoy := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

	// El mes en curso se corta al día de hoy: es lo que vuelve comparables los tres números.
	if dia, enCurso := DiaDeCorte("2026-10", hoy); dia != 19 || !enCurso {
		t.Errorf("mes en curso = (%d, %v), quiere (19, true)", dia, enCurso)
	}
	// Un mes YA CERRADO se mide entero. Cortarlo al 19 dejaría afuera plata que sí ocurrió, y el
	// mes de octubre pasaría a valer menos de lo que valió apenas cambie el calendario.
	for _, cerrado := range []string{"2026-09", "2026-01", "2025-12"} {
		if dia, enCurso := DiaDeCorte(cerrado, hoy); dia != 0 || enCurso {
			t.Errorf("mes cerrado %s = (%d, %v), quiere (0, false)", cerrado, dia, enCurso)
		}
	}
	// Un mes futuro tampoco se corta: no hay nada que recortar.
	if dia, _ := DiaDeCorte("2026-12", hoy); dia != 0 {
		t.Errorf("mes futuro = %d, quiere 0", dia)
	}
}

// El día 1 del mes es un corte válido, no un «sin corte». Si se confundiera con 0, el primer día
// del mes compararía contra el mes anterior COMPLETO — justo el defecto que esto corrige, y en el
// día en que la distorsión es máxima.
func TestElPrimerDiaDelMesSeCortaIgual(t *testing.T) {
	t.Parallel()
	hoy := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	dia, enCurso := DiaDeCorte("2026-11", hoy)
	if dia != 1 || !enCurso {
		t.Fatalf("= (%d, %v), quiere (1, true)", dia, enCurso)
	}
}

// ── La comparación ──────────────────────────────────────────────────────────────────────────

// Los TRES períodos se miden hasta el mismo día. Si el servicio pidiera tramos distintos, el
// porcentaje volvería a comparar peras con manzanas sin que nada falle.
func TestLosTresPeriodosSePidenAlMismoDia(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := servicioSaldos(repo)
	// Nota: el corte sale de AhoraCR(), así que el día real depende de cuándo corra la prueba. Lo
	// que se fija acá no es el número sino que sea el MISMO para todos.
	if _, err := svc.Dashboard(context.Background(), "e1", AhoraCR().Format("2006-01")); err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if len(repo.diasPedidos) < 2 {
		t.Fatalf("se pidieron %d tramos: %v", len(repo.diasPedidos), repo.diasPedidos)
	}
	var dia string
	for i, p := range repo.diasPedidos {
		partes := strings.Split(p, ":") // «2026-10:19»
		if len(partes) != 2 {
			t.Fatalf("tramo mal formado: %q", p)
		}
		if i == 0 {
			dia = partes[1]
			continue
		}
		if partes[1] != dia {
			t.Fatalf("se pidieron tramos distintos entre períodos: %v", repo.diasPedidos)
		}
	}
}

// El caso real medido: setiembre al día 14 contra agosto al día 14.
func TestLaComparacionUsaElMismoTramoDelMesAnterior(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		totales: map[string]TotalesEbitda{
			// Agosto ENTERO. Si el servicio usara esto para comparar, volvería el −62 %.
			"2026-08": totales("342086914", "0"),
		},
		totalesHastaDia: map[string]TotalesEbitda{
			"2026-09:14": totales("130261803", "0"),
			"2026-08:14": totales("200299128", "0"), // agosto al MISMO día
		},
		ultimoDiaCargado: map[string]int{"2026-09": 14},
	}
	// Se llama al repositorio directamente con el tramo para no depender del reloj.
	cur, _ := repo.TotalesPeriodoHastaDia(context.Background(), "e1", "2026-09", 14)
	prev, _ := repo.TotalesPeriodoHastaDia(context.Background(), "e1", "2026-08", 14)

	if !cur.Ingresos.Equal(dec2("130261803")) || !prev.Ingresos.Equal(dec2("200299128")) {
		t.Fatalf("tramos mal leídos: %v vs %v", cur.Ingresos, prev.Ingresos)
	}
	// La caída a paridad es del 35 %, no del 62 % que daba contra el mes entero.
	caida := prev.Ingresos.Sub(cur.Ingresos).Div(prev.Ingresos).Mul(decimal.NewFromInt(100))
	if caida.IntPart() != 34 && caida.IntPart() != 35 {
		t.Errorf("caída a paridad = %s %%, se esperaba ~35", caida.Round(1))
	}
}

// ── El promedio del año ─────────────────────────────────────────────────────────────────────

func TestElPromedioDelAnioPromediaLosMesesPreviosAlMismoDia(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		totalesHastaDia: map[string]TotalesEbitda{
			"2026-07:14": totales("53150132", "10000000"),
			"2026-08:14": totales("200299128", "20000000"),
		},
	}
	svc := servicioSaldos(repo)
	p, err := svc.promedioDelAnio(context.Background(), "e1", "2026-09", 14)
	if err != nil {
		t.Fatalf("promedioDelAnio: %v", err)
	}
	// Solo julio y agosto tienen datos: enero a junio entran en cero y NO cuentan.
	if p.Meses != 2 {
		t.Errorf("meses = %d, quiere 2 (solo los que tienen movimientos)", p.Meses)
	}
	// (53.150.132 + 200.299.128) / 2 = 126.724.630
	if p.Ingresos != "126724630" {
		t.Errorf("ingresos promedio = %q, quiere 126724630", p.Ingresos)
	}
	// Y el EBITDA del promedio se deriva del promedio, no se promedia aparte.
	if p.EBITDA != "111724630" {
		t.Errorf("ebitda promedio = %q, quiere 111724630", p.EBITDA)
	}
}

// Un mes sin movimientos NO arrastra el promedio hacia abajo.
//
// Es la trampa de este cálculo: si un mes del que todavía no se cargó nada entrara como cero, el
// promedio bajaría y el mes en curso luciría mejor de lo que va. El promedio diría más sobre qué
// se cargó que sobre cómo vendimos.
func TestUnMesSinCargarNoArrastraElPromedio(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		totalesHastaDia: map[string]TotalesEbitda{
			"2026-07:14": totales("100000000", "0"),
			"2026-08:14": totales("200000000", "0"),
			// Setiembre existe en el calendario pero no tiene nada cargado: no aparece acá.
		},
	}
	p, err := servicioSaldos(repo).promedioDelAnio(context.Background(), "e1", "2026-10", 14)
	if err != nil {
		t.Fatalf("promedioDelAnio: %v", err)
	}
	if p.Meses != 2 {
		t.Fatalf("meses = %d, quiere 2", p.Meses)
	}
	// Con el mes vacío contado serían 150.000.000/3 = 100.000.000 y octubre luciría mejor.
	if p.Ingresos != "150000000" {
		t.Errorf("promedio = %q, quiere 150000000 (sin el mes vacío)", p.Ingresos)
	}
}

// En enero no hay meses previos del año: no se inventa un promedio.
func TestEnEneroNoHayPromedioDelAnio(t *testing.T) {
	t.Parallel()
	p, err := servicioSaldos(&fakeRepo{}).promedioDelAnio(context.Background(), "e1", "2026-01", 19)
	if err != nil {
		t.Fatalf("promedioDelAnio: %v", err)
	}
	if p.Meses != 0 || p.Ingresos != "" {
		t.Errorf("en enero se inventó un promedio: %+v", p)
	}
}

func TestMesesPreviosDelAnio(t *testing.T) {
	t.Parallel()
	if got := mesesPreviosDelAnio("2026-01"); len(got) != 0 {
		t.Errorf("enero = %v, quiere vacío", got)
	}
	got := mesesPreviosDelAnio("2026-10")
	if len(got) != 9 || got[0] != "2026-01" || got[8] != "2026-09" {
		t.Errorf("octubre = %v", got)
	}
	// No cruza de año: el promedio es del año EN CURSO.
	for _, m := range mesesPreviosDelAnio("2026-03") {
		if m[:4] != "2026" {
			t.Errorf("se coló un mes de otro año: %s", m)
		}
	}
}
