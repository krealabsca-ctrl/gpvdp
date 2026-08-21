package nomina

// Endpoints de incapacidades y vacaciones (permiso rrhh.ausencias; lectura con rrhh.ver).

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// periodoDeQuery lee anio/mes de la query, con el mes actual por defecto.
func periodoDeQuery(c *gin.Context) (anio, mes int, ok bool) {
	ahora := time.Now()
	anio, err := strconv.Atoi(c.DefaultQuery("anio", strconv.Itoa(ahora.Year())))
	if err != nil || anio < 2024 || anio > 2100 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año inválido")
		return 0, 0, false
	}
	mes, err = strconv.Atoi(c.DefaultQuery("mes", strconv.Itoa(int(ahora.Month()))))
	if err != nil || mes < 1 || mes > 12 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "mes inválido")
		return 0, 0, false
	}
	return anio, mes, true
}

// ListarIncapacidades GET /v1/rrhh/incapacidades?anio=&mes= (rrhh.ver)
func (h *Handler) ListarIncapacidades(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	anio, mes, ok := periodoDeQuery(c)
	if !ok {
		return
	}
	items, err := h.svc.ListarIncapacidades(c.Request.Context(), empresaID, anio, mes)
	if err != nil {
		h.responderError(c, err, "listar-incapacidades")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type incapacidadRequest struct {
	EmpleadoID    string `json:"empleado_id" validate:"required,uuid"`
	Entidad       string `json:"entidad" validate:"required,oneof=CCSS INS"`
	FechaInicio   string `json:"fecha_inicio" validate:"required,datetime=2006-01-02"`
	Dias          int    `json:"dias" validate:"required,gte=1,lte=365"`
	Boleta        string `json:"boleta" validate:"max=60"`
	Observaciones string `json:"observaciones" validate:"max=300"`
}

// RegistrarIncapacidad POST /v1/rrhh/incapacidades (rrhh.ausencias)
func (h *Handler) RegistrarIncapacidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req incapacidadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	inc, err := h.svc.RegistrarIncapacidad(c.Request.Context(), empresaID, IncapacidadInput{
		EmpleadoID: req.EmpleadoID, Entidad: req.Entidad, FechaInicio: req.FechaInicio,
		Dias: req.Dias, Boleta: req.Boleta, Observaciones: req.Observaciones,
	}, usuarioID)
	if err != nil {
		h.responderError(c, err, "registrar-incapacidad")
		return
	}
	c.JSON(http.StatusCreated, inc)
}

// AnularIncapacidad POST /v1/rrhh/incapacidades/:id/anular (rrhh.ausencias)
func (h *Handler) AnularIncapacidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.AnularIncapacidad(c.Request.Context(), empresaID, c.Param("id"), usuarioID); err != nil {
		h.responderError(c, err, "anular-incapacidad")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// SaldosVacaciones GET /v1/rrhh/vacaciones/saldos?anio= (rrhh.ver)
func (h *Handler) SaldosVacaciones(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	anio, err := strconv.Atoi(c.DefaultQuery("anio", strconv.Itoa(time.Now().Year())))
	if err != nil || anio < 2024 || anio > 2100 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año inválido")
		return
	}
	items, err := h.svc.SaldosVacaciones(c.Request.Context(), empresaID, anio)
	if err != nil {
		h.responderError(c, err, "saldos-vacaciones")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// ListarVacaciones GET /v1/rrhh/vacaciones?empleado_id= (rrhh.ver)
func (h *Handler) ListarVacaciones(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.ListarVacaciones(c.Request.Context(), empresaID, c.Query("empleado_id"))
	if err != nil {
		h.responderError(c, err, "listar-vacaciones")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type vacacionRequest struct {
	EmpleadoID    string `json:"empleado_id" validate:"required,uuid"`
	FechaInicio   string `json:"fecha_inicio" validate:"required,datetime=2006-01-02"`
	Dias          string `json:"dias" validate:"required"`
	Observaciones string `json:"observaciones" validate:"max=300"`
}

// RegistrarVacacion POST /v1/rrhh/vacaciones (rrhh.ausencias)
func (h *Handler) RegistrarVacacion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req vacacionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	v, err := h.svc.RegistrarVacacion(c.Request.Context(), empresaID, VacacionInput{
		EmpleadoID: req.EmpleadoID, FechaInicio: req.FechaInicio, Dias: req.Dias,
		Observaciones: req.Observaciones,
	}, usuarioID)
	if err != nil {
		h.responderError(c, err, "registrar-vacacion")
		return
	}
	c.JSON(http.StatusCreated, v)
}

// AnularVacacion POST /v1/rrhh/vacaciones/:id/anular (rrhh.ausencias)
func (h *Handler) AnularVacacion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.AnularVacacion(c.Request.Context(), empresaID, c.Param("id"), usuarioID); err != nil {
		h.responderError(c, err, "anular-vacacion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
