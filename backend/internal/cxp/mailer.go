package cxp

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/smtp"

	"go.uber.org/zap"
)

// Mailer envía correos por SMTP (en dev, MailHog en mailhog:1025, sin autenticación).
type Mailer struct {
	addr string
	from string
	log  *zap.Logger
}

// NewMailer construye el mailer con la dirección SMTP (host:puerto) y el remitente.
func NewMailer(addr, from string, log *zap.Logger) *Mailer {
	return &Mailer{addr: addr, from: from, log: log}
}

// EnviarConAdjunto manda un correo multipart con un archivo adjunto (base64).
func (m *Mailer) EnviarConAdjunto(to, asunto, cuerpo, filename, mime string, adjunto []byte) error {
	if m == nil {
		return errors.New("cxp: mailer no configurado")
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

	// MailHog no requiere auth (auth = nil).
	return smtp.SendMail(m.addr, nil, m.from, []string{to}, b.Bytes())
}
