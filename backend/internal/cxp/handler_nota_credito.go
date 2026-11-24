package cxp

// Endpoints de la nota de crédito (mig 0106). El porqué está en nota_credito.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

type aplicarNotaRequest struct {
	NotaID string `json:"nota_id" validate:"required,uuid"`
	Monto  string `json:"monto" validate:"required"`
}

// AplicarNotaCreditoHandler POST /v1/cxp/documentos/:id/nota-credito
func (h *Handler) AplicarNotaCreditoHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req aplicarNotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	doc, err := h.svc.AplicarNotaCredito(c.Request.Context(), empresaID, c.Param("id"), req.NotaID, req.Monto, usuarioID)
	if err != nil {
		h.responderError(c, err, "aplicar-nota-credito")
		return
	}
	c.JSON(http.StatusOK, doc)
}

// NotasDeCreditoHandler GET /v1/cxp/notas-credito?proveedor_id=
//
// Sin proveedor devuelve todas las que tengan saldo; con proveedor, solo las suyas. La pantalla
// manda el del documento que está rebajando: una nota rebaja lo que le debemos a QUIEN la emitió,
// y ofrecer las de otro invita a un error que el servidor después rechaza.
func (h *Handler) NotasDeCreditoHandler(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	notas, err := h.svc.NotasDeCreditoDisponibles(c.Request.Context(), empresaID, c.Query("proveedor_id"))
	if err != nil {
		h.responderError(c, err, "notas-credito")
		return
	}
	c.JSON(http.StatusOK, notas)
}

// NotasDeFacturaHandler GET /v1/cxp/documentos/:id/notas-credito
func (h *Handler) NotasDeFacturaHandler(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	aplicadas, err := h.svc.NotasAplicadasAFactura(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "notas-de-factura")
		return
	}
	c.JSON(http.StatusOK, aplicadas)
}

// ReversarNotaCreditoHandler DELETE /v1/cxp/notas-credito/aplicaciones/:id
//
// Existe para que aplicar la nota a la factura equivocada no sea un camino de ida: un proveedor
// con varias facturas abiertas por montos parecidos es el caso común.
func (h *Handler) ReversarNotaCreditoHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.ReversarNotaCredito(c.Request.Context(), empresaID, c.Param("id"), usuarioID); err != nil {
		h.responderError(c, err, "reversar-nota-credito")
		return
	}
	c.Status(http.StatusNoContent)
}
