package bancos

// Endpoint del barrido de huellas Bancos↔CxP.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// ConciliarCxP POST /v1/bancos/conciliacion-cxp (cxp.tesoreria)
// Barre los movimientos que traen la huella de CxP y empareja cada uno con su pago. Corre solo
// al importar; este endpoint sirve para repetirlo sobre lo ya cargado.
//
// El permiso es de tesorería de CxP porque la acción cambia el estado de un pago.
func (h *Handler) ConciliarCxP(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	rep, err := h.svc.ConciliarCxP(c.Request.Context(), claims.EmpresaID,
		c.Query("importacion_id"), claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "conciliar-cxp")
		return
	}
	c.JSON(http.StatusOK, rep)
}
