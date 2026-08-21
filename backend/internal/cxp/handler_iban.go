package cxp

// Endpoints de la carga de IBAN de proveedores.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

type filasIBANRequest struct {
	Filas []FilaIBAN `json:"filas"`
}

// PrevisualizarIBAN POST /v1/cxp/proveedores/iban/preview — dice qué pasaría, sin escribir nada.
func (h *Handler) PrevisualizarIBAN(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req filasIBANRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Filas) == 0 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta al menos una fila")
		return
	}
	res, err := h.svc.PrevisualizarIBAN(c.Request.Context(), empresaID, req.Filas)
	if err != nil {
		h.responderError(c, err, "preview-iban")
		return
	}
	c.JSON(http.StatusOK, res)
}

// CargarIBAN POST /v1/cxp/proveedores/iban — guarda las filas válidas de la carga.
func (h *Handler) CargarIBAN(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req filasIBANRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Filas) == 0 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta al menos una fila")
		return
	}
	n, err := h.svc.CargarIBAN(c.Request.Context(), empresaID, req.Filas, usuarioID)
	if err != nil {
		h.responderError(c, err, "cargar-iban")
		return
	}
	c.JSON(http.StatusOK, gin.H{"actualizados": n})
}

// ProveedoresSinIBAN GET /v1/cxp/proveedores/sin-iban — los que no se pueden pagar todavía.
//
// Es la lista de trabajo: sin IBAN la línea de la macro la rechaza el banco, así que esto dice
// exactamente a quién hay que pedirle la cuenta antes de la próxima corrida.
func (h *Handler) ProveedoresSinIBAN(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	provs, err := h.svc.ProveedoresSinIBAN(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "proveedores-sin-iban")
		return
	}
	c.JSON(http.StatusOK, gin.H{"proveedores": provs, "total": len(provs)})
}
