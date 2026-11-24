package cxp

// Endpoints del marcado masivo del convenio. El porqué está en convenio_masivo.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// CandidatosConvenioHandler GET /v1/cxp/proveedores/candidatos-convenio?pendientes=1
//
// Los proveedores que declaran contado en sus comprobantes, con cuántas facturas traen así y
// cuántas tienen trancadas hoy. Es la lista que dice a cuáles conviene entrar: sobre los datos
// reales son 588 proveedores con 3.565 facturas de contado, y buscarlos a ojo en un maestro de
// 649 no es viable.
func (h *Handler) CandidatosConvenioHandler(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	lista, err := h.svc.CandidatosConvenio(c.Request.Context(), empresaID, c.Query("pendientes") == "1")
	if err != nil {
		h.responderError(c, err, "candidatos-convenio")
		return
	}
	c.JSON(http.StatusOK, lista)
}

type convenioMasivoRequest struct {
	ProveedorIDs []string `json:"proveedor_ids" validate:"required,min=1,dive,uuid"`
	Convenio     string   `json:"convenio" validate:"required,oneof=XML PROVEEDOR"`
	// PasarACredito es un PUNTERO a propósito: ausente significa «sí», que es lo que resuelve el
	// pedido. Un `bool` pelado haría que un cliente que no manda el campo caiga en `false`, o sea
	// en el comportamiento a medias que este cambio vino a eliminar, sin que nadie lo note.
	PasarACredito *bool `json:"pasar_a_credito"`
}

// MarcarConvenioMasivoHandler POST /v1/cxp/proveedores/convenio-masivo
//
// Devuelve cuántos se actualizaron y, si se marcó «el convenio manda», cuáles quedaron con la
// ficha en CONTADO: a esos la regla NO se les activa, y decirlo es la diferencia entre que la
// persona lo corrija hoy o lo descubra dentro de un mes con la factura trancada.
func (h *Handler) MarcarConvenioMasivoHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req convenioMasivoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	pasarACredito := req.PasarACredito == nil || *req.PasarACredito
	res, err := h.svc.MarcarConvenioMasivo(c.Request.Context(), empresaID,
		ConvenioMasivoInput{
			ProveedorIDs: req.ProveedorIDs, Convenio: req.Convenio, PasarACredito: pasarACredito,
		}, usuarioID)
	if err != nil {
		h.responderError(c, err, "convenio-masivo")
		return
	}
	c.JSON(http.StatusOK, res)
}

type bloqueadasRequest struct {
	ProveedorIDs []string `json:"proveedor_ids" validate:"required,min=1,dive,uuid"`
}

// BloqueadasDeProveedoresHandler POST /v1/cxp/proveedores/bloqueadas
//
// Cuántas facturas y por cuánto se liberarían. Es POST y no GET porque la lista de proveedores
// puede ser larga y no entra cómoda en la URL; no cambia nada.
func (h *Handler) BloqueadasDeProveedoresHandler(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req bloqueadasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	n, monto, err := h.svc.BloqueadasDeProveedores(c.Request.Context(), empresaID, req.ProveedorIDs)
	if err != nil {
		h.responderError(c, err, "bloqueadas-proveedores")
		return
	}
	c.JSON(http.StatusOK, gin.H{"bloqueadas": n, "monto_crc": monto})
}

type liberarMasivoRequest struct {
	ProveedorIDs []string `json:"proveedor_ids" validate:"required,min=1,dive,uuid"`
	Motivo       string   `json:"motivo" validate:"required"`
}

// LiberarDeProveedoresHandler POST /v1/cxp/proveedores/liberar-masivo
//
// Levanta el candado de todo lo bloqueado de esos proveedores. Va con `cxp.liberar_pago`, el mismo
// permiso que liberar de a una: hacerlo en lote no es una afirmación más liviana, es la misma
// repetida muchas veces.
func (h *Handler) LiberarDeProveedoresHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req liberarMasivoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	n, err := h.svc.LiberarDeProveedores(c.Request.Context(), empresaID, req.ProveedorIDs, req.Motivo, usuarioID)
	if err != nil {
		h.responderError(c, err, "liberar-masivo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"liberadas": n})
}
