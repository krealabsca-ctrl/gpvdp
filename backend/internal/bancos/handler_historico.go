package bancos

// Endpoints de «Cargar histórico»: subir/previsualizar y confirmar.

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// maxHistorico acota el archivo subido. Es el mismo tope del otro importador de Excel: acá una fila
// es un MOVIMIENTO y dos años de quince cuentas no caben en una sola subida.
const maxHistorico = 16 << 20 // 16 MiB

// SubirHistorico POST /v1/bancos/importaciones/historico (bancos.importar)
//
// multipart con `archivo`. NO escribe nada: guarda el archivo y devuelve el resumen POR CUENTA para
// que se apruebe antes. El id que devuelve (`carga_id`) es el que se confirma después.
func (h *Handler) SubirHistorico(c *gin.Context) {
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
	// El tope se dice en voz alta y con qué hacer. Un recorte silencioso acá dejaría medio año sin
	// cargar y nadie se enteraría hasta que no cuadre.
	if fh.Size > maxHistorico {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"el archivo excede 16 MB: partilo por año o por cuenta y subí los pedazos")
		return
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}
	defer func() { _ = f.Close() }()
	archivo, err := io.ReadAll(io.LimitReader(f, maxHistorico))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer el archivo")
		return
	}

	plan, err := h.svc.SubirHistorico(c.Request.Context(), claims.EmpresaID, fh.Filename, archivo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "subir-historico")
		return
	}
	c.JSON(http.StatusCreated, plan)
}

// PreviewHistorico GET /v1/bancos/importaciones/historico/:id (bancos.importar)
//
// Reconstruye la previsualización de una carga ya subida, sin volver a subir el archivo.
func (h *Handler) PreviewHistorico(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	plan, err := h.svc.PrevisualizarHistorico(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "preview-historico")
		return
	}
	c.JSON(http.StatusOK, plan)
}

// ConfirmarHistorico POST /v1/bancos/importaciones/historico/:id/confirmar (bancos.importar)
//
// Escribe: una importación POR CUENTA y sus movimientos, con la partida del archivo puesta.
func (h *Handler) ConfirmarHistorico(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	plan, err := h.svc.ConfirmarHistorico(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "confirmar-historico")
		return
	}
	c.JSON(http.StatusOK, plan)
}
