package cxp

import "errors"

var (
	// ErrComprobanteNoEncontrado indica que la factura no tiene comprobante adjunto.
	ErrComprobanteNoEncontrado = errors.New("cxp: la factura no tiene comprobante adjunto")
	// ErrDocNoPagado indica que el comprobante solo aplica a facturas pagadas/conciliadas.
	ErrDocNoPagado = errors.New("cxp: el comprobante solo aplica a facturas pagadas o conciliadas")
	// ErrProveedorSinEmail indica que el proveedor no tiene correo para enviarle el comprobante.
	ErrProveedorSinEmail = errors.New("cxp: el proveedor no tiene correo registrado")
)

// Comprobante es el archivo adjunto de pago (para descarga).
type Comprobante struct {
	Filename  string
	Mime      string
	Contenido []byte
}

// ComprobanteEnvio agrega los datos del proveedor y la factura para el correo.
type ComprobanteEnvio struct {
	Comprobante
	ProveedorEmail  string
	ProveedorNombre string
	Consecutivo     string
	TotalCRC        string
	// Datos que alimentan la plantilla del correo: el monto se informa en la MONEDA de la
	// factura (antes se decía «₡» aunque fuera en dólares).
	Moneda      string
	Total       string
	Huella      string
	Descripcion string
}
