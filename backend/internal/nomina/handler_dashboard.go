package nomina

// Endpoint del dashboard de RRHH (lectura: rrhh.ver).

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// DashboardRRHH GET /v1/rrhh/dashboard?anio=&mes= (rrhh.ver) — resumen del mes en curso.
func (h *Handler) DashboardRRHH(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	ahora := time.Now()
	anio, err := strconv.Atoi(c.DefaultQuery("anio", strconv.Itoa(ahora.Year())))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "año inválido")
		return
	}
	mes, err := strconv.Atoi(c.DefaultQuery("mes", strconv.Itoa(int(ahora.Month()))))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "mes inválido")
		return
	}
	d, err := h.svc.Dashboard(c.Request.Context(), empresaID, anio, mes)
	if err != nil {
		h.responderError(c, err, "dashboard-rrhh")
		return
	}
	c.JSON(http.StatusOK, d)
}
