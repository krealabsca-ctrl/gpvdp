package rbac

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone la administración de la matriz RBAC.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Permisos GET /v1/rbac/permisos — catálogo completo de permisos.
func (h *Handler) Permisos(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"permisos": Catalogo})
}

// MisPermisos GET /v1/rbac/mis-permisos — permisos EFECTIVOS del usuario en la
// empresa activa (para que el frontend oculte lo que no puede hacer). No requiere
// admin.roles: cada quien puede ver su propio alcance.
func (h *Handler) MisPermisos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	permisos, err := h.svc.PermisosDe(c.Request.Context(), claims.EmpresaID, claims.Rol)
	if err != nil {
		h.error(c, err)
		return
	}
	if permisos == nil {
		permisos = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"rol": claims.Rol, "es_admin": claims.Rol == RolAdmin, "permisos": permisos})
}

// Roles GET /v1/rbac/roles — roles visibles para la empresa activa.
func (h *Handler) Roles(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	roles, err := h.svc.Roles(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.error(c, err)
		return
	}
	if roles == nil {
		roles = []RolItem{}
	}
	c.JSON(http.StatusOK, roles)
}

// Matriz GET /v1/rbac/matriz — concesiones (rol × permiso) de la empresa activa.
func (h *Handler) Matriz(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	grants, err := h.svc.Matriz(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.error(c, err)
		return
	}
	if grants == nil {
		grants = []MatrizGrant{}
	}
	c.JSON(http.StatusOK, grants)
}

type setPermisosRequest struct {
	Permisos []string `json:"permisos"`
}

// SetPermisos PUT /v1/rbac/roles/:codigo/permisos — reemplaza los permisos del rol.
func (h *Handler) SetPermisos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req setPermisosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if req.Permisos == nil {
		req.Permisos = []string{}
	}
	if err := h.svc.SetPermisosDeRol(c.Request.Context(), claims.EmpresaID, c.Param("codigo"), req.Permisos, claims.UsuarioID()); err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type crearRolRequest struct {
	Nombre string `json:"nombre" validate:"required"`
}

// CrearRol POST /v1/rbac/roles — crea un rol a medida para la empresa.
func (h *Handler) CrearRol(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req crearRolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	it, err := h.svc.CrearRol(c.Request.Context(), claims.EmpresaID, req.Nombre, claims.UsuarioID())
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusCreated, it)
}

func (h *Handler) error(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrRolNoEncontrado):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, "rol no encontrado")
	case errors.Is(err, ErrRolDuplicado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, "ya existe un rol con ese nombre")
	case errors.Is(err, ErrPermisoInvalido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "permiso desconocido")
	case errors.Is(err, ErrRolBaseProtegido):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, "ADMIN es superusuario; su matriz no se edita")
	case errors.Is(err, ErrEmailDuplicado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, "ya existe un usuario con ese correo")
	case errors.Is(err, ErrUsuarioNoEncontrado):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, "usuario no encontrado en esta empresa")
	default:
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}
