package cxc

// HTTP de la suspensión por mora. La regla es 18 cuotas vencidas sin pagar; el sistema dice
// cuándo se puede y una persona decide.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// EstadoSuspension GET /v1/cxc/contratos/:numero/suspension
func (h *Handler) EstadoSuspension(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	est, err := h.svc.EstadoDeSuspension(c.Request.Context(), empresaID, c.Param("numero"))
	if err != nil {
		h.error(c, err, "estado-suspension")
		return
	}
	c.JSON(http.StatusOK, est)
}

type motivoRequest struct {
	Motivo string `json:"motivo" binding:"required"`
}

// Suspender POST /v1/cxc/contratos/:numero/suspender
func (h *Handler) Suspender(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req motivoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta el motivo de la suspensión")
		return
	}
	est, err := h.svc.Suspender(c.Request.Context(), empresaID, c.Param("numero"), req.Motivo, usuarioID)
	if err != nil {
		h.error(c, err, "suspender")
		return
	}
	c.JSON(http.StatusOK, est)
}

// Reactivar POST /v1/cxc/contratos/:numero/reactivar
func (h *Handler) Reactivar(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req motivoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta el motivo de la reactivación")
		return
	}
	est, err := h.svc.Reactivar(c.Request.Context(), empresaID, c.Param("numero"), req.Motivo, usuarioID)
	if err != nil {
		h.error(c, err, "reactivar")
		return
	}
	c.JSON(http.StatusOK, est)
}
