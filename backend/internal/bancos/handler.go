package bancos

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// maxEstadoCuenta es el tope del archivo de estado de cuenta que se lee en memoria (24 MiB,
// igual al límite del borde en Caddy).
const maxEstadoCuenta = 24 << 20

// Handler expone los endpoints del importador (bajo RequireEmpresa).
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler del importador.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Cuentas GET /v1/bancos/cuentas — cuentas de la empresa activa (selector del importador).
func (h *Handler) Cuentas(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	list, err := h.svc.Cuentas(c.Request.Context(), claims.EmpresaID, c.Query("incluir_inactivas") == "true")
	if err != nil {
		h.responderError(c, err, "cuentas")
		return
	}
	if list == nil {
		list = []CuentaListItem{}
	}
	c.JSON(http.StatusOK, list)
}

// Subir POST /v1/bancos/importaciones — multipart (cuenta_bancaria_id + archivo). Parsea y previsualiza.
func (h *Handler) Subir(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	cuentaID := c.PostForm("cuenta_bancaria_id")
	if cuentaID == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuenta_bancaria_id requerido")
		return
	}
	fh, err := c.FormFile("archivo")
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "archivo requerido")
		return
	}
	// Tope de tamaño: el estado de cuenta se lee entero en memoria. Sin este freno un archivo
	// enorme (por error o a propósito) infla la memoria del backend. Caddy ya corta a 24 MB en
	// el borde; este es el mismo límite como defensa en profundidad si se llega al backend directo.
	if fh.Size > maxEstadoCuenta {
		httpx.Abort(c, http.StatusRequestEntityTooLarge, httpx.CodeValidacion, "el archivo excede 24 MB")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxEstadoCuenta))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}

	res, err := h.svc.Preview(c.Request.Context(), claims.EmpresaID, cuentaID, fh.Filename, data, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "subir")
		return
	}
	c.JSON(http.StatusCreated, res)
}

// Preview GET /v1/bancos/importaciones/:id/preview — reconstruye la previsualización.
func (h *Handler) Preview(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.PreviewExistente(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "preview")
		return
	}
	c.JSON(http.StatusOK, res)
}

type confirmarRequest struct {
	// Excluir: natural_key de las líneas que el usuario decide NO incluir (duplicados reales).
	Excluir []string `json:"excluir"`
}

// Confirmar POST /v1/bancos/importaciones/:id/confirmar — persiste los movimientos.
func (h *Handler) Confirmar(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req confirmarRequest
	_ = c.ShouldBindJSON(&req) // cuerpo opcional (sin exclusiones)

	n, err := h.svc.Confirmar(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Excluir, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "confirmar")
		return
	}
	c.JSON(http.StatusOK, gin.H{"importacion_id": c.Param("id"), "insertados": n})
}

func (h *Handler) responderError(c *gin.Context, err error, op string) {
	var enUso *CatalogoEnUsoError
	var noPermitido *CambioNoPermitidoError
	var fechasIlegibles *FechasIlegiblesError
	switch {
	case errors.Is(err, ErrCuentaNoEncontrada), errors.Is(err, ErrImportacionNoEncontrada),
		errors.Is(err, ErrMovimientoNoEncontrado), errors.Is(err, ErrConceptoNoEncontrado),
		errors.Is(err, ErrBancoNoEncontrado), errors.Is(err, ErrReglaNoEncontrada),
		errors.Is(err, ErrClasificacionNoEncontrada), errors.Is(err, ErrEmpresaNoEncontrada):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrReglaSinPalabras):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, "la regla debe conservar al menos una palabra clave")
	case errors.Is(err, ErrToleranciaFueraDeRango):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, "la tolerancia de traslado debe estar entre 0% y 5%")
	case errors.Is(err, ErrBCCRNoConfigurado):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"la sincronización con el BCCR no está configurada; registrá la cotización manualmente o configurá las credenciales del BCCR")
	case errors.Is(err, ErrMonedaNoCoincide):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"la moneda del archivo no coincide con la de la cuenta seleccionada; elegí la cuenta correcta")
	case errors.Is(err, ErrFechaInvalida):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "fecha inválida (se espera YYYY-MM-DD)")
	case errors.Is(err, ErrExportacionVacia):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, "no hay datos para exportar con ese filtro")
	// Rechazos del archivo de clasificación en bloque. El mensaje ya viene redactado con qué hacer
	// («partilo por cuenta o por año», «se esperan al menos las columnas Fecha y Clasificación») y sin
	// este caso salía como 500 «error interno»: el usuario no podía distinguir un archivo rechazado de
	// una caída del servidor, y reintentaba el mismo archivo.
	case errors.Is(err, ErrClasifExcelSinEncabezado), errors.Is(err, ErrClasifExcelVacio),
		errors.Is(err, ErrClasifExcelDemasiadasFilas):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijoPaquete(err))
	// Lo mismo para el diccionario del catálogo: sus dos centinelas tampoco estaban mapeados.
	case errors.Is(err, ErrDiccionarioVacio), errors.Is(err, ErrDiccionarioSinEncabezado):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijoPaquete(err))
	case errors.As(err, &enUso):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"No se puede eliminar: está en uso por "+enUso.Detalle+".")
	case errors.As(err, &noPermitido):
		// El motivo ya viene redactado completo (qué pasa y qué hacer en su lugar).
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, noPermitido.Motivo)
	case errors.Is(err, ErrFusionMismaEntrada), errors.Is(err, ErrFusionOtroConcepto):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrCatalogoDuplicado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, "ya existe una entrada de catálogo con ese nombre")
	case errors.Is(err, ErrNoReconocido):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, "formato de banco no reconocido")
	case errors.As(err, &fechasIlegibles):
		// El mensaje va completo: dice que NO es un mes sin movimientos y muestra la fecha que
		// no se entendió, que es el dato con el que se arregla el adaptador.
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, fechasIlegibles.Error())
	case errors.Is(err, ErrIBANNoCoincide):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"el IBAN del archivo no coincide con la cuenta seleccionada; elegí la cuenta correcta antes de importar")
	case errors.Is(err, ErrClasificacionInvalida):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, "la clasificación no corresponde al concepto")
	case errors.Is(err, ErrCotizacionesIncompletas):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, "faltan cotizaciones del mes (día 1, 15 y último)")
	case errors.Is(err, ErrTCYaCongelado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, "el tipo de cambio del mes ya está congelado")
	case errors.Is(err, ErrPeriodoYaCerrado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, "el período ya está cerrado")
	case errors.Is(err, ErrTrasladoInvalido):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, "par de traslado inválido (misma cuenta, no opuestos o fuera de tolerancia)")
	case errors.Is(err, ErrPartidaNoEncontrada):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado,
			"la partida no existe, ya fue anulada, o el acta de ese mes ya está firmada")
	case errors.Is(err, ErrPartidaInvalida), errors.Is(err, ErrSignoRequerido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
	case errors.Is(err, ErrActaNoCuadra), errors.Is(err, ErrSaldoDelMesFaltante):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrConciliacionPendiente):
		// Detalle de cuáles cuentas faltan, para que el cierre diga qué hacer y no solo «no».
		var conc *ErrorConciliacion
		msg := "hay cuentas sin conciliar; el período no se puede cerrar hasta que todas cuadren y estén firmadas"
		if errors.As(err, &conc) {
			msg = "faltan " + strconv.Itoa(conc.Pendientes) + " acta(s) de conciliación sin firmar: " +
				strings.Join(conc.Cuentas, ", ")
		}
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, msg)
	case errors.Is(err, ErrSaldoCongelado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrSaldoInvalido), errors.Is(err, ErrSinCuentas):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
	default:
		h.log.Error("bancos "+op, zap.Error(err))
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}

// sinPrefijoPaquete quita el «bancos: » del texto del error antes de mandarlo a la pantalla.
//
// El prefijo es útil en los logs —dice de qué paquete salió— y ruido para quien lee el mensaje en la
// interfaz: lo que necesita es «no se encontró el encabezado…», no el nombre del módulo de Go.
func sinPrefijoPaquete(err error) string {
	return strings.TrimPrefix(err.Error(), "bancos: ")
}
