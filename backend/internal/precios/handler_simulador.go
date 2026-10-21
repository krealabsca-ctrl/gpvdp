package precios

// Endpoint del simulador.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

type simulacionReq struct {
	Fecha     string    `json:"fecha"`
	Desde     string    `json:"desde"`
	Hasta     string    `json:"hasta"`
	Vendedor  string    `json:"vendedor"`
	Escenario Escenario `json:"escenario"`
}

// Simular corre un escenario sobre un plan. No escribe nada: es un «qué pasaría si».
func (h *Handler) Simular(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in simulacionReq
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	r, err := h.svc.Simular(c.Request.Context(), cl.EmpresaID, c.Param("id"), OpcionesSimulacion{
		Fecha: in.Fecha, Desde: in.Desde, Hasta: in.Hasta,
		Vendedor: in.Vendedor, Escenario: in.Escenario,
	})
	if err != nil {
		h.responder(c, err, "precios.simular")
		return
	}
	c.JSON(http.StatusOK, r)
}
