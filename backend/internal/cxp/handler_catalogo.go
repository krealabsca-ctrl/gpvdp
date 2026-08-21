package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// ListarSubclasificaciones GET /v1/cxp/catalogo/subclasificaciones?clasificacion_id=
func (h *Handler) ListarSubclasificaciones(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	lista, err := h.svc.Subclasificaciones(c.Request.Context(), empresaID, c.Query("clasificacion_id"))
	if err != nil {
		h.responderError(c, err, "listar-subclasificaciones")
		return
	}
	c.JSON(http.StatusOK, lista)
}

type crearSubclasifRequest struct {
	ClasificacionID string `json:"clasificacion_id" validate:"required,uuid"`
	Nombre          string `json:"nombre" validate:"required"`
}

// CrearSubclasificacion POST /v1/cxp/catalogo/subclasificaciones
func (h *Handler) CrearSubclasificacion(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req crearSubclasifRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	s, err := h.svc.CrearSubclasificacion(c.Request.Context(), empresaID, req.ClasificacionID, req.Nombre)
	if err != nil {
		h.responderError(c, err, "crear-subclasificacion")
		return
	}
	c.JSON(http.StatusCreated, s)
}
