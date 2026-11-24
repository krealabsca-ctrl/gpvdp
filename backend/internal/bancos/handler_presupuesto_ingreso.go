package bancos

// HTTP del presupuesto de INGRESO (mig 0116) y del resultado presupuestado.

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// metaIngresoRequest es lo que manda la pantalla al guardar una meta.
type metaIngresoRequest struct {
	Periodo string `json:"periodo"`
	Monto   string `json:"monto"`
	Nota    string `json:"nota"`
	// ClasificacionID vacío = el TOTAL del mes; con valor = la meta de esa línea.
	ClasificacionID string `json:"clasificacion_id"`
}

// MetasDeIngreso GET /v1/bancos/presupuesto-ingreso?desde=&hasta= (bancos.ver_control)
func (h *Handler) MetasDeIngreso(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	metas, err := h.svc.MetasDeIngreso(c.Request.Context(), claims.EmpresaID, desde, hasta)
	if err != nil {
		h.responderError(c, err, "metas-de-ingreso")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": metas})
}

// GuardarMetaIngreso PUT /v1/bancos/presupuesto-ingreso (bancos.ajustes)
//
// Mismo permiso que el presupuesto de gasto: son las dos mitades de una sola decisión de gestión
// —cuánto esperamos que entre y cuánto autorizamos gastar— y partirla en dos permisos obligaría a
// mantener dos listas para un solo acto.
func (h *Handler) GuardarMetaIngreso(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req metaIngresoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"hacen falta periodo (AAAA-MM) y monto")
		return
	}
	if err := h.svc.GuardarMetaIngreso(c.Request.Context(), claims.EmpresaID,
		req.ClasificacionID, req.Periodo, req.Monto, req.Nota, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "guardar-meta-ingreso")
		return
	}
	c.Status(http.StatusNoContent)
}

// BorrarMetaIngreso DELETE /v1/bancos/presupuesto-ingreso (bancos.ajustes)
//
// Sin `clasificacion_id` borra el TOTAL del mes; con él, solo la meta de esa línea. Borrar es la
// forma de volver a «no definida»: una meta en CERO es una decisión distinta —«no se espera
// ingreso por acá»— y por eso no sirve para quitarla.
func (h *Handler) BorrarMetaIngreso(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	periodo := c.Query("periodo")
	if err := h.svc.BorrarMetaIngreso(c.Request.Context(), claims.EmpresaID,
		c.Query("clasificacion_id"), periodo, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "borrar-meta-ingreso")
		return
	}
	c.Status(http.StatusNoContent)
}

// ControlDeIngreso GET /v1/bancos/control-ingreso?desde=&hasta= (bancos.ver_control)
func (h *Handler) ControlDeIngreso(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	ctrl, err := h.svc.ControlDeIngreso(c.Request.Context(), claims.EmpresaID, desde, hasta)
	if err != nil {
		h.responderError(c, err, "control-de-ingreso")
		return
	}
	c.JSON(http.StatusOK, ctrl)
}

// ResultadoPresupuestado GET /v1/bancos/resultado-presupuestado?desde=&hasta= (bancos.ver_control)
//
// Es lo que no se podía calcular con media meta: ingreso esperado menos gasto autorizado, contra
// lo mismo en real.
func (h *Handler) ResultadoPresupuestado(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	res, err := h.svc.ResultadoPresupuestado(c.Request.Context(), claims.EmpresaID, desde, hasta)
	if err != nil {
		h.responderError(c, err, "resultado-presupuestado")
		return
	}
	c.JSON(http.StatusOK, res)
}

// PropuestaDeMeta GET /v1/bancos/presupuesto-ingreso/propuesta?desde=&hasta=&ajuste_pct=
// (bancos.ver_control)
//
// Propone, no guarda: devuelve lo que entró en el mismo rango del año anterior, ajustado por el
// porcentaje pedido. Con 41 líneas de ingreso, una planilla vacía se queda vacía.
func (h *Handler) PropuestaDeMeta(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	filas, err := h.svc.PropuestaDeMeta(c.Request.Context(), claims.EmpresaID, desde, hasta,
		c.DefaultQuery("ajuste_pct", "0"))
	if err != nil {
		h.responderError(c, err, "propuesta-de-meta")
		return
	}
	// UNA LISTA VACÍA NO SE DEVUELVE MUDA. Sin histórico del año anterior —y la carga de Valle de
	// Paz arranca en julio de 2026, así que pasa de verdad— la pantalla recibía `[]` y el botón
	// parecía roto. El aviso dice qué rango se miró y por qué no hay nada.
	resp := gin.H{"items": filas, "desde_base": unAnioAtras(desde), "hasta_base": unAnioAtras(hasta)}
	if len(filas) == 0 {
		resp["aviso"] = "No hay ingreso registrado entre " + unAnioAtras(desde) + " y " +
			unAnioAtras(hasta) + ", así que no hay desde qué proponer. Cargá la meta a mano."
	}
	c.JSON(http.StatusOK, resp)
}

// Outlook GET /v1/bancos/outlook?anio=2026 (bancos.ver_control)
//
// La tercera columna: cómo cierra el año. Mismo permiso que el control presupuestario porque es
// la misma pregunta vista a doce meses en vez de a un rango.
func (h *Handler) Outlook(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	anio := time.Now().Year()
	if s := strings.TrimSpace(c.Query("anio")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el año tiene que ser un número")
			return
		}
		anio = n
	}
	out, err := h.svc.Outlook(c.Request.Context(), claims.EmpresaID, anio)
	if err != nil {
		h.responderError(c, err, "outlook")
		return
	}
	c.JSON(http.StatusOK, out)
}
