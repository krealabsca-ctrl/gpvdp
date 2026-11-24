package conta

// La capa HTTP de la Fase 4: balance, apertura y cierre de período.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Balance GET /v1/conta/balance?desde=YYYY-MM-DD&hasta=YYYY-MM-DD (conta.libros)
//
// Mismo permiso que los libros: es lectura, y un auditor interno lo necesita sin poder escribir.
func (h *Handler) Balance(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	b, err := h.svc.Balance(c.Request.Context(), cl.EmpresaID, c.Query("desde"), c.Query("hasta"))
	if err != nil {
		h.responder(c, err, "balance")
		return
	}
	c.JSON(http.StatusOK, b)
}

// Periodos GET /v1/conta/periodos (conta.libros)
func (h *Handler) Periodos(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	ps, err := h.svc.Periodos(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err, "periodos")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": ps})
}

// CerrarPeriodo POST /v1/conta/periodos/cerrar (conta.periodos)
func (h *Handler) CerrarPeriodo(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req struct {
		Periodo string `json:"periodo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí qué período")
		return
	}
	if err := h.svc.CerrarPeriodo(c.Request.Context(), cl.EmpresaID, req.Periodo, cl.UsuarioID()); err != nil {
		h.responder(c, err, "cerrar-periodo")
		return
	}
	c.Status(http.StatusNoContent)
}

// ReabrirPeriodo POST /v1/conta/periodos/reabrir (conta.periodos)
//
// Exige motivo: es la primera pregunta que hace un auditor al ver un mes que se cerró y se
// volvió a abrir.
func (h *Handler) ReabrirPeriodo(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req struct {
		Periodo string `json:"periodo"`
		Motivo  string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí qué período y por qué")
		return
	}
	if err := h.svc.ReabrirPeriodo(c.Request.Context(), cl.EmpresaID, req.Periodo, req.Motivo, cl.UsuarioID()); err != nil {
		h.responder(c, err, "reabrir-periodo")
		return
	}
	c.Status(http.StatusNoContent)
}

// PrevisualizarApertura POST /v1/conta/apertura/previsualizar (conta.periodos)
//
// NO escribe: dice qué saldos traería el archivo y si cuadran.
func (h *Handler) PrevisualizarApertura(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	archivo, err := archivoDeFormulario(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	prev, err := h.svc.PrevisualizarApertura(c.Request.Context(), cl.EmpresaID, c.PostForm("periodo"), archivo)
	if err != nil {
		h.responder(c, err, "previsualizar-apertura")
		return
	}
	c.JSON(http.StatusOK, prev)
}

// CargarApertura POST /v1/conta/apertura (conta.periodos)
func (h *Handler) CargarApertura(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	archivo, err := archivoDeFormulario(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	a, err := h.svc.CargarApertura(c.Request.Context(), cl.EmpresaID, c.PostForm("periodo"), archivo, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "cargar-apertura")
		return
	}
	c.JSON(http.StatusCreated, a)
}
