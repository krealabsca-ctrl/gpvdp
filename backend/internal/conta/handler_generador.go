package conta

// La capa HTTP del generador de asientos (Fase 3B).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// PrevisualizarGeneracion GET /v1/conta/generar?periodo=YYYY-MM (conta.generar)
//
// NO ESCRIBE NADA: dice qué pasaría. Mismo criterio que el importador de bancos y la carga del
// plan de cuentas, y acá pesa más: generar toca miles de movimientos de una vez.
func (h *Handler) PrevisualizarGeneracion(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.PrevisualizarGeneracion(c.Request.Context(), cl.EmpresaID, c.Query("periodo"))
	if err != nil {
		h.responder(c, err, "previsualizar-generacion")
		return
	}
	c.JSON(http.StatusOK, res)
}

// Generar POST /v1/conta/generar (conta.generar)
func (h *Handler) Generar(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req struct {
		Periodo string `json:"periodo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí de qué período")
		return
	}
	res, err := h.svc.Generar(c.Request.Context(), cl.EmpresaID, req.Periodo, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "generar-asientos")
		return
	}
	c.JSON(http.StatusOK, res)
}

// RevertirGeneracion POST /v1/conta/generar/revertir (conta.generar)
//
// Anula, no borra: el asiento queda en el libro sin efecto y con su motivo. Como el índice de
// idempotencia excluye los anulados, después se puede volver a generar el período corregido.
func (h *Handler) RevertirGeneracion(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req struct {
		Periodo string `json:"periodo"`
		Motivo  string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí qué período y por qué")
		return
	}
	res, err := h.svc.Revertir(c.Request.Context(), cl.EmpresaID, req.Periodo, req.Motivo, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "revertir-generacion")
		return
	}
	c.JSON(http.StatusOK, res)
}
