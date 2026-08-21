package bancos

// Handler de parámetros de negocio por empresa (Fase D).

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Parametros GET /v1/bancos/parametros
func (h *Handler) Parametros(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	p, err := h.svc.Parametros(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "parametros")
		return
	}
	c.JSON(http.StatusOK, p)
}

type toleranciaRequest struct {
	// Porcentaje ingresado por el usuario (p. ej. "1" = 1%, "1.5" = 1.5%).
	ToleranciaPct string `json:"tolerancia_pct" validate:"required"`
}

// ActualizarTolerancia PATCH /v1/bancos/parametros/tolerancia (requiere rol autorizado).
func (h *Handler) ActualizarTolerancia(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req toleranciaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	pctUsuario, err := decimal.NewFromString(req.ToleranciaPct)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "tolerancia inválida")
		return
	}
	// El usuario escribe porcentaje (1.5); se guarda como proporción (0.015).
	pct := pctUsuario.Div(decimal.NewFromInt(100))
	if err := h.svc.ActualizarTolerancia(c.Request.Context(), claims.EmpresaID, pct, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "actualizar-tolerancia")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
