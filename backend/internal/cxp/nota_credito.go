package cxp

// La nota de crédito del proveedor (mig 0106, hallazgo 3.6). El porqué del modelo está en la
// migración. Acá viven las reglas.
//
// ── LO QUE LA NOTA DE CRÉDITO ES, Y LO QUE NO ──────────────────────────────────────────────
//
// Es un comprobante electrónico que el PROVEEDOR emite para rebajar lo que le debemos: un precio
// mal facturado, una devolución, un descuento acordado después. Llega con su clave de Hacienda,
// igual que una factura, y por eso vive en `documento_cxp` con tipo NOTA_CREDITO.
//
// NO es un anticipo. El anticipo es plata que ya le entregamos —por eso exige estado PAGADO y
// tiene un saldo que se consume—; la nota de crédito no se paga nunca, solo reduce. Comparten la
// forma de la aplicación y nada más, y por eso son dos tablas.
//
// NO es un pago. El banco no debita nada cuando llega una nota de crédito: lo que cambia es
// cuánto hay que pagar después.
//
// ── POR QUÉ NO SE APLICA DESPUÉS DE APROBAR ────────────────────────────────────────────────
//
// Misma ventana que el anticipo: solo antes de aprobar. Lo que se aprueba es el NETO, y es la
// cifra con la que se arma el archivo de pago. Dejar que el neto cambie después de la firma
// convierte la aprobación en una formalidad sobre un número que ya no existe — y si cambia
// después de que la macro se subió al banco, el lote deja de cuadrar con lo que se pagó.
//
// Si la nota llega tarde, el camino es el que ya existe: se registra lo que el banco debitó de
// menos como pago parcial (mig 0105), que para eso se construyó.

import (
	"errors"

	"github.com/shopspring/decimal"
)

// TipoNotaCredito es el tipo del documento en `documento_cxp`.
const TipoNotaCredito = "NOTA_CREDITO"

var (
	// ErrNoEsNotaCredito: el documento que se quiso aplicar no es una nota de crédito.
	ErrNoEsNotaCredito = errors.New("cxp: ese documento no es una nota de crédito")
	// ErrNotaSobreNota: se intentó rebajar una nota de crédito con otra.
	ErrNotaSobreNota = errors.New("cxp: una nota de crédito no se aplica sobre otra nota de crédito")
	// ErrNotaProveedorDistinto: la nota y la factura son de proveedores distintos. Una nota de
	// crédito rebaja lo que le debemos a QUIEN la emitió; cruzarlas le pagaría de menos a uno y
	// de más al otro.
	ErrNotaProveedorDistinto = errors.New("cxp: la nota de crédito y la factura son de proveedores distintos")
	// ErrNotaMonedaDistinta: no se convierte entre monedas, por lo mismo que en los anticipos.
	ErrNotaMonedaDistinta = errors.New("cxp: la nota de crédito y la factura tienen que estar en la misma moneda; no se convierte sola porque con qué tipo de cambio es una decisión de negocio")
	// ErrNotaFacturaNoNeteable: la factura ya pasó el punto donde el neto se puede mover.
	ErrNotaFacturaNoNeteable = errors.New("cxp: esa factura ya está aprobada; si la nota llegó después, registrá como pago parcial lo que el banco debitó de menos")
	// ErrNotaMontoInvalido: el monto no es positivo, o no es un número.
	ErrNotaMontoInvalido = errors.New("cxp: el monto de la nota de crédito tiene que ser mayor que cero")
	// ErrNotaExcedeFactura: lo aplicado superaría lo que la factura todavía debe.
	ErrNotaExcedeFactura = errors.New("cxp: ese monto supera lo que la factura todavía debe")
	// ErrNotaExcedeSuSaldo: se quiso aplicar más de lo que la nota vale.
	ErrNotaExcedeSuSaldo = errors.New("cxp: esa nota de crédito no tiene tanto sin aplicar")
)

// NotaCreditoSaldo es una nota con cuánto le queda sin aplicar.
type NotaCreditoSaldo struct {
	ID          string `json:"id"`
	Clave       string `json:"clave"`
	Consecutivo string `json:"consecutivo"`
	Proveedor   string `json:"proveedor"`
	ProveedorID string `json:"proveedor_id"`
	Fecha       string `json:"fecha_emision"`
	Moneda      string `json:"moneda"`
	Total       string `json:"total"`
	Aplicado    string `json:"aplicado"`
	Saldo       string `json:"saldo"`
}

// AplicacionNotaCredito es una rebaja viva sobre una factura.
type AplicacionNotaCredito struct {
	ID          string `json:"id"`
	NotaID      string `json:"nota_id"`
	FacturaID   string `json:"factura_id"`
	Monto       string `json:"monto"`
	Consecutivo string `json:"consecutivo"`
	Clave       string `json:"clave"`
	AplicadoEn  string `json:"aplicado_en"`
}

// estadosNeteables son los estados donde el neto todavía se puede mover: antes de aprobar.
//
// Es la MISMA lista que usa el anticipo, a propósito. Dos ventanas distintas para dos cosas que
// cambian el mismo número serían dos reglas que alguien tiene que recordar, y la segunda se
// olvida.
var estadosNeteables = []string{EstRecibido, EstRevisado, EstValidadoDepto}

func esNeteable(estado string) bool {
	for _, e := range estadosNeteables {
		if e == estado {
			return true
		}
	}
	return false
}

// montoNotaValido convierte y valida en un solo lugar.
func montoNotaValido(s string) (decimal.Decimal, error) {
	m, err := decimal.NewFromString(s)
	if err != nil || m.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrNotaMontoInvalido
	}
	return m, nil
}
