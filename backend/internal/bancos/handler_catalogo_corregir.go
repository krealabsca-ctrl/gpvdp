package bancos

// HTTP de las correcciones del catálogo: eliminar o desactivar bancos y cuentas, corregir
// la moneda de una cuenta, y fusionar conceptos o clasificaciones.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// EliminarBanco DELETE /v1/bancos/catalogo/bancos/:id
func (h *Handler) EliminarBanco(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.EliminarBanco(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "eliminar-banco")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type activoRequest struct {
	// Puntero para distinguir «no vino» de «vino false».
	Activo *bool `json:"activo" binding:"required"`
}

// CambiarActivoBanco POST /v1/bancos/catalogo/bancos/:id/activo
func (h *Handler) CambiarActivoBanco(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req activoRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Activo == nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "falta el campo «activo»")
		return
	}
	if err := h.svc.CambiarActivoBanco(c.Request.Context(), claims.EmpresaID, c.Param("id"), *req.Activo, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "activo-banco")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UsoDeCuenta GET /v1/bancos/catalogo/cuentas/:id/uso — qué cuelga de la cuenta.
// La pantalla lo consulta ANTES de ofrecer eliminar: así el usuario ve por qué no puede.
func (h *Handler) UsoDeCuenta(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	uso, err := h.svc.UsoDeCuenta(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "uso-cuenta")
		return
	}
	c.JSON(http.StatusOK, uso)
}

// EliminarCuenta DELETE /v1/bancos/catalogo/cuentas/:id
func (h *Handler) EliminarCuenta(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.EliminarCuenta(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "eliminar-cuenta")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CambiarActivoCuenta POST /v1/bancos/catalogo/cuentas/:id/activo
func (h *Handler) CambiarActivoCuenta(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req activoRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Activo == nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "falta el campo «activo»")
		return
	}
	if err := h.svc.CambiarActivoCuenta(c.Request.Context(), claims.EmpresaID, c.Param("id"), *req.Activo, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "activo-cuenta")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// actualizarCuentaRequest: campos opcionales. Lo que no viene, no se toca.
type actualizarCuentaRequest struct {
	Alias   *string `json:"alias"`
	IBAN    *string `json:"iban"`
	Moneda  *string `json:"moneda" validate:"omitempty,oneof=CRC USD"`
	BancoID *string `json:"banco_id" validate:"omitempty,uuid"`
}

// ActualizarCuenta PATCH /v1/bancos/catalogo/cuentas/:id — alias, banco, IBAN o moneda.
// La moneda y el IBAN solo se pueden corregir si la cuenta todavía no tiene movimientos.
func (h *Handler) ActualizarCuenta(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req actualizarCuentaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if req.Alias == nil && req.IBAN == nil && req.Moneda == nil && req.BancoID == nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "nada que actualizar")
		return
	}
	cambio := CambioDeCuenta{Alias: req.Alias, IBAN: req.IBAN, Moneda: req.Moneda, BancoID: req.BancoID}
	if err := h.svc.ActualizarCuenta(c.Request.Context(), claims.EmpresaID, c.Param("id"), cambio, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "actualizar-cuenta")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type fusionRequest struct {
	DestinoID string `json:"destino_id" validate:"required,uuid"`
	// ConfirmarCambioDeConcepto: al fusionar clasificaciones de conceptos distintos, los
	// movimientos cambian de concepto y con eso cambia el cuadre. Se exige decirlo.
	ConfirmarCambioDeConcepto bool `json:"confirmar_cambio_de_concepto"`
}

// FusionarConcepto POST /v1/bancos/catalogo/conceptos/:id/fusionar
// Mueve TODO lo del concepto al destino y borra el origen. Irreversible.
func (h *Handler) FusionarConcepto(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req fusionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.FusionarConceptos(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.DestinoID, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "fusionar-concepto")
		return
	}
	c.JSON(http.StatusOK, res)
}

// FusionarClasificacion POST /v1/bancos/catalogo/clasificaciones/:id/fusionar
func (h *Handler) FusionarClasificacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req fusionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.FusionarClasificaciones(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		req.DestinoID, req.ConfirmarCambioDeConcepto, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "fusionar-clasificacion")
		return
	}
	c.JSON(http.StatusOK, res)
}
