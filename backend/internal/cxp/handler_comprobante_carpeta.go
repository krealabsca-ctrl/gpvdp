package cxp

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// topeArchivosCarpeta acota cuántos archivos entran de una. Una corrida grande son ~100; el tope
// deja margen y evita que un clic equivocado sobre una carpeta de miles quede esperando.
const topeArchivosCarpeta = 400

// topeBytesCarpeta acota el peso total de una subida. Los comprobantes del banco pesan unos cientos
// de kilobytes; 60 MB cubren con holgura una corrida de cien y cortan una carpeta de escaneos.
const topeBytesCarpeta = 60 << 20

// PrevisualizarCarpeta POST /v1/cxp/comprobantes/carpeta/previsualizar — recibe solo los NOMBRES.
//
// Sin los archivos a propósito: el código de pago viaja en el nombre, así que el cruce no necesita
// el contenido. Cien nombres son unos kilobytes y la respuesta llega al instante, de modo que quien
// bautizó mal un archivo lo corrige antes de esperar una subida de treinta megas.
func (h *Handler) PrevisualizarCarpeta(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var in struct {
		Nombres []string `json:"nombres"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "mandá la lista de nombres de archivo")
		return
	}
	if len(in.Nombres) > topeArchivosCarpeta {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"son demasiados archivos de una vez; subilos por corrida")
		return
	}
	prev, err := h.svc.PrevisualizarCarpeta(c.Request.Context(), empresaID, in.Nombres)
	if err != nil {
		h.responderError(c, err, "previsualizar-carpeta")
		return
	}
	c.JSON(http.StatusOK, prev)
}

// AdjuntarCarpeta POST /v1/cxp/comprobantes/carpeta — sube los archivos (multipart, campo "archivos").
//
// Responde con lo que QUEDÓ ESCRITO y con el cruce completo, no con un «ok». Son dos preguntas
// distintas —cuántos entraron y qué pasó con los que no— y la segunda es la que manda a arreglar la
// carpeta. Un 200 pelado dejaría al operador sin saber cuáles de sus cien archivos quedaron afuera.
func (h *Handler) AdjuntarCarpeta(c *gin.Context) {
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
	if len(fhs) > topeArchivosCarpeta {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"son demasiados archivos de una vez; subilos por corrida")
		return
	}
	var total int64
	for _, fh := range fhs {
		total += fh.Size
	}
	if total > topeBytesCarpeta {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"la carpeta pesa demasiado; subila en dos tandas")
		return
	}

	archivos := make([]ArchivoSubido, 0, len(fhs))
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
		mime := fh.Header.Get("Content-Type")
		if strings.TrimSpace(mime) == "" {
			mime = "application/pdf"
		}
		// El nombre llega con la ruta cuando el navegador sube una CARPETA entera
		// (webkitdirectory manda «recibos/octubre/FC 000487 CXP….pdf»). Se queda el último tramo:
		// el código vive en el nombre del archivo, y guardar la ruta del equipo de quien subió no
		// le sirve a nadie después.
		archivos = append(archivos, ArchivoSubido{
			Nombre: nombreDeArchivo(fh.Filename), Mime: mime, Contenido: data,
		})
	}

	res, prev, err := h.svc.AdjuntarCarpeta(c.Request.Context(), empresaID, archivos, usuarioID)
	if err != nil {
		// Cuando no se pudo adjuntar NADA, el cruce igual viaja: es lo que dice qué arreglar.
		if err == ErrNadaQueAdjuntar {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":    gin.H{"code": httpx.CodeReglaNegocio, "message": ErrNadaQueAdjuntar.Error()},
				"resumen":  prev.Resumen,
				"archivos": prev.Archivos,
			})
			return
		}
		h.responderError(c, err, "adjuntar-carpeta")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"adjuntados": res.Archivos,
		"facturas":   res.Facturas,
		"rechazados": res.Rechazados,
		"resumen":    prev.Resumen,
		"archivos":   prev.Archivos,
	})
}

// PagosConRecibo GET /v1/cxp/comprobantes/pagos — el control inverso: de lo pagado, qué tiene
// recibo y qué no. `lote` lo acota a una corrida; `faltantes=1` deja solo lo que falta.
func (h *Handler) PagosConRecibo(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	pagos, err := h.svc.PagosConRecibo(c.Request.Context(), empresaID,
		strings.TrimSpace(c.Query("lote")), c.Query("faltantes") == "1")
	if err != nil {
		h.responderError(c, err, "pagos-con-recibo")
		return
	}
	// Se cuenta acá y no en la pantalla: el conteo de lo que falta es el número por el que se
	// pregunta, y derivarlo en el cliente lo deja fuera de cualquier prueba del servidor.
	faltan, facturas := 0, 0
	for _, p := range pagos {
		facturas += p.Facturas
		if p.ConRecibo < p.Facturas {
			faltan++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"pagos":          pagos,
		"transferencias": len(pagos),
		"facturas":       facturas,
		"faltan":         faltan,
	})
}

// nombreDeArchivo deja el último tramo de una ruta, con separador de Windows o de Unix. No usa
// filepath.Base: el separador que importa es el del NAVEGADOR de quien sube, no el del servidor,
// y en Linux filepath.Base("recibos\\FC 1.pdf") devuelve la cadena entera.
func nombreDeArchivo(ruta string) string {
	s := strings.TrimSpace(ruta)
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	return s
}
