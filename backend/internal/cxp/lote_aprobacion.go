package cxp

// Aprobar el corte antes de que salga al banco (mig 0123).
//
// ── POR QUÉ ESTE PASO EXISTE ─────────────────────────────────────────────────────────────────
//
// Medido antes de construirlo: la macro del banco se bajaba de un lote ABIERTO, sin que nadie lo
// aprobara. El archivo que mueve el dinero no tenía puerta. Y si la gerencia financiera rechazaba
// una factura del listado, no había forma de sacarla: había que reabrir y rehacer el corte entero.
//
// Es el «payment proposal» de las organizaciones grandes, con sus tres reglas:
//
//   1. LA PROPUESTA ES EDITABLE; LA FACTURA NO SE CASTIGA. Excluir una línea del corte no la
//      deniega: vuelve a la bandeja con su aprobación y su validación de área intactas, lista
//      para el siguiente. Denegar es otra cosa, la hace otro permiso y tiene otro estado.
//   2. LA APROBACIÓN DEJA AUTOR, FECHA Y, SI HUBO, MOTIVO. No es un acuerdo verbal ni un correo.
//   3. EL ARCHIVO BANCARIO SE GENERA DESPUÉS DE APROBAR. Es el control que evita que lo que se
//      sube al banco difiera de lo que la gerencia vio.
//
// ── POR QUÉ DEVOLVER ES DISTINTO DE EXCLUIR ──────────────────────────────────────────────────
//
// Son dos respuestas distintas de la gerencia y confundirlas cuesta caro:
//
//   · EXCLUIR saca UNA factura y deja el resto del corte vivo. Es lo normal: «esta no, todavía».
//   · DEVOLVER rechaza el corte ENTERO y lo manda de vuelta a tesorería. Es para cuando lo que
//     está mal es el corte —la fecha, el banco de origen, la mitad de las líneas—, y arreglarlo
//     factura por factura sería más lento que rearmarlo.
//
// Un solo verbo para las dos cosas obligaría a excluir veinte facturas de a una para rechazar un
// corte, o a tumbar el corte entero para sacar una.

import (
	"errors"
	"strings"
)

// Los estados del corte. ABIERTO y CERRADO ya existían; los dos del medio son de la mig 0123.
const (
	// LoteAbierto: tesorería lo está armando.
	LoteAbierto = "ABIERTO"
	// LoteEnAprobacion: se mandó a la gerencia financiera y está esperando su decisión.
	LoteEnAprobacion = "EN_APROBACION"
	// LoteAprobado: la gerencia lo aprobó. RECIÉN ACÁ se habilita la macro del banco.
	LoteAprobado = "APROBADO"
	// LoteCerrado: ya se pagó.
	LoteCerrado = "CERRADO"
)

var (
	// ErrLoteNoEstaAbierto: se quiso mandar a aprobar algo que no está abierto.
	ErrLoteNoEstaAbierto = errors.New("cxp: solo se manda a aprobar un corte abierto")

	// ErrLoteNoEstaEnAprobacion: se quiso aprobar o devolver un corte que nadie mandó a aprobar.
	// Aprobar lo que no se pidió dejaría una firma sobre un corte que la gerencia nunca vio.
	ErrLoteNoEstaEnAprobacion = errors.New("cxp: ese corte no está esperando aprobación")

	// ErrLoteSinAprobar: se pidió la macro de un corte que todavía no se aprobó. Es LA guarda de
	// la mig 0123: el archivo que mueve el dinero no sale antes que la decisión.
	ErrLoteSinAprobar = errors.New("cxp: el archivo del banco sale cuando el corte está aprobado; este todavía no lo está")

	// ErrMotivoDevolucionRequerido: devolver sin decir por qué. Quien recibe el corte de vuelta
	// tiene que saber qué arreglar, o lo manda igual otra vez y se pierde el viaje.
	ErrMotivoDevolucionRequerido = errors.New("cxp: para devolver un corte hay que decir qué hay que corregir")

	// ErrMotivoExclusionRequerido: sacar una factura sin decir por qué. La factura vuelve a la
	// bandeja y alguien la va a volver a cortar la semana entrante; sin el motivo, la vuelve a
	// meter en el mismo corte y la gerencia la vuelve a sacar.
	ErrMotivoExclusionRequerido = errors.New("cxp: para sacar una factura del corte hay que decir por qué")

	// ErrFacturaNoEstaEnElLote: se quiso excluir una factura que no está en ese corte.
	ErrFacturaNoEstaEnElLote = errors.New("cxp: esa factura no está en este corte")

	// ErrLoteNoSeTocaAprobado: se quiso sacar una factura de un corte ya aprobado. El contenido
	// aprobado es lo que la gerencia firmó; cambiarlo después haría que la firma describa otro
	// conjunto. Se devuelve el corte —con su motivo— y recién entonces se mueve.
	ErrLoteNoSeTocaAprobado = errors.New("cxp: ese corte ya está aprobado; para sacarle una factura hay que devolverlo primero")
)

// motivoDevolucionMinimo y motivoExclusionMinimo usan el MISMO mínimo que reabrir y liberar.
//
// Veinte caracteres no garantizan una explicación, pero descartan la respuesta refleja —«ok»,
// «no», «ya»— que deja el expediente sin nada que leer. Se reusa el valor a propósito: tres
// mínimos distintos para la misma clase de huella son tres reglas que alguien tiene que recordar.
const (
	motivoDevolucionMinimo = motivoLiberacionMinimo
	motivoExclusionMinimo  = motivoLiberacionMinimo
)

// MotivoDevolucionValido exige un motivo con sustancia.
func MotivoDevolucionValido(m string) bool {
	return len([]rune(strings.TrimSpace(m))) >= motivoDevolucionMinimo
}

// MotivoExclusionValido exige un motivo con sustancia.
func MotivoExclusionValido(m string) bool {
	return len([]rune(strings.TrimSpace(m))) >= motivoExclusionMinimo
}

// FacturaExcluida es una factura que salió de un corte y volvió a la bandeja.
//
// Se devuelve como lista propia —y no solo como columnas de la factura— porque la pregunta que
// contesta es del CORTE: «¿qué sacó la gerencia y por qué?». Es lo que va en el cronograma
// aprobado para poder compararlo después contra lo que se pagó.
type FacturaExcluida struct {
	DocumentoID string `json:"documento_id"`
	Consecutivo string `json:"consecutivo"`
	Proveedor   string `json:"proveedor"`
	MontoCRC    string `json:"monto_crc"`
	Motivo      string `json:"motivo"`
	ExcluidoPor string `json:"excluido_por"`
	ExcluidoEn  string `json:"excluido_en"`
	LoteNumero  int64  `json:"lote_numero"`
}
