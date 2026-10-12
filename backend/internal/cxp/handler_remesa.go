package cxp

// Endpoints del reporte del corte y del aviso de pago.

import (
	"encoding/csv"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/shared"
)

// ReporteLote GET /v1/cxp/lotes/:id/reporte — «lo que se va a pagar», una entrada por transferencia.
//
// Es la misma información que va a la macro, leída para que una persona la revise ANTES de subir
// el archivo al banco: gasto, proveedor, factura, montos, vencimiento y total.
func (h *Handler) ReporteLote(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	rep, err := h.svc.ReporteDeLote(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "reporte-lote")
		return
	}
	if c.Query("formato") == "csv" {
		escribirReporteCSV(c, rep)
		return
	}
	c.JSON(http.StatusOK, rep)
}

// RemesasLote GET /v1/cxp/lotes/:id/remesas — los pagos agrupados del lote.
func (h *Handler) RemesasLote(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	remesas, err := h.svc.RemesasDeLote(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "remesas-lote")
		return
	}
	c.JSON(http.StatusOK, remesas)
}

// AvisoRemesa GET /v1/cxp/remesas/:id/aviso — el detalle de UNA transferencia agrupada.
func (h *Handler) AvisoRemesa(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	av, err := h.svc.AvisoDeRemesa(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "aviso-remesa")
		return
	}
	c.JSON(http.StatusOK, av)
}

// escribirReporteCSV baja el reporte como planilla, una fila por FACTURA.
//
// Plano y no agrupado a propósito: en una hoja de cálculo lo que sirve es poder filtrar y sumar,
// y la agrupación se recupera con la referencia, que se repite en cada fila del mismo pago. Así
// nadie tiene que deshacer subtotales para cruzar contra otra cosa.
func escribirReporteCSV(c *gin.Context, rep ReporteLote) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="lote-%d-por-pagar.csv"`, rep.LoteNumero))
	// BOM: sin esto Excel en Windows abre el archivo en ANSI y parte las tildes y las eñes.
	_, _ = c.Writer.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"Referencia", "Codigo interno", "Proveedor", "IBAN", "Factura", "Gasto",
		"Emision", "Vencimiento", "Total", "Anticipos", "Neto a pagar",
	})
	for _, p := range rep.Pagos {
		for _, l := range p.Lineas {
			_ = w.Write([]string{
				p.Referencia, p.Huella, shared.TextoParaBanco(p.Proveedor), p.IBAN,
				l.Consecutivo, l.Gasto, l.FechaEmision, l.FechaVencimiento,
				l.TotalCRC, l.AnticiposCRC, l.NetoCRC,
			})
		}
	}
	// El total va en su propia fila, al final y rotulado: una planilla sin total obliga a sumar a
	// mano justo el número que hay que comparar contra el banco.
	_ = w.Write(nil)
	_ = w.Write([]string{"TOTAL", "", "", "", "",
		fmt.Sprintf("%d factura(s) en %d transferencia(s)", rep.Facturas, rep.Transferencias),
		"", "", "", "", rep.TotalCRC})
	w.Flush()
}
