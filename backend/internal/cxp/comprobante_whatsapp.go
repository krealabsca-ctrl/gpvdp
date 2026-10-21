package cxp

// Mandar el comprobante por WhatsApp (pedido del Director Financiero, 19-oct-2026).
//
// EL TEXTO ES EL MISMO DEL CORREO. Sale de la misma plantilla y del mismo armado de variables
// (ValoresDelComprobante): dos textos para el mismo hecho terminan diciendo cosas distintas, y el
// proveedor que recibe los dos no sabe cuál vale.
//
// EL PDF NO VIAJA ACÁ, Y NO ES UNA LIMITACIÓN NUESTRA. WhatsApp Web se abre con un enlace
// `wa.me/<número>?text=<mensaje>` que solo admite TEXTO: no hay forma de adjuntar un archivo por
// URL. Por eso el flujo que se eligió (opción 1) es: el sistema abre el chat con el mensaje ya
// escrito y descarga el PDF al mismo tiempo, y quien envía lo arrastra. Prometer el adjunto sería
// prometer algo que el canal no hace.
//
// EL TELÉFONO SE ESCRIBE AL MOMENTO (decisión del Director Financiero). Los 649 proveedores tienen
// el campo vacío, igual que pasó con el IBAN; el número que se escriba queda guardado en la ficha
// para no tener que buscarlo de nuevo la próxima vez.

import (
	"context"
	"errors"
	"strings"

	"github.com/gpvdp/erp/internal/plantillas"
)

// ErrTelefonoInvalido: sin un número utilizable no hay a quién abrirle el chat.
var ErrTelefonoInvalido = errors.New("cxp: escribí el número de WhatsApp del proveedor (8 dígitos, o con el código de país)")

// MensajeWhatsApp es lo que la pantalla necesita para abrir el chat y adjuntar el comprobante.
type MensajeWhatsApp struct {
	// Telefono normalizado a solo dígitos con código de país: es lo que pide wa.me.
	Telefono string `json:"telefono"`
	// Texto del mensaje, el MISMO que iría por correo.
	Texto string `json:"texto"`
	// Archivo es el nombre del comprobante que hay que adjuntar a mano, para poder nombrarlo en
	// la instrucción de la pantalla.
	Archivo string `json:"archivo"`
}

// codigoPaisCR se antepone cuando el número viene con los 8 dígitos locales.
//
// Un número sin código de país abre un chat vacío en WhatsApp sin decir por qué, que es la clase
// de falla que parece del sistema.
const codigoPaisCR = "506"

// NormalizarTelefonoCR deja el número como lo pide wa.me: solo dígitos, con código de país.
//
// Acepta lo que una persona escribe de verdad: «8888-8888», «+506 8888 8888», «(506) 88888888».
func NormalizarTelefonoCR(s string) (string, error) {
	var d strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			d.WriteRune(r)
		}
	}
	n := d.String()
	switch {
	case len(n) == 8:
		// Local: se le antepone el país.
		return codigoPaisCR + n, nil
	case len(n) == 11 && strings.HasPrefix(n, codigoPaisCR):
		return n, nil
	case len(n) >= 10 && len(n) <= 15:
		// Otro país: se respeta tal cual. No se valida el plan de numeración de cada país —sería
		// inventar reglas— pero sí el largo, que es lo que descarta un dedazo.
		return n, nil
	}
	return "", ErrTelefonoInvalido
}

// MensajeDeComprobante arma el texto del comprobante para WhatsApp.
//
// `telefono` vacío usa el de la ficha del proveedor, si lo tiene.
func (s *Service) MensajeDeComprobante(ctx context.Context, empresaID, docID, telefono string) (MensajeWhatsApp, error) {
	envio, err := s.repo.ObtenerComprobanteEnvio(ctx, empresaID, docID)
	if err != nil {
		return MensajeWhatsApp{}, err
	}
	if strings.TrimSpace(telefono) == "" {
		telefono = envio.ProveedorTelefono
	}
	num, err := NormalizarTelefonoCR(telefono)
	if err != nil {
		return MensajeWhatsApp{}, err
	}

	valores := ValoresDelComprobante(envio)
	var texto string
	if s.plantillas != nil {
		_, cuerpo, err := s.plantillas.Armar(ctx, empresaID, plantillas.ClaveCxPComprobante, valores)
		if err != nil {
			return MensajeWhatsApp{}, err
		}
		texto = cuerpo
	} else {
		t, _ := plantillas.TipoPorClave(plantillas.ClaveCxPComprobante)
		texto = plantillas.Render(t.CuerpoDefault, valores)
	}

	// El número se guarda en la ficha: se escribe al momento la PRIMERA vez, no todas. Es
	// best-effort —si falla, el mensaje igual se manda— porque el envío es lo que se pidió y
	// guardar el contacto es la comodidad de la próxima vez.
	if envio.ProveedorTelefono == "" {
		if e := s.repo.GuardarTelefonoProveedorDeDocumento(ctx, empresaID, docID, num); e != nil {
			s.log.Warn("cxp: no se pudo guardar el teléfono del proveedor")
		}
	}
	return MensajeWhatsApp{Telefono: num, Texto: texto, Archivo: envio.Filename}, nil
}
