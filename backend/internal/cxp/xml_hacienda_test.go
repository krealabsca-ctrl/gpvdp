package cxp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// construirXLSXDePrueba arma el .xlsx de facturación mínimo, para verificar que el despachador
// sigue mandando ese formato al parser de Excel.
func construirXLSXDePrueba(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	hoja := f.GetSheetName(0)
	encabezados := []string{
		"Clave (50 dígitos)", "Fecha de Emisión", "Número Consecutivo", "Nombre del Proveedor",
		"Cédula", "Moneda", "Subtotal (Total Venta)", "Total Impuestos", "Total Comprobante",
	}
	fila := []string{
		claveAgosto13, "2026-08-13", "45300001010000000401", "COMAPAN S.A",
		"3101090247", "CRC", "100.00", "13.00", "113.00",
	}
	for i := range encabezados {
		enc, _ := excelize.CoordinatesToCellName(i+1, 1)
		val, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(hoja, enc, encabezados[i])
		_ = f.SetCellValue(hoja, val, fila[i])
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("escribir xlsx: %v", err)
	}
	return buf.Bytes()
}

// ── Fábrica de comprobantes de prueba ───────────────────────────────────────────────────────
//
// Los nombres de los elementos NO son inventados: son exactamente los que el Apps Script de
// facturación viene leyendo en producción sobre miles de facturas (`Clave`, `NumeroConsecutivo`,
// `FechaEmision`, `CondicionVenta`, `PlazoCredito`, `Emisor>Identificacion>Numero`,
// `ResumenFactura`, `CodigoTipoMoneda>CodigoMoneda`, `TipoCambio`, `TotalVenta`, `TotalDescuentos`,
// `TotalImpuesto`, `TotalComprobante`). El único nodo sin ese respaldo es `Receptor`, y por eso el
// diseño lo trata como dato a cotejar y no como verdad: si viniera vacío, la recepción se parquea.

const claveAgosto13 = "50613082600310109024745300001010000000401177634001"
const claveAgosto05 = "50605082600310109024745300001010000000402177634002"

// facturaV44 arma una factura electrónica v4.4 con el nodo de moneda COMPUESTO.
func facturaV44(clave, condicion, plazo, moneda, tc, neta, impuesto, total string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica">
  <Clave>` + clave + `</Clave>
  <NumeroConsecutivo>45300001010000000401</NumeroConsecutivo>
  <FechaEmision>2026-08-13T14:30:00-06:00</FechaEmision>
  <Emisor>
    <Nombre>COMAPAN S.A</Nombre>
    <Identificacion><Tipo>02</Tipo><Numero>3101090247</Numero></Identificacion>
  </Emisor>
  <Receptor>
    <Nombre>VALLE DE PAZ SERVICIOS FUNERARIOS</Nombre>
    <Identificacion><Tipo>02</Tipo><Numero>3101555555</Numero></Identificacion>
  </Receptor>
  <CondicionVenta>` + condicion + `</CondicionVenta>
  <PlazoCredito>` + plazo + `</PlazoCredito>
  <ResumenFactura>
    <CodigoTipoMoneda><CodigoMoneda>` + moneda + `</CodigoMoneda><TipoCambio>` + tc + `</TipoCambio></CodigoTipoMoneda>
    <TotalVenta>` + neta + `</TotalVenta>
    <TotalDescuentos>0.00000</TotalDescuentos>
    <TotalVentaNeta>` + neta + `</TotalVentaNeta>
    <TotalImpuesto>` + impuesto + `</TotalImpuesto>
    <TotalComprobante>` + total + `</TotalComprobante>
  </ResumenFactura>
  <ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:SignedInfo><ds:Reference><ds:DigestValue>abc</ds:DigestValue></ds:Reference></ds:SignedInfo></ds:Signature>
</FacturaElectronica>`
}

// El caso completo y realista: crédito a 30 días en dólares, con firma incrustada y sangría.
func TestParsearFacturasXMLLeeLaFacturaCompleta(t *testing.T) {
	filas, traza, err := parsearFacturasXML([]byte(
		facturaV44(claveAgosto13, "02", "30", "USD", "512.35", "200.00000", "26.00000", "226.00000")))
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %d, quería 1", len(filas))
	}
	f := filas[0]
	casos := []struct{ campo, got, want string }{
		{"clave", f.Clave, claveAgosto13},
		{"consecutivo", f.Consecutivo, "45300001010000000401"},
		// De la CLAVE (posiciones 4-9), no del <FechaEmision>.
		{"fecha_emision", f.FechaEmision, "2026-08-13"},
		{"proveedor", f.Proveedor, "COMAPAN S.A"},
		{"cedula", f.Cedula, "3101090247"},
		{"moneda", f.Moneda, "USD"},
		{"tc", f.TC, "512.35"},
		{"subtotal", f.Subtotal, "200.00000"},
		{"iva", f.IVA, "26.00000"},
		{"total", f.Total, "226.00000"},
		{"condicion", f.Condicion, "Crédito"},
		// Emisión + PlazoCredito del XML (decisión del 2026-09-09).
		{"vencimiento", f.Vencimiento, "2026-09-12"},
		{"tipo_documento", f.TipoDocumento, TipoFacturaElectronica},
		// El Receptor: el dato que el Excel perdía y que decide de qué empresa es la factura.
		{"receptor", f.Receptor, "3101555555"},
		{"descuadre", f.Descuadre, ""},
	}
	for _, c := range casos {
		if c.got != c.want {
			t.Errorf("%s = %q, quería %q", c.campo, c.got, c.want)
		}
	}
	if len(traza.Versiones) != 1 || traza.Versiones[0] != "v4.4" {
		t.Errorf("versiones = %v, quería [v4.4]: el informe tiene que decir contra qué esquema se leyó", traza.Versiones)
	}
	// Y el plazo tiene que poder derivarse de vuelta, porque es lo que aprende el proveedor.
	cond, plazo := condicionDeFila(f)
	if cond != "CREDITO" || plazo != 30 {
		t.Errorf("condicionDeFila = %s/%d, quería CREDITO/30", cond, plazo)
	}
}

// El namespace NO va en los tags: un solo struct tiene que leer las tres versiones, con prefijo y
// sin xmlns. Si algún día alguien "mejora" el parser poniendo la URI, este test se cae.
func TestParsearFacturasXMLEsIndiferenteAlNamespace(t *testing.T) {
	cuerpo := `<Clave>` + claveAgosto13 + `</Clave>
  <FechaEmision>2026-08-13T14:30:00-06:00</FechaEmision>
  <Emisor><Nombre>X</Nombre><Identificacion><Numero>3101090247</Numero></Identificacion></Emisor>
  <ResumenFactura><CodigoMoneda>CRC</CodigoMoneda><TotalVentaNeta>100.00</TotalVentaNeta><TotalImpuesto>13.00</TotalImpuesto><TotalComprobante>113.00</TotalComprobante></ResumenFactura>`

	casos := []struct{ nombre, xml, wantVersion string }{
		{
			"v4.3 con xmlns por defecto",
			`<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.3/facturaElectronica">` + cuerpo + `</FacturaElectronica>`,
			"v4.3",
		},
		{
			"v4.2 con xmlns por defecto",
			`<FacturaElectronica xmlns="https://tribunet.hacienda.go.cr/docs/esquemas/2017/v4.2/facturaElectronica">` + cuerpo + `</FacturaElectronica>`,
			"v4.2",
		},
		{
			// Hacienda emite con xmlns por defecto, pero un intermediario puede reescribir con
			// prefijo. Para Go es indistinguible: xml.Name.Space siempre trae la URI.
			"con prefijo fe:",
			`<fe:FacturaElectronica xmlns:fe="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica">` +
				strings.NewReplacer("<Clave>", "<fe:Clave>", "</Clave>", "</fe:Clave>").Replace(cuerpo) +
				`</fe:FacturaElectronica>`,
			"v4.4",
		},
		{"sin xmlns del todo", `<FacturaElectronica>` + cuerpo + `</FacturaElectronica>`, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			filas, traza, err := parsearFacturasXML([]byte(c.xml))
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(filas) != 1 || filas[0].Clave != claveAgosto13 || filas[0].Total != "113.00" {
				t.Fatalf("no se leyó bien: %+v", filas)
			}
			if c.wantVersion != "" && (len(traza.Versiones) != 1 || traza.Versiones[0] != c.wantVersion) {
				t.Errorf("versiones = %v, quería [%s]", traza.Versiones, c.wantVersion)
			}
		})
	}
}

// Solo la factura electrónica genera deuda. Los otros seis tipos se cuentan y se informan.
//
// Sin la lista blanca por la RAÍZ del documento, una NOTA DE CRÉDITO —que RESTA— se registraría
// como una cuenta por pagar, sumando.
func TestParsearFacturasXMLSoloLaFacturaGeneraDeuda(t *testing.T) {
	nc := `<?xml version="1.0" encoding="UTF-8"?>
<NotaCreditoElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/notaCreditoElectronica">
  <Clave>` + claveAgosto05 + `</Clave>
  <FechaEmision>2026-08-05T10:00:00-06:00</FechaEmision>
  <Emisor><Nombre>COMAPAN S.A</Nombre><Identificacion><Numero>3101090247</Numero></Identificacion></Emisor>
  <ResumenFactura><CodigoTipoMoneda><CodigoMoneda>CRC</CodigoMoneda></CodigoTipoMoneda><TotalComprobante>500000.00</TotalComprobante></ResumenFactura>
</NotaCreditoElectronica>`
	factura := facturaV44(claveAgosto13, "01", "0", "CRC", "", "100.00", "13.00", "113.00")

	filas, traza, err := parsearFacturasXML([]byte(factura + "\n" + nc))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %d, quería 1: la nota de crédito NO puede volverse cuenta por pagar", len(filas))
	}
	if filas[0].TipoDocumento != TipoFacturaElectronica {
		t.Errorf("entró un %s como factura", filas[0].TipoDocumento)
	}
	if traza.Descartados["Nota de crédito"] != 1 {
		t.Errorf("descartados = %v: la nota de crédito tiene que quedar contada e informada, no perdida", traza.Descartados)
	}
	// Y de paso queda probado que se leen los DOS comprobantes del archivo: con xml.Unmarshal se
	// habría leído solo el primero, en silencio.
	if traza.FilasEnHoja != 2 {
		t.Errorf("filas_hoja = %d, quería 2 (los dos comprobantes del archivo)", traza.FilasEnHoja)
	}
}

// Hacienda emite algunos comprobantes en ISO-8859-1. Sin CharsetReader falla el documento ENTERO.
func TestParsearFacturasXMLLeeLatin1(t *testing.T) {
	// «PANADERÍA SEÑOR» en latin-1: la Í es 0xCD y la Ñ es 0xD1.
	xmlLatin1 := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n" +
		"<FacturaElectronica><Clave>" + claveAgosto13 + "</Clave>" +
		"<FechaEmision>2026-08-13T08:00:00-06:00</FechaEmision>" +
		"<Emisor><Nombre>PANADER\xcdA SE\xd1OR</Nombre><Identificacion><Numero>3101090247</Numero></Identificacion></Emisor>" +
		"<ResumenFactura><TotalVentaNeta>100.00</TotalVentaNeta><TotalImpuesto>13.00</TotalImpuesto><TotalComprobante>113.00</TotalComprobante></ResumenFactura>" +
		"</FacturaElectronica>")

	filas, _, err := parsearFacturasXML(xmlLatin1)
	if err != nil {
		t.Fatalf("un XML en ISO-8859-1 tiene que entrar: %v", err)
	}
	if filas[0].Proveedor != "PANADERÍA SEÑOR" {
		t.Errorf("proveedor = %q, quería «PANADERÍA SEÑOR»", filas[0].Proveedor)
	}
}

// LA CLAVE SE NORMALIZA O SE RECHAZA.
//
// El XML viene indentado y encoding/xml no recorta el contenido, así que la clave llega con
// espacios y saltos de línea. Sin normalizar pasan dos cosas: el UNIQUE (empresa_id, clave) es
// sobre el texto tal cual —« 506…» y «506…» serían DOS facturas— y `fechaDeClave` exige largo 50,
// así que con la clave sucia la fecha se caería al respaldo del XML.
func TestClaveNormalizada(t *testing.T) {
	casos := []struct{ entrada, quiero, porque string }{
		{claveAgosto13, claveAgosto13, "limpia, pasa igual"},
		{"\n   " + claveAgosto13 + "\n  ", claveAgosto13, "indentada en el XML: se recorta"},
		{" " + claveAgosto13 + " ", claveAgosto13, "con espacios al borde"},
		{"", "", "vacía"},
		{"506130826", "", "corta: no es una clave de Hacienda"},
		{claveAgosto13 + "0", "", "larga"},
		{claveAgosto13[:49] + "X", "", "con una letra no es una clave"},
		{claveAgosto13[:25] + " " + claveAgosto13[26:], "", "con un espacio en medio"},
	}
	for _, c := range casos {
		if got := claveNormalizada(c.entrada); got != c.quiero {
			t.Errorf("claveNormalizada(%q) = %q, quería %q — %s", c.entrada, got, c.quiero, c.porque)
		}
	}
}

// La clave indentada, de punta a punta: tiene que entrar UNA vez, limpia y con la fecha de la clave.
func TestParsearFacturasXMLRecortaLaClaveIndentada(t *testing.T) {
	x := `<FacturaElectronica>
  <Clave>
    ` + claveAgosto05 + `
  </Clave>
  <FechaEmision>2026-08-14T05:41:07Z</FechaEmision>
  <Emisor><Nombre>X</Nombre><Identificacion><Numero>3101090247</Numero></Identificacion></Emisor>
  <ResumenFactura><TotalVentaNeta>100.00</TotalVentaNeta><TotalImpuesto>13.00</TotalImpuesto><TotalComprobante>113.00</TotalComprobante></ResumenFactura>
</FacturaElectronica>`
	filas, _, err := parsearFacturasXML([]byte(x))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if filas[0].Clave != claveAgosto05 {
		t.Errorf("clave = %q: quedó sin recortar y duplicaría la factura", filas[0].Clave)
	}
	// El <FechaEmision> de este XML está en UTC y daría el 14 de agosto; la clave dice el 5, y la
	// clave manda. Es la misma trampa que reventó producción el 2026-09-09, por otra puerta.
	if filas[0].FechaEmision != "2026-08-05" {
		t.Errorf("fecha_emision = %q, quería 2026-08-05 (de la clave, no del UTC del XML)", filas[0].FechaEmision)
	}
}

// El vencimiento sale de la condición de venta + el plazo del XML, que es lo que declaró el emisor.
func TestCondicionYVencimiento(t *testing.T) {
	const emision = "2026-08-13"
	casos := []struct {
		nombre, codigo, plazo string
		wantCond, wantVence   string
	}{
		{"contado (01)", "01", "0", "Contado", emision},
		{"crédito (02) a 30 días", "02", "30", "Crédito", "2026-09-12"},
		{"crédito con IVA 90 días (10)", "10", "90", "Crédito", "2026-11-11"},
		{"plazo escrito con palabras", "02", "30 días", "Crédito", "2026-09-12"},
		{"crédito declarado sin plazo: no se inventa uno", "02", "0", "Crédito", ""},
		{"código desconocido cae en contado", "99", "0", "Contado", emision},
		{"consignación se trata como contado", "03", "0", "Contado", emision},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cond, vence := condicionYVencimiento(c.codigo, c.plazo, emision)
			if cond != c.wantCond || vence != c.wantVence {
				t.Errorf("= %s/%q, quería %s/%q", cond, vence, c.wantCond, c.wantVence)
			}
		})
	}
}

// LA FACTURA DE CONTADO NACE BLOQUEADA PARA PAGO.
//
// El caso real: se paga en la ferretería con caja chica, el custodio registra el vale, y la misma
// factura llega por correo. Sin el bloqueo se paga dos veces —al proveedor y al custodio en la
// reposición— y nada puede detectarlo, porque el vale no tiene la clave del comprobante.
func TestFilaAInputBloqueaElContadoDelXML(t *testing.T) {
	base := FilaImportada{
		Clave: claveAgosto13, FechaEmision: "2026-08-13", Moneda: "CRC",
		Subtotal: "100", IVA: "13", Total: "113",
	}

	contado := base
	contado.TipoDocumento = TipoFacturaElectronica
	contado.Condicion = "Contado"
	in, err := filaAInput(contado, "prov-1")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if !in.BloqueadoParaPago {
		t.Error("la factura de contado tiene que nacer bloqueada para pago: si ya se pagó con caja chica, se pagaría dos veces")
	}
	if !strings.Contains(in.BloqueoMotivo, "contado") {
		t.Errorf("el motivo del bloqueo dice %q y tiene que explicar por qué", in.BloqueoMotivo)
	}

	credito := base
	credito.TipoDocumento = TipoFacturaElectronica
	credito.Condicion = "Crédito"
	credito.Vencimiento = "2026-09-12"
	in, err = filaAInput(credito, "prov-1")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if in.BloqueadoParaPago {
		t.Error("la factura a crédito NO debe nacer bloqueada: es una cuenta por pagar normal")
	}

	// El camino del Excel no trae la condición declarada por el emisor, así que no bloquea nada:
	// su columna «Condición» solo se usa para derivar el plazo.
	excel := base
	excel.Condicion = "Contado"
	in, err = filaAInput(excel, "prov-1")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if in.BloqueadoParaPago {
		t.Error("el importador de Excel no debe cambiar de comportamiento por este cambio")
	}
}

// LA VERIFICACIÓN ARITMÉTICA es lo que convierte «cero en silencio» en algo visible.
//
// encoding/xml no tiene «campo requerido»: un elemento renombrado en una versión futura devuelve
// cadena vacía con err=nil, y decOrZero la vuelve 0 sin error. Sin esta verificación, un
// <TotalImpuesto> renombrado produciría facturas con IVA 0 que pasan todos los tests.
func TestDescuadreDe(t *testing.T) {
	casos := []struct {
		nombre          string
		r               resumenXML
		quieroDescuadre bool
		motivoContiene  string
	}{
		{
			nombre: "cuadra exacto",
			r:      resumenXML{TotalVentaNeta: "100.00", TotalImpuesto: "13.00", TotalComprobante: "113.00"},
		},
		{
			nombre: "cuadra con la tolerancia de un céntimo (5 decimales vs 2)",
			r:      resumenXML{TotalVentaNeta: "100.00500", TotalImpuesto: "13.00000", TotalComprobante: "113.00"},
		},
		{
			nombre: "con otros cargos e IVA devuelto",
			r: resumenXML{TotalVentaNeta: "100.00", TotalImpuesto: "13.00", TotalOtrosCargos: "5.00",
				TotalIVADevuelto: "2.00", TotalComprobante: "116.00"},
		},
		{
			// EL CASO QUE MOTIVA TODO ESTO: el elemento del impuesto no calzó y vino vacío.
			nombre:          "el impuesto vino vacío (elemento renombrado)",
			r:               resumenXML{TotalVentaNeta: "100.00", TotalImpuesto: "", TotalComprobante: "113.00"},
			quieroDescuadre: true,
			motivoContiene:  "no cuadra",
		},
		{
			nombre:          "sin total no hay factura",
			r:               resumenXML{TotalVentaNeta: "100.00", TotalImpuesto: "13.00"},
			quieroDescuadre: true,
			motivoContiene:  "Total Comprobante",
		},
		{
			nombre:          "un monto ilegible",
			r:               resumenXML{TotalVentaNeta: "cien", TotalImpuesto: "13.00", TotalComprobante: "113.00"},
			quieroDescuadre: true,
			motivoContiene:  "no se pudieron leer",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := descuadreDe(c.r)
			if c.quieroDescuadre && got == "" {
				t.Fatal("esperaba un descuadre y no lo hubo: un monto en cero pasaría en silencio")
			}
			if !c.quieroDescuadre && got != "" {
				t.Fatalf("no esperaba descuadre y dijo: %s", got)
			}
			if c.motivoContiene != "" && !strings.Contains(got, c.motivoContiene) {
				t.Errorf("el motivo dice %q y tiene que mencionar %q", got, c.motivoContiene)
			}
		})
	}
}

// El descuadre NO rechaza la fila: la marca y la cuenta. Rechazar exigiría que la fórmula fuera
// correcta para todas las formas de comprobante, y si estuviera mal trancaría gasto real.
func TestParsearFacturasXMLMarcaElDescuadreSinRechazar(t *testing.T) {
	// Total 999 contra neta 100 + impuesto 13: no cuadra por mucho.
	x := facturaV44(claveAgosto13, "02", "30", "CRC", "", "100.00", "13.00", "999.00")
	filas, traza, err := parsearFacturasXML([]byte(x))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("la fila tiene que entrar, marcada: filas = %d", len(filas))
	}
	if filas[0].Descuadre == "" {
		t.Error("la fila tiene que quedar marcada con el descuadre")
	}
	if traza.Descuadres != 1 {
		t.Errorf("descuadres = %d, quería 1: el informe tiene que poder decirlo", traza.Descuadres)
	}
	// Y el total que se le debe al proveedor sigue siendo el TotalComprobante, que es el campo que
	// el script de facturación viene usando en producción.
	if filas[0].Total != "999.00" {
		t.Errorf("total = %q, quería 999.00", filas[0].Total)
	}
}

// Un comprobante repetido en la misma entrega se cuenta una vez.
func TestParsearFacturasXMLDedupeDentroDeLaEntrega(t *testing.T) {
	x := facturaV44(claveAgosto13, "02", "30", "CRC", "", "100.00", "13.00", "113.00")
	filas, traza, err := parsearFacturasXML([]byte(x + "\n" + x))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(filas) != 1 || traza.RepetidasEnArchivo != 1 {
		t.Errorf("filas = %d, repetidas = %d; quería 1 y 1", len(filas), traza.RepetidasEnArchivo)
	}
}

// Un .zip de XML es la forma natural de entregar cientos de comprobantes de una vez.
func TestParsearEntregaLeeUnZipDeXML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, x := range []string{
		facturaV44(claveAgosto13, "02", "30", "CRC", "", "100.00", "13.00", "113.00"),
		facturaV44(claveAgosto05, "01", "0", "CRC", "", "50.00", "6.50", "56.50"),
	} {
		w, err := zw.Create([]string{"a.xml", "sub/b.xml"}[i])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(x)); err != nil {
			t.Fatal(err)
		}
	}
	// Un archivo que no es XML tiene que ignorarse sin romper nada.
	if w, err := zw.Create("leeme.txt"); err == nil {
		_, _ = w.Write([]byte("nada"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	filas, traza, err := parsearEntrega(buf.Bytes())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("filas = %d, quería 2", len(filas))
	}
	if !strings.Contains(traza.Hoja, "zip") {
		t.Errorf("la traza dice %q y tiene que decir que salió de un zip", traza.Hoja)
	}
}

// El despachador elige por la FORMA del archivo. Un .xlsx sigue yendo al parser de Excel: si esto
// se rompe, el importador que ya está en producción deja de funcionar.
func TestParsearEntregaNoSeLlevaElCaminoDelExcel(t *testing.T) {
	xlsx := construirXLSXDePrueba(t)
	if esXML(xlsx) {
		t.Fatal("un .xlsx no puede detectarse como XML: tiene [Content_Types].xml y va al otro parser")
	}
	filas, traza, err := parsearEntrega(xlsx)
	if err != nil {
		t.Fatalf("el .xlsx tiene que seguir entrando: %v", err)
	}
	if len(filas) != 1 || traza.Hoja == "XML" {
		t.Errorf("filas = %d, hoja = %q: se fue por el camino equivocado", len(filas), traza.Hoja)
	}
}

// NINGÚN SLICE DEL RESUMEN PUEDE SALIR NIL.
//
// Un slice nil de Go se serializa como `null`, y el frontend hace `resumen.hojas.length`: eso es
// justo lo que tiró la pantalla de saldos diarios en producción el 8 de setiembre de 2026. Solo se
// ve cuando el campo no trae datos, así que un test es la única forma de que no vuelva.
func TestResumenXMLNoDevuelveSlicesNil(t *testing.T) {
	_, traza, err := parsearFacturasXML([]byte(
		facturaV44(claveAgosto13, "02", "30", "CRC", "", "100.00", "13.00", "113.00")))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if traza.Hojas == nil {
		t.Error("Hojas es nil: sale como null en JSON y el frontend revienta al pedirle .length")
	}
	if traza.Descartados == nil {
		t.Error("Descartados es nil: sale como null y Object.entries(null) revienta")
	}

	// Y el JSON de verdad: es lo único que prueba lo que ve el navegador.
	b, err := json.Marshal(traza)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(b, []byte(":null")) {
		t.Errorf("el resumen serializa un null: %s", b)
	}
}

// Un archivo que no es ni XML ni Excel tiene que fallar con el error de formato, no con un panic.
func TestParsearEntregaRechazaLoQueNoEs(t *testing.T) {
	casos := []struct {
		nombre string
		data   []byte
		want   error
	}{
		{"vacío", nil, ErrArchivoVacio},
		{"texto suelto", []byte("hola"), ErrFormatoImportacion},
		{"XML que no es comprobante", []byte(`<html><body>404</body></html>`), ErrSinComprobantes},
		{"XML truncado", []byte(`<FacturaElectronica><Clave>506`), ErrFormatoImportacion},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, _, err := parsearEntrega(c.data)
			if err == nil {
				t.Fatal("esperaba un error")
			}
		})
	}
}
