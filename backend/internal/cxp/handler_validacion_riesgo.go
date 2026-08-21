package cxp

// Handlers de los umbrales de validación por riesgo.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// ParametrosValidacion GET /v1/cxp/parametros — los umbrales vigentes.
func (h *Handler) ParametrosValidacion(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	ps, err := h.svc.ParametrosValidacion(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "parametros-validacion")
		return
	}
	// El efecto va en la MISMA respuesta: un umbral sin el número de facturas que mueve es un
	// formulario a ciegas. Si falla la medición, los umbrales se muestran igual.
	efecto, err := h.svc.EfectoValidacion(c.Request.Context(), empresaID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"parametros": ps})
		return
	}
	c.JSON(http.StatusOK, gin.H{"parametros": ps, "efecto": efecto})
}

type parametroRequest struct {
	Valor string `json:"valor" validate:"required"`
}

// GuardarParametroValidacion PUT /v1/cxp/parametros/:clave
func (h *Handler) GuardarParametroValidacion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req parametroRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := h.svc.GuardarParametroValidacion(c.Request.Context(), empresaID, c.Param("clave"), req.Valor, usuarioID); err != nil {
		h.responderError(c, err, "guardar-parametro-validacion")
		return
	}
	c.Status(http.StatusNoContent)
}
