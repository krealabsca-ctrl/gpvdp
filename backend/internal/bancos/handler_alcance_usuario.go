package bancos

// Los endpoints de «qué ve» una persona. El porqué está en alcance_usuario.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gpvdp/erp/internal/httpx"
)

// AlcanceDeUsuario GET /v1/bancos/usuarios/:id/alcance (admin.roles)
func (h *Handler) AlcanceDeUsuario(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	res, err := h.svc.AlcanceDeUnUsuario(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "alcance-usuario")
		return
	}
	c.JSON(http.StatusOK, res)
}

type guardarAreasReq struct {
	// Sin `required`: una lista vacía es la forma de QUITARLE todas las áreas a alguien, y es una
	// operación legítima —alguien que cambia de puesto—, no un formulario incompleto.
	Areas []AreaAsignada `json:"areas"`
}

// GuardarAreasDeUsuario PUT /v1/bancos/usuarios/:id/alcance/areas (admin.roles)
//
// PUT y no POST: reemplaza la lista entera, que es lo que la pantalla manda.
func (h *Handler) GuardarAreasDeUsuario(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req guardarAreasReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"cada área necesita su departamento_id")
		return
	}
	if err := h.svc.GuardarAreasDeUnUsuario(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Areas); err != nil {
		h.responderError(c, err, "guardar-areas-usuario")
		return
	}
	res, err := h.svc.AlcanceDeUnUsuario(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "alcance-usuario")
		return
	}
	c.JSON(http.StatusOK, res)
}

type guardarPartidasReq struct {
	// Igual que arriba: vacío significa «ninguna», no «faltó el campo».
	ClasificacionIDs []string `json:"clasificacion_ids"`
}

// GuardarPartidasDeUsuario PUT /v1/bancos/usuarios/:id/alcance/partidas (admin.roles)
//
// SEPARADO del de áreas, y no un solo PUT con las dos listas: son dos actos con consecuencias
// distintas —las áreas también reparten poder de aprobación en CxP— y un PUT conjunto haría que
// guardar las partidas reescriba las áreas de paso.
func (h *Handler) GuardarPartidasDeUsuario(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req guardarPartidasReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"se esperaba la lista clasificacion_ids")
		return
	}
	if err := h.svc.GuardarPartidasDeIngresoDeUnUsuario(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.ClasificacionIDs); err != nil {
		h.responderError(c, err, "guardar-partidas-usuario")
		return
	}
	res, err := h.svc.AlcanceDeUnUsuario(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "alcance-usuario")
		return
	}
	c.JSON(http.StatusOK, res)
}
