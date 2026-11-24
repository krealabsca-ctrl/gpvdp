package cxp

// Endpoints de la aprobación del corte (mig 0123). El porqué está en lote_aprobacion.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// MandarLoteAAprobar POST /v1/cxp/lotes/:id/mandar-a-aprobar (cxp.tesoreria)
//
// Lo pide TESORERÍA: es quien armó el corte y quien dice que ya está listo para que lo miren.
func (h *Handler) MandarLoteAAprobar(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	lote, err := h.svc.MandarLoteAAprobar(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "mandar-lote-a-aprobar")
		return
	}
	c.JSON(http.StatusOK, lote)
}

// AprobarLote POST /v1/cxp/lotes/:id/aprobar (cxp.aprobar_lote)
//
// Permiso PROPIO y separado de `cxp.tesoreria`: quien arma el corte no puede aprobarlo. Esa
// separación es la razón de ser de este paso.
func (h *Handler) AprobarLote(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	lote, err := h.svc.AprobarLote(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "aprobar-lote")
		return
	}
	c.JSON(http.StatusOK, lote)
}

// DevolverLote POST /v1/cxp/lotes/:id/devolver (cxp.aprobar_lote)
func (h *Handler) DevolverLote(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		Motivo string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí qué hay que corregir")
		return
	}
	lote, err := h.svc.DevolverLote(c.Request.Context(), empresaID, c.Param("id"), usuarioID, req.Motivo)
	if err != nil {
		h.responderError(c, err, "devolver-lote")
		return
	}
	c.JSON(http.StatusOK, lote)
}

// ExcluirFacturaDelLote POST /v1/cxp/lotes/:id/excluir/:documentoId (cxp.aprobar_lote)
//
// Saca UNA factura del corte. La factura vuelve a la bandeja en PROGRAMADO, con su aprobación
// intacta: no se deniega, se posterga.
func (h *Handler) ExcluirFacturaDelLote(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		Motivo string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí por qué sale del corte")
		return
	}
	ex, err := h.svc.ExcluirFacturaDelLote(c.Request.Context(), empresaID, c.Param("id"),
		c.Param("documentoId"), usuarioID, req.Motivo)
	if err != nil {
		h.responderError(c, err, "excluir-del-lote")
		return
	}
	c.JSON(http.StatusOK, ex)
}

// ExcluidasDelLote GET /v1/cxp/lotes/:id/excluidas (cxp.ver)
//
// Se LEE con permiso de ver, no de aprobar: quien revisa un corte después —o lo audita— no es
// necesariamente quien tiene la firma.
func (h *Handler) ExcluidasDelLote(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.ExcluidasDelLote(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "excluidas-del-lote")
		return
	}
	if items == nil {
		items = []FacturaExcluida{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
