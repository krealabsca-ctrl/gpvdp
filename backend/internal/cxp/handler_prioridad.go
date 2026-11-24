package cxp

// Endpoints del catálogo de prioridades (mig 0121). El porqué está en prioridad.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// ListarPrioridades GET /v1/cxp/prioridades?activas=1 (cxp.ver)
//
// Se LEE con el permiso de ver y no con el de configurar: la Bandeja necesita el catálogo para
// dibujar las insignias y el selector, y eso lo usa cualquiera que mire facturas.
func (h *Handler) ListarPrioridades(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.Prioridades(c.Request.Context(), empresaID, c.Query("activas") == "1")
	if err != nil {
		h.responderError(c, err, "listar-prioridades")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// CrearPrioridad POST /v1/cxp/prioridades (cxp.catalogo)
func (h *Handler) CrearPrioridad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var in PrioridadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	p, err := h.svc.CrearPrioridad(c.Request.Context(), empresaID, in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-prioridad")
		return
	}
	c.JSON(http.StatusCreated, p)
}

// ActualizarPrioridad PATCH /v1/cxp/prioridades/:id (cxp.catalogo)
func (h *Handler) ActualizarPrioridad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var in PrioridadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	p, err := h.svc.ActualizarPrioridad(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "actualizar-prioridad")
		return
	}
	c.JSON(http.StatusOK, p)
}

// EliminarPrioridad DELETE /v1/cxp/prioridades/:id (cxp.catalogo)
//
// Solo borra si no cuelga ninguna factura. Si cuelga, responde explicando que se desactiva: es el
// mismo criterio del resto del catálogo.
func (h *Handler) EliminarPrioridad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.EliminarPrioridad(c.Request.Context(), empresaID, c.Param("id"), usuarioID); err != nil {
		h.responderError(c, err, "eliminar-prioridad")
		return
	}
	c.Status(http.StatusNoContent)
}
