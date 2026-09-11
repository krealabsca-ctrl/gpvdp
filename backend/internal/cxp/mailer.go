package cxp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"strings"

	"go.uber.org/zap"
)

// Mailer envía correos por SMTP (en dev, MailHog en mailhog:1025, sin autenticación).
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
// `user` y `pass` vacíos = sin autenticación (MailHog). Contra un servidor real hacen falta: sin
// ellas el envío falla, y así estuvo producción hasta el 9 de setiembre de 2026 —«error interno» al
// mandarle el comprobante al proveedor, porque `SMTP_ADDR` seguía apuntando al MailHog de
// desarrollo, que en el servidor no existe—.
func NewMailer(addr, from, user, pass string, log *zap.Logger) *Mailer {
	return &Mailer{addr: addr, from: from, user: user, pass: pass, log: log}
}

// EnviarConAdjunto manda un correo multipart con un archivo adjunto (base64).
func (m *Mailer) EnviarConAdjunto(to, asunto, cuerpo, filename, mime string, adjunto []byte) error {
	if m == nil || m.addr == "" || m.from == "" {
		// Centinela, no un error genérico: el handler lo traduce a un mensaje que dice qué falta.
		// Antes esto salía como «error interno» y mandaba a revisar el sistema por una variable
		// de entorno sin poner.
		return ErrCorreoNoConfigurado
	}
	const boundary = "GPVDPB0UNDARY7f3a"
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", m.from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", asunto)
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%s\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(cuerpo)
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprintf(&b, "Content-Type: %s\r\n", mime)
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	fmt.Fprintf(&b, "Content-Disposition: attachment; filename=%q\r\n\r\n", filename)
	enc := base64.StdEncoding.EncodeToString(adjunto)
	for i := 0; i < len(enc); i += 76 {
		end := i + 76
		if end > len(enc) {
			end = len(enc)
		}
		b.WriteString(enc[i:end])
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)

	// Con credenciales se autentica (STARTTLS lo negocia `SendMail`); sin ellas va sin auth, que es
	// el modo de MailHog. El puerto soportado es el 587; el 465 exige TLS implícito.
	var auth smtp.Auth
	if m.user != "" {
		host := m.addr
		if i := strings.LastIndex(host, ":"); i > 0 {
			host = host[:i]
		}
		auth = smtp.PlainAuth("", m.user, m.pass, host)
	}
	if err := smtp.SendMail(m.addr, auth, m.from, []string{to}, b.Bytes()); err != nil {
		// Envuelto con el destino y el servidor: el error crudo del paquete `smtp` no dice a quién
		// ni por dónde se intentó, y sin eso el diagnóstico es adivinar.
		return fmt.Errorf("cxp: enviar comprobante a %s por %s: %w", to, m.addr, err)
	}
	return nil
}
