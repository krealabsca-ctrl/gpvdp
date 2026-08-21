package cxp

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

func (h *Handler) leerArchivo(c *gin.Context) ([]byte, bool) {
	fh, err := c.FormFile("archivo")
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "archivo requerido")
		return nil, false
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return nil, false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return nil, false
	}
	return data, true
}

// SubirImportacion POST /v1/cxp/importaciones — multipart (archivo .xlsx). Previsualiza sin crear nada.
func (h *Handler) SubirImportacion(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	data, ok := h.leerArchivo(c)
	if !ok {
		return
	}
	prev, err := h.svc.PreviewImportacion(c.Request.Context(), empresaID, data)
	if err != nil {
		h.responderError(c, err, "importacion-preview")
		return
	}
	c.JSON(http.StatusOK, prev)
}

// ConfirmarImportacion POST /v1/cxp/importaciones/confirmar — crea documentos + proveedores nuevos.
func (h *Handler) ConfirmarImportacion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	data, ok := h.leerArchivo(c)
	if !ok {
		return
	}
	res, err := h.svc.ConfirmarImportacion(c.Request.Context(), empresaID, data, usuarioID)
	if err != nil {
		h.responderError(c, err, "importacion-confirmar")
		return
	}
	c.JSON(http.StatusOK, res)
}
