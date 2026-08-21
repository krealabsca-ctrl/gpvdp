package cxp

import (
	"encoding/csv"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// ArchivoPago GET /v1/cxp/pagos/archivo?fecha=YYYY-MM-DD — CSV formato SINPE de los PROGRAMADOS.
func (h *Handler) ArchivoPago(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	rows, err := h.svc.ArchivoPago(c.Request.Context(), empresaID, c.Query("fecha"))
	if err != nil {
		h.responderError(c, err, "archivo-pago")
		return
	}
	escribirCSVPago(c, rows)
}

// escribirCSVPago serializa las líneas del archivo de pago (formato SINPE) al response.
func escribirCSVPago(c *gin.Context, rows []PagoRow) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="pagos-cxp.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"Cedula", "Nombre", "IBAN", "Moneda", "MontoNeto", "Descripcion", "Consecutivo"})
	for _, r := range rows {
		_ = w.Write([]string{r.Cedula, r.Nombre, r.IBAN, r.Moneda, r.MontoNeto, r.Descripcion, r.Consecutivo})
	}
	w.Flush()
}

// escribirMacroTxt genera la macro de pago en el formato del banco (TXT, sin encabezado,
// separado por comas): IBAN, Nombre, Cédula, Monto, Descripción, Descripción, Descripción.
// La descripción se repite 3 veces según lo exige la macro.
func escribirMacroTxt(c *gin.Context, rows []PagoRow) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="macro-pagos.txt"`)
	var b strings.Builder
	for _, r := range rows {
		desc := macroDescripcion(r.Consecutivo, r.Descripcion)
		campos := []string{r.IBAN, sanitizarMacro(r.Nombre), r.Cedula, r.MontoNeto, desc, desc, desc}
		b.WriteString(strings.Join(campos, ","))
		b.WriteString("\r\n")
	}
	_, _ = c.Writer.WriteString(b.String())
}

// macroDescripcion arma la descripción que viaja al banco: "FC XXXXXX <huella>".
//
// «FC» + los 6 dígitos es la nomenclatura que pidió el usuario (2026-08-17): es lo que el
// proveedor necesita reconocer en su estado de cuenta. Los 6 son los ÚLTIMOS del consecutivo,
// conservando los ceros de la izquierda (la factura 651 se paga como «FC 000651»).
//
// La HUELLA va al final y no es decorativa: es lo que permite que, al importar el estado de
// cuenta, el movimiento se empareje solo con este pago —verificando el monto—. Va última a
// propósito: si el banco truncara el campo, se pierde el emparejamiento automático (que se
// resuelve a mano) pero NO el sentido del pago para el proveedor.
func macroDescripcion(consecutivo, huella string) string {
	n := soloDigitos(consecutivo)
	desc := "FC"
	if n != "" {
		if len(n) > 6 {
			n = n[len(n)-6:]
		}
		for len(n) < 6 {
			n = "0" + n
		}
		desc += " " + n
	}
	if h := strings.TrimSpace(huella); h != "" {
		desc += " " + h
	}
	return desc
}

func soloDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizarMacro quita comas (el formato es separado por comas sin comillas).
func sanitizarMacro(s string) string {
	return strings.ReplaceAll(s, ",", " ")
}

type conciliarMatchRequest struct {
	Descripcion string `json:"descripcion" validate:"required"`
	Monto       string `json:"monto"` // opcional (la huella es la llave; monto/fecha reservados para validación extra)
	Fecha       string `json:"fecha"`
}

// ConciliarMatch POST /v1/cxp/conciliacion/match — empareja un movimiento bancario con un pago CxP por huella.
func (h *Handler) ConciliarMatch(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req conciliarMatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	doc, conciliado, err := h.svc.Conciliar(c.Request.Context(), empresaID, req.Descripcion, usuarioID)
	if err != nil {
		h.responderError(c, err, "conciliar-match")
		return
	}
	if !conciliado {
		c.JSON(http.StatusOK, gin.H{"conciliado": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conciliado": true, "documento": doc})
}
