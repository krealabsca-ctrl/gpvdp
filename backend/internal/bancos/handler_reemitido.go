package bancos

// Endpoints para enlazar dos movimientos que el banco reemitió. El porqué está en reemitido.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// CandidatosReemision GET /v1/bancos/movimientos/reemisiones/candidatos (bancos.ver_clasificar)
//
// Propone pares, no enlaza. Se lee con el permiso de VER por la misma razón que la lista de
// excluidos: quien cierra el mes tiene que poder mirar qué se está proponiendo aunque no sea quien
// decide.
func (h *Handler) CandidatosReemision(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	items, err := h.svc.CandidatosReemision(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "candidatos-reemision")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "dias_max": diasMaxCandidatosReemision})
}

// Reemisiones GET /v1/bancos/movimientos/reemisiones (bancos.ver_clasificar)
func (h *Handler) Reemisiones(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	items, err := h.svc.Reemisiones(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "reemisiones")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// EnlazarReemision POST /v1/bancos/movimientos/:id/reemplaza (bancos.excluir_movimiento)
//
// El `:id` de la ruta es el que SE QUEDA en los libros; el del cuerpo es el que sale. Se escribe así
// y no al revés porque el enlace vive en la fila del que queda (`reemplaza_a`), y porque la pantalla
// siempre parte del movimiento que el usuario eligió conservar.
//
// Mismo permiso que excluir: enlazar saca plata de los libros, que es exactamente la misma autoridad.
func (h *Handler) EnlazarReemision(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req struct {
		ReemplazaA string `json:"reemplaza_a"`
		Motivo     string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"indicá cuál movimiento se reemplaza y por qué")
		return
	}
	if req.ReemplazaA == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"indicá cuál movimiento sale de los libros")
		return
	}
	err := h.svc.EnlazarReemision(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		req.ReemplazaA, req.Motivo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "enlazar-reemision")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeshacerReemision POST /v1/bancos/movimientos/:id/deshacer-reemplazo (bancos.excluir_movimiento)
//
// El `:id` es el que quedó vigente. Devuelve a los libros el que había salido.
func (h *Handler) DeshacerReemision(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	err := h.svc.DeshacerReemision(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "deshacer-reemision")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
