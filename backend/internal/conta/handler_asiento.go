package conta

// La capa HTTP del asiento y del enlace bancario (mig 0111, Fase 3A).

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Bancos GET /v1/conta/bancos (conta.bancos)
//
// La lista y su cobertura vienen juntas, igual que en el mapeo: pedirlas aparte haría que el
// contador y la lista puedan venir de instantes distintos.
func (h *Handler) Bancos(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	filas, cob, err := h.svc.Bancos(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err, "bancos-contables")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": filas, "cobertura": cob})
}

// EnlazarBanco PUT /v1/conta/bancos/:id (conta.bancos)
func (h *Handler) EnlazarBanco(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req struct {
		CuentaID string `json:"cuenta_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "elegí la cuenta contable")
		return
	}
	if err := h.svc.EnlazarBanco(c.Request.Context(), cl.EmpresaID, c.Param("id"), req.CuentaID, cl.UsuarioID()); err != nil {
		h.responder(c, err, "enlazar-banco")
		return
	}
	c.Status(http.StatusNoContent)
}

// AceptarSugerencias POST /v1/conta/bancos/aceptar-sugerencias (conta.bancos)
func (h *Handler) AceptarSugerencias(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	n, err := h.svc.AceptarSugerencias(c.Request.Context(), cl.EmpresaID, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "aceptar-sugerencias")
		return
	}
	c.JSON(http.StatusOK, gin.H{"enlazadas": n})
}

// DesenlazarBanco DELETE /v1/conta/bancos/:id (conta.bancos)
func (h *Handler) DesenlazarBanco(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.DesenlazarBanco(c.Request.Context(), cl.EmpresaID, c.Param("id"), cl.UsuarioID()); err != nil {
		h.responder(c, err, "desenlazar-banco")
		return
	}
	c.Status(http.StatusNoContent)
}

// Diario GET /v1/conta/diario?desde=&hasta=&estado=&origen=&limite=&desde_fila= (conta.libros)
func (h *Handler) Diario(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	limite, _ := strconv.Atoi(c.Query("limite"))
	desplaz, _ := strconv.Atoi(c.Query("desde_fila"))
	f := FiltroDiario{
		Desde: c.Query("desde"), Hasta: c.Query("hasta"),
		Estado: c.Query("estado"), OrigenTipo: c.Query("origen"),
		Limite: limite, Desplaz: desplaz,
	}
	items, err := h.svc.Diario(c.Request.Context(), cl.EmpresaID, f)
	if err != nil {
		h.responder(c, err, "diario")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// VerAsiento GET /v1/conta/asientos/:id (conta.libros)
func (h *Handler) VerAsiento(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	a, err := h.svc.Asiento(c.Request.Context(), cl.EmpresaID, c.Param("id"))
	if err != nil {
		h.responder(c, err, "ver-asiento")
		return
	}
	c.JSON(http.StatusOK, a)
}

// MayorDeCuenta GET /v1/conta/mayor?cuenta=&desde=&hasta= (conta.libros)
func (h *Handler) MayorDeCuenta(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	m, err := h.svc.Mayor(c.Request.Context(), cl.EmpresaID, FiltroMayor{
		CuentaID: c.Query("cuenta"), Desde: c.Query("desde"), Hasta: c.Query("hasta"),
	})
	if err != nil {
		h.responder(c, err, "mayor")
		return
	}
	c.JSON(http.StatusOK, m)
}

// CrearAsiento POST /v1/conta/asientos (conta.asientos)
func (h *Handler) CrearAsiento(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req NuevoAsiento
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el asiento no se pudo leer")
		return
	}
	a, err := h.svc.CrearAsientoManual(c.Request.Context(), cl.EmpresaID, req, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "crear-asiento")
		return
	}
	c.JSON(http.StatusCreated, a)
}

// AnularAsiento POST /v1/conta/asientos/:id/anular (conta.asientos)
//
// Anular y no borrar: borrar deja un hueco en la numeración y un mayor que ya no se puede
// reconstruir. El asiento queda en el libro, visible, sin efecto y con el motivo escrito.
func (h *Handler) AnularAsiento(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req struct {
		Motivo string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí por qué se anula")
		return
	}
	if err := h.svc.AnularAsiento(c.Request.Context(), cl.EmpresaID, c.Param("id"), req.Motivo, cl.UsuarioID()); err != nil {
		h.responder(c, err, "anular-asiento")
		return
	}
	c.Status(http.StatusNoContent)
}
