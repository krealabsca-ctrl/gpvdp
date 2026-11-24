package conta

// La capa HTTP del mapeo contable (mig 0110, Fase 2).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Mapeo GET /v1/conta/mapeo?pendientes=1&con_movimientos=1 (conta.mapeo)
//
// Devuelve también la cobertura en la misma respuesta: es el número que dice cuánto falta, y
// pedirlo aparte haría que la lista y el contador puedan venir de instantes distintos.
func (h *Handler) Mapeo(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	filas, err := h.svc.Mapeo(c.Request.Context(), cl.EmpresaID,
		c.Query("pendientes") == "1", c.Query("con_movimientos") == "1")
	if err != nil {
		h.responder(c, err, "mapeo")
		return
	}
	cob, err := h.svc.Cobertura(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err, "cobertura")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": filas, "cobertura": cob})
}

// Decidir POST /v1/conta/mapeo (conta.mapeo)
//
// Acepta una decisión o varias. El lote existe porque el trabajo viene en familias: las 23
// clasificaciones de caja chica van todas a cuentas de caja chica, y resolverlas de a una, con
// 332 por delante, es lo que hace que el mapeo no se termine nunca.
func (h *Handler) Decidir(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req struct {
		Decisiones []DecisionDeMapeo `json:"decisiones"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Decisiones) == 0 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "mandá al menos una decisión")
		return
	}
	res, err := h.svc.DecidirVarias(c.Request.Context(), cl.EmpresaID, req.Decisiones, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "decidir")
		return
	}
	c.JSON(http.StatusOK, res)
}

// Deshacer DELETE /v1/conta/mapeo/:clasificacionID (conta.mapeo)
func (h *Handler) Deshacer(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.Deshacer(c.Request.Context(), cl.EmpresaID, c.Param("id"), cl.UsuarioID()); err != nil {
		h.responder(c, err, "deshacer-mapeo")
		return
	}
	c.Status(http.StatusNoContent)
}
