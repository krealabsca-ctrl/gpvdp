package cxp

// Endpoints del arqueo de excepción y del corte del lote (hallazgos 3.1 y 3.3).

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// ReporteExcepcionHandler GET /v1/cxp/excepcion?desde=&hasta=
//
// Las fechas acotan SOLO la columna de débitos sin factura; por defecto, los últimos 90 días. Las
// facturas sin respaldo nunca se acotan: la más vieja es la más grave y filtrarla por mes la
// escondería justo cuando más importa.
func (h *Handler) ReporteExcepcionHandler(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	hasta := c.Query("hasta")
	desde := c.Query("desde")
	if hasta == "" {
		// Hoy en Costa Rica, con el mismo offset que ya usa el tablero de CxP: un reporte que
		// arranque un día corrido deja fuera lo que se debitó anoche.
		hasta = time.Now().UTC().Add(offsetCR).Format("2006-01-02")
	}
	if desde == "" {
		h, err := time.Parse("2006-01-02", hasta)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "la fecha «hasta» tiene que ser AAAA-MM-DD")
			return
		}
		desde = h.AddDate(0, 0, -90).Format("2006-01-02")
	}
	rep, err := h.svc.ReporteDeExcepcion(c.Request.Context(), empresaID, desde, hasta)
	if err != nil {
		h.responderError(c, err, "reporte-excepcion")
		return
	}
	c.JSON(http.StatusOK, rep)
}

// CerrarLoteHandler POST /v1/cxp/lotes/:id/cerrar
//
// Devuelve el lote y el respaldo bancario del corte: qué facturas suyas el banco todavía no
// debitó. Cerrar NO se bloquea por eso —el banco puede ir atrasado y el corte seguir bien dado—,
// pero es el único momento en que alguien está mirando ese lote con intención de terminarlo.
func (h *Handler) CerrarLoteHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	lote, respaldo, err := h.svc.CerrarLote(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "cerrar-lote")
		return
	}
	c.JSON(http.StatusOK, gin.H{"lote": lote, "respaldo": respaldo})
}

type reabrirLoteRequest struct {
	Motivo string `json:"motivo" validate:"required"`
}

// ReabrirLoteHandler POST /v1/cxp/lotes/:id/reabrir
func (h *Handler) ReabrirLoteHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req reabrirLoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	lote, err := h.svc.ReabrirLote(c.Request.Context(), empresaID, c.Param("id"), usuarioID, req.Motivo)
	if err != nil {
		h.responderError(c, err, "reabrir-lote")
		return
	}
	c.JSON(http.StatusOK, lote)
}

// RespaldoDelLoteHandler GET /v1/cxp/lotes/:id/respaldo
//
// El mismo cálculo del cierre, sin cerrar: para poder mirarlo ANTES de dar el corte.
func (h *Handler) RespaldoDelLoteHandler(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	r, err := h.svc.RespaldoBancarioDelLote(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "respaldo-lote")
		return
	}
	c.JSON(http.StatusOK, r)
}
