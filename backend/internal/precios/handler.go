package precios

// Endpoints de la calculadora. El handler solo parsea, llama al servicio y traduce el error a
// HTTP: cero SQL y cero reglas de negocio.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone la calculadora por HTTP.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

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
// Todo error de dominio del paquete tiene que tener su case acá. Si falta uno, el usuario recibe
// un 500 «error interno» cuando en realidad hizo algo que se puede corregir. El centinela de
// handler_centinelas_test.go verifica que no se escape ninguno.
func (h *Handler) responder(c *gin.Context, err error, donde string) {
	switch {
	case errors.Is(err, ErrInsumoNoEncontrado), errors.Is(err, ErrPlanNoEncontrado),
		errors.Is(err, ErrDriverNoEncontrado), errors.Is(err, ErrPoliticaNoEncontrada):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, sinPrefijo(err.Error()))
	case errors.Is(err, ErrCodigoDuplicado), errors.Is(err, ErrInsumoRepetido):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, sinPrefijo(err.Error()))
	case errors.Is(err, ErrNombreVacio), errors.Is(err, ErrCodigoVacio),
		errors.Is(err, ErrTipoCostoInvalido), errors.Is(err, ErrBaseIVAInvalida),
		errors.Is(err, ErrModalidadInvalida), errors.Is(err, ErrCostoNegativo),
		errors.Is(err, ErrCantidadNoPositiva), errors.Is(err, ErrFechaVigenciaVacia),
		errors.Is(err, ErrPrecioNegativo),
		// Del costeo por actividad: los cuatro son datos que el usuario escribe y puede corregir.
		errors.Is(err, ErrPorcentajeInvalido), errors.Is(err, ErrConsumoNoPositivo),
		errors.Is(err, ErrBaseNoPositiva), errors.Is(err, ErrPeriodoInvalido),
		// De las políticas de comisión: todos son datos que el usuario escribe y puede corregir.
		errors.Is(err, ErrTipoComisionInvalido), errors.Is(err, ErrBaseComisionInvalida),
		errors.Is(err, ErrValorComisionInvalido), errors.Is(err, ErrPorcentajeMayorACien),
		errors.Is(err, ErrVigenciaInvertida),
		// Del simulador: supuestos que el usuario escribe y puede corregir.
		errors.Is(err, ErrAjusteInvalido), errors.Is(err, ErrFrecuenciaInvalida),
		errors.Is(err, ErrSinPrecioParaSimular):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, sinPrefijo(err.Error()))
	case errors.Is(err, ErrInsumoEnUso), errors.Is(err, ErrInsumoDeOtraEmpresa),
		errors.Is(err, ErrDriverEnUso),
		errors.Is(err, ErrConfirmacionNoCoincide):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijo(err.Error()))
	// Errores del archivo que sube el usuario: son correcciones que él puede hacer, no fallas del
	// servidor, así que van con su mensaje completo y no detrás de un 500.
	case errors.Is(err, ErrArchivoVacio), errors.Is(err, ErrArchivoIlegible),
		errors.Is(err, ErrHojaNoEncontrada), errors.Is(err, ErrColumnaNombreNoHallada),
		errors.Is(err, ErrEncabezadoDePlanesNoHallado):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, sinPrefijo(err.Error()))
	default:
		httpx.AbortInterno(c, h.log, donde, err)
	}
}

// sinPrefijo quita el «precios: » de los mensajes de dominio. Adentro del código el prefijo dice
// de qué paquete viene; en pantalla solo estorba.
func sinPrefijo(s string) string { return strings.TrimPrefix(s, "precios: ") }

// Insumos devuelve el catálogo a una fecha.
func (h *Handler) Insumos(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.Insumos(c.Request.Context(), cl.EmpresaID,
		c.Query("fecha"), c.Query("busca"), c.Query("tipo"), c.Query("sin_costo") == "true")
	if err != nil {
		h.responder(c, err, "precios.insumos")
		return
	}
	// El conteo de los que no tienen costo va en la respuesta y no se calcula en el cliente:
	// es la cifra que dice si el catálogo sirve para costear, y tiene que ser la misma para todos.
	sinCosto := 0
	for _, i := range lista {
		if !i.TieneCosto() {
			sinCosto++
		}
	}
	c.JSON(http.StatusOK, gin.H{"insumos": lista, "total": len(lista), "sin_costo": sinCosto})
}

// CrearInsumo agrega al catálogo.
func (h *Handler) CrearInsumo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in InsumoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	id, err := h.svc.CrearInsumo(c.Request.Context(), cl.EmpresaID, in)
	if err != nil {
		h.responder(c, err, "precios.crear_insumo")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ActualizarInsumo edita una línea del catálogo.
func (h *Handler) ActualizarInsumo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in InsumoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.ActualizarInsumo(c.Request.Context(), cl.EmpresaID, c.Param("id"), in); err != nil {
		h.responder(c, err, "precios.actualizar_insumo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// EliminarInsumo quita del catálogo lo que no esté en uso.
func (h *Handler) EliminarInsumo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	if err := h.svc.EliminarInsumo(c.Request.Context(), cl.EmpresaID, c.Param("id")); err != nil {
		h.responder(c, err, "precios.eliminar_insumo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CapturarCosto guarda un costo para un insumo.
func (h *Handler) CapturarCosto(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in CostoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.CapturarCosto(c.Request.Context(), cl.EmpresaID, c.Param("id"), in, cl.UsuarioID()); err != nil {
		h.responder(c, err, "precios.capturar_costo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// HistorialDeCosto devuelve las capturas de un insumo.
func (h *Handler) HistorialDeCosto(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.HistorialDeCosto(c.Request.Context(), cl.EmpresaID, c.Param("id"))
	if err != nil {
		h.responder(c, err, "precios.historial_costo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"capturas": lista})
}

// Planes lista los planes.
func (h *Handler) Planes(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.Planes(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err, "precios.planes")
		return
	}
	c.JSON(http.StatusOK, gin.H{"planes": lista})
}

// CrearPlan agrega un plan.
func (h *Handler) CrearPlan(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in PlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	id, err := h.svc.CrearPlan(c.Request.Context(), cl.EmpresaID, in)
	if err != nil {
		h.responder(c, err, "precios.crear_plan")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ActualizarPlan edita un plan.
func (h *Handler) ActualizarPlan(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in PlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.ActualizarPlan(c.Request.Context(), cl.EmpresaID, c.Param("id"), in); err != nil {
		h.responder(c, err, "precios.actualizar_plan")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Costear devuelve el costeo de un plan a una fecha.
func (h *Handler) Costear(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	costeo, err := h.svc.Costear(c.Request.Context(), cl.EmpresaID, c.Param("id"), c.Query("fecha"))
	if err != nil {
		h.responder(c, err, "precios.costear")
		return
	}
	c.JSON(http.StatusOK, costeo)
}

type lineaRecetaReq struct {
	InsumoID string `json:"insumo_id"`
	Cantidad string `json:"cantidad"`
	Nota     string `json:"nota"`
}

// AgregarALaReceta pone un insumo en un plan.
func (h *Handler) AgregarALaReceta(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in lineaRecetaReq
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.AgregarALaReceta(c.Request.Context(), cl.EmpresaID, c.Param("id"),
		in.InsumoID, in.Cantidad, in.Nota); err != nil {
		h.responder(c, err, "precios.agregar_receta")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

// CambiarCantidad ajusta una línea de la receta.
func (h *Handler) CambiarCantidad(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in struct {
		Cantidad string `json:"cantidad"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.CambiarCantidad(c.Request.Context(), cl.EmpresaID, c.Param("lineaID"), in.Cantidad); err != nil {
		h.responder(c, err, "precios.cambiar_cantidad")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// QuitarDeLaReceta saca una línea.
func (h *Handler) QuitarDeLaReceta(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	if err := h.svc.QuitarDeLaReceta(c.Request.Context(), cl.EmpresaID, c.Param("lineaID")); err != nil {
		h.responder(c, err, "precios.quitar_receta")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
