package shared

// Envío de correo por SMTP, compartido por los módulos que notifican (CxP manda el comprobante
// al proveedor; RRHH manda la boleta y el aviso de vacaciones).
//
// En dev apunta a MailHog (mailhog:1025, sin autenticación), así que se puede ver exactamente lo
// que sale sin mandarle nada a nadie de verdad.

import (
	"bytes"
	"errors"
	"fmt"
	"net/smtp"
	"strings"

	"go.uber.org/zap"
)

// Mailer manda correos de texto.
type Mailer struct {
	addr string
	from string
	user string
	pass string
	log  *zap.Logger
}

// NewMailer construye el mailer con la dirección SMTP (host:puerto), el remitente y las
// credenciales.
//
// `user` y `pass` vacíos = sin autenticación, que es el modo de MailHog en desarrollo. Contra un
// servidor real hay que ponerlas: **sin ellas ningún proveedor acepta el correo**, y así estuvo el
// sistema hasta el 9 de setiembre de 2026 —el envío del comprobante daba error interno en
// producción porque `SMTP_ADDR` seguía apuntando al MailHog que allá no existe—.
func NewMailer(addr, from, user, pass string, log *zap.Logger) *Mailer {
	return &Mailer{addr: addr, from: from, user: user, pass: pass, log: log}
}

// Configurado dice si el mailer puede intentar un envío real.
//
// Sirve para que quien lo use responda «el correo no está configurado» en vez de un error interno:
// no es lo mismo para el usuario, porque uno se arregla poniendo variables y el otro manda a
// revisar el sistema.
func (m *Mailer) Configurado() bool {
	return m != nil && m.addr != "" && m.from != ""
}

// Enviar manda un correo de texto plano.
func (m *Mailer) Enviar(to, asunto, cuerpo string) error {
	if m == nil {
		return errors.New("shared: mailer no configurado")
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", m.from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", asunto)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(cuerpo)
	b.WriteString("\r\n")
	// Con credenciales se autentica; sin ellas va sin auth (MailHog en desarrollo).
	//
	// `smtp.SendMail` hace STARTTLS por su cuenta cuando el servidor lo anuncia y el auth no es
	// nil, así que el 587 de Gmail/Workspace, Microsoft 365, SendGrid y Mailgun funciona sin más.
	// El 465 (TLS implícito) NO: ese requiere abrir la conexión ya cifrada y no está soportado.
	var auth smtp.Auth
	if m.user != "" {
		host := m.addr
		if i := strings.LastIndex(host, ":"); i > 0 {
			host = host[:i]
		}
		auth = smtp.PlainAuth("", m.user, m.pass, host)
	}
	if err := smtp.SendMail(m.addr, auth, m.from, []string{to}, b.Bytes()); err != nil {
		// El error del servidor va envuelto porque dice QUÉ pasó —credenciales, relay denegado,
		// host inalcanzable— y sin eso el diagnóstico es adivinar.
		return fmt.Errorf("shared: enviar correo a %s por %s: %w", to, m.addr, err)
	}
	return nil
}
