package cxp

// Liberar una factura de contado para pagarla por banco.
//
// EL PEDIDO (Director Financiero, 12-oct-2026): «necesitamos generar una huella para ciertas
// facturas que el proveedor lo pone como contado pero no lo es, porque de lo contrario quedaría
// sucio el sistema y facturas sin pagar cuando deben pagarse».
//
// QUÉ SE LEVANTA. El candado `bloqueado_para_pago`, que el importador pone cuando el comprobante
// declara condición de venta «contado». No es un capricho: una compra de contado suele estar ya
// pagada con el fondo de caja chica, y la misma factura que llega después por correo se vuelve
// cuenta por pagar; pagarla por banco sacaría el gasto dos veces, y nada puede detectarlo porque
// el vale de caja chica no lleva la clave del comprobante.
//
// POR ESO LIBERAR EXIGE MOTIVO. Es una afirmación con consecuencias: «esta factura NO se pagó por
// caja chica». Si en seis meses aparece un doble pago, lo único que permite reconstruir qué pasó
// es quién lo dijo, cuándo y con qué argumento. Un botón sin motivo convierte la única barrera
// contra el doble pago en un trámite de un clic.
//
// LO QUE NO HACE: no cambia el estado de la factura, no la aprueba y no la mete en ningún lote.
// Solo quita el candado; de ahí en adelante sigue el mismo camino que cualquier otra.

import (
	"errors"
	"strings"
)

var (
	// ErrNoEstaBloqueada: liberar algo que no está trancado no es un error del sistema, pero sí
	// una señal de que quien lo pidió está mirando otra factura.
	ErrNoEstaBloqueada = errors.New("cxp: la factura no está bloqueada para pago")
	// ErrMotivoLiberacionRequerido: sin motivo no se libera. Ver arriba.
	ErrMotivoLiberacionRequerido = errors.New("cxp: decí por qué esta factura sí se paga por banco")
)

// motivoLiberacionMinimo es el largo mínimo del motivo.
//
// Veinte caracteres no garantizan una explicación, pero sí descartan la respuesta refleja —«ok»,
// «si», «ya»— que es la que deja el expediente sin nada que leer. Quien tiene una razón de verdad
// la escribe en una línea sin notar el mínimo.
const motivoLiberacionMinimo = 20

// LiberacionInput es lo que hace falta para levantar el candado de varias facturas.
//
// Varias y no una: el emisor que marca mal la condición lo hace con TODAS sus facturas, no con
// una. Liberarlas de a una es el mismo trabajo manual que ya nos costó la carpeta de
// comprobantes, y un motivo escrito treinta veces termina siendo «ok» a la tercera.
//
// EL CAMINO ES LA FICHA DEL PROVEEDOR, no una pantalla aparte. Se marca «manda el convenio», el
// sistema dice cuántas facturas de ese proveedor quedaron trancadas por la condición que el
// emisor declaró mal, y se liberan ahí mismo. Es un solo gesto en el lugar donde se tomó la
// decisión; una pantalla separada obligaría a acordarse de ir después, y nadie se acuerda.
type LiberacionInput struct {
	DocumentoIDs []string
	Motivo       string
}

// FacturaDeContado es una factura trancada por el candado del contado, lista para mostrar antes
// de liberarla.
//
// Se muestran ANTES de liberar y con su monto: liberar es levantar la única barrera que queda
// contra el doble pago, y hacerlo sobre una lista que no se vio es firmar en blanco.
type FacturaDeContado struct {
	DocumentoID  string `json:"documento_id"`
	Consecutivo  string `json:"consecutivo"`
	FechaEmision string `json:"fecha_emision"`
	Total        string `json:"total"`
	Moneda       string `json:"moneda"`
	Motivo       string `json:"motivo"`
}

// Validar revisa el pedido antes de tocar la base.
func (in LiberacionInput) Validar() error {
	if len(in.DocumentoIDs) == 0 {
		return ErrSinDocumentos
	}
	if len([]rune(strings.TrimSpace(in.Motivo))) < motivoLiberacionMinimo {
		return ErrMotivoLiberacionRequerido
	}
	return nil
}

// FacturaLiberada es el parte de una factura que quedó libre.
type FacturaLiberada struct {
	DocumentoID string `json:"documento_id"`
	Consecutivo string `json:"consecutivo"`
	Proveedor   string `json:"proveedor"`
	Total       string `json:"total"`
	Moneda      string `json:"moneda"`
}

// ResultadoLiberacion dice qué quedó libre y qué no, con el motivo de cada descarte.
//
// Los descartes viajan con nombre: pedir diez y que salga «7 liberadas» sin decir cuáles fueron
// las otras tres deja al operador creyendo que están todas, y esas tres siguen sin poder pagarse
// hasta que alguien las vuelva a buscar de a una.
type ResultadoLiberacion struct {
	Liberadas []FacturaLiberada `json:"liberadas"`
	// NoSePudo: id → por qué. Vacío si todas entraron.
	NoSePudo map[string]string `json:"no_se_pudo"`
}
