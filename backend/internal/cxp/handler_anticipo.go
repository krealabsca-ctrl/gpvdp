package cxp

// Netting de anticipos: billetera del proveedor, aplicar y reversar contra una factura.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// AnticiposDisponibles GET /v1/cxp/anticipos/disponibles?proveedor_id= — billetera del proveedor.
func (h *Handler) AnticiposDisponibles(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	prov := c.Query("proveedor_id")
	if prov == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "proveedor_id es requerido")
		return
	}
	items, err := h.svc.AnticiposDisponibles(c.Request.Context(), empresaID, prov)
	if err != nil {
		h.responderError(c, err, "anticipos-disponibles")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// AnticiposEmpresa GET /v1/cxp/anticipos — billetera global de anticipos con saldo.
func (h *Handler) AnticiposEmpresa(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.AnticiposEmpresa(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "anticipos-empresa")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// AplicacionesDocumento GET /v1/cxp/documentos/:id/anticipos — anticipos aplicados a la factura.
func (h *Handler) AplicacionesDocumento(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.AplicacionesDeFactura(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "aplicaciones-documento")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type aplicarAnticipoRequest struct {
	AnticipoID string `json:"anticipo_id" validate:"required,uuid"`
	Monto      string `json:"monto" validate:"required"`
}

// AplicarAnticipoDocumento POST /v1/cxp/documentos/:id/anticipos — netea un anticipo a la factura.
func (h *Handler) AplicarAnticipoDocumento(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req aplicarAnticipoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	doc, err := h.svc.AplicarAnticipo(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.AnticipoID, req.Monto, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "aplicar-anticipo")
		return
	}
	c.JSON(http.StatusOK, doc)
}

type aplicarLoteRequest struct {
	Aplicaciones []struct {
		AnticipoID string `json:"anticipo_id" validate:"required,uuid"`
		Monto      string `json:"monto" validate:"required"`
	} `json:"aplicaciones" validate:"required,min=1,dive"`
}

// AplicarAnticiposLoteDocumento POST /v1/cxp/documentos/:id/anticipos/lote — aplica varios
// anticipos a la factura en una sola operación (todo-o-nada).
func (h *Handler) AplicarAnticiposLoteDocumento(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req aplicarLoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	lineas := make([]AplicacionInput, 0, len(req.Aplicaciones))
	for _, a := range req.Aplicaciones {
		m, err := decimal.NewFromString(a.Monto)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "monto inválido")
			return
		}
		lineas = append(lineas, AplicacionInput{AnticipoID: a.AnticipoID, Monto: m})
	}
	doc, err := h.svc.AplicarAnticiposLote(c.Request.Context(), claims.EmpresaID, c.Param("id"), lineas, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "aplicar-anticipos-lote")
		return
	}
	c.JSON(http.StatusOK, doc)
}

// ReversarAnticipoDocumento DELETE /v1/cxp/documentos/:id/anticipos/:aplicacionId — deshace una aplicación.
func (h *Handler) ReversarAnticipoDocumento(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	doc, err := h.svc.ReversarAplicacion(c.Request.Context(), claims.EmpresaID, c.Param("id"), c.Param("aplicacionId"), claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "reversar-anticipo")
		return
	}
	c.JSON(http.StatusOK, doc)
}
