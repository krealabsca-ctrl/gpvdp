package cxp

// Endpoint del calendario de pagos. El porqué está en calendario.go.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// CalendarioDePagos GET /v1/cxp/calendario (cxp.ver)
//
// La fecha se calcula EN HORA DE COSTA RICA y se pasa al servicio. El contenedor corre en UTC: a
// las 18:00 de un viernes allá ya es sábado, y «esta semana» se correría un día entero justo el
// día en que se paga. Es el mismo `offsetCR` que usa el resto del paquete.
func (h *Handler) CalendarioDePagos(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	hoy := time.Now().UTC().Add(offsetCR)
	cal, err := h.svc.CalendarioDePagos(c.Request.Context(), empresaID,
		time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		h.responderError(c, err, "calendario-pagos")
		return
	}
	c.JSON(http.StatusOK, cal)
}
