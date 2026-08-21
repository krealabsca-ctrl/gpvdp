package cxp

// Caja chica (fondo fijo): fondos, vales y reposición.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

type fondoRequest struct {
	Nombre         string `json:"nombre" validate:"required,max=120"`
	CustodioID     string `json:"custodio_id" validate:"omitempty,uuid"`
	DepartamentoID string `json:"departamento_id" validate:"omitempty,uuid"`
	ProveedorID    string `json:"proveedor_id" validate:"omitempty,uuid"`
	MontoAsignado  string `json:"monto_asignado" validate:"required"`
	UmbralPct      string `json:"umbral_pct"`
	LimiteVale     string `json:"limite_vale"`
}

func (req fondoRequest) aInput() (FondoInput, error) {
	monto, err := decimal.NewFromString(req.MontoAsignado)
	if err != nil || !monto.IsPositive() {
		return FondoInput{}, errTCInvalido
	}
	umbral, err := decOrZero(req.UmbralPct)
	if err != nil {
		return FondoInput{}, err
	}
	if umbral.IsZero() {
		umbral = decimal.NewFromInt(30) // default de la maqueta: alerta al 30% disponible
	}
	limite, err := decOrZero(req.LimiteVale)
	if err != nil {
		return FondoInput{}, err
	}
	return FondoInput{
		Nombre: req.Nombre, CustodioID: req.CustodioID, DepartamentoID: req.DepartamentoID,
		ProveedorID: req.ProveedorID, MontoAsignado: monto, UmbralPct: umbral, LimiteVale: limite,
	}, nil
}

// ListarFondos GET /v1/cxp/cajas — fondos visibles (todos, o solo el del custodio).
func (h *Handler) ListarFondos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	items, err := h.svc.ListarFondos(c.Request.Context(), claims.EmpresaID, claims.Rol, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "listar-fondos")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// CrearFondo POST /v1/cxp/cajas (cxp.caja_administrar).
func (h *Handler) CrearFondo(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req fondoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	in, err := req.aInput()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "montos inválidos (monto_asignado > 0)")
		return
	}
	f, err := h.svc.CrearFondo(c.Request.Context(), claims.EmpresaID, in, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-fondo")
		return
	}
	c.JSON(http.StatusCreated, f)
}

// ActualizarFondo PATCH /v1/cxp/cajas/:id (cxp.caja_administrar).
func (h *Handler) ActualizarFondo(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req fondoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	in, err := req.aInput()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "montos inválidos")
		return
	}
	f, err := h.svc.ActualizarFondo(c.Request.Context(), claims.EmpresaID, c.Param("id"), in, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "actualizar-fondo")
		return
	}
	c.JSON(http.StatusOK, f)
}

// DesactivarFondo POST /v1/cxp/cajas/:id/desactivar (cxp.caja_administrar).
func (h *Handler) DesactivarFondo(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.DesactivarFondo(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "desactivar-fondo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListarVales GET /v1/cxp/cajas/:id/vales — vales del fondo con estado derivado.
func (h *Handler) ListarVales(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	items, err := h.svc.ListarVales(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.Rol, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "listar-vales")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type valeRequest struct {
	Fecha              string `json:"fecha" validate:"omitempty,datetime=2006-01-02"`
	Detalle            string `json:"detalle" validate:"required,max=300"`
	MontoCRC           string `json:"monto_crc" validate:"required"`
	ConceptoID         string `json:"concepto_id" validate:"required,uuid"`
	ClasificacionID    string `json:"clasificacion_id" validate:"required,uuid"`
	SubclasificacionID string `json:"subclasificacion_id" validate:"omitempty,uuid"`
	Comprobante        string `json:"comprobante" validate:"omitempty,oneof=FE RECIBO"`
}

// CrearVale POST /v1/cxp/cajas/:id/vales (cxp.caja_vale, custodio o Conta).
func (h *Handler) CrearVale(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req valeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	monto, err := decimal.NewFromString(req.MontoCRC)
	if err != nil || !monto.IsPositive() {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "monto_crc inválido")
		return
	}
	id, err := h.svc.CrearVale(c.Request.Context(), claims.EmpresaID, c.Param("id"), ValeInput{
		Fecha: req.Fecha, Detalle: req.Detalle, MontoCRC: monto,
		ConceptoID: req.ConceptoID, ClasificacionID: req.ClasificacionID,
		SubclasificacionID: req.SubclasificacionID, Comprobante: req.Comprobante,
	}, claims.Rol, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-vale")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "ok": true})
}

// AnularVale POST /v1/cxp/cajas/:id/vales/:valeId/anular (cxp.caja_vale).
func (h *Handler) AnularVale(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.AnularVale(c.Request.Context(), claims.EmpresaID, c.Param("id"), c.Param("valeId"), claims.Rol, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "anular-vale")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GenerarReposicion POST /v1/cxp/cajas/:id/reposicion (cxp.caja_reponer) — agrupa los vales
// pendientes en un documento REINTEGRO y lo devuelve (entra al flujo normal de CxP).
func (h *Handler) GenerarReposicion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	doc, err := h.svc.GenerarReposicion(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.Rol, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "generar-reposicion")
		return
	}
	c.JSON(http.StatusCreated, doc)
}
