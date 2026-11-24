package cxp

// El calendario de pagos: dónde cae cada fecha y cómo se agrupa lo vencido (16-nov-2026).
//
// LAS SEMANAS SON LA MITAD DEL PEDIDO —«para este, el viernes que sigue o el otro»— y es
// aritmética de calendario, que es donde se cuelan los errores que nadie ve: el domingo contado
// como primer día, el viernes de hoy contado como «la próxima», el borde del mes. Todo eso se
// prueba acá, sin base, con fechas fijas.
//
// Y LO VENCIDO SE AGRUPA POR LO QUE FALTA HACER, no por antigüedad. Medido: de ₡998 M vencidos,
// solo ₡3,9 M se pueden pagar; ₡984 M están en recepción sin revisar. Un bloque que los sumara
// todos diría «hay que pagar ₡998 M», que no es cierto y esconde lo único accionable.

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func dia(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// UbicarFecha, con un LUNES como hoy (16-nov-2026 es lunes).
func TestLasSemanasSeCuentanDeLunesADomingo(t *testing.T) {
	hoy := dia("2026-11-16") // lunes
	for _, c := range []struct {
		fecha, cuando string
		atrasado      bool
	}{
		{"2026-11-13", "atrasado", true},           // el viernes pasado
		{"2026-11-15", "atrasado", true},           // ayer, domingo
		{"2026-11-16", "esta semana", false},       // hoy
		{"2026-11-20", "esta semana", false},       // EL VIERNES QUE SIGUE
		{"2026-11-22", "esta semana", false},       // domingo: cierra la semana
		{"2026-11-23", "la próxima semana", false}, // lunes siguiente
		{"2026-11-27", "la próxima semana", false}, // EL OTRO VIERNES
		{"2026-11-29", "la próxima semana", false},
		{"2026-11-30", "más adelante", false},
		{"2026-12-25", "más adelante", false},
	} {
		cuando, atrasado := UbicarFecha(dia(c.fecha), hoy)
		if cuando != c.cuando || atrasado != c.atrasado {
			t.Errorf("%s (lunes 16-nov) → %q/%v; se esperaba %q/%v",
				c.fecha, cuando, atrasado, c.cuando, c.atrasado)
		}
	}
}

// El mismo cálculo visto desde un VIERNES: el día en que de verdad se paga.
func TestDesdeUnViernesElMismoDiaEsEstaSemana(t *testing.T) {
	hoy := dia("2026-11-20") // viernes
	if cuando, atrasado := UbicarFecha(hoy, hoy); cuando != "esta semana" || atrasado {
		t.Errorf("el propio viernes → %q/%v; pagar hoy no es «atrasado»", cuando, atrasado)
	}
	if cuando, _ := UbicarFecha(dia("2026-11-22"), hoy); cuando != "esta semana" {
		t.Errorf("el domingo de la misma semana → %q", cuando)
	}
	if cuando, _ := UbicarFecha(dia("2026-11-27"), hoy); cuando != "la próxima semana" {
		t.Errorf("el viernes siguiente → %q; es «la próxima», no «más adelante»", cuando)
	}
}

// Un DOMINGO no puede abrir una semana nueva: en Go es el día 0 y sin corregirlo, «esta semana»
// se reduciría al propio domingo y el lunes caería en «la próxima».
func TestElDomingoCierraLaSemanaNoLaAbre(t *testing.T) {
	hoy := dia("2026-11-22") // domingo
	if cuando, _ := UbicarFecha(hoy, hoy); cuando != "esta semana" {
		t.Errorf("el propio domingo → %q", cuando)
	}
	if cuando, _ := UbicarFecha(dia("2026-11-27"), hoy); cuando != "la próxima semana" {
		t.Errorf("el viernes que viene, visto un domingo → %q; se esperaba «la próxima semana»", cuando)
	}
}

func TestElDiaSaleEnEspanol(t *testing.T) {
	if d := NombreDelDia(dia("2026-11-20")); d != "viernes" {
		t.Errorf("20-nov-2026 = %q, se esperaba «viernes»", d)
	}
	if d := NombreDelDia(dia("2026-11-16")); d != "lunes" {
		t.Errorf("16-nov-2026 = %q, se esperaba «lunes»", d)
	}
}

// ── El armado del calendario, con un doble que devuelve los datos reales medidos ────────────

type repoCalendario struct {
	Repository
	prog []FilaProgramada
	venc []FilaVencida
}

func (r *repoCalendario) ProgramadoPorFecha(context.Context, string) ([]FilaProgramada, error) {
	return r.prog, nil
}
func (r *repoCalendario) VencidoPorEstado(context.Context, string) ([]FilaVencida, error) {
	return r.venc, nil
}

// LO VENCIDO SE SEPARA POR LO QUE FALTA HACER, y «listo para pagar» sale aparte.
func TestElVencidoDistingueLoPagableDeLoTrabado(t *testing.T) {
	// Escenario CONSTRUIDO, no la foto real: acá entran las PROGRAMADAS al bloque de vencido para
	// comprobar que se funden con las aprobadas. En la base no pasa —las programadas tienen fecha
	// de pago y por eso viven en el otro bloque—, y es a propósito: contarlas en los dos haría
	// que los totales no sumen la cartera. Medido el 16-nov-2026, lo «listo para pagar» real son
	// ₡209.050 (3 aprobadas sin fecha) de ₡994,8 M vencidos: el 0,02 %.
	repo := &repoCalendario{venc: []FilaVencida{
		{Estado: "RECIBIDO", Facturas: 4301, TotalCRC: "984212798.00"},
		{Estado: "REVISADO", Facturas: 118, TotalCRC: "7893301.00"},
		{Estado: "VALIDADO_DEPTO", Facturas: 3, TotalCRC: "2597335.00"},
		{Estado: "APROBADO", Facturas: 3, TotalCRC: "209050.00"},
		{Estado: "PROGRAMADO", Facturas: 38, TotalCRC: "3648249.00"},
	}}
	svc := NewService(repo, nil, zap.NewNop())

	cal, err := svc.CalendarioDePagos(context.Background(), "e1", dia("2026-11-16"))
	if err != nil {
		t.Fatalf("calendario: %v", err)
	}
	if len(cal.Vencido) != 4 {
		t.Fatalf("grupos = %d, se esperaban 4 (listo, falta aprobar, falta validar, sin revisar)", len(cal.Vencido))
	}

	// El orden es el del flujo: lo que se puede pagar hoy, primero.
	if cal.Vencido[0].Situacion != "listo para pagar" {
		t.Errorf("el primer grupo es %q; lo pagable va arriba", cal.Vencido[0].Situacion)
	}
	// APROBADO + PROGRAMADO se funden: para quien mira son lo mismo.
	if cal.Vencido[0].Facturas != 41 {
		t.Errorf("«listo para pagar» = %d facturas, se esperaban 41 (3 aprobadas + 38 programadas)",
			cal.Vencido[0].Facturas)
	}
	if cal.Vencido[0].TotalCRC != "3857299.00" {
		t.Errorf("«listo para pagar» = %s, se esperaba 3857299.00", cal.Vencido[0].TotalCRC)
	}
	if cal.ListoParaPagar != "3857299.00" {
		t.Errorf("ListoParaPagar = %s; es EL número del bloque y va aparte", cal.ListoParaPagar)
	}

	// Y el total NO se confunde con lo pagable: es la diferencia que hace útil la pantalla.
	if cal.TotalVencido != "998560733.00" {
		t.Errorf("total vencido = %s, se esperaba 998560733.00", cal.TotalVencido)
	}
	if cal.TotalVencido == cal.ListoParaPagar {
		t.Error("el total vencido y lo pagable salieron iguales: la pantalla diría que hay " +
			"₡998 M por pagar cuando solo ₡3,9 M se pueden pagar")
	}

	// El grupo más grande dice QUÉ HACER, no solo en qué estado está.
	ultimo := cal.Vencido[len(cal.Vencido)-1]
	if ultimo.QueFalta != "falta revisarlas" {
		t.Errorf("el grupo de recepción dice %q; tiene que decir la acción", ultimo.QueFalta)
	}
}

// Las fechas programadas se ubican y se suman.
func TestElProgramadoUbicaCadaFechaYSuma(t *testing.T) {
	repo := &repoCalendario{prog: []FilaProgramada{
		{Fecha: "2026-11-13", Facturas: 6, TotalCRC: "412189.00"}, // el viernes pasado
		{Fecha: "2026-11-20", Facturas: 4, TotalCRC: "100000.00"}, // este viernes
		{Fecha: "2026-11-27", Facturas: 2, TotalCRC: "50000.00"},  // el otro
	}}
	svc := NewService(repo, nil, zap.NewNop())

	cal, err := svc.CalendarioDePagos(context.Background(), "e1", dia("2026-11-16"))
	if err != nil {
		t.Fatalf("calendario: %v", err)
	}
	if len(cal.Programado) != 3 {
		t.Fatalf("tramos = %d", len(cal.Programado))
	}
	if !cal.Programado[0].Atrasado || cal.Programado[0].Cuando != "atrasado" {
		t.Errorf("el viernes pasado salió como %q/%v; no se pagó y ya venció",
			cal.Programado[0].Cuando, cal.Programado[0].Atrasado)
	}
	if cal.Programado[1].Cuando != "esta semana" || cal.Programado[1].Dia != "viernes" {
		t.Errorf("el 20-nov salió como %q/%q", cal.Programado[1].Cuando, cal.Programado[1].Dia)
	}
	if cal.Programado[2].Cuando != "la próxima semana" {
		t.Errorf("el 27-nov salió como %q", cal.Programado[2].Cuando)
	}
	if cal.TotalProgramado != "562189.00" {
		t.Errorf("total programado = %s, se esperaba 562189.00", cal.TotalProgramado)
	}
}

// Sin nada que mostrar, las listas van VACÍAS y no nulas: el cliente no tiene que distinguir
// «no hay» de «no vino».
func TestElCalendarioVacioNoDevuelveNulos(t *testing.T) {
	svc := NewService(&repoCalendario{}, nil, zap.NewNop())
	cal, err := svc.CalendarioDePagos(context.Background(), "e1", dia("2026-11-16"))
	if err != nil {
		t.Fatalf("calendario: %v", err)
	}
	if cal.Programado == nil || cal.Vencido == nil {
		t.Error("las listas vinieron nulas")
	}
	if cal.ListoParaPagar != "0.00" || cal.TotalVencido != "0.00" {
		t.Errorf("los totales vacíos son %q y %q", cal.ListoParaPagar, cal.TotalVencido)
	}
}
