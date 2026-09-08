package bancos

// Consulta por segmento — la capa HTTP.
//
// La puerta del equipo es propia y no el listado de movimientos que ya existe. Ese endpoint lo usan
// cuatro superficies (Clasificar, el dashboard, el exportador y la entrada de Inventario) y
// agregarle un recorte por alcance es la clase de cambio que un día le muestra a alguien lo que no
// debía. Acá el recorte no es una opción: se aplica siempre, en el servicio.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// MiSegmento GET /v1/bancos/mi-segmento/movimientos (bancos.ver_mi_segmento)
//
// Acepta los mismos filtros de la hoja de trabajo —mes, cuenta, búsqueda— porque son los que el
// equipo necesita para contestar «¿entró la planilla de setiembre?». Lo que NO acepta es ensanchar
// el alcance: el servicio lo sobreescribe con el del rol.
func (h *Handler) MiSegmento(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	f := filtrosDeQuery(c)
	if !abortarSiFiltrosInvalidos(c, f) {
		return
	}
	f.Page = atoiDefault(c.Query("page"), 1)
	f.PageSize = atoiDefault(c.Query("page_size"), 100)

	res, err := h.svc.MiSegmento(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(), f)
	// «Tu rol no tiene partidas asignadas» NO es un error: es el estado inicial de todo rol nuevo,
	// y devolverlo como 4xx haría que la pantalla mostrara «algo falló» cuando lo único que pasa es
	// que falta una marca en el catálogo. Va 200 con la lista vacía y el aviso.
	if errors.Is(err, ErrSinAlcance) {
		c.JSON(http.StatusOK, gin.H{
			"partidas":      []PartidaDelSegmento{},
			"cuentas":       []CuentaDelSegmento{},
			"movimientos":   res.Movimientos,
			"cargado_hasta": "",
			"sin_alcance":   true,
			"aviso":         "Tu rol todavía no tiene partidas asignadas para consulta. Pedile a Dirección Financiera que las marque en el catálogo de Bancos.",
		})
		return
	}
	if err != nil {
		h.responderError(c, err, "mi-segmento")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"partidas":      res.Partidas,
		"cuentas":       res.Cuentas,
		"movimientos":   res.Movimientos,
		"cargado_hasta": res.CargadoHasta,
		"sin_alcance":   false,
	})
}

type reportarSegmentacionRequest struct {
	MovimientoID string `json:"movimiento_id" validate:"required"`
	Motivo       string `json:"motivo" validate:"required"`
}

// ReportarSegmentacion POST /v1/bancos/mi-segmento/reportes (bancos.ver_mi_segmento)
//
// Avisar es parte de consultar: sin esto la pantalla solo sirve para mirar, y el equipo vuelve a
// avisar por WhatsApp —que es donde el aviso se pierde—. Por eso no lleva un permiso aparte.
func (h *Handler) ReportarSegmentacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req reportarSegmentacionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.MovimientoID == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"hacen falta el movimiento y el motivo")
		return
	}
	if !pareceUUID(req.MovimientoID) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el movimiento no tiene un identificador válido")
		return
	}
	if err := h.svc.ReportarSegmentacion(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(),
		req.MovimientoID, req.Motivo); err != nil {
		h.responderError(c, err, "reportar-segmentacion")
		return
	}
	c.Status(http.StatusCreated)
}

type buscarFaltanteRequest struct {
	Fecha string `json:"fecha" validate:"required"`
	Monto string `json:"monto" validate:"required"`
}

// BuscarFaltante POST /v1/bancos/mi-segmento/buscar (bancos.ver_mi_segmento)
//
// POST y no GET aunque solo lea: cada consulta deja un evento de auditoría, y el monto no tiene por
// qué viajar en la URL —de donde termina en los registros del servidor y en el historial del
// navegador—.
func (h *Handler) BuscarFaltante(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req buscarFaltanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hacen falta la fecha y el monto")
		return
	}
	res, err := h.svc.BuscarFaltante(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(),
		req.Fecha, req.Monto)
	if err != nil {
		h.responderError(c, err, "buscar-faltante")
		return
	}
	c.JSON(http.StatusOK, res)
}

type reportarFaltanteRequest struct {
	Fecha      string `json:"fecha" validate:"required"`
	Monto      string `json:"monto" validate:"required"`
	Referencia string `json:"referencia"`
	Motivo     string `json:"motivo" validate:"required"`
}

// ReportarFaltante POST /v1/bancos/mi-segmento/faltantes (bancos.ver_mi_segmento)
func (h *Handler) ReportarFaltante(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req reportarFaltanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"hacen falta la fecha, el monto y qué esperabas")
		return
	}
	if err := h.svc.ReportarFaltante(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(),
		req.Fecha, req.Monto, req.Referencia, req.Motivo); err != nil {
		h.responderError(c, err, "reportar-faltante")
		return
	}
	c.Status(http.StatusCreated)
}

// AlcanceConsulta GET /v1/bancos/catalogo/consulta (bancos.catalogo)
//
// Los roles que se pueden elegir y lo que ya está asignado, en una sola llamada: la columna del
// catálogo se dibuja completa sin encadenar pedidos.
func (h *Handler) AlcanceConsulta(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.AlcanceConsulta(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "alcance-consulta")
		return
	}
	c.JSON(http.StatusOK, res)
}

type guardarConsultaRequest struct {
	// RolIDs es la lista COMPLETA de roles que consultan la partida. Una lista vacía la deja sin
	// nadie, que es el estado normal de casi todas las partidas.
	RolIDs []string `json:"rol_ids"`
}

// GuardarConsultaDePartida PUT /v1/bancos/catalogo/clasificaciones/:id/consulta (bancos.catalogo)
//
// PUT y no PATCH porque reemplaza el conjunto: la fila del catálogo se piensa como «esta partida la
// ven estos», y mandar la lista completa evita el problema clásico de no poder quitar el último.
func (h *Handler) GuardarConsultaDePartida(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req guardarConsultaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	for _, id := range req.RolIDs {
		if !pareceUUID(id) {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
				"uno de los roles no tiene un identificador válido")
			return
		}
	}
	if err := h.svc.GuardarConsultaDePartida(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		req.RolIDs, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "guardar-consulta-partida")
		return
	}
	c.Status(http.StatusNoContent)
}

// ReportesSegmentacion GET /v1/bancos/reportes-segmentacion?pendientes=1
//
// La cola de quien clasifica. Vive en Clasificar y no en una bandeja aparte: un movimiento
// reportado es un movimiento a reclasificar, y ahí ya están el motor, las reglas y el masivo.
func (h *Handler) ReportesSegmentacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.ReportesSegmentacion(c.Request.Context(), claims.EmpresaID, c.Query("pendientes") == "1")
	if err != nil {
		h.responderError(c, err, "reportes-segmentacion")
		return
	}
	if res == nil {
		res = []ReporteSegmentacion{}
	}
	c.JSON(http.StatusOK, gin.H{"reportes": res})
}

type resolverReporteRequest struct {
	Resolucion string `json:"resolucion" validate:"required"`
	Respuesta  string `json:"respuesta"`
}

// ResolverReporte POST /v1/bancos/reportes-segmentacion/:id/resolver (bancos.clasificar)
//
// Pide `bancos.clasificar` y no solo la lectura: cerrar un aviso es decir «la partida está bien» o
// «ya la corregí», y las dos son afirmaciones de quien clasifica.
func (h *Handler) ResolverReporte(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req resolverReporteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := h.svc.ResolverReporte(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		claims.UsuarioID(), req.Resolucion, req.Respuesta); err != nil {
		h.responderError(c, err, "resolver-reporte-segmentacion")
		return
	}
	c.Status(http.StatusNoContent)
}
