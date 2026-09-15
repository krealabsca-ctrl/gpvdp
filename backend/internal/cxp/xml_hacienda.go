package cxp

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// centimo es la tolerancia de la verificación aritmética: los comprobantes traen los montos con
// 5 decimales y el total con 2, así que la diferencia por redondeo es legítima.
var centimo = decimal.New(1, -2)

// parsearEntrega elige el parser según la FORMA del archivo, no según su nombre.
//
// El nombre miente: el navegador manda lo que el usuario le puso, y el mismo contenido puede llegar
// como «facturas.xls», «facturas.txt» o sin extensión. La forma no miente:
//
//   - un .xlsx es un zip que trae `[Content_Types].xml` (marca de Office Open XML);
//   - un zip SIN esa marca pero con .xml adentro es un paquete de comprobantes;
//   - un archivo que empieza con `<?xml` o `<` es un comprobante (o varios concatenados).
func parsearEntrega(data []byte) ([]FilaImportada, ResumenImportacion, error) {
	if len(data) == 0 {
		return nil, ResumenImportacion{}, ErrArchivoVacio
	}
	if esXML(data) {
		return parsearFacturasXML(data)
	}
	return parsearFacturas(data)
}

// ── EL PARSER DE COMPROBANTE ELECTRÓNICO DE HACIENDA CR (v4.2 / 4.3 / 4.4) ──────────────────
//
// Hermano de `parsearFacturas` (el del .xlsx) y con el MISMO contrato, para que todo lo que viene
// después —dedupe por clave, alta de proveedor, `filaAInput`, `CrearDocumento`, la acumulación de
// errores fila por fila— sea exactamente el mismo código y no se reimplemente nada.
//
// Cada regla de acá corresponde a un modo de falla que se probó ejecutando código. Están explicadas
// una por una porque varias son silenciosas, y una lectura futura que las "simplifique" reintroduce
// un error que no se ve.

// Los 7 tipos que emite Hacienda. El tipo sale de la RAÍZ del documento (`XMLName.Local`).
//
// Solo la factura electrónica se vuelve cuenta por pagar (decisión del Director Financiero,
// 2026-09-09). Los otros se leen, se cuentan y se informan, pero NO generan deuda:
//
//   - la nota de crédito RESTA: entraría sumando;
//   - el recibo electrónico de pago documenta que YA se pagó;
//   - en la factura de compra los papeles están invertidos (la empresa emite), así que del nodo
//     Emisor se daría de alta un «proveedor» que es la propia empresa;
//   - el tiquete normalmente no trae nodo Receptor, así que no hay contra qué cotejar la empresa.
const (
	TipoFacturaElectronica            = "FacturaElectronica"
	TipoNotaCreditoElectronica        = "NotaCreditoElectronica"
	TipoNotaDebitoElectronica         = "NotaDebitoElectronica"
	TipoTiqueteElectronico            = "TiqueteElectronico"
	TipoFacturaElectronicaCompra      = "FacturaElectronicaCompra"
	TipoFacturaElectronicaExportacion = "FacturaElectronicaExportacion"
	TipoReciboElectronicoPago         = "ReciboElectronicoPago"
)

// nombreDeTipo traduce la raíz a algo legible para el usuario en el informe de lectura.
var nombreDeTipo = map[string]string{
	TipoFacturaElectronica:            "Factura electrónica",
	TipoNotaCreditoElectronica:        "Nota de crédito",
	TipoNotaDebitoElectronica:         "Nota de débito",
	TipoTiqueteElectronico:            "Tiquete electrónico",
	TipoFacturaElectronicaCompra:      "Factura electrónica de compra",
	TipoFacturaElectronicaExportacion: "Factura de exportación",
	TipoReciboElectronicoPago:         "Recibo electrónico de pago",
}

// ErrSinComprobantes indica que el archivo se pudo leer pero no traía ningún comprobante.
var ErrSinComprobantes = errors.New("cxp: el archivo no trae comprobantes electrónicos")

// resumenXML son los totales del comprobante.
//
// La moneda cambia de FORMA entre versiones y hay que soportar las dos:
//   - v4.3/4.4: <CodigoTipoMoneda><CodigoMoneda>USD</CodigoMoneda><TipoCambio>512.35</TipoCambio></...>
//   - v4.2:     <CodigoMoneda>USD</CodigoMoneda>   (nodo simple, sin tipo de cambio)
//
// Los dos campos pueden convivir porque `CodigoMoneda` a secas solo calza con un hijo DIRECTO del
// resumen, y en 4.3/4.4 es nieto (vive dentro de CodigoTipoMoneda).
type resumenXML struct {
	CodigoTipoMoneda struct {
		CodigoMoneda string `xml:"CodigoMoneda"`
		TipoCambio   string `xml:"TipoCambio"`
	} `xml:"CodigoTipoMoneda"`
	MonedaSimple string `xml:"CodigoMoneda"`

	// Todos los montos se leen a STRING, nunca a decimal.Decimal ni a float64:
	//
	//   · decimal.Decimal implementa UnmarshalText, así que compila y funciona con un XML de una
	//     línea — y se cae con TODO el documento en cuanto llega uno con sangría, porque
	//     encoding/xml NO recorta el contenido: «\n   3390.00\n  » no convierte a decimal;
	//   · float64 funciona perfecto y los tests pasan, y está prohibido para dinero (CLAUDE.md).
	//
	// Se limpian con limpiarNumero y recién en `filaAInput` van a decimal.NewFromString.
	TotalVenta       string `xml:"TotalVenta"`
	TotalDescuentos  string `xml:"TotalDescuentos"`
	TotalVentaNeta   string `xml:"TotalVentaNeta"`
	TotalImpuesto    string `xml:"TotalImpuesto"`
	TotalOtrosCargos string `xml:"TotalOtrosCargos"`
	TotalIVADevuelto string `xml:"TotalIVADevuelto"`
	TotalComprobante string `xml:"TotalComprobante"`

	// ── SOLO PARA EL VISOR (14 de setiembre de 2026) ────────────────────────────────────────
	//
	// La composición de la venta: es lo que contesta «por qué esta factura paga menos IVA del que
	// parece». Se lee para mostrar, no para calcular: el importador sigue usando TotalComprobante.
	TotalServGravados       string `xml:"TotalServGravados"`
	TotalServExentos        string `xml:"TotalServExentos"`
	TotalServExonerado      string `xml:"TotalServExonerado"`
	TotalMercanciasGravadas string `xml:"TotalMercanciasGravadas"`
	TotalMercanciasExentas  string `xml:"TotalMercanciasExentas"`
	TotalMercExonerada      string `xml:"TotalMercExonerada"`
	TotalGravado            string `xml:"TotalGravado"`
	TotalExento             string `xml:"TotalExento"`
	TotalExonerado          string `xml:"TotalExonerado"`

	// EL DESGLOSE POR TARIFA. Es REPETIBLE, y ahí está la respuesta a los «varios IVAs»: una
	// factura con líneas al 13 % y al 1 % trae dos de estos.
	//
	// Medido: 414 de 3.455 facturas con IVA (12,0 %) tienen una tasa efectiva que no es ninguna
	// tarifa legal —12,9 %, 11,2 %, 9,3 %, 7,1 %…— porque el ERP aplasta la mezcla en un solo
	// campo `iva`. Sin este desglose esas facturas son inexplicables.
	Desglose []desgloseImpuestoXML `xml:"TotalDesgloseImpuesto"`

	// MedioPago es compuesto en 4.4 y repetible (una factura puede declarar pago mixto). Leerlo
	// como string devuelve cadena vacía SIN error — la misma trampa que la moneda compuesta.
	MediosPago []struct {
		TipoMedioPago  string `xml:"TipoMedioPago"`
		TotalMedioPago string `xml:"TotalMedioPago"`
	} `xml:"MedioPago"`
}

// desgloseImpuestoXML es una fila del desglose de impuestos del comprobante.
type desgloseImpuestoXML struct {
	Codigo             string `xml:"Codigo"`
	CodigoTarifaIVA    string `xml:"CodigoTarifaIVA"`
	CodigoTarifa       string `xml:"CodigoTarifa"`
	TotalMontoImpuesto string `xml:"TotalMontoImpuesto"`
}

// codigoTarifa devuelve el código con el nombre que traiga el documento (ver impuestoXML).
func (d desgloseImpuestoXML) codigoTarifa() string {
	if c := strings.TrimSpace(d.CodigoTarifaIVA); c != "" {
		return c
	}
	return strings.TrimSpace(d.CodigoTarifa)
}

// comprobanteXML sirve para las tres versiones del esquema y para los 7 tipos de documento.
//
// ── NINGÚN TAG LLEVA URI DE NAMESPACE. NUNCA. ──
//
// Sin namespace, encoding/xml calza por NOMBRE LOCAL e ignora el namespace: el mismo struct lee un
// documento v4.2, v4.3 o v4.4, con `xmlns` por defecto, con prefijo (`<fe:Clave>`) o sin `xmlns`
// del todo. Escribir el namespace es el error, no la solución: un tag con la URI de la v4.3 contra
// un documento v4.4 da error DURO, o sea que la ingesta reventaría el día que Hacienda suba de
// versión. Y repetir el namespace en cada segmento de una ruta `A>B` es peor todavía: devuelve
// cadena vacía SIN error, o sea un total en cero sin una sola queja.
//
// `XMLName` sí lleva el nombre del elemento, pero se rellena con lo que traiga el documento porque
// se decodifica contra `sobreXML` (ver `leerComprobantes`). Un struct SIN XMLName acepta cualquier
// raíz, y con eso una NOTA DE CRÉDITO se registraría como factura por pagar, sumando en vez de
// restar.
type comprobanteXML struct {
	XMLName           xml.Name
	Clave             string `xml:"Clave"`
	NumeroConsecutivo string `xml:"NumeroConsecutivo"`
	FechaEmision      string `xml:"FechaEmision"`
	CondicionVenta    string `xml:"CondicionVenta"`
	PlazoCredito      string `xml:"PlazoCredito"`

	Emisor struct {
		Nombre          string `xml:"Nombre"`
		NombreComercial string `xml:"NombreComercial"`
		Identificacion  struct {
			Tipo   string `xml:"Tipo"`
			Numero string `xml:"Numero"`
		} `xml:"Identificacion"`
	} `xml:"Emisor"`

	// El Receptor dice A QUIÉN se le facturó. Es el único dato del comprobante que puede decir de
	// qué empresa es la factura, y el script de hoy nunca lo mira: de ahí que haga falta un buzón
	// por empresa y que la empresa quede determinada por dónde llegó el correo.
	Receptor struct {
		Nombre         string `xml:"Nombre"`
		Identificacion struct {
			Numero string `xml:"Numero"`
		} `xml:"Identificacion"`
	} `xml:"Receptor"`

	ResumenFactura     resumenXML `xml:"ResumenFactura"`
	ResumenComprobante resumenXML `xml:"ResumenComprobante"`

	// ── LO QUE SE LEE SOLO PARA EL VISOR (14 de setiembre de 2026) ──────────────────────────
	//
	// El detalle NO participa de la creación de la cuenta por pagar: esos números salen del
	// resumen, igual que siempre. Se lee aparte para poder MOSTRAR la factura, porque hoy no hay
	// dónde mirar qué se compró: `documento_cxp.descripcion` está cortada a 45 caracteres en
	// 4.526 de 4.542 filas (99,6 %) y no existe ninguna tabla de líneas.
	//
	// Todo lo repetible va en slice. Declarar `LineaDetalle` como campo escalar no da error:
	// encoding/xml se queda con la ÚLTIMA y devuelve err=nil, así que dos líneas de 1.000 y 2.000
	// mostrarían «2000», que parece un total válido. Y los `Impuesto` anidados bajo un padre
	// colapsado se ACUMULAN, mezclando los de una línea con los de otra.
	Lineas []lineaXML `xml:"DetalleServicio>LineaDetalle"`
}

// lineaXML es una línea del detalle. Todos los montos son string por la misma razón que el resto
// del archivo: `decimal.Decimal` como tipo de campo XML NO falla siempre —pasa limpio cuando el
// valor viene pegado a las etiquetas, como en el XML real— y revienta el documento ENTERO el día
// que un intermediario reformatee y deje el número en su propia línea. Un bug así no lo detecta
// ninguna prueba hecha con la factura de hoy.
type lineaXML struct {
	NumeroLinea     string `xml:"NumeroLinea"`
	CodigoCABYS     string `xml:"CodigoCABYS"`
	CodigoComercial struct {
		Tipo   string `xml:"Tipo"`
		Codigo string `xml:"Codigo"`
	} `xml:"CodigoComercial"`
	Cantidad       string `xml:"Cantidad"`
	UnidadMedida   string `xml:"UnidadMedida"`
	Detalle        string `xml:"Detalle"`
	PrecioUnitario string `xml:"PrecioUnitario"`
	MontoTotal     string `xml:"MontoTotal"`
	// Descuento es repetible: una línea puede traer varios con naturalezas distintas.
	Descuentos []struct {
		MontoDescuento      string `xml:"MontoDescuento"`
		CodigoDescuento     string `xml:"CodigoDescuento"`
		NaturalezaDescuento string `xml:"NaturalezaDescuento"`
	} `xml:"Descuento"`
	SubTotal      string `xml:"SubTotal"`
	BaseImponible string `xml:"BaseImponible"`
	// Impuesto es repetible: es la forma en que una sola línea lleva más de una tarifa.
	Impuestos       []impuestoXML `xml:"Impuesto"`
	ImpuestoNeto    string        `xml:"ImpuestoNeto"`
	MontoTotalLinea string        `xml:"MontoTotalLinea"`
}

// impuestoXML es un impuesto de una línea.
//
// `Tarifa` es el PORCENTAJE y viene en el propio documento (13). `CodigoTarifaIVA` es el código de
// catálogo (08) y se muestra crudo: no hay tabla oficial en el repo para traducirlo, y una
// etiqueta equivocada se cree mientras que un código crudo se puede buscar.
type impuestoXML struct {
	Codigo          string `xml:"Codigo"`
	CodigoTarifaIVA string `xml:"CodigoTarifaIVA"`
	// CodigoTarifa sin «IVA» es como se llamaba en esquemas anteriores. Se leen los dos porque el
	// visor parsea HACIA ATRÁS: si solo se leyera el de 4.4, la tarifa saldría vacía en silencio
	// justo en los comprobantes viejos, que son buena parte del público de esta pantalla.
	CodigoTarifa string `xml:"CodigoTarifa"`
	Tarifa       string `xml:"Tarifa"`
	Monto        string `xml:"Monto"`
}

// codigoTarifa devuelve el código de tarifa con el nombre que traiga el documento.
func (i impuestoXML) codigoTarifa() string {
	if c := strings.TrimSpace(i.CodigoTarifaIVA); c != "" {
		return c
	}
	return strings.TrimSpace(i.CodigoTarifa)
}

// resumen devuelve el que el documento traiga: la factura usa ResumenFactura y algunos tipos
// (el tiquete, por ejemplo) usan ResumenComprobante.
func (c comprobanteXML) resumen() resumenXML {
	r := c.ResumenFactura
	if strings.TrimSpace(r.TotalComprobante) == "" &&
		strings.TrimSpace(r.MonedaSimple) == "" &&
		strings.TrimSpace(r.CodigoTipoMoneda.CodigoMoneda) == "" {
		return c.ResumenComprobante
	}
	return r
}

// moneda devuelve el código y el tipo de cambio DE LA FACTURA, resolviendo las dos formas del nodo.
func (r resumenXML) moneda() (codigo, tc string) {
	if m := strings.TrimSpace(r.CodigoTipoMoneda.CodigoMoneda); m != "" {
		return strings.ToUpper(m), limpiarNumero(r.CodigoTipoMoneda.TipoCambio)
	}
	if m := strings.TrimSpace(r.MonedaSimple); m != "" {
		return strings.ToUpper(m), ""
	}
	return "CRC", ""
}

// versionDeEsquema saca «v4.4» de la URI del namespace; "" si no se reconoce.
//
// La URI es la única forma de saber contra qué versión se está leyendo, y sirve para que el informe
// pueda decir «se leyeron 3 comprobantes v4.3 y 1 v4.4» — y para que el día que aparezca una v4.5
// se vea en pantalla en vez de descubrirlo por un total en cero.
func versionDeEsquema(space string) string {
	i := strings.Index(space, "/v4.")
	if i < 0 {
		return ""
	}
	resto := space[i+1:]
	if j := strings.IndexByte(resto, '/'); j > 0 {
		return resto[:j]
	}
	return resto
}

// claveNormalizada exige la clave de 50 dígitos de Hacienda; "" si no lo es.
//
// El XML viene indentado, y encoding/xml NO recorta el contenido, así que la clave llega como
// «\n  506130826…\n  ». Sin recortar pasan dos cosas en cadena: el UNIQUE (empresa_id, clave) es
// sobre el texto tal cual —o sea que « 506…» y «506…» son DOS filas y las dos entran, duplicando la
// factura—, y `fechaDeClave` exige largo 50 exacto, así que con la clave sucia devuelve "" y la
// fecha se cae al respaldo <FechaEmision>, que en UTC da el día equivocado.
func claveNormalizada(s string) string {
	s = strings.TrimSpace(s)
	if len(s) != 50 {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return s
}

// condicionYVencimiento traduce el código de condición de venta y calcula el vencimiento.
//
// El XML de Hacienda NO trae fecha de vencimiento: trae el código de condición y el plazo de
// crédito en días. Manda lo que el proveedor declaró en el comprobante (decisión del Director
// Financiero, 2026-09-09): vencimiento = emisión + PlazoCredito.
//
// Códigos de crédito del catálogo de Hacienda: «02» crédito y «10» venta a crédito con IVA a 90
// días. Todo lo demás se trata como contado.
func condicionYVencimiento(codigo, plazoTexto, emisionISO string) (condicion, vencimiento string) {
	cod := strings.TrimSpace(codigo)
	esCredito := cod == "02" || cod == "10"
	if !esCredito {
		// Contado: el vencimiento es el día de la emisión. La factura de contado además nace
		// bloqueada para pago (ver `filaAInput`), así que no sale al banco por aparecer «vencida».
		return "Contado", emisionISO
	}
	dias := diasDePlazo(plazoTexto)
	if dias <= 0 || emisionISO == "" {
		// Crédito declarado sin plazo utilizable: se deja sin vencimiento en vez de inventar uno.
		// `condicionDeFila` interpreta eso como plazo 0, que es lo mismo que hace el importador de
		// Excel cuando la columna de vencimiento viene vacía.
		return "Crédito", ""
	}
	t, err := time.Parse("2006-01-02", emisionISO)
	if err != nil {
		return "Crédito", ""
	}
	return "Crédito", t.AddDate(0, 0, dias).Format("2006-01-02")
}

// diasDePlazo saca el número de un plazo escrito como «30» o «30 días»; 0 si no hay ninguno.
// Reusa `soloDigitos` (handler_conciliacion.go), que ya extrae los dígitos de un texto.
func diasDePlazo(s string) int {
	d := soloDigitos(s)
	if d == "" {
		return 0
	}
	n, err := strconv.Atoi(d)
	if err != nil {
		return 0
	}
	return n
}

// charsetReaderLatin1 permite leer los comprobantes que Hacienda emite en ISO-8859-1.
//
// Sin esto, un XML declarado `encoding="ISO-8859-1"` falla ENTERO con «encoding "ISO-8859-1"
// declared but Decoder.CharsetReader is nil», y el proveedor cuyo nombre lleva una Ñ o una tilde
// termina en la cola de errores sin que nadie entienda por qué.
func charsetReaderLatin1(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "iso-8859-1", "iso8859-1", "latin1", "windows-1252":
		b, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		// En latin-1 cada byte ES un code point, así que la conversión a UTF-8 es directa.
		out := make([]rune, 0, len(b))
		for _, x := range b {
			out = append(out, rune(x))
		}
		return strings.NewReader(string(out)), nil
	case "", "utf-8", "utf8":
		return input, nil
	}
	return nil, fmt.Errorf("cxp: codificación de XML no soportada: %s", charset)
}

// leerComprobantes decodifica todos los comprobantes de un archivo XML.
//
// Se usa `xml.NewDecoder` en BUCLE y no `xml.Unmarshal`, por dos razones verificadas: `Unmarshal`
// lee SOLO EL PRIMER comprobante de un archivo con varios, en silencio y sin error; y no admite
// `CharsetReader`, así que revienta con cualquier encoding que no sea UTF-8.
func leerComprobantes(data []byte) ([]comprobanteXML, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReaderLatin1
	var out []comprobanteXML
	for {
		var c comprobanteXML
		err := dec.Decode(&c)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Si ya se leyó algo, se devuelve lo bueno: un archivo con un comprobante corrupto al
			// final no debe tirar a la basura los que sí se entendieron.
			if len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		if c.XMLName.Local != "" {
			out = append(out, c)
		}
	}
	return out, nil
}

// xmlsDeZip saca los .xml de un zip que NO sea un .xlsx.
//
// Un .xlsx también es un zip, así que se distingue por su marca: todo Office Open XML trae
// `[Content_Types].xml` en la raíz. Sin esa marca, el zip se trata como un paquete de comprobantes
// —que es la forma natural de entregar 800 archivos de una vez—.
func xmlsDeZip(data []byte) ([][]byte, bool) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, false
	}
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" {
			return nil, false // es un .xlsx: lo lee el otro parser
		}
	}
	var out [][]byte
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.EqualFold(strings.TrimSpace(pathExt(f.Name)), ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		// Tope por archivo: el zip lo sube un tercero y un miembro puede declarar un tamaño
		// enorme. 4 MB es holgado para un comprobante (los reales pesan entre 5 y 60 KB).
		b, err := io.ReadAll(io.LimitReader(rc, 4<<20))
		_ = rc.Close()
		if err == nil && len(b) > 0 {
			out = append(out, b)
		}
	}
	return out, len(out) > 0
}

// pathExt devuelve la extensión en minúsculas (sin depender de path/filepath, que normaliza
// separadores distinto según el sistema y acá los nombres vienen del zip, no del disco).
func pathExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return strings.ToLower(name[i:])
	}
	return ""
}

// esXML dice si el archivo parece un XML (o un zip de XML), para elegir parser.
func esXML(data []byte) bool {
	// El BOM lo parsea bien encoding/xml, pero acá estorba para mirar el inicio del archivo, así
	// que se descarta a nivel de BYTES (0xEF 0xBB 0xBF) y no con un literal invisible en el código.
	cabeza := bytes.TrimPrefix(data[:min(len(data), 512)], []byte{0xEF, 0xBB, 0xBF})
	s := strings.TrimSpace(string(cabeza))
	if strings.HasPrefix(s, "<?xml") || strings.HasPrefix(s, "<") {
		return true
	}
	if strings.HasPrefix(s, "PK") {
		_, ok := xmlsDeZip(data)
		return ok
	}
	return false
}

// parsearFacturasXML lee comprobantes electrónicos y los devuelve con el MISMO contrato que
// `parsearFacturas` (el del .xlsx), para que el resto del importador sea el mismo código.
//
// Acepta un XML con un comprobante, un XML con varios concatenados, o un .zip de XML.
func parsearFacturasXML(data []byte) ([]FilaImportada, ResumenImportacion, error) {
	var traza ResumenImportacion
	traza.Descartados = map[string]int{}
	// Nunca nil: un slice nil de Go se serializa como `null` y `.length` revienta en el frontend.
	// Es la misma trampa que tiró `/saldos-diarios` en producción el 8 de setiembre de 2026, y solo
	// se ve cuando no hay datos —o, como acá, cuando el campo no aplica a este formato—.
	traza.Hojas = []string{}
	traza.Versiones = []string{}

	archivos := [][]byte{data}
	if xs, ok := xmlsDeZip(data); ok {
		archivos = xs
		traza.Hoja = fmt.Sprintf("%d archivo(s) XML del zip", len(xs))
	} else {
		traza.Hoja = "XML"
	}

	var comprobantes []comprobanteXML
	var ilegibles int
	for _, a := range archivos {
		cs, err := leerComprobantes(a)
		if err != nil {
			ilegibles++
			continue
		}
		comprobantes = append(comprobantes, cs...)
	}
	traza.FilasEnHoja = len(comprobantes) + ilegibles
	traza.XMLIlegibles = ilegibles
	if len(comprobantes) == 0 {
		if ilegibles > 0 {
			return nil, traza, ErrFormatoImportacion
		}
		return nil, traza, ErrSinComprobantes
	}

	vistas := map[string]bool{}
	out := make([]FilaImportada, 0, len(comprobantes))
	for _, c := range comprobantes {
		tipo := c.XMLName.Local
		if v := versionDeEsquema(c.XMLName.Space); v != "" && !contiene(traza.Versiones, v) {
			traza.Versiones = append(traza.Versiones, v)
		}

		// Lista blanca por la RAÍZ del documento. Todo lo que no sea factura electrónica se cuenta
		// y se informa, pero no genera deuda.
		if tipo != TipoFacturaElectronica {
			etiqueta := nombreDeTipo[tipo]
			if etiqueta == "" {
				etiqueta = tipo
			}
			traza.Descartados[etiqueta]++
			continue
		}

		m := mapearComprobante(c)
		if m.ClaveInvalida {
			traza.SinClave++
			continue
		}
		if vistas[m.Fila.Clave] {
			traza.RepetidasEnArchivo++
			continue
		}
		vistas[m.Fila.Clave] = true

		if m.FechaCorregida {
			traza.FechaCorregida++
		}
		if m.Fila.FechaEmision == "" {
			traza.SinFecha++
		}
		if m.Fila.Descuadre != "" {
			traza.Descuadres++
		}
		if m.Fila.Receptor == "" {
			traza.SinReceptor++
		}
		out = append(out, m.Fila)
	}

	if len(out) == 0 {
		return nil, traza, ErrSinComprobantes
	}
	return out, traza, nil
}

// mapeoComprobante es el resultado de interpretar UN comprobante electrónico.
type mapeoComprobante struct {
	Fila FilaImportada
	// FechaCorregida: la columna del XML no coincidía con la fecha que trae la clave.
	FechaCorregida bool
	// ClaveInvalida: la clave no es la de Hacienda (50 dígitos), así que la fila no sirve.
	ClaveInvalida bool
}

// mapearComprobante traduce un comprobante ya decodificado a la fila del importador.
//
// Vive aparte del bucle de `parsearFacturasXML` porque la recepción por correo necesita el MISMO
// mapeo para un solo comprobante, y duplicarlo garantizaría que las dos puertas se separen con el
// tiempo. El bucle se queda con la contabilidad de la traza; acá está solo la traducción.
func mapearComprobante(c comprobanteXML) mapeoComprobante {
	clave := claveNormalizada(c.Clave)
	if clave == "" {
		return mapeoComprobante{ClaveInvalida: true}
	}
	res := c.resumen()
	moneda, tc := res.moneda()
	// La fecha sale de la CLAVE (posiciones 4-9, `ddmmaa`), que no admite dos lecturas ni
	// ambigüedad de zona horaria. El <FechaEmision> es el respaldo: viene con offset y, si alguien
	// lo sellara en UTC, «2026-08-14T05:41:07Z» daría el 14 cuando en Costa Rica son las 23:41
	// del 13.
	emision, corregida := fechaDeEmision(clave, c.FechaEmision)
	condicion, vencimiento := condicionYVencimiento(c.CondicionVenta, c.PlazoCredito, emision)

	return mapeoComprobante{
		FechaCorregida: corregida,
		Fila: FilaImportada{
			Clave:          clave,
			Consecutivo:    strings.TrimSpace(c.NumeroConsecutivo),
			FechaEmision:   emision,
			Proveedor:      nombreDelEmisor(c),
			Cedula:         strings.TrimSpace(c.Emisor.Identificacion.Numero),
			Moneda:         moneda,
			Subtotal:       limpiarNumero(subtotalDe(res)),
			IVA:            limpiarNumero(res.TotalImpuesto),
			Total:          limpiarNumero(res.TotalComprobante),
			Condicion:      condicion,
			Vencimiento:    vencimiento,
			TC:             tc,
			TipoDocumento:  c.XMLName.Local,
			Receptor:       strings.TrimSpace(c.Receptor.Identificacion.Numero),
			ReceptorNombre: strings.TrimSpace(c.Receptor.Nombre),
			Descuadre:      descuadreDe(res),
		},
	}
}

// subtotalDe elige la venta neta si viene, y si no la deriva de venta − descuentos.
//
// El subtotal del ERP es lo que se vendió antes de impuestos. En 4.3/4.4 eso es `TotalVentaNeta`;
// cuando no viene, se calcula. No se usa `TotalVenta` a secas porque incluiría el descuento.
func subtotalDe(r resumenXML) string {
	if s := strings.TrimSpace(r.TotalVentaNeta); s != "" {
		return s
	}
	venta, err1 := decOrZero(limpiarNumero(r.TotalVenta))
	desc, err2 := decOrZero(limpiarNumero(r.TotalDescuentos))
	if err1 != nil || err2 != nil {
		return strings.TrimSpace(r.TotalVenta)
	}
	return venta.Sub(desc).String()
}

// descuadreDe verifica que la aritmética del comprobante cierre; "" si cierra.
//
// ── POR QUÉ ESTA VERIFICACIÓN NO ES OPCIONAL ──
//
// `encoding/xml` no tiene equivalente de `DisallowUnknownFields` ni de «campo requerido»: un
// elemento renombrado entre versiones —o un typo en el struct— devuelve CERO, con err=nil y sin una
// sola queja. Y como `decOrZero` convierte "" en 0 sin error, un `<TotalImpuesto>` renombrado en
// una versión futura produciría facturas con IVA 0 **que pasan todos los tests**.
//
// La verificación NO rechaza la fila: la marca. Rechazar exigiría que esta fórmula fuera correcta
// para todas las formas de comprobante (otros cargos, IVA devuelto, exoneraciones), y si estuviera
// mal trancaría gasto real — el error opuesto y peor. `TotalComprobante`, en cambio, es el campo
// que el script de facturación viene usando en producción sobre miles de facturas: ese es el que
// manda para lo que se debe. Lo que hace falta es que un descuadre se VEA.
func descuadreDe(r resumenXML) string {
	total, err := decOrZero(limpiarNumero(r.TotalComprobante))
	if err != nil || total.IsZero() {
		return "el comprobante no trae Total Comprobante"
	}
	neta, e1 := decOrZero(limpiarNumero(subtotalDe(r)))
	imp, e2 := decOrZero(limpiarNumero(r.TotalImpuesto))
	cargos, e3 := decOrZero(limpiarNumero(r.TotalOtrosCargos))
	ivaDev, e4 := decOrZero(limpiarNumero(r.TotalIVADevuelto))
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return "hay montos que no se pudieron leer"
	}
	esperado := neta.Add(imp).Add(cargos).Sub(ivaDev)
	dif := esperado.Sub(total).Abs()
	// Un céntimo de tolerancia: los comprobantes traen 5 decimales y el total 2, así que la
	// diferencia por redondeo es legítima.
	if dif.GreaterThan(centimo) {
		return fmt.Sprintf("la aritmética no cuadra: neta %s + impuesto %s = %s, pero el total dice %s",
			neta.StringFixed(2), imp.StringFixed(2), esperado.StringFixed(2), total.StringFixed(2))
	}
	return ""
}

// nombreDelEmisor elige el nombre del proveedor con la información del comprobante.
//
// El catálogo del ERP manda siempre: este nombre se usa SOLO al dar de alta un proveedor que no
// existe (`resolverProveedor` no pisa el nombre de uno existente). Por eso acá no se replica la
// cascada de 6 niveles del Apps Script, que podía terminar escribiendo el DOMINIO DEL CORREO como
// razón social — y ese nombre viaja al archivo que va al banco.
func nombreDelEmisor(c comprobanteXML) string {
	if n := strings.TrimSpace(c.Emisor.Nombre); n != "" {
		return n
	}
	if n := strings.TrimSpace(c.Emisor.NombreComercial); n != "" {
		return n
	}
	return ""
}

func contiene(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
