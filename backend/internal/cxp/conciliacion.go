package cxp

import (
	"regexp"
	"strings"
)

// reHuella reconoce la huella que CxP embebe en la descripción del pago (ver generarHuella).
//
// EL GUION ES OPCIONAL, y no por gusto. La macro emite «CXP-54A4281AE49A» —con guion, para que una
// persona pueda leerlo— pero el canal del banco lo borra: medido el 30 de setiembre de 2026 sobre
// un débito SINPE real de Valle de Paz, lo que vuelve en el estado de cuenta es
//
//	DEBITO MASIVO SINPE ( - 1060746795 Pagar Proveedores Nuevo FC 000487 CXP54A4281AE49A Lote 888607…
//
// Mientras el patrón exigió el guion, el barrido de conciliación no enganchó UN solo movimiento en
// toda la vida del sistema, y nadie se enteró: «ningún pago reconocido» se ve exactamente igual que
// «todavía no llegó el estado de cuenta». Es el filtro silencioso de siempre, esta vez de un
// carácter. Reconocer las dos formas es obligatorio; emitir con guion es solo cortesía.
var reHuella = regexp.MustCompile(`CXP-?([A-Z0-9]{12})`)

// reRemesa reconoce el código de una REMESA: un pago que cubre varias facturas del mismo proveedor.
//
// Mismo largo que el de factura (12) a propósito, y por eso el orden de lectura importa. Con el
// guion las dos formas son inconfundibles, pero el banco lo borra, así que hay que mirarlas sin él:
//
//	factura «CXP-R7K2M9P4XQ1B» → «CXPR7K2M9P4XQ1B» → como remesa daría CXPR + 11 → NO calza (pide 12)
//	remesa  «CXPR-7K2M9P4XQ1BT» → «CXPR7K2M9P4XQ1BT» → como factura daría CXP + R7K2M9P4XQ1B → SÍ calza
//
// O sea: una remesa puede leerse como factura, pero una factura nunca puede leerse como remesa. Por
// eso la remesa se prueba PRIMERO. Si se invirtiera, un pago agrupado iría a buscar una factura que
// no existe y el veredicto sería «sin documento»: el pago quedaría sin conciliar en silencio y las
// cinco facturas que cubre, abiertas. Hay una prueba que fija este orden.
var reRemesa = regexp.MustCompile(`CXPR-?([A-Z0-9]{12})`)

// extraerHuella busca el código de CxP dentro de la descripción de un movimiento bancario.
//
// Devuelve SIEMPRE la forma canónica («CXP-» o «CXPR-» + 12), que es como está guardada en
// `documento_cxp.huella` y `remesa_pago.huella`: quien la recibe la usa para buscar, no para
// mostrarla. El segundo valor dice si se encontró; para saber de cuál de los dos se trata está
// EsRemesa, porque el que concilia tiene que cerrar una factura o cinco según el caso.
func extraerHuella(descripcion string) (string, bool) {
	up := strings.ToUpper(descripcion)
	// La remesa primero: ver el comentario de reRemesa. No es preferencia, es corrección.
	if m := reRemesa.FindStringSubmatch(up); m != nil {
		return "CXPR-" + m[1], true
	}
	m := reHuella.FindStringSubmatch(up)
	if m == nil {
		return "", false
	}
	return "CXP-" + m[1], true
}

// EsRemesa dice si un código leído corresponde a un pago agrupado (varias facturas) y no a una
// factura suelta. Se pregunta por el código y no por el movimiento: es el código el que decide.
func EsRemesa(huella string) bool {
	return strings.HasPrefix(huella, "CXPR-")
}

// PagoRow es una línea del archivo de pago (formato SINPE: cédula, nombre, IBAN, monto neto).
type PagoRow struct {
	Cedula      string `json:"cedula"`
	Nombre      string `json:"nombre"`
	IBAN        string `json:"iban"`
	Moneda      string `json:"moneda"`
	MontoNeto   string `json:"monto_neto"`  // total − retención (lo que recibe el proveedor)
	Descripcion string `json:"descripcion"` // la huella, para conciliar contra el banco
	Consecutivo string `json:"consecutivo"`
	DocumentoID string `json:"documento_id"`
	// Referencia es la mitad LEGIBLE de la descripción cuando el pago no corresponde a una sola
	// factura: «REM 0043-07». Vacía en el pago por factura, donde esa mitad se deriva del
	// consecutivo («FC 000487»). Ver descripcionDePago.
	Referencia string `json:"referencia,omitempty"`
	// CodigoPago es la nomenclatura interna («A8», «A9»): sale del catálogo de gasto de la factura
	// y va PRIMERO en la descripción. Vacío cuando el gasto no tiene código, o cuando un pago
	// agrupado mezcla rubros con códigos distintos.
	CodigoPago string `json:"codigo_pago,omitempty"`
}
