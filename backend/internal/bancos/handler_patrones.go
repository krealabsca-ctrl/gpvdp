package bancos

// Endpoint del descubridor de patrones.

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Patrones GET /v1/bancos/patrones?periodo=YYYY-MM&limite=N (bancos.ver)
// Grupos de movimientos sin clasificar que comparten forma, con la palabra clave propuesta.
// Sin período, mira toda la empresa (que es como se descubre el patrón grande).
func (h *Handler) Patrones(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	limite, _ := strconv.Atoi(c.Query("limite"))
	items, err := h.svc.Patrones(c.Request.Context(), claims.EmpresaID, c.Query("periodo"), limite)
	if err != nil {
		h.responderError(c, err, "patrones")
		return
	}
	c.JSON(http.StatusOK, items)
}
