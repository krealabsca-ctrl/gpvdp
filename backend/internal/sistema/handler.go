package sistema

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

// Handler expone la consulta de incidentes por HTTP.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Incidentes GET /v1/sistema/incidentes (admin.incidentes)
//
// `ref` es la búsqueda principal: el usuario dicta seis caracteres y acá se canjean por la causa.
// La empresa sale del TOKEN, nunca del pedido: no hay forma de preguntar por los errores de otra.
func (h *Handler) Incidentes(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	f := FiltrosIncidentes{
		Referencia: c.Query("ref"),
		Desde:      c.Query("desde"),
		Hasta:      c.Query("hasta"),
		SoloPanico: c.Query("panico") == "1",
	}
	if n, err := strconv.Atoi(c.Query("limite")); err == nil {
		f.Limite = n
	}

	items, err := h.svc.Listar(c.Request.Context(), cl.EmpresaID, f)
	if err != nil {
		h.responder(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// ResumenIncidentes GET /v1/sistema/incidentes/resumen (admin.incidentes)
//
// Va aparte de la lista porque contesta otra pregunta —«¿está pasando ahora?»— y la pantalla la
// muestra arriba, antes de que nadie abra nada.
func (h *Handler) ResumenIncidentes(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.Resumen(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) responder(c *gin.Context, err error) {
	switch {
	// Una referencia mal escrita es 400 y no una lista vacía: vacío se lee como «ese código no
	// existe», y el código puede existir perfectamente con otra forma de tecleo.
	case errors.Is(err, ErrReferenciaInvalida), errors.Is(err, ErrFechaInvalida):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, sinPrefijo(err))
	// El contador de consultas apagado es una REGLA del servidor, no un fallo: 422 con la
	// explicación. `handler_rendimiento.go` ya lo traduce en su propio camino, pero va también acá
	// porque este switch es la red que atrapa cualquier error del paquete que llegue por otra vía
	// —y el centinela de este paquete lo exige por esa misma razón: lo que no esté acá sale como
	// 500 y queda registrado como un error del propio sistema.
	case errors.Is(err, ErrRendimientoNoDisponible):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, sinPrefijo(err))
	default:
		httpx.AbortInterno(c, h.log, "sistema: incidentes", err)
	}
}

func sinPrefijo(err error) string {
	return strings.TrimPrefix(err.Error(), "sistema: ")
}
