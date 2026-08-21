package bancos

// Endpoints del diccionario del catálogo (exportar / previsualizar / importar).

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// maxDiccionario acota el archivo: un diccionario son cientos de filas, no megabytes.
const maxDiccionario = 4 << 20 // 4 MiB

// ExportarDiccionario GET /v1/bancos/catalogo/diccionario (bancos.exportar)
// .xlsx con Concepto › Clasificación y sus palabras clave. Se puede reimportar tal cual.
func (h *Handler) ExportarDiccionario(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	buf, err := h.svc.ExportarDiccionario(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "exportar-diccionario")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="diccionario-catalogo.xlsx"`)
	c.Data(http.StatusOK, xlsxContentType, buf)
}

// ImportarDiccionario POST /v1/bancos/catalogo/diccionario (bancos.reglas)
// multipart con `archivo`. Con ?aplicar=true escribe; sin eso, solo previsualiza.
//
// Exige bancos.reglas (y no solo bancos.catalogo) porque una fila con palabras clave crea una
// regla del motor: quien importa el diccionario tiene que poder crear reglas.
func (h *Handler) ImportarDiccionario(c *gin.Context) {
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
	if fh.Size > maxDiccionario {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el archivo excede 4 MB")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}
	defer func() { _ = f.Close() }()
	archivo, err := io.ReadAll(io.LimitReader(f, maxDiccionario))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}

	plan, err := h.svc.ImportarDiccionario(c.Request.Context(), claims.EmpresaID, archivo,
		c.Query("aplicar") == "true", claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "importar-diccionario")
		return
	}
	c.JSON(http.StatusOK, plan)
}
