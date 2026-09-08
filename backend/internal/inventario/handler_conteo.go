package inventario

// Endpoints del conteo cíclico.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// Conteos GET /v1/inventario/conteos?estado=&sede_id= (inventario.ver)
func (h *Handler) Conteos(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.Conteos(c.Request.Context(), cl.EmpresaID, c.Query("estado"), c.Query("sede_id"))
	if err != nil {
		h.responder(c, err, "conteos")
		return
	}
	c.JSON(http.StatusOK, list)
}

// Conteo GET /v1/inventario/conteos/:id (inventario.ver) — la hoja con sus líneas.
func (h *Handler) Conteo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	hoja, err := h.svc.Conteo(c.Request.Context(), cl.EmpresaID, c.Param("id"))
	if err != nil {
		h.responder(c, err, "conteo")
		return
	}
	c.JSON(http.StatusOK, hoja)
}

type abrirConteoRequest struct {
	SedeID      string `json:"sede_id" binding:"required,uuid"`
	CategoriaID string `json:"categoria_id"`
	Fecha       string `json:"fecha"`
	Nota        string `json:"nota"`
}

// AbrirConteo POST /v1/inventario/conteos (inventario.conteo)
//
// Congela lo que el sistema dice que hay: de ahí en adelante la hoja se compara contra esa foto.
func (h *Handler) AbrirConteo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req abrirConteoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta sede_id (uuid)")
		return
	}
	hoja, err := h.svc.AbrirConteo(c.Request.Context(), cl.EmpresaID, ConteoNuevo{
		SedeID: req.SedeID, CategoriaID: req.CategoriaID, Fecha: req.Fecha, Nota: req.Nota,
	}, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "abrir-conteo")
		return
	}
	c.JSON(http.StatusCreated, hoja)
}

type lineaConteoRequest struct {
	// Puntero para poder distinguir «contó cero» de «no mandó el campo».
	CantidadContada *int   `json:"cantidad_contada" binding:"required"`
	Motivo          string `json:"motivo"`
}

// GuardarLineaConteo PUT /v1/inventario/conteos/:id/lineas/:linea (inventario.conteo)
func (h *Handler) GuardarLineaConteo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req lineaConteoRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.CantidadContada == nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"hace falta cantidad_contada (un número, aunque sea 0)")
		return
	}
	if err := h.svc.GuardarLineaConteo(c.Request.Context(), cl.EmpresaID, c.Param("id"),
		c.Param("linea"), *req.CantidadContada, req.Motivo, cl.UsuarioID()); err != nil {
		h.responder(c, err, "guardar-linea-conteo")
		return
	}
	c.Status(http.StatusNoContent)
}

// CerrarConteo POST /v1/inventario/conteos/:id/cerrar (inventario.conteo)
//
// Genera los ajustes de las diferencias explicadas. No cierra si queda algo sin contar o sin
// explicar: el error dice exactamente qué falta.
func (h *Handler) CerrarConteo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req struct {
		Fecha string `json:"fecha"`
	}
	_ = c.ShouldBindJSON(&req)
	res, err := h.svc.CerrarConteo(c.Request.Context(), cl.EmpresaID, c.Param("id"), req.Fecha, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "cerrar-conteo")
		return
	}
	c.JSON(http.StatusOK, res)
}

// AnularConteo POST /v1/inventario/conteos/:id/anular (inventario.conteo)
func (h *Handler) AnularConteo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req struct {
		Motivo string `json:"motivo"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.AnularConteo(c.Request.Context(), cl.EmpresaID, c.Param("id"), req.Motivo, cl.UsuarioID()); err != nil {
		h.responder(c, err, "anular-conteo")
		return
	}
	c.Status(http.StatusNoContent)
}

// PlanDeConteo GET /v1/inventario/conteos/plan (inventario.ver)
//
// Qué toca contar: por sede y clase de capital, con cuánto hace que no se cuenta.
func (h *Handler) PlanDeConteo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	plan, err := h.svc.PlanDeConteo(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err, "plan-conteo")
		return
	}
	c.JSON(http.StatusOK, plan)
}
