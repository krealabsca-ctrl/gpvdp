package plantillas

// Endpoints de las plantillas de correo.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone la edición de plantillas.
type Handler struct{ svc *Service }

// NewHandler construye el handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Listar GET /v1/plantillas — tipos de notificación con su texto vigente y sus variables.
func (h *Handler) Listar(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	items, err := h.svc.Listar(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
		return
	}
	c.JSON(http.StatusOK, items)
}

type guardarRequest struct {
	Asunto string `json:"asunto"`
	Cuerpo string `json:"cuerpo"`
}

// Guardar PUT /v1/plantillas/:clave (admin.plantillas)
func (h *Handler) Guardar(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req guardarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido: se esperan asunto y cuerpo")
		return
	}
	desconocidas, err := h.svc.Guardar(c.Request.Context(), claims.EmpresaID, c.Param("clave"),
		req.Asunto, req.Cuerpo, claims.UsuarioID())
	if err != nil {
		responder(c, err, desconocidas)
		return
	}
	c.JSON(http.StatusOK, gin.H{"guardada": true})
}

// Restablecer DELETE /v1/plantillas/:clave (admin.plantillas) — vuelve al texto de fábrica.
func (h *Handler) Restablecer(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.Restablecer(c.Request.Context(), claims.EmpresaID, c.Param("clave"), claims.UsuarioID()); err != nil {
		responder(c, err, nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"restablecida": true})
}

// VistaPrevia POST /v1/plantillas/:clave/vista-previa — arma el correo con datos de ejemplo.
// No guarda nada: sirve para ver cómo queda el texto que se está editando.
func (h *Handler) VistaPrevia(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req guardarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	asunto, cuerpo, desconocidas, err := h.svc.VistaPrevia(c.Request.Context(), claims.EmpresaID,
		c.Param("clave"), req.Asunto, req.Cuerpo)
	if err != nil {
		responder(c, err, desconocidas)
		return
	}
	c.JSON(http.StatusOK, gin.H{"asunto": asunto, "cuerpo": cuerpo, "desconocidas": desconocidas})
}

// responder mapea los errores de dominio a HTTP.
func responder(c *gin.Context, err error, desconocidas []string) {
	switch {
	case errors.Is(err, ErrTipoDesconocido):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, "esa notificación no existe")
	case errors.Is(err, ErrVariablesDesconocidas):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el texto usa variables que el sistema no sabe llenar: ["+strings.Join(desconocidas, "], [")+"]")
	case errors.Is(err, ErrAsuntoVacio), errors.Is(err, ErrCuerpoVacio):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
	default:
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}
