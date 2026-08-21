package bancos

// Fase B — análisis visual: endpoints de solo lectura para el tablero.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// SerieMensual GET /v1/bancos/analisis/serie-mensual?hasta=YYYY-MM&meses=12
func (h *Handler) SerieMensual(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	serie, err := h.svc.SerieMensual(c.Request.Context(), claims.EmpresaID,
		c.Query("hasta"), atoiDefault(c.Query("meses"), 12))
	if err != nil {
		h.responderError(c, err, "serie-mensual")
		return
	}
	if serie == nil {
		serie = []SerieMensualPunto{}
	}
	c.JSON(http.StatusOK, serie)
}

// CalendarioDiario GET /v1/bancos/analisis/calendario?periodo=YYYY-MM
func (h *Handler) CalendarioDiario(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	periodo := c.Query("periodo")
	if periodo == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "periodo (YYYY-MM) es requerido")
		return
	}
	dias, err := h.svc.CalendarioDiario(c.Request.Context(), claims.EmpresaID, periodo)
	if err != nil {
		h.responderError(c, err, "calendario-diario")
		return
	}
	if dias == nil {
		dias = []DiaCalendario{}
	}
	c.JSON(http.StatusOK, dias)
}

// ResumenPorCuenta GET /v1/bancos/analisis/cuentas?periodo=YYYY-MM
func (h *Handler) ResumenPorCuenta(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	periodo := c.Query("periodo")
	if periodo == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "periodo (YYYY-MM) es requerido")
		return
	}
	cuentas, err := h.svc.ResumenPorCuenta(c.Request.Context(), claims.EmpresaID, periodo)
	if err != nil {
		h.responderError(c, err, "resumen-cuentas")
		return
	}
	if cuentas == nil {
		cuentas = []CuentaResumen{}
	}
	c.JSON(http.StatusOK, cuentas)
}
