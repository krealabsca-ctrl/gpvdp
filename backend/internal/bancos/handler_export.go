package bancos

// Handlers de exportación .xlsx (Fase D). Devuelven el binario OOXML como descarga.

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ExportarMovimientos GET /v1/bancos/exportaciones/movimientos?periodo=&solo_creditos=
func (h *Handler) ExportarMovimientos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	// Los filtros del reporte son los MISMOS de la hoja de trabajo (banco, cuenta, concepto,
	// clasificación, tipo, fechas, búsqueda) más las listas en plural: «puede ser periodo o
	// periodos; un concepto o varios, o todos» (decisión del negocio).
	f := filtrosDeQuery(c)
	if !abortarSiFiltrosInvalidos(c, f) {
		return
	}
	// Compatibilidad con el llamado viejo `?solo_creditos=true`.
	if c.Query("solo_creditos") == "true" {
		f.Tipo = "CREDITO"
	}
	if f.Periodo == "" && len(f.Periodos) == 0 && f.Desde == "" && f.Hasta == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"indicá al menos un período (periodo=YYYY-MM o periodos=YYYY-MM,YYYY-MM) o un rango de fechas")
		return
	}
	op := OpcionesReporte{AgruparPorPartida: agruparDeQuery(c)}
	buf, _, err := h.svc.ExportarMovimientosXLSX(c.Request.Context(), claims.EmpresaID, f, claims.UsuarioID(), op)
	if err != nil {
		h.responderError(c, err, "exportar-movimientos")
		return
	}
	// Nomenclatura del usuario: «VDP 17082026.xlsx» (sigla de la empresa + fecha). El detalle
	// distingue las dos presentaciones: sin él, las dos descargas del mismo día se pisarían en la
	// carpeta y no se sabría cuál es cuál.
	detalle := ""
	if !op.AgruparPorPartida {
		detalle = "corrido"
	}
	nombre := h.svc.NombreArchivo(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(), detalle)
	c.Header("Content-Disposition", "attachment; filename=\""+nombre+"\"")
	c.Data(http.StatusOK, xlsxContentType, buf)
}

// ExportarCuadre GET /v1/bancos/exportaciones/cuadre?periodo=
func (h *Handler) ExportarCuadre(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	periodo := c.Query("periodo")
	if periodo == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "periodo (YYYY-MM) es requerido")
		return
	}
	buf, _, err := h.svc.ExportarCuadreXLSX(c.Request.Context(), claims.EmpresaID, periodo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "exportar-cuadre")
		return
	}
	nombre := h.svc.NombreArchivo(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(), "cuadre")
	c.Header("Content-Disposition", "attachment; filename=\""+nombre+"\"")
	c.Data(http.StatusOK, xlsxContentType, buf)
}

// listaDeQuery lee un parámetro que puede venir repetido (?conceptos=a&conceptos=b) o separado
// por comas (?conceptos=a,b). Las dos formas se usan en la práctica y ninguna debe fallar en
// silencio; los vacíos se descartan.
func listaDeQuery(c *gin.Context, nombre string) []string {
	out := []string{}
	for _, v := range c.QueryArray(nombre) {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// agruparDeQuery lee la presentación del detalle. Por defecto AGRUPADO: es la que ya estaba
// aprobada, así que un llamado viejo sin el parámetro sigue devolviendo el mismo archivo.
//
//	?agrupar=partida        → bandas por partida con subtotales (defecto)
//	?agrupar=ninguno|no|false|0 → listado corrido
func agruparDeQuery(c *gin.Context) bool {
	switch strings.ToLower(strings.TrimSpace(c.Query("agrupar"))) {
	case "ninguno", "no", "false", "0", "corrido":
		return false
	default:
		return true
	}
}

// sufijoArchivo nombra el archivo según lo pedido: un mes, un rango de meses, o «seleccion»
// cuando son meses sueltos. El nombre tiene que decir qué trae sin abrirlo.
func sufijoArchivo(f FiltrosMovimientos) string {
	ps := append([]string{}, f.Periodos...)
	if f.Periodo != "" {
		ps = append(ps, f.Periodo)
	}
	sort.Strings(ps)
	switch {
	case len(ps) == 1:
		return ps[0]
	case len(ps) > 1 && periodosContiguos(ps):
		return ps[0] + "_a_" + ps[len(ps)-1]
	case len(ps) > 1:
		return "seleccion-" + strconv.Itoa(len(ps)) + "-periodos"
	case f.Desde != "" || f.Hasta != "":
		return strings.ReplaceAll(f.Desde+"_a_"+f.Hasta, "-", "")
	default:
		return "historico"
	}
}
