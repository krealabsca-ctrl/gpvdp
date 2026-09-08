package bancos

// La SEGUNDA PUERTA del catálogo de gasto: la de Contabilidad.
//
// ── POR QUÉ EXISTE ──────────────────────────────────────────────────────────
//
// Cuando a CxP le entra una factura de un gasto que no está en el catálogo, no se puede clasificar
// y la factura queda trancada. Medido el 2026-09-03 en Valle de Paz: **834 de las 938 facturas que
// esperaban validación de área estaban SIN CLASIFICAR**, y solo 4 de los 22 conceptos eran visibles
// para CxP. Contabilidad no tenía forma de abrir un rubro: escribir el catálogo era exclusivo de
// `bancos.catalogo`, que abre además bancos, cuentas, naturaleza y visibilidad.
//
// Es el mismo patrón que ya usa el catálogo de departamentos (misma tabla, dos puertas, un permiso
// por puerta), y por eso vive en el paquete `bancos`: el catálogo es UNO, con dos accesos.
//
// ── QUÉ PUEDE Y QUÉ NO ──────────────────────────────────────────────────────
//
// Crear y renombrar, nada más. Lo que sigue siendo de Bancos, y por qué:
//
//   - apagar y fusionar   → el catálogo es COMPARTIDO: tocan la clasificación bancaria histórica.
//   - declarar naturaleza → es la palanca del EBITDA (mig 0060).
//   - la visibilidad      → decidir qué ve Contabilidad no es de Contabilidad.
//
// ── LA REGLA DE VISIBILIDAD (decisión del usuario, 2026-09-03) ──────────────
//
// Lo que Contabilidad crea se ve en Bancos; lo que se crea en Bancos NO se ve en CxP hasta que
// alguien lo marque visible. Así que acá lo creado nace `visible_cxp = true` y no es negociable
// desde el pedido, y en la puerta de Bancos nace en `false` (ver CrearConcepto).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

type crearConceptoCxPRequest struct {
	Nombre string `json:"nombre" validate:"required"`
}

type crearClasificacionCxPRequest struct {
	ConceptoID string `json:"concepto_id" validate:"required"`
	Nombre     string `json:"nombre" validate:"required"`
}

// CrearConceptoCxP POST /v1/cxp/catalogo/conceptos (cxp.catalogo)
//
// El concepto nace VISIBLE para CxP: Contabilidad lo está creando porque lo necesita, y crear algo
// que después no puede ver sería una trampa.
func (h *Handler) CrearConceptoCxP(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req crearConceptoCxPRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Nombre == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta el nombre del rubro")
		return
	}
	// `true` va quemado, no viene del pedido: es la regla de visibilidad, no una opción.
	con, err := h.svc.CrearConcepto(c.Request.Context(), claims.EmpresaID, req.Nombre, true, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-concepto-cxp")
		return
	}
	c.JSON(http.StatusCreated, con)
}

// RenombrarConceptoCxP PATCH /v1/cxp/catalogo/conceptos/:id (cxp.catalogo)
//
// Solo el nombre. Y solo si el concepto es visible para CxP: sin esa guarda, Contabilidad podría
// renombrar un rubro bancario que no debería ni ver, pasándole por encima al alcance del permiso.
func (h *Handler) RenombrarConceptoCxP(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req renombrarCatalogoRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Nombre == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta el nombre")
		return
	}
	if !h.conceptoVisibleParaCxP(c, claims.EmpresaID, c.Param("id")) {
		return
	}
	if err := h.svc.RenombrarConcepto(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Nombre, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "renombrar-concepto-cxp")
		return
	}
	c.Status(http.StatusNoContent)
}

// CrearClasificacionCxP POST /v1/cxp/catalogo/clasificaciones (cxp.catalogo)
//
// Cuelga de un concepto que Contabilidad SÍ ve. La clasificación hereda la visibilidad de su
// concepto, así que colgarla de uno invisible crearía un rubro que ni se ve ni se puede usar.
func (h *Handler) CrearClasificacionCxP(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req crearClasificacionCxPRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Nombre == "" || req.ConceptoID == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"hacen falta el concepto y el nombre de la clasificación")
		return
	}
	if !h.conceptoVisibleParaCxP(c, claims.EmpresaID, req.ConceptoID) {
		return
	}
	// Sin cuenta contable: ese dato es del plan contable y no lo decide quien abre el rubro. Se
	// completa después desde el catálogo de Bancos, igual que la naturaleza.
	cl, err := h.svc.CrearClasificacion(c.Request.Context(), claims.EmpresaID, req.ConceptoID, req.Nombre, "", claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "crear-clasificacion-cxp")
		return
	}
	c.JSON(http.StatusCreated, cl)
}

// RenombrarClasificacionCxP PATCH /v1/cxp/catalogo/clasificaciones/:id (cxp.catalogo)
func (h *Handler) RenombrarClasificacionCxP(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req renombrarCatalogoRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Nombre == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta el nombre")
		return
	}
	if !h.clasificacionVisibleParaCxP(c, claims.EmpresaID, c.Param("id")) {
		return
	}
	if err := h.svc.RenombrarClasificacion(c.Request.Context(), claims.EmpresaID, c.Param("id"), req.Nombre, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "renombrar-clasificacion-cxp")
		return
	}
	c.Status(http.StatusNoContent)
}

// conceptoVisibleParaCxP corta el pedido si el concepto no es de Contabilidad.
//
// Responde 404 y no 403 a propósito: para este permiso ese concepto no existe, y un 403 confirmaría
// que existe un rubro bancario con ese id. Devuelve false cuando ya escribió la respuesta.
func (h *Handler) conceptoVisibleParaCxP(c *gin.Context, empresaID, conceptoID string) bool {
	visible, err := h.svc.ConceptoEsVisibleCxP(c.Request.Context(), empresaID, conceptoID)
	if err != nil {
		h.responderError(c, err, "verificar-visibilidad-concepto")
		return false
	}
	if !visible {
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado,
			"ese rubro no está entre los de Contabilidad")
		return false
	}
	return true
}

func (h *Handler) clasificacionVisibleParaCxP(c *gin.Context, empresaID, clasificacionID string) bool {
	visible, err := h.svc.ClasificacionEsVisibleCxP(c.Request.Context(), empresaID, clasificacionID)
	if err != nil {
		h.responderError(c, err, "verificar-visibilidad-clasificacion")
		return false
	}
	if !visible {
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado,
			"esa clasificación no está entre las de Contabilidad")
		return false
	}
	return true
}
