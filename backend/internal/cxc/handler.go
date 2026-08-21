package cxc

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone CxC por HTTP. Sin SQL y sin reglas de negocio: parsea, llama al
// servicio y traduce el error al código correcto.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler de CxC.
func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

// TopeArchivo es el máximo que se acepta subir. Un archivo de 70 000 contratos ronda los
// 20 MB; 64 dan aire sin dejar la puerta abierta a un archivo absurdo.
const TopeArchivo = 64 << 20

func (h *Handler) claims(c *gin.Context) (empresaID, rol, usuarioID string, ok bool) {
	cl, existe := auth.ClaimsFromContext(c)
	if !existe {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return "", "", "", false
	}
	return cl.EmpresaID, cl.Rol, cl.UsuarioID(), true
}

// Catalogos GET /v1/cxc/catalogos — sedes, modalidades, formas de pago, asociaciones y tramos.
func (h *Handler) Catalogos(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	cat, err := h.svc.Catalogos(c.Request.Context(), empresaID)
	if err != nil {
		h.error(c, err, "catalogos")
		return
	}
	c.JSON(http.StatusOK, cat)
}

// Contratos GET /v1/cxc/contratos — la cartera filtrada, con el resumen de lo filtrado.
func (h *Handler) Contratos(c *gin.Context) {
	empresaID, rol, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	f := FiltrosContratos{
		Q:            c.Query("q"),
		SedeID:       c.Query("sede_id"),
		ModalidadID:  c.Query("modalidad_id"),
		FormaPagoID:  c.Query("forma_pago_id"),
		AsociacionID: c.Query("asociacion_id"),
		Estado:       c.Query("estado"),
		ConSaldo:     c.Query("con_saldo") == "true",
		EnRevision:   c.Query("en_revision") == "true",
		Orden:        c.Query("orden"),
		Page:         atoiDefault(c.Query("page"), 1),
		PageSize:     atoiDefault(c.Query("page_size"), 50),
	}
	lista, err := h.svc.ListarContratos(c.Request.Context(), empresaID, rol, usuarioID, f)
	if err != nil {
		h.error(c, err, "contratos")
		return
	}
	if lista.Items == nil {
		lista.Items = []Contrato{}
	}
	c.JSON(http.StatusOK, lista)
}

// Contrato GET /v1/cxc/contratos/:numero — la ficha con sus cargos.
func (h *Handler) Contrato(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	ficha, err := h.svc.Contrato360(c.Request.Context(), empresaID, c.Param("numero"), c.Query("solo_abiertos") == "true")
	if err != nil {
		h.error(c, err, "contrato")
		return
	}
	c.JSON(http.StatusOK, ficha)
}

// PrevisualizarImportacion POST /v1/cxc/importaciones/contratos/previsualizar
// Lee el archivo y devuelve el reporte de conciliación SIN escribir en la cartera.
func (h *Handler) PrevisualizarImportacion(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	archivo, nombre, err := h.archivoDe(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	id, rep, err := h.svc.PrevisualizarContratos(c.Request.Context(), empresaID, archivo, nombre, usuarioID)
	if err != nil {
		h.error(c, err, "previsualizar-importacion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"importacion_id": id, "reporte": rep})
}

// ConfirmarImportacion POST /v1/cxc/importaciones/contratos/confirmar
// Se manda el MISMO archivo: la fuente de verdad es el archivo, no un estado guardado a
// medias entre dos llamadas.
func (h *Handler) ConfirmarImportacion(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	archivo, _, err := h.archivoDe(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	rep, ap, err := h.svc.ConfirmarContratos(c.Request.Context(), empresaID, c.PostForm("importacion_id"), archivo, usuarioID)
	if err != nil {
		h.error(c, err, "confirmar-importacion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"reporte": rep, "aplicado": ap})
}

// PlanCargos GET /v1/cxc/cargos/plan?desde=&hasta= — cuántos cargos se crearían.
func (h *Handler) PlanCargos(c *gin.Context) {
	empresaID, rol, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	plan, err := h.svc.PrevisualizarCargos(c.Request.Context(), empresaID, rol, usuarioID, c.Query("desde"), c.Query("hasta"))
	if err != nil {
		h.error(c, err, "plan-cargos")
		return
	}
	c.JSON(http.StatusOK, plan)
}

type generarRequest struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`
	// Total es el número que el usuario vio en el plan. Si el plan cambió, se aborta.
	Total int `json:"total"`
}

// GenerarCargos POST /v1/cxc/cargos/generar — crea los cargos del plan (idempotente).
func (h *Handler) GenerarCargos(c *gin.Context) {
	empresaID, rol, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var in generarRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	plan, creados, err := h.svc.GenerarCargos(c.Request.Context(), empresaID, rol, usuarioID, in.Desde, in.Hasta, in.Total)
	if err != nil {
		h.error(c, err, "generar-cargos")
		return
	}
	c.JSON(http.StatusOK, gin.H{"plan": plan, "creados": creados})
}

// archivoDe saca el archivo del multipart. Acepta «archivo» o «file» porque los clientes
// que ya existen usan los dos nombres.
func (h *Handler) archivoDe(c *gin.Context) ([]byte, string, error) {
	fh, err := c.FormFile("archivo")
	if err != nil {
		fh, err = c.FormFile("file")
	}
	if err != nil {
		return nil, "", errors.New("falta el archivo (campo «archivo»)")
	}
	if fh.Size > TopeArchivo {
		return nil, "", errors.New("el archivo supera el tamaño máximo permitido")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, "", errors.New("no se pudo abrir el archivo")
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, TopeArchivo))
	if err != nil {
		return nil, "", errors.New("no se pudo leer el archivo")
	}
	return b, fh.Filename, nil
}

func (h *Handler) error(c *gin.Context, err error, op string) {
	var paramInvalido *ErrParametroInvalido
	switch {
	case errors.Is(err, ErrContratoNoEncontrado), errors.Is(err, ErrImportacionAjena):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrArchivoVacio), errors.Is(err, ErrSinEncabezado), errors.Is(err, ErrSinFilas):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrSinDesde), errors.Is(err, ErrRangoInvalido), errors.Is(err, ErrRangoDemasiadoAmplio):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrCobroNoEncontrado):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrCobroYaReversado), errors.Is(err, ErrCargoSinSaldo), errors.Is(err, ErrCargoAjeno),
		errors.Is(err, ErrMontoInvalido), errors.Is(err, ErrContratoAjeno), errors.Is(err, ErrCobroYaIdentificado):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrCanalInvalido), errors.Is(err, ErrResultadoInvalido),
		errors.Is(err, ErrPromesaRequerida), errors.Is(err, ErrPromesaFechaInvalida),
		errors.Is(err, ErrPromesaEnElPasado), errors.Is(err, ErrPromesaMontoInvalido):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrTramoNoEncontrado), errors.Is(err, ErrFormaPagoNoEncontrada),
		errors.Is(err, ErrSedeNoEncontrada), errors.Is(err, ErrUsuarioSinAcceso):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrPlanillaNoEncontrada), errors.Is(err, ErrMovimientoAjeno):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrNotaNoEncontrada):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrNotaYaAnulada), errors.Is(err, ErrMotivoRequerido),
		errors.Is(err, ErrContratoYaSuspendido), errors.Is(err, ErrContratoNoSuspendido):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrArregloNoEncontrado):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrArregloVigente):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrArregloCerrado), errors.Is(err, ErrPlazoInvalido),
		errors.Is(err, ErrPlazoExcedeTope), errors.Is(err, ErrMontoArregloInvalido),
		errors.Is(err, ErrPrimaExcedeMonto), errors.Is(err, ErrSinVencido),
		errors.Is(err, ErrFechaArregloInvalida):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrPlazoNoAutorizado):
		// 403 y no 422: no es que el plazo esté mal, es que ESTE usuario no lo puede autorizar.
		// La pantalla necesita distinguirlo para ofrecer «pedile al supervisor» en vez de
		// «corregí el número».
		httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, err.Error())
	case errors.Is(err, ErrMovimientoYaVinculado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrMovimientoNoEsCredito):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrSedeDuplicada):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrTramosSeTraslapan):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.As(err, &paramInvalido):
		// El motivo ya viene redactado: dice qué valor se esperaba, o por qué ese parámetro
		// todavía no se puede cambiar.
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, paramInvalido.Error())
	case errors.Is(err, ErrSinPermisoSedes):
		httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, err.Error())
	default:
		if h.log != nil {
			h.log.Error("cxc: error interno", zap.String("op", op), zap.Error(err))
		}
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// ---- Cobros (fase 2) ----

// Cobros GET /v1/cxc/cobros — lista con el resumen de lo filtrado.
func (h *Handler) Cobros(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.ListarCobros(c.Request.Context(), empresaID, FiltrosCobros{
		Q:              c.Query("q"),
		Contrato:       c.Query("contrato"),
		AsociacionID:   c.Query("asociacion_id"),
		Estado:         c.Query("estado"),
		Desde:          c.Query("desde"),
		Hasta:          c.Query("hasta"),
		SinIdentificar: c.Query("sin_identificar") == "true",
		Page:           atoiDefault(c.Query("page"), 1),
		PageSize:       atoiDefault(c.Query("page_size"), 50),
	})
	if err != nil {
		h.error(c, err, "cobros")
		return
	}
	if lista.Items == nil {
		lista.Items = []CobroFila{}
	}
	c.JSON(http.StatusOK, lista)
}

type cobroRequest struct {
	Contrato      string `json:"contrato"`
	Consecutivo   string `json:"consecutivo"`
	FechaPago     string `json:"fecha_pago" binding:"required"`
	FechaBancaria string `json:"fecha_bancaria"`
	Monto         string `json:"monto" binding:"required"`
	FormaPago     string `json:"forma_pago"`
	Asociacion    string `json:"asociacion"`
	Referencia    string `json:"referencia"`
	Concepto      string `json:"concepto"`
	Origen        string `json:"origen"`
	// Destinos: cargos elegidos por el operador. Vacío = FIFO (más viejo primero).
	Destinos []string `json:"destinos"`
}

// RegistrarCobro POST /v1/cxc/cobros — la vía de la API y de la caja.
//
// Idempotente por la cabecera `Idempotency-Key` (o por el consecutivo): reenviar el mismo
// cobro devuelve el MISMO resultado con `repetido: true`, no un duplicado. Es lo que
// permite reintentar sin miedo cuando se cae la red a mitad de un lote.
func (h *Handler) RegistrarCobro(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var in cobroRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido: fecha_pago y monto son obligatorios")
		return
	}
	monto, err := decimal.NewFromString(in.Monto)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "monto inválido")
		return
	}
	llave := c.GetHeader("Idempotency-Key")
	if llave == "" && in.Consecutivo != "" {
		llave = "api:" + in.Consecutivo
	}
	res, err := h.svc.RegistrarCobro(c.Request.Context(), empresaID, CobroInput{
		Contrato: in.Contrato, Consecutivo: in.Consecutivo,
		FechaPago: in.FechaPago, FechaBancaria: in.FechaBancaria,
		Monto: monto, FormaPago: in.FormaPago, Asociacion: in.Asociacion,
		Referencia: in.Referencia, Concepto: in.Concepto, Origen: in.Origen,
		IdempotencyKey: llave, Destinos: in.Destinos,
	}, usuarioID)
	if err != nil {
		h.error(c, err, "registrar-cobro")
		return
	}
	// 200 en un reenvío y 201 en el primero: el cliente puede distinguirlos sin leer el
	// cuerpo, que es lo que espera de una API idempotente.
	estado := http.StatusCreated
	if res.Repetido {
		estado = http.StatusOK
	}
	c.JSON(estado, res)
}

type reversaRequest struct {
	Motivo string `json:"motivo" binding:"required"`
}

// ReversarCobro POST /v1/cxc/cobros/:id/reversar — cheque devuelto, débito rechazado.
func (h *Handler) ReversarCobro(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var in reversaRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el motivo de la reversa es obligatorio")
		return
	}
	if err := h.svc.ReversarCobro(c.Request.Context(), empresaID, c.Param("id"), in.Motivo, usuarioID); err != nil {
		h.error(c, err, "reversar-cobro")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type identificarRequest struct {
	Contrato string `json:"contrato" binding:"required"`
}

// IdentificarCobro POST /v1/cxc/cobros/:id/identificar — le pone dueño a un depósito.
func (h *Handler) IdentificarCobro(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var in identificarRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el número de contrato es obligatorio")
		return
	}
	res, err := h.svc.IdentificarCobro(c.Request.Context(), empresaID, c.Param("id"), in.Contrato, usuarioID)
	if err != nil {
		h.error(c, err, "identificar-cobro")
		return
	}
	c.JSON(http.StatusOK, res)
}

// PrevisualizarCobros POST /v1/cxc/importaciones/cobros/previsualizar
func (h *Handler) PrevisualizarCobros(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	archivo, nombre, err := h.archivoDe(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	id, rep, err := h.svc.PrevisualizarCobros(c.Request.Context(), empresaID, archivo, nombre, usuarioID)
	if err != nil {
		h.error(c, err, "previsualizar-cobros")
		return
	}
	c.JSON(http.StatusOK, gin.H{"importacion_id": id, "reporte": rep})
}

// ConfirmarCobros POST /v1/cxc/importaciones/cobros/confirmar
func (h *Handler) ConfirmarCobros(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	archivo, _, err := h.archivoDe(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	rep, ap, fallas, err := h.svc.ConfirmarCobros(c.Request.Context(), empresaID, c.PostForm("importacion_id"), archivo, usuarioID)
	if err != nil {
		h.error(c, err, "confirmar-cobros")
		return
	}
	if fallas == nil {
		fallas = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"reporte": rep, "aplicado": ap, "fallas": fallas})
}

// Asociaciones GET /v1/cxc/asociaciones/panorama?periodo=YYYY-MM
// Cuánto debía traer cada asociación contra cuánto trajo, y quién no envió nada.
func (h *Handler) PanoramaAsociaciones(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	pan, err := h.svc.PanoramaAsociaciones(c.Request.Context(), empresaID, c.Query("periodo"))
	if err != nil {
		h.error(c, err, "panorama-asociaciones")
		return
	}
	c.JSON(http.StatusOK, pan)
}

// ---- Gestión de cobro (fase 3) ----

// Cola GET /v1/cxc/cola — la cola de trabajo ordenada por valor esperado.
func (h *Handler) Cola(c *gin.Context) {
	empresaID, rol, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	f := FiltrosCola{
		Q:                 c.Query("q"),
		SedeID:            c.Query("sede_id"),
		FormaPagoID:       c.Query("forma_pago_id"),
		AsociacionID:      c.Query("asociacion_id"),
		Tramo:             c.Query("tramo"),
		SinGestionar:      c.Query("sin_gestionar") == "true",
		PromesaIncumplida: c.Query("promesa_incumplida") == "true",
		ParaSuspender:     c.Query("para_suspender") == "true",
		TarjetaVencida:    c.Query("tarjeta_vencida") == "true",
		TarjetaPorVencer:  c.Query("tarjeta_por_vencer") == "true",
		Morosa:            c.Query("morosa") == "true",
		Arreglo:           c.Query("arreglo"),
		Orden:             c.Query("orden"),
		Page:              atoiDefault(c.Query("page"), 1),
		PageSize:          atoiDefault(c.Query("page_size"), 50),
	}
	lista, err := h.svc.ColaDeCobro(c.Request.Context(), empresaID, rol, usuarioID, f)
	if err != nil {
		h.error(c, err, "cola")
		return
	}
	if lista.Items == nil {
		lista.Items = []FilaCola{}
	}
	c.JSON(http.StatusOK, lista)
}

// CatalogosGestion GET /v1/cxc/gestiones/catalogos — canales y resultados del formulario.
func (h *Handler) CatalogosGestion(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	cat, err := h.svc.CatalogosGestion(c.Request.Context(), empresaID)
	if err != nil {
		h.error(c, err, "catalogos-gestion")
		return
	}
	c.JSON(http.StatusOK, cat)
}

// GestionRequest es el cuerpo de una gestión registrada desde la pantalla.
type GestionRequest struct {
	Contrato     string `json:"contrato" binding:"required"`
	CanalID      string `json:"canal_id" binding:"required,uuid"`
	ResultadoID  string `json:"resultado_id" binding:"required,uuid"`
	Notas        string `json:"notas"`
	PromesaFecha string `json:"promesa_fecha"`
	PromesaMonto string `json:"promesa_monto"`
}

// RegistrarGestion POST /v1/cxc/gestiones — anota una llamada, un mensaje o una visita.
func (h *Handler) RegistrarGestion(c *gin.Context) {
	empresaID, rol, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req GestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.RegistrarGestion(c.Request.Context(), empresaID, rol, usuarioID, GestionInput{
		Contrato: req.Contrato, CanalID: req.CanalID, ResultadoID: req.ResultadoID,
		Notas: req.Notas, PromesaFecha: req.PromesaFecha, PromesaMonto: req.PromesaMonto,
	})
	if err != nil {
		h.error(c, err, "registrar-gestion")
		return
	}
	c.JSON(http.StatusCreated, res)
}

// Gestiones GET /v1/cxc/contratos/:numero/gestiones — el historial del contrato.
func (h *Handler) Gestiones(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.GestionesDeContrato(c.Request.Context(), empresaID, c.Param("numero"))
	if err != nil {
		h.error(c, err, "gestiones")
		return
	}
	if lista == nil {
		lista = []GestionFila{}
	}
	c.JSON(http.StatusOK, lista)
}
