package cxp

import (
	"errors"
	"testing"
)

// EL DÍA 31 EN UN MES QUE NO LO TIENE.
//
// Varios proveedores cobran a fin de mes, así que «vence el 31» es un acuerdo real. Pero febrero no
// tiene 31, y si esto se dejara en manos de time.Date el 31 de febrero se normalizaría al 3 de
// marzo: la obligación de FEBRERO aparecería venciendo en MARZO, febrero nunca tendría su fila, y
// el olvido de febrero quedaría invisible. Exactamente lo contrario de para lo que existe esto.
func TestFechaDeVencimientoNuncaSeSaleDelMes(t *testing.T) {
	casos := []struct {
		periodo string
		dia     int
		quiero  string
		porque  string
	}{
		{"2026-01", 31, "2026-01-31", "enero sí tiene 31"},
		{"2026-02", 31, "2026-02-28", "febrero de año común: cae al 28, NO al 3 de marzo"},
		{"2024-02", 31, "2024-02-29", "febrero bisiesto: cae al 29"},
		{"2026-02", 30, "2026-02-28", "el 30 tampoco existe en febrero"},
		{"2026-04", 31, "2026-04-30", "abril tiene 30"},
		{"2026-12", 31, "2026-12-31", "diciembre 31 no puede saltar al año siguiente"},
		{"2026-12", 1, "2026-12-01", "el 1 de diciembre es el 1 de diciembre"},
		{"2026-09", 15, "2026-09-15", "un día normal en un mes normal"},
		{"2026-09", 0, "2026-09-01", "día 0 no existe: se trata como el 1"},
	}
	for _, c := range casos {
		got, err := FechaDeVencimiento(c.periodo, c.dia)
		if err != nil {
			t.Errorf("FechaDeVencimiento(%q, %d) devolvió error: %v", c.periodo, c.dia, err)
			continue
		}
		if s := got.Format("2006-01-02"); s != c.quiero {
			t.Errorf("FechaDeVencimiento(%q, %d) = %s, quería %s — %s", c.periodo, c.dia, s, c.quiero, c.porque)
		}
		// La invariante que de verdad importa: el vencimiento NUNCA se sale de su propio mes.
		if got.Format("2006-01") != c.periodo {
			t.Errorf("FechaDeVencimiento(%q, %d) se fue al mes %s", c.periodo, c.dia, got.Format("2006-01"))
		}
	}
}

func TestFechaDeVencimientoRechazaPeriodoConBasura(t *testing.T) {
	for _, p := range []string{"", "2026", "2026-13", "setiembre", "2026-09-01", "26-09"} {
		if _, err := FechaDeVencimiento(p, 1); !errors.Is(err, ErrPeriodoInvalido) {
			t.Errorf("FechaDeVencimiento(%q) debía rechazar el período, devolvió %v", p, err)
		}
	}
}

// UNA TRIMESTRAL NO CAE TODOS LOS MESES.
//
// Si esto se equivocara, «Abrir el mes» crearía obligaciones que no corresponden y alguien tendría
// que ir marcando «no aplica» una por una — hasta que deje de mirar la pantalla.
func TestAplicaEnPeriodo(t *testing.T) {
	casos := []struct {
		periodicidad string
		ancla        int
		periodo      string
		quiero       bool
		porque       string
	}{
		{"MENSUAL", 0, "2026-01", true, "la mensual cae siempre, sin importar el ancla"},
		{"MENSUAL", 0, "2026-07", true, "la mensual cae siempre"},
		{"TRIMESTRAL", 3, "2026-03", true, "marzo es el ancla"},
		{"TRIMESTRAL", 3, "2026-06", true, "junio: tres meses después"},
		{"TRIMESTRAL", 3, "2026-09", true, "setiembre"},
		{"TRIMESTRAL", 3, "2026-12", true, "diciembre"},
		{"TRIMESTRAL", 3, "2026-04", false, "abril NO"},
		{"TRIMESTRAL", 3, "2026-01", false, "enero NO"},
		// La trampa del cruce de año: enero (1) menos noviembre (11) da -10, y un módulo negativo
		// en Go daría la respuesta equivocada.
		{"TRIMESTRAL", 11, "2026-02", true, "ancla en noviembre: febrero SÍ cae (cruza el año)"},
		{"TRIMESTRAL", 11, "2026-01", false, "ancla en noviembre: enero no"},
		{"BIMENSUAL", 1, "2026-03", true, "bimensual desde enero: marzo"},
		{"BIMENSUAL", 1, "2026-04", false, "bimensual desde enero: abril no"},
		{"SEMESTRAL", 6, "2026-12", true, "semestral desde junio: diciembre"},
		{"SEMESTRAL", 6, "2026-09", false, "semestral desde junio: setiembre no"},
		{"ANUAL", 5, "2026-05", true, "la anual cae en su mes"},
		{"ANUAL", 5, "2026-06", false, "la anual NO cae en ningún otro"},
	}
	for _, c := range casos {
		got, err := AplicaEnPeriodo(c.periodicidad, c.ancla, c.periodo)
		if err != nil {
			t.Errorf("AplicaEnPeriodo(%s, %d, %s) devolvió error: %v", c.periodicidad, c.ancla, c.periodo, err)
			continue
		}
		if got != c.quiero {
			t.Errorf("AplicaEnPeriodo(%s, ancla %d, %s) = %v, quería %v — %s",
				c.periodicidad, c.ancla, c.periodo, got, c.quiero, c.porque)
		}
	}
}

func TestAplicaEnPeriodoExigeAnclaCuandoNoEsMensual(t *testing.T) {
	if _, err := AplicaEnPeriodo("TRIMESTRAL", 0, "2026-03"); !errors.Is(err, ErrPeriodicidadInvalida) {
		t.Errorf("una trimestral sin ancla tiene que fallar, devolvió %v", err)
	}
	if _, err := AplicaEnPeriodo("QUINCENAL", 1, "2026-03"); !errors.Is(err, ErrPeriodicidadInvalida) {
		t.Errorf("una periodicidad inventada tiene que fallar, devolvió %v", err)
	}
}

// LA GUARDA DE HONESTIDAD: no decir VENCIDA cuando no se puede saber.
//
// Si el banco está importado solo hasta el 25 y el alquiler vencía el 1, el pago pudo haber salido
// por débito automático y todavía no lo sabemos. Un tablero que grita «VENCIDA» en falso se deja de
// mirar en la primera semana, y después ya no hay forma de recuperar su autoridad.
func TestSemaforoNoGritaVencidaSinDatosDelBanco(t *testing.T) {
	casos := []struct {
		nombre     string
		estado     string
		vence      string
		hoy        string
		bancoHasta string
		quiero     string
	}{
		{"cumplida manda sobre todo", PerCumplida, "2026-09-01", "2026-09-30", "", SemCumplida},
		{"no aplica manda sobre todo", PerNoAplica, "2026-09-01", "2026-09-30", "", SemNoAplica},
		{"venció y el banco alcanza", PerPendiente, "2026-09-01", "2026-09-30", "2026-09-30", SemVencida},
		{"venció y el banco llega justo al día", PerPendiente, "2026-09-01", "2026-09-30", "2026-09-01", SemVencida},
		{"venció pero el banco se quedó corto", PerPendiente, "2026-09-10", "2026-09-30", "2026-09-05", SemSinDato},
		{"venció y no hay banco del todo", PerPendiente, "2026-09-01", "2026-09-30", "", SemSinDato},
		{"vence dentro de la ventana", PerPendiente, "2026-09-20", "2026-09-15", "2026-09-15", SemPorVencer},
		{"vence justo en el borde de la ventana", PerPendiente, "2026-09-22", "2026-09-15", "2026-09-15", SemPorVencer},
		{"vence un día después de la ventana", PerPendiente, "2026-09-23", "2026-09-15", "2026-09-15", SemAlDia},
		{"vence hoy mismo todavía no está vencida", PerPendiente, "2026-09-15", "2026-09-15", "2026-09-15", SemPorVencer},
	}
	for _, c := range casos {
		if got := Semaforo(c.estado, c.vence, c.hoy, c.bancoHasta); got != c.quiero {
			t.Errorf("%s: Semaforo(%s, vence %s, hoy %s, banco %q) = %s, quería %s",
				c.nombre, c.estado, c.vence, c.hoy, c.bancoHasta, got, c.quiero)
		}
	}
}

// Una fecha ilegible no puede volverse VENCIDA por accidente: ante la duda, SIN DATO.
func TestSemaforoConFechasIlegiblesNoAcusa(t *testing.T) {
	for _, c := range [][3]string{
		{"", "2026-09-30", "2026-09-30"},
		{"2026-09-01", "", "2026-09-30"},
		{"basura", "2026-09-30", "2026-09-30"},
	} {
		if got := Semaforo(PerPendiente, c[0], c[1], c[2]); got != SemSinDato {
			t.Errorf("Semaforo con fecha ilegible %v = %s, quería %s", c, got, SemSinDato)
		}
	}
}

func TestDiasDeAtraso(t *testing.T) {
	casos := []struct {
		vence, hoy string
		quiero     int
	}{
		{"2026-09-01", "2026-09-11", 10},
		{"2026-09-11", "2026-09-11", 0},
		{"2026-09-20", "2026-09-11", -9},
		{"basura", "2026-09-11", 0},
	}
	for _, c := range casos {
		if got := DiasDeAtraso(c.vence, c.hoy); got != c.quiero {
			t.Errorf("DiasDeAtraso(%s, %s) = %d, quería %d", c.vence, c.hoy, got, c.quiero)
		}
	}
}

// LA REGLA FISCAL (decisión del Director, 13-set-2026).
// En Costa Rica un gasto sin comprobante no lo acepta Hacienda. Una pantalla que normalice
// «respaldo: verbal» sin decirlo estaría empujando a registrar gasto que después se cae.
func TestValidarRespaldo(t *testing.T) {
	casos := []struct {
		tipo      string
		archivo   string
		deducible bool
		quiero    error
		porque    string
	}{
		{"VERBAL", "", true, ErrSinComprobanteNoDeducible, "de palabra no puede ser deducible"},
		{"NINGUNO", "", true, ErrSinComprobanteNoDeducible, "sin respaldo tampoco"},
		{"VERBAL", "", false, nil, "de palabra y no deducible: correcto"},
		{"CONTRATO", "", true, ErrRespaldoSinArchivo, "si hay contrato, que esté adjunto"},
		{"ACTA", "", false, ErrRespaldoSinArchivo, "el acta también se adjunta"},
		{"CORREO", "hilo.eml", true, nil, "correo adjunto y deducible: correcto"},
		{"CONTRATO", "alquiler.pdf", true, nil, "contrato adjunto y deducible: correcto"},
	}
	for _, c := range casos {
		err := ValidarRespaldo(c.tipo, c.archivo, c.deducible)
		if !errors.Is(err, c.quiero) {
			t.Errorf("ValidarRespaldo(%s, %q, %v) = %v, quería %v — %s",
				c.tipo, c.archivo, c.deducible, err, c.quiero, c.porque)
		}
	}
}

// EL ENCABEZADO DEL MES no puede inflar la salida esperada con plata que nadie va a pagar.
func TestResumirMes(t *testing.T) {
	filas := []PeriodoResponsabilidad{
		{MontoEsperado: "1000000.00", Estado: PerPendiente, Semaforo: SemVencida},
		{MontoEsperado: "500000.00", Estado: PerPendiente, Semaforo: SemPorVencer},
		{MontoEsperado: "250000.00", Estado: PerPendiente, Semaforo: SemAlDia},
		{MontoEsperado: "125000.00", Estado: PerPendiente, Semaforo: SemSinDato},
		{MontoEsperado: "999000.00", Estado: PerCumplida, Semaforo: SemCumplida},
		// Ésta NO debe sumar al esperado: ya se declaró que este mes no aplicaba.
		{MontoEsperado: "777000.00", Estado: PerNoAplica, Semaforo: SemNoAplica},
	}
	r := ResumirMes("2026-09", filas, 3, "2026-09-30")

	if r.SinAbrir != 3 {
		t.Errorf("SinAbrir = %d, quería 3", r.SinAbrir)
	}
	if r.Vencida != 1 || r.PorVencer != 1 || r.SinDato != 1 || r.Cumplida != 1 || r.NoAplica != 1 {
		t.Errorf("conteos mal: %+v", r)
	}
	// Pendiente junta todo lo que sigue abierto: vencida + por vencer + al día + sin dato.
	if r.Pendiente != 4 {
		t.Errorf("Pendiente = %d, quería 4 (vencida + por vencer + al día + sin dato)", r.Pendiente)
	}
	// 1.000.000 + 500.000 + 250.000 + 125.000 + 999.000 = 2.874.000 (SIN los 777.000 del no aplica)
	if r.MontoEsperado != "2874000.00" {
		t.Errorf("MontoEsperado = %s, quería 2874000.00 — el «no aplica» no puede inflar la salida", r.MontoEsperado)
	}
	if r.MontoVencido != "1000000.00" {
		t.Errorf("MontoVencido = %s, quería 1000000.00", r.MontoVencido)
	}
	if r.BancoHasta != "2026-09-30" {
		t.Errorf("BancoHasta = %s", r.BancoHasta)
	}
}

// Un mes sin ninguna fila no puede devolver montos vacíos: la pantalla los formatea como dinero.
func TestResumirMesVacio(t *testing.T) {
	r := ResumirMes("2026-09", nil, 0, "")
	if r.MontoEsperado != "0.00" || r.MontoVencido != "0.00" {
		t.Errorf("un mes vacío tiene que dar 0.00, dio %q y %q", r.MontoEsperado, r.MontoVencido)
	}
}

// Un monto ilegible en la base no puede tumbar el encabezado entero.
func TestResumirMesTragaMontoIlegible(t *testing.T) {
	r := ResumirMes("2026-09", []PeriodoResponsabilidad{
		{MontoEsperado: "", Estado: PerPendiente, Semaforo: SemVencida},
		{MontoEsperado: "1000.00", Estado: PerPendiente, Semaforo: SemVencida},
	}, 0, "2026-09-30")
	if r.Vencida != 2 {
		t.Errorf("Vencida = %d, quería 2", r.Vencida)
	}
	if r.MontoEsperado != "1000.00" {
		t.Errorf("MontoEsperado = %s, quería 1000.00", r.MontoEsperado)
	}
}
