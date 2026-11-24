package bancos

// Endpoint del análisis de partidas en el tiempo.

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// rangoDeAnalisis lee y valida `desde`/`hasta` de la consulta.
//
// Está extraído porque ahora lo usan el análisis y su exportación, y un tope distinto entre los
// dos haría que el archivo traiga un rango que la pantalla no puede mostrar —o al revés—.
//
// Devuelve false cuando ya respondió el error.
func rangoDeAnalisis(c *gin.Context) (string, string, bool) {
	hasta := c.Query("hasta")
	if hasta == "" {
		hasta = AhoraCR().Format("2006-01")
	}
	// Validar la FORMA en el borde: un período mal escrito llegaría a `to_date()` y saldría como un
	// 500 sin decir qué está mal.
	tHasta, err := time.Parse("2006-01", hasta)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hasta debe ser YYYY-MM")
		return "", "", false
	}
	desde := c.Query("desde")
	if desde == "" {
		desde = tHasta.AddDate(0, -11, 0).Format("2006-01")
	}
	tDesde, err := time.Parse("2006-01", desde)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "desde debe ser YYYY-MM")
		return "", "", false
	}
	if tDesde.After(tHasta) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "desde no puede ser posterior a hasta")
		return "", "", false
	}
	// Tope de rango: cada mes multiplica por el número de partidas (168 en la empresa más grande).
	// Sin tope, un rango de años arma una tabla que ninguna pantalla puede mostrar.
	if MesesDelRango(tDesde, tHasta) > maxMesesAnalisis {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el rango no puede pasar de 24 meses")
		return "", "", false
	}
	return desde, hasta, true
}

// AnalisisPartidas GET /v1/bancos/analisis/partidas?desde=YYYY-MM&hasta=YYYY-MM&comparar=1
//
// Sin parámetros toma los últimos 12 meses hasta el mes en curso (día de Costa Rica).
func (h *Handler) AnalisisPartidas(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	desde, hasta, ok := rangoDeAnalisis(c)
	if !ok {
		return
	}
	// `comparar=1` trae además el mismo rango del año anterior. Va como opción y no siempre
	// porque duplica el trabajo de la consulta, y en una empresa con tres meses cargados el año
	// anterior está vacío: la columna sería una fila de ceros con aire de dato.
	if c.Query("comparar") == "1" {
		res, err := h.svc.AnalisisPartidasComparado(c.Request.Context(), claims.EmpresaID, desde, hasta)
		if err != nil {
			h.responderError(c, err, "analisis-partidas-comparado")
			return
		}
		c.JSON(http.StatusOK, res)
		return
	}
	res, err := h.svc.AnalisisPartidas(c.Request.Context(), claims.EmpresaID, desde, hasta)
	if err != nil {
		h.responderError(c, err, "analisis-partidas")
		return
	}
	c.JSON(http.StatusOK, res)
}

// ExportarAnalisis GET /v1/bancos/analisis/partidas/export?desde=&hasta=&grupo=&comparar=1
//
// Devuelve el .xlsx del análisis. Mismo rango y mismo filtro que la pantalla: un archivo que trae
// otra cosa que lo que se está mirando es peor que no tener archivo.
func (h *Handler) ExportarAnalisis(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	desde, hasta, ok := rangoDeAnalisis(c)
	if !ok {
		return
	}
	buf, nombre, err := h.svc.ExportarAnalisis(c.Request.Context(), claims.EmpresaID,
		desde, hasta, c.Query("grupo"), c.Query("comparar") == "1", claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "exportar-analisis")
		return
	}
	c.Header("Access-Control-Expose-Headers", "Content-Disposition")
	c.Header("Content-Disposition", `attachment; filename="`+nombre+`"`)
	c.Data(http.StatusOK, xlsxContentType, buf)
}

// FlujoProyectado GET /v1/bancos/analisis/flujo?desde=&hasta=&meses=6
//
// `desde`/`hasta` acotan el HISTÓRICO del que sale el patrón; `meses` cuántos se proyectan hacia
// adelante. Por defecto, los últimos 12 meses de base y 6 de proyección.
func (h *Handler) FlujoProyectado(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	desde, hasta, ok := rangoDeAnalisis(c)
	if !ok {
		return
	}
	meses, _ := strconv.Atoi(c.Query("meses"))
	res, err := h.svc.ProyectarFlujo(c.Request.Context(), claims.EmpresaID, desde, hasta, meses)
	if err != nil {
		h.responderError(c, err, "flujo-proyectado")
		return
	}
	c.JSON(http.StatusOK, res)
}

// SerieDiariaPartidas GET /v1/bancos/analisis/partidas/diario?desde=YYYY-MM&hasta=YYYY-MM&clasificaciones=id1,id2
//
// El día a día de unas partidas concretas: contesta CUÁNDO se movió la plata, no si está mal.
// El juicio de anomalía sigue siendo mensual (ver analisis_diario.go).
//
// Sin `clasificaciones` devuelve vacío, no «todas»: es una pregunta sobre partidas elegidas.
func (h *Handler) SerieDiariaPartidas(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	hasta := c.Query("hasta")
	if hasta == "" {
		hasta = AhoraCR().Format("2006-01")
	}
	tHasta, err := time.Parse("2006-01", hasta)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hasta debe ser YYYY-MM")
		return
	}
	desde := c.Query("desde")
	if desde == "" {
		desde = tHasta.Format("2006-01")
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
	if MesesDelRango(tDesde, tHasta) > maxMesesAnalisis {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el rango no puede pasar de 24 meses")
		return
	}

	var ids []string
	if crudo := strings.TrimSpace(c.Query("clasificaciones")); crudo != "" {
		ids = strings.Split(crudo, ",")
	}
	res, err := h.svc.SerieDiariaDePartidas(c.Request.Context(), claims.EmpresaID, desde, hasta, ids)
	if err != nil {
		h.responderError(c, err, "analisis-partidas-diario")
		return
	}
	c.JSON(http.StatusOK, res)
}
