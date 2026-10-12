package cxp

// Detalle de factura: traer las líneas del XML a una tabla para poder analizarlas.
//
// EL PROBLEMA. Las facturas del sistema entraron por el importador de Excel y solo traen el
// encabezado. El XML de Hacienda trae, línea por línea, el CABYS, la descripción, la cantidad, la
// unidad y el precio unitario. El parser del repo ya los lee; faltaba guardarlos.
//
// CÓMO SE EMPAREJA. Por la CLAVE de 50 dígitos, que viene dentro del XML y que ya está en las
// facturas cargadas (`documento_cxp.clave`, con índice único por empresa). NO por el nombre del
// archivo: Hacienda lo nombra con la clave, pero un correo reenviado, un ZIP de un proveedor o una
// descarga del portal lo renombran, y entonces la carga dejaría afuera facturas que sí calzaban.
// Leer la clave de adentro funciona se llame como se llame el archivo.

import (
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

// LineaDetalle es una línea de factura lista para guardar.
//
// Los montos son PUNTEROS a propósito: nil significa que el comprobante NO TRAJO el elemento, y
// eso es distinto de que lo trajera en cero. Un precio unitario ausente no entra en ningún
// promedio; uno de cero sí, porque es un dato que el emisor declaró.
type LineaDetalle struct {
	Linea           int
	NumeroLinea     string
	CABYS           string
	CodigoComercial string
	Detalle         string
	DetalleNorm     string
	UnidadMedida    string

	Cantidad        *decimal.Decimal
	PrecioUnitario  *decimal.Decimal
	MontoTotal      *decimal.Decimal
	Descuento       *decimal.Decimal
	SubTotal        *decimal.Decimal
	Impuesto        *decimal.Decimal
	MontoTotalLinea *decimal.Decimal
}

// DetalleDeComprobante es lo que se extrae de UN archivo XML.
type DetalleDeComprobante struct {
	Clave  string
	Moneda string
	Lineas []LineaDetalle
}

// DocumentoBreve es lo mínimo para decir a qué factura fue a dar un XML.
type DocumentoBreve struct {
	ID          string `json:"id"`
	Consecutivo string `json:"consecutivo"`
	Moneda      string `json:"moneda"`
	Proveedor   string `json:"proveedor"`
	// Lineas son las que la factura YA tenía: >0 significa que esta carga reemplaza un detalle
	// anterior, y eso se le dice a quien carga en vez de hacerlo callando.
	Lineas int `json:"lineas"`
}

// LineaGuardada es una línea ya persistida, lista para pintar.
//
// Los montos viajan como STRING, igual que en el resto de CxP: `numeric` en la base, decimal en
// Go, string en el JSON. Cadena vacía significa que el comprobante no trajo ese elemento, que es
// distinto de "0".
type LineaGuardada struct {
	Linea           int    `json:"linea"`
	NumeroLinea     string `json:"numero_linea"`
	CABYS           string `json:"cabys"`
	CodigoComercial string `json:"codigo_comercial"`
	Detalle         string `json:"detalle"`
	UnidadMedida    string `json:"unidad_medida"`
	Cantidad        string `json:"cantidad"`
	PrecioUnitario  string `json:"precio_unitario"`
	Descuento       string `json:"descuento"`
	Impuesto        string `json:"impuesto"`
	Total           string `json:"total"`
}

// numeroOpcional convierte el texto del XML en un decimal, o nil si no vino.
//
// Un valor ilegible se trata como ausente, no como cero: el documento lo declaró mal y lo que se
// sabe de él es nada. Cero sería una afirmación que el comprobante nunca hizo.
func numeroOpcional(s string) *decimal.Decimal {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil
	}
	return &d
}

// normalizarDetalle lleva la descripción a una forma comparable entre proveedores.
//
// Minúsculas, sin tildes, y todo lo que no sea letra o dígito pasa a ser un separador. Así
// "TORNILLO 1/2" y "Tornillo 1/2" caen en el mismo grupo.
//
// NO intenta ser un normalizador de catálogo: no corrige plurales, no expande abreviaturas y no
// adivina sinónimos. Juntar "CAJA" con "CAJAS" parece una mejora hasta que junta "CAJA" (de
// cartón) con "CAJAS" (de madera) de dos proveedores distintos y el precio promedio deja de
// significar algo. La agrupación fuerte la da el CABYS, que es la que el emisor declaró ante
// Hacienda; esto es solo para distinguir ítems DENTRO de un mismo CABYS.
func normalizarDetalle(s string) string {
	var b strings.Builder
	espacioPendiente := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		r = sinTilde(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if espacioPendiente && b.Len() > 0 {
				b.WriteRune(' ')
			}
			espacioPendiente = false
			b.WriteRune(r)
			continue
		}
		espacioPendiente = true
	}
	return b.String()
}

// sinTilde quita el diacrítico de las vocales acentuadas del español y convierte la eñe.
//
// Se hace a mano y no con una tabla Unicode completa porque el universo es el español de facturas
// de Costa Rica. La eñe cae a "n" para que "AÑO" y "ANO" —que en una descripción escrita a mano
// son la misma palabra mal tecleada— no abran dos grupos.
func sinTilde(r rune) rune {
	switch r {
	case 'á', 'à', 'ä', 'â':
		return 'a'
	case 'é', 'è', 'ë', 'ê':
		return 'e'
	case 'í', 'ì', 'ï', 'î':
		return 'i'
	case 'ó', 'ò', 'ö', 'ô':
		return 'o'
	case 'ú', 'ù', 'ü', 'û':
		return 'u'
	case 'ñ':
		return 'n'
	}
	return r
}

// DetalleDeXML extrae de un comprobante ya parseado lo que va a la tabla de líneas.
//
// `Linea` es el ORDINAL de aparición y no lo que el XML numere: es lo que sostiene la clave única,
// y un emisor que numera repetido o saltado no puede impedir que su factura se cargue. Lo que el
// documento declaró queda igual en `NumeroLinea`, para que se vea tal cual si alguien lo revisa.
func DetalleDeXML(c comprobanteXML) DetalleDeComprobante {
	// La moneda sale del helper que ya resuelve las DOS formas del nodo (v4.2 contra v4.3/4.4) y
	// el resumen correcto según el tipo de comprobante. Leer el campo a mano se vería bien con la
	// factura de hoy y devolvería vacío con la de la otra versión.
	moneda, _ := c.resumen().moneda()
	d := DetalleDeComprobante{
		Clave:  strings.TrimSpace(c.Clave),
		Moneda: moneda,
		Lineas: make([]LineaDetalle, 0, len(c.Lineas)),
	}
	for i, l := range c.Lineas {
		det := strings.TrimSpace(l.Detalle)
		ld := LineaDetalle{
			Linea:           i + 1,
			NumeroLinea:     strings.TrimSpace(l.NumeroLinea),
			CABYS:           strings.TrimSpace(l.CodigoCABYS),
			CodigoComercial: strings.TrimSpace(l.CodigoComercial.Codigo),
			Detalle:         det,
			DetalleNorm:     normalizarDetalle(det),
			UnidadMedida:    strings.TrimSpace(l.UnidadMedida),

			Cantidad:        numeroOpcional(l.Cantidad),
			PrecioUnitario:  numeroOpcional(l.PrecioUnitario),
			MontoTotal:      numeroOpcional(l.MontoTotal),
			SubTotal:        numeroOpcional(l.SubTotal),
			Impuesto:        numeroOpcional(l.ImpuestoNeto),
			MontoTotalLinea: numeroOpcional(l.MontoTotalLinea),
		}
		// El descuento es repetible y se SUMA. Guardar solo el primero dejaría una línea con tres
		// descuentos declarando menos rebaja de la que tuvo, y el precio unitario efectivo que se
		// calcule después saldría alto sin que nada lo delate.
		if len(l.Descuentos) > 0 {
			suma := decimal.Zero
			hubo := false
			for _, dd := range l.Descuentos {
				if v := numeroOpcional(dd.MontoDescuento); v != nil {
					suma = suma.Add(*v)
					hubo = true
				}
			}
			if hubo {
				ld.Descuento = &suma
			}
		}
		d.Lineas = append(d.Lineas, ld)
	}
	return d
}
