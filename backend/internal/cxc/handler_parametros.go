package cxc

// HTTP de la configuración del módulo: parámetros, tramos, factores por forma de pago,
// sedes y la asignación de sedes por usuario (la frontera de datos).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// Config GET /v1/cxc/parametros — todo lo configurable en una sola llamada, con la nota de
// qué se puede cambiar hoy y qué no (y por qué).
func (h *Handler) Config(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	cfg, err := h.svc.Config(c.Request.Context(), empresaID)
	if err != nil {
		h.error(c, err, "config")
		return
	}
	c.JSON(http.StatusOK, cfg)
}

type guardarParametrosRequest struct {
	Valores map[string]string `json:"valores" binding:"required"`
}

// GuardarParametros PUT /v1/cxc/parametros — upsert de las claves recibidas.
func (h *Handler) GuardarParametros(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req guardarParametrosRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Valores) == 0 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "mandá al menos un parámetro en «valores»")
		return
	}
	n, err := h.svc.GuardarParametros(c.Request.Context(), empresaID, req.Valores, usuarioID)
	if err != nil {
		h.error(c, err, "guardar-parametros")
		return
	}
	c.JSON(http.StatusOK, gin.H{"cambiados": n})
}

type tramoRequest struct {
	Prob          *string `json:"prob_recuperacion"`
	Estrategia    *string `json:"estrategia"`
	CanalSugerido *string `json:"canal_sugerido"`
	DiasMin       *int    `json:"dias_min"`
	DiasMax       *int    `json:"dias_max"`
}

// ActualizarTramo PATCH /v1/cxc/tramos/:codigo — probabilidad, estrategia, canal o rango.
// La probabilidad multiplica el valor esperado: cambiarla reordena la cola de cobro.
func (h *Handler) ActualizarTramo(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req tramoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	err := h.svc.ActualizarTramo(c.Request.Context(), empresaID, c.Param("codigo"), CambioTramo{
		Prob: req.Prob, Estrategia: req.Estrategia, CanalSugerido: req.CanalSugerido,
		DiasMin: req.DiasMin, DiasMax: req.DiasMax,
	}, usuarioID)
	if err != nil {
		h.error(c, err, "actualizar-tramo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type formaPagoRequest struct {
	Factor *string `json:"factor_recuperacion"`
	Activa *bool   `json:"activa"`
}

// ActualizarFormaPago PATCH /v1/cxc/formas-pago/:id — el factor de recuperación del canal.
func (h *Handler) ActualizarFormaPago(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req formaPagoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := h.svc.ActualizarFormaPago(c.Request.Context(), empresaID, c.Param("id"), req.Factor, req.Activa, usuarioID); err != nil {
		h.error(c, err, "actualizar-forma-pago")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type sedeRequest struct {
	Nombre      *string `json:"nombre"`
	RazonSocial string  `json:"razon_social"`
	Plaza       string  `json:"plaza"`
	Activa      *bool   `json:"activa"`
}

// CrearSede POST /v1/cxc/sedes
func (h *Handler) CrearSede(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req sedeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Nombre == nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "falta el nombre de la sede")
		return
	}
	sede, err := h.svc.CrearSede(c.Request.Context(), empresaID, *req.Nombre, req.RazonSocial, req.Plaza, usuarioID)
	if err != nil {
		h.error(c, err, "crear-sede")
		return
	}
	c.JSON(http.StatusCreated, sede)
}

// ActualizarSede PATCH /v1/cxc/sedes/:id — renombrar o activar/desactivar.
func (h *Handler) ActualizarSede(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req sedeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if req.Nombre == nil && req.Activa == nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "nada que actualizar")
		return
	}
	if err := h.svc.ActualizarSede(c.Request.Context(), empresaID, c.Param("id"), req.Nombre, req.Activa, usuarioID); err != nil {
		h.error(c, err, "actualizar-sede")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type asignarSedesRequest struct {
	// Lista COMPLETA de sedes del usuario: lo que no viene, se le quita.
	SedeIDs []string `json:"sede_ids"`
}

// AsignarSedes PUT /v1/cxc/usuarios/:id/sedes — qué cartera puede ver ese usuario.
func (h *Handler) AsignarSedes(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req asignarSedesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if req.SedeIDs == nil {
		req.SedeIDs = []string{}
	}
	if err := h.svc.AsignarSedes(c.Request.Context(), empresaID, c.Param("id"), req.SedeIDs, usuarioID); err != nil {
		h.error(c, err, "asignar-sedes")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "sedes": len(req.SedeIDs)})
}

// ---- Planillas de asociación: conciliación contra el depósito bancario ----

// PlanillaAsociacion GET /v1/cxc/asociaciones/:id/planilla?periodo=YYYY-MM
func (h *Handler) PlanillaAsociacion(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	d, err := h.svc.PlanillaDeAsociacion(c.Request.Context(), empresaID, c.Param("id"), c.Query("periodo"))
	if err != nil {
		h.error(c, err, "planilla-asociacion")
		return
	}
	c.JSON(http.StatusOK, d)
}

type abrirPlanillaRequest struct {
	Periodo    string `json:"periodo"`
	Referencia string `json:"referencia"`
	Nota       string `json:"nota"`
}

// AbrirPlanilla POST /v1/cxc/asociaciones/:id/planilla — registra el comprobante del correo.
func (h *Handler) AbrirPlanilla(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req abrirPlanillaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	d, err := h.svc.AbrirPlanilla(c.Request.Context(), empresaID, c.Param("id"), req.Periodo, req.Referencia, req.Nota, usuarioID)
	if err != nil {
		h.error(c, err, "abrir-planilla")
		return
	}
	c.JSON(http.StatusOK, d)
}

// CandidatosDeposito GET /v1/cxc/planillas/:id/candidatos — créditos de Bancos que podrían
// ser este depósito, con la señal de por qué.
func (h *Handler) CandidatosDeposito(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.CandidatosDeposito(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.error(c, err, "candidatos-deposito")
		return
	}
	c.JSON(http.StatusOK, lista)
}

type depositoRequest struct {
	MovimientoID string `json:"movimiento_id" validate:"required,uuid"`
}

// VincularDeposito POST /v1/cxc/planillas/:id/depositos
func (h *Handler) VincularDeposito(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req depositoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	d, err := h.svc.VincularDeposito(c.Request.Context(), empresaID, c.Param("id"), req.MovimientoID, usuarioID)
	if err != nil {
		h.error(c, err, "vincular-deposito")
		return
	}
	c.JSON(http.StatusOK, d)
}

// DesvincularDeposito DELETE /v1/cxc/planillas/:id/depositos/:movimiento
func (h *Handler) DesvincularDeposito(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	d, err := h.svc.DesvincularDeposito(c.Request.Context(), empresaID, c.Param("id"), c.Param("movimiento"), usuarioID)
	if err != nil {
		h.error(c, err, "desvincular-deposito")
		return
	}
	c.JSON(http.StatusOK, d)
}
