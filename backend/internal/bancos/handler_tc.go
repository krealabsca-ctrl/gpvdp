package bancos

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

type cotizacionRequest struct {
	Fecha  string `json:"fecha" validate:"required"` // YYYY-MM-DD
	Valor  string `json:"valor" validate:"required"`
	Fuente string `json:"fuente" validate:"required,oneof=BCCR MANUAL"`
}

// RegistrarCotizacion POST /v1/bancos/cotizaciones — registra una cotización (día 1/15/último).
func (h *Handler) RegistrarCotizacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req cotizacionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	valor, err := decimal.NewFromString(req.Valor)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "valor inválido")
		return
	}
	if err := h.svc.RegistrarCotizacion(c.Request.Context(), claims.EmpresaID, req.Fecha, valor, req.Fuente); err != nil {
		h.responderError(c, err, "cotizacion")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

// EstadoTC GET /v1/bancos/tipo-cambio/:anio/:mes — estado y cotizaciones del mes.
func (h *Handler) EstadoTC(c *gin.Context) {
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
	res, err := h.svc.EstadoMes(c.Request.Context(), claims.EmpresaID, anio, mes)
	if err != nil {
		h.responderError(c, err, "estado-tc")
		return
	}
	c.JSON(http.StatusOK, res)
}

// CongelarTC POST /v1/bancos/tipo-cambio/:anio/:mes/congelar — congela el TC (requiere rol autorizado).
func (h *Handler) CongelarTC(c *gin.Context) {
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
	n, err := h.svc.Congelar(c.Request.Context(), claims.EmpresaID, anio, mes, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "congelar-tc")
		return
	}
	c.JSON(http.StatusOK, gin.H{"anio": anio, "mes": mes, "movimientos": n})
}

type syncBCCRRequest struct {
	Fecha string `json:"fecha"` // YYYY-MM-DD; vacío = hoy
}

// SincronizarBCCR POST /v1/bancos/tipo-cambio/sync — dispara el sync manual con el BCCR.
func (h *Handler) SincronizarBCCR(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req syncBCCRRequest
	_ = c.ShouldBindJSON(&req) // cuerpo opcional
	fecha := req.Fecha
	if fecha == "" {
		fecha = time.Now().Format("2006-01-02")
	}
	res, err := h.svc.SincronizarBCCR(c.Request.Context(), claims.EmpresaID, fecha, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "sync-bccr")
		return
	}
	c.JSON(http.StatusOK, res)
}

// UltimoSyncBCCR GET /v1/bancos/tipo-cambio/ultimo-sync — resultado del último intento.
func (h *Handler) UltimoSyncBCCR(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	l, err := h.svc.UltimoSyncBCCR(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "ultimo-sync-bccr")
		return
	}
	if l == nil {
		c.JSON(http.StatusOK, gin.H{"sincronizado": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sincronizado": true, "log": l})
}

func parseAnioMes(c *gin.Context) (int, int, bool) {
	anio, err1 := strconv.Atoi(c.Param("anio"))
	mes, err2 := strconv.Atoi(c.Param("mes"))
	if err1 != nil || err2 != nil || anio < 2000 || anio > 2100 || mes < 1 || mes > 12 {
		return 0, 0, false
	}
	return anio, mes, true
}
