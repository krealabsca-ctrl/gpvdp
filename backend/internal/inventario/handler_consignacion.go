package inventario

// Endpoints de consignación.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// Consignadas GET /v1/inventario/consignacion?proveedor_id=&estado=&situacion= (inventario.ver)
func (h *Handler) Consignadas(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	proveedorID, ok := uuidDeQuery(c, "proveedor_id")
	if !ok {
		return
	}
	cola, err := h.svc.Consignadas(c.Request.Context(), cl.EmpresaID, FiltroConsignacion{
		ProveedorID: proveedorID,
		Estado:      c.Query("estado"),
		Situacion:   c.Query("situacion"),
	})
	if err != nil {
		h.responder(c, err, "consignadas")
		return
	}
	c.JSON(http.StatusOK, cola)
}

// FacturarConsignada POST /v1/inventario/consignacion/:unidadId/facturar (inventario.consignacion)
//
// Crea la provisión de la cuenta por pagar. Es un POST sin cuerpo: todo lo que hace falta —proveedor,
// monto, fecha, servicio— sale de la unidad. Pedirlo por el cuerpo dejaría que el cliente mandara un
// monto distinto del que dice el inventario.
func (h *Handler) FacturarConsignada(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	unidadID, ok := uuidDeRuta(c, "unidadId", "la unidad")
	if !ok {
		return
	}
	// confirmar=true es el «facturar a mano» de los casos que no son un uso normal (se dañó, el
	// conteo no la encontró). Va explícito en la llamada para que quede claro que alguien lo decidió.
	u, err := h.svc.FacturarConsignada(c.Request.Context(), cl.EmpresaID, unidadID, cl.UsuarioID(),
		c.Query("confirmar") == "true")
	if err != nil {
		h.responder(c, err, "facturar consignada")
		return
	}
	c.JSON(http.StatusOK, u)
}

type conciliarConsignadaRequest struct {
	// DocumentoID es la factura electrónica REAL del proveedor, ya cargada en CxP. Con `uuid` en el
	// binding: un id mal escrito tiene que salir como 400 en el borde y no como un 500 desde el SQL.
	DocumentoID string `json:"documento_id" binding:"required,uuid"`
}

// ConciliarConsignada POST /v1/inventario/consignacion/:unidadId/conciliar (inventario.consignacion)
//
// Reemplaza la provisión por la factura real: anula la que generó el sistema y enlaza la unidad a la
// del proveedor. Es la mitad que evita el doble pago.
func (h *Handler) ConciliarConsignada(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	unidadID, ok := uuidDeRuta(c, "unidadId", "la unidad")
	if !ok {
		return
	}
	var req conciliarConsignadaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"hace falta indicar la factura del proveedor contra la que se concilia, con su identificador completo")
		return
	}
	u, err := h.svc.ConciliarConsignada(c.Request.Context(), cl.EmpresaID, unidadID,
		req.DocumentoID, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "conciliar consignada")
		return
	}
	c.JSON(http.StatusOK, u)
}

// CandidatasConciliacion GET /v1/inventario/consignacion/:unidadId/candidatas (inventario.ver)
//
// Las facturas del proveedor que SÍ sirven para conciliar. La pantalla ofrecía todas y el servidor
// rechazaba después: elegir de una lista que incluye opciones inválidas es una trampa.
func (h *Handler) CandidatasConciliacion(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	unidadID, ok := uuidDeRuta(c, "unidadId", "la unidad")
	if !ok {
		return
	}
	res, err := h.svc.CandidatasParaConciliar(c.Request.Context(), cl.EmpresaID, unidadID)
	if err != nil {
		h.responder(c, err, "candidatas de conciliación")
		return
	}
	c.JSON(http.StatusOK, res)
}

// Servicio GET /v1/inventario/servicios/:id (inventario.ver)
//
// El repositorio ya sabía traerlo pero no tenía ruta: la única forma de ver qué consumió un servicio
// era la respuesta del POST que lo creó, así que después de cerrar la pantalla el dato se perdía.
func (h *Handler) Servicio(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	sv, err := h.svc.Servicio(c.Request.Context(), cl.EmpresaID, c.Param("id"))
	if err != nil {
		h.responder(c, err, "servicio")
		return
	}
	c.JSON(http.StatusOK, sv)
}

// Unidad GET /v1/inventario/unidades/:numero (inventario.ver) — la ficha de un objeto físico.
func (h *Handler) Unidad(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	u, err := h.svc.Unidad(c.Request.Context(), cl.EmpresaID, c.Param("numero"))
	if err != nil {
		h.responder(c, err, "unidad")
		return
	}
	c.JSON(http.StatusOK, u)
}
