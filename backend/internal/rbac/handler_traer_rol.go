package rbac

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// RolesTraibles GET /v1/rbac/roles/traibles (admin.roles)
//
// Los roles a medida que existen en OTRAS empresas del usuario y todavía no acá.
func (h *Handler) RolesTraibles(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	list, err := h.svc.RolesTraibles(c.Request.Context(), claims.EmpresaID, claims.UsuarioID())
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

type traerRolRequest struct {
	EmpresaOrigenID string `json:"empresa_origen_id" validate:"required,uuid"`
	Codigo          string `json:"codigo" validate:"required"`
}

// TraerRol POST /v1/rbac/roles/traer (admin.roles)
//
// Copia acá un rol a medida de otra empresa, con sus permisos. Exige que quien lo trae tenga acceso
// a la empresa de origen: leer los permisos de un rol ajeno es leer datos de esa empresa.
func (h *Handler) TraerRol(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req traerRolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"hacen falta empresa_origen_id (uuid) y codigo")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if req.EmpresaOrigenID == claims.EmpresaID {
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"ese rol ya es de esta empresa")
		return
	}
	it, copiados, err := h.svc.TraerRol(c.Request.Context(), claims.EmpresaID,
		req.EmpresaOrigenID, req.Codigo, claims.UsuarioID())
	if err != nil {
		h.error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"rol": it, "permisos_copiados": copiados})
}
