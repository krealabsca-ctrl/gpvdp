package bancos

import (
	"net/http"
	"strconv"
	"time"

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

// validarFiltros revisa que los identificadores, las FECHAS y los períodos tengan forma ANTES de
// llegar al SQL, y devuelve el nombre del parámetro que está mal.
//
// Sin esto un `?clasificaciones=no-es-uuid` llega al `::uuid[]` de Postgres, revienta el cast y el
// cliente recibe un 500 «error interno» por escribir mal un parámetro. Es un 400 y hay que decir
// cuál parámetro está mal. Se valida en el handler porque es validación de borde.
//
// Las fechas entraron a esta función el 21-set-2026 por el mismo defecto y una vuelta de tuerca:
// `?desde=0000-09-01` salía 500, y `?desde=01/09/2026` salía 200 con el filtro midiendo otra cosa.
// Un identificador mal escrito se nota; una fecha mal entendida devuelve una lista que se lee como
// un hecho sobre el dinero.
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
	// Las fechas del rango: sin esta rama el texto crudo del query string llegaba al `m.fecha >= $N`
	// y una fecha imposible («0000-09-01», «2026-02-31») volvía como 500 «error interno» por un dato
	// mal escrito. Peor todavía: lo que Postgres SÍ entiende con otro formato («01/09/2026», que con
	// DateStyle=MDY es el 9 de enero) se aceptaba en silencio y el filtro filtraba otra cosa.
	fechas := map[string]string{"desde": f.Desde, "hasta": f.Hasta}
	for nombre, v := range fechas {
		if v != "" && !fechaISOValida(v) {
			return nombre, false
		}
	}
	// El período singular con la MISMA regla que el plural: los dos terminan en el mismo
	// `to_char(m.fecha,'YYYY-MM') = $N`, que con un valor mal escrito no falla — devuelve cero filas,
	// y «no hay movimientos» es una respuesta distinta y más peligrosa que «escribiste mal el mes».
	if f.Periodo != "" && !periodoValido(f.Periodo) {
		return "periodo", false
	}
	for _, p := range f.Periodos {
		if !periodoValido(p) {
			return "periodos", false
		}
	}
	return "", true
}

// periodoValido acepta la forma YYYY-MM. Es la regla que ya aplicaba la lista de períodos.
//
// Tiene que ser un mes que EXISTA. Antes solo se miraba el largo y el guion, así que «2026-13»
// pasaba, llegaba a la consulta y devolvía cero filas sin avisar — una lista vacía que se lee como
// «ese mes no entró nada», que es una respuesta distinta y peligrosa. El piso de año es el mismo de
// fechaISOValida.
func periodoValido(p string) bool {
	if len(p) != 7 || p[4] != '-' {
		return false
	}
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return false
	}
	return t.Year() >= anioMinimo
}

// anioMinimo es el piso de año que acepta cualquier filtro de fecha de Bancos.
//
// Coincide con la regla de la pantalla (rangoFechas.ts: año de cuatro cifras que no empieza en
// cero), y tiene que coincidir: si el servidor aceptara menos que la pantalla, el endpoint quedaría
// expuesto por URL y por Clasificar, que comparte este validador y no tiene esa guarda. Con el piso
// en 1, «0202-09-01» —lo que emite el selector de fecha mientras se teclea «2026»— pasaba y
// devolvía el HISTÓRICO COMPLETO en silencio, como si fuera un filtro.
const anioMinimo = 1000

// fechaISOValida acepta ÚNICAMENTE un YYYY-MM-DD que exista en el calendario y que Postgres pueda
// representar como `date`.
//
// Las tres comprobaciones son tres defectos distintos, y ninguna sobra:
//   - el largo fijo descarta lo que no es ISO («01/09/2026», «2026-09», «2026»), que es justo lo que
//     Postgres interpreta a su manera y deja el filtro midiendo otra cosa sin avisar;
//   - time.Parse descarta el día y el mes que no existen («2026-02-31», «2026-13-01»);
//   - el piso de año (anioMinimo) descarta «0000-09-01», que Go parsea sin chistar y Postgres
//     rechaza con 500, y también «0202-09-01» o «0020-09-01», que Postgres SÍ acepta y convierten
//     el filtro en «todo el histórico». Son los valores que emite el selector de fecha del
//     navegador mientras se teclea el año.
func fechaISOValida(v string) bool {
	if len(v) != 10 {
		return false
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return false
	}
	return t.Year() >= anioMinimo
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

// motivoFiltrosInvalidos devuelve el mensaje a mostrar, o "" si los filtros están bien.
//
// Está aparte del `Abort` para poder probarlo sin levantar un servidor, y devuelve el TEXTO y no el
// nombre del parámetro porque el rango invertido no es un problema de formato: los dos parámetros
// están bien escritos y aun así la consulta no puede devolver nada.
func motivoFiltrosInvalidos(f FiltrosMovimientos) string {
	if param, ok := validarFiltros(f); !ok {
		return "el parámetro «" + param + "» no tiene un formato válido"
	}
	// Rango al revés. Antes respondía 200 con la lista vacía, y la pantalla lo explicaba como
	// «ningún movimiento coincide con lo que filtraste»: el usuario leía «no entró la plata» cuando
	// lo que pasaba era que las dos fechas estaban invertidas. Una lista vacía es una AFIRMACIÓN
	// sobre el dinero; esto es un error de quien filtra y hay que decirlo.
	if f.Desde != "" && f.Hasta != "" && f.Desde > f.Hasta {
		return "el rango de fechas está al revés: «hasta» (" + f.Hasta + ") es anterior a «desde» (" + f.Desde + ")"
	}
	return ""
}

// abortarSiFiltrosInvalidos responde 400 y devuelve false cuando algún filtro viene mal.
func abortarSiFiltrosInvalidos(c *gin.Context, f FiltrosMovimientos) bool {
	if motivo := motivoFiltrosInvalidos(f); motivo != "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, motivo)
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
	// Un concepto creado desde BANCOS nace INVISIBLE para CxP (decisión del usuario, 2026-09-03):
	// «lo que crea en conta lo puedo ver en Bancos, pero lo que creo en Bancos no lo puede ver conta
	// a menos que lo marque como visible». Bancos abre rubros bancarios —traslados, ahorro,
	// overnight— que no son gasto a pagar, y llenarle el selector de Contabilidad con eso es
	// exactamente lo que hace que clasificar sea difícil.
	//
	// Se puede pedir visible explícitamente en el cuerpo: quien crea desde acá tiene
	// `bancos.catalogo` y puede decidirlo. Lo que cambió es el DEFAULT.
	visible := false
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
