package cxp

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// AdjuntarComprobante POST /v1/cxp/documentos/:id/comprobante — sube el PDF (multipart "archivo").
func (h *Handler) AdjuntarComprobante(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	fh, err := c.FormFile("archivo")
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "archivo requerido")
		return
	}
	if fh.Size > maxArchivoCxP {
		httpx.Abort(c, http.StatusRequestEntityTooLarge, httpx.CodeValidacion, "el archivo excede 24 MB")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxArchivoCxP))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}
	mime := fh.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/pdf"
	}
	if err := h.svc.AdjuntarComprobante(c.Request.Context(), empresaID, c.Param("id"), fh.Filename, mime, data, usuarioID); err != nil {
		h.responderError(c, err, "adjuntar-comprobante")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "filename": fh.Filename})
}

// DescargarComprobante GET /v1/cxp/documentos/:id/comprobante — descarga el PDF adjunto.
func (h *Handler) DescargarComprobante(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	comp, err := h.svc.DescargarComprobante(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "descargar-comprobante")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+comp.Filename+`"`)
	c.Data(http.StatusOK, comp.Mime, comp.Contenido)
}

// EnviarComprobante POST /v1/cxp/documentos/:id/comprobante/enviar — envía el comprobante al
// proveedor, con copia oculta al aprobador. Reenviar es esta misma llamada.
//
// La respuesta dice A QUIÉN se le mandó: sin eso la pantalla solo puede decir «listo», que es
// justamente lo que no se puede auditar después.
func (h *Handler) EnviarComprobante(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	res, err := h.svc.EnviarComprobante(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "enviar-comprobante")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "destinatario": res.Destinatario, "copia": res.Copia,
		"origen": res.Origen, "reenvio": res.Reenvio, "enviado_en": res.EnviadoEn,
	})
}

// EnviosComprobante GET /v1/cxp/documentos/:id/comprobante/envios — la bitácora de ese documento.
//
// Lista vacía = nunca se intentó. No se hizo backfill de los documentos que ya figuraban como
// enviados antes de la migración 0084: no se sabe a qué dirección se les mandó ni con qué archivo,
// y una bitácora que inventa es peor que una que arranca vacía.
func (h *Handler) EnviosComprobante(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	envios, err := h.svc.EnviosComprobante(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "envios-comprobante")
		return
	}
	c.JSON(http.StatusOK, envios)
}
