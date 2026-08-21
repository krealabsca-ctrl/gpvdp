package cxc

// HTTP de las notas de crédito. Las autoriza el supervisor de piso y no tienen tope: el
// control es el motivo obligatorio, el consecutivo y la auditoría, no un límite de monto.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/httpx"
)

type notaRequest struct {
	Contrato string `json:"contrato" binding:"required"`
	// CargoID opcional: si viene, la nota va a ese cargo; si no, al más viejo (FIFO).
	CargoID string `json:"cargo_id" validate:"omitempty,uuid"`
	Fecha   string `json:"fecha"`
	Monto   string `json:"monto" binding:"required"`
	Motivo  string `json:"motivo" binding:"required"`
}

// EmitirNota POST /v1/cxc/notas-credito
func (h *Handler) EmitirNota(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req notaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"faltan datos: contrato, monto y motivo son obligatorios")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	monto, err := decimal.NewFromString(req.Monto)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "monto inválido")
		return
	}
	nota, err := h.svc.EmitirNotaCredito(c.Request.Context(), empresaID, NotaCreditoInput{
		Contrato: req.Contrato, CargoID: req.CargoID, Fecha: req.Fecha, Monto: monto, Motivo: req.Motivo,
	}, usuarioID)
	if err != nil {
		h.error(c, err, "emitir-nota-credito")
		return
	}
	c.JSON(http.StatusCreated, nota)
}

type anularNotaRequest struct {
	Motivo string `json:"motivo" binding:"required"`
}

// AnularNota POST /v1/cxc/notas-credito/:id/anular — devuelve los cargos a su saldo.
func (h *Handler) AnularNota(c *gin.Context) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req anularNotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta el motivo de la anulación")
		return
	}
	if err := h.svc.AnularNotaCredito(c.Request.Context(), empresaID, c.Param("id"), req.Motivo, usuarioID); err != nil {
		h.error(c, err, "anular-nota-credito")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Notas GET /v1/cxc/notas-credito — con el resumen de lo condonado y por quién.
func (h *Handler) Notas(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	lista, err := h.svc.ListarNotas(c.Request.Context(), empresaID, FiltrosNotas{
		Contrato:        c.Query("contrato"),
		Desde:           c.Query("desde"),
		Hasta:           c.Query("hasta"),
		IncluirAnuladas: c.Query("incluir_anuladas") == "true",
		Page:            atoiDefault(c.Query("page"), 1),
		PageSize:        atoiDefault(c.Query("page_size"), 50),
	})
	if err != nil {
		h.error(c, err, "notas-credito")
		return
	}
	c.JSON(http.StatusOK, lista)
}
