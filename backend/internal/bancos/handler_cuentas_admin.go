package bancos

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Bancos GET /v1/bancos/catalogo/bancos
func (h *Handler) Bancos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	list, err := h.svc.Bancos(c.Request.Context(), claims.EmpresaID, c.Query("incluir_inactivos") == "true")
	if err != nil {
		h.responderError(c, err, "bancos")
		return
	}
	if list == nil {
		list = []BancoItem{}
	}
	c.JSON(http.StatusOK, list)
}

type nombreRequest struct {
	Nombre string `json:"nombre" validate:"required"`
}

// CrearBanco POST /v1/bancos/catalogo/bancos
func (h *Handler) CrearBanco(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	req, ok := bindNombre(c)
	if !ok {
		return
	}
	b, err := h.svc.CrearBanco(c.Request.Context(), claims.EmpresaID, req.Nombre, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-banco")
		return
	}
	c.JSON(http.StatusCreated, b)
}

// RenombrarBanco PATCH /v1/bancos/catalogo/bancos/:id
func (h *Handler) RenombrarBanco(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	req, ok := bindNombre(c)
	if !ok {
		return
	}
	if err := h.svc.RenombrarBanco(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Nombre, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "renombrar-banco")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type crearCuentaRequest struct {
	BancoID string `json:"banco_id" validate:"required,uuid"`
	Alias   string `json:"alias" validate:"required"`
	IBAN    string `json:"iban"`
	Moneda  string `json:"moneda" validate:"required,oneof=CRC USD"`
}

// CrearCuenta POST /v1/bancos/catalogo/cuentas
func (h *Handler) CrearCuenta(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req crearCuentaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	cuenta, err := h.svc.CrearCuenta(c.Request.Context(), claims.EmpresaID, req.BancoID, req.Alias, req.IBAN, req.Moneda, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-cuenta")
		return
	}
	c.JSON(http.StatusCreated, cuenta)
}

// RenombrarCuenta PATCH /v1/bancos/catalogo/cuentas/:id
func (h *Handler) RenombrarCuenta(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	req, ok := bindAlias(c)
	if !ok {
		return
	}
	if err := h.svc.RenombrarCuenta(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Alias, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "renombrar-cuenta")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type aliasRequest struct {
	Alias string `json:"alias" validate:"required"`
}

func bindNombre(c *gin.Context) (nombreRequest, bool) {
	var req nombreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return nombreRequest{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return nombreRequest{}, false
	}
	return req, true
}

func bindAlias(c *gin.Context) (aliasRequest, bool) {
	var req aliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return aliasRequest{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return aliasRequest{}, false
	}
	return req, true
}
