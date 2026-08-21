package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// ListarDepartamentos GET /v1/cxp/departamentos?activos=1
// Por defecto trae todos (para la administración); activos=1 filtra a solo activos
// (para poblar selects de proveedor, filtros y enrutamiento).
func (h *Handler) ListarDepartamentos(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	soloActivos := c.Query("activos") == "1" || c.Query("activos") == "true"
	lista, err := h.svc.Departamentos(c.Request.Context(), empresaID, soloActivos)
	if err != nil {
		h.responderError(c, err, "listar-departamentos")
		return
	}
	c.JSON(http.StatusOK, lista)
}

type departamentoRequest struct {
	Nombre      string `json:"nombre" validate:"required,max=60"`
	Codigo      string `json:"codigo" validate:"omitempty,max=12"`
	CentroCosto string `json:"centro_costo" validate:"omitempty,max=24"`
}

func bindDepartamento(c *gin.Context) (DepartamentoInput, bool) {
	var req departamentoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return DepartamentoInput{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return DepartamentoInput{}, false
	}
	return DepartamentoInput{Nombre: req.Nombre, Codigo: req.Codigo, CentroCosto: req.CentroCosto}, true
}

// CrearDepartamento POST /v1/cxp/departamentos
func (h *Handler) CrearDepartamento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindDepartamento(c)
	if !ok {
		return
	}
	d, err := h.svc.CrearDepartamento(c.Request.Context(), empresaID, in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-departamento")
		return
	}
	c.JSON(http.StatusCreated, d)
}

// ActualizarDepartamento PATCH /v1/cxp/departamentos/:id
func (h *Handler) ActualizarDepartamento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindDepartamento(c)
	if !ok {
		return
	}
	d, err := h.svc.ActualizarDepartamento(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "actualizar-departamento")
		return
	}
	c.JSON(http.StatusOK, d)
}

// DesactivarDepartamento POST /v1/cxp/departamentos/:id/desactivar
func (h *Handler) DesactivarDepartamento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.DesactivarDepartamento(c.Request.Context(), empresaID, c.Param("id"), usuarioID); err != nil {
		h.responderError(c, err, "desactivar-departamento")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
