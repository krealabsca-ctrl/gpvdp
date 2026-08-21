package bancos

// Endpoints de tesorería: saldos diarios y checklist de carga.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Tesoreria GET /v1/bancos/tesoreria?fecha=YYYY-MM-DD (bancos.ver)
// Saldos del día por cuenta + totales por moneda y banco + serie de 7 días.
// Sin fecha se asume el día de operación de Costa Rica.
func (h *Handler) Tesoreria(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	t, err := h.svc.Tesoreria(c.Request.Context(), claims.EmpresaID, c.Query("fecha"))
	if err != nil {
		h.responderError(c, err, "tesoreria")
		return
	}
	c.JSON(http.StatusOK, t)
}

// guardarSaldosRequest es la captura del día: una fecha y los saldos de las cuentas.
type guardarSaldosRequest struct {
	Fecha  string       `json:"fecha"`
	Saldos []SaldoInput `json:"saldos" binding:"required,min=1,dive"`
}

// GuardarSaldos PUT /v1/bancos/saldos (bancos.saldos)
// Registra o corrige los saldos del día. Es idempotente por cuenta y fecha.
func (h *Handler) GuardarSaldos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req guardarSaldosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido: se espera al menos un saldo")
		return
	}
	n, err := h.svc.GuardarSaldos(c.Request.Context(), claims.EmpresaID, req.Fecha, req.Saldos, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "guardar-saldos")
		return
	}
	c.JSON(http.StatusOK, gin.H{"guardados": n})
}

// CargaDelPeriodo GET /v1/bancos/carga?periodo=YYYY-MM (bancos.ver)
// Checklist de carga de estados de cuenta: qué cuenta está al día y cuál quedó rezagada.
func (h *Handler) CargaDelPeriodo(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	items, err := h.svc.CargaDelPeriodo(c.Request.Context(), claims.EmpresaID, c.Query("periodo"))
	if err != nil {
		h.responderError(c, err, "carga-periodo")
		return
	}
	if items == nil {
		items = []CargaCuenta{}
	}
	c.JSON(http.StatusOK, items)
}
