package cxp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

type crearLoteRequest struct {
	FechaCorte string   `json:"fecha_corte" validate:"required,datetime=2006-01-02"`
	IDs        []string `json:"ids" validate:"required,min=1,max=1000,dive,uuid"`
}

// CrearLote POST /v1/cxp/lotes — arma un lote de pago con las facturas seleccionadas.
func (h *Handler) CrearLote(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req crearLoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	lote, err := h.svc.CrearLote(c.Request.Context(), empresaID, req.FechaCorte, req.IDs, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-lote")
		return
	}
	c.JSON(http.StatusCreated, lote)
}

// ListarLotes GET /v1/cxp/lotes
func (h *Handler) ListarLotes(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	lotes, err := h.svc.Lotes(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "listar-lotes")
		return
	}
	c.JSON(http.StatusOK, lotes)
}

// MacroLote GET /v1/cxp/lotes/:id/macro — macro .txt del lote (formato del banco).
func (h *Handler) MacroLote(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	rows, err := h.svc.MacroLote(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "macro-lote")
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
