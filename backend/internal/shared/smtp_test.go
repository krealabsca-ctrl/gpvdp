package shared

// El transporte, contra un servidor SMTP de mentira que corre en el propio test.
//
// Lo que se prueba acá no es «manda correos»: es que la copia al aprobador SE ENTREGUE y quede
// OCULTA, y que el texto que devuelve un servidor hostil NO cruce la frontera de este paquete.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
)

// esErrorSMTP es `errors.As` con nombre, para que cada caso se lea como lo que afirma.
func esErrorSMTP(err error, destino **ErrorSMTP) bool { return errors.As(err, destino) }

// sprint fuerza los tres verbos con los que alguien podría volcar una struct a un log.
func sprint(v any) string { return fmt.Sprintf("%v|%+v|%#v", v, v, v) }

// primeraLinea busca la línea que empieza con el prefijo (para mostrarla en un fallo).
func primeraLinea(mensaje, prefijo string) string {
	for _, l := range strings.Split(mensaje, "\r\n") {
		if strings.HasPrefix(l, prefijo) {
			return l
		}
	}
	return ""
}

// servidorFalso habla lo justo de SMTP para que el cliente de la biblioteca estándar complete una
// conversación. Guarda a quién se le pidió entregar (el SOBRE) y el mensaje completo.
type servidorFalso struct {
	ln net.Listener
	// respuestaAuth reemplaza el "235 ..." de éxito. Sirve para simular un servidor que rechaza
	// las credenciales y —esto es el punto— repite la contraseña en su propia respuesta.
	respuestaAuth string

	mu        sync.Mutex
	recibidos []string
	mensaje   string
}

func levantarServidor(t *testing.T, respuestaAuth string) *servidorFalso {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no se pudo levantar el servidor de prueba: %v", err)
	}
	s := &servidorFalso{ln: ln, respuestaAuth: respuestaAuth}
	go s.atender()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *servidorFalso) puerto() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *servidorFalso) atender() {
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	escribir := func(linea string) { _, _ = conn.Write([]byte(linea + "\r\n")) }

	escribir("220 servidorfalso.test ESMTP")
	for {
		linea, err := r.ReadString('\n')
		if err != nil {
			return
		}
		comando := strings.ToUpper(strings.TrimSpace(linea))
		switch {
		case strings.HasPrefix(comando, "EHLO"), strings.HasPrefix(comando, "HELO"):
			escribir("250-servidorfalso.test")
			escribir("250 AUTH PLAIN LOGIN")
		case strings.HasPrefix(comando, "AUTH"):
			if s.respuestaAuth != "" {
				escribir(s.respuestaAuth)
				continue
			}
			escribir("235 2.7.0 autenticado")
		case strings.HasPrefix(comando, "MAIL FROM"):
			escribir("250 2.1.0 ok")
		case strings.HasPrefix(comando, "RCPT TO"):
			s.mu.Lock()
			s.recibidos = append(s.recibidos, direccionDe(linea))
			s.mu.Unlock()
			escribir("250 2.1.5 ok")
		case comando == "DATA":
			escribir("354 mandá el mensaje, terminá con un punto")
			var cuerpo strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
				cuerpo.WriteString(l)
			}
			s.mu.Lock()
			s.mensaje = cuerpo.String()
			s.mu.Unlock()
			escribir("250 2.0.0 recibido")
		case comando == "QUIT":
			escribir("221 2.0.0 chau")
			return
		default:
			escribir("250 2.0.0 ok")
		}
	}
}

func (s *servidorFalso) loRecibido() ([]string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.recibidos...), s.mensaje
}

// direccionDe saca la dirección de «RCPT TO:<x@y.com>».
func direccionDe(linea string) string {
	i, j := strings.Index(linea, "<"), strings.LastIndex(linea, ">")
	if i < 0 || j < i {
		return strings.TrimSpace(linea)
	}
	return linea[i+1 : j]
}

// configDe arma un SMTP apuntado al servidor de prueba.
//
// El host es 127.0.0.1 a propósito: la biblioteca estándar solo deja autenticar sin cifrar contra
// localhost. Es la misma defensa que protege al sistema de verdad.
func configDe(s *servidorFalso, usuario, password string) SMTP {
	return SMTP{
		Host: "127.0.0.1", Puerto: s.puerto(), Seguridad: SeguridadNinguno,
		Usuario: usuario, Password: Secreto(password),
		Remitente: "cxp@valledepazcr.com", RemitenteNombre: "Valle de Paz — Cuentas por pagar",
	}
}

// ─────────────────────────────────────────────────────────────────────────────

// TestCopiaOcultaVaEnElSobreYNoEnLasCabeceras es la prueba de las dos trampas de la copia oculta.
//
// Con SMTP, quien recibe lo define el sobre (RCPT TO), no las cabeceras. Si la copia fuera solo una
// cabecera `Bcc:`, no se entregaría a nadie y la bitácora registraría una copia que nunca existió;
// y si fuera cabecera Y sobre, muchos servidores la retransmiten y el proveedor terminaría viendo
// justo la dirección de quien autoriza los pagos.
func TestCopiaOcultaVaEnElSobreYNoEnLasCabeceras(t *testing.T) {
	s := levantarServidor(t, "")
	err := Enviar(context.Background(), configDe(s, "", ""), Sobre{
		Para:        "proveedor@ferreteria.cr",
		CopiaOculta: "director@valledepazcr.com",
		Asunto:      "Comprobante de pago",
		Cuerpo:      "adjunto el comprobante",
		Adjunto:     &Adjunto{Filename: "comp.pdf", Mime: "application/pdf", Contenido: []byte("%PDF-1.4")},
	})
	if err != nil {
		t.Fatalf("el envío tenía que salir bien: %v", err)
	}
	recibidos, mensaje := s.loRecibido()
	if len(recibidos) != 2 {
		t.Fatalf("el sobre tenía que llevar 2 destinatarios (proveedor + copia oculta), llevó %d: %v", len(recibidos), recibidos)
	}
	if recibidos[0] != "proveedor@ferreteria.cr" || recibidos[1] != "director@valledepazcr.com" {
		t.Fatalf("el sobre no lleva a quien corresponde: %v", recibidos)
	}
	if strings.Contains(strings.ToLower(mensaje), "bcc:") {
		t.Fatalf("el mensaje NO puede llevar cabecera Bcc (muchos servidores la retransmiten): %q", mensaje)
	}
	if strings.Contains(mensaje, "director@valledepazcr.com") {
		t.Fatalf("la dirección del aprobador no puede aparecer en las cabeceras que ve el proveedor")
	}
}

// TestNoSeDuplicaLaCopiaSiEsLaMismaDireccion: copiar a quien ya es el destinatario mandaría el
// mismo correo dos veces al mismo buzón.
func TestNoSeDuplicaLaCopiaSiEsLaMismaDireccion(t *testing.T) {
	s := levantarServidor(t, "")
	err := Enviar(context.Background(), configDe(s, "", ""), Sobre{
		Para: "mismo@correo.cr", CopiaOculta: "MISMO@correo.cr", Asunto: "x", Cuerpo: "y",
	})
	if err != nil {
		t.Fatalf("el envío tenía que salir bien: %v", err)
	}
	if recibidos, _ := s.loRecibido(); len(recibidos) != 1 {
		t.Fatalf("tenía que haber un solo destinatario, hubo %d: %v", len(recibidos), recibidos)
	}
}

// TestLaContrasenaNoSaleEnElErrorNiAunqueElServidorLaRepita es el hallazgo S-2 convertido en
// prueba.
//
// El servidor falso hace lo peor que puede hacer un servidor: devuelve la contraseña adentro de su
// propio texto de error. Si ese texto se envolviera con %w —que es lo natural— terminaría en la
// respuesta HTTP, en el log y en una columna PERMANENTE de la bitácora. Por eso el transporte
// clasifica y se queda solo con el código.
func TestLaContrasenaNoSaleEnElErrorNiAunqueElServidorLaRepita(t *testing.T) {
	const password = "hunter2-secreto-del-buzon"
	s := levantarServidor(t, "535 5.7.3 Authentication unsuccessful for cxp@valledepazcr.com with password "+password)

	err := Enviar(context.Background(), configDe(s, "cxp@valledepazcr.com", password),
		Sobre{Para: "proveedor@ferreteria.cr", Asunto: "x", Cuerpo: "y"})
	if err == nil {
		t.Fatal("el envío tenía que fallar: el servidor rechazó las credenciales")
	}
	var e *ErrorSMTP
	if !esErrorSMTP(err, &e) {
		t.Fatalf("el error tenía que ser *ErrorSMTP, fue %T", err)
	}
	if e.Categoria != CategoriaAutenticacion {
		t.Fatalf("categoría = %q, se esperaba %q", e.Categoria, CategoriaAutenticacion)
	}
	if e.Codigo != 535 {
		t.Fatalf("código = %d, se esperaba 535 (es lo que sí sirve para diagnosticar)", e.Codigo)
	}
	for _, texto := range []string{e.Error(), e.Detalle()} {
		if strings.Contains(texto, password) {
			t.Fatalf("la contraseña NO puede aparecer en el error: %q", texto)
		}
		if strings.Contains(texto, "Authentication unsuccessful") {
			t.Fatalf("el texto crudo del servidor no puede cruzar esta frontera: %q", texto)
		}
	}
	// Y la struct entera tampoco la revela si alguien la loguea de un plumazo.
	if strings.Contains(sprint(configDe(s, "cxp@valledepazcr.com", password)), password) {
		t.Fatal("imprimir la configuración no puede revelar la contraseña")
	}
}

// TestElDetalleTecnicoLlevaCategoriaCodigoYServidor: es lo que se guarda en
// correo_saliente.probado_error, detrás de admin.correo. Sin esto, «falló» no se puede diagnosticar.
func TestElDetalleTecnicoLlevaCategoriaCodigoYServidor(t *testing.T) {
	s := levantarServidor(t, "535 5.7.3 rechazado")
	err := Enviar(context.Background(), configDe(s, "usuario@x.cr", "clave"),
		Sobre{Para: "p@x.cr", Asunto: "x", Cuerpo: "y"})
	var e *ErrorSMTP
	if !esErrorSMTP(err, &e) {
		t.Fatalf("se esperaba *ErrorSMTP, fue %v", err)
	}
	d := e.Detalle()
	for _, parte := range []string{CategoriaAutenticacion, "535", "127.0.0.1"} {
		if !strings.Contains(d, parte) {
			t.Fatalf("el detalle técnico tenía que mencionar %q: %q", parte, d)
		}
	}
}

// TestHostCaidoEsHostInalcanzable: la categoría distingue «no se pudo conectar» de «te rechazó»,
// que son dos avisos distintos para quien los lee.
func TestHostCaidoEsHostInalcanzable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	puerto := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // ahora ese puerto no tiene a nadie escuchando

	err = Enviar(context.Background(), SMTP{
		Host: "127.0.0.1", Puerto: puerto, Seguridad: SeguridadNinguno, Remitente: "a@b.cr",
	}, Sobre{Para: "p@x.cr", Asunto: "x", Cuerpo: "y"})
	var e *ErrorSMTP
	if !esErrorSMTP(err, &e) {
		t.Fatalf("se esperaba *ErrorSMTP, fue %v", err)
	}
	if e.Categoria != CategoriaHost {
		t.Fatalf("categoría = %q, se esperaba %q", e.Categoria, CategoriaHost)
	}
}

// TestSinConfiguracionNoSeIntenta: el centinela existe para poder decir «falta configurar el
// correo» en vez de «error interno».
func TestSinConfiguracionNoSeIntenta(t *testing.T) {
	if err := Enviar(context.Background(), SMTP{}, Sobre{Para: "p@x.cr"}); err != ErrSMTPNoConfigurado {
		t.Fatalf("se esperaba ErrSMTPNoConfigurado, fue %v", err)
	}
	s := levantarServidor(t, "")
	if err := Enviar(context.Background(), configDe(s, "", ""), Sobre{Para: "  "}); err != ErrSinDestinatario {
		t.Fatalf("se esperaba ErrSinDestinatario, fue %v", err)
	}
}

// TestElAsuntoVaCodificado: los asuntos de las plantillas llevan tildes y guion largo
// («Comprobante de pago — factura …»). Contra MailHog no se nota; contra Outlook o Gmail, un
// asunto con UTF-8 crudo llega como mojibake.
func TestElAsuntoVaCodificado(t *testing.T) {
	s := levantarServidor(t, "")
	err := Enviar(context.Background(), configDe(s, "", ""), Sobre{
		Para: "p@x.cr", Asunto: "Comprobante de pago — factura N° 123", Cuerpo: "y",
	})
	if err != nil {
		t.Fatalf("envío: %v", err)
	}
	_, mensaje := s.loRecibido()
	if !strings.Contains(mensaje, "Subject: =?utf-8?q?") {
		t.Fatalf("el asunto tenía que ir codificado en RFC 2047: %q", primeraLinea(mensaje, "Subject:"))
	}
	if strings.Contains(mensaje, "Subject: Comprobante de pago —") {
		t.Fatal("el asunto NO puede viajar con los bytes UTF-8 crudos")
	}
}

// TestSTARTTLSExigidoNoCaeEnClaro: si el usuario eligió STARTTLS y el servidor no lo ofrece, el
// envío se detiene. Lo contrario sería presentar la contraseña del buzón por un canal sin cifrar.
func TestSTARTTLSExigidoNoCaeEnClaro(t *testing.T) {
	s := levantarServidor(t, "")
	cfg := configDe(s, "cxp@valledepazcr.com", "clave")
	cfg.Seguridad = SeguridadSTARTTLS // el servidor falso no anuncia STARTTLS
	err := Enviar(context.Background(), cfg, Sobre{Para: "p@x.cr", Asunto: "x", Cuerpo: "y"})
	var e *ErrorSMTP
	if !esErrorSMTP(err, &e) {
		t.Fatalf("se esperaba *ErrorSMTP, fue %v", err)
	}
	if e.Categoria != CategoriaTLS {
		t.Fatalf("categoría = %q, se esperaba %q", e.Categoria, CategoriaTLS)
	}
	if recibidos, _ := s.loRecibido(); len(recibidos) != 0 {
		t.Fatalf("no tenía que entregarse nada: %v", recibidos)
	}
}

// TestCategoriaDeClasificaCualquierError: lo que consume la bitácora.
func TestCategoriaDeClasificaCualquierError(t *testing.T) {
	if c := CategoriaDe(&ErrorSMTP{Categoria: CategoriaRelay}); c != CategoriaRelay {
		t.Fatalf("categoría = %q", c)
	}
	if c := CategoriaDe(ErrSinDestinatario); c != CategoriaOtro {
		t.Fatalf("un error ajeno al transporte tiene que caer en OTRO, cayó en %q", c)
	}
}
