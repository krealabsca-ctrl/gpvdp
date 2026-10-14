package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// FacturasDeContado GET /v1/cxp/proveedores/:id/bloqueadas
//
// Lo que quedó trancado por el candado del contado para ese proveedor. Se consulta al abrir su
// ficha, porque es ahí donde se toma la decisión de que manda el convenio.
func (h *Handler) FacturasDeContado(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	fs, err := h.svc.FacturasDeContado(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "facturas-de-contado")
		return
	}
	c.JSON(http.StatusOK, gin.H{"facturas": fs})
}

// LiberarParaPago POST /v1/cxp/proveedores/:id/liberar
//
// Responde con lo que quedó libre Y con lo que no, cada uno con su motivo. Un «ok» pelado dejaría
// al operador creyendo que liberó las seis cuando fueron cuatro, y las dos restantes seguirían
// sin poder pagarse hasta que alguien las busque de a una.
func (h *Handler) LiberarParaPago(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		DocumentoIDs []string `json:"documento_ids"`
		Motivo       string   `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.responderError(c, ErrAccionInvalida, "liberar-para-pago")
		return
	}
	res, err := h.svc.LiberarParaPago(c.Request.Context(), empresaID, c.Param("id"),
		LiberacionInput{DocumentoIDs: req.DocumentoIDs, Motivo: req.Motivo}, usuarioID)
	if err != nil {
		h.responderError(c, err, "liberar-para-pago")
		return
	}
	c.JSON(http.StatusOK, res)
}
