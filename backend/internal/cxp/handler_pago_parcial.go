package cxp

// Endpoints del pago parcial (mig 0105). El porqué está en pago_parcial.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

type pagoParcialRequest struct {
	MovimientoID string `json:"movimiento_id" validate:"required,uuid"`
	Monto        string `json:"monto" validate:"required"`
	Motivo       string `json:"motivo" validate:"required"`
}

// RegistrarPagoParcialHandler POST /v1/cxp/documentos/:id/pago-parcial
//
// Registra lo que el banco debitó cuando no alcanzó el neto. Devuelve el saldo con su detalle: los
// tres números —neto, pagado, saldo— viajan juntos para que quien los lea pueda verificar la resta.
func (h *Handler) RegistrarPagoParcialHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req pagoParcialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	saldo, err := h.svc.RegistrarPagoParcial(c.Request.Context(), empresaID,
		c.Param("id"), req.MovimientoID, req.Monto, req.Motivo, usuarioID)
	if err != nil {
		h.responderError(c, err, "registrar-pago-parcial")
		return
	}
	c.JSON(http.StatusOK, saldo)
}

// SaldoDeFacturaHandler GET /v1/cxp/documentos/:id/saldo
func (h *Handler) SaldoDeFacturaHandler(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	saldo, err := h.svc.SaldoDeFactura(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "saldo-de-factura")
		return
	}
	c.JSON(http.StatusOK, saldo)
}

type cobrarSaldoRequest struct {
	Motivo string `json:"motivo" validate:"required"`
}

// CobrarSaldoHandler POST /v1/cxp/documentos/:id/cobrar-saldo
//
// Devuelve la factura al circuito de pago por lo que todavía debe. No es automático a propósito:
// si la diferencia era una nota de crédito o una retención no aplicada, volver a cobrarla le paga
// de más al proveedor.
func (h *Handler) CobrarSaldoHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req cobrarSaldoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	doc, err := h.svc.MandarACobrarElSaldo(c.Request.Context(), empresaID, c.Param("id"), req.Motivo, usuarioID)
	if err != nil {
		h.responderError(c, err, "cobrar-saldo")
		return
	}
	c.JSON(http.StatusOK, doc)
}
