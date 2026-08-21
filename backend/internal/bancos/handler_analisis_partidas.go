package bancos

// Endpoint del análisis de partidas en el tiempo.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// AnalisisPartidas GET /v1/bancos/analisis/partidas?desde=YYYY-MM&hasta=YYYY-MM
//
// Sin parámetros toma los últimos 12 meses hasta el mes en curso (día de Costa Rica).
func (h *Handler) AnalisisPartidas(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	hasta := c.Query("hasta")
	if hasta == "" {
		hasta = AhoraCR().Format("2006-01")
	}
	// Validar la FORMA en el borde: un período mal escrito llegaría a `to_date()` y saldría como un
	// 500 sin decir qué está mal.
	tHasta, err := time.Parse("2006-01", hasta)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hasta debe ser YYYY-MM")
		return
	}
	desde := c.Query("desde")
	if desde == "" {
		desde = tHasta.AddDate(0, -11, 0).Format("2006-01")
	}
	tDesde, err := time.Parse("2006-01", desde)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "desde debe ser YYYY-MM")
		return
	}
	if tDesde.After(tHasta) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "desde no puede ser posterior a hasta")
		return
	}
	// Tope de rango: cada mes multiplica por el número de partidas (168 en la empresa más grande).
	// Sin tope, un rango de años arma una tabla que ninguna pantalla puede mostrar.
	if meses := int(tHasta.Sub(tDesde).Hours()/24/28) + 1; meses > maxMesesAnalisis {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el rango no puede pasar de 24 meses")
		return
	}
	res, err := h.svc.AnalisisPartidas(c.Request.Context(), claims.EmpresaID, desde, hasta)
	if err != nil {
		h.responderError(c, err, "analisis-partidas")
		return
	}
	c.JSON(http.StatusOK, res)
}
