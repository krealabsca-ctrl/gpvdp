package bancos

// Fase C — Proyecciones: calcular escenario (GET), guardar escenario (POST, spec
// openapi /bancos/proyecciones) y listar escenarios guardados con su precisión.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

func parseMetodo(m string) (string, bool) {
	switch m {
	case "", MetodoRitmo:
		return MetodoRitmo, true
	case MetodoHistorico, MetodoPromedio, MetodoCoincidencia:
		return m, true
	}
	return "", false
}

// Proyeccion GET /v1/bancos/proyecciones?periodo=YYYY-MM&metodo=&meta_pct=
func (h *Handler) Proyeccion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	metodo, ok := parseMetodo(c.Query("metodo"))
	if !ok {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "metodo inválido (RITMO|HISTORICO|PROMEDIO|COINCIDENCIA)")
		return
	}
	metaPct := decimal.Zero
	if v := c.Query("meta_pct"); v != "" {
		p, err := decimal.NewFromString(v)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "meta_pct inválido")
			return
		}
		metaPct = p
	}
	res, err := h.svc.CalcularProyeccion(c.Request.Context(), claims.EmpresaID, c.Query("periodo"), metodo, metaPct)
	if err != nil {
		h.responderError(c, err, "proyeccion")
		return
	}
	c.JSON(http.StatusOK, res)
}

type guardarEscenarioRequest struct {
	Periodo            string   `json:"periodo" validate:"required"`
	Metodo             string   `json:"metodo"`
	MetaCrecimientoPct string   `json:"meta_crecimiento_pct"`
	LineasIngreso      []string `json:"lineas_ingreso"`
}

// GuardarEscenario POST /v1/bancos/proyecciones — calcula y persiste el escenario.
func (h *Handler) GuardarEscenario(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req guardarEscenarioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	metodo, ok := parseMetodo(req.Metodo)
	if !ok {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "metodo inválido (RITMO|HISTORICO|PROMEDIO|COINCIDENCIA)")
		return
	}
	metaPct := decimal.Zero
	if req.MetaCrecimientoPct != "" {
		p, err := decimal.NewFromString(req.MetaCrecimientoPct)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "meta_crecimiento_pct inválido")
			return
		}
		metaPct = p
	}
	res, id, err := h.svc.GuardarEscenario(c.Request.Context(), claims.EmpresaID,
		req.Periodo, metodo, metaPct, req.LineasIngreso, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "guardar-escenario")
		return
	}
	if res.SinDatos {
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"no hay ingresos en el período; no hay nada que proyectar")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"escenario_id": id, "resultado": res})
}

// Escenarios GET /v1/bancos/proyecciones/escenarios?periodo=
func (h *Handler) Escenarios(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	list, err := h.svc.EscenariosGuardados(c.Request.Context(), claims.EmpresaID, c.Query("periodo"))
	if err != nil {
		h.responderError(c, err, "escenarios")
		return
	}
	if list == nil {
		list = []EscenarioGuardado{}
	}
	c.JSON(http.StatusOK, list)
}
