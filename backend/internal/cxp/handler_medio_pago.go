package cxp

// POST /v1/cxp/documentos/:id/cerrar-otro-medio — la factura ya se pagó por fuera del banco.

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CerrarPorOtroMedio archiva una factura pagada por caja chica, viáticos, tarjeta o efectivo.
//
// La respuesta incluye `salio_de_un_lote`: cerrar una factura que estaba en un corte le quita una
// línea a ese corte, y quien lo hizo tiene que enterarse en el momento, no cuando baje la macro.
func (h *Handler) CerrarPorOtroMedio(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var in MedioPagoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		h.responderError(c, ErrAccionInvalida, "cerrar-otro-medio")
		return
	}
	doc, salioDeUnLote, err := h.svc.CerrarPorOtroMedio(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "cerrar-otro-medio")
		return
	}
	c.JSON(http.StatusOK, gin.H{"documento": doc, "salio_de_un_lote": salioDeUnLote})
}
