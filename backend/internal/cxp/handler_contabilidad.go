package cxp

// Handlers de la marca «de Contabilidad».

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// marcarDocRequest usa un PUNTERO en `es_contabilidad` a propósito: el campo tiene tres estados
// (true / false / ausente) y con un bool plano «ausente» y «false» llegarían iguales, así que no
// habría forma de pedir «volvé a heredar del proveedor o del rubro».
type marcarDocRequest struct {
	EsContabilidad *bool  `json:"es_contabilidad"`
	Motivo         string `json:"motivo" validate:"omitempty,max=300"`
}

// MarcarDocumentoContabilidad PATCH /v1/cxp/documentos/:id/contabilidad
func (h *Handler) MarcarDocumentoContabilidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req marcarDocRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	d, err := h.svc.MarcarDocumentoContabilidad(c.Request.Context(), empresaID, c.Param("id"), req.EsContabilidad, req.Motivo, usuarioID)
	if err != nil {
		h.responderError(c, err, "marcar-documento-contabilidad")
		return
	}
	c.JSON(http.StatusOK, d)
}

// marcarRequest es la marca de catálogo/proveedor: acá sí es un booleano simple (marcado o no).
type marcarRequest struct {
	EsContabilidad bool `json:"es_contabilidad"`
}

func (h *Handler) bindMarca(c *gin.Context) (bool, bool) {
	var req marcarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return false, false
	}
	return req.EsContabilidad, true
}

// MarcarProveedorContabilidad PATCH /v1/cxp/proveedores/:id/contabilidad
func (h *Handler) MarcarProveedorContabilidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	valor, ok := h.bindMarca(c)
	if !ok {
		return
	}
	if err := h.svc.MarcarProveedorContabilidad(c.Request.Context(), empresaID, c.Param("id"), valor, usuarioID); err != nil {
		h.responderError(c, err, "marcar-proveedor-contabilidad")
		return
	}
	c.Status(http.StatusNoContent)
}

// MarcarConceptoContabilidad PATCH /v1/cxp/contabilidad/conceptos/:id
func (h *Handler) MarcarConceptoContabilidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	valor, ok := h.bindMarca(c)
	if !ok {
		return
	}
	if err := h.svc.MarcarConceptoContabilidad(c.Request.Context(), empresaID, c.Param("id"), valor, usuarioID); err != nil {
		h.responderError(c, err, "marcar-concepto-contabilidad")
		return
	}
	c.Status(http.StatusNoContent)
}

// MarcarClasificacionContabilidad PATCH /v1/cxp/contabilidad/clasificaciones/:id
func (h *Handler) MarcarClasificacionContabilidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	valor, ok := h.bindMarca(c)
	if !ok {
		return
	}
	if err := h.svc.MarcarClasificacionContabilidad(c.Request.Context(), empresaID, c.Param("id"), valor, usuarioID); err != nil {
		h.responderError(c, err, "marcar-clasificacion-contabilidad")
		return
	}
	c.Status(http.StatusNoContent)
}

// AprobarDocumentoContabilidad POST /v1/cxp/documentos/:id/aprobar-contabilidad
//
// Vía propia para las facturas marcadas «de Contabilidad»: la ruta normal de aprobar exige
// `cxp.aprobar` y el Supervisor Financiero no lo tiene. La matriz de firmas por monto se aplica
// igual; lo único que se salta es la validación de área.
func (h *Handler) AprobarDocumentoContabilidad(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	d, err := h.svc.AprobarContabilidad(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID(), claims.Rol)
	if err != nil {
		h.responderError(c, err, "aprobar-documento-contabilidad")
		return
	}
	c.JSON(http.StatusOK, d)
}

// MarcasContabilidad GET /v1/cxp/contabilidad/marcas — el cuadro de lo marcado hoy.
func (h *Handler) MarcasContabilidad(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	m, err := h.svc.MarcasContabilidad(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "marcas-contabilidad")
		return
	}
	c.JSON(http.StatusOK, m)
}
