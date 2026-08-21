package bancos

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// PropuestasTraslados GET /v1/bancos/traslados/propuestas?periodo=YYYY-MM
func (h *Handler) PropuestasTraslados(c *gin.Context) {
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
	list, err := h.svc.PropuestasTraslados(c.Request.Context(), claims.EmpresaID, periodo)
	if err != nil {
		h.responderError(c, err, "propuestas-traslados")
		return
	}
	if list == nil {
		list = []PropuestaTraslado{}
	}
	c.JSON(http.StatusOK, list)
}

type emparejarRequest struct {
	MovimientoDebitoID  string `json:"movimiento_debito_id" validate:"required,uuid"`
	MovimientoCreditoID string `json:"movimiento_credito_id" validate:"required,uuid"`
}

// EmparejarTraslado POST /v1/bancos/traslados/emparejar
func (h *Handler) EmparejarTraslado(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req emparejarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.EmparejarTraslado(c.Request.Context(), claims.EmpresaID, req.MovimientoDebitoID, req.MovimientoCreditoID, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "emparejar-traslado")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type desemparejarRequest struct {
	MovimientoID string `json:"movimiento_id" validate:"required,uuid"`
}

// DesemparejarTraslado POST /v1/bancos/traslados/desemparejar
func (h *Handler) DesemparejarTraslado(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req desemparejarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.DesemparejarTraslado(c.Request.Context(), claims.EmpresaID, req.MovimientoID, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "desemparejar-traslado")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// EstadoPeriodo GET /v1/bancos/periodos/:anio/:mes
func (h *Handler) EstadoPeriodo(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	anio, mes, ok := parseAnioMes(c)
	if !ok {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año/mes inválidos")
		return
	}
	cerrado, err := h.svc.EstadoPeriodo(c.Request.Context(), claims.EmpresaID, anio, mes)
	if err != nil {
		h.responderError(c, err, "estado-periodo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"anio": anio, "mes": mes, "cerrado": cerrado})
}

// CerrarPeriodo POST /v1/bancos/periodos/:anio/:mes/cerrar (requiere rol autorizado)
func (h *Handler) CerrarPeriodo(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	anio, mes, ok := parseAnioMes(c)
	if !ok {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año/mes inválidos")
		return
	}
	n, err := h.svc.CerrarPeriodo(c.Request.Context(), claims.EmpresaID, anio, mes, claims.UsuarioID())
	if err != nil {
		if errors.Is(err, ErrPeriodoConNoIdentificados) {
			// 409 con la cantidad de pendientes, para que la UI la muestre.
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"code":             httpx.CodeConflicto,
				"message":          "hay movimientos No identificado pendientes; no se puede cerrar el período",
				"no_identificados": n,
			})
			return
		}
		h.responderError(c, err, "cerrar-periodo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"anio": anio, "mes": mes, "cerrado": true, "no_identificados_al_cierre": n})
}
