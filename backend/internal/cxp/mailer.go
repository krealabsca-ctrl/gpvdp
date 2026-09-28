package cxp

// El envío de correo de CxP. Es una envoltura fina: resuelve CON QUÉ BUZÓN manda esta empresa y le
// pasa el sobre al transporte compartido (internal/shared/smtp.go), que es el único lugar del
// proyecto que habla SMTP.
//
// Lo que cambió con la migración 0084: antes este mailer guardaba el servidor y las credenciales
// que se leyeron UNA vez en el arranque, así que las tres empresas mandaban desde el mismo buzón.
// Ahora no guarda ninguna credencial: las pide en cada envío.

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// ResolverSMTP devuelve el correo saliente de una empresa en el momento del envío, y de dónde
// salió la configuración (EMPRESA o GLOBAL).
//
// Es un puerto: lo satisface correo.Service. CxP no importa ese paquete para no acoplarse a él —y
// para poder probar el envío sin base de datos—.
type ResolverSMTP interface {
	SMTPDe(ctx context.Context, empresaID string) (shared.SMTP, string, error)
}

// EnviadorComprobante es el puerto que usa el servicio para mandar el correo. Lo implementa
// *Mailer; en los tests se sustituye por un doble que falla a pedido.
type EnviadorComprobante interface {
	// EnviarConAdjunto manda el sobre resolviendo el buzón de esa empresa. Devuelve el origen y el
	// remitente SIEMPRE que se hayan podido resolver, incluso si después el envío falló: la
	// bitácora tiene que poder decir contra qué se intentó.
	EnviarConAdjunto(ctx context.Context, empresaID string, sobre shared.Sobre) (origen, remitente string, err error)
}

// Mailer manda los correos de CxP.
type Mailer struct {
	resolver ResolverSMTP
	// global es la caída cuando no hay resolver conectado (o el resolver no resuelve nada).
	global shared.SMTP
	log    *zap.Logger
}

// NewMailer construye el mailer. `resolver` puede ser nil: sin él se manda por el correo global,
// que es exactamente lo que hacía el sistema antes de la migración 0084.
func NewMailer(resolver ResolverSMTP, global shared.SMTP, log *zap.Logger) *Mailer {
	return &Mailer{resolver: resolver, global: global, log: log}
}

// EnviarConAdjunto resuelve el buzón de la empresa y manda el correo.
func (m *Mailer) EnviarConAdjunto(ctx context.Context, empresaID string, sobre shared.Sobre) (string, string, error) {
	if m == nil {
		return "", "", ErrCorreoNoConfigurado
	}
	cfg := m.global
	origen := "GLOBAL"
	if m.resolver != nil {
		resuelto, org, err := m.resolver.SMTPDe(ctx, empresaID)
		origen = org
		if err != nil {
			return origen, "", traducirCorreo(err)
		}
		cfg = resuelto
	}
	if !cfg.Configurado() {
		return origen, "", ErrCorreoNoConfigurado
	}
	if err := shared.Enviar(ctx, cfg, sobre); err != nil {
		// El error del transporte viaja TAL CUAL (es un *shared.ErrorSMTP ya clasificado y sin el
		// texto crudo del servidor): el handler lo traduce a 422 y el service lo guarda en la
		// bitácora con su categoría. Envolverlo con fmt.Errorf acá rompería el errors.As de los dos.
		return origen, cfg.Remitente, err
	}
	return origen, cfg.Remitente, nil
}

// traducirCorreo pasa los fallos de la resolución al vocabulario de CxP.
//
// Sin esta traducción, un «falta CIFRADO_SECRET» llega al switch de responderError, que no lo
// reconoce, y sale como 500 «error interno»: el usuario no puede distinguir «falta una variable en
// el servidor» de «el sistema se cayó», así que reintenta lo mismo. Es la misma clase de defecto
// con la que este proyecto ya se quemó varias veces.
func traducirCorreo(err error) error {
	switch {
	case errors.Is(err, shared.ErrSMTPNoConfigurado):
		return ErrCorreoNoConfigurado
	case errors.Is(err, shared.ErrClaveAusente), errors.Is(err, shared.ErrClaveCorta),
		errors.Is(err, shared.ErrClaveDebil):
		return ErrCifradoNoDisponible
	case errors.Is(err, shared.ErrNoDescifra), errors.Is(err, shared.ErrFormato):
		return ErrSecretoCorreoIlegible
	default:
		return err
	}
}
