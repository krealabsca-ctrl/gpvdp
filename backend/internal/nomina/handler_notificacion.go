package nomina

// Endpoints de las notificaciones de RRHH.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// EnviarBoletas POST /v1/rrhh/corridas/:id/boletas (rrhh.corrida)
// Manda la boleta de pago a cada colaborador de la corrida que tenga correo en su ficha.
// Devuelve a quiénes se les envió y a quiénes no (y por qué).
func (h *Handler) EnviarBoletas(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	res, err := h.svc.EnviarBoletas(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderNotificacion(c, err, "enviar-boletas")
		return
	}
	c.JSON(http.StatusOK, res)
}

// EnviarAvisoVacaciones POST /v1/rrhh/vacaciones/:id/aviso (rrhh.ausencias)
func (h *Handler) EnviarAvisoVacaciones(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.EnviarAvisoVacaciones(c.Request.Context(), empresaID, c.Param("id"), usuarioID); err != nil {
		h.responderNotificacion(c, err, "enviar-aviso-vacaciones")
		return
	}
	c.JSON(http.StatusOK, gin.H{"enviado": true})
}

// responderNotificacion mapea los errores propios del envío; el resto va al mapeo general.
func (h *Handler) responderNotificacion(c *gin.Context, err error, op string) {
	switch {
	case errors.Is(err, ErrCorreoNoConfigurado):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"el envío de correos no está configurado en el servidor")
	case errors.Is(err, ErrEmpleadoSinCorreo):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	default:
		h.responderError(c, err, op)
	}
}
