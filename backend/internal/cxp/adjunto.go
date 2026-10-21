package cxp

import "errors"

// Los DOS documentos que puede llevar una factura (mig 0103).
//
// Validar un pago es cruzarlos: lo que el proveedor facturó contra lo que el banco debitó. Hasta
// la 0103 solo se podía guardar el segundo, y el primero no existía en ningún lado: las 4.542
// facturas entraron por Excel y `cxp_recepcion` —el buzón que conserva el XML— tenía 0 filas.
const (
	// AdjuntoComprobante es el recibo del banco: la prueba de que el dinero salió.
	AdjuntoComprobante = "COMPROBANTE"
	// AdjuntoFactura es el documento que emitió el proveedor.
	AdjuntoFactura = "FACTURA"
)

// TipoAdjuntoValido acepta el vacío como COMPROBANTE: es el que ya existía, y así las llamadas
// viejas —que no mandaban tipo— siguen significando lo mismo que antes.
func TipoAdjuntoValido(t string) (string, bool) {
	switch t {
	case "", AdjuntoComprobante:
		return AdjuntoComprobante, true
	case AdjuntoFactura:
		return AdjuntoFactura, true
	}
	return "", false
}

// EtiquetaAdjunto nombra el documento para los mensajes y la bitácora.
func EtiquetaAdjunto(t string) string {
	if t == AdjuntoFactura {
		return "factura"
	}
	return "comprobante"
}

// ErrTipoAdjuntoInvalido: el tipo pedido no es ninguno de los dos del expediente.
var ErrTipoAdjuntoInvalido = errors.New("cxp: el documento tiene que ser «COMPROBANTE» (el recibo del banco) o «FACTURA» (el del proveedor)")
