package cxc

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// CorregirContrato PATCH /v1/cxc/contratos/:numero (cxc.importar)
//
// Arregla los datos que dejaron al contrato apartado. Es PATCH y no PUT a propósito: solo llegan los
// campos que se quieren cambiar, y los que no vienen quedan como están. Con PUT, corregir la
// modalidad pondría la cuota en cero y volvería a apartar el contrato.
func (h *Handler) CorregirContrato(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req CorreccionContrato
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el cuerpo tiene que traer al menos uno de: cuota, dia_pago o modalidad_id")
		return
	}
	out, err := h.svc.CorregirContrato(c.Request.Context(), empresaID, usuarioID, c.Param("numero"), req)
	if err != nil {
		h.error(c, err, "corregir contrato")
		return
	}
	c.JSON(http.StatusOK, out)
}
