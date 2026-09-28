package bancos

// Endpoints del listado de cargas y de la reversa (mig 0085).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Importaciones GET /v1/bancos/importaciones?cuenta_bancaria_id=&page=&page_size= (bancos.importar)
//
// Las cargas de la empresa del token, la más reciente primero, con lo que hace falta para decidir
// sin abrir otra pantalla: qué trajo, cuánta plata, quién la subió, qué se apoya en ella y si ya
// fue revertida.
func (h *Handler) Importaciones(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.Importaciones(c.Request.Context(), claims.EmpresaID, FiltrosImportaciones{
		CuentaBancariaID: c.Query("cuenta_bancaria_id"),
		Page:             atoiDefault(c.Query("page"), 1),
		PageSize:         atoiDefault(c.Query("page_size"), 50),
	})
	if err != nil {
		h.responderError(c, err, "importaciones")
		return
	}
	c.JSON(http.StatusOK, res)
}

// reversaRequest es el cuerpo de revertir y de deshacer.
//
// El motivo es OBLIGATORIO al revertir (lo valida el servicio, que es donde vive la regla) y
// opcional al deshacer. No se valida acá con `binding` para que el rechazo salga con el mensaje
// redactado del centinela y no con el texto del validador.
type reversaRequest struct {
	Motivo string `json:"motivo"`
}

// RevertirImportacion POST /v1/bancos/importaciones/:id/revertir (bancos.revertir_importacion)
func (h *Handler) RevertirImportacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req reversaRequest
	_ = c.ShouldBindJSON(&req) // un cuerpo ilegible es un motivo vacío: lo rechaza el servicio

	res, err := h.svc.RevertirImportacion(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		req.Motivo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "revertir importación")
		return
	}
	c.JSON(http.StatusOK, res)
}

// DeshacerReversaImportacion POST /v1/bancos/importaciones/:id/deshacer-reversa (bancos.revertir_importacion)
func (h *Handler) DeshacerReversaImportacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req reversaRequest
	_ = c.ShouldBindJSON(&req) // cuerpo opcional

	res, err := h.svc.DeshacerReversaImportacion(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		req.Motivo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "deshacer reversa")
		return
	}
	c.JSON(http.StatusOK, res)
}
