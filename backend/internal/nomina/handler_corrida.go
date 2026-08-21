package nomina

// Endpoints de la corrida quincenal (lectura: rrhh.ver; mutaciones: rrhh.corrida, crítico).

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// ListarCorridas GET /v1/rrhh/corridas?anio= (rrhh.ver)
func (h *Handler) ListarCorridas(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	anio, err := strconv.Atoi(c.DefaultQuery("anio", strconv.Itoa(time.Now().Year())))
	if err != nil || anio < 2024 || anio > 2100 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año inválido")
		return
	}
	items, err := h.svc.ListarCorridas(c.Request.Context(), empresaID, anio)
	if err != nil {
		h.responderError(c, err, "listar-corridas")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// CorridaPorID GET /v1/rrhh/corridas/:id (rrhh.ver) — cabecera + colillas + novedades.
func (h *Handler) CorridaPorID(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	det, err := h.svc.CorridaDetalle(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "corrida-por-id")
		return
	}
	c.JSON(http.StatusOK, det)
}

type corridaRequest struct {
	Anio      int    `json:"anio" validate:"required,gte=2024,lte=2100"`
	Mes       int    `json:"mes" validate:"required,gte=1,lte=12"`
	Tipo      string `json:"tipo" validate:"required,oneof=ADELANTO LIQUIDACION"`
	FechaPago string `json:"fecha_pago" validate:"required,datetime=2006-01-02"`
}

// CrearCorrida POST /v1/rrhh/corridas (rrhh.corrida) — crea el BORRADOR y calcula al instante.
func (h *Handler) CrearCorrida(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req corridaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	det, err := h.svc.CrearCorrida(c.Request.Context(), empresaID, req.Anio, req.Mes, req.Tipo, req.FechaPago, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-corrida")
		return
	}
	c.JSON(http.StatusCreated, det)
}

type novedadesRequest struct {
	Novedades []struct {
		EmpleadoID string `json:"empleado_id" validate:"required,uuid"`
		ConceptoID string `json:"concepto_id" validate:"required,uuid"`
		Monto      string `json:"monto"`
		// cantidad = HORAS (extra). Con cantidad, el monto lo calcula el sistema; sin ella,
		// el monto es obligatorio (lo valida el servicio).
		Cantidad string `json:"cantidad"`
	} `json:"novedades" validate:"dive"`
}

// GuardarNovedades PUT /v1/rrhh/corridas/:id/novedades (rrhh.corrida) — reemplaza el set
// completo del mes (comisiones, extras, bonos, viáticos…) y recalcula.
func (h *Handler) GuardarNovedades(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req novedadesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	novedades := make([]NovedadInput, 0, len(req.Novedades))
	for _, n := range req.Novedades {
		novedades = append(novedades, NovedadInput{
			EmpleadoID: n.EmpleadoID, ConceptoID: n.ConceptoID, Monto: n.Monto, Cantidad: n.Cantidad,
		})
	}
	det, err := h.svc.GuardarNovedades(c.Request.Context(), empresaID, c.Param("id"), novedades, usuarioID)
	if err != nil {
		h.responderError(c, err, "guardar-novedades")
		return
	}
	c.JSON(http.StatusOK, det)
}

// RecalcularCorrida POST /v1/rrhh/corridas/:id/recalcular (rrhh.corrida)
func (h *Handler) RecalcularCorrida(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	det, err := h.svc.RecalcularCorrida(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "recalcular-corrida")
		return
	}
	c.JSON(http.StatusOK, det)
}

// AprobarCorrida POST /v1/rrhh/corridas/:id/aprobar (rrhh.corrida, crítico)
func (h *Handler) AprobarCorrida(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	corrida, err := h.svc.AprobarCorrida(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "aprobar-corrida")
		return
	}
	c.JSON(http.StatusOK, corrida)
}

// PagarCorrida POST /v1/rrhh/corridas/:id/pagar (rrhh.corrida) — descuenta saldos de deducciones.
func (h *Handler) PagarCorrida(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	corrida, err := h.svc.PagarCorrida(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "pagar-corrida")
		return
	}
	c.JSON(http.StatusOK, corrida)
}

// AnularCorrida POST /v1/rrhh/corridas/:id/anular (rrhh.corrida)
func (h *Handler) AnularCorrida(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	corrida, err := h.svc.AnularCorrida(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "anular-corrida")
		return
	}
	c.JSON(http.StatusOK, corrida)
}
