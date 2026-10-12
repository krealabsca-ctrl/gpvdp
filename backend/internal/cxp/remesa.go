package cxp

// Remesa de pago: UN pago que cubre varias facturas del mismo proveedor dentro de un lote.
//
// El criterio se elige al cortar y queda en el lote (`modo_pago`), no se infiere: un corte por
// proveedor donde cada proveedor trajo una sola factura no produce agrupación visible y aun así se
// armó con ese criterio. La macro y el reporte tienen que saberlo para no cambiar de forma.

import (
	"errors"
	"fmt"
)

var (
	// ErrModoPagoInvalido: el modo que llegó no es ninguno de los dos conocidos. No se trata como
	// «el de siempre» a propósito: el modo decide cuánta plata sale en cada transferencia.
	ErrModoPagoInvalido = errors.New("cxp: el modo de pago tiene que ser «FACTURA» o «PROVEEDOR»")
	// ErrRemesaNoEncontrada: no existe esa remesa en esta empresa.
	ErrRemesaNoEncontrada = errors.New("cxp: no se encontró la remesa")
	// ErrLoteNoEncontrado: no existe ese lote en esta empresa.
	ErrLoteNoEncontrado = errors.New("cxp: no se encontró el lote")
)

// Modos de pago de un lote.
const (
	// ModoFactura: una transferencia por factura, cada una con su propio código. El de siempre.
	ModoFactura = "FACTURA"
	// ModoProveedor: una transferencia por proveedor, que cubre todas sus facturas del lote.
	ModoProveedor = "PROVEEDOR"
)

// ModoPagoValido acota lo que puede llegar del cliente. Un modo desconocido NO se acepta como
// «el de siempre»: el modo decide cuánta plata sale en cada transferencia, así que un valor que no
// se entiende es un error, no un default.
func ModoPagoValido(m string) bool {
	return m == ModoFactura || m == ModoProveedor
}

// Remesa es un pago agrupado: lo que sale del banco en UNA transferencia.
type Remesa struct {
	ID     string `json:"id"`
	LoteID string `json:"lote_id"`
	// LoteNumero es la primera mitad de la referencia legible («REM 0029-01»). Viaja con la remesa
	// y no se pide aparte: sin él, el reporte de la conciliación mostraba «REM 0000-01» y nadie
	// podía cruzarlo contra el estado de cuenta, que trae el número de verdad.
	LoteNumero  int64  `json:"lote_numero"`
	ProveedorID string `json:"proveedor_id"`
	Proveedor   string `json:"proveedor"`
	// Numero es el consecutivo DENTRO del lote: la mitad legible de la referencia.
	Numero int `json:"numero"`
	// Huella es el código interno («CXPR-…»), lo que vuelve del banco y permite conciliar.
	Huella string `json:"huella"`
	// MontoCRC es lo que se mandó pagar: la suma de los netos al momento del corte. Se guarda y no
	// se recalcula, porque las facturas que cubre pueden cambiar de estado después.
	MontoCRC string `json:"monto_crc"`
	// Facturas es cuántas cubre (para la pantalla y el aviso de pago).
	Facturas int `json:"facturas"`
}

// ReferenciaRemesa arma la mitad legible de la descripción que viaja al banco: «REM 0043-07».
//
// Lleva el número del LOTE y el de la remesa dentro de él, no el id: es lo que una persona puede
// leer en el estado de cuenta y buscar en el sistema sin tenerlo delante. El código interno
// («CXPR-…») viaja aparte, al final, por la misma razón que en el pago por factura: si el banco
// truncara, se pierde el automatismo pero no el sentido del pago.
func ReferenciaRemesa(numeroLote int64, numeroRemesa int) string {
	return fmt.Sprintf("REM %04d-%02d", numeroLote, numeroRemesa)
}

// referenciaRemesaSQL es la MISMA referencia, armada en SQL para los listados que la muestran sin
// pasar por Go. Espera un alias de lote («lrem») y uno de remesa («rcxp»).
//
// Está pegada a ReferenciaRemesa a propósito: son dos escrituras del mismo formato y tienen que
// coincidir carácter por carácter, porque lo que emite la macro sale de la función y lo que la
// pantalla muestra sale de acá. Si se separaran, la Bandeja mostraría «REM 35-1» y el estado de
// cuenta «REM 0035-01», y volveríamos al problema que esto vino a arreglar. Hay una prueba que
// compara las dos salidas número por número (ver remesa_en_la_bandeja_test.go).
//
// `lpad` con `greatest` y no `to_char(n, 'FM0000')`: el formato de to_char tiene ancho FIJO y
// devuelve «####» cuando el número no cabe, mientras que el %04d de Go crece. Con to_char, el lote
// 10000 habría mostrado «REM ####-01» en la pantalla —y ninguna búsqueda por referencia habría
// calzado— sin que nada fallara. El greatest es lo que evita la otra cara de lpad, que TRUNCA por
// la izquierda cuando el texto ya es más largo que el ancho pedido.
const referenciaRemesaSQL = `('REM ' ||
	lpad(lrem.numero::text, greatest(4, length(lrem.numero::text)), '0') || '-' ||
	lpad(rcxp.numero::text, greatest(2, length(rcxp.numero::text)), '0'))`

// LineaDeAviso es una factura dentro del aviso de pago que se le manda al proveedor.
type LineaDeAviso struct {
	Consecutivo string `json:"consecutivo"`
	// Gasto es la ruta del catálogo («Servicios · Mantenimiento»), para que el proveedor y el
	// área reconozcan contra qué se aplicó el pago.
	Gasto            string `json:"gasto"`
	FechaEmision     string `json:"fecha_emision"`
	FechaVencimiento string `json:"fecha_vencimiento"`
	TotalCRC         string `json:"total_crc"`
	// AnticiposCRC es lo que se descontó por anticipos ya pagados; NetoCRC es lo que se transfiere.
	AnticiposCRC string `json:"anticipos_crc"`
	NetoCRC      string `json:"neto_crc"`
}

// AvisoDePago es el detalle de UNA transferencia: qué facturas cubre y por cuánto.
//
// Es el documento que las entidades grandes llaman «remittance advice» y que resuelve el problema
// real del pago agrupado: el banco lleva una referencia corta, y el detalle —que no cabe ahí— viaja
// por este lado. Sirve para dos cosas distintas: mandárselo al proveedor para que sepa qué le
// pagaron, y conservarlo como respaldo de qué cubrió esa transferencia.
type AvisoDePago struct {
	// Referencia es lo que aparece en el banco («REM 0043-07», o «FC 000487» si se pagó suelta).
	Referencia string `json:"referencia"`
	// Huella es el código interno del pago, el que permite buscarlo en el estado de cuenta.
	Huella      string `json:"huella"`
	LoteNumero  int64  `json:"lote_numero"`
	FechaCorte  string `json:"fecha_corte"`
	Proveedor   string `json:"proveedor"`
	ProveedorID string `json:"proveedor_id"`
	// IBAN es la cuenta a la que se transfirió, para que el proveedor pueda verificarlo.
	IBAN   string         `json:"iban"`
	Lineas []LineaDeAviso `json:"lineas"`
	// TotalCRC es lo que salió del banco en esa transferencia. Se compara contra la suma de los
	// netos de las líneas: si no cuadra, el aviso estaría mintiendo y eso se detecta al armarlo.
	TotalCRC string `json:"total_crc"`
}

// ReporteLote es «lo que se va a pagar» de un corte: una entrada por TRANSFERENCIA, con el detalle
// de las facturas que cubre.
//
// Está armado como una lista de avisos y no como una tabla plana a propósito: así la pantalla, el
// archivo descargable y el correo al proveedor leen la misma estructura, y el total del reporte no
// puede diferir del de la macro porque sale de las mismas filas.
type ReporteLote struct {
	LoteID     string `json:"lote_id"`
	LoteNumero int64  `json:"lote_numero"`
	FechaCorte string `json:"fecha_corte"`
	ModoPago   string `json:"modo_pago"`
	// Pagos: uno por transferencia. En el modo por factura hay tantos como facturas.
	Pagos []AvisoDePago `json:"pagos"`
	// Facturas es el total de facturas cubiertas; Transferencias, cuántas salen al banco. La
	// diferencia entre los dos números ES el ahorro del modo agrupado, y por eso se muestran juntos.
	Facturas       int    `json:"facturas"`
	Transferencias int    `json:"transferencias"`
	TotalCRC       string `json:"total_crc"`
	// SinIBAN son las transferencias cuyo proveedor no tiene cuenta registrada: el banco las
	// rechaza. Se cuentan aparte porque es lo único del reporte que hay que arreglar ANTES de subir
	// el archivo, no después.
	SinIBAN int `json:"sin_iban"`
}
