package bancos

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Cuadre GET /v1/bancos/cuadre?periodo=YYYY-MM — totales por concepto del período.
func (h *Handler) Cuadre(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	periodo := c.Query("periodo")
	if periodo == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "periodo requerido (YYYY-MM)")
		return
	}
	rows, err := h.svc.Cuadre(c.Request.Context(), claims.EmpresaID, periodo)
	if err != nil {
		h.responderError(c, err, "cuadre")
		return
	}
	if rows == nil {
		rows = []CuadreRow{}
	}
	c.JSON(http.StatusOK, rows)
}

// CuadreArbol GET /v1/bancos/cuadre/arbol?periodo=YYYY-MM — cuadre jerárquico (Tipo→Concepto→Clasificación).
func (h *Handler) CuadreArbol(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	periodo := c.Query("periodo")
	if periodo == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "periodo requerido (YYYY-MM)")
		return
	}
	res, err := h.svc.CuadreArbol(c.Request.Context(), claims.EmpresaID, periodo)
	if err != nil {
		h.responderError(c, err, "cuadre-arbol")
		return
	}
	c.JSON(http.StatusOK, res)
}

// Dashboard GET /v1/bancos/dashboard?periodo=YYYY-MM — KPIs del período.
func (h *Handler) Dashboard(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	periodo := c.Query("periodo")
	if periodo == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "periodo requerido (YYYY-MM)")
		return
	}
	res, err := h.svc.Dashboard(c.Request.Context(), claims.EmpresaID, periodo)
	if err != nil {
		h.responderError(c, err, "dashboard")
		return
	}
	c.JSON(http.StatusOK, res)
}
