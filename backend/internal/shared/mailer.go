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

	"go.uber.org/zap"
)

// Mailer manda correos de texto.
type Mailer struct {
	addr string
	from string
	log  *zap.Logger
}

// NewMailer construye el mailer con la dirección SMTP (host:puerto) y el remitente.
func NewMailer(addr, from string, log *zap.Logger) *Mailer {
	return &Mailer{addr: addr, from: from, log: log}
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
	// MailHog no requiere auth (auth = nil).
	if err := smtp.SendMail(m.addr, nil, m.from, []string{to}, b.Bytes()); err != nil {
		return fmt.Errorf("shared: enviar correo: %w", err)
	}
	return nil
}
