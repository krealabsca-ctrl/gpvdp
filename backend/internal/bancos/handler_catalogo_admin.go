package bancos

// Administración del catálogo: PATCH (renombrar) y DELETE de conceptos/clasificaciones.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

type renombrarCatalogoRequest struct {
	Nombre string `json:"nombre" validate:"required"`
}

type actualizarConceptoRequest struct {
	Nombre     string `json:"nombre"`
	VisibleCxP *bool  `json:"visible_cxp"`
	// Naturaleza: INGRESO | GASTO | NEUTRO. Decide si el concepto entra al EBITDA y por qué lado.
	Naturaleza string `json:"naturaleza"`
}

// RenombrarConcepto PATCH /v1/bancos/catalogo/conceptos/:id — nombre y/o visible_cxp.
func (h *Handler) RenombrarConcepto(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req actualizarConceptoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if req.Nombre == "" && req.VisibleCxP == nil && req.Naturaleza == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "nada que actualizar")
		return
	}
	// Se valida en el borde: un valor fuera de la lista llegaría al CHECK de Postgres y saldría
	// como 500 en vez de decir qué está mal.
	if req.Naturaleza != "" && !NaturalezaValida(req.Naturaleza) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"naturaleza debe ser INGRESO, GASTO o NEUTRO")
		return
	}
	if req.Nombre != "" {
		if err := h.svc.RenombrarConcepto(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Nombre, claims.UsuarioID()); err != nil {
			h.responderError(c, err, "renombrar-concepto")
			return
		}
	}
	if req.VisibleCxP != nil {
		if err := h.svc.CambiarVisibilidadCxP(c.Request.Context(), claims.EmpresaID, c.Param("id"), *req.VisibleCxP, claims.UsuarioID()); err != nil {
			h.responderError(c, err, "visibilidad-concepto")
			return
		}
	}
	if req.Naturaleza != "" {
		if err := h.svc.CambiarNaturaleza(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Naturaleza, claims.UsuarioID()); err != nil {
			h.responderError(c, err, "naturaleza-concepto")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// EliminarConcepto DELETE /v1/bancos/catalogo/conceptos/:id
func (h *Handler) EliminarConcepto(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.EliminarConcepto(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "eliminar-concepto")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type actualizarClasificacionRequest struct {
	Nombre     string `json:"nombre"`
	ConceptoID string `json:"concepto_id" validate:"omitempty,uuid"`
}

// RenombrarClasificacion PATCH /v1/bancos/catalogo/clasificaciones/:id — nombre y/o concepto.
func (h *Handler) RenombrarClasificacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req actualizarClasificacionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if req.Nombre == "" && req.ConceptoID == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "nada que actualizar")
		return
	}
	// Mover de concepto (si se pide) y/o renombrar.
	if req.ConceptoID != "" {
		if err := h.svc.ReasignarConceptoClasificacion(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.ConceptoID, claims.UsuarioID()); err != nil {
			h.responderError(c, err, "reasignar-concepto-clasificacion")
			return
		}
	}
	if req.Nombre != "" {
		if err := h.svc.RenombrarClasificacion(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Nombre, claims.UsuarioID()); err != nil {
			h.responderError(c, err, "renombrar-clasificacion")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// EliminarClasificacion DELETE /v1/bancos/catalogo/clasificaciones/:id
func (h *Handler) EliminarClasificacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.EliminarClasificacion(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "eliminar-clasificacion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
