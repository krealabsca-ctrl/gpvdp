package flujocaja

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone el flujo de caja por HTTP.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

// monedaDe lee la moneda del query. Por defecto colones: es donde está casi todo el movimiento,
// y pedir la moneda en cada llamada haría que un enlace sin el parámetro falle en vez de mostrar
// lo más probable.
func monedaDe(c *gin.Context) string {
	m := strings.ToUpper(strings.TrimSpace(c.Query("moneda")))
	if m == "" {
		return "CRC"
	}
	return m
}

// Flujo GET /v1/flujo-caja?moneda=CRC (bancos.ver_proyecciones)
func (h *Handler) Flujo(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	f, err := h.svc.Flujo(c.Request.Context(), cl.EmpresaID, monedaDe(c))
	if err != nil {
		h.responder(c, err, "flujo")
		return
	}
	c.JSON(http.StatusOK, f)
}

// Politica GET /v1/flujo-caja/politica?moneda=CRC (bancos.ver_proyecciones)
func (h *Handler) Politica(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	p, err := h.svc.Politica(c.Request.Context(), cl.EmpresaID, monedaDe(c))
	if err != nil {
		h.responder(c, err, "politica")
		return
	}
	c.JSON(http.StatusOK, p)
}

// GuardarPolitica PUT /v1/flujo-caja/politica (bancos.politica_caja)
func (h *Handler) GuardarPolitica(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req PoliticaCaja
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "la política no se pudo leer")
		return
	}
	if err := h.svc.GuardarPolitica(c.Request.Context(), cl.EmpresaID, cl.UsuarioID(), req); err != nil {
		h.responder(c, err, "guardar-politica")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) responder(c *gin.Context, err error, op string) {
	switch {
	// Todos van con su texto completo: cada uno dice QUÉ corregir, y quien declara una política
	// de pago no tiene otra forma de saber por qué se rechazó.
	case errors.Is(err, ErrMonedaInvalida), errors.Is(err, ErrModoInvalido),
		errors.Is(err, ErrTopeInvalido), errors.Is(err, ErrPctInvalido),
		errors.Is(err, ErrMinimoInvalido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			strings.TrimPrefix(err.Error(), "flujocaja: "))
	default:
		httpx.AbortInterno(c, h.log, "flujocaja "+op, err)
	}
}
