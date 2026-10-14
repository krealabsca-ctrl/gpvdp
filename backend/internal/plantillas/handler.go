package plantillas

// Endpoints de las plantillas de correo.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone la edición de plantillas.
//
// Lleva logger porque un error inesperado acá tiene que quedar escrito: sin él, el 500 sale con
// una referencia que no lleva a ninguna línea de log y la trazabilidad es una promesa vacía.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

// Listar GET /v1/plantillas — tipos de notificación con su texto vigente y sus variables.
func (h *Handler) Listar(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	items, err := h.svc.Listar(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		httpx.AbortInterno(c, h.log, "plantillas: listar", err)
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
		responder(c, h.log, err, desconocidas)
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
		responder(c, h.log, err, nil)
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
		responder(c, h.log, err, desconocidas)
		return
	}
	c.JSON(http.StatusOK, gin.H{"asunto": asunto, "cuerpo": cuerpo, "desconocidas": desconocidas})
}

// responder mapea los errores de dominio a HTTP. Recibe el logger porque el caso `default`
// necesita dejar la causa escrita junto a la referencia que ve el usuario.
func responder(c *gin.Context, log *zap.Logger, err error, desconocidas []string) {
	switch {
	case errors.Is(err, ErrTipoDesconocido):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, "esa notificación no existe")
	case errors.Is(err, ErrVariablesDesconocidas):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el texto usa variables que el sistema no sabe llenar: ["+strings.Join(desconocidas, "], [")+"]")
	case errors.Is(err, ErrAsuntoVacio), errors.Is(err, ErrCuerpoVacio):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
	default:
		httpx.AbortInterno(c, log, "plantillas", err)
	}
}
