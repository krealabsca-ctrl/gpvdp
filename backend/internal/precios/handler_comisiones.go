package precios

// Endpoints de las políticas de comisión.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// Politicas lista las políticas de comisión.
func (h *Handler) Politicas(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.Politicas(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err, "precios.politicas")
		return
	}
	c.JSON(http.StatusOK, gin.H{"politicas": lista})
}

// Vendedores lista los empleados que pueden tener política propia.
func (h *Handler) Vendedores(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.Vendedores(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err, "precios.vendedores")
		return
	}
	c.JSON(http.StatusOK, gin.H{"vendedores": lista})
}

// CrearPolitica agrega una política de comisión.
func (h *Handler) CrearPolitica(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in PoliticaInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	id, err := h.svc.CrearPolitica(c.Request.Context(), cl.EmpresaID, in, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "precios.crear_politica")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ActualizarPolitica edita una política.
func (h *Handler) ActualizarPolitica(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	var in PoliticaInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el cuerpo no es válido")
		return
	}
	if err := h.svc.ActualizarPolitica(c.Request.Context(), cl.EmpresaID, c.Param("id"), in); err != nil {
		h.responder(c, err, "precios.actualizar_politica")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// EliminarPolitica borra una política.
func (h *Handler) EliminarPolitica(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	if err := h.svc.EliminarPolitica(c.Request.Context(), cl.EmpresaID, c.Param("id")); err != nil {
		h.responder(c, err, "precios.eliminar_politica")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ComisionDelPlan resuelve qué política aplica y cuánto cuesta.
//
// Devuelve 200 con `politica: null` cuando ninguna aplica. NO es un error: es información —no
// hay política para ese plan— y la pantalla tiene que poder mostrarlo como un faltante, no como
// una falla.
func (h *Handler) ComisionDelPlan(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	com, err := h.svc.ComisionDelPlan(c.Request.Context(), cl.EmpresaID, c.Param("id"),
		c.Query("vendedor"), c.Query("fecha"))
	if err != nil {
		h.responder(c, err, "precios.comision_del_plan")
		return
	}
	if com == nil {
		c.JSON(http.StatusOK, gin.H{
			"comision": nil,
			"motivo":   "no hay política de comisión para este plan: el costo no la incluye",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"comision": com})
}
