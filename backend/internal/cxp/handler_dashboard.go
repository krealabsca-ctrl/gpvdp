package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// HistorialDocumento GET /v1/cxp/documentos/:id/historial — línea de tiempo del documento.
func (h *Handler) HistorialDocumento(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	eventos, err := h.svc.HistorialDocumento(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "historial-documento")
		return
	}
	c.JSON(http.StatusOK, gin.H{"eventos": eventos})
}

// Bandeja GET /v1/cxp/bandeja — resumen por fase (pestañas de la Bandeja), con scoping por área.
func (h *Handler) Bandeja(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	fases, err := h.svc.Bandeja(c.Request.Context(), claims.EmpresaID, claims.Rol, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "bandeja")
		return
	}
	if fases == nil {
		fases = []FaseBandeja{}
	}
	c.JSON(http.StatusOK, gin.H{"fases": fases})
}

// Dashboard GET /v1/cxp/dashboard?periodo=YYYY-MM — tablero del módulo CxP.
// El período (selector global de la barra) manda sobre el MOVIMIENTO; la cartera es a hoy.
// Sin período explícito se asume el mes en curso de Costa Rica.
func (h *Handler) Dashboard(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	d, err := h.svc.Dashboard(c.Request.Context(), claims.EmpresaID, c.Query("periodo"),
		claims.Rol, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "dashboard-cxp")
		return
	}
	c.JSON(http.StatusOK, d)
}
