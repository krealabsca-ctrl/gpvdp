package precios

// Endpoints del importador del catálogo.

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// maxArchivo es el tope del archivo de costeo. El real pesa ~200 KB; 15 MB deja margen de sobra
// y evita que una subida equivocada ocupe memoria sin control.
const maxArchivo = 15 << 20

// leerArchivo saca el .xlsx del formulario. El mensaje de tamaño dice CUÁNTO pesa el archivo y
// cuál es el tope: «supera el máximo» a secas obliga a adivinar de cuánto se pasó.
func leerArchivo(c *gin.Context) ([]byte, bool) {
	fh, err := c.FormFile("archivo")
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "falta el archivo")
		return nil, false
	}
	if fh.Size > maxArchivo {
		httpx.Abort(c, http.StatusRequestEntityTooLarge, httpx.CodeValidacion,
			"el archivo pesa "+megas(fh.Size)+" y el máximo es "+megas(maxArchivo))
		return nil, false
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo abrir el archivo")
		return nil, false
	}
	defer func() { _ = f.Close() }()
	datos, err := io.ReadAll(io.LimitReader(f, maxArchivo+1))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return nil, false
	}
	return datos, true
}

func megas(b int64) string {
	m := float64(b) / (1 << 20)
	return strconv.FormatFloat(m, 'f', 1, 64) + " MB"
}

// AnalizarImportacion mide qué pasaría, sin escribir nada.
func (h *Handler) AnalizarImportacion(c *gin.Context) {
	if _, ok := claims(c); !ok {
		return
	}
	datos, ok := leerArchivo(c)
	if !ok {
		return
	}
	a, err := h.svc.AnalizarArchivo(datos, c.PostForm("hoja"), c.PostForm("columna_nombre"), c.PostForm("columna_costo"))
	if err != nil {
		h.responder(c, err, "precios.analizar_importacion")
		return
	}
	c.JSON(http.StatusOK, a)
}

// PlanearImportacion devuelve la lista exacta que se escribiría.
func (h *Handler) PlanearImportacion(c *gin.Context) {
	if _, ok := claims(c); !ok {
		return
	}
	datos, ok := leerArchivo(c)
	if !ok {
		return
	}
	plan, err := h.svc.PlanearImportacion(datos, c.PostForm("hoja"),
		c.PostForm("columna_nombre"), c.PostForm("columna_costo"),
		c.PostForm("unificar_sedes") == "true", c.PostFormArray("sedes"))
	if err != nil {
		h.responder(c, err, "precios.planear_importacion")
		return
	}
	// La muestra va acotada: el plan puede traer cientos de insumos y la pantalla solo necesita
	// el conteo y unos ejemplos para que el usuario reconozca que es su archivo.
	muestra := plan.Insumos
	if len(muestra) > 25 {
		muestra = muestra[:25]
	}
	c.JSON(http.StatusOK, gin.H{
		"total":         len(plan.Insumos),
		"con_costo":     plan.ConCosto,
		"sin_costo":     plan.SinCosto,
		"muestra":       muestra,
		"confirmar_con": len(plan.Insumos),
	})
}

// Importar escribe el catálogo.
func (h *Handler) Importar(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	datos, ok := leerArchivo(c)
	if !ok {
		return
	}
	confirmacion, _ := strconv.Atoi(c.PostForm("confirmacion"))
	res, err := h.svc.Importar(c.Request.Context(), cl.EmpresaID, datos, OpcionesImportacion{
		Hoja:          c.PostForm("hoja"),
		ColumnaNombre: c.PostForm("columna_nombre"),
		ColumnaCosto:  c.PostForm("columna_costo"),
		UnificarSedes: c.PostForm("unificar_sedes") == "true",
		Sedes:         c.PostFormArray("sedes"),
		VigenteDesde:  c.PostForm("vigente_desde"),
		Confirmacion:  confirmacion,
	}, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "precios.importar")
		return
	}
	c.JSON(http.StatusOK, res)
}

// AnalizarRecetas mide qué planes y recetas traería la matriz de costeo, sin escribir nada.
func (h *Handler) AnalizarRecetas(c *gin.Context) {
	if _, ok := claims(c); !ok {
		return
	}
	datos, ok := leerArchivo(c)
	if !ok {
		return
	}
	a, err := h.svc.AnalizarRecetasDelArchivo(datos, c.PostForm("hoja"))
	if err != nil {
		h.responder(c, err, "precios.analizar_recetas")
		return
	}
	c.JSON(http.StatusOK, a)
}

// ImportarRecetas escribe los planes y sus recetas.
func (h *Handler) ImportarRecetas(c *gin.Context) {
	cl, ok := claims(c)
	if !ok {
		return
	}
	datos, ok := leerArchivo(c)
	if !ok {
		return
	}
	// Las modalidades llegan como pares «CODIGO:MODALIDAD», uno por plan que el usuario corrija.
	modalidades := map[string]string{}
	for _, par := range c.PostFormArray("modalidad") {
		if i := strings.IndexByte(par, ':'); i > 0 {
			modalidades[par[:i]] = strings.ToUpper(par[i+1:])
		}
	}
	confirmacion, _ := strconv.Atoi(c.PostForm("confirmacion"))
	res, err := h.svc.ImportarRecetas(c.Request.Context(), cl.EmpresaID, datos, OpcionesRecetas{
		Hoja:         c.PostForm("hoja"),
		VigenteDesde: c.PostForm("vigente_desde"),
		Modalidades:  modalidades,
		Confirmacion: confirmacion,
	}, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "precios.importar_recetas")
		return
	}
	c.JSON(http.StatusOK, res)
}
