package bancos

// El flujo de caja proyectado.
//
// Lo que se prueba acá no es que sepa multiplicar, sino que NO MIENTA: que la cartera vencida no
// se cuele como si fuera un egreso del mes que viene, que las partidas sin ritmo no se proyecten
// como si lo tuvieran, y que con poca historia lo diga en vez de dibujar una curva.

import (
	"context"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// servicioConFlujo arma un servicio con historia y cartera sembradas.
func servicioConFlujo(salud map[string][]SaludMes, series map[string][]TendenciaPartida, vencido, porVencer string) *Service {
	repo := &fakeRepo{
		saludPorRango: salud, seriePorRango: series,
		cxpVencido: montoDec(vencido), cxpPorVencer: montoDec(porVencer),
	}
	return &Service{repo: repo}
}

// docePeriodos arma doce meses sanos y una partida con el mismo monto cada mes.
func docePeriodos(monto string, naturaleza string) ([]SaludMes, map[string]string) {
	salud := make([]SaludMes, 0, 12)
	serie := map[string]string{}
	for i := 1; i <= 12; i++ {
		p := "2026-" + dosDigitos(i)
		salud = append(salud, mesSano(p))
		serie[p] = monto
	}
	_ = naturaleza
	return salud, serie
}

func partidaCon(id, nombre, naturaleza string, serie map[string]string) TendenciaPartida {
	p := partida(id, nombre, serie)
	p.Naturaleza = naturaleza
	p.NaturalezaDeclarada = true
	if naturaleza == "INGRESO" {
		p.Concepto = "Ingresos"
	}
	return p
}

// LA PRUEBA QUE IMPORTA: la cartera vencida NO entra en la proyección.
//
// Medido el 28-oct-2026 sobre la cartera real de Valle de Paz: ₡936,9 M vencidos contra ₡61,6 M
// por vencer —el 99,9 % ya venció—. Si eso se sumara al mes que viene, el flujo proyectado diría
// que la empresa necesita mil millones en noviembre, cuando es una deuda que se arrastra desde
// 2023. Se informa aparte, nunca mezclado.
func TestLaCarteraVencidaNoSeMezclaConLaProyeccion(t *testing.T) {
	salud, serie := docePeriodos("1000000", "GASTO")
	svc := servicioConFlujo(
		map[string][]SaludMes{"2026-01..2026-12": salud},
		map[string][]TendenciaPartida{"2026-01..2026-12": {
			partidaCon("c1", "Planilla", "GASTO", serie),
		}},
		"936930724.00", "61630009.00",
	)

	res, err := svc.ProyectarFlujo(context.Background(), "e1", "2026-01", "2026-12", 3)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	if len(res.Meses) != 3 {
		t.Fatalf("vinieron %d meses, se pidieron 3", len(res.Meses))
	}
	// El gasto proyectado es el ritmo de la partida, no la cartera.
	for _, m := range res.Meses {
		if m.Gastos != "1000000.00" {
			t.Errorf("%s: gastos %q; se esperaba el ritmo de 1000000.00, no la cartera", m.Periodo, m.Gastos)
		}
	}
	// Y la cartera se informa, en su propio lugar.
	if res.CxPVencido != "936930724.00" || res.CxPPorVencer != "61630009.00" {
		t.Errorf("cartera: vencido %q, por vencer %q", res.CxPVencido, res.CxPPorVencer)
	}
}

// Los meses proyectados son los SIGUIENTES al histórico, en orden y sin saltarse el fin de año.
func TestLosMesesProyectadosSiguenAlHistorico(t *testing.T) {
	salud, serie := docePeriodos("500", "GASTO")
	svc := servicioConFlujo(
		map[string][]SaludMes{"2026-01..2026-12": salud},
		map[string][]TendenciaPartida{"2026-01..2026-12": {partidaCon("c1", "Planilla", "GASTO", serie)}},
		"0", "0",
	)
	res, err := svc.ProyectarFlujo(context.Background(), "e1", "2026-01", "2026-12", 3)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	quiero := []string{"2027-01", "2027-02", "2027-03"}
	for i, m := range res.Meses {
		if m.Periodo != quiero[i] {
			t.Errorf("mes %d: %q, se esperaba %q", i, m.Periodo, quiero[i])
		}
	}
}

// Ingresos y gastos van por su NATURALEZA DECLARADA. Lo que nadie declaró no entra: no se sabe de
// qué lado sumarlo, y elegir uno sería inventar el resultado.
func TestElFlujoSeparaIngresosDeGastosYDejaFueraLoNoDeclarado(t *testing.T) {
	salud, serie := docePeriodos("100", "")
	sinDeclarar := partida("c3", "Sin decidir", serie)
	sinDeclarar.NaturalezaDeclarada = false

	svc := servicioConFlujo(
		map[string][]SaludMes{"2026-01..2026-12": salud},
		map[string][]TendenciaPartida{"2026-01..2026-12": {
			partidaCon("c1", "Ventas", "INGRESO", serie),
			partidaCon("c2", "Planilla", "GASTO", serie),
			sinDeclarar,
		}},
		"0", "0",
	)
	res, err := svc.ProyectarFlujo(context.Background(), "e1", "2026-01", "2026-12", 1)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	m := res.Meses[0]
	if m.Ingresos != "100.00" || m.Gastos != "100.00" {
		t.Fatalf("ingresos %q, gastos %q; se esperaba 100.00 en cada uno", m.Ingresos, m.Gastos)
	}
	if m.Neto != "0.00" {
		t.Errorf("neto %q, se esperaba 0.00", m.Neto)
	}
}

// UNA PARTIDA SIN RITMO NO SE PROYECTA COMO SI LO TUVIERA, pero su gasto tampoco desaparece: se
// reparte sobre el rango. Una que gasta ₡1,2 M una vez al año mueve ₡100.000 por mes, no ₡1,2 M.
func TestLaPartidaEsporadicaSeRepartePeroNoSeDuplica(t *testing.T) {
	salud := make([]SaludMes, 0, 12)
	serie := map[string]string{}
	for i := 1; i <= 12; i++ {
		p := "2026-" + dosDigitos(i)
		salud = append(salud, mesSano(p))
		serie[p] = "0"
	}
	serie["2026-06"] = "1200000" // una sola vez en el año

	svc := servicioConFlujo(
		map[string][]SaludMes{"2026-01..2026-12": salud},
		map[string][]TendenciaPartida{"2026-01..2026-12": {
			partidaCon("c1", "Aguinaldos", "GASTO", serie),
		}},
		"0", "0",
	)
	res, err := svc.ProyectarFlujo(context.Background(), "e1", "2026-01", "2026-12", 1)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	if res.PartidasEsporadicas != 1 {
		t.Errorf("esporádicas: %d, se esperaba 1", res.PartidasEsporadicas)
	}
	// 1.200.000 repartido entre 12 meses comparables = 100.000.
	if res.Meses[0].Gastos != "100000.00" {
		t.Errorf("gasto proyectado %q; se esperaba 100000.00 (el anual repartido)", res.Meses[0].Gastos)
	}
	// Y se dice que ese bloque existe, para que nadie lo lea como un gasto fijo.
	if !strings.Contains(res.Aviso, "esporádicas") {
		t.Errorf("el aviso no menciona las partidas esporádicas: %q", res.Aviso)
	}
}

// CON POCA HISTORIA NO SE PROYECTA: se dice. Dibujar una curva sobre dos meses da una precisión
// que los datos no tienen, y una curva convence más que una frase.
func TestConMenosDeTresMesesNoProyecta(t *testing.T) {
	svc := servicioConFlujo(
		map[string][]SaludMes{"2026-01..2026-02": {mesSano("2026-01"), mesFlojo("2026-02")}},
		map[string][]TendenciaPartida{"2026-01..2026-02": {
			partidaCon("c1", "Planilla", "GASTO", map[string]string{"2026-01": "100", "2026-02": "100"}),
		}},
		"0", "0",
	)
	res, err := svc.ProyectarFlujo(context.Background(), "e1", "2026-01", "2026-02", 6)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	if len(res.Meses) != 0 {
		t.Errorf("proyectó %d meses con un solo mes comparable", len(res.Meses))
	}
	if !strings.Contains(res.Aviso, "suposición") {
		t.Errorf("no avisó de la falta de historia: %q", res.Aviso)
	}
}

// LA CARTERA SE VE AUNQUE NO SE PUEDA PROYECTAR, y es cuando más importa: lo encontró el E2E
// sobre Valle de Paz, que no tiene historia clasificada para proyectar y debe ₡936,9 M. La
// pantalla salía en blanco sobre la empresa con la cartera más grande.
func TestLaCarteraSeInformaAunqueNoSePuedaProyectar(t *testing.T) {
	svc := servicioConFlujo(
		map[string][]SaludMes{"2026-07..2026-09": {mesFlojo("2026-07"), mesFlojo("2026-08"), mesFlojo("2026-09")}},
		map[string][]TendenciaPartida{"2026-07..2026-09": {
			partidaCon("c1", "Planilla", "GASTO", map[string]string{"2026-07": "100", "2026-08": "100"}),
		}},
		"936930724.00", "61630009.00",
	)
	res, err := svc.ProyectarFlujo(context.Background(), "e1", "2026-07", "2026-09", 3)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	if len(res.Meses) != 0 {
		t.Fatalf("proyectó sin meses comparables")
	}
	if res.CxPVencido != "936930724.00" {
		t.Errorf("no informó la cartera vencida: %q", res.CxPVencido)
	}
	if res.CxPPorVencer != "61630009.00" {
		t.Errorf("no informó lo que está por vencer: %q", res.CxPPorVencer)
	}
}

// Sin estacionalidad confiable, todos los meses salen iguales Y SE DICE. Si no, una curva plana
// se leería como «el negocio no tiene estacionalidad», que es otra afirmación.
func TestSinEstacionalidadLoDice(t *testing.T) {
	salud, serie := docePeriodos("1000", "GASTO")
	svc := servicioConFlujo(
		map[string][]SaludMes{"2026-01..2026-12": salud},
		map[string][]TendenciaPartida{"2026-01..2026-12": {partidaCon("c1", "Planilla", "GASTO", serie)}},
		"0", "0",
	)
	res, err := svc.ProyectarFlujo(context.Background(), "e1", "2026-01", "2026-12", 3)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	// Doce meses sin repetir ninguno: no hay mes con dos años.
	if res.ConEstacionalidad {
		t.Error("dijo tener estacionalidad con un solo año de datos")
	}
	if !strings.Contains(res.Aviso, "estacionalidad") {
		t.Errorf("no avisó de que no corrige por estacionalidad: %q", res.Aviso)
	}
	for _, m := range res.Meses {
		if m.IndiceEstacional != "" {
			t.Errorf("%s declaró un índice sin respaldo: %q", m.Periodo, m.IndiceEstacional)
		}
	}
}

// Con dos años, el mes caro sale más caro en la proyección: para eso sirve la estacionalidad.
func TestConDosAniosLaProyeccionCorrigePorElMes(t *testing.T) {
	salud := []SaludMes{}
	serie := map[string]string{}
	// Dos años de enero y de julio; julio gasta el doble.
	for _, a := range []string{"2025", "2026"} {
		for _, m := range []string{"01", "07"} {
			p := a + "-" + m
			salud = append(salud, mesSano(p))
			if m == "07" {
				serie[p] = "2000"
			} else {
				serie[p] = "1000"
			}
		}
	}
	ordenarSalud(salud)

	svc := servicioConFlujo(
		map[string][]SaludMes{"2025-01..2026-07": salud},
		map[string][]TendenciaPartida{"2025-01..2026-07": {partidaCon("c1", "Planilla", "GASTO", serie)}},
		"0", "0",
	)
	res, err := svc.ProyectarFlujo(context.Background(), "e1", "2025-01", "2026-07", 6)
	if err != nil {
		t.Fatalf("proyectar: %v", err)
	}
	if !res.ConEstacionalidad {
		t.Fatal("con dos eneros y dos julios tendría que haber estacionalidad")
	}
	// Agosto a diciembre no tienen índice; enero de 2027 sí.
	var enero *MesProyectado
	for i := range res.Meses {
		if res.Meses[i].Periodo == "2027-01" {
			enero = &res.Meses[i]
		}
	}
	if enero == nil {
		t.Fatalf("no se proyectó enero 2027: %+v", res.Meses)
	}
	if enero.IndiceEstacional == "" {
		t.Error("enero tiene dos años y no se le aplicó el índice")
	}
}

func TestSumarMeses(t *testing.T) {
	casos := map[string]string{
		"2026-09": "2026-10",
		"2026-12": "2027-01",
		"2026-01": "2026-02",
	}
	for in, quiero := range casos {
		if got := sumarMeses(in, 1); got != quiero {
			t.Errorf("sumarMeses(%q,1) = %q, se esperaba %q", in, got, quiero)
		}
	}
	if got := sumarMeses("2026-12", 13); got != "2028-01" {
		t.Errorf("sumarMeses con más de un año: %q", got)
	}
}

func TestRitmoMensualRepartElAnual(t *testing.T) {
	// Una partida con mediana cero —casi nunca aparece— reparte su total sobre el rango.
	p := TendenciaPartida{Clasificacion: "Aguinaldos", Mediana: "0", Total: "1200000", MesesConGasto: 1}
	r, err := ritmoMensual(p, 12)
	if err != nil {
		t.Fatalf("ritmo: %v", err)
	}
	if !r.Equal(decimal.NewFromInt(100000)) {
		t.Errorf("ritmo %s, se esperaba 100000", r)
	}
	// Sin meses con gasto no hay ritmo que calcular.
	vacia := TendenciaPartida{Mediana: "0", Total: "0", MesesConGasto: 0}
	if r, _ := ritmoMensual(vacia, 12); !r.IsZero() {
		t.Errorf("una partida sin gasto dio ritmo %s", r)
	}
}
