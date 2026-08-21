package bancos

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// filtrosDeQuery lee los filtros de la hoja de trabajo del query string.
//
// Lo comparten la lista y el resumen de la selección: si cada endpoint leyera sus
// parámetros por su cuenta, agregar un filtro a la pantalla dejaría el resumen midiendo
// otra cosa (y nadie se daría cuenta hasta que los números no cuadren).
func filtrosDeQuery(c *gin.Context) FiltrosMovimientos {
	return FiltrosMovimientos{
		Desde:           c.Query("desde"),
		Hasta:           c.Query("hasta"),
		Periodo:         c.Query("periodo"),
		ConceptoID:      c.Query("concepto_id"),
		ClasificacionID: c.Query("clasificacion_id"),
		// Las listas van acá, en el MISMO parser que usan la lista y el resumen: así la vista
		// previa del armador de reportes cuenta exactamente lo que va a exportar.
		Periodos:         listaDeQuery(c, "periodos"),
		ConceptoIDs:      listaDeQuery(c, "conceptos"),
		ClasificacionIDs: listaDeQuery(c, "clasificaciones"),
		BancoID:          c.Query("banco_id"),
		CuentaID:         c.Query("cuenta_bancaria_id"),
		Estado:           c.Query("estado_clasificacion"),
		Tipo:             c.Query("tipo"),
		Traslado:         c.Query("traslado"),
		Q:                c.Query("q"),
		Orden:            c.Query("orden"),
	}
}

// validarFiltros revisa que los identificadores tengan forma de UUID ANTES de llegar al SQL.
//
// Sin esto un `?clasificaciones=no-es-uuid` llega al `::uuid[]` de Postgres, revienta el cast y el
// cliente recibe un 500 «error interno» por escribir mal un parámetro. Es un 400 y hay que decir
// cuál parámetro está mal. Se valida en el handler porque es validación de borde.
func validarFiltros(f FiltrosMovimientos) (string, bool) {
	uno := map[string]string{
		"concepto_id":        f.ConceptoID,
		"clasificacion_id":   f.ClasificacionID,
		"banco_id":           f.BancoID,
		"cuenta_bancaria_id": f.CuentaID,
	}
	for nombre, v := range uno {
		if v != "" && !pareceUUID(v) {
			return nombre, false
		}
	}
	varios := map[string][]string{
		"conceptos":       f.ConceptoIDs,
		"clasificaciones": f.ClasificacionIDs,
	}
	for nombre, vs := range varios {
		for _, v := range vs {
			if !pareceUUID(v) {
				return nombre, false
			}
		}
	}
	for _, p := range f.Periodos {
		if len(p) != 7 || p[4] != '-' {
			return "periodos", false
		}
	}
	return "", true
}

// pareceUUID valida la forma 8-4-4-4-12 en hexadecimal. Alcanza para decidir si mandarlo al SQL:
// lo que exista o no ya lo dice la consulta.
func pareceUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		ch := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return false
			}
			continue
		}
		hex := (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
		if !hex {
			return false
		}
	}
	return true
}

// abortarSiFiltrosInvalidos responde 400 y devuelve false cuando algún identificador viene mal.
func abortarSiFiltrosInvalidos(c *gin.Context, f FiltrosMovimientos) bool {
	if param, ok := validarFiltros(f); !ok {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el parámetro «"+param+"» no tiene un formato válido")
		return false
	}
	return true
}

// ResumenSeleccion GET /v1/bancos/movimientos/resumen — cuántos y cuánto hay en la
// selección activa (mismos filtros que la lista), con desglose de dos niveles.
func (h *Handler) ResumenSeleccion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	f := filtrosDeQuery(c)
	if !abortarSiFiltrosInvalidos(c, f) {
		return
	}
	res, err := h.svc.ResumenFiltro(c.Request.Context(), claims.EmpresaID, f, c.Query("agrupar"))
	if err != nil {
		h.responderError(c, err, "resumen-seleccion")
		return
	}
	if res.Conceptos == nil {
		res.Conceptos = []CuadreConceptoNodo{}
	}
	c.JSON(http.StatusOK, res)
}

// Movimientos GET /v1/bancos/movimientos — hoja de trabajo con filtros y totales.
func (h *Handler) Movimientos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	f := filtrosDeQuery(c)
	if !abortarSiFiltrosInvalidos(c, f) {
		return
	}
	f.Page = atoiDefault(c.Query("page"), 1)
	f.PageSize = atoiDefault(c.Query("page_size"), 100)
	lista, err := h.svc.ListarMovimientos(c.Request.Context(), claims.EmpresaID, f)
	if err != nil {
		h.responderError(c, err, "movimientos")
		return
	}
	if lista.Items == nil {
		lista.Items = []MovimientoRow{}
	}
	c.JSON(http.StatusOK, lista)
}

type reclasificarRequest struct {
	ConceptoID      string `json:"concepto_id" validate:"required,uuid"`
	ClasificacionID string `json:"clasificacion_id" validate:"required,uuid"`
}

// Reclasificar PATCH /v1/bancos/movimientos/:id/clasificacion
func (h *Handler) Reclasificar(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req reclasificarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	err := h.svc.ReclasificarManual(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.ConceptoID, req.ClasificacionID, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "reclasificar")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type crearReglaRequest struct {
	Nombre          string   `json:"nombre" validate:"required"`
	AplicaA         string   `json:"aplica_a" validate:"required,oneof=DEBITO CREDITO MIXTO"`
	ConceptoID      string   `json:"concepto_id" validate:"required,uuid"`
	ClasificacionID string   `json:"clasificacion_id" validate:"required,uuid"`
	Prioridad       int      `json:"prioridad"`
	Palabras        []string `json:"palabras_clave" validate:"required,min=1"`
}

// CrearRegla POST /v1/bancos/reglas — crea la regla y clasifica el bloque no identificado.
func (h *Handler) CrearRegla(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req crearReglaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	id, clasificados, err := h.svc.CrearRegla(c.Request.Context(), claims.EmpresaID, NuevaRegla{
		Nombre: req.Nombre, AplicaA: req.AplicaA, ConceptoID: req.ConceptoID,
		ClasificacionID: req.ClasificacionID, Prioridad: req.Prioridad, Palabras: req.Palabras,
	}, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-regla")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"regla_id": id, "clasificados": clasificados})
}

// Reglas GET /v1/bancos/reglas — lista las reglas activas con su prioridad (motor de segmentación).
func (h *Handler) Reglas(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	list, err := h.svc.Reglas(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "reglas")
		return
	}
	if list == nil {
		list = []Regla{}
	}
	c.JSON(http.StatusOK, list)
}

// Conceptos GET /v1/bancos/catalogo/conceptos?ambito=cxp
// ambito=cxp devuelve solo los conceptos visibles para contabilidad (CxP).
func (h *Handler) Conceptos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	list, err := h.svc.Conceptos(c.Request.Context(), claims.EmpresaID, c.Query("ambito") == "cxp")
	if err != nil {
		h.responderError(c, err, "conceptos")
		return
	}
	if list == nil {
		list = []Concepto{}
	}
	c.JSON(http.StatusOK, list)
}

type crearConceptoRequest struct {
	Nombre string `json:"nombre" validate:"required"`
	// nil = true (compatibilidad): lo creado desde CxP debe verse en CxP.
	VisibleCxP *bool `json:"visible_cxp"`
}

// CrearConcepto POST /v1/bancos/catalogo/conceptos
func (h *Handler) CrearConcepto(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req crearConceptoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	visible := true
	if req.VisibleCxP != nil {
		visible = *req.VisibleCxP
	}
	res, err := h.svc.CrearConcepto(c.Request.Context(), claims.EmpresaID, req.Nombre, visible, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-concepto")
		return
	}
	c.JSON(http.StatusCreated, res)
}

type crearClasificacionRequest struct {
	ConceptoID           string `json:"concepto_id" validate:"required,uuid"`
	Nombre               string `json:"nombre" validate:"required"`
	CuentaContableFutura string `json:"cuenta_contable_futura"`
}

// CrearClasificacion POST /v1/bancos/catalogo/clasificaciones
func (h *Handler) CrearClasificacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req crearClasificacionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.CrearClasificacion(c.Request.Context(), claims.EmpresaID, req.ConceptoID, req.Nombre, req.CuentaContableFutura, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-clasificacion")
		return
	}
	c.JSON(http.StatusCreated, res)
}

// Clasificaciones GET /v1/bancos/catalogo/clasificaciones?ambito=cxp
func (h *Handler) Clasificaciones(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	list, err := h.svc.Clasificaciones(c.Request.Context(), claims.EmpresaID, c.Query("ambito") == "cxp")
	if err != nil {
		h.responderError(c, err, "clasificaciones")
		return
	}
	if list == nil {
		list = []ClasificacionItem{}
	}
	c.JSON(http.StatusOK, list)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
