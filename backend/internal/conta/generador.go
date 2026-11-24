package conta

// FASE 3B · El generador de asientos desde el movimiento bancario.
//
// ── LA DECISIÓN QUE LO HABILITA ─────────────────────────────────────────────────────────────
//
// El Director Financiero la tomó el 28-oct-2026, con estas palabras: «nace cuando llega la
// factura, pero se materializa en bancos cuando se paga. Actualmente la lógica de la empresa es
// cuando se materializa porque el flujo se maneja bancario. No es cuando debo sino cuánto pagué,
// y no es cuánto me deben sino cuánto ingresó».
//
// Eso es contabilidad POR CAJA, y es lo que este generador implementa.
//
// ── LO QUE ESTO NO ES, DICHO SIN VUELTAS ────────────────────────────────────────────────────
//
// Por caja NO cumple NIIF. Un estado de resultados armado así no es comparable con el de Softland
// ni sirve para un due diligence: registra cuándo se movió la plata, no cuándo se generó la
// obligación. El destino declarado es devengo, y el diseño lo deja abierto —`origen_tipo` acepta
// CXP_FACTURA desde el primer día y la idempotencia es por documento— pero lo que se genere por
// caja no se reexpresa solo. La fecha de corte de esa migración es la decisión D3 de la propuesta.
//
// ── LA REGLA, QUE NO SE INVENTA ─────────────────────────────────────────────────────────────
//
// La dirección del asiento la da el SIGNO del movimiento, que ya está en el dato:
//
//   · DÉBITO  (salió plata del banco):  Debe = cuenta de la partida · Haber = cuenta del banco
//   · CRÉDITO (entró plata al banco):   Debe = cuenta del banco     · Haber = cuenta de la partida
//
// No hay criterio contable que elegir acá: es la partida doble aplicada a un hecho ya registrado.
// Lo que sí es criterio —a qué cuenta va cada partida— lo decidió el mapeo de la Fase 2.

import (
	"errors"
	"strings"
)

// Motivos por los que un movimiento NO se puede contabilizar. Cada uno nombra qué falta y dónde
// se arregla: una lista de «no se pudo» sin el porqué deja a alguien revisando 20.000 filas.
const (
	// FaltaClasificacion: el movimiento no tiene partida. Se arregla en Bancos › Clasificar.
	FaltaClasificacion = "SIN_CLASIFICAR"
	// FaltaMapeo: la partida existe pero nadie dijo a qué cuenta va. Se arregla en el Mapeo.
	FaltaMapeo = "SIN_MAPEO"
	// NoContabilizable: se decidió que esa partida no produce asiento. No es un error.
	NoContabilizable = "NO_CONTABILIZABLE"
	// FaltaCuentaBanco: la cuenta bancaria no tiene cuenta contable. Se arregla en Cuentas bancarias.
	FaltaCuentaBanco = "SIN_CUENTA_BANCO"
	// FaltaTipoCambio: un movimiento en dólares sin tipo de cambio del mes vale cero en colones,
	// y un asiento en cero no es un asiento.
	FaltaTipoCambio = "SIN_TIPO_CAMBIO"
	// MontoCero: ni débito ni crédito. No hay nada que registrar.
	MontoCero = "MONTO_CERO"
)

// MovimientoAContabilizar es una fila del banco lista para convertirse en asiento, o la razón por
// la que no puede.
type MovimientoAContabilizar struct {
	MovimientoID string `json:"movimiento_id"`
	Fecha        string `json:"fecha"`
	Descripcion  string `json:"descripcion"`
	Documento    string `json:"documento"`
	// MontoCRC es el monto en colones. Siempre positivo: la dirección la dice EsDebito.
	MontoCRC string `json:"monto_crc"`
	EsDebito bool   `json:"es_debito"`

	Partida        string  `json:"partida"`
	CuentaPartida  *string `json:"cuenta_partida"`
	CuentaBanco    *string `json:"cuenta_banco"`
	CuentaBancaria string  `json:"cuenta_bancaria"`

	// Motivo vacío = se puede contabilizar.
	Motivo string `json:"motivo"`
}

// SePuede dice si el movimiento tiene todo lo que hace falta.
func (m MovimientoAContabilizar) SePuede() bool { return m.Motivo == "" }

// ResumenGeneracion es lo que se va a hacer, antes de hacerlo.
//
// EXISTE PORQUE GENERAR ES MASIVO. Un período de Valle de Paz son miles de movimientos: ver
// primero qué entra y qué queda fuera es la diferencia entre decidir y enterarse.
type ResumenGeneracion struct {
	Periodo string `json:"periodo"`
	// Movimientos vivos del período.
	Total int `json:"total"`
	// Contabilizables: los que tienen todo lo que hace falta, ya tengan asiento o no.
	Contabilizables int    `json:"contabilizables"`
	MontoDebitos    string `json:"monto_debitos"`
	MontoCreditos   string `json:"monto_creditos"`
	// Nuevos son los contabilizables que TODAVÍA NO tienen asiento: es lo que de verdad se va a
	// escribir. Va aparte de Contabilizables porque en un período ya generado los dos números se
	// separan, y un botón que promete «Generar 6.340» para después crear cero es una mentira
	// aunque ninguna de las dos cifras esté mal.
	Nuevos int `json:"nuevos"`
	// PorMotivo cuenta lo que queda fuera, agrupado por la razón. El orden de la pantalla sale
	// de acá: lo que más movimientos bloquea es lo primero que conviene arreglar.
	PorMotivo []MotivoExcluido `json:"por_motivo"`
	// YaGenerados: movimientos de este período que ya tienen asiento vivo. Volver a generar no
	// los duplica —lo impide la base— pero saber cuántos son evita el susto.
	YaGenerados int `json:"ya_generados"`
	// Aviso explica en una frase qué hay que hacer antes de generar.
	Aviso string `json:"aviso"`
}

// MotivoExcluido es cuántos movimientos bloquea una causa, y por cuánta plata.
type MotivoExcluido struct {
	Motivo         string `json:"motivo"`
	Etiqueta       string `json:"etiqueta"`
	DondeSeArregla string `json:"donde_se_arregla"`
	Movimientos    int    `json:"movimientos"`
	MontoCRC       string `json:"monto_crc"`
}

// ResultadoGeneracion es lo que pasó al generar.
type ResultadoGeneracion struct {
	Periodo  string `json:"periodo"`
	Creados  int    `json:"creados"`
	Omitidos int    `json:"omitidos"`
	// YaExistian son los que ya tenían asiento: la segunda corrida de un período los cuenta acá
	// en vez de fallar. Es lo que hace que regenerar sea seguro.
	YaExistian int `json:"ya_existian"`
	// Desde y Hasta son los números de asiento asignados, para poder encontrarlos en el diario.
	DesdeNumero int64 `json:"desde_numero"`
	HastaNumero int64 `json:"hasta_numero"`
}

// ResultadoReversa es lo que pasó al deshacer una generación.
type ResultadoReversa struct {
	Periodo  string `json:"periodo"`
	Anulados int    `json:"anulados"`
}

var (
	// ErrPeriodoInvalido: el período no tiene la forma YYYY-MM.
	ErrPeriodoInvalido = errors.New("conta: el período tiene que venir como 2026-10")
	// ErrNadaQueGenerar: ningún movimiento del período se puede contabilizar todavía.
	ErrNadaQueGenerar = errors.New("conta: ningún movimiento de ese período se puede contabilizar todavía")
	// ErrMotivoReversaRequerido: anular una generación entera queda escrito.
	ErrMotivoReversaRequerido = errors.New("conta: decí por qué se revierte la generación de este período")
)

// etiquetaMotivo pone en palabras qué falta y dónde se arregla.
//
// Las dos cosas juntas y no solo la primera: «1.843 sin mapeo» informa; «1.843 sin mapeo · se
// arregla en Contabilidad › Mapeo contable» se puede accionar sin preguntarle a nadie.
func etiquetaMotivo(m string) (string, string) {
	switch m {
	case FaltaClasificacion:
		return "Sin partida asignada", "Bancos › Clasificar"
	case FaltaMapeo:
		return "La partida no tiene cuenta contable", "Contabilidad › Mapeo contable"
	case NoContabilizable:
		return "Declarada no contabilizable", "—"
	case FaltaCuentaBanco:
		return "La cuenta bancaria no tiene cuenta contable", "Contabilidad › Cuentas bancarias"
	case FaltaTipoCambio:
		return "En dólares, sin tipo de cambio del mes", "Bancos › Tipo de cambio"
	case MontoCero:
		return "Sin monto", "—"
	default:
		return m, "—"
	}
}

// esPeriodo comprueba la forma «YYYY-MM» sin construir una fecha.
func esPeriodo(s string) bool {
	if len(s) != 7 || s[4] != '-' {
		return false
	}
	for i, r := range s {
		if i == 4 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	mes := int(s[5]-'0')*10 + int(s[6]-'0')
	return mes >= 1 && mes <= 12
}

// glosaDe arma el texto que se va a leer en el diario dentro de seis meses.
//
// Lleva la partida Y la descripción del banco: la partida sola no distingue dos pagos del mismo
// rubro, y la descripción sola es un texto del banco que a veces no dice nada («TRANSF 00123»).
func glosaDe(partida, descripcion string) string {
	partida = strings.TrimSpace(partida)
	descripcion = strings.TrimSpace(descripcion)
	switch {
	case partida != "" && descripcion != "":
		return partida + " · " + descripcion
	case partida != "":
		return partida
	case descripcion != "":
		return descripcion
	default:
		// La base exige glosa no vacía. Un movimiento sin partida ni descripción no debería
		// llegar hasta acá, pero si llega, el asiento tiene que poder existir y decir qué es.
		return "Movimiento bancario"
	}
}
