package cxp

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

type crearLoteRequest struct {
	FechaCorte string   `json:"fecha_corte" validate:"required,datetime=2006-01-02"`
	IDs        []string `json:"ids" validate:"required,min=1,max=1000,dive,uuid"`
	// ModoPago: "" o "FACTURA" = una transferencia por factura (el de siempre); "PROVEEDOR" = una
	// por proveedor, cubriendo todas sus facturas del lote. Se valida en el service con
	// ModoPagoValido, no con un tag, para que el mensaje explique las dos opciones.
	ModoPago string `json:"modo_pago"`
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
	lote, err := h.svc.CrearLote(c.Request.Context(), empresaID, req.FechaCorte, req.ModoPago, req.IDs, usuarioID)
	// Ninguna de las seleccionadas se pudo cortar. Va con el detalle factura por factura y NO por
	// responderError, que solo sabe mandar un mensaje: acá lo que sirve es la lista.
	var vacio *LoteVacioError
	if errors.As(err, &vacio) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":    "LOTE_VACIO",
			"message": vacio.Error(),
			"fuera":   vacio.Fuera,
		})
		return
	}
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
	rows, loteNumero, err := h.svc.MacroLote(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "macro-lote")
		return
	}
	// UNA MACRO POR MONEDA. Una transferencia en dólares sale de una cuenta en dólares, y el banco
	// no acepta las dos cosas en la misma carga; mezclarlas mandaba un monto sin moneda y lo
	// decidía la cuenta desde la que alguien subiera el archivo.
	//
	// El lote NO se parte —«sin perder la integridad del mismo»—: lo que se separa es el archivo.
	// Sin `?moneda=`, un lote de una sola moneda baja esa y uno mixto contesta qué hay, para que
	// nadie se lleve media macro creyendo que son todas las líneas.
	moneda := strings.ToUpper(strings.TrimSpace(c.Query("moneda")))
	// Se le pasa el número del lote para que `Archivo` traiga el nombre REAL de cada macro
	// («macro-lote-0043-USD.txt»): con 0 decía «macro-lote-0000-…» y la pantalla anunciaba un
	// archivo que no es el que después se baja.
	if grupos := SepararPorMoneda(rows, loteNumero); len(grupos) > 1 && moneda == "" {
		detalle := make([]gin.H, 0, len(grupos))
		for _, g := range grupos {
			detalle = append(detalle, gin.H{
				"moneda": g.Moneda, "lineas": len(g.Lineas), "total": g.Total, "archivo": g.Archivo,
			})
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code": "MACRO_MIXTA",
			"message": "Este lote tiene facturas en más de una moneda y cada una va en su propio " +
				"archivo. Pedí la macro indicando la moneda.",
			"monedas": detalle,
		})
		return
	} else if moneda != "" {
		if rows = LineasDeMoneda(rows, moneda); len(rows) == 0 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"code":    "MACRO_SIN_ESA_MONEDA",
				"message": "Este lote no tiene ninguna factura en " + moneda + ".",
			})
			return
		}
	}
	// GUARDARRAÍL: una macro SIN LÍNEAS no se baja.
	//
	// Un .txt vacío se sube al banco igual que uno bueno y no paga a nadie; el error aparece días
	// después, cuando el proveedor reclama. Pasaba con los lotes que se creaban vacíos (ya no se
	// pueden crear) y sigue siendo posible si a un lote con facturas se le bloquea la última
	// después de cortarlo, porque la consulta de la macro salta lo bloqueado.
	if len(rows) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code": "MACRO_VACIA",
			"message": "Este lote no tiene ninguna línea para pagar: o quedó sin facturas, " +
				"o las que tiene están bloqueadas para pago. Revisalo antes de subir nada al banco.",
		})
		return
	}
	// GUARDARRAÍL: sin IBAN el banco rechaza la línea. Antes la macro se bajaba igual y el error
	// aparecía en la ventanilla del banco; ahora se corta acá y se dice a quién le falta la cuenta.
	// Se puede forzar con ?igual=si para el caso de que alguien quiera el archivo parcial a mano.
	if sin := FaltanIBAN(rows); len(sin) > 0 && c.Query("igual") != "si" {
		faltantes := make([]gin.H, 0, len(sin))
		for _, r := range sin {
			faltantes = append(faltantes, gin.H{
				"proveedor": r.Nombre, "consecutivo": r.Consecutivo, "monto": r.MontoNeto,
				// El PORQUÉ de cada uno. Sin esto el mensaje decía «sin cuenta IBAN» también
				// cuando la cuenta estaba cargada pero mal escrita, y mandaba a mirar una ficha
				// donde el dato sí está.
				"motivo": MotivoIBANInvalido(r.IBAN),
			})
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":      "IBAN_FALTANTE",
			"message":   mensajeIBAN(sin),
			"faltantes": faltantes,
		})
		return
	}
	escribirMacroTxt(c, rows, loteNumero)
}
