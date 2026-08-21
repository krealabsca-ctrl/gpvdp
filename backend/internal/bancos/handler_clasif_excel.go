package bancos

// Endpoints de la clasificación en bloque desde Excel (plantilla / previsualizar / aplicar).

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// maxClasifExcel acota el archivo. Es más grande que el del diccionario porque acá una fila es un
// MOVIMIENTO: una cuenta activa de Valle de Paz tiene más de nueve mil en mes y medio.
const maxClasifExcel = 16 << 20 // 16 MiB

// PlantillaClasificacion GET /v1/bancos/movimientos/plantilla-clasificacion (bancos.exportar)
//
// Baja el .xlsx que se llena en Excel y se vuelve a subir. Parámetros:
//
//	desde, hasta        YYYY-MM-DD (por defecto, el último año hasta hoy)
//	solo_sin_clasificar «false» para traer también lo ya clasificado (revisar o corregir)
func (h *Handler) PlantillaClasificacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	hoy := AhoraCR()
	hasta := c.Query("hasta")
	if hasta == "" {
		hasta = hoy.Format("2006-01-02")
	}
	desde := c.Query("desde")
	if desde == "" {
		desde = hoy.AddDate(-1, 0, 0).Format("2006-01-02")
	}
	// Validar la forma en el borde: una fecha mal escrita llegaría a `to_date()` y saldría como 500.
	for _, v := range []string{desde, hasta} {
		if _, err := time.Parse("2006-01-02", v); err != nil {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "desde y hasta deben ser AAAA-MM-DD")
			return
		}
	}
	if desde > hasta {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "desde no puede ser posterior a hasta")
		return
	}
	solo := c.Query("solo_sin_clasificar") != "false"

	buf, n, err := h.svc.PlantillaClasificacion(c.Request.Context(), claims.EmpresaID, desde, hasta, solo)
	if err != nil {
		h.responderError(c, err, "plantilla-clasificacion")
		return
	}
	// El conteo va en un encabezado propio: el navegador descarga el archivo y la pantalla necesita
	// poder decir «trae N movimientos» sin abrirlo.
	c.Header("X-Filas", itoa(n))
	c.Header("Access-Control-Expose-Headers", "X-Filas, Content-Disposition")
	nombre := h.svc.NombreArchivo(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(), "clasificar")
	c.Header("Content-Disposition", `attachment; filename="`+nombre+`"`)
	c.Data(http.StatusOK, xlsxContentType, buf)
}

// ImportarClasificacionExcel POST /v1/bancos/movimientos/clasificar-excel (bancos.clasificar)
//
// multipart con `archivo` y, opcionalmente, `cuenta_bancaria_id` para cuando el archivo no trae
// columna de cuenta. Con ?aplicar=true escribe; sin eso, solo previsualiza.
// Con ?reemplazar=true también cambia lo que YA tenía otra partida (por defecto no se toca).
func (h *Handler) ImportarClasificacionExcel(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	fh, err := c.FormFile("archivo")
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "falta el archivo (campo «archivo»)")
		return
	}
	if fh.Size > maxClasifExcel {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el archivo excede 16 MB: partilo por cuenta o por año")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}
	defer func() { _ = f.Close() }()
	archivo, err := io.ReadAll(io.LimitReader(f, maxClasifExcel))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}

	plan, err := h.svc.ImportarClasificacionExcel(c.Request.Context(), claims.EmpresaID, archivo,
		c.PostForm("cuenta_bancaria_id"),
		c.Query("reemplazar") == "true",
		c.Query("aplicar") == "true",
		claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "clasificar-excel")
		return
	}
	c.JSON(http.StatusOK, plan)
}
