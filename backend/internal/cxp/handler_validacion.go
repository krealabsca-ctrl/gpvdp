package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

type asignarDeptoRequest struct {
	DepartamentoID string `json:"departamento_id" validate:"required,uuid"`
}

// AsignarDepartamentoDocumento PATCH /v1/cxp/documentos/:id/departamento
func (h *Handler) AsignarDepartamentoDocumento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req asignarDeptoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	d, err := h.svc.AsignarDepartamentoDoc(c.Request.Context(), empresaID, c.Param("id"), req.DepartamentoID, usuarioID)
	if err != nil {
		h.responderError(c, err, "asignar-departamento-doc")
		return
	}
	c.JSON(http.StatusOK, d)
}

type validarDeptoRequest struct {
	Respaldo string `json:"respaldo" validate:"required,max=200"`
	Nota     string `json:"nota" validate:"omitempty,max=500"`
}

// ValidarDeptoDocumento POST /v1/cxp/documentos/:id/validar-depto
func (h *Handler) ValidarDeptoDocumento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req validarDeptoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	d, err := h.svc.ValidarDepto(c.Request.Context(), empresaID, c.Param("id"), usuarioID, req.Respaldo, req.Nota)
	if err != nil {
		h.responderError(c, err, "validar-depto")
		return
	}
	c.JSON(http.StatusOK, d)
}

type validarEscaladoRequest struct {
	Respaldo string `json:"respaldo" validate:"omitempty,max=200"`
	Motivo   string `json:"motivo" validate:"required,max=500"`
}

// ValidarEscaladoDocumento POST /v1/cxp/documentos/:id/validar-escalado
func (h *Handler) ValidarEscaladoDocumento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req validarEscaladoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	d, err := h.svc.ValidarEscalado(c.Request.Context(), empresaID, c.Param("id"), usuarioID, req.Respaldo, req.Motivo)
	if err != nil {
		h.responderError(c, err, "validar-escalado")
		return
	}
	c.JSON(http.StatusOK, d)
}

type devolverRequest struct {
	Nota string `json:"nota" validate:"omitempty,max=500"`
}

// DevolverDocumento POST /v1/cxp/documentos/:id/devolver
func (h *Handler) DevolverDocumento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req devolverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	d, err := h.svc.Devolver(c.Request.Context(), empresaID, c.Param("id"), req.Nota, usuarioID)
	if err != nil {
		h.responderError(c, err, "devolver-documento")
		return
	}
	c.JSON(http.StatusOK, d)
}

// ListarUsuarios GET /v1/cxp/usuarios — usuarios de la empresa (para el selector de validadores).
func (h *Handler) ListarUsuarios(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	us, err := h.svc.UsuariosEmpresa(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "listar-usuarios")
		return
	}
	c.JSON(http.StatusOK, us)
}

// ListarValidadores GET /v1/cxp/departamentos/:id/validadores
func (h *Handler) ListarValidadores(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	vs, err := h.svc.Validadores(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "listar-validadores")
		return
	}
	c.JSON(http.StatusOK, vs)
}

type asignarValidadorRequest struct {
	UsuarioID string `json:"usuario_id" validate:"required,uuid"`
	Rol       string `json:"rol" validate:"omitempty,oneof=TITULAR SUPLENTE"`
}

// AsignarValidador POST /v1/cxp/departamentos/:id/validadores
func (h *Handler) AsignarValidador(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req asignarValidadorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.AsignarValidador(c.Request.Context(), empresaID, c.Param("id"), req.UsuarioID, req.Rol, usuarioID); err != nil {
		h.responderError(c, err, "asignar-validador")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// QuitarValidador DELETE /v1/cxp/departamentos/:id/validadores/:usuarioId
func (h *Handler) QuitarValidador(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.QuitarValidador(c.Request.Context(), empresaID, c.Param("id"), c.Param("usuarioId"), usuarioID); err != nil {
		h.responderError(c, err, "quitar-validador")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
