package shared

// El transporte SMTP: el ÚNICO lugar del proyecto que habla con un servidor de correo.
//
// Antes había dos armados de SMTP casi idénticos (internal/cxp/mailer.go e internal/shared/
// mailer.go), los dos apoyados en `smtp.SendMail`. Eso alcanzaba mientras el servidor era uno solo
// y venía de variables de entorno, pero deja de alcanzar cuando cada empresa manda desde su propio
// buzón (mig 0084), porque hacen falta tres cosas que `SendMail` no da:
//
//   - TLS IMPLÍCITO (puerto 465). `SendMail` solo hace STARTTLS oportunista. La columna
//     `correo_saliente.seguridad` acepta 'TLS', así que sin esto la pantalla ofrecería una opción
//     que falla y parecería un problema del servidor del usuario.
//   - PLAZO. `SendMail` no tiene timeout: un servidor que acepta la conexión y no contesta deja el
//     handler colgado para siempre. Acá el dial y la conversación entera llevan plazo explícito.
//   - COPIA OCULTA por el SOBRE. La copia al aprobador va en la lista de destinatarios y NO como
//     cabecera `Bcc:` — ver Sobre.CopiaOculta.
//
// Y una cuarta, que es de seguridad: el texto crudo que devuelve el servidor NO SALE DE ACÁ. Se
// clasifica en una categoría y, si viene, un código SMTP de tres dígitos. Ver ErrorSMTP.

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// Modos de cifrado del canal, tal como los guarda `correo_saliente.seguridad`.
const (
	// SeguridadNinguno: sin cifrado. Es el modo de MailHog en desarrollo.
	SeguridadNinguno = "NINGUNO"
	// SeguridadSTARTTLS: se abre en claro y se sube a TLS con STARTTLS (puerto 587).
	SeguridadSTARTTLS = "STARTTLS"
	// SeguridadTLS: la conexión NACE cifrada (puerto 465, TLS implícito).
	SeguridadTLS = "TLS"
)

// Plazos. Sin esto, un servidor que acepta la conexión y se queda callado cuelga el handler.
const (
	// TimeoutDial es lo que se espera por la conexión TCP (y el handshake TLS).
	TimeoutDial = 10 * time.Second
	// TimeoutSesion es el plazo de la conversación completa una vez conectado.
	TimeoutSesion = 30 * time.Second
)

// SMTP es un servidor de correo saliente ya resuelto: o el de una empresa (correo_saliente) o el
// global del proceso (las variables SMTP_*).
//
// La contraseña es `Secreto` y no `string` a propósito: así un `%v` sobre esta struct, un
// `zap.Any` o un `json.Marshal` imprimen `***` en vez de la credencial del buzón corporativo.
type SMTP struct {
	Host            string
	Puerto          int
	Seguridad       string
	Usuario         string
	Password        Secreto
	Remitente       string
	RemitenteNombre string
}

// Addr devuelve "host:puerto".
func (s SMTP) Addr() string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Puerto)) }

// Configurado dice si hay con qué intentar un envío: servidor y remitente.
//
// Sirve para responder «falta configurar el correo» en vez de un error interno — no es lo mismo
// para quien lo lee, porque uno se arregla poniendo un dato y el otro manda a revisar el sistema.
func (s SMTP) Configurado() bool { return s.Host != "" && s.Puerto > 0 && s.Remitente != "" }

// Adjunto es un archivo que viaja con el correo (hoy: el PDF del comprobante de pago).
type Adjunto struct {
	Filename  string
	Mime      string
	Contenido []byte
}

// Sobre es el correo a enviar.
type Sobre struct {
	Para   string
	Asunto string
	Cuerpo string
	// CopiaOculta es la copia al aprobador del pago (decisión del DF, 17-set-2026).
	//
	// VA POR EL SOBRE Y SIN CABECERA. Con SMTP, quien recibe lo define la lista de destinatarios
	// del sobre (`RCPT TO`), no las cabeceras del mensaje. De ahí las dos trampas que este campo
	// evita por construcción:
	//
	//   - escribir una cabecera `Bcc:` NO oculta nada: muchos servidores la retransmiten y el
	//     proveedor termina viendo justo la dirección que se quiso reservar;
	//   - escribir la cabecera SIN meter la dirección en el sobre no entrega la copia a nadie, y
	//     entonces la bitácora diría «copia» sobre una copia que nunca existió.
	//
	// Oculta y no visible porque publicarle a 649 proveedores externos la dirección de quien
	// autoriza los pagos es entregar, factura por factura, el blanco exacto de un fraude de cambio
	// de cuenta bancaria. (Hallazgo S-9 de la revisión de seguridad del 2026-09-17.)
	CopiaOculta string
	// Adjunto opcional. Sin él sale un correo de texto plano.
	Adjunto *Adjunto
}

// ─────────────────────────────────────────────────────────────────────────────
// Errores
// ─────────────────────────────────────────────────────────────────────────────

var (
	// ErrSMTPNoConfigurado: no hay servidor ni remitente con qué intentar el envío.
	ErrSMTPNoConfigurado = errors.New("shared: el correo saliente no está configurado")
	// ErrSinDestinatario: no se puede mandar un correo a nadie.
	ErrSinDestinatario = errors.New("shared: falta el destinatario del correo")
)

// Categorías de fallo. Son EXACTAMENTE los valores que acepta el CHECK de
// `comprobante_envio.error_categoria` (mig 0084): agregar una acá sin agregarla allá hace que el
// INSERT de la bitácora rebote justo cuando el envío ya falló.
const (
	CategoriaAutenticacion = "AUTENTICACION_RECHAZADA"
	CategoriaHost          = "HOST_INALCANZABLE"
	CategoriaRelay         = "RELAY_DENEGADO"
	CategoriaTLS           = "TLS_FALLIDO"
	CategoriaTimeout       = "TIEMPO_AGOTADO"
	CategoriaOtro          = "OTRO"
)

// ErrorSMTP es el ÚNICO error que sale de este archivo cuando el servidor rechaza algo.
//
// NO envuelve el error original, y es deliberado: el texto libre que devuelve un servidor SMTP
// termina (a) en la respuesta HTTP, (b) en una columna permanente de la bitácora y (c) en el log.
// Un servidor puede repetir en su propia respuesta lo que se le mandó —incluida la credencial— y
// ningún `strings.ReplaceAll` en la capa de arriba lo ataja, porque AUTH PLAIN viaja en base64 y
// la contraseña literal no aparece como tal. La forma segura no es limpiar el texto: es no
// dejarlo cruzar. Acá se clasifica y se queda con el código de tres dígitos, que es lo que sirve
// para diagnosticar. (Hallazgo S-2.)
type ErrorSMTP struct {
	// Categoria es uno de los valores de arriba.
	Categoria string
	// Codigo es el código SMTP de 3 dígitos (0 si el fallo fue antes de que el servidor hablara).
	Codigo int
	// Servidor es "host:puerto". NUNCA el usuario ni la contraseña.
	Servidor string
}

// Error es la frase para el operador: dice qué pasó y a quién avisarle, sin nombrar el servidor ni
// el buzón. Es lo que se guarda en `comprobante_envio.error`, que leen SIETE roles.
func (e *ErrorSMTP) Error() string {
	switch e.Categoria {
	case CategoriaAutenticacion:
		return "el servidor de correo rechazó las credenciales — avisar a Dirección para que revise el correo saliente"
	case CategoriaHost:
		return "no se pudo conectar con el servidor de correo — avisar a Dirección"
	case CategoriaRelay:
		return "el servidor de correo no aceptó la dirección de destino; revisá el correo del proveedor"
	case CategoriaTLS:
		return "la conexión segura con el servidor de correo falló — avisar a Dirección"
	case CategoriaTimeout:
		return "el servidor de correo no contestó a tiempo; se puede reintentar"
	default:
		return "el servidor de correo rechazó el envío — avisar a Dirección"
	}
}

// Detalle agrega el servidor y el código SMTP. Es el DETALLE TÉCNICO y va únicamente a
// `correo_saliente.probado_error`, detrás del permiso admin.correo, que hoy tiene un solo rol.
// Nunca a la bitácora del comprobante. (Hallazgo S-3.)
func (e *ErrorSMTP) Detalle() string {
	var b strings.Builder
	b.WriteString(e.Error())
	b.WriteString(" [")
	b.WriteString(e.Categoria)
	if e.Codigo > 0 {
		b.WriteString(", código SMTP ")
		b.WriteString(strconv.Itoa(e.Codigo))
	}
	if e.Servidor != "" {
		b.WriteString(", servidor ")
		b.WriteString(e.Servidor)
	}
	b.WriteString("]")
	return b.String()
}

// CategoriaDe devuelve la categoría de cualquier error de envío, para guardarla en la bitácora.
// Un error que no venga del transporte cae en OTRO.
func CategoriaDe(err error) string {
	var e *ErrorSMTP
	if errors.As(err, &e) {
		return e.Categoria
	}
	return CategoriaOtro
}

// ─────────────────────────────────────────────────────────────────────────────
// El envío
// ─────────────────────────────────────────────────────────────────────────────

// Enviar manda el sobre por el servidor indicado.
//
// No guarda estado entre llamadas: cada envío abre su conexión con las credenciales que le pasan.
// Esa es la forma de que dos empresas no se mezclen — un cliente SMTP cacheado en el proceso
// reintroduciría exactamente el defecto que la migración 0084 viene a arreglar, ahora en silencio.
// (Hallazgo S-8.)
func Enviar(ctx context.Context, cfg SMTP, sobre Sobre) error {
	if !cfg.Configurado() {
		return ErrSMTPNoConfigurado
	}
	if strings.TrimSpace(sobre.Para) == "" {
		return ErrSinDestinatario
	}
	addr := cfg.Addr()

	dialer := net.Dialer{Timeout: TimeoutDial}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return errorDeConexion(err, addr)
	}
	defer func() { _ = conn.Close() }()
	// El plazo cubre la conversación entera (saludo, auth, datos): un servidor que contesta el
	// TCP y después se calla no puede dejar el handler colgado.
	_ = conn.SetDeadline(time.Now().Add(TimeoutSesion))

	if cfg.Seguridad == SeguridadTLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return &ErrorSMTP{Categoria: CategoriaTLS, Servidor: addr}
		}
		conn = tlsConn
	}

	cliente, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return clasificar(err, CategoriaHost, addr)
	}
	defer func() { _ = cliente.Close() }()

	if cfg.Seguridad == SeguridadSTARTTLS {
		// Exigido, no oportunista: si el usuario eligió STARTTLS y el servidor no lo ofrece, el
		// envío NO sigue en claro con la contraseña del buzón adentro.
		if ok, _ := cliente.Extension("STARTTLS"); !ok {
			return &ErrorSMTP{Categoria: CategoriaTLS, Servidor: addr}
		}
		if err := cliente.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return clasificar(err, CategoriaTLS, addr)
		}
	}

	if cfg.Usuario != "" {
		if ok, _ := cliente.Extension("AUTH"); !ok {
			return &ErrorSMTP{Categoria: CategoriaAutenticacion, Servidor: addr}
		}
		// Revelar() es el único camino al valor y existe para poder buscarlo con grep: esta es la
		// ÚNICA línea del proyecto que expone la contraseña del correo, y solo hacia el servidor.
		if err := cliente.Auth(smtp.PlainAuth("", cfg.Usuario, cfg.Password.Revelar(), cfg.Host)); err != nil {
			return clasificar(err, CategoriaAutenticacion, addr)
		}
	}

	if err := cliente.Mail(cfg.Remitente); err != nil {
		return clasificar(err, CategoriaRelay, addr)
	}
	// Los destinatarios del SOBRE. La copia oculta entra acá y en ningún otro lado: es lo que la
	// entrega de verdad y lo que la mantiene invisible para el proveedor.
	for _, dest := range destinatarios(sobre) {
		if err := cliente.Rcpt(dest); err != nil {
			return clasificar(err, CategoriaRelay, addr)
		}
	}
	w, err := cliente.Data()
	if err != nil {
		return clasificar(err, CategoriaOtro, addr)
	}
	if _, err := w.Write(mensaje(cfg, sobre)); err != nil {
		_ = w.Close()
		return clasificar(err, CategoriaOtro, addr)
	}
	if err := w.Close(); err != nil {
		return clasificar(err, CategoriaOtro, addr)
	}
	if err := cliente.Quit(); err != nil {
		return clasificar(err, CategoriaOtro, addr)
	}
	return nil
}

// destinatarios arma la lista del sobre: el proveedor y, si la hay, la copia oculta.
func destinatarios(s Sobre) []string {
	out := []string{strings.TrimSpace(s.Para)}
	if c := strings.TrimSpace(s.CopiaOculta); c != "" && !strings.EqualFold(c, out[0]) {
		out = append(out, c)
	}
	return out
}

// mensaje arma las cabeceras y el cuerpo.
//
// El asunto va codificado en RFC 2047 porque los de las plantillas llevan tildes y guion largo
// («Comprobante de pago — factura …»). Contra MailHog no se nota; contra Outlook o Gmail, un
// asunto con bytes UTF-8 crudos llega como mojibake.
func mensaje(cfg SMTP, sobre Sobre) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", direccion(cfg.RemitenteNombre, cfg.Remitente))
	// Trimeado igual que en `destinatarios`: si la cabecera lleva el valor crudo y el sobre el
	// limpio, una dirección con un salto de línea al final corta el bloque de cabeceras y el
	// adjunto se pierde. El mismo valor en los dos lados y no hay diferencia que explotar.
	fmt.Fprintf(&b, "To: %s\r\n", strings.TrimSpace(sobre.Para))
	// Sin cabecera Bcc: ver Sobre.CopiaOculta.
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", sobre.Asunto))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")

	if sobre.Adjunto == nil {
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		b.WriteString(sobre.Cuerpo)
		b.WriteString("\r\n")
		return []byte(b.String())
	}

	const frontera = "GPVDPB0UNDARY7f3a"
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%s\r\n\r\n", frontera)
	fmt.Fprintf(&b, "--%s\r\n", frontera)
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(sobre.Cuerpo)
	b.WriteString("\r\n")
	fmt.Fprintf(&b, "--%s\r\n", frontera)
	fmt.Fprintf(&b, "Content-Type: %s\r\n", sobre.Adjunto.Mime)
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	fmt.Fprintf(&b, "Content-Disposition: attachment; filename=%q\r\n\r\n", sobre.Adjunto.Filename)
	b.WriteString(base64EnLineas(sobre.Adjunto.Contenido))
	fmt.Fprintf(&b, "--%s--\r\n", frontera)
	return []byte(b.String())
}

// direccion arma «Nombre <correo>» cuando hay nombre. El nombre también va codificado: «Valle de
// Paz — Cuentas por pagar» tiene tilde y guion largo.
func direccion(nombre, correo string) string {
	if strings.TrimSpace(nombre) == "" {
		return correo
	}
	return mime.QEncoding.Encode("utf-8", nombre) + " <" + correo + ">"
}

// ─────────────────────────────────────────────────────────────────────────────
// Clasificación de los fallos
// ─────────────────────────────────────────────────────────────────────────────

// errorDeConexion distingue «no contestó a tiempo» de «no se pudo conectar»: son dos avisos
// distintos para quien lo lee (uno se reintenta, el otro hay que arreglarlo).
func errorDeConexion(err error, addr string) *ErrorSMTP {
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return &ErrorSMTP{Categoria: CategoriaTimeout, Servidor: addr}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &ErrorSMTP{Categoria: CategoriaTimeout, Servidor: addr}
	}
	return &ErrorSMTP{Categoria: CategoriaHost, Servidor: addr}
}

// clasificar convierte el error de la biblioteca en un ErrorSMTP, SIN arrastrar el texto del
// servidor. `porDefecto` es la categoría del paso en el que se estaba (auth, destinatarios, …).
func clasificar(err error, porDefecto, addr string) *ErrorSMTP {
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return &ErrorSMTP{Categoria: CategoriaTimeout, Servidor: addr}
	}
	var terr *textproto.Error
	if errors.As(err, &terr) {
		return &ErrorSMTP{Categoria: categoriaDeCodigo(terr.Code, porDefecto), Codigo: terr.Code, Servidor: addr}
	}
	return &ErrorSMTP{Categoria: porDefecto, Servidor: addr}
}

// categoriaDeCodigo manda el código SMTP a su categoría. Los de autenticación y los de relay son
// los dos que el usuario sí puede distinguir: uno lo arregla Dirección, el otro es el correo del
// proveedor. El resto se queda con la categoría del paso.
func categoriaDeCodigo(codigo int, porDefecto string) string {
	switch codigo {
	case 530, 534, 535, 538: // credenciales rechazadas / se exige autenticación
		return CategoriaAutenticacion
	case 550, 551, 553, 554: // buzón inexistente, relay denegado, transacción rechazada
		return CategoriaRelay
	case 421, 450, 451: // servicio no disponible / demorado
		if porDefecto == CategoriaAutenticacion {
			return CategoriaAutenticacion
		}
		return CategoriaHost
	default:
		return porDefecto
	}
}

// base64EnLineas codifica el adjunto en líneas de 76 caracteres, como pide MIME.
func base64EnLineas(datos []byte) string {
	const ancho = 76
	enc := base64.StdEncoding.EncodeToString(datos)
	var b strings.Builder
	for i := 0; i < len(enc); i += ancho {
		fin := i + ancho
		if fin > len(enc) {
			fin = len(enc)
		}
		b.WriteString(enc[i:fin])
		b.WriteString("\r\n")
	}
	return b.String()
}
