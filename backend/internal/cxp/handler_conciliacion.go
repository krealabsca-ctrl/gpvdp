package cxp

import (
	"encoding/csv"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
	"github.com/gpvdp/erp/internal/shared"
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
//
// El nombre pasa por `shared.TextoParaBanco`: sin eso, un nombre con coma obliga al escritor de CSV
// a entrecomillar el campo (y hay bancos que no esperan comillas), y una tilde o una eñe pueden
// rebotar en la plataforma. Ver el comentario de esa función.
func escribirCSVPago(c *gin.Context, rows []PagoRow) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="pagos-cxp.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"Cedula", "Nombre", "IBAN", "Moneda", "MontoNeto", "Descripcion", "Consecutivo"})
	for _, r := range rows {
		_ = w.Write([]string{r.Cedula, shared.TextoParaBanco(r.Nombre), r.IBAN, r.Moneda,
			r.MontoNeto, r.Descripcion, r.Consecutivo})
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
		desc := descripcionDePago(r)
		// El nombre se limpia con la regla del banco: sin tildes, sin eñes y sin caracteres
		// especiales. Antes solo se cambiaban las comas, así que «PETRÓLEOS DELTA S.A.» viajaba con
		// la tilde y «X; S.A.» partía la línea en dos campos.
		campos := []string{r.IBAN, shared.TextoParaBanco(r.Nombre), r.Cedula, r.MontoNeto, desc, desc, desc}
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

// clavePagoFC lee al revés lo que escribió macroDescripcion: de «FC 000487», que es lo que la
// persona copia del estado de cuenta, saca los «000487» con los que se puede volver a la factura.
// Devuelve "" cuando la búsqueda no tiene esa forma.
//
// Solo con el prefijo FC a propósito. Unos dígitos sueltos ya los encuentra el ILIKE sobre el
// consecutivo; lo que no encontraba NADA era justo el texto completo, porque «FC » no vive en
// ningún campo de la factura: lo agrega la macro al salir. Quien tenía el estado de cuenta delante
// copiaba lo que veía y la Bandeja le respondía igual que si la factura no existiera.
func clavePagoFC(q string) string {
	t := strings.ToUpper(strings.TrimSpace(q))
	if !strings.HasPrefix(t, "FC") {
		return ""
	}
	n := soloDigitos(t[2:])
	// Lo que queda entre «FC» y los dígitos tiene que ser separación, no otro texto: así «FACTURA
	// 12» no se lee como un código de pago.
	if n == "" || len(n) > 6 || strings.Trim(soloLetras(t[2:]), " ") != "" {
		return ""
	}
	for len(n) < 6 {
		n = "0" + n
	}
	return n
}

// soloLetras deja las letras de un texto; sirve para comprobar que no hay nada más que el código.
func soloLetras(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// descripcionDePago arma lo que viaja al banco para UNA transferencia, sea de una factura o de una
// remesa. Las dos formas terminan igual —referencia legible + código interno al final— porque la
// conciliación lee el código y no le importa qué hay delante:
//
//	A8 FC 000487 CXP-54A4281AE49A      (una factura, rubro con nomenclatura)
//	FC 000487 CXP-54A4281AE49A         (la misma, si su gasto todavía no tiene código)
//	A9 REM 0043-07 CXPR-7K2M9P4XQ1BT   (varias facturas de un proveedor, todas del mismo rubro)
//
// LA NOMENCLATURA VA PRIMERO y el código interno último, a propósito: es lo primero que el ojo
// busca al recorrer un estado de cuenta, y si algún día un banco truncara el campo, lo que se
// pierde es el automatismo —que se resuelve a mano— y no el sentido del pago.
func descripcionDePago(r PagoRow) string {
	partes := make([]string, 0, 3)
	if cod := NormalizarCodigoPago(r.CodigoPago); cod != "" {
		partes = append(partes, cod)
	}
	if ref := strings.TrimSpace(r.Referencia); ref != "" {
		partes = append(partes, ref)
		if h := strings.TrimSpace(r.Descripcion); h != "" {
			partes = append(partes, h)
		}
	} else {
		partes = append(partes, macroDescripcion(r.Consecutivo, r.Descripcion))
	}
	return strings.Join(partes, " ")
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
