package cxp

import (
	"errors"

	"github.com/shopspring/decimal"
)

// Errores del netting de anticipos (se mapean a HTTP en el handler).
var (
	ErrNoEsAnticipo            = errors.New("cxp: el documento seleccionado no es un anticipo")
	ErrAnticipoNoPagado        = errors.New("cxp: el anticipo debe estar pagado para poder aplicarse")
	ErrProveedorDistinto       = errors.New("cxp: el anticipo y la factura deben ser del mismo proveedor")
	ErrMonedaNoNeteable        = errors.New("cxp: el neteo de anticipos solo está disponible en colones (CRC)")
	ErrFacturaNoNeteable       = errors.New("cxp: los anticipos se aplican antes de aprobar la factura")
	ErrMontoAplicacionInvalido = errors.New("cxp: el monto excede el saldo del anticipo o el neto de la factura")
	ErrReversaNoPermitida      = errors.New("cxp: no se puede reversar: la factura ya fue pagada")
	ErrAplicacionNoEncontrada  = errors.New("cxp: aplicación de anticipo no encontrada")
)

// AnticipoSaldo es un anticipo pagado del proveedor con su saldo disponible (billetera).
// ProveedorID/Proveedor solo se llenan en la vista de toda la empresa (AnticiposEmpresa).
type AnticipoSaldo struct {
	ID          string `json:"id"`
	Consecutivo string `json:"consecutivo"`
	FechaPago   string `json:"fecha_pago"`
	TotalCRC    string `json:"total_crc"`
	Aplicado    string `json:"aplicado"`
	Saldo       string `json:"saldo"`
	ProveedorID string `json:"proveedor_id,omitempty"`
	Proveedor   string `json:"proveedor,omitempty"`
	// Estado del documento anticipo. Solo PAGADO/CONCILIADO son aplicables a una factura;
	// los demás se muestran en la billetera como "en trámite" (para que no parezcan perdidos).
	Estado string `json:"estado,omitempty"`
}

// AplicacionInput es una línea a aplicar: qué anticipo y por cuánto.
type AplicacionInput struct {
	AnticipoID string
	Monto      decimal.Decimal
}

// AplicacionAnticipo es un anticipo aplicado (activo) a una factura.
type AplicacionAnticipo struct {
	ID                  string `json:"id"`
	AnticipoID          string `json:"anticipo_id"`
	AnticipoConsecutivo string `json:"anticipo_consecutivo"`
	MontoCRC            string `json:"monto_crc"`
	AplicadoPorNombre   string `json:"aplicado_por_nombre"`
	AplicadoEn          string `json:"aplicado_en"`
}
