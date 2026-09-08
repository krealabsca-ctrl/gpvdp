package grupo

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone la vista de grupo por HTTP.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Resumen GET /v1/grupo/resumen?periodo=AAAA-MM (grupo.ver)
//
// El usuario y su rol salen del TOKEN. No hay ningún parámetro que permita pedir «las empresas X e
// Y»: el alcance lo resuelve el servidor desde las membresías, y por eso el pedido no puede ampliarlo.
func (h *Handler) Resumen(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	periodo := c.Query("periodo")

	res, err := h.svc.Resumen(c.Request.Context(), cl.UsuarioID(), cl.Rol, periodo)
	if err != nil {
		h.responder(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) responder(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrPeriodoInvalido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, sinPrefijo(err))
	// Sin empresas visibles NO es un error del servidor: es que no hay acceso. 403 y con el mensaje
	// que lo explica, para que nadie salga a buscar un bug que no existe.
	case errors.Is(err, ErrSinEmpresasVisibles):
		httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, sinPrefijo(err))
	default:
		h.log.Error("grupo: resumen", zap.Error(err))
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}

func sinPrefijo(err error) string {
	s := err.Error()
	if len(s) > 7 && s[:7] == "grupo: " {
		return s[7:]
	}
	return s
}
