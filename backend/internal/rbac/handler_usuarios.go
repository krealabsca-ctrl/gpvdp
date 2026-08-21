package rbac

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// ListarUsuarios GET /v1/rbac/usuarios — usuarios con acceso a la empresa activa.
func (h *Handler) ListarUsuarios(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	us, err := h.svc.Usuarios(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, us)
}

type crearUsuarioRequest struct {
	Nombre    string `json:"nombre" validate:"required,max=120"`
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8,max=72"`
	RolCodigo string `json:"rol_codigo" validate:"required"`
}

// CrearUsuario POST /v1/rbac/usuarios — alta (o vinculación) de usuario en la empresa activa.
func (h *Handler) CrearUsuario(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req crearUsuarioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	nuevo, err := h.svc.CrearUsuario(c.Request.Context(), claims.EmpresaID, req.Nombre, req.Email, req.Password, req.RolCodigo, claims.UsuarioID())
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "nuevo": nuevo})
}

type actualizarUsuarioRequest struct {
	Nombre    string `json:"nombre" validate:"required,max=120"`
	Activo    *bool  `json:"activo" validate:"required"`
	RolCodigo string `json:"rol_codigo"`
}

// ActualizarUsuario PATCH /v1/rbac/usuarios/:id
func (h *Handler) ActualizarUsuario(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req actualizarUsuarioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.ActualizarUsuario(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Nombre, *req.Activo, req.RolCodigo, claims.UsuarioID()); err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type resetPasswordRequest struct {
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// ResetPassword POST /v1/rbac/usuarios/:id/reset-password
func (h *Handler) ResetPassword(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Password, claims.UsuarioID()); err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// QuitarAcceso DELETE /v1/rbac/usuarios/:id/acceso — quita el acceso a la empresa activa.
func (h *Handler) QuitarAcceso(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.QuitarAcceso(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AplicarPermisosFaltantes POST /v1/rbac/permisos/aplicar-faltantes
func (h *Handler) AplicarPermisosFaltantes(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	n, err := h.svc.AplicarPermisosFaltantes(c.Request.Context(), claims.EmpresaID, claims.UsuarioID())
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "agregados": n})
}
