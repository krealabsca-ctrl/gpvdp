package cxp

// Endpoints del catálogo de nomenclaturas.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// Nomenclaturas GET /v1/cxp/nomenclaturas — el catálogo de gasto con el código de cada rubro.
func (h *Handler) Nomenclaturas(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	rubros, err := h.svc.CatalogoNomenclaturas(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "nomenclaturas")
		return
	}
	c.JSON(http.StatusOK, rubros)
}

type codigoPagoRequest struct {
	// Nivel: CONCEPTO o CLASIFICACION. Decide en qué tabla se escribe, así que se valida contra
	// una whitelist en el service, no con un tag.
	Nivel string `json:"nivel" validate:"required"`
	ID    string `json:"id" validate:"required,uuid"`
	// Codigo vacío QUITA el código. No es un error: desasignar es una operación legítima.
	Codigo string `json:"codigo"`
}

// AsignarNomenclatura PUT /v1/cxp/nomenclaturas — pone o quita el código de un rubro.
func (h *Handler) AsignarNomenclatura(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req codigoPagoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.AsignarCodigoPago(c.Request.Context(), empresaID, req.Nivel, req.ID, req.Codigo, usuarioID); err != nil {
		h.responderError(c, err, "asignar-nomenclatura")
		return
	}
	c.Status(http.StatusNoContent)
}
