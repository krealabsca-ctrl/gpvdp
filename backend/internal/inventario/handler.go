package inventario

// Endpoints del inventario. El handler solo parsea, llama al servicio y traduce el error a HTTP:
// cero SQL y cero reglas de negocio.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone el inventario por HTTP.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

// claims saca los claims o corta con 401.
func claims(c *gin.Context) (*auth.Claims, bool) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return nil, false
	}
	return cl, true
}

// responder traduce el error de dominio al HTTP correcto.
//
// Los errores que llevan datos (sin existencia, unidad no disponible) van con su mensaje completo:
// ya están redactados con los números y el nombre, que es justo lo que hace falta para arreglarlo.
func (h *Handler) responder(c *gin.Context, err error, donde string) {
	var sinExistencia *SinExistenciaError
	var noDisponible *UnidadNoDisponibleError
	var otraSede *UnidadEnOtraSedeError
	var sinExplicar *DiferenciasSinExplicarError
	var sinContar *SinContarError
	var soloPorUnidad *ConsignacionSoloPorUnidadError
	switch {
	case errors.As(err, &soloPorUnidad):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, soloPorUnidad.Error())
	case errors.As(err, &sinExistencia):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinExistencia.Error())
	case errors.As(err, &noDisponible):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, noDisponible.Error())
	case errors.As(err, &otraSede):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, otraSede.Error())
	// Los dos rechazos del cierre de un conteo. Van con su mensaje completo: ya nombran qué falta
	// (cuántas líneas, cuáles artículos), que es lo único que sirve para terminar la hoja.
	case errors.As(err, &sinExplicar):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinExplicar.Error())
	case errors.As(err, &sinContar):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinContar.Error())
	case errors.Is(err, ErrArticuloNoEncontrado), errors.Is(err, ErrCategoriaNoEncontrada),
		errors.Is(err, ErrUnidadNoEncontrada), errors.Is(err, ErrTrasladoNoEncontrado),
		errors.Is(err, ErrServicioNoEncontrado), errors.Is(err, ErrConteoNoEncontrado),
		errors.Is(err, ErrLineaNoEncontrada), errors.Is(err, ErrDocumentoRealNoEncontrado):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, sinPrefijo(err))
	case errors.Is(err, ErrDuplicado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, sinPrefijo(err))
	case errors.Is(err, ErrNombreRequerido), errors.Is(err, ErrCodigoRequerido),
		errors.Is(err, ErrModoInvalido), errors.Is(err, ErrCantidadInvalida),
		errors.Is(err, ErrCostoNegativo), errors.Is(err, ErrSedeRequerida),
		errors.Is(err, ErrFechaInvalida), errors.Is(err, ErrMismaSede),
		errors.Is(err, ErrEstadoUnidadInvalido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, sinPrefijo(err))
	case errors.Is(err, ErrMotivoRequerido), errors.Is(err, ErrTrasladoYaRecibido),
		errors.Is(err, ErrConteoYaCerrado), errors.Is(err, ErrConteoAbiertoEnLaSede),
		errors.Is(err, ErrConteoSinLineas), errors.Is(err, ErrMotivoAnularRequerido),
		errors.Is(err, ErrServicioSinConsumos), errors.Is(err, ErrProveedorConsignadaRequerido),
		errors.Is(err, ErrNoEsConsignada), errors.Is(err, ErrConsignadaEnBodega),
		errors.Is(err, ErrConsignadaEnTransito),
		errors.Is(err, ErrConsignadaSinFactura), errors.Is(err, ErrDocumentoRealEsLaProvision),
		// Consignación: los rechazos que protegen la plata. Todos van con su mensaje completo porque
		// ya nombran el caso; un «no se puede» a secas obliga a adivinar qué pasó.
		errors.Is(err, ErrCostoCeroNoSeFactura), errors.Is(err, ErrSalidaNoFacturableSola),
		errors.Is(err, ErrDevueltaNoSeFactura), errors.Is(err, ErrDocumentoRealDeOtroProveedor),
		errors.Is(err, ErrDocumentoRealAnulado), errors.Is(err, ErrCxPRechazo):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijo(err))
	case errors.Is(err, ErrConsignadaYaFacturada), errors.Is(err, ErrYaConciliada),
		errors.Is(err, ErrProvisionYaExisteEnCxP), errors.Is(err, ErrDocumentoRealYaEnlazado),
		errors.Is(err, ErrProvisionYaNoAnulable):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, sinPrefijo(err))
	case errors.Is(err, ErrSituacionInvalida), errors.Is(err, ErrDocumentoRealRequerido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, sinPrefijo(err))
	// El módulo de CxP no conectado es un problema de configuración del servidor, no del usuario:
	// 503 y no 500, porque el pedido está bien y lo que falta es una pieza del entorno.
	case errors.Is(err, ErrSinFacturadorCxP):
		h.log.Error("inventario " + donde + ": sin facturador de CxP conectado")
		httpx.Abort(c, http.StatusServiceUnavailable, httpx.CodeErrorInterno, sinPrefijo(err))
	default:
		h.log.Error("inventario "+donde, zap.Error(err))
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}

// sinPrefijo quita el «inventario: » para que el mensaje se lea como una frase.
func sinPrefijo(err error) string { return strings.TrimPrefix(err.Error(), "inventario: ") }

// pareceUUID comprueba la FORMA del id antes de que llegue al SQL.
//
// Sin esto, un id mal escrito viaja hasta el casteo ::uuid de Postgres, revienta ahí y el error sale
// como 500 «error interno»: el usuario ve una caída del servidor cuando lo único que pasó es que
// pegó mal un identificador. Es el mismo helper que bancos.pareceUUID; está duplicado a propósito
// porque compartirlo obligaría a que un módulo importe al otro solo por seis líneas.
func pareceUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		ch := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return false
			}
			continue
		}
		esHex := (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
		if !esHex {
			return false
		}
	}
	return true
}

// uuidDeRuta saca un id de la ruta y corta con 400 si no tiene forma de uuid.
func uuidDeRuta(c *gin.Context, param, queEs string) (string, bool) {
	v := c.Param(param)
	if !pareceUUID(v) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el identificador de "+queEs+" no tiene forma de uuid")
		return "", false
	}
	return v, true
}

// uuidDeQuery valida un filtro opcional: vacío se acepta, mal escrito se rechaza en el borde.
func uuidDeQuery(c *gin.Context, clave string) (string, bool) {
	v := c.Query(clave)
	if v == "" {
		return "", true
	}
	if !pareceUUID(v) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			clave+" no tiene forma de uuid")
		return "", false
	}
	return v, true
}

// boolOpcional distingue TRES respuestas: sí, no, y «no filtres». Un `== "true"` colapsaría las dos
// últimas y no habría forma de pedir «solo las propias»: pedirlo devolvería todo.
func boolOpcional(c *gin.Context, clave string) *bool {
	v := c.Query(clave)
	if v != "true" && v != "false" {
		return nil
	}
	b := v == "true"
	return &b
}

func entero(c *gin.Context, clave string, porDefecto int) int {
	if v := c.Query(clave); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return porDefecto
}

// ── Catálogo ────────────────────────────────────────────────────────────────

// Categorias GET /v1/inventario/categorias (inventario.ver)
func (h *Handler) Categorias(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.Categorias(c.Request.Context(), cl.EmpresaID, c.Query("incluir_inactivas") == "true")
	if err != nil {
		h.responder(c, err, "categorias")
		return
	}
	c.JSON(http.StatusOK, list)
}

type categoriaRequest struct {
	Nombre  string `json:"nombre" binding:"required"`
	PadreID string `json:"padre_id"`
	Activo  bool   `json:"activo"`
}

// CrearCategoria POST /v1/inventario/categorias (inventario.catalogo)
func (h *Handler) CrearCategoria(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req categoriaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el nombre de la categoría es obligatorio")
		return
	}
	cat, err := h.svc.CrearCategoria(c.Request.Context(), cl.EmpresaID, req.PadreID, req.Nombre, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "crear-categoria")
		return
	}
	c.JSON(http.StatusCreated, cat)
}

// ActualizarCategoria PATCH /v1/inventario/categorias/:id (inventario.catalogo)
func (h *Handler) ActualizarCategoria(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req categoriaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el nombre de la categoría es obligatorio")
		return
	}
	if err := h.svc.ActualizarCategoria(c.Request.Context(), cl.EmpresaID, c.Param("id"),
		req.Nombre, req.Activo, cl.UsuarioID()); err != nil {
		h.responder(c, err, "actualizar-categoria")
		return
	}
	c.Status(http.StatusNoContent)
}

// Articulos GET /v1/inventario/articulos (inventario.ver)
func (h *Handler) Articulos(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.Articulos(c.Request.Context(), cl.EmpresaID, FiltroArticulos{
		CategoriaID:      c.Query("categoria_id"),
		ModoControl:      c.Query("modo_control"),
		Q:                c.Query("q"),
		IncluirInactivos: c.Query("incluir_inactivos") == "true",
	})
	if err != nil {
		h.responder(c, err, "articulos")
		return
	}
	c.JSON(http.StatusOK, list)
}

// CrearArticulo POST /v1/inventario/articulos (inventario.catalogo)
func (h *Handler) CrearArticulo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req ArticuloNuevo
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	id, err := h.svc.CrearArticulo(c.Request.Context(), cl.EmpresaID, req, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "crear-articulo")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ActualizarArticulo PATCH /v1/inventario/articulos/:id (inventario.catalogo)
func (h *Handler) ActualizarArticulo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req ArticuloNuevo
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := h.svc.ActualizarArticulo(c.Request.Context(), cl.EmpresaID, c.Param("id"), req, cl.UsuarioID()); err != nil {
		h.responder(c, err, "actualizar-articulo")
		return
	}
	c.Status(http.StatusNoContent)
}

type nivelRequest struct {
	SedeID string `json:"sede_id" binding:"required,uuid"`
	Minimo int    `json:"minimo"`
	Maximo int    `json:"maximo"`
}

// FijarNivel PUT /v1/inventario/articulos/:id/nivel (inventario.catalogo)
func (h *Handler) FijarNivel(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req nivelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hacen falta sede_id (uuid), minimo y maximo")
		return
	}
	if err := h.svc.FijarNivel(c.Request.Context(), cl.EmpresaID, c.Param("id"), req.SedeID,
		req.Minimo, req.Maximo, cl.UsuarioID()); err != nil {
		h.responder(c, err, "fijar-nivel")
		return
	}
	c.Status(http.StatusNoContent)
}

// NivelesDeArticulo GET /v1/inventario/articulos/:id/niveles (inventario.ver)
func (h *Handler) NivelesDeArticulo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.NivelesDeArticulo(c.Request.Context(), cl.EmpresaID, c.Param("id"))
	if err != nil {
		h.responder(c, err, "niveles")
		return
	}
	c.JSON(http.StatusOK, list)
}

// ── Existencias ─────────────────────────────────────────────────────────────

// Existencias GET /v1/inventario/existencias (inventario.ver)
func (h *Handler) Existencias(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	res, err := h.svc.Existencias(c.Request.Context(), cl.EmpresaID, FiltroExistencias{
		SedeID:          c.Query("sede_id"),
		CategoriaID:     c.Query("categoria_id"),
		ModoControl:     c.Query("modo_control"),
		Q:               c.Query("q"),
		SoloBajoMinimo:  c.Query("solo_bajo_minimo") == "true",
		SoloConsignadas: c.Query("solo_consignadas") == "true",
	})
	if err != nil {
		h.responder(c, err, "existencias")
		return
	}
	c.JSON(http.StatusOK, res)
}

// Unidades GET /v1/inventario/unidades (inventario.ver)
func (h *Handler) Unidades(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.Unidades(c.Request.Context(), cl.EmpresaID, FiltroUnidades{
		ArticuloID:       c.Query("articulo_id"),
		SedeID:           c.Query("sede_id"),
		Estado:           c.Query("estado"),
		Q:                c.Query("q"),
		QuietasDesdeDias: entero(c, "quietas_desde_dias", 0),
		Limite:           entero(c, "limite", 200),
		Consignada:       boolOpcional(c, "consignada"),
	})
	if err != nil {
		h.responder(c, err, "unidades")
		return
	}
	c.JSON(http.StatusOK, list)
}

// Movimientos GET /v1/inventario/movimientos (inventario.ver)
func (h *Handler) Movimientos(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.Movimientos(c.Request.Context(), cl.EmpresaID, FiltroMovimientos{
		ArticuloID: c.Query("articulo_id"),
		UnidadID:   c.Query("unidad_id"),
		SedeID:     c.Query("sede_id"),
		Tipo:       c.Query("tipo"),
		Desde:      c.Query("desde"),
		Hasta:      c.Query("hasta"),
		Limite:     entero(c, "limite", 300),
	})
	if err != nil {
		h.responder(c, err, "movimientos")
		return
	}
	c.JSON(http.StatusOK, list)
}

// ── Operaciones ─────────────────────────────────────────────────────────────

// RegistrarEntrada POST /v1/inventario/entradas (inventario.entrada)
func (h *Handler) RegistrarEntrada(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req EntradaNueva
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	hecha, err := h.svc.RegistrarEntrada(c.Request.Context(), cl.EmpresaID, req, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "entrada")
		return
	}
	c.JSON(http.StatusCreated, hecha)
}

// RegistrarServicio POST /v1/inventario/servicios (inventario.servicio)
//
// Es el endpoint que descarga el inventario: sin esto, las existencias solo suben.
func (h *Handler) RegistrarServicio(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req ServicioNuevo
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	sv, err := h.svc.RegistrarServicio(c.Request.Context(), cl.EmpresaID, req, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "servicio")
		return
	}
	c.JSON(http.StatusCreated, sv)
}

// Servicios GET /v1/inventario/servicios (inventario.ver)
func (h *Handler) Servicios(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.Servicios(c.Request.Context(), cl.EmpresaID, c.Query("desde"), c.Query("hasta"))
	if err != nil {
		h.responder(c, err, "servicios")
		return
	}
	c.JSON(http.StatusOK, list)
}

// RegistrarAjuste POST /v1/inventario/ajustes (inventario.ajuste)
func (h *Handler) RegistrarAjuste(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req AjusteNuevo
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := h.svc.RegistrarAjuste(c.Request.Context(), cl.EmpresaID, req, cl.UsuarioID()); err != nil {
		h.responder(c, err, "ajuste")
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Traslados ───────────────────────────────────────────────────────────────

// CrearTraslado POST /v1/inventario/traslados (inventario.traslado)
func (h *Handler) CrearTraslado(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req TrasladoNuevo
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	t, err := h.svc.CrearTraslado(c.Request.Context(), cl.EmpresaID, req, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "crear-traslado")
		return
	}
	c.JSON(http.StatusCreated, t)
}

// RecibirTraslado POST /v1/inventario/traslados/:id/recibir (inventario.traslado)
func (h *Handler) RecibirTraslado(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var req struct {
		Fecha string `json:"fecha"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.RecibirTraslado(c.Request.Context(), cl.EmpresaID, c.Param("id"), req.Fecha, cl.UsuarioID()); err != nil {
		h.responder(c, err, "recibir-traslado")
		return
	}
	c.Status(http.StatusNoContent)
}

// Traslados GET /v1/inventario/traslados (inventario.ver)
func (h *Handler) Traslados(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.Traslados(c.Request.Context(), cl.EmpresaID, c.Query("estado"))
	if err != nil {
		h.responder(c, err, "traslados")
		return
	}
	c.JSON(http.StatusOK, list)
}

// ── Análisis ────────────────────────────────────────────────────────────────

// Reposicion GET /v1/inventario/reposicion (inventario.ver)
func (h *Handler) Reposicion(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	list, err := h.svc.Reposicion(c.Request.Context(), cl.EmpresaID, c.Query("sede_id"), entero(c, "semanas", 8))
	if err != nil {
		h.responder(c, err, "reposicion")
		return
	}
	c.JSON(http.StatusOK, list)
}

// Rotacion GET /v1/inventario/rotacion (inventario.ver)
func (h *Handler) Rotacion(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	desde, hasta := c.Query("desde"), c.Query("hasta")
	if hasta == "" {
		hasta = ahoraCR().Format("2006-01-02")
	}
	if desde == "" {
		// Un año hacia atrás por defecto: con menos, un artículo de rotación baja parecería detenido.
		desde = ahoraCR().AddDate(-1, 0, 0).Format("2006-01-02")
	}
	list, err := h.svc.Rotacion(c.Request.Context(), cl.EmpresaID, desde, hasta)
	if err != nil {
		h.responder(c, err, "rotacion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"desde": desde, "hasta": hasta, "filas": list})
}
