package precios

// Endpoints del costeo por actividad.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// Drivers lista los generadores de costo.
func (h *Handler) Drivers(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.Drivers(c.Request.Context(), cl.EmpresaID, c.Query("periodo"))
	if err != nil {
		h.responder(c, err, "precios.drivers")
		return
	}
	c.JSON(http.StatusOK, gin.H{"drivers": lista})
}

// CrearDriver agrega un generador de costo.
func (h *Handler) CrearDriver(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in DriverInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	id, err := h.svc.CrearDriver(c.Request.Context(), cl.EmpresaID, in)
	if err != nil {
		h.responder(c, err, "precios.crear_driver")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// EliminarDriver quita un driver que no esté en uso.
func (h *Handler) EliminarDriver(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	if err := h.svc.EliminarDriver(c.Request.Context(), cl.EmpresaID, c.Param("id")); err != nil {
		h.responder(c, err, "precios.eliminar_driver")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// FijarBase guarda la medición de un driver para un período.
func (h *Handler) FijarBase(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in struct {
		Periodo  string `json:"periodo"`
		Cantidad string `json:"cantidad"`
		Nota     string `json:"nota"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.FijarBase(c.Request.Context(), cl.EmpresaID, c.Param("id"),
		in.Periodo, in.Cantidad, in.Nota, cl.UsuarioID()); err != nil {
		h.responder(c, err, "precios.fijar_base")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Repartos lista las clasificaciones de gasto con su reparto y el monto real del período.
func (h *Handler) Repartos(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.repo.Repartos(c.Request.Context(), cl.EmpresaID, c.Query("desde"), c.Query("hasta"))
	if err != nil {
		h.responder(c, err, "precios.repartos")
		return
	}
	// El conteo de repartos inválidos va en la respuesta y no se calcula en el cliente: es la
	// cifra que dice si el costeo se puede usar, y tiene que ser la misma para todos.
	malos := 0
	for _, r := range lista {
		if !r.Valido && (len(r.Lineas) > 0) {
			malos++
		}
	}
	c.JSON(http.StatusOK, gin.H{"repartos": lista, "con_problema": malos})
}

// FijarReparto asigna parte de un gasto a un driver.
func (h *Handler) FijarReparto(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in struct {
		ClasificacionID string `json:"clasificacion_id"`
		DriverID        string `json:"driver_id"`
		Porcentaje      string `json:"porcentaje"`
		VigenteDesde    string `json:"vigente_desde"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.FijarReparto(c.Request.Context(), cl.EmpresaID, in.ClasificacionID,
		in.DriverID, in.Porcentaje, in.VigenteDesde, cl.UsuarioID()); err != nil {
		h.responder(c, err, "precios.fijar_reparto")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// QuitarReparto borra una línea de reparto.
func (h *Handler) QuitarReparto(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	if err := h.svc.QuitarReparto(c.Request.Context(), cl.EmpresaID, c.Param("id")); err != nil {
		h.responder(c, err, "precios.quitar_reparto")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Repartir devuelve la foto del período: bolsas, costos unitarios y lo que quedó sin absorber.
func (h *Handler) Repartir(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	rp, err := h.svc.Repartir(c.Request.Context(), cl.EmpresaID, c.Query("desde"), c.Query("hasta"))
	if err != nil {
		h.responder(c, err, "precios.repartir")
		return
	}
	c.JSON(http.StatusOK, rp)
}

// ConsumosDelPlan dice cuánto consume un plan de cada driver.
func (h *Handler) ConsumosDelPlan(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.repo.ConsumosDelPlan(c.Request.Context(), cl.EmpresaID, c.Param("id"))
	if err != nil {
		h.responder(c, err, "precios.consumos_del_plan")
		return
	}
	c.JSON(http.StatusOK, gin.H{"consumos": lista})
}

// FijarConsumo dice cuántas unidades de un driver consume un plan.
func (h *Handler) FijarConsumo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in struct {
		DriverID string `json:"driver_id"`
		Consumo  string `json:"consumo"`
		Nota     string `json:"nota"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.FijarConsumo(c.Request.Context(), cl.EmpresaID, c.Param("id"),
		in.DriverID, in.Consumo, in.Nota); err != nil {
		h.responder(c, err, "precios.fijar_consumo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// QuitarConsumo saca un driver de un plan.
func (h *Handler) QuitarConsumo(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	if err := h.svc.QuitarConsumo(c.Request.Context(), cl.EmpresaID, c.Param("id"), c.Param("driverID")); err != nil {
		h.responder(c, err, "precios.quitar_consumo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Absorcion devuelve el indirecto que le toca a un plan en el período.
func (h *Handler) Absorcion(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	a, err := h.svc.AbsorcionDelPlan(c.Request.Context(), cl.EmpresaID, c.Param("id"),
		c.Query("desde"), c.Query("hasta"))
	if err != nil {
		h.responder(c, err, "precios.absorcion")
		return
	}
	c.JSON(http.StatusOK, a)
}
