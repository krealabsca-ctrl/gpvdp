package nomina

// Endpoints de finiquito (rrhh.finiquito, crítico), exportaciones .xlsx de la corrida
// (archivo SINPE: rrhh.corrida; planilla CCSS: rrhh.ver) y reporte de provisiones.

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ---- Finiquitos ----

// ListarFiniquitos GET /v1/rrhh/finiquitos (rrhh.ver)
func (h *Handler) ListarFiniquitos(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.ListarFiniquitos(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "listar-finiquitos")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// FiniquitoPorID GET /v1/rrhh/finiquitos/:id (rrhh.ver)
func (h *Handler) FiniquitoPorID(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	fi, err := h.svc.FiniquitoPorID(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "finiquito-por-id")
		return
	}
	c.JSON(http.StatusOK, fi)
}

type finiquitoRequest struct {
	EmpleadoID     string `json:"empleado_id" validate:"omitempty,uuid"`
	Motivo         string `json:"motivo" validate:"required,oneof=DESPIDO_RESPONSABILIDAD RENUNCIA FIN_CONTRATO MUTUO_ACUERDO"`
	FechaSalida    string `json:"fecha_salida" validate:"required,datetime=2006-01-02"`
	DiasVacaciones string `json:"dias_vacaciones"`
}

func bindFiniquito(c *gin.Context, exigirEmpleado bool) (FiniquitoInput, bool) {
	var req finiquitoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return FiniquitoInput{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return FiniquitoInput{}, false
	}
	if exigirEmpleado && req.EmpleadoID == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "empleado_id es requerido")
		return FiniquitoInput{}, false
	}
	return FiniquitoInput{EmpleadoID: req.EmpleadoID, Motivo: req.Motivo,
		FechaSalida: req.FechaSalida, DiasVacaciones: req.DiasVacaciones}, true
}

// CrearFiniquito POST /v1/rrhh/finiquitos (rrhh.finiquito)
func (h *Handler) CrearFiniquito(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindFiniquito(c, true)
	if !ok {
		return
	}
	fi, err := h.svc.CrearFiniquito(c.Request.Context(), empresaID, in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-finiquito")
		return
	}
	c.JSON(http.StatusCreated, fi)
}

// ActualizarFiniquito PATCH /v1/rrhh/finiquitos/:id (rrhh.finiquito) — recalcula el borrador.
func (h *Handler) ActualizarFiniquito(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindFiniquito(c, false)
	if !ok {
		return
	}
	fi, err := h.svc.ActualizarFiniquito(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "actualizar-finiquito")
		return
	}
	c.JSON(http.StatusOK, fi)
}

// AprobarFiniquito POST /v1/rrhh/finiquitos/:id/aprobar (rrhh.finiquito, crítico)
func (h *Handler) AprobarFiniquito(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	fi, err := h.svc.AprobarFiniquito(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "aprobar-finiquito")
		return
	}
	c.JSON(http.StatusOK, fi)
}

// PagarFiniquito POST /v1/rrhh/finiquitos/:id/pagar (rrhh.finiquito)
func (h *Handler) PagarFiniquito(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	fi, err := h.svc.PagarFiniquito(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "pagar-finiquito")
		return
	}
	c.JSON(http.StatusOK, fi)
}

// AnularFiniquito POST /v1/rrhh/finiquitos/:id/anular (rrhh.finiquito)
func (h *Handler) AnularFiniquito(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	fi, err := h.svc.AnularFiniquito(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "anular-finiquito")
		return
	}
	c.JSON(http.StatusOK, fi)
}

// ---- Exportaciones de la corrida ----

// ArchivoPago GET /v1/rrhh/corridas/:id/archivo-pago (rrhh.corrida) — .xlsx de carga masiva.
func (h *Handler) ArchivoPago(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	buf, nombre, err := h.svc.ArchivoPagoXLSX(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "archivo-pago")
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+nombre+"\"")
	c.Data(http.StatusOK, xlsxContentType, buf)
}

// PlanillaCCSS GET /v1/rrhh/corridas/:id/planilla-ccss (rrhh.ver) — .xlsx para SICERE.
func (h *Handler) PlanillaCCSS(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	buf, nombre, err := h.svc.PlanillaCCSSXLSX(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "planilla-ccss")
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+nombre+"\"")
	c.Data(http.StatusOK, xlsxContentType, buf)
}

// Provisiones GET /v1/rrhh/reportes/provisiones?anio= (rrhh.ver)
func (h *Handler) Provisiones(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	anio, err := strconv.Atoi(c.DefaultQuery("anio", strconv.Itoa(time.Now().Year())))
	if err != nil || anio < 2024 || anio > 2100 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año inválido")
		return
	}
	items, err := h.svc.ProvisionesDelAnio(c.Request.Context(), empresaID, anio)
	if err != nil {
		h.responderError(c, err, "provisiones")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
