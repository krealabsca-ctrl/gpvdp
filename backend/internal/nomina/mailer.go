package nomina

// El envío de correo de Nómina (boletas de pago y avisos de vacaciones).
//
// POR QUÉ EXISTE: hasta la migración 0084 estos correos salían por `shared.Mailer`, que guardaba
// el servidor y las credenciales leídas UNA sola vez en el arranque. Consecuencia concreta: la
// boleta de pago de un empleado de Coopeprofa le llegaba desde `cxp@valledepazcr.com` —el buzón de
// otra empresa—. Es el mismo defecto que la 0084 arregló para los proveedores, y quedaba vivo para
// los empleados.
//
// Es una envoltura fina, igual que la de CxP: resuelve con qué buzón manda esta empresa y le pasa
// el sobre al transporte compartido (internal/shared/smtp.go), único lugar del proyecto que habla
// SMTP. No guarda ninguna credencial: las pide en cada envío.
//
// El otro efecto, y no es menor: `shared.Enviar` devuelve un *shared.ErrorSMTP ya clasificado y
// SIN el texto crudo del servidor. El camino viejo envolvía con %w el error de `smtp.SendMail`, y
// ese texto —que en un rechazo de autenticación puede repetir la credencial— terminaba en el log
// del servidor vía `s.log.Warn(..., zap.Error(err))`.

import (
	"context"

	"github.com/gpvdp/erp/internal/shared"
)

// ResolverSMTP devuelve el correo saliente de una empresa en el momento del envío, y de dónde
// salió la configuración (EMPRESA o GLOBAL).
//
// Es un puerto: lo satisface correo.Service. Nómina no importa ese paquete para no acoplarse a él
// —y para poder probar el envío sin base de datos—.
type ResolverSMTP interface {
	SMTPDe(ctx context.Context, empresaID string) (shared.SMTP, string, error)
}

// Mailer manda los correos de Nómina.
type Mailer struct {
	resolver ResolverSMTP
	// global es la caída cuando no hay resolver conectado o la empresa no configuró su buzón.
	global shared.SMTP
}

// NewMailer construye el mailer. `resolver` puede ser nil: sin él se manda por el correo global,
// que es lo que hacía el sistema antes de la 0084.
func NewMailer(resolver ResolverSMTP, global shared.SMTP) *Mailer {
	return &Mailer{resolver: resolver, global: global}
}

// Enviar manda un correo de texto plano desde el buzón de ESA empresa.
func (m *Mailer) Enviar(ctx context.Context, empresaID, to, asunto, cuerpo string) error {
	if m == nil {
		return ErrCorreoNoConfigurado
	}
	cfg := m.global
	if m.resolver != nil {
		resuelto, _, err := m.resolver.SMTPDe(ctx, empresaID)
		if err != nil {
			return err
		}
		cfg = resuelto
	}
	if !cfg.Configurado() {
		return ErrCorreoNoConfigurado
	}
	// El error del transporte viaja TAL CUAL: ya viene clasificado y saneado. Envolverlo con
	// fmt.Errorf rompería el errors.As del handler y volvería a meter texto del servidor en el log.
	return shared.Enviar(ctx, cfg, shared.Sobre{Para: to, Asunto: asunto, Cuerpo: cuerpo})
}
