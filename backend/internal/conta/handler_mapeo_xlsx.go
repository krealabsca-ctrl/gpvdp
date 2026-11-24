package conta

// Endpoints del mapeo en .xlsx (12-nov-2026). El porqué está en mapeo_xlsx.go.

import (
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// tipoXLSX y maxMapeoXLSX: un mapeo son cientos de filas, no megabytes.
const (
	tipoXLSX      = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	maxMapeoXLSX  = 4 << 20 // 4 MiB
	campoDelMapeo = "archivo"
)

// ExportarMapeoXLSX GET /v1/conta/mapeo.xlsx (conta.mapeo)
//
// La hoja de trabajo para llenar afuera, con el plan de cuentas en la segunda hoja.
func (h *Handler) ExportarMapeoXLSX(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	datos, err := h.svc.ExportarMapeoXLSX(c.Request.Context(), cl.EmpresaID)
	if err != nil {
		h.responder(c, err, "exportar-mapeo-xlsx")
		return
	}
	nombre := NombreMapeoXLSX()
	c.Header("Content-Type", tipoXLSX)
	c.Header("Content-Disposition",
		`attachment; filename="`+nombre+`"; filename*=UTF-8''`+url.PathEscape(nombre))
	c.Data(http.StatusOK, tipoXLSX, datos)
}

// ImportarMapeoXLSX POST /v1/conta/mapeo.xlsx (conta.mapeo)
//
// multipart con `archivo`. Aplica fila por fila y devuelve qué se rechazó y por qué.
func (h *Handler) ImportarMapeoXLSX(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	archivo, err := archivoDelMapeo(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.ImportarMapeoXLSX(c.Request.Context(), cl.EmpresaID, archivo, cl.UsuarioID())
	if err != nil {
		h.responder(c, err, "importar-mapeo-xlsx")
		return
	}
	c.JSON(http.StatusOK, res)
}

// archivoDelMapeo lee el .xlsx subido. No reusa `archivoDeFormulario` porque el mensaje de aquel
// nombra el plan de cuentas, y mandar a alguien a buscar el archivo equivocado cuesta más que
// repetir diez líneas.
func archivoDelMapeo(c *gin.Context) ([]byte, error) {
	fh, err := c.FormFile(campoDelMapeo)
	if err != nil {
		return nil, errors.New("subí el Excel del mapeo (campo «archivo»)")
	}
	if fh.Size > maxMapeoXLSX {
		return nil, errors.New("el archivo excede 4 MB")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, errors.New("no se pudo leer el archivo")
	}
	defer func() { _ = f.Close() }()
	datos, err := io.ReadAll(io.LimitReader(f, maxMapeoXLSX))
	if err != nil {
		return nil, errors.New("no se pudo leer el archivo")
	}
	return datos, nil
}
