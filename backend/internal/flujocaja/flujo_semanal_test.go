package flujocaja

// La matemática del flujo a 13 semanas.
//
// Lo que se prueba acá es que el SALDO se arrastre bien y que la represa se desagote al ritmo
// declarado. Un error en cualquiera de las dos cosas no se ve: la curva sigue siendo una curva
// plausible, y alguien decide sobre ella.

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func fdec(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}

// Un miércoles, para que el lunes de la semana 1 sea anterior a «hoy» y se vea que el horizonte
// arranca en el lunes y no en la fecha de consulta.
var miercoles = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)

func politicaFija(tope, minimo string) PoliticaCaja {
	return PoliticaCaja{
		Modo: TopeMontoFijo, TopeSemanal: tope, TopePct: "0",
		MinimoOperativo: minimo, Moneda: "CRC", Definida: true,
	}
}

// EL SALDO SE ARRASTRA: el final de una semana es el inicial de la siguiente. Es la invariante
// del reporte —si se rompe, cada fila es correcta por separado y la curva entera es falsa—.
func TestElSaldoSeArrastraDeSemanaEnSemana(t *testing.T) {
	movs := []MovimientoEsperado{
		{Fecha: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), Monto: fdec("1000000"),
			Origen: OrigenCxCCargo, Capa: CapaComprometido, Entrada: true},
		{Fecha: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), Monto: fdec("400000"),
			Origen: OrigenPlanilla, Capa: CapaComprometido, Entrada: false},
	}
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("500000"), SaldoDeclarado, "2026-11-03",
		movs, decimal.Zero, politicaFija("0", "0"))

	if len(f.Semanas) != SemanasHorizonte {
		t.Fatalf("vinieron %d semanas, se esperaban %d", len(f.Semanas), SemanasHorizonte)
	}
	// La semana 1 arranca el LUNES 2026-11-02, no el miércoles de la consulta.
	if f.Semanas[0].Desde != "2026-11-02" || f.Semanas[0].Hasta != "2026-11-08" {
		t.Errorf("semana 1: %s a %s", f.Semanas[0].Desde, f.Semanas[0].Hasta)
	}
	// s1: 500.000 + 1.000.000 = 1.500.000
	if f.Semanas[0].SaldoFinal != "1500000.00" {
		t.Errorf("saldo s1 %q", f.Semanas[0].SaldoFinal)
	}
	// s2: 1.500.000 − 400.000 = 1.100.000
	if f.Semanas[1].SaldoInicial != "1500000.00" {
		t.Errorf("el saldo no se arrastró: inicial de s2 %q", f.Semanas[1].SaldoInicial)
	}
	if f.Semanas[1].SaldoFinal != "1100000.00" {
		t.Errorf("saldo s2 %q", f.Semanas[1].SaldoFinal)
	}
	// Y de ahí en adelante, sin movimientos, se mantiene.
	for i := 2; i < SemanasHorizonte; i++ {
		if f.Semanas[i].SaldoFinal != "1100000.00" {
			t.Fatalf("semana %d cambió sin movimientos: %q", i+1, f.Semanas[i].SaldoFinal)
		}
	}
	if f.SaldoFinalHorizonte != "1100000.00" {
		t.Errorf("saldo del horizonte %q", f.SaldoFinalHorizonte)
	}
}

// LA REPRESA SE DESAGOTA AL TOPE, NO DE GOLPE. Es la razón de ser del diseño: sin esto, los
// ₡936 millones vencidos caen en la semana 1 y el reporte deja de servir.
func TestLaRepresaSeDesagotaAlRitmoDeLaPolitica(t *testing.T) {
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("10000000"), SaldoDeclarado, "2026-11-03",
		nil, fdec("9000000"), politicaFija("500000", "0"))

	// 500.000 por semana × 13 = 6.500.000 desagotados; quedan 2.500.000.
	if f.RepresaDesagotada != "6500000.00" {
		t.Errorf("desagotado %q, se esperaba 6500000.00", f.RepresaDesagotada)
	}
	if f.RepresaRemanente != "2500000.00" {
		t.Errorf("remanente %q, se esperaba 2500000.00", f.RepresaRemanente)
	}
	// Cada semana sale exactamente el tope, no más.
	if f.Semanas[0].SalidasRepresa != "500000.00" {
		t.Errorf("la semana 1 pagó %q de atrasos", f.Semanas[0].SalidasRepresa)
	}
	// Y el saldo baja parejo: 10.000.000 − 500.000 × 13 = 3.500.000.
	if f.SaldoFinalHorizonte != "3500000.00" {
		t.Errorf("saldo final %q", f.SaldoFinalHorizonte)
	}
	// 9.000.000 al ritmo de 500.000/semana = 18 semanas.
	if f.SemanasParaVaciar != 18 {
		t.Errorf("semanas para vaciar %d, se esperaban 18", f.SemanasParaVaciar)
	}
}

// La represa no paga de más cuando queda menos que el tope: pagar 500.000 de una deuda de
// 120.000 sacaría plata del banco que nadie debe.
func TestLaRepresaNoPagaMasDeLoQueQueda(t *testing.T) {
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("5000000"), SaldoDeclarado, "2026-11-03",
		nil, fdec("1200000"), politicaFija("500000", "0"))

	if f.RepresaDesagotada != "1200000.00" || f.RepresaRemanente != "0.00" {
		t.Errorf("desagotado %q, remanente %q", f.RepresaDesagotada, f.RepresaRemanente)
	}
	// s1 y s2 pagan 500.000; s3 paga los 200.000 que quedan; s4 en adelante, nada.
	if f.Semanas[2].SalidasRepresa != "200000.00" {
		t.Errorf("s3 pagó %q, se esperaba 200000.00", f.Semanas[2].SalidasRepresa)
	}
	if f.Semanas[3].SalidasRepresa != "0.00" {
		t.Errorf("s4 pagó %q sin deuda pendiente", f.Semanas[3].SalidasRepresa)
	}
	if f.SaldoFinalHorizonte != "3800000.00" {
		t.Errorf("saldo final %q, se esperaba 3800000.00", f.SaldoFinalHorizonte)
	}
}

// SIN POLÍTICA DECLARADA NO SE PAGA NADA, y «semanas para vaciar» dice NUNCA (-1).
//
// Devolver 0 se leería como «ya está pagada», que es exactamente lo contrario de lo que pasa.
func TestSinPoliticaLaRepresaNoBajaYSeDiceAsi(t *testing.T) {
	sinPol := PoliticaCaja{Moneda: "CRC", Definida: false}
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("1000000"), SaldoSinDatos, "2026-11-03",
		nil, fdec("900000000"), sinPol)

	if f.RepresaDesagotada != "0.00" {
		t.Errorf("pagó %q sin política declarada", f.RepresaDesagotada)
	}
	if f.SemanasParaVaciar != -1 {
		t.Errorf("semanas para vaciar %d, se esperaba -1 (nunca)", f.SemanasParaVaciar)
	}
	if f.SaldoFinalHorizonte != "1000000.00" {
		t.Errorf("el saldo se movió sin movimientos: %q", f.SaldoFinalHorizonte)
	}
}

// EL MODO POR % DE INGRESOS se calcula sobre lo que entra ESA semana, no sobre un promedio: es
// como se desagota de verdad cuando la entrada es irregular.
func TestElTopePorPorcentajeSigueALoQueEntra(t *testing.T) {
	movs := []MovimientoEsperado{
		{Fecha: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC), Monto: fdec("2000000"),
			Origen: OrigenCxCCuota, Capa: CapaEsperado, Entrada: true}, // s1
		{Fecha: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), Monto: fdec("500000"),
			Origen: OrigenCxCCuota, Capa: CapaEsperado, Entrada: true}, // s2
	}
	pol := PoliticaCaja{
		Modo: TopePctIngresos, TopeSemanal: "0", TopePct: "30",
		MinimoOperativo: "0", Moneda: "CRC", Definida: true,
	}
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("0"), SaldoDeclarado, "2026-11-03",
		movs, fdec("10000000"), pol)

	// s1: 30 % de 2.000.000 = 600.000
	if f.Semanas[0].SalidasRepresa != "600000.00" {
		t.Errorf("s1 pagó %q, se esperaba 600000.00", f.Semanas[0].SalidasRepresa)
	}
	// s2: 30 % de 500.000 = 150.000 — sigue a la entrada, no al promedio
	if f.Semanas[1].SalidasRepresa != "150000.00" {
		t.Errorf("s2 pagó %q, se esperaba 150000.00", f.Semanas[1].SalidasRepresa)
	}
	// s3 en adelante: sin entradas, no se paga nada.
	if f.Semanas[2].SalidasRepresa != "0.00" {
		t.Errorf("s3 pagó %q sin entradas", f.Semanas[2].SalidasRepresa)
	}
}

// LAS DOS ALARMAS SON DISTINTAS: bajar del mínimo operativo es un aviso, quedar en negativo es
// un sobregiro. Mezclarlas haría que la primera no se mire.
func TestDistingueBajoMinimoDeEnRojo(t *testing.T) {
	movs := []MovimientoEsperado{
		{Fecha: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), Monto: fdec("700000"),
			Origen: OrigenPlanilla, Capa: CapaComprometido, Entrada: false}, // s2
		{Fecha: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC), Monto: fdec("500000"),
			Origen: OrigenPlanilla, Capa: CapaComprometido, Entrada: false}, // s3
	}
	// Saldo 1.000.000, mínimo operativo 500.000.
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("1000000"), SaldoDeclarado, "2026-11-03",
		movs, decimal.Zero, politicaFija("0", "500000"))

	// s2 cierra en 300.000: bajo el mínimo, pero no en rojo.
	if !f.Semanas[1].BajoMinimo || f.Semanas[1].EnRojo {
		t.Errorf("s2: bajo_minimo=%v en_rojo=%v, saldo %q",
			f.Semanas[1].BajoMinimo, f.Semanas[1].EnRojo, f.Semanas[1].SaldoFinal)
	}
	// s3 cierra en −200.000: las dos cosas.
	if !f.Semanas[2].BajoMinimo || !f.Semanas[2].EnRojo {
		t.Errorf("s3: bajo_minimo=%v en_rojo=%v, saldo %q",
			f.Semanas[2].BajoMinimo, f.Semanas[2].EnRojo, f.Semanas[2].SaldoFinal)
	}
	if f.PrimeraSemanaBajoMinimo != 2 {
		t.Errorf("primera bajo mínimo: %d, se esperaba 2", f.PrimeraSemanaBajoMinimo)
	}
	if f.PrimeraSemanaEnRojo != 3 {
		t.Errorf("primera en rojo: %d, se esperaba 3", f.PrimeraSemanaEnRojo)
	}
	// El punto más bajo es el de s3 y hay que poder nombrarlo sin recorrer la tabla.
	if f.PuntoMasBajo != "-200000.00" || f.SemanaPuntoMasBajo != 3 {
		t.Errorf("punto más bajo %q en la semana %d", f.PuntoMasBajo, f.SemanaPuntoMasBajo)
	}
}

// UNA SALIDA CON FECHA YA VENCIDA NO SE PIERDE: se suma a la represa.
//
// Descartarla por «caer fuera del horizonte» borraría la cartera atrasada del reporte sin decir
// nada, que es el peor resultado posible: un flujo optimista y silencioso.
func TestLoVencidoCaeEnLaRepresaYNoSePierde(t *testing.T) {
	movs := []MovimientoEsperado{
		{Fecha: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), Monto: fdec("3000000"),
			Origen: OrigenCxPProgramada, Capa: CapaComprometido, Entrada: false},
	}
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("5000000"), SaldoDeclarado, "2026-11-03",
		movs, fdec("1000000"), politicaFija("250000", "0"))

	// La represa son 1.000.000 declarados + 3.000.000 que ya vencieron.
	if f.RepresaTotal != "4000000.00" {
		t.Errorf("represa total %q, se esperaba 4000000.00", f.RepresaTotal)
	}
	// Y ninguna semana carga los 3.000.000 de golpe.
	for _, s := range f.Semanas {
		if s.SalidasRepresa != "250000.00" {
			t.Fatalf("semana %d pagó %q, se esperaba el tope de 250000.00", s.Numero, s.SalidasRepresa)
		}
	}
}

// EL PUNTO MÁS BAJO ES UNA SEMANA DEL HORIZONTE, nunca «la semana 0».
//
// Medirlo arrancando desde el saldo de hoy devolvía 0 cuando ninguna semana bajaba de él, y la
// pantalla terminaba diciendo «el punto más bajo es la semana 0», que no existe en la tabla.
func TestElPuntoMasBajoSiempreCaeEnUnaSemanaReal(t *testing.T) {
	t.Run("sin movimientos, es la semana 1", func(t *testing.T) {
		f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("4000000"), SaldoDeclarado, "2026-11-03",
			nil, decimal.Zero, politicaFija("0", "0"))
		if f.SemanaPuntoMasBajo != 1 {
			t.Errorf("semana del punto más bajo %d, se esperaba 1", f.SemanaPuntoMasBajo)
		}
		if f.PuntoMasBajo != "4000000.00" {
			t.Errorf("punto más bajo %q", f.PuntoMasBajo)
		}
	})

	t.Run("si el saldo sube todo el tiempo, sigue siendo la semana 1", func(t *testing.T) {
		movs := []MovimientoEsperado{
			{Fecha: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC), Monto: fdec("100000"),
				Origen: OrigenCxCCuota, Capa: CapaEsperado, Entrada: true},
			{Fecha: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), Monto: fdec("100000"),
				Origen: OrigenCxCCuota, Capa: CapaEsperado, Entrada: true},
		}
		f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("1000000"), SaldoDeclarado, "2026-11-03",
			movs, decimal.Zero, politicaFija("0", "0"))
		if f.SemanaPuntoMasBajo != 1 || f.PuntoMasBajo != "1100000.00" {
			t.Errorf("punto más bajo %q en la semana %d; se esperaba 1100000.00 en la 1",
				f.PuntoMasBajo, f.SemanaPuntoMasBajo)
		}
	})
}

// Una entrada posterior al horizonte no entra: estas son trece semanas, no el año.
func TestLoQueCaeDespuesDelHorizonteNoEntra(t *testing.T) {
	movs := []MovimientoEsperado{
		{Fecha: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC), Monto: fdec("9000000"),
			Origen: OrigenCxCCuota, Capa: CapaEsperado, Entrada: true},
	}
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("100000"), SaldoDeclarado, "2026-11-03",
		movs, decimal.Zero, politicaFija("0", "0"))
	if f.SaldoFinalHorizonte != "100000.00" {
		t.Errorf("entró plata de fuera del horizonte: %q", f.SaldoFinalHorizonte)
	}
}

// Las capas se reportan SEPARADAS. Si el total fuera lo único visible, no se sabría cuál de las
// tres mover para que una semana deje de estar en rojo —que es para lo que existe el reporte—.
func TestLasTresCapasSeReportanPorSeparado(t *testing.T) {
	movs := []MovimientoEsperado{
		{Fecha: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC), Monto: fdec("300000"),
			Origen: OrigenCxCCargo, Capa: CapaComprometido, Entrada: true},
		{Fecha: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), Monto: fdec("700000"),
			Origen: OrigenCxCCuota, Capa: CapaEsperado, Entrada: true},
		{Fecha: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC), Monto: fdec("200000"),
			Origen: OrigenPlanilla, Capa: CapaComprometido, Entrada: false},
	}
	f := ConstruirFlujoSemanal(miercoles, "CRC", fdec("0"), SaldoDeclarado, "2026-11-03",
		movs, fdec("5000000"), politicaFija("100000", "0"))

	s := f.Semanas[0]
	if s.EntradasComprometidas != "300000.00" || s.EntradasEsperadas != "700000.00" {
		t.Errorf("entradas: comprometidas %q, esperadas %q",
			s.EntradasComprometidas, s.EntradasEsperadas)
	}
	if s.Entradas != "1000000.00" {
		t.Errorf("entradas totales %q", s.Entradas)
	}
	if s.SalidasComprometidas != "200000.00" || s.SalidasRepresa != "100000.00" {
		t.Errorf("salidas: comprometidas %q, represa %q",
			s.SalidasComprometidas, s.SalidasRepresa)
	}
	if s.Salidas != "300000.00" || s.Neto != "700000.00" {
		t.Errorf("salidas %q, neto %q", s.Salidas, s.Neto)
	}

	// Y el desglose por origen permite abrir la semana sin otra consulta, en orden estable.
	if len(s.PorOrigen) != 4 {
		t.Fatalf("se esperaban 4 orígenes, vinieron %d: %+v", len(s.PorOrigen), s.PorOrigen)
	}
	if s.PorOrigen[0].Origen != OrigenCxCCargo || !s.PorOrigen[0].Entrada {
		t.Errorf("el primer origen debería ser el cargo cobrado: %+v", s.PorOrigen[0])
	}
	if s.PorOrigen[3].Origen != OrigenCxPAtraso || s.PorOrigen[3].Capa != CapaRepresa {
		t.Errorf("el último debería ser el atraso: %+v", s.PorOrigen[3])
	}
}

// El lunes de una semana que arranca en domingo: el domingo pertenece a la semana que TERMINA,
// no a la que empieza. Con Weekday()==0 para domingo, un cálculo ingenuo lo manda siete días
// adelante y toda la grilla queda corrida.
func TestElDomingoPerteneceALaSemanaQueTermina(t *testing.T) {
	domingo := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	if got := lunesDe(domingo).Format("2006-01-02"); got != "2026-11-02" {
		t.Errorf("el lunes del domingo 2026-11-08 dio %s, se esperaba 2026-11-02", got)
	}
	lunes := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	if got := lunesDe(lunes).Format("2006-01-02"); got != "2026-11-02" {
		t.Errorf("el lunes de un lunes dio %s", got)
	}
}
