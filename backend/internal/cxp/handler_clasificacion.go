package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

type clasificarRequest struct {
	ConceptoID         string `json:"concepto_id" validate:"omitempty,uuid"`
	ClasificacionID    string `json:"clasificacion_id" validate:"omitempty,uuid"`
	SubclasificacionID string `json:"subclasificacion_id" validate:"omitempty,uuid"`
}

// ClasificarDocumento PATCH /v1/cxp/documentos/:id/clasificacion — asigna gasto a un documento.
func (h *Handler) ClasificarDocumento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req clasificarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	d, err := h.svc.ClasificarDocumento(c.Request.Context(), empresaID, c.Param("id"), req.ConceptoID, req.ClasificacionID, req.SubclasificacionID, usuarioID)
	if err != nil {
		h.responderError(c, err, "clasificar-documento")
		return
	}
	c.JSON(http.StatusOK, d)
}

type clasificarMasivoRequest struct {
	IDs                []string `json:"ids" validate:"required,min=1,max=500,dive,uuid"`
	ConceptoID         string   `json:"concepto_id" validate:"omitempty,uuid"`
	ClasificacionID    string   `json:"clasificacion_id" validate:"omitempty,uuid"`
	SubclasificacionID string   `json:"subclasificacion_id" validate:"omitempty,uuid"`
}

// ClasificarMasivo POST /v1/cxp/documentos/clasificar-masivo — misma clasificación a un lote.
func (h *Handler) ClasificarMasivo(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req clasificarMasivoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.ClasificarMasivo(c.Request.Context(), empresaID, usuarioID, req.IDs, req.ConceptoID, req.ClasificacionID, req.SubclasificacionID)
	if err != nil {
		h.responderError(c, err, "clasificar-masivo")
		return
	}
	c.JSON(http.StatusOK, res)
}

type prioridadMasivaRequest struct {
	IDs       []string `json:"ids" validate:"required,min=1,max=500,dive,uuid"`
	Prioridad string   `json:"prioridad" validate:"omitempty,oneof=A AA"`
}

// PrioridadMasiva POST /v1/cxp/documentos/prioridad-masiva — AA (sí o sí) / A / "" (normal).
func (h *Handler) PrioridadMasiva(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req prioridadMasivaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.AsignarPrioridadMasivo(c.Request.Context(), empresaID, usuarioID, req.IDs, req.Prioridad)
	if err != nil {
		h.responderError(c, err, "prioridad-masiva")
		return
	}
	c.JSON(http.StatusOK, res)
}

// GastosDeProveedor GET /v1/cxp/proveedores/:id/gastos — categorías frecuentes del proveedor.
func (h *Handler) GastosDeProveedor(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	gastos, err := h.svc.GastosFrecuentes(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "gastos-proveedor")
		return
	}
	if gastos == nil {
		gastos = []GastoFrecuente{}
	}
	c.JSON(http.StatusOK, gastos)
}

type tipoMasivoRequest struct {
	IDs  []string `json:"ids" validate:"required,min=1,max=500,dive,uuid"`
	Tipo string   `json:"tipo" validate:"required,oneof=CXP ANTICIPO VIATICOS REINTEGRO"`
}

// TipoMasivo POST /v1/cxp/documentos/tipo-masivo — marca el tipo de factura de un lote.
func (h *Handler) TipoMasivo(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req tipoMasivoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.AsignarTipoMasivo(c.Request.Context(), empresaID, usuarioID, req.IDs, req.Tipo)
	if err != nil {
		h.responderError(c, err, "tipo-masivo")
		return
	}
	c.JSON(http.StatusOK, res)
}
