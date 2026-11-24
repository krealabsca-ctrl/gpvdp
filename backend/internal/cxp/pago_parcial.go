package cxp

// El pago parcial: lo que el banco debitó cuando no alcanzó el neto (mig 0105, hallazgo 3.5).
//
// El porqué del modelo está en la migración. Acá viven las reglas.
//
// ── LO QUE ESTO NO ES ───────────────────────────────────────────────────────────────────────
//
// No es una tolerancia. La conciliación sigue exigiendo coincidencia EXACTA, y eso no cambia: en
// colones no hay diferencial que justifique aflojar, y una tolerancia produce emparejamientos
// «casi correctos» que nadie vuelve a mirar porque quedan conciliados.
//
// Lo que esto agrega es la SALIDA que faltaba. Antes, un débito que no calzaba dejaba el veredicto
// `MONTO_DIFERENTE`, que era terminal: ninguna pantalla lo resolvía. Ahora una persona puede decir
// «entró esto, y el resto se sigue debiendo», que es lo que de verdad pasó.
//
// ── POR QUÉ EL RESTO NO VUELVE SOLO A LA COLA DE PAGO ───────────────────────────────────────
//
// Decisión del Director Financiero (20-oct-2026). La diferencia tiene al menos tres causas con
// desenlaces opuestos: el banco rechazó una línea (hay que volver a cobrar), el proveedor emitió
// una nota de crédito (NO hay que cobrar nada), o faltó aplicar una retención (tampoco). Mandar el
// saldo al próximo corte automáticamente acierta en el primer caso y le paga de más al proveedor
// en los otros dos.

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	// ErrParcialMontoInvalido: el monto no es positivo, o no es un número.
	ErrParcialMontoInvalido = errors.New("cxp: el monto del pago parcial tiene que ser mayor que cero")
	// ErrParcialExcedeFactura: lo aplicado superaría el neto de la factura. Es la invariante 1:
	// permitirlo daría la factura por pagada de más.
	ErrParcialExcedeFactura = errors.New("cxp: ese monto supera lo que la factura todavía debe")
	// ErrParcialExcedeMovimiento: lo aplicado superaría el débito del banco. Es la invariante 2:
	// permitirlo usaría el mismo colón dos veces y el cuadre contra el banco dejaría de cerrar.
	ErrParcialExcedeMovimiento = errors.New("cxp: ese movimiento no tiene tanto sin aplicar")
	// ErrMovimientoYaAplicado: el movimiento ya apunta entero a una factura o a una remesa. Es la
	// invariante 3: son dos formas de decir lo mismo y hay que elegir una.
	ErrMovimientoYaAplicado = errors.New("cxp: ese movimiento ya está aplicado entero a una factura o a una remesa")
	// ErrParcialMotivoRequerido: sin motivo no se registra.
	ErrParcialMotivoRequerido = errors.New("cxp: decí por qué el banco debitó menos que el neto de la factura")
	// ErrParcialFacturaNoPagable: la factura no está en un estado donde un débito tenga sentido.
	ErrParcialFacturaNoPagable = errors.New("cxp: esa factura no está programada ni declarada pagada, así que no hay pago que registrar")
	// ErrSinSaldoQueCobrar: se pidió mandar a cobrar el resto de una factura que no debe nada.
	ErrSinSaldoQueCobrar = errors.New("cxp: esa factura no tiene saldo pendiente")
	// ErrMotivoCobroRequerido: sin motivo no se vuelve a cobrar.
	ErrMotivoCobroRequerido = errors.New("cxp: decí por qué el saldo se sigue debiendo")
)

// PagoParcial es una aplicación viva de un débito contra una factura.
type PagoParcial struct {
	ID           string `json:"id"`
	DocumentoID  string `json:"documento_id"`
	MovimientoID string `json:"movimiento_id"`
	Monto        string `json:"monto"`
	Moneda       string `json:"moneda"`
	Motivo       string `json:"motivo"`
	AplicadoEn   string `json:"aplicado_en"`
	// Del movimiento, para poder mostrar la línea sin otra consulta.
	Fecha       string `json:"fecha"`
	Descripcion string `json:"descripcion"`
	Cuenta      string `json:"cuenta"`
}

// SaldoDeFactura es cuánto lleva pagada una factura y cuánto le falta.
type SaldoDeFactura struct {
	DocumentoID string `json:"documento_id"`
	// Neto es lo que se debía ANTES de los parciales; Pagado la suma de ellos; Saldo lo que falta.
	// Los tres viajan juntos porque mostrar solo el saldo no deja verificar la resta.
	Neto      string        `json:"neto"`
	Pagado    string        `json:"pagado"`
	Saldo     string        `json:"saldo"`
	Moneda    string        `json:"moneda"`
	Parciales []PagoParcial `json:"parciales"`
}

// montoParcialValido convierte y valida el monto en un solo lugar.
func montoParcialValido(s string) (decimal.Decimal, error) {
	m, err := decimal.NewFromString(s)
	if err != nil || m.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrParcialMontoInvalido
	}
	return m, nil
}
