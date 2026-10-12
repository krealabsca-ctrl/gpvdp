package cxp

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// topeArchivosXML acota cuántos XML entran de una. Un mes de facturas de Valle de Paz ronda los
// 120; el tope deja margen para subir un trimestre sin que una carpeta de años quede esperando.
const topeArchivosXML = 600

// topeBytesXML acota el peso total. Un comprobante pesa entre 5 y 40 KB —son XML, no escaneos— así
// que 40 MB cubren con holgura seiscientos y cortan una carpeta con PDF mezclados.
const topeBytesXML = 40 << 20

// CargarDetalleXML POST /v1/cxp/detalle/xml — sube XML (multipart, campo "archivos").
//
// Responde con el parte completo: lo que entró, lo que no encontró factura y lo que no se pudo
// leer. El segundo grupo es el interesante —son facturas que existen en Hacienda y no en el
// sistema— y por eso viaja con nombre de archivo y clave, no como un contador.
func (h *Handler) CargarDetalleXML(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer la subida")
		return
	}
	fhs := form.File["archivos"]
	if len(fhs) == 0 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no llegó ningún archivo")
		return
	}
	if len(fhs) > topeArchivosXML {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"son demasiados archivos de una vez; subilos por tandas")
		return
	}
	var total int64
	for _, fh := range fhs {
		total += fh.Size
	}
	if total > topeBytesXML {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"la carpeta pesa demasiado; subila en dos tandas")
		return
	}

	archivos := make([]ArchivoXML, 0, len(fhs))
	for _, fh := range fhs {
		f, err := fh.Open()
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer «"+fh.Filename+"»")
			return
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer «"+fh.Filename+"»")
			return
		}
		// El nombre llega con la ruta cuando se sube una CARPETA entera; se queda el último tramo.
		// Acá el nombre es SOLO para poder ir a buscar el archivo en el disco si algo falló: el
		// emparejamiento se hace con la clave que viene adentro del XML.
		archivos = append(archivos, ArchivoXML{Nombre: nombreDeArchivo(fh.Filename), Datos: data})
	}

	res, err := h.svc.CargarDetalleXML(c.Request.Context(), empresaID, archivos, usuarioID)
	if err != nil {
		h.responderError(c, err, "cargar-detalle-xml")
		return
	}
	c.JSON(http.StatusOK, res)
}

// LineasDeDocumento GET /v1/cxp/documentos/:id/lineas — el detalle guardado de una factura.
func (h *Handler) LineasDeDocumento(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	lineas, err := h.svc.LineasDeDocumento(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "lineas-de-documento")
		return
	}
	c.JSON(http.StatusOK, gin.H{"lineas": lineas})
}

// AnalisisGasto GET /v1/cxp/gasto — la lista de todo lo que se compra, por CABYS.
func (h *Handler) AnalisisGasto(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	res, err := h.svc.AnalisisGasto(c.Request.Context(), empresaID, filtrosGastoDe(c))
	if err != nil {
		h.responderError(c, err, "analisis-gasto")
		return
	}
	c.JSON(http.StatusOK, res)
}

// ComprasDeItem GET /v1/cxp/gasto/:cabys — cada compra de un ítem.
func (h *Handler) ComprasDeItem(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	cabys := strings.TrimSpace(c.Param("cabys"))
	if cabys == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "falta el código del ítem")
		return
	}
	compras, err := h.svc.ComprasDeItem(c.Request.Context(), empresaID, cabys, filtrosGastoDe(c))
	if err != nil {
		h.responderError(c, err, "compras-de-item")
		return
	}
	c.JSON(http.StatusOK, gin.H{"compras": compras})
}

// filtrosGastoDe lee el filtro de la query.
//
// El texto se normaliza con la MISMA función que escribió `detalle_norm` al guardar. Si se buscara
// con el texto crudo, escribir "café" no encontraría nada —en la columna está "cafe"— y la
// pantalla diría que no se compra café, que es peor que no tener buscador.
func filtrosGastoDe(c *gin.Context) FiltrosGasto {
	limite, _ := strconv.Atoi(c.Query("limite"))
	return FiltrosGasto{
		Desde:       strings.TrimSpace(c.Query("desde")),
		Hasta:       strings.TrimSpace(c.Query("hasta")),
		ProveedorID: strings.TrimSpace(c.Query("proveedor_id")),
		Texto:       normalizarDetalle(c.Query("q")),
		Limite:      limite,
	}
}
