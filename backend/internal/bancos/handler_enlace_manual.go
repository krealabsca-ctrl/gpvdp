package bancos

// Endpoint del enlace manual (hallazgo 3.1). El porqué está en enlace_manual.go.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

type enlazarAManoRequest struct {
	MovimientoID string `json:"movimiento_id" validate:"required,uuid"`
	DocumentoID  string `json:"documento_id" validate:"required,uuid"`
	Motivo       string `json:"motivo" validate:"required"`
}

// EnlazarPagoAMano POST /v1/bancos/conciliacion-cxp/enlazar
//
// Cierra una factura contra un débito que eligió una persona, para los casos que el barrido por
// huella no puede resolver: el pago salió, pero la descripción del banco no trae el código.
func (h *Handler) EnlazarPagoAMano(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req enlazarAManoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}

	res, err := h.svc.EnlazarPagoAMano(c.Request.Context(), claims.EmpresaID,
		req.MovimientoID, req.DocumentoID, claims.UsuarioID(), req.Motivo)
	if err != nil {
		h.responderError(c, err, "enlazar-pago-a-mano")
		return
	}

	// Los veredictos que NO son un fallo del sistema sino la respuesta. Van 422 con la cifra de los
	// dos lados: sin ella el mensaje diría que algo no calza sin decir contra qué.
	switch res.Veredicto {
	case HuellaMontoDiferente:
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"el banco debitó "+res.MontoBanco+" y la factura espera "+res.MontoEsperado+
				"; si fue un pago parcial o hubo una nota de crédito, todavía no hay camino para registrarlo")
		return
	case HuellaMonedaDiferente:
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"el débito está en otra moneda que la factura ("+res.MonedaEsperada+
				"): no se convierte sola porque con qué tipo de cambio es una decisión de negocio")
		return
	case HuellaEstadoInvalido:
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto,
			"esa factura no está declarada como pagada, así que no hay pago que conciliar")
		return
	case HuellaSinDocumento:
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, "no se encontró esa factura")
		return
	}
	c.JSON(http.StatusOK, res)
}

type desenlazarRequest struct {
	MovimientoID string `json:"movimiento_id" validate:"required,uuid"`
	Motivo       string `json:"motivo" validate:"required"`
}

// DesenlazarPago POST /v1/bancos/conciliacion-cxp/desenlazar
//
// Deshace una conciliación: el movimiento deja de apuntar a la factura y la factura vuelve a
// aparecer en el arqueo esperando su débito. Existe porque enlazar a mano era un camino de ida: si
// alguien empareja el par equivocado —dos facturas del mismo proveedor por el mismo monto en la
// misma semana es corriente—, sin esto la factura quedaba cerrada contra plata ajena para siempre.
func (h *Handler) DesenlazarPago(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req desenlazarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	n, err := h.svc.DesenlazarPago(c.Request.Context(), claims.EmpresaID,
		req.MovimientoID, claims.UsuarioID(), req.Motivo)
	if err != nil {
		h.responderError(c, err, "desenlazar-pago")
		return
	}
	c.JSON(http.StatusOK, gin.H{"facturas_reabiertas": n})
}
