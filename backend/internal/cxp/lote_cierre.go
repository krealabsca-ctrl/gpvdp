package cxp

// Cerrar el lote de pago: el corte que no existía (mig 0104, hallazgo 3.3 de la auditoría).
//
// ── LO QUE SE MIDIÓ, Y LO QUE RESULTÓ SER DISTINTO ──────────────────────────────────────────
//
// La auditoría encontró 39 lotes, los 39 en ABIERTO, y ningún punto del código que escribiera
// 'CERRADO'. Eso es exacto. Pero le atribuyó una consecuencia que al ir a construir el cierre
// resultó falsa: decía que «un lote cuya macro ya se subió al banco puede seguir recibiendo
// facturas».
//
// No puede. `lote_id` tiene exactamente DOS escritores: `CrearLote`, que siempre INSERTA un lote
// nuevo y le asigna las facturas en la misma transacción, y `Reintentar`, que lo pone en NULL. No
// existe ningún camino para agregarle una factura a un lote que ya existe.
//
// El hueco real es el inverso, y por eso la guarda va donde va:
//
//   · Una factura puede SALIR de un lote ya pagado. `Reintentar` (REBOTADA → PROGRAMADO) le quita
//     el `lote_id` sin mirar el estado del lote. El lote queda describiendo un conjunto distinto
//     del que se le subió al banco, y la macro emitida deja de cuadrar con el reporte del lote.
//   · No hay marca de corte. Nada en el sistema dice «esta macro ya se subió, este corte está
//     terminado», así que tampoco hay un momento natural para preguntar si todo lo del lote tiene
//     ya su débito en el banco — que es el hallazgo 3.1.
//
// Por eso cerrar hace dos cosas: congela el contenido del lote y es el punto donde se pregunta
// por el respaldo bancario.
//
// ── POR QUÉ SE REABRE, Y POR QUÉ CON MOTIVO ─────────────────────────────────────────────────
//
// Decisión del Director Financiero (20-oct-2026). Un candado sin llave no es un control, es un
// defecto: ya pasó con `bloqueado_para_pago` (doce lectores, un escritor, la factura trancada
// hasta que la 0097 le puso llave) y con `incluido` antes de la 0093. Un lote cerrado por error y
// sin reapertura obligaría a armar otro, dejando huérfana la macro ya emitida.
//
// El motivo es obligatorio por lo mismo que en liberar y en cerrar por otro medio: reabrir es
// afirmar que un corte que alguien dio por terminado no lo estaba. Si dentro de seis meses el
// contenido de un lote no cuadra con la macro que se subió al banco, lo único que permite
// reconstruir qué pasó es quién lo reabrió, cuándo y con qué argumento.

import (
	"errors"
	"strings"
)

var (
	// ErrLoteYaCerrado: se pidió cerrar un lote que ya estaba cerrado. No es un error de datos
	// —el lote existe y está como se quería— pero tampoco es un éxito silencioso: quien lo pide
	// cree que está dando un corte que ya se dio, y confundir las dos cosas es lo que hace que
	// después nadie sepa cuál fue el cierre bueno.
	ErrLoteYaCerrado = errors.New("cxp: el lote ya estaba cerrado")

	// ErrLoteNoEstaCerrado: se pidió reabrir un lote abierto. Reabrir lo que nunca se cerró
	// dejaría un motivo de reapertura sobre un lote que nadie cerró, y esa huella miente.
	ErrLoteNoEstaCerrado = errors.New("cxp: el lote no está cerrado, así que no hay nada que reabrir")

	// ErrMotivoReaperturaRequerido: reabrir sin decir por qué. Ver el comentario de arriba.
	ErrMotivoReaperturaRequerido = errors.New("cxp: para reabrir un lote hay que decir por qué")

	// ErrLoteCerradoNoSeToca: se intentó sacar una factura de un lote cerrado. Es la guarda que
	// le da sentido al cierre: si el contenido puede cambiar después del corte, el corte no
	// existe. Se reabre el lote —con su motivo— y recién entonces se mueve la factura.
	ErrLoteCerradoNoSeToca = errors.New("cxp: ese lote está cerrado; para mover una factura hay que reabrirlo primero")
)

// motivoReaperturaMinimo es el largo mínimo del motivo, el MISMO que exige liberar una factura
// de contado.
//
// No es cosmético y no es un número elegido acá: veinte caracteres no garantizan una explicación,
// pero descartan la respuesta refleja —«ok», «si», «ya»— que deja el expediente sin nada que leer
// seis meses después, que es justo para lo que existe el campo. Se usa el mismo valor que
// `motivoLiberacionMinimo` a propósito: dos mínimos distintos para la misma clase de huella se
// vuelven dos reglas que alguien tiene que recordar.
const motivoReaperturaMinimo = motivoLiberacionMinimo

// MotivoReaperturaValido exige un motivo con sustancia.
func MotivoReaperturaValido(motivo string) bool {
	return len([]rune(strings.TrimSpace(motivo))) >= motivoReaperturaMinimo
}
