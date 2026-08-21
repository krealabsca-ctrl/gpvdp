package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

type transicionMasivaRequest struct {
	Accion              string   `json:"accion" validate:"required,oneof=revisar aprobar programar pagar conciliar denegar anular liquidar rebotar reintentar"`
	IDs                 []string `json:"ids" validate:"required,min=1,max=500,dive,uuid"`
	FechaPagoProgramada string   `json:"fecha_pago_programada" validate:"omitempty,datetime=2006-01-02"`
	// Nota: motivo del archivo (denegar/anular/liquidar/rebotar). Opcional.
	Nota string `json:"nota" validate:"omitempty,max=500"`
}

// TransicionMasiva POST /v1/cxp/documentos/transicion-masiva — aplica una acción del flujo
// a un lote de documentos. La ruta se abre a la unión de roles del flujo; el servicio
// reverifica que el rol pueda ejecutar la acción concreta (403 si no).
func (h *Handler) TransicionMasiva(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req transicionMasivaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	res, err := h.svc.TransicionMasiva(c.Request.Context(), claims.EmpresaID, claims.UsuarioID(),
		claims.Rol, req.Accion, req.IDs, req.FechaPagoProgramada, req.Nota)
	if err != nil {
		h.responderError(c, err, "transicion-masiva")
		return
	}
	c.JSON(http.StatusOK, res)
}

type archivoPagoLoteRequest struct {
	IDs []string `json:"ids" validate:"required,min=1,max=1000,dive,uuid"`
}

// ArchivoPagoLote POST /v1/cxp/pagos/archivo — CSV SINPE de los documentos PROGRAMADOS
// indicados por id (la "macro" del lote seleccionado). Mismo formato que el GET por fecha.
func (h *Handler) ArchivoPagoLote(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req archivoPagoLoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	rows, err := h.svc.ArchivoPagoLote(c.Request.Context(), empresaID, req.IDs)
	if err != nil {
		h.responderError(c, err, "archivo-pago-lote")
		return
	}
	// GUARDARRAÍL: sin IBAN el banco rechaza la línea. Antes la macro se bajaba igual y el error
	// aparecía en la ventanilla del banco; ahora se corta acá y se dice a quién le falta la cuenta.
	// Se puede forzar con ?igual=si para el caso de que alguien quiera el archivo parcial a mano.
	if sin := FaltanIBAN(rows); len(sin) > 0 && c.Query("igual") != "si" {
		faltantes := make([]gin.H, 0, len(sin))
		for _, r := range sin {
			faltantes = append(faltantes, gin.H{"proveedor": r.Nombre, "consecutivo": r.Consecutivo, "monto": r.MontoNeto})
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":      "IBAN_FALTANTE",
			"message":   "Hay pagos sin cuenta IBAN: el banco rechazaría esas líneas. Cargá el IBAN de estos proveedores y volvé a generar la macro.",
			"faltantes": faltantes,
		})
		return
	}
	escribirMacroTxt(c, rows)
}
