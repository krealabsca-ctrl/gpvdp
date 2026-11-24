package sistema

// Endpoints de las consultas lentas. El porqué está en rendimiento.go.

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/httpx"
)

// HandlerRendimiento expone el contador de consultas por HTTP.
type HandlerRendimiento struct {
	svc *ServiceRendimiento
	log *zap.Logger
}

// NewHandlerRendimiento construye el handler.
func NewHandlerRendimiento(svc *ServiceRendimiento, log *zap.Logger) *HandlerRendimiento {
	return &HandlerRendimiento{svc: svc, log: log}
}

// ConsultasLentas GET /v1/sistema/rendimiento/consultas?limite=20 (admin.rendimiento)
//
// NO filtra por empresa, y no es un olvido: los tiempos son del motor entero y no se pueden
// repartir por empresa. Lo que devuelve es la FORMA de cada consulta —PostgreSQL ya reemplazó los
// valores por $1, $2— así que no hay datos de ninguna empresa que proteger acá.
func (h *HandlerRendimiento) ConsultasLentas(c *gin.Context) {
	limite, _ := strconv.Atoi(c.Query("limite"))
	res, err := h.svc.ConsultasLentas(c.Request.Context(), limite)
	if err != nil {
		httpx.AbortInterno(c, h.log, "consultas-lentas", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ReiniciarContadores POST /v1/sistema/rendimiento/reiniciar (admin.rendimiento)
//
// Pone los acumulados en cero para medir una ventana concreta: se reinicia, se reproduce lo que va
// lento y se mira. No toca ningún dato del ERP.
func (h *HandlerRendimiento) ReiniciarContadores(c *gin.Context) {
	if err := h.svc.ReiniciarContadores(c.Request.Context()); err != nil {
		// Si la extensión no está, es una regla del servidor y no un fallo: 422 con la explicación,
		// no un 500 que mande a buscar un error que no existe.
		if errors.Is(err, ErrRendimientoNoDisponible) {
			httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, motivoNoDisponible)
			return
		}
		httpx.AbortInterno(c, h.log, "reiniciar-contadores", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
