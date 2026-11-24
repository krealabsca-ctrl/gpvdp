package ratios

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone los ratios por HTTP.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

// Ciclo GET /v1/ratios/ciclo?meses=12 (ratios.ver)
func (h *Handler) Ciclo(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	meses := 0
	if s := strings.TrimSpace(c.Query("meses")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
				"la ventana tiene que ser un número de meses")
			return
		}
		meses = n
	}
	ciclo, err := h.svc.Ciclo(c.Request.Context(), cl.EmpresaID, meses)
	if err != nil {
		if errors.Is(err, ErrVentanaInvalida) {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
				strings.TrimPrefix(err.Error(), "ratios: "))
			return
		}
		httpx.AbortInterno(c, h.log, "ratios ciclo", err)
		return
	}
	c.JSON(http.StatusOK, ciclo)
}

// Drivers GET /v1/ratios/drivers?dimension=tipo_servicio&periodo=2026-09&meses=12 (ratios.ver)
//
// El negocio por lo que lo mueve: cuántos contratos, de qué tipo, con qué cuota, y cuántos de
// ellos facturaron de verdad.
func (h *Handler) Drivers(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	meses := 0
	if s := strings.TrimSpace(c.Query("meses")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
				"los meses de altas tienen que ser un número")
			return
		}
		meses = n
	}
	d, err := h.svc.Drivers(c.Request.Context(), cl.EmpresaID,
		strings.TrimSpace(c.Query("dimension")), strings.TrimSpace(c.Query("periodo")), meses)
	if err != nil {
		if errors.Is(err, ErrDimensionInvalida) {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
				strings.TrimPrefix(err.Error(), "ratios: "))
			return
		}
		httpx.AbortInterno(c, h.log, "ratios drivers", err)
		return
	}
	c.JSON(http.StatusOK, d)
}
