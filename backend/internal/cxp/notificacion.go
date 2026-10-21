package cxp

// Texto de las notificaciones de CxP. El contenido lo define la plantilla de la empresa
// (Configuración → Notificaciones); acá solo se llenan los datos.

import (
	"context"
	"strings"

	"github.com/gpvdp/erp/internal/plantillas"
	"github.com/gpvdp/erp/internal/shared"
)

// Plantillero arma el asunto y el cuerpo de una notificación a partir de la plantilla vigente.
// Es un puerto para no acoplar CxP al servicio concreto (y para poder probar el envío).
type Plantillero interface {
	Armar(ctx context.Context, empresaID, clave string, valores map[string]string) (string, string, error)
}

// SetPlantillas conecta el servicio de plantillas. Sin él, el correo sale con el texto de
// fábrica que vive en el catálogo de plantillas (nunca se queda sin texto).
func (s *Service) SetPlantillas(p Plantillero) { s.plantillas = p }

// textoComprobante arma el correo del comprobante de pago al proveedor.
func (s *Service) textoComprobante(ctx context.Context, empresaID string, envio ComprobanteEnvio) (string, string, error) {
	valores := ValoresDelComprobante(envio)
	if s.plantillas != nil {
		return s.plantillas.Armar(ctx, empresaID, plantillas.ClaveCxPComprobante, valores)
	}
	// Sin el servicio conectado se usa el texto de fábrica, para no dejar de notificar.
	t, _ := plantillas.TipoPorClave(plantillas.ClaveCxPComprobante)
	return plantillas.Render(t.AsuntoDefault, valores), plantillas.Render(t.CuerpoDefault, valores), nil
}

// ValoresDelComprobante llena las variables de la plantilla.
//
// Vive aparte y es exportada porque la usan DOS canales —el correo y el texto de WhatsApp— y los
// dos tienen que decir exactamente lo mismo. Con dos armados, en seis meses dicen cosas distintas
// y el proveedor recibe dos versiones del mismo pago.
func ValoresDelComprobante(e ComprobanteEnvio) map[string]string {
	return map[string]string{
		"NOMBRE_PROVEEDOR": e.ProveedorNombre,
		"CONSECUTIVO":      identificaDeLaFactura(e),
		// Formateado para que lo lea una persona: antes salía «₡56500.00».
		"MONTO":               shared.MontoConMoneda(e.Total, e.Moneda),
		"MONEDA":              e.Moneda,
		"REFERENCIA":          ReferenciaParaElProveedor(e),
		"DESCRIPCION_FACTURA": e.Descripcion,
		"DETALLE_FACTURAS":    DetalleDeFacturas(e),
		"FECHA_FACTURA":       e.FechaEmision,
		"LO_PAGADO":           loPagado(e),
	}
}

// loPagado es la frase entera de QUÉ se pagó, con el verbo ya concordado.
//
// Lleva el verbo adentro porque el número cambia la conjugación: con una plantilla fija del tipo
// «su factura [CONSECUTIVO] fue pagada», un pago agrupado salía «su factura 3 facturas fue
// pagada». La concordancia no se puede resolver desde afuera de este dato.
func loPagado(e ComprobanteEnvio) string {
	if e.FacturasDelPago > 1 {
		return "se le pagaron " + itoa(e.FacturasDelPago) + " facturas"
	}
	if c := strings.TrimSpace(e.Consecutivo); c != "" {
		return "se le pagó la factura " + c
	}
	if e.FechaEmision != "" {
		return "se le pagó la factura del " + e.FechaEmision
	}
	return "se le pagó su factura"
}

// identificaDeLaFactura es cómo se nombra lo que se pagó.
//
// Con consecutivo, el consecutivo. SIN consecutivo —10 facturas de 4.542— la fecha, porque el
// texto decía «su factura  fue pagada» con un hueco en el medio y eso se lee como un error del
// sistema. En un pago agrupado, cuántas facturas son: el detalle va en DETALLE_FACTURAS.
func identificaDeLaFactura(e ComprobanteEnvio) string {
	// En un pago agrupado se nombra la PRIMERA y se dice cuántas más. Así sigue calzando en una
	// plantilla vieja («su factura [CONSECUTIVO] fue pagada» → «su factura FC 487 y 2 más fue
	// pagada»), que es lo que ve una empresa que ya personalizó su texto. La frase buena del
	// default sale de [LO_PAGADO].
	if e.FacturasDelPago > 1 {
		if len(e.Facturas) > 0 && strings.TrimSpace(e.Facturas[0].Consecutivo) != "" {
			return e.Facturas[0].Consecutivo + " y " + itoa(e.FacturasDelPago-1) + " más"
		}
		return itoa(e.FacturasDelPago) + " facturas"
	}
	if c := strings.TrimSpace(e.Consecutivo); c != "" {
		return c
	}
	if e.FechaEmision != "" {
		return "del " + e.FechaEmision
	}
	return "pagada"
}

// ReferenciaParaElProveedor es el código con el que el pago aparece EN SU estado de cuenta.
//
// Dos correcciones sobre lo que se mandaba (19-oct-2026):
//
//  1. En un pago AGRUPADO viaja el código de la remesa, no el de la factura. El correo mandaba el
//     de la factura y el banco nunca trae ese: el proveedor buscaba algo inexistente.
//  2. SIN GUION, porque el canal del banco lo borra (ver ParaElBanco). Mandar «CXP-854A6F44CA6A»
//     cuando el estado de cuenta dice «CXP854A6F44CA6A» obliga a adivinar que son el mismo.
func ReferenciaParaElProveedor(e ComprobanteEnvio) string {
	codigo := ParaElBanco(e.HuellaQueViaja)
	if codigo == "" {
		codigo = ParaElBanco(e.Huella)
	}
	// La mitad legible adelante, igual que viaja en la macro: «REM 006127 CXPR9D191E93161E».
	if ref := ParaElBanco(e.ReferenciaRemesa); ref != "" {
		return ref + " " + codigo
	}
	return codigo
}

// DetalleDeFacturas lista las facturas de un pago agrupado, una por línea. Vacío cuando el pago
// cubre una sola: ahí el consecutivo ya está en el texto y repetirlo sobra.
func DetalleDeFacturas(e ComprobanteEnvio) string {
	if len(e.Facturas) < 2 {
		return ""
	}
	var b strings.Builder
	for _, f := range e.Facturas {
		b.WriteString("  · ")
		if c := strings.TrimSpace(f.Consecutivo); c != "" {
			b.WriteString(c)
		} else {
			b.WriteString("(sin consecutivo)")
		}
		if f.FechaEmision != "" {
			b.WriteString(" — " + f.FechaEmision)
		}
		b.WriteString(" — " + shared.MontoConMoneda(f.Total, f.Moneda) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
