package bancos

// Endpoints de las dimensiones del gasto (departamento y sede) y del presupuesto.

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// rePeriodo valida la forma YYYY-MM en el borde. Sin esto, un período mal escrito llegaría al CHECK
// de la tabla y saldría como un 500 en vez de decir qué está mal.
var rePeriodo = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// claimsDe extrae los claims o corta con 401. Evita repetir seis veces el mismo bloque.
func claimsDe(c *gin.Context) (*auth.Claims, bool) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return nil, false
	}
	return claims, true
}

// rangoDeMeses lee desde/hasta con los mismos defaults y validaciones que el análisis de partidas.
func rangoDeMeses(c *gin.Context) (string, string, bool) {
	hasta := c.Query("hasta")
	if hasta == "" {
		hasta = AhoraCR().Format("2006-01")
	}
	if !rePeriodo.MatchString(hasta) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hasta debe ser AAAA-MM")
		return "", "", false
	}
	desde := c.Query("desde")
	if desde == "" {
		desde = hasta
	}
	if !rePeriodo.MatchString(desde) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "desde debe ser AAAA-MM")
		return "", "", false
	}
	if desde > hasta {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "desde no puede ser posterior a hasta")
		return "", "", false
	}
	return desde, hasta, true
}

// ── Catálogo de sedes ────────────────────────────────────────────────────────

// Sedes GET /v1/bancos/catalogo/sedes (bancos.ver)
func (h *Handler) Sedes(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	list, err := h.svc.Sedes(c.Request.Context(), claims.EmpresaID, c.Query("incluir_inactivas") == "true")
	if err != nil {
		h.responderError(c, err, "sedes")
		return
	}
	c.JSON(http.StatusOK, list)
}

// Departamentos GET /v1/bancos/catalogo/departamentos (bancos.ver)
//
// Es el MISMO catálogo que usa CxP (una sola tabla, migración 0026). Se administra desde los dos
// módulos: dos listas de departamentos que se contradicen es peor que no tener ninguna.
func (h *Handler) Departamentos(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	list, err := h.svc.DepartamentosDeLaEmpresa(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "departamentos")
		return
	}
	c.JSON(http.StatusOK, list)
}

// ── Catálogo de departamentos (la MISMA tabla que CxP) ───────────────────────

type departamentoRequest struct {
	Nombre string `json:"nombre" binding:"required"`
	Codigo string `json:"codigo"`
}

// CrearDepartamento POST /v1/bancos/catalogo/departamentos (bancos.catalogo)
func (h *Handler) CrearDepartamento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req departamentoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el nombre del departamento es obligatorio")
		return
	}
	d, err := h.svc.CrearDepartamento(c.Request.Context(), claims.EmpresaID, req.Nombre, req.Codigo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-departamento")
		return
	}
	c.JSON(http.StatusCreated, d)
}

// ActualizarDepartamento PATCH /v1/bancos/catalogo/departamentos/:id (bancos.catalogo)
func (h *Handler) ActualizarDepartamento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req departamentoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el nombre del departamento es obligatorio")
		return
	}
	if err := h.svc.ActualizarDepartamento(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		req.Nombre, req.Codigo, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "actualizar-departamento")
		return
	}
	c.Status(http.StatusNoContent)
}

// CambiarActivoDepartamento POST /v1/bancos/catalogo/departamentos/:id/activo (bancos.catalogo)
func (h *Handler) CambiarActivoDepartamento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req struct {
		Activo bool `json:"activo"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.CambiarActivoDepartamento(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		req.Activo, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "activo-departamento")
		return
	}
	c.Status(http.StatusNoContent)
}

// UsoDeDepartamento GET /v1/bancos/catalogo/departamentos/:id/uso (bancos.ver)
//
// Se consulta ANTES de ofrecer el botón de eliminar: así la pantalla puede decir «tiene 12
// movimientos, solo se puede desactivar» en vez de dejar apretar y devolver un error.
func (h *Handler) UsoDeDepartamento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	uso, err := h.svc.UsoDeDepartamento(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "uso-departamento")
		return
	}
	c.JSON(http.StatusOK, gin.H{"uso": uso, "total": uso.Total(), "detalle": uso.Detalle(),
		"se_puede_eliminar": uso.Total() == 0})
}

// EliminarDepartamento DELETE /v1/bancos/catalogo/departamentos/:id (bancos.catalogo)
//
// Solo borra si no cuelga NADA. Si cuelga algo devuelve 422 nombrando qué, y el camino es desactivar.
func (h *Handler) EliminarDepartamento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	if err := h.svc.EliminarDepartamento(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "eliminar-departamento")
		return
	}
	c.Status(http.StatusNoContent)
}

// UsoDeSede GET /v1/bancos/catalogo/sedes/:id/uso (bancos.ver)
func (h *Handler) UsoDeSede(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	uso, err := h.svc.UsoDeSede(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "uso-sede")
		return
	}
	c.JSON(http.StatusOK, gin.H{"uso": uso, "total": uso.Total(), "detalle": uso.Detalle(),
		"se_puede_eliminar": uso.Total() == 0})
}

// EliminarSede DELETE /v1/bancos/catalogo/sedes/:id (bancos.catalogo)
func (h *Handler) EliminarSede(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	if err := h.svc.EliminarSede(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "eliminar-sede")
		return
	}
	c.Status(http.StatusNoContent)
}

type sedeRequest struct {
	Nombre string `json:"nombre" binding:"required"`
	Codigo string `json:"codigo"`
}

// CrearSede POST /v1/bancos/catalogo/sedes (bancos.catalogo)
func (h *Handler) CrearSede(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req sedeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el nombre de la sede es obligatorio")
		return
	}
	sd, err := h.svc.CrearSede(c.Request.Context(), claims.EmpresaID, req.Nombre, req.Codigo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-sede")
		return
	}
	c.JSON(http.StatusCreated, sd)
}

// ActualizarSede PATCH /v1/bancos/catalogo/sedes/:id (bancos.catalogo)
func (h *Handler) ActualizarSede(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req sedeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el nombre de la sede es obligatorio")
		return
	}
	if err := h.svc.ActualizarSede(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Nombre, req.Codigo, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "actualizar-sede")
		return
	}
	c.Status(http.StatusNoContent)
}

// CambiarActivoSede POST /v1/bancos/catalogo/sedes/:id/activo (bancos.catalogo)
func (h *Handler) CambiarActivoSede(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req struct {
		Activo bool `json:"activo"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.CambiarActivoSede(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Activo, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "activo-sede")
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Asignar las dimensiones ──────────────────────────────────────────────────

type dimensionesRequest struct {
	// Vacío = quitar la asignación. No se usa `omitempty`: mandar "" tiene que poder BORRAR.
	DepartamentoID string `json:"departamento_id"`
	SedeID         string `json:"sede_id"`
}

// AsignarDimensionesPartida PATCH /v1/bancos/clasificaciones/:id/dimensiones (bancos.catalogo)
//
// Es el camino que evita teclear: define el departamento y la sede POR DEFECTO de la partida, y con
// eso queda atribuida toda su historia y todo su futuro. Devuelve cuántos movimientos abarca.
func (h *Handler) AsignarDimensionesPartida(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req dimensionesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	n, err := h.svc.AsignarDimensionesClasificacion(c.Request.Context(), claims.EmpresaID,
		c.Param("id"), req.DepartamentoID, req.SedeID, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "dimensiones-partida")
		return
	}
	c.JSON(http.StatusOK, gin.H{"movimientos_afectados": n})
}

// AsignarDimensionesMovimiento PATCH /v1/bancos/movimientos/:id/dimensiones (bancos.clasificar)
//
// La excepción: este movimiento en particular no es del departamento de su partida.
func (h *Handler) AsignarDimensionesMovimiento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req dimensionesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := h.svc.AsignarDimensionesMovimiento(c.Request.Context(), claims.EmpresaID,
		c.Param("id"), req.DepartamentoID, req.SedeID, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "dimensiones-movimiento")
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Control presupuestario ───────────────────────────────────────────────────

// ControlPresupuestario GET /v1/bancos/control?desde=&hasta=&agrupar_por=&umbral= (bancos.ver)
func (h *Handler) ControlPresupuestario(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	agruparPor := c.DefaultQuery("agrupar_por", AgruparPorDepartamento)
	res, err := h.svc.ControlPresupuestario(c.Request.Context(), claims.EmpresaID, desde, hasta, agruparPor, c.Query("umbral"))
	if err != nil {
		h.responderError(c, err, "control-presupuestario")
		return
	}
	c.JSON(http.StatusOK, res)
}

// PartidasDeDimension GET /v1/bancos/control/partidas?agrupar_por=&id=&desde=&hasta= (bancos.ver)
//
// `id` vacío pide el gasto SIN dimensión asignada, que es donde empieza el trabajo de atribuirlo.
func (h *Handler) PartidasDeDimension(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	agruparPor := c.DefaultQuery("agrupar_por", AgruparPorDepartamento)
	filas, err := h.svc.PartidasDeDimension(c.Request.Context(), claims.EmpresaID, desde, hasta,
		agruparPor, c.Query("id"), c.Query("umbral"))
	if err != nil {
		h.responderError(c, err, "partidas-de-dimension")
		return
	}
	c.JSON(http.StatusOK, filas)
}

// Presupuesto GET /v1/bancos/presupuesto?desde=&hasta= (bancos.ver) — las líneas cargadas.
func (h *Handler) Presupuesto(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	lineas, err := h.svc.PresupuestoDelRango(c.Request.Context(), claims.EmpresaID, desde, hasta)
	if err != nil {
		h.responderError(c, err, "presupuesto")
		return
	}
	c.JSON(http.StatusOK, lineas)
}

type presupuestoRequest struct {
	DepartamentoID string `json:"departamento_id" binding:"required,uuid"`
	Periodo        string `json:"periodo" binding:"required"`
	Monto          string `json:"monto" binding:"required"`
	Nota           string `json:"nota"`
	// ClasificacionID vacío = el total del departamento. Con valor = subpresupuesto de esa partida.
	// `omitempty` en el binding para que se pueda omitir, y `uuid` para que si viene, venga bien: sin
	// eso un id mal escrito llega al SQL y sale como 500 en vez de decir qué está mal.
	ClasificacionID string `json:"clasificacion_id" binding:"omitempty,uuid"`
}

// GuardarPresupuesto PUT /v1/bancos/presupuesto (bancos.ajustes)
//
// Exige `bancos.ajustes` y no `bancos.ver`: fijar cuánto puede gastar un departamento es una
// decisión de dirección, no una consulta.
func (h *Handler) GuardarPresupuesto(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req presupuestoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hacen falta departamento_id (uuid), periodo (AAAA-MM) y monto")
		return
	}
	if !rePeriodo.MatchString(req.Periodo) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "periodo debe ser AAAA-MM")
		return
	}
	if err := h.svc.GuardarPresupuesto(c.Request.Context(), claims.EmpresaID,
		req.DepartamentoID, req.ClasificacionID, req.Periodo, req.Monto, req.Nota, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "guardar-presupuesto")
		return
	}
	c.Status(http.StatusNoContent)
}

// BorrarPresupuesto DELETE /v1/bancos/presupuesto (bancos.ajustes)
//
// Sin `clasificacion_id` borra el TOTAL del departamento; con él, solo el subpresupuesto de esa
// partida. Borrar el total no arrastra los subpresupuestos: el desglose queda y el control lo dice.
func (h *Handler) BorrarPresupuesto(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	deptoID := c.Query("departamento_id")
	periodo := c.Query("periodo")
	if deptoID == "" || !rePeriodo.MatchString(periodo) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hacen falta departamento_id y periodo (AAAA-MM)")
		return
	}
	clasifID := c.Query("clasificacion_id")
	if clasifID != "" && !pareceUUID(clasifID) {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "clasificacion_id no tiene forma de uuid")
		return
	}
	if err := h.svc.BorrarPresupuesto(c.Request.Context(), claims.EmpresaID, deptoID, clasifID, periodo, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "borrar-presupuesto")
		return
	}
	c.Status(http.StatusNoContent)
}
