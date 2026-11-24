package cxp

// El calendario de pagos: qué se paga y cuándo, y qué está vencido esperando (16-nov-2026).
//
// EL PEDIDO (Director Financiero): «un calendario que me indique dos modalidades: uno que ya está
// aprobado para pagar y en qué fecha —así sea para este, el viernes que sigue o el otro— y qué
// está vencido que se necesita pagar».
//
// ── POR QUÉ NO ES UN CALENDARIO DE MES ───────────────────────────────────────────────────────
//
// Medido antes de construirlo: hay 38 facturas con fecha de pago en NUEVE fechas distintas, y
// 4.471 sin ninguna. Un calendario mensual mostraría una cuadrícula con dos o tres días pintados
// y el resto vacío, mientras el grueso del trabajo —lo vencido sin fecha— no tendría dónde
// aparecer. Son dos preguntas distintas y se contestan en dos bloques.
//
// ── EL HALLAZGO QUE DEFINE EL SEGUNDO BLOQUE ─────────────────────────────────────────────────
//
// «Vencido» no es lo mismo que «vencido y se puede pagar». Medido sobre Valle de Paz:
//
//	listo para pagar               41 facturas      ₡3.857.299
//	validado, falta aprobar         3 facturas      ₡2.597.335
//	revisado, falta validar       118 facturas      ₡7.893.301
//	en recepción, sin revisar   4.301 facturas    ₡984.212.798
//
// De ₡998 M vencidos, solo ₡3,9 M se pueden pagar hoy: el 98,6 % del dinero está trabado en
// recepción. Un bloque que dijera «vencido: ₡998 M» sería alarmante e inútil —nadie puede pagar
// eso— y escondería lo único accionable, que es QUÉ FALTA HACER con cada grupo. Por eso lo
// vencido se agrupa por SITUACIÓN EN EL FLUJO y no por antigüedad.
//
// ── LOS VIERNES ──────────────────────────────────────────────────────────────────────────────
//
// Siete de las nueve fechas de pago del sistema son viernes. No se codifica el viernes como regla
// —una empresa puede pagar otro día y el dato manda— pero sí se agrupa por SEMANA, que es como se
// decide: «esta», «la que viene», «más adelante».

import "time"

// TramoProgramado es un grupo de facturas que se pagan en la misma fecha.
type TramoProgramado struct {
	Fecha string `json:"fecha"`
	// Dia es el nombre del día en español. Va calculado acá y no en la pantalla: el servidor ya
	// sabe la fecha y el idioma del negocio es uno solo.
	Dia string `json:"dia"`
	// Cuando ubica la fecha respecto de hoy: «atrasado», «esta semana», «la próxima», «después».
	Cuando    string `json:"cuando"`
	Facturas  int    `json:"facturas"`
	TotalCRC  string `json:"total_crc"`
	Atrasado  bool   `json:"atrasado"`
	LoteValor int64  `json:"lote_numero,omitempty"`
}

// GrupoVencido es lo vencido que todavía no tiene fecha de pago, por lo que le falta.
type GrupoVencido struct {
	// Orden del flujo: 1 listo, 2 falta aprobar, 3 falta validar, 4 sin revisar.
	Orden int `json:"orden"`
	// Situacion se lee en pantalla: «listo para pagar», «falta aprobarlas»…
	Situacion string `json:"situacion"`
	// QueFalta dice la ACCIÓN, que es lo que convierte el bloque en trabajo y no en un diagnóstico.
	QueFalta string   `json:"que_falta"`
	Estados  []string `json:"estados"`
	Facturas int      `json:"facturas"`
	TotalCRC string   `json:"total_crc"`
}

// CalendarioPagos son las dos preguntas, contestadas por separado.
type CalendarioPagos struct {
	Hoy        string            `json:"hoy"`
	Programado []TramoProgramado `json:"programado"`
	Vencido    []GrupoVencido    `json:"vencido"`
	// Los totales de cada bloque, para no obligar a sumar en la pantalla.
	TotalProgramado string `json:"total_programado"`
	TotalVencido    string `json:"total_vencido"`
	// ListoParaPagar es el subtotal del grupo 1. Va aparte porque es EL número del bloque: de
	// todo lo vencido, es lo único que alguien puede pagar sin esperar a nadie.
	ListoParaPagar string `json:"listo_para_pagar"`
}

// diasEnEspanol traduce el día de la semana.
//
// Se traduce acá en vez de pedirle a PostgreSQL `to_char(..., 'Day')`: esa salida depende del
// `lc_time` del servidor, que en el contenedor es inglés, y hacer que el idioma de una pantalla
// dependa de la configuración regional de la base es una fuente de sorpresas.
var diasEnEspanol = map[time.Weekday]string{
	time.Sunday: "domingo", time.Monday: "lunes", time.Tuesday: "martes",
	time.Wednesday: "miércoles", time.Thursday: "jueves", time.Friday: "viernes",
	time.Saturday: "sábado",
}

// UbicarFecha dice en qué tramo cae una fecha de pago respecto de hoy.
//
// Las semanas se cuentan de LUNES a domingo, que es como se habla del trabajo: «esta semana»
// incluye el viernes que viene si hoy es lunes, y no el de dentro de ocho días.
func UbicarFecha(fecha, hoy time.Time) (cuando string, atrasado bool) {
	f := fecha.Truncate(24 * time.Hour)
	h := hoy.Truncate(24 * time.Hour)
	if f.Before(h) {
		return "atrasado", true
	}
	// Días hasta el domingo de esta semana (lunes = 1 … domingo = 7).
	diaSemana := int(h.Weekday())
	if diaSemana == 0 {
		diaSemana = 7
	}
	finDeEstaSemana := h.AddDate(0, 0, 7-diaSemana)
	if !f.After(finDeEstaSemana) {
		return "esta semana", false
	}
	if !f.After(finDeEstaSemana.AddDate(0, 0, 7)) {
		return "la próxima semana", false
	}
	return "más adelante", false
}

// NombreDelDia devuelve el día en español.
func NombreDelDia(t time.Time) string { return diasEnEspanol[t.Weekday()] }

// situacionDelVencido traduce el estado del flujo a lo que falta hacer.
//
// El texto NO es decorativo: es la diferencia entre una pantalla que informa y una que reparte
// trabajo. «4.301 en RECIBIDO» no le dice a nadie qué hacer; «falta revisarlas» sí.
func situacionDelVencido(estado string) (orden int, situacion, queFalta string) {
	switch estado {
	case EstAprobado, EstProgramado:
		return 1, "listo para pagar", "solo falta incluirlas en un corte"
	case EstValidadoDepto:
		return 2, "validado por el área", "falta aprobarlas"
	case EstRevisado:
		return 3, "revisado", "falta la validación del área"
	default:
		return 4, "en recepción", "falta revisarlas"
	}
}
