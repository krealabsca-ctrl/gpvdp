package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Endpoints de las responsabilidades mensuales (migración 0082).
//
// Cero SQL y cero reglas acá: el handler ata el HTTP con el service y nada más. El alcance de lo
// que cada persona ve lo resuelve el service, del lado del servidor — nunca escondiendo botones.

// ctxResponsabilidad saca empresa, rol y usuario. El ROL hace falta además del usuario porque el
// alcance por partida se resuelve por rol (`rol_clasificacion_consulta`), no por persona.
func ctxResponsabilidad(c *gin.Context) (empresaID, rol, usuarioID string, ok bool) {
	claims, exists := auth.ClaimsFromContext(c)
	if !exists {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return "", "", "", false
	}
	return claims.EmpresaID, claims.Rol, claims.UsuarioID(), true
}

type responsabilidadRequest struct {
	Nombre          string `json:"nombre" validate:"required"`
	Contraparte     string `json:"contraparte" validate:"required"`
	ProveedorID     string `json:"proveedor_id" validate:"omitempty,uuid"`
	Tipo            string `json:"tipo" validate:"omitempty,oneof=PAGO TRAMITE"`
	Periodicidad    string `json:"periodicidad" validate:"omitempty,oneof=MENSUAL BIMENSUAL TRIMESTRAL SEMESTRAL ANUAL"`
	DiaVencimiento  int    `json:"dia_vencimiento" validate:"required,min=1,max=31"`
	MesAncla        int    `json:"mes_ancla" validate:"omitempty,min=1,max=12"`
	Moneda          string `json:"moneda" validate:"omitempty,oneof=CRC USD"`
	MontoEsperado   string `json:"monto_esperado"`
	MontoTipo       string `json:"monto_tipo" validate:"omitempty,oneof=FIJO VARIABLE"`
	RespaldoTipo    string `json:"respaldo_tipo" validate:"omitempty,oneof=CONTRATO ACTA CORREO VERBAL NINGUNO"`
	RespaldoArchivo string `json:"respaldo_archivo"`
	EsperaFactura   bool   `json:"espera_factura"`
	Deducible       bool   `json:"deducible"`
	ClasificacionID string `json:"clasificacion_id" validate:"omitempty,uuid"`
	DepartamentoID  string `json:"departamento_id" validate:"omitempty,uuid"`
	Notas           string `json:"notas"`
	TitularID       string `json:"titular_id" validate:"omitempty,uuid"`
	SuplenteID      string `json:"suplente_id" validate:"omitempty,uuid"`
}

func (r responsabilidadRequest) aInput() ResponsabilidadInput {
	return ResponsabilidadInput{
		Nombre: r.Nombre, Contraparte: r.Contraparte, ProveedorID: r.ProveedorID,
		Tipo: r.Tipo, Periodicidad: r.Periodicidad,
		DiaVencimiento: r.DiaVencimiento, MesAncla: r.MesAncla,
		Moneda: r.Moneda, MontoEsperado: r.MontoEsperado, MontoTipo: r.MontoTipo,
		RespaldoTipo: r.RespaldoTipo, RespaldoArchivo: r.RespaldoArchivo,
		EsperaFactura: r.EsperaFactura, Deducible: r.Deducible,
		ClasificacionID: r.ClasificacionID, DepartamentoID: r.DepartamentoID,
		Notas: r.Notas, TitularID: r.TitularID, SuplenteID: r.SuplenteID,
	}
}

func bindResponsabilidad(c *gin.Context) (ResponsabilidadInput, bool) {
	var req responsabilidadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return ResponsabilidadInput{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return ResponsabilidadInput{}, false
	}
	return req.aInput(), true
}

// Responsabilidades GET /v1/cxp/responsabilidades
func (h *Handler) Responsabilidades(c *gin.Context) {
	empresaID, rol, usuarioID, ok := ctxResponsabilidad(c)
	if !ok {
		return
	}
	lista, err := h.svc.ListarResponsabilidades(c.Request.Context(), empresaID, rol, usuarioID, FiltrosResponsabilidad{
		Estado: c.Query("estado"),
		Tipo:   c.Query("tipo"),
		Q:      c.Query("q"),
	})
	if err != nil {
		h.responderError(c, err, "listar-responsabilidades")
		return
	}
	c.JSON(http.StatusOK, lista)
}

// ResponsabilidadPorID GET /v1/cxp/responsabilidades/:id
func (h *Handler) ResponsabilidadPorID(c *gin.Context) {
	empresaID, rol, usuarioID, ok := ctxResponsabilidad(c)
	if !ok {
		return
	}
	r, err := h.svc.ResponsabilidadPorID(c.Request.Context(), empresaID, rol, usuarioID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "responsabilidad-por-id")
		return
	}
	c.JSON(http.StatusOK, r)
}

// CrearResponsabilidad POST /v1/cxp/responsabilidades
func (h *Handler) CrearResponsabilidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindResponsabilidad(c)
	if !ok {
		return
	}
	r, err := h.svc.CrearResponsabilidad(c.Request.Context(), empresaID, in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-responsabilidad")
		return
	}
	c.JSON(http.StatusCreated, r)
}

// ActualizarResponsabilidad PUT /v1/cxp/responsabilidades/:id
func (h *Handler) ActualizarResponsabilidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindResponsabilidad(c)
	if !ok {
		return
	}
	r, err := h.svc.ActualizarResponsabilidad(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "actualizar-responsabilidad")
		return
	}
	c.JSON(http.StatusOK, r)
}

type estadoResponsabilidadRequest struct {
	Estado string `json:"estado" validate:"required,oneof=ACTIVA SUSPENDIDA FINALIZADA"`
	Motivo string `json:"motivo"`
}

// CambiarEstadoResponsabilidad POST /v1/cxp/responsabilidades/:id/estado
func (h *Handler) CambiarEstadoResponsabilidad(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req estadoResponsabilidadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.CambiarEstadoResponsabilidad(c.Request.Context(), empresaID, c.Param("id"), req.Estado, req.Motivo, usuarioID); err != nil {
		h.responderError(c, err, "estado-responsabilidad")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MesDeResponsabilidades GET /v1/cxp/responsabilidades/mes?periodo=YYYY-MM
func (h *Handler) MesDeResponsabilidades(c *gin.Context) {
	empresaID, rol, usuarioID, ok := ctxResponsabilidad(c)
	if !ok {
		return
	}
	v, err := h.svc.MesDeResponsabilidades(c.Request.Context(), empresaID, rol, usuarioID, c.Query("periodo"))
	if err != nil {
		h.responderError(c, err, "mes-responsabilidades")
		return
	}
	c.JSON(http.StatusOK, v)
}

// MisResponsabilidades GET /v1/cxp/responsabilidades/mias?periodo=YYYY-MM
//
// La pantalla corta: dos o tres filas con nombre propio. Es la única que alguien de otra área va a
// abrir, y por eso no pide permiso extra — lo que te asignaron, lo ves.
func (h *Handler) MisResponsabilidades(c *gin.Context) {
	empresaID, _, usuarioID, ok := ctxResponsabilidad(c)
	if !ok {
		return
	}
	filas, err := h.svc.MisResponsabilidades(c.Request.Context(), empresaID, usuarioID, c.Query("periodo"))
	if err != nil {
		h.responderError(c, err, "mis-responsabilidades")
		return
	}
	c.JSON(http.StatusOK, filas)
}

// PrevisualizarMes GET /v1/cxp/responsabilidades/mes/plan?periodo=YYYY-MM
//
// Dice qué va a hacer «Abrir el mes» ANTES de hacerlo, con lo que queda afuera explicado por razón.
func (h *Handler) PrevisualizarMes(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	periodo := c.Query("periodo")
	if periodo == "" {
		periodo = PeriodoActualCR()
	}
	plan, err := h.svc.PrevisualizarMes(c.Request.Context(), empresaID, periodo)
	if err != nil {
		h.responderError(c, err, "previsualizar-mes")
		return
	}
	c.JSON(http.StatusOK, plan)
}

type abrirMesRequest struct {
	Periodo string `json:"periodo" validate:"required"`
	// Esperadas es el número que la persona vio en la previsualización. Si el plan cambió en el
	// medio, la apertura se detiene: uno confirma un total, no una intención. -1 lo salta.
	Esperadas *int `json:"esperadas"`
}

// AbrirMes POST /v1/cxp/responsabilidades/mes/abrir
func (h *Handler) AbrirMes(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req abrirMesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	esperadas := -1
	if req.Esperadas != nil {
		esperadas = *req.Esperadas
	}
	plan, err := h.svc.AbrirMes(c.Request.Context(), empresaID, req.Periodo, esperadas, usuarioID)
	if err != nil {
		h.responderError(c, err, "abrir-mes")
		return
	}
	c.JSON(http.StatusOK, plan)
}

type cerrarPeriodoRequest struct {
	Estado       string `json:"estado" validate:"required,oneof=CUMPLIDA NO_APLICA"`
	CumplidaCon  string `json:"cumplida_con" validate:"omitempty,oneof=FACTURA MOVIMIENTO ACUSE"`
	DocumentoID  string `json:"documento_id" validate:"omitempty,uuid"`
	MovimientoID string `json:"movimiento_id" validate:"omitempty,uuid"`
	AcuseArchivo string `json:"acuse_archivo"`
	Motivo       string `json:"motivo"`
}

// CerrarPeriodo POST /v1/cxp/responsabilidades/periodos/:id/cerrar
func (h *Handler) CerrarPeriodo(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req cerrarPeriodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	err := h.svc.CerrarPeriodo(c.Request.Context(), empresaID, c.Param("id"), CierrePeriodo{
		Estado: req.Estado, CumplidaCon: req.CumplidaCon,
		DocumentoID: req.DocumentoID, MovimientoID: req.MovimientoID,
		AcuseArchivo: req.AcuseArchivo, Motivo: req.Motivo,
	}, usuarioID)
	if err != nil {
		h.responderError(c, err, "cerrar-periodo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type reabrirPeriodoRequest struct {
	Motivo string `json:"motivo" validate:"required"`
}

// ReabrirPeriodo POST /v1/cxp/responsabilidades/periodos/:id/reabrir
func (h *Handler) ReabrirPeriodo(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req reabrirPeriodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.ReabrirPeriodo(c.Request.Context(), empresaID, c.Param("id"), req.Motivo, usuarioID); err != nil {
		h.responderError(c, err, "reabrir-periodo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
