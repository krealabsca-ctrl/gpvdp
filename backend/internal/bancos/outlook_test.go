package bancos

// El outlook del año.
//
// Lo que se prueba acá es el CRITERIO, no la aritmética: que el mes a medio transcurrir no hunda
// el ritmo, que los dos escenarios se calculen sobre la misma parte hecha, y que un año casi
// entero por delante se declare como proyección. Un outlook equivocado sale como un número de
// nueve cifras que alguien lleva a una junta.

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func od(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// par arma el (ingreso, gasto) de un mes.
func par(ing, gas string) [2]decimal.Decimal { return [2]decimal.Decimal{od(ing), od(gas)} }

// EL MES EN CURSO NO HUNDE EL RITMO. Es la trampa que ya mordió en el Dashboard: un mes a medio
// transcurrir metido en el promedio mide los días que pasaron, no el desempeño.
func TestElMesEnCursoNoEntraEnElRitmo(t *testing.T) {
	// Tres meses cerrados de 1.000 cada uno, y un cuarto mes que lleva 100 al día 3 de 30.
	reales := map[string][2]decimal.Decimal{
		"2026-01": par("1000", "400"),
		"2026-02": par("1000", "400"),
		"2026-03": par("1000", "400"),
		"2026-04": par("100", "40"),
	}
	hoy := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2026, reales, map[string][2]decimal.Decimal{})

	if o.MesesCerrados != 3 {
		t.Fatalf("meses cerrados %d, se esperaban 3", o.MesesCerrados)
	}
	if o.MesesFuturos != 8 {
		t.Fatalf("meses futuros %d, se esperaban 8", o.MesesFuturos)
	}
	// El ritmo sale de los 3 cerrados (1.000/mes), NO de incluir el cuarto a medias:
	//   base = 3.000 (cerrados) + 1.000 (abril a paridad: 100 × 30/3) = 4.000
	//   futuros = 1.000 × 8 = 8.000  →  ingreso = 12.000
	if o.Ritmo.Ingreso != "12000.00" {
		t.Errorf("ingreso del ritmo %q, se esperaba 12000.00", o.Ritmo.Ingreso)
	}
	// Si el mes en curso entrara en el promedio, el ritmo sería (3.000+100)/4 = 775 y el año
	// cerraría mucho más abajo. Ese es el defecto que este test protege.
}

// EL MES EN CURSO SE LLEVA A MES COMPLETO, no se ignora ni se cuenta como mes malo.
func TestElMesEnCursoVaAParidadDeDia(t *testing.T) {
	reales := map[string][2]decimal.Decimal{"2026-04": par("100", "40")}
	hoy := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC) // día 3 de 30
	o := ConstruirOutlook(hoy, 2026, reales, map[string][2]decimal.Decimal{})

	var abril MesDelAnio
	for _, m := range o.Meses {
		if m.Periodo == "2026-04" {
			abril = m
		}
	}
	if abril.Estado != MesEnCurso {
		t.Fatalf("abril quedó como %q", abril.Estado)
	}
	// 100 × 30/3 = 1.000
	if abril.IngresoProyectado != "1000.00" {
		t.Errorf("proyección de abril %q, se esperaba 1000.00", abril.IngresoProyectado)
	}
	// El YTD lleva lo que VA, sin proyectar: son dos cifras distintas y las dos hacen falta.
	if o.IngresoYTD != "100.00" {
		t.Errorf("YTD %q; tiene que ser lo que va, no lo proyectado", o.IngresoYTD)
	}
	if o.AvanceDelMes != "10 %" {
		t.Errorf("avance del mes %q, se esperaba 10 %%", o.AvanceDelMes)
	}
}

// El último día del mes NO se escala: ya transcurrió entero y multiplicarlo lo duplicaría.
func TestElUltimoDiaDelMesNoSeEscala(t *testing.T) {
	reales := map[string][2]decimal.Decimal{"2026-04": par("900", "300")}
	hoy := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2026, reales, map[string][2]decimal.Decimal{})
	for _, m := range o.Meses {
		if m.Periodo == "2026-04" && m.IngresoProyectado != "900.00" {
			t.Errorf("el último día del mes se escaló a %q", m.IngresoProyectado)
		}
	}
}

// LOS DOS ESCENARIOS COMPARTEN LA PARTE HECHA y solo difieren en los meses FUTUROS. Si cada uno
// calculara también lo pasado a su manera, la brecha entre ellos dejaría de medir lo único que
// se quiere comparar.
func TestLosDosEscenariosSoloDifierenEnLoQueFalta(t *testing.T) {
	reales := map[string][2]decimal.Decimal{
		"2026-01": par("1000", "400"),
		"2026-02": par("1000", "400"),
	}
	// La meta de los meses futuros es el doble del ritmo.
	metas := map[string][2]decimal.Decimal{}
	for m := 1; m <= 12; m++ {
		metas[periodoDe(2026, m)] = par("2000", "400")
	}
	hoy := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) // marzo, día 1 de 31

	o := ConstruirOutlook(hoy, 2026, reales, metas)

	// Hecho: 2.000 de los cerrados + 0 de marzo (no hay movimiento) = 2.000.
	// PLAN:  + 2.000 × 9 meses futuros = 20.000
	// RITMO: + 1.000 × 9 = 11.000
	if o.Plan.Ingreso != "20000.00" {
		t.Errorf("ingreso del plan %q, se esperaba 20000.00", o.Plan.Ingreso)
	}
	if o.Ritmo.Ingreso != "11000.00" {
		t.Errorf("ingreso del ritmo %q, se esperaba 11000.00", o.Ritmo.Ingreso)
	}
	// Y el aviso tiene que nombrar la brecha en la dirección correcta.
	var hallado bool
	for _, a := range o.Avisos {
		if strings.Contains(a, "POR DEBAJO del plan") {
			hallado = true
		}
	}
	if !hallado {
		t.Errorf("no se avisó que el ritmo cierra bajo el plan: %v", o.Avisos)
	}
}

// SIN META NO HAY ESCENARIO «PLAN», y se dice en vez de devolver un año que cierra en cero.
func TestSinMetaElPlanNoSeCalcula(t *testing.T) {
	reales := map[string][2]decimal.Decimal{"2026-01": par("1000", "400")}
	hoy := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2026, reales, map[string][2]decimal.Decimal{})

	if o.Plan.Calculable {
		t.Error("el plan se declaró calculable sin ninguna meta")
	}
	if !strings.Contains(o.Plan.Aviso, "artificialmente bajo") {
		t.Errorf("el aviso no dice hacia qué lado engaña: %q", o.Plan.Aviso)
	}
	// El ritmo sí: hay un mes cerrado.
	if !o.Ritmo.Calculable {
		t.Error("el ritmo tendría que calcularse con un mes cerrado")
	}
}

// SIN NINGÚN MES CERRADO NO HAY RITMO. Proyectar el año desde un mes que apenas empezó sería
// tomar tres días como si describieran doce meses.
func TestSinMesesCerradosNoHayRitmo(t *testing.T) {
	reales := map[string][2]decimal.Decimal{"2026-01": par("100", "40")}
	hoy := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2026, reales, map[string][2]decimal.Decimal{})

	if o.MesesCerrados != 0 {
		t.Fatalf("meses cerrados %d", o.MesesCerrados)
	}
	if o.Ritmo.Calculable {
		t.Error("se calculó un ritmo sin ningún mes cerrado")
	}
	if !strings.Contains(o.Ritmo.Aviso, "no hay un ritmo") {
		t.Errorf("aviso del ritmo: %q", o.Ritmo.Aviso)
	}
}

// UN AÑO CASI ENTERO POR DELANTE SE DECLARA COMO PROYECCIÓN. El número se ve igual de firme
// armado en febrero que en noviembre, y no lo es.
func TestUnAnioCasiEnteroPorDelanteSeDeclara(t *testing.T) {
	hoy := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2026, map[string][2]decimal.Decimal{"2026-01": par("1000", "400")},
		map[string][2]decimal.Decimal{})
	var hallado bool
	for _, a := range o.Avisos {
		if strings.Contains(a, "sobre todo proyección") {
			hallado = true
		}
	}
	if !hallado {
		t.Errorf("con 10 meses por venir no se avisó que es proyección: %v", o.Avisos)
	}
}

// Un año ya terminado no tiene mes en curso ni futuros: es todo medición.
func TestUnAnioPasadoEsTodoMedicion(t *testing.T) {
	reales := map[string][2]decimal.Decimal{}
	for m := 1; m <= 12; m++ {
		reales[periodoDe(2025, m)] = par("1000", "400")
	}
	hoy := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2025, reales, map[string][2]decimal.Decimal{})

	if o.MesesCerrados != 12 || o.MesesFuturos != 0 || o.MesEnCurso != "" {
		t.Errorf("año pasado: cerrados=%d futuros=%d enCurso=%q",
			o.MesesCerrados, o.MesesFuturos, o.MesEnCurso)
	}
	// Sin meses futuros, los dos escenarios dan lo mismo: lo que de verdad pasó.
	if o.Plan.Ingreso != "12000.00" || o.Ritmo.Ingreso != "12000.00" {
		t.Errorf("plan %q y ritmo %q; en un año cerrado tienen que coincidir con el real",
			o.Plan.Ingreso, o.Ritmo.Ingreso)
	}
	if o.ResultadoYTD != "7200.00" {
		t.Errorf("resultado del año %q, se esperaba 7200.00", o.ResultadoYTD)
	}
}

// El presupuesto del año es el DECLARADO completo, no solo el de los meses que faltan: es la
// columna contra la que se compara todo lo demás.
func TestElPresupuestoDelAnioEsElDeLosDoceMeses(t *testing.T) {
	metas := map[string][2]decimal.Decimal{}
	for m := 1; m <= 12; m++ {
		metas[periodoDe(2026, m)] = par("1000", "600")
	}
	hoy := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2026, map[string][2]decimal.Decimal{}, metas)

	if o.IngresoPresupuesto != "12000.00" || o.GastoPresupuesto != "7200.00" {
		t.Errorf("presupuesto del año: ingreso %q, gasto %q",
			o.IngresoPresupuesto, o.GastoPresupuesto)
	}
	if o.ResultadoPresupuesto != "4800.00" {
		t.Errorf("resultado presupuestado %q", o.ResultadoPresupuesto)
	}
}

// UN MES CERRADO SIN MOVIMIENTO NO HUNDE EL RITMO.
//
// Lo destapó la pantalla con datos reales: Memorial tenía nueve meses con movimiento y octubre
// cerrado en blanco. Contarlo como un mes de cero bajaba el promedio un 10 % y el año proyectado
// salía corto —con un número perfectamente plausible—. Un mes sin movimiento casi nunca es un mes
// sin operación: es un estado de cuenta que falta importar.
func TestUnMesCerradoSinDatosNoEntraEnElRitmo(t *testing.T) {
	reales := map[string][2]decimal.Decimal{
		"2026-01": par("1000", "400"),
		"2026-02": par("1000", "400"),
		"2026-03": par("0", "0"), // cerrado y en blanco: no se cargó
	}
	hoy := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2026, reales, map[string][2]decimal.Decimal{})

	if o.MesesCerrados != 3 {
		t.Fatalf("meses cerrados %d, se esperaban 3", o.MesesCerrados)
	}
	if o.MesesCerradosSinDatos != 1 {
		t.Errorf("meses cerrados sin datos %d, se esperaba 1", o.MesesCerradosSinDatos)
	}
	// El ritmo promedia sobre los DOS con movimiento (1.000/mes), no sobre los tres (666,67):
	//   base = 2.000 + 0 (abril, sin movimiento) ; futuros = 1.000 × 8 = 8.000 → 10.000
	if o.Ritmo.Ingreso != "10000.00" {
		t.Errorf("ingreso del ritmo %q, se esperaba 10000.00 (el mes en blanco no promedia)", o.Ritmo.Ingreso)
	}
	// Y hay que decirlo: mientras ese mes falte, el año va corto por los dos lados.
	var avisado bool
	for _, a := range o.Avisos {
		if strings.Contains(a, "no tienen ningún movimiento cargado") {
			avisado = true
		}
	}
	if !avisado {
		t.Errorf("no se avisó del mes cerrado sin datos: %v", o.Avisos)
	}
}

// Con TODOS los meses cerrados en blanco no hay ritmo: no se inventa uno desde ceros.
func TestSinNingunMesConMovimientoNoHayRitmo(t *testing.T) {
	reales := map[string][2]decimal.Decimal{
		"2026-01": par("0", "0"),
		"2026-02": par("0", "0"),
	}
	hoy := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	o := ConstruirOutlook(hoy, 2026, reales, map[string][2]decimal.Decimal{})
	if o.Ritmo.Calculable {
		t.Error("se calculó un ritmo con todos los meses en blanco")
	}
	if !strings.Contains(o.Ritmo.Aviso, "CON MOVIMIENTO") {
		t.Errorf("el aviso no distingue «cerrado» de «cerrado con movimiento»: %q", o.Ritmo.Aviso)
	}
}
