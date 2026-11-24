package conta

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone el plan de cuentas por HTTP.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

// Plan GET /v1/conta/cuentas?activas=1&formato=arbol (conta.plan_cuentas)
//
// Devuelve el árbol por defecto, que es como se mira un plan de cuentas. `formato=plano` da la
// lista, que es lo que necesita un selector.
func (h *Handler) Plan(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	soloActivas := c.Query("activas") == "1"
	if c.Query("formato") == "plano" {
		cuentas, err := h.svc.Cuentas(c.Request.Context(), cl.EmpresaID, soloActivas)
		if err != nil {
			h.responder(c, err, "plan")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": cuentas})
		return
	}
	arbol, err := h.svc.Arbol(c.Request.Context(), cl.EmpresaID, soloActivas)
	if err != nil {
		h.responder(c, err, "plan")
		return
	}
	c.JSON(http.StatusOK, gin.H{"arbol": arbol})
}

// CrearCuenta POST /v1/conta/cuentas (conta.plan_cuentas)
func (h *Handler) CrearCuenta(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req Cuenta
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "la cuenta no se pudo leer")
		return
	}
	creada, err := h.svc.CrearCuenta(c.Request.Context(), cl.EmpresaID, req, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "crear-cuenta")
		return
	}
	c.JSON(http.StatusCreated, creada)
}

// ActualizarCuenta PATCH /v1/conta/cuentas/:id (conta.plan_cuentas)
func (h *Handler) ActualizarCuenta(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req Cuenta
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "la cuenta no se pudo leer")
		return
	}
	req.ID = c.Param("id")
	act, err := h.svc.ActualizarCuenta(c.Request.Context(), cl.EmpresaID, req, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "actualizar-cuenta")
		return
	}
	c.JSON(http.StatusOK, act)
}

// BorrarCuenta DELETE /v1/conta/cuentas/:id (conta.plan_cuentas)
func (h *Handler) BorrarCuenta(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.BorrarCuenta(c.Request.Context(), cl.EmpresaID, c.Param("id"), cl.UsuarioID()); err != nil {
		h.responder(c, err, "borrar-cuenta")
		return
	}
	c.Status(http.StatusNoContent)
}

// PrevisualizarPlan POST /v1/conta/cuentas/previsualizar (conta.plan_cuentas)
//
// Multipart con el Excel. NO escribe nada: dice qué pasaría. Es el mismo criterio que el
// importador de bancos —primero se ve, después se confirma—, y acá pesa más: un plan de cuentas
// mal cargado se arrastra a todos los asientos que vengan después.
func (h *Handler) PrevisualizarPlan(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	archivo, err := archivoDeFormulario(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	prev, err := h.svc.Previsualizar(c.Request.Context(), cl.EmpresaID, archivo)
	if err != nil {
		h.responder(c, err, "previsualizar-plan")
		return
	}
	c.JSON(http.StatusOK, prev)
}

// ImportarPlan POST /v1/conta/cuentas/importar (conta.plan_cuentas)
//
// `destino=grupo` carga las cuentas como del GRUPO (las tres empresas las comparten);
// `destino=empresa` las carga para la empresa activa. Es la decisión D1 del mandato, tomada al
// cargar y no al diseñar: cambiar de opinión es volver a importar.
func (h *Handler) ImportarPlan(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	archivo, err := archivoDeFormulario(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	alGrupo := c.PostForm("destino") == "grupo"
	res, err := h.svc.Importar(c.Request.Context(), cl.EmpresaID, archivo, alGrupo, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "importar-plan")
		return
	}
	c.JSON(http.StatusOK, res)
}

func archivoDeFormulario(c *gin.Context) ([]byte, error) {
	fh, err := c.FormFile("archivo")
	if err != nil {
		return nil, errors.New("subí el Excel del plan de cuentas")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, errors.New("no se pudo leer el archivo")
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, errors.New("no se pudo leer el archivo")
	}
	return data, nil
}

func (h *Handler) responder(c *gin.Context, err error, op string) {
	switch {
	// Los de validación van con su texto completo: cada uno explica QUÉ falta, y es lo único que
	// tiene quien está cargando un plan de 258 cuentas.
	case errors.Is(err, ErrCodigoRequerido), errors.Is(err, ErrNombreRequerido),
		errors.Is(err, ErrClaseDesconocida),
		errors.Is(err, ErrMotivoRequerido), errors.Is(err, ErrCuentaRequerida),
		errors.Is(err, ErrEstadoInvalido),
		// Los del asiento. Van con su texto completo porque cada uno dice qué corregir, y quien
		// carga un asiento a mano no tiene otra forma de saberlo.
		errors.Is(err, ErrGlosaRequerida), errors.Is(err, ErrFechaAsientoRequerida),
		errors.Is(err, ErrFechaInvalida), errors.Is(err, ErrMontoInvalido),
		errors.Is(err, ErrMontoNegativo), errors.Is(err, ErrLineaSinMonto),
		errors.Is(err, ErrLineaDebeYHaber), errors.Is(err, ErrMotivoAnulacionRequerido),
		errors.Is(err, ErrPeriodoInvalido), errors.Is(err, ErrMotivoReversaRequerido),
		errors.Is(err, ErrMotivoReaperturaRequerido),
		// Los del .xlsx del mapeo: los tres dicen qué hacer con el archivo —cambiarlo, llenarlo o
		// volver a bajar la plantilla—, así que el texto completo es justamente lo que sirve.
		errors.Is(err, ErrMapeoArchivoIlegible), errors.Is(err, ErrMapeoSinEncabezado),
		errors.Is(err, ErrMapeoSinFilas):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, sinPrefijo(err))
	// Nada que generar no es un error de forma: el pedido está bien y lo que falta son datos.
	case errors.Is(err, ErrNadaQueGenerar), errors.Is(err, ErrAperturaVacia):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijo(err))
	// Las del período: el pedido está bien escrito y lo que no se cumple es la regla del libro.
	case errors.Is(err, ErrPeriodoNoCuadra), errors.Is(err, ErrAperturaDescuadrada),
		errors.Is(err, ErrPeriodoEstaCerrado):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijo(err))
	case errors.Is(err, ErrPeriodoCerradoYa), errors.Is(err, ErrAperturaYaExiste):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, sinPrefijo(err))
	case errors.Is(err, ErrPeriodoNoCerrado):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, sinPrefijo(err))
	case errors.Is(err, ErrCuentaNoEncontrada), errors.Is(err, ErrMapeoNoEncontrado),
		errors.Is(err, ErrAsientoNoEncontrado), errors.Is(err, ErrCuentaBancariaNoMapeada):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, sinPrefijo(err))
	case errors.Is(err, ErrAsientoYaAnulado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, sinPrefijo(err))
	case errors.Is(err, ErrCodigoDuplicado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, sinPrefijo(err))
	// Las dos reglas del árbol son de negocio, no de forma: el pedido está bien escrito y lo que
	// no se cumple es la regla del plan de cuentas. La de agrupación es la misma regla vista desde
	// el mapeo: la cuenta existe y el pedido es válido, pero esa cuenta no admite asientos.
	case errors.Is(err, ErrPadreAceptaMovimiento), errors.Is(err, ErrTieneHijas),
		errors.Is(err, ErrCuentaConMovimientos), errors.Is(err, ErrCuentaDeAgrupacion),
		// La partida doble es la regla del libro: el pedido está bien escrito y lo que no se
		// cumple es que el asiento cuadre.
		errors.Is(err, ErrAsientoDescuadrado), errors.Is(err, ErrAsientoSinLineas):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijo(err))
	default:
		// Un error del lector del Excel llega acá con su texto, que ya dice qué columna faltó.
		if strings.HasPrefix(err.Error(), "conta: ") {
			httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijo(err))
			return
		}
		httpx.AbortInterno(c, h.log, "conta "+op, err)
	}
}

func sinPrefijo(err error) string {
	return strings.TrimPrefix(err.Error(), "conta: ")
}
