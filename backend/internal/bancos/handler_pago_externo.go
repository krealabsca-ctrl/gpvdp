package bancos

// Endpoints del sello «aplicado en el sistema anterior»: resumen, previsualizar, confirmar y
// revertir una carga.

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// maxArchivoReporte acota lo que se acepta subir. Un reporte de pagos de varios años pesa unos
// pocos MB; más que esto es un archivo equivocado y vale más decirlo que quedarse sin memoria.
const maxArchivoReporte = 25 << 20 // 25 MB

// ResumenPagosExternos GET /v1/bancos/pagos-externos (bancos.ver_conciliacion)
//
// Cuánto hay sellado y —lo que importa— cuántos débitos quedan sin explicar.
func (h *Handler) ResumenPagosExternos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.ResumenPagosExternos(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "pagos-externos-resumen")
		return
	}
	c.JSON(http.StatusOK, res)
}

// PrevisualizarPagosExternos POST /v1/bancos/pagos-externos/previsualizar (bancos.importar)
//
// Lee el reporte del otro ERP y dice QUÉ PASARÍA. No escribe nada.
func (h *Handler) PrevisualizarPagosExternos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	archivo, ok := h.archivoDelReporte(c)
	if !ok {
		return
	}
	rep, _, err := h.svc.PrevisualizarPagosExternos(c.Request.Context(), claims.EmpresaID, archivo)
	if err != nil {
		h.responderErrorDeReporte(c, err)
		return
	}
	c.JSON(http.StatusOK, rep)
}

// ConfirmarPagosExternos POST /v1/bancos/pagos-externos/confirmar (bancos.importar)
//
// Se manda el MISMO archivo: la fuente de verdad es el archivo, no un estado a medias guardado
// entre los dos pasos.
func (h *Handler) ConfirmarPagosExternos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	archivo, ok := h.archivoDelReporte(c)
	if !ok {
		return
	}
	rep, carga, n, err := h.svc.ConfirmarPagosExternos(
		c.Request.Context(), claims.EmpresaID, claims.UsuarioID(), archivo)
	if err != nil {
		h.responderErrorDeReporte(c, err)
		return
	}
	despues, err := h.svc.ResumenPagosExternos(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "pagos-externos-confirmar")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"reporte": rep, "carga_id": carga, "sellados": n, "despues": despues,
	})
}

// RevertirPagosExternos POST /v1/bancos/pagos-externos/:carga/revertir (bancos.revertir_importacion)
//
// Quita las marcas de una carga entera. Pide el MISMO permiso que revertir una carga de banco: las
// dos deshacen en bloque algo que alguien confirmó.
func (h *Handler) RevertirPagosExternos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	n, err := h.svc.RevertirCargaDeSellos(
		c.Request.Context(), claims.EmpresaID, claims.UsuarioID(), c.Param("carga"))
	if err != nil {
		h.responderError(c, err, "pagos-externos-revertir")
		return
	}
	c.JSON(http.StatusOK, gin.H{"quitados": n})
}

// archivoDelReporte saca el archivo del multipart y lo lee acotado.
func (h *Handler) archivoDelReporte(c *gin.Context) ([]byte, bool) {
	fh, err := c.FormFile("archivo")
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "falta el archivo del reporte")
		return nil, false
	}
	if fh.Size > maxArchivoReporte {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el archivo pasa de 25 MB: revisá si es el reporte correcto")
		return nil, false
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo abrir el archivo")
		return nil, false
	}
	defer func() { _ = f.Close() }()

	datos, err := io.ReadAll(io.LimitReader(f, maxArchivoReporte))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return nil, false
	}
	return datos, true
}

// responderErrorDeReporte traduce los centinelas del lector a un 400 con el mensaje redactado.
//
// Van acá y no en `responderError` porque son de VALIDACIÓN del archivo, no fallas del servidor: un
// reporte que no se entiende es un problema del archivo y la persona tiene que poder leer por qué.
// Un centinela nuevo que no se traduzca sale como 500 «error interno», que es justo el modo de
// falla que ya nos costó caro en CxP.
func (h *Handler) responderErrorDeReporte(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrReporteSinEncabezado):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"no se reconoció el encabezado: el reporte tiene que traer una columna de referencia, o una de fecha y otra de monto")
	case errors.Is(err, ErrReporteSinFilas):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el reporte no trae ninguna fila de pago debajo del encabezado")
	case errors.Is(err, ErrReporteFormato):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, ErrReporteFormato.Error())
	case errors.Is(err, ErrReporteSinLlave):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el reporte no trae ni referencia, ni fecha, ni monto: sin alguno de esos no hay forma de cruzarlo contra el banco")
	default:
		h.responderError(c, err, "pagos-externos")
	}
}
