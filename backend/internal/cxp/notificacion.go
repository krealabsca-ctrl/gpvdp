package cxp

// Texto de las notificaciones de CxP. El contenido lo define la plantilla de la empresa
// (Configuración → Notificaciones); acá solo se llenan los datos.

import (
	"context"

	"github.com/gpvdp/erp/internal/plantillas"
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
	valores := map[string]string{
		"NOMBRE_PROVEEDOR":    envio.ProveedorNombre,
		"CONSECUTIVO":         envio.Consecutivo,
		"MONTO":               montoConMoneda(envio.Total, envio.Moneda),
		"MONEDA":              envio.Moneda,
		"REFERENCIA":          envio.Huella,
		"DESCRIPCION_FACTURA": envio.Descripcion,
	}
	if s.plantillas != nil {
		return s.plantillas.Armar(ctx, empresaID, plantillas.ClaveCxPComprobante, valores)
	}
	// Sin el servicio conectado se usa el texto de fábrica, para no dejar de notificar.
	t, _ := plantillas.TipoPorClave(plantillas.ClaveCxPComprobante)
	return plantillas.Render(t.AsuntoDefault, valores), plantillas.Render(t.CuerpoDefault, valores), nil
}

// montoConMoneda escribe el monto con el símbolo que corresponde a SU moneda.
func montoConMoneda(monto, moneda string) string {
	if moneda == "USD" {
		return "USD " + monto
	}
	return "₡" + monto
}
