package bancos

// Endpoint de «Mi presupuesto». El porqué está en mi_presupuesto.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MiPresupuesto GET /v1/bancos/mi-presupuesto (bancos.ver_mi_presupuesto)
//
// ENDPOINT PROPIO y no `/bancos/control` con un filtro: ese lo usan otras superficies con permiso
// de Finanzas, y meterle un recorte por alcance es cómo un día se le muestra a alguien lo que no
// debía. Es la misma decisión que tomó «Mi partida» con `/bancos/mi-segmento/movimientos`.
//
// El usuario sale del TOKEN, nunca de un parámetro: si el id de la persona viajara en el query,
// cualquiera podría pedir el presupuesto de otra.
func (h *Handler) MiPresupuesto(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	res, err := h.svc.MiPresupuestoDeArea(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(),
		desde, hasta, c.Query("umbral"))
	if err != nil {
		h.responderError(c, err, "mi-presupuesto")
		return
	}
	c.JSON(http.StatusOK, res)
}

// MiIngreso GET /v1/bancos/mi-ingreso (bancos.ver_mi_ingreso)
//
// Permiso SEPARADO del de gasto: el usuario dijo que «en algunos casos aplica solo para gastos,
// en otras ingresos también». Con un permiso único no habría forma de dar uno sin el otro.
func (h *Handler) MiIngreso(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	res, err := h.svc.MiIngresoDePartidas(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(), desde, hasta)
	if err != nil {
		h.responderError(c, err, "mi-ingreso")
		return
	}
	c.JSON(http.StatusOK, res)
}

// MisPartidasDeArea GET /v1/bancos/mi-presupuesto/partidas (bancos.ver_mi_presupuesto)
//
// El desglose del área: en qué partidas se fue el gasto y en qué meses. El área va en el query
// porque la persona ELIGE cuál mirar cuando tiene más de una; el servicio comprueba que sea suya.
func (h *Handler) MisPartidasDeArea(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	res, err := h.svc.MisPartidasDeArea(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(),
		desde, hasta, c.Query("departamento_id"), c.Query("umbral"))
	if err != nil {
		h.responderError(c, err, "mis-partidas-de-area")
		return
	}
	c.JSON(http.StatusOK, res)
}

// MiSerieDeIngreso GET /v1/bancos/mi-ingreso/serie (bancos.ver_mi_ingreso)
func (h *Handler) MiSerieDeIngreso(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	desde, hasta, ok := rangoDeMeses(c)
	if !ok {
		return
	}
	res, err := h.svc.MiSerieDeIngreso(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(), desde, hasta)
	if err != nil {
		h.responderError(c, err, "mi-serie-de-ingreso")
		return
	}
	c.JSON(http.StatusOK, res)
}

// Periodos GET /v1/bancos/periodos — qué meses tiene cargados la empresa.
//
// SIN PERMISO DE MÓDULO, como `/rbac/mis-permisos`: el selector de período está en la barra de
// TODAS las pantallas, también las de quien no tiene nada de Bancos —un jefe de área con solo
// «Mi presupuesto»—, y un 403 acá le dejaría la barra rota. Devuelve etiquetas «YYYY-MM» y nada
// más: ni montos, ni cuentas, ni partidas.
func (h *Handler) Periodos(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	res, err := h.svc.Periodos(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "periodos")
		return
	}
	c.JSON(http.StatusOK, res)
}
