package nomina

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone los endpoints de RRHH / Nómina (bajo RequireEmpresa).
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler de nómina.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func ctxEmpresa(c *gin.Context) (empresaID, usuarioID string, ok bool) {
	claims, exists := auth.ClaimsFromContext(c)
	if !exists {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return "", "", false
	}
	return claims.EmpresaID, claims.UsuarioID(), true
}

// ---- Empleados ----

type empleadoRequest struct {
	Nombre             string `json:"nombre" validate:"required,max=200"`
	TipoIdentificacion string `json:"tipo_identificacion" validate:"omitempty,oneof=CEDULA DIMEX PASAPORTE"`
	Identificacion     string `json:"identificacion" validate:"required,max=40"`
	Email              string `json:"email" validate:"omitempty,email"`
	Telefono           string `json:"telefono" validate:"max=30"`
	IBAN               string `json:"iban" validate:"max=34"`
	DepartamentoID     string `json:"departamento_id" validate:"omitempty,uuid"`
	Puesto             string `json:"puesto" validate:"max=120"`
	FechaIngreso       string `json:"fecha_ingreso" validate:"required,datetime=2006-01-02"`
	SalarioBase        string `json:"salario_base" validate:"required"`
	Jornada            string `json:"jornada" validate:"omitempty,oneof=MENSUAL QUINCENAL SEMANAL HORAS"`
	Hijos              int    `json:"hijos" validate:"gte=0,lte=20"`
	Conyuge            bool   `json:"conyuge"`
}

func bindEmpleado(c *gin.Context) (EmpleadoInput, bool) {
	var req empleadoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return EmpleadoInput{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return EmpleadoInput{}, false
	}
	salario, err := decimal.NewFromString(req.SalarioBase)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "salario_base inválido")
		return EmpleadoInput{}, false
	}
	return EmpleadoInput{
		Nombre: req.Nombre, TipoIdentificacion: req.TipoIdentificacion, Identificacion: req.Identificacion,
		Email: req.Email, Telefono: req.Telefono, IBAN: req.IBAN, DepartamentoID: req.DepartamentoID,
		Puesto: req.Puesto, FechaIngreso: req.FechaIngreso, SalarioBase: salario, Jornada: req.Jornada,
		Hijos: req.Hijos, Conyuge: req.Conyuge,
	}, true
}

// ListarEmpleados GET /v1/rrhh/empleados?q=&estado= (rrhh.ver)
func (h *Handler) ListarEmpleados(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.ListarEmpleados(c.Request.Context(), empresaID,
		FiltrosEmpleado{Q: c.Query("q"), Estado: c.Query("estado")})
	if err != nil {
		h.responderError(c, err, "listar-empleados")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// EmpleadoPorID GET /v1/rrhh/empleados/:id (rrhh.ver)
func (h *Handler) EmpleadoPorID(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	e, err := h.svc.EmpleadoPorID(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "empleado-por-id")
		return
	}
	c.JSON(http.StatusOK, e)
}

// CrearEmpleado POST /v1/rrhh/empleados (rrhh.empleados)
func (h *Handler) CrearEmpleado(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindEmpleado(c)
	if !ok {
		return
	}
	e, err := h.svc.CrearEmpleado(c.Request.Context(), empresaID, in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-empleado")
		return
	}
	c.JSON(http.StatusCreated, e)
}

// ActualizarEmpleado PATCH /v1/rrhh/empleados/:id (rrhh.empleados)
func (h *Handler) ActualizarEmpleado(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindEmpleado(c)
	if !ok {
		return
	}
	e, err := h.svc.ActualizarEmpleado(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "actualizar-empleado")
		return
	}
	c.JSON(http.StatusOK, e)
}

// DesactivarEmpleado POST /v1/rrhh/empleados/:id/desactivar (rrhh.empleados)
func (h *Handler) DesactivarEmpleado(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		FechaSalida string `json:"fecha_salida" validate:"omitempty,datetime=2006-01-02"`
	}
	_ = c.ShouldBindJSON(&req) // cuerpo opcional
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.DesactivarEmpleado(c.Request.Context(), empresaID, c.Param("id"), req.FechaSalida, usuarioID); err != nil {
		h.responderError(c, err, "desactivar-empleado")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Parámetros ----

// Parametros GET /v1/rrhh/parametros/:anio (rrhh.ver)
func (h *Handler) Parametros(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	anio, err := strconv.Atoi(c.Param("anio"))
	if err != nil || anio < 2024 || anio > 2100 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año inválido")
		return
	}
	p, err := h.svc.Parametros(c.Request.Context(), empresaID, anio)
	if err != nil {
		h.responderError(c, err, "parametros")
		return
	}
	c.JSON(http.StatusOK, p)
}

type parametrosRequest struct {
	Cargas        []Carga     `json:"cargas" validate:"required,min=1"`
	Renta         RentaConfig `json:"renta"`
	INSRiesgosPct string      `json:"ins_riesgos_pct" validate:"required"`
	AplicaINA     bool        `json:"aplica_ina"`
	AdelantoPct   string      `json:"adelanto_pct" validate:"required"`
	AdelantoBase  string      `json:"adelanto_base" validate:"required,oneof=SALARIO_BASE BRUTO"`
	Redondeo      string      `json:"redondeo" validate:"required,oneof=COLON CENTIMO"`
	ProvisionBase string      `json:"provision_base" validate:"required,oneof=REMUNERACION_TOTAL SALARIO_BASE"`
	AguinaldoPct  string      `json:"aguinaldo_pct" validate:"required"`
	VacacionesPct string      `json:"vacaciones_pct" validate:"required"`
	CesantiaPct   string      `json:"cesantia_pct" validate:"required"`
	// Horas extra (art. 139). Opcionales: vacíos conservan lo vigente. El mínimo legal del
	// factor lo garantiza el CHECK de la migración y el cálculo (nunca menos de 1,5).
	HorasJornadaMes string `json:"horas_jornada_mes"`
	FactorHoraExtra string `json:"factor_hora_extra"`
}

// GuardarParametros PUT /v1/rrhh/parametros/:anio (rrhh.parametros, crítico)
func (h *Handler) GuardarParametros(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	anio, err := strconv.Atoi(c.Param("anio"))
	if err != nil || anio < 2024 || anio > 2100 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año inválido")
		return
	}
	var req parametrosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	ins, err1 := decimal.NewFromString(req.INSRiesgosPct)
	adelanto, err2 := decimal.NewFromString(req.AdelantoPct)
	aguinaldo, err3 := decimal.NewFromString(req.AguinaldoPct)
	vacaciones, err4 := decimal.NewFromString(req.VacacionesPct)
	cesantia, err5 := decimal.NewFromString(req.CesantiaPct)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || ins.IsNegative() {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "porcentajes inválidos (ins, adelanto, provisiones)")
		return
	}
	p, err := h.svc.GuardarParametros(c.Request.Context(), empresaID, anio, ParametrosInput{
		Cargas: req.Cargas, Renta: req.Renta, INSRiesgosPct: ins, AplicaINA: req.AplicaINA,
		AdelantoPct: adelanto, AdelantoBase: req.AdelantoBase, Redondeo: req.Redondeo,
		ProvisionBase: req.ProvisionBase, AguinaldoPct: aguinaldo, VacacionesPct: vacaciones,
		CesantiaPct: cesantia,
		// Horas extra: vacíos conservan lo que la empresa ya tenía (o los de referencia).
		HorasJornadaMes: req.HorasJornadaMes, FactorHoraExtra: req.FactorHoraExtra,
	}, usuarioID)
	if err != nil {
		h.responderError(c, err, "guardar-parametros")
		return
	}
	c.JSON(http.StatusOK, p)
}

// ---- Conceptos ----

// ListarConceptos GET /v1/rrhh/conceptos (rrhh.ver)
func (h *Handler) ListarConceptos(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.ListarConceptos(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "listar-conceptos")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type conceptoRequest struct {
	Nombre          string `json:"nombre" validate:"required,max=120"`
	Tipo            string `json:"tipo" validate:"required,oneof=INGRESO DEDUCCION"`
	AfectaCCSS      bool   `json:"afecta_ccss"`
	AfectaRenta     bool   `json:"afecta_renta"`
	AfectaAguinaldo bool   `json:"afecta_aguinaldo"`
	BaseLegal       string `json:"base_legal" validate:"max=300"`
}

func bindConcepto(c *gin.Context) (ConceptoInput, bool) {
	var req conceptoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return ConceptoInput{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return ConceptoInput{}, false
	}
	return ConceptoInput{
		Nombre: req.Nombre, Tipo: req.Tipo, AfectaCCSS: req.AfectaCCSS,
		AfectaRenta: req.AfectaRenta, AfectaAguinaldo: req.AfectaAguinaldo, BaseLegal: req.BaseLegal,
	}, true
}

// CrearConcepto POST /v1/rrhh/conceptos (rrhh.parametros)
func (h *Handler) CrearConcepto(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindConcepto(c)
	if !ok {
		return
	}
	concepto, err := h.svc.CrearConcepto(c.Request.Context(), empresaID, in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-concepto")
		return
	}
	c.JSON(http.StatusCreated, concepto)
}

// ActualizarConcepto PATCH /v1/rrhh/conceptos/:id (rrhh.parametros)
func (h *Handler) ActualizarConcepto(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindConcepto(c)
	if !ok {
		return
	}
	concepto, err := h.svc.ActualizarConcepto(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "actualizar-concepto")
		return
	}
	c.JSON(http.StatusOK, concepto)
}

// DesactivarConcepto POST /v1/rrhh/conceptos/:id/desactivar (rrhh.parametros)
func (h *Handler) DesactivarConcepto(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.DesactivarConcepto(c.Request.Context(), empresaID, c.Param("id"), usuarioID); err != nil {
		h.responderError(c, err, "desactivar-concepto")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Deducciones ----

type deduccionRequest struct {
	ConceptoID string `json:"concepto_id" validate:"required,uuid"`
	Etiqueta   string `json:"etiqueta" validate:"required,max=120"`
	Cuota      string `json:"cuota" validate:"required"`
	SaldoTotal string `json:"saldo_total"`
	Prioridad  int    `json:"prioridad" validate:"gte=0,lte=1000"`
	Frecuencia string `json:"frecuencia" validate:"omitempty,oneof=AMBAS PRIMERA SEGUNDA MENSUAL"`
}

func bindDeduccion(c *gin.Context) (DeduccionInput, bool) {
	var req deduccionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return DeduccionInput{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return DeduccionInput{}, false
	}
	cuota, err := decimal.NewFromString(req.Cuota)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuota inválida")
		return DeduccionInput{}, false
	}
	in := DeduccionInput{ConceptoID: req.ConceptoID, Etiqueta: req.Etiqueta, Cuota: cuota,
		Prioridad: req.Prioridad, Frecuencia: req.Frecuencia}
	if req.Prioridad == 0 {
		in.Prioridad = 100
	}
	if req.SaldoTotal != "" {
		saldo, err := decimal.NewFromString(req.SaldoTotal)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "saldo_total inválido")
			return DeduccionInput{}, false
		}
		in.SaldoTotal = &saldo
	}
	return in, true
}

// ListarDeducciones GET /v1/rrhh/empleados/:id/deducciones (rrhh.ver)
func (h *Handler) ListarDeducciones(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	items, err := h.svc.ListarDeducciones(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "listar-deducciones")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// CrearDeduccion POST /v1/rrhh/empleados/:id/deducciones (rrhh.empleados)
func (h *Handler) CrearDeduccion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindDeduccion(c)
	if !ok {
		return
	}
	d, err := h.svc.CrearDeduccion(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-deduccion")
		return
	}
	c.JSON(http.StatusCreated, d)
}

// ActualizarDeduccion PATCH /v1/rrhh/empleados/:id/deducciones/:dedId (rrhh.empleados)
func (h *Handler) ActualizarDeduccion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindDeduccion(c)
	if !ok {
		return
	}
	d, err := h.svc.ActualizarDeduccion(c.Request.Context(), empresaID, c.Param("id"), c.Param("dedId"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "actualizar-deduccion")
		return
	}
	c.JSON(http.StatusOK, d)
}

// DesactivarDeduccion POST /v1/rrhh/empleados/:id/deducciones/:dedId/desactivar (rrhh.empleados)
func (h *Handler) DesactivarDeduccion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.DesactivarDeduccion(c.Request.Context(), empresaID, c.Param("id"), c.Param("dedId"), usuarioID); err != nil {
		h.responderError(c, err, "desactivar-deduccion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Errores ----

func (h *Handler) responderError(c *gin.Context, err error, op string) {
	switch {
	case errors.Is(err, ErrEmpleadoNoEncontrado), errors.Is(err, ErrConceptoNoEncontrado),
		errors.Is(err, ErrDeduccionNoEncontrada), errors.Is(err, ErrParametrosNoEncontrados),
		errors.Is(err, ErrCorridaNoEncontrada), errors.Is(err, ErrFiniquitoNoEncontrado),
		errors.Is(err, ErrIncapacidadNoEncontrada), errors.Is(err, ErrVacacionNoEncontrada):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrEmpleadoDuplicado), errors.Is(err, ErrConceptoDuplicado),
		errors.Is(err, ErrCorridaDuplicada), errors.Is(err, ErrFiniquitoDuplicado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrCorridaNoEditable), errors.Is(err, ErrCorridaNoAprobable),
		errors.Is(err, ErrCorridaNoPagable), errors.Is(err, ErrCorridaNoAnulable),
		errors.Is(err, ErrLiquidacionCerrada), errors.Is(err, ErrAdelantoDescontado),
		errors.Is(err, ErrFiniquitoNoEditable), errors.Is(err, ErrFiniquitoNoAprobable),
		errors.Is(err, ErrFiniquitoNoPagable), errors.Is(err, ErrFiniquitoNoAnulable),
		errors.Is(err, ErrCorridaNoCongelada), errors.Is(err, ErrFiniquitoRespaldaCorrida),
		errors.Is(err, ErrFiniquitoModificado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrConceptoDeSistema):
		httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, err.Error())
	case errors.Is(err, ErrSalarioInvalido), errors.Is(err, ErrCargaInvalida), errors.Is(err, ErrCargasIncompletas),
		errors.Is(err, ErrTramosInvalidos), errors.Is(err, ErrBaseLegalRequerida),
		errors.Is(err, ErrDeduccionInvalida), errors.Is(err, ErrConceptoNoEsDeduccion),
		errors.Is(err, ErrCorridaSinEmpleados), errors.Is(err, ErrNovedadSoloLiquidacion),
		errors.Is(err, ErrNovedadInvalida), errors.Is(err, ErrMesInvalido),
		errors.Is(err, ErrAdelantoPendiente), errors.Is(err, ErrNetoNegativo),
		errors.Is(err, ErrAdelantoSinColilla), errors.Is(err, ErrMotivoInvalido),
		errors.Is(err, ErrFechaSalidaInvalida), errors.Is(err, ErrArchivoSinRegistros),
		errors.Is(err, ErrPlanillaSoloLiquidacion), errors.Is(err, ErrDiasVacacionesInvalidos),
		errors.Is(err, ErrEntidadInvalida), errors.Is(err, ErrDiasInvalidos),
		errors.Is(err, ErrFechaInvalida), errors.Is(err, ErrSinSaldoVacaciones),
		errors.Is(err, ErrAusenciaCorridaCerrada):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "22P02" || pgErr.Code == "22007" || pgErr.Code == "22008") {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "dato inválido (id o fecha con formato incorrecto)")
			return
		}
		h.log.Error("nomina "+op, zap.Error(err))
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}
