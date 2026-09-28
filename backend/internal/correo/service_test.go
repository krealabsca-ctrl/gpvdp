package correo

// Lo que se prueba acá es, sobre todo, lo que NO tiene que pasar: que la contraseña no vuelva al
// navegador, que no se guarde en claro cuando no se puede cifrar, que cambiar el servidor no
// permita reusar la credencial guardada, y que una empresa con su buzón configurado no termine
// mandando en silencio por el buzón de otra.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

const (
	empresaVDP  = "427f8851-1e94-4c13-abbd-3f7bb9f456e9"
	empresaCoop = "f151fabc-f575-4871-817a-8f7bb193d04b"
	// claveDePrueba imita lo que genera el instalador: 64 caracteres variados. NewCifrador exige
	// largo Y variedad, así que una frase corta o repetida no sirve ni acá.
	claveDePrueba = "7Kq2mZ9vX4pR8tL3wN6yB1cF5hJ0dS7gA2eU4iO9kM3nQ8rT6uY1zP5xC0bV7jH4"
)

// ─────────────────────────────────────────────────────────────────────────────
// Dobles
// ─────────────────────────────────────────────────────────────────────────────

type repoFalso struct {
	fila Guardado
	hay  bool
	err  error

	// Lo que el service le mandó escribir.
	guardada         *Guardado
	passwordGuardada string
	escrituras       int
	detallePrueba    string
	pruebas          int

	usuarioNombre string
	usuarioEmail  string
	errUsuario    error
}

func (r *repoFalso) Obtener(context.Context, string) (Guardado, error) {
	if r.err != nil {
		return Guardado{}, r.err
	}
	if !r.hay {
		return Guardado{}, ErrConfigNoEncontrada
	}
	return r.fila, nil
}

func (r *repoFalso) Guardar(_ context.Context, _ string, g Guardado, passwordCifrada, _ string) error {
	copia := g
	r.guardada = &copia
	r.passwordGuardada = passwordCifrada
	r.escrituras++
	r.fila, r.hay = g, true
	r.fila.PasswordCifrada = passwordCifrada
	return nil
}

func (r *repoFalso) MarcarPrueba(_ context.Context, _, detalle string) error {
	r.detallePrueba = detalle
	r.pruebas++
	return nil
}

func (r *repoFalso) CorreoUsuario(context.Context, string) (string, string, error) {
	return r.usuarioNombre, r.usuarioEmail, r.errUsuario
}

func cifradorDePrueba(t *testing.T) *shared.Cifrador {
	t.Helper()
	c, err := shared.NewCifrador(claveDePrueba)
	if err != nil {
		t.Fatalf("no se pudo construir el cifrador de prueba: %v", err)
	}
	return c
}

// servicio arma el service con lo mínimo. `global` vacío = no hay correo global.
func servicio(t *testing.T, repo *repoFalso, cif *shared.Cifrador, global shared.SMTP) *Service {
	t.Helper()
	return NewService(repo, nil, zap.NewNop(), cif, global, false)
}

func globalDePrueba() shared.SMTP {
	return shared.SMTP{Host: "mailhog", Puerto: 1025, Seguridad: shared.SeguridadNinguno,
		Remitente: "cxp@valledepazcr.com"}
}

// entradaValida es un PUT completo y correcto.
func entradaValida(password string) Input {
	in := Input{
		Host: "smtp.office365.com", Puerto: 587, Seguridad: shared.SeguridadSTARTTLS,
		Usuario: "cxp@valledepazcr.com", Remitente: "cxp@valledepazcr.com",
		RemitenteNombre: "Valle de Paz — Cuentas por pagar", Activo: true,
	}
	if password != "" {
		s := shared.Secreto(password)
		in.Password = &s
	}
	return in
}

// ─────────────────────────────────────────────────────────────────────────────
// La contraseña no vuelve
// ─────────────────────────────────────────────────────────────────────────────

// TestElGETNuncaDevuelveLaContrasena es la garantía central de toda la función.
//
// Se serializa el DTO exactamente como sale por HTTP y se afirma que ni la contraseña ni el valor
// cifrado aparecen, y que NO EXISTE una clave "password" en el JSON. Lo último importa tanto como
// lo primero: un campo vacío o enmascarado invita a que mañana alguien lo llene.
func TestElGETNuncaDevuelveLaContrasena(t *testing.T) {
	const password = "contrasena-del-buzon-corporativo"
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())

	if err := svc.Guardar(context.Background(), empresaVDP, entradaValida(password), "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	cifrada := repo.passwordGuardada
	if cifrada == "" || !strings.HasPrefix(cifrada, "v1.") {
		t.Fatalf("lo guardado tenía que ser un valor cifrado con marca de versión, fue %q", cifrada)
	}
	if strings.Contains(cifrada, password) {
		t.Fatal("la contraseña se guardó en claro")
	}

	cfg, err := svc.Obtener(context.Background(), empresaVDP)
	if err != nil {
		t.Fatalf("obtener: %v", err)
	}
	if !cfg.TienePassword {
		t.Fatal("el GET tiene que decir que HAY una contraseña guardada")
	}
	crudo, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	cuerpo := string(crudo)
	if strings.Contains(cuerpo, password) {
		t.Fatalf("la contraseña salió en la respuesta: %s", cuerpo)
	}
	if strings.Contains(cuerpo, cifrada) {
		t.Fatalf("el valor cifrado tampoco puede salir: %s", cuerpo)
	}
	var mapa map[string]any
	if err := json.Unmarshal(crudo, &mapa); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, existe := mapa["password"]; existe {
		t.Fatal("el contrato NO puede tener un campo `password`, ni vacío ni enmascarado")
	}
}

// TestSinCifradoSecretNoSeGuardaNada: si no se puede cifrar, no se guarda NI EL RESTO de los
// campos. Guardar «casi todo» dejaría una configuración que parece puesta y no manda.
func TestSinCifradoSecretNoSeGuardaNada(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, nil, globalDePrueba()) // sin cifrador: instalación sin CIFRADO_SECRET

	err := svc.Guardar(context.Background(), empresaVDP, entradaValida("una-clave"), "u1")
	if !errors.Is(err, shared.ErrClaveAusente) {
		t.Fatalf("se esperaba el centinela de clave ausente, fue %v", err)
	}
	if !strings.Contains(err.Error(), "CIFRADO_SECRET") {
		t.Fatalf("el mensaje tiene que nombrar la variable que hay que poner: %v", err)
	}
	if repo.escrituras != 0 {
		t.Fatalf("no tenía que escribirse nada, hubo %d escrituras", repo.escrituras)
	}
}

// TestGuardarSinContrasenaNoExigeCifrado: MailHog y cualquier relay interno no piden usuario. Sin
// contraseña que proteger, la falta de CIFRADO_SECRET no tiene por qué estorbar.
func TestGuardarSinContrasenaNoExigeCifrado(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, nil, shared.SMTP{})
	in := Input{Host: "mailhog", Puerto: 1025, Seguridad: shared.SeguridadNinguno,
		Remitente: "cxp@valledepazcr.com", Activo: true}
	if err := svc.Guardar(context.Background(), empresaVDP, in, "u1"); err != nil {
		t.Fatalf("tenía que poder guardarse sin contraseña: %v", err)
	}
	if repo.passwordGuardada != "" {
		t.Fatalf("no tenía que quedar ninguna contraseña: %q", repo.passwordGuardada)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Las tres semánticas de `password`
// ─────────────────────────────────────────────────────────────────────────────

// TestPasswordAusenteConservaLaGuardada: el GET no puede devolver la contraseña, así que el
// formulario no la puede round-tripear. «Ausente» tiene que significar «no la toques».
func TestPasswordAusenteConservaLaGuardada(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	if err := svc.Guardar(context.Background(), empresaVDP, entradaValida("la-original"), "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	original := repo.passwordGuardada

	// Mismo servidor, solo cambia el nombre que se muestra: la contraseña no viene.
	in := entradaValida("")
	in.RemitenteNombre = "Valle de Paz — Pagos"
	if err := svc.Guardar(context.Background(), empresaVDP, in, "u1"); err != nil {
		t.Fatalf("segunda guardada: %v", err)
	}
	if repo.passwordGuardada != original {
		t.Fatal("la contraseña guardada tenía que conservarse tal cual")
	}
}

// TestPasswordVaciaLaBorra: es la única forma de volver a «sin autenticación» sin borrar la fila.
func TestPasswordVaciaLaBorra(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	if err := svc.Guardar(context.Background(), empresaVDP, entradaValida("la-original"), "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	vacia := shared.Secreto("")
	in := entradaValida("")
	in.Password = &vacia
	in.Usuario = "" // sin contraseña no puede haber usuario: lo ataja el CHECK de la base y el service
	if err := svc.Guardar(context.Background(), empresaVDP, in, "u1"); err != nil {
		t.Fatalf("borrar la contraseña: %v", err)
	}
	if repo.passwordGuardada != "" {
		t.Fatalf("la contraseña tenía que quedar borrada, quedó %q", repo.passwordGuardada)
	}
}

// TestUsuarioSinContrasenaSeRechaza: con usuario y sin contraseña, el envío intenta AUTH PLAIN con
// la contraseña vacía, el servidor rechaza, el correo no sale y nadie entiende por qué.
func TestUsuarioSinContrasenaSeRechaza(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	if err := svc.Guardar(context.Background(), empresaVDP, entradaValida(""), "u1"); !errors.Is(err, ErrPasswordRequerida) {
		t.Fatalf("se esperaba ErrPasswordRequerida, fue %v", err)
	}
	if repo.escrituras != 0 {
		t.Fatal("no tenía que escribirse nada")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// S-1: no se puede exfiltrar la contraseña cambiando el servidor
// ─────────────────────────────────────────────────────────────────────────────

// TestCambiarElServidorExigeEscribirLaContrasenaDeNuevo reproduce el ataque del hallazgo S-1.
//
// Sin esta regla, quien pueda editar la pantalla apunta el `host` a su propio servidor SIN mandar
// contraseña («ausente = se conserva la guardada»), aprieta «probar», y el sistema le presenta la
// contraseña real del buzón corporativo a ese servidor. Nunca la ve en pantalla y se la lleva igual.
func TestCambiarElServidorExigeEscribirLaContrasenaDeNuevo(t *testing.T) {
	casos := []struct {
		nombre string
		tocar  func(*Input)
	}{
		{"otro host", func(in *Input) { in.Host = "smtp.atacante.com" }},
		{"otro puerto", func(in *Input) { in.Puerto = 2525 }},
		{"otro usuario", func(in *Input) { in.Usuario = "otro@valledepazcr.com" }},
		{"sin cifrado del canal", func(in *Input) { in.Seguridad = shared.SeguridadNinguno }},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			repo := &repoFalso{}
			svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
			if err := svc.Guardar(context.Background(), empresaVDP, entradaValida("la-del-buzon"), "u1"); err != nil {
				t.Fatalf("guardar: %v", err)
			}
			antes := repo.escrituras

			in := entradaValida("") // sin contraseña: se pretende conservar la guardada
			caso.tocar(&in)
			err := svc.Guardar(context.Background(), empresaVDP, in, "u1")
			if !errors.Is(err, ErrPasswordRequeridaPorCambio) {
				t.Fatalf("se esperaba ErrPasswordRequeridaPorCambio, fue %v", err)
			}
			if repo.escrituras != antes {
				t.Fatal("no tenía que guardarse nada")
			}
		})
	}
}

// TestElSecretoNoSirveContraOtroServidor es la mitad criptográfica de lo mismo: aunque alguien
// edite la fila en la base para apuntarla a otro host, el valor guardado NO DESCIFRA. La regla de
// arriba es la puerta; esta es la cerradura.
func TestElSecretoNoSirveContraOtroServidor(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	if err := svc.Guardar(context.Background(), empresaVDP, entradaValida("la-del-buzon"), "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	// Alguien edita la fila directamente en la base y apunta el host a su servidor.
	repo.fila.Host = "smtp.atacante.com"

	_, origen, err := svc.SMTPDe(context.Background(), empresaVDP)
	if !errors.Is(err, shared.ErrNoDescifra) {
		t.Fatalf("se esperaba que NO descifrara, fue %v", err)
	}
	if origen != OrigenEmpresa {
		t.Fatalf("el origen tiene que decir contra qué se intentó: %q", origen)
	}
}

// TestElSecretoNoCruzaEmpresas: pegar en Valle de Paz la fila de Coopeprofa no entrega el secreto
// de Coopeprofa.
func TestElSecretoNoCruzaEmpresas(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	if err := svc.Guardar(context.Background(), empresaCoop, entradaValida("la-de-coopeprofa"), "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	// La MISMA fila, leída bajo el empresa_id del contexto de otra empresa.
	if _, _, err := svc.SMTPDe(context.Background(), empresaVDP); !errors.Is(err, shared.ErrNoDescifra) {
		t.Fatalf("una fila de otra empresa NO puede descifrar, fue %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// La resolución al enviar
// ─────────────────────────────────────────────────────────────────────────────

// TestSinFilaPropiaSeUsaElGlobal: es lo que hace que nada se rompa el día que se instala esto.
func TestSinFilaPropiaSeUsaElGlobal(t *testing.T) {
	svc := servicio(t, &repoFalso{}, cifradorDePrueba(t), globalDePrueba())
	cfg, origen, err := svc.SMTPDe(context.Background(), empresaVDP)
	if err != nil {
		t.Fatalf("tenía que caer al global: %v", err)
	}
	if origen != OrigenGlobal || cfg.Remitente != "cxp@valledepazcr.com" {
		t.Fatalf("origen=%q remitente=%q", origen, cfg.Remitente)
	}
}

// TestApagadaVuelveAlGlobalSinPerderLaCredencial: `activo:false` es reversible; por eso no hay
// DELETE en esta pantalla.
func TestApagadaVuelveAlGlobalSinPerderLaCredencial(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	in := entradaValida("la-del-buzon")
	in.Activo = false
	if err := svc.Guardar(context.Background(), empresaVDP, in, "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if repo.passwordGuardada == "" {
		t.Fatal("apagar NO puede destruir la credencial guardada")
	}
	_, origen, err := svc.SMTPDe(context.Background(), empresaVDP)
	if err != nil || origen != OrigenGlobal {
		t.Fatalf("apagada tenía que volver al global: origen=%q err=%v", origen, err)
	}
	cfg, err := svc.Obtener(context.Background(), empresaVDP)
	if err != nil {
		t.Fatalf("obtener: %v", err)
	}
	if !cfg.TienePassword || cfg.OrigenVigente != OrigenGlobal {
		t.Fatalf("la pantalla tiene que decir que la credencial sigue guardada y que hoy rige el global: %+v", cfg)
	}
}

// TestSinNadaConfiguradoNoSeInventaUnServidor.
func TestSinNadaConfiguradoNoSeInventaUnServidor(t *testing.T) {
	svc := servicio(t, &repoFalso{}, cifradorDePrueba(t), shared.SMTP{})
	if _, _, err := svc.SMTPDe(context.Background(), empresaVDP); !errors.Is(err, shared.ErrSMTPNoConfigurado) {
		t.Fatalf("se esperaba ErrSMTPNoConfigurado, fue %v", err)
	}
	cfg, err := svc.Obtener(context.Background(), empresaVDP)
	if err != nil {
		t.Fatalf("obtener: %v", err)
	}
	if cfg.OrigenVigente != OrigenNinguno {
		t.Fatalf("origen_vigente = %q, se esperaba %q", cfg.OrigenVigente, OrigenNinguno)
	}
}

// TestSecretoIlegibleNoCaeAlGlobal es la regla de la FALLA RUIDOSA.
//
// Si la empresa configuró su buzón y la contraseña no se puede leer (CIFRADO_SECRET ausente o
// rotada), mandar igual por el buzón global significaría que Coopeprofa le escribe a sus
// proveedores desde Valle de Paz: el defecto original, ahora silencioso. Mejor un 422 que dice qué
// pasó.
func TestSecretoIlegibleNoCaeAlGlobal(t *testing.T) {
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	if err := svc.Guardar(context.Background(), empresaVDP, entradaValida("la-del-buzon"), "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}

	// Mismo dato, servidor reiniciado sin la variable.
	sinClave := NewService(repo, nil, zap.NewNop(), nil, globalDePrueba(), false)
	cfg, origen, err := sinClave.SMTPDe(context.Background(), empresaVDP)
	if !errors.Is(err, shared.ErrClaveAusente) {
		t.Fatalf("se esperaba ErrClaveAusente, fue %v", err)
	}
	if origen == OrigenGlobal || cfg.Configurado() {
		t.Fatal("NO puede caerse al correo global: sería mandar desde el buzón de otra empresa en silencio")
	}

	// Y con una clave DISTINTA (rotación sin re-escribir las contraseñas): tampoco.
	otra, err := shared.NewCifrador("Zz9Yy8Xx7Ww6Vv5Uu4Tt3Ss2Rr1Qq0Pp9Oo8Nn7Mm6Ll5Kk4Jj3Ii2Hh1Gg0Ff9Ee")
	if err != nil {
		t.Fatalf("cifrador alterno: %v", err)
	}
	rotado := NewService(repo, nil, zap.NewNop(), otra, globalDePrueba(), false)
	if _, _, err := rotado.SMTPDe(context.Background(), empresaVDP); !errors.Is(err, shared.ErrNoDescifra) {
		t.Fatalf("se esperaba ErrNoDescifra, fue %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Probar
// ─────────────────────────────────────────────────────────────────────────────

// servidorMinimo acepta una conversación SMTP y anota a quién se le pidió entregar.
func servidorMinimo(t *testing.T) (puerto int, recibidos func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	var destinos []string
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		escribir := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		escribir("220 prueba ESMTP")
		for {
			linea, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(linea))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				escribir("250-prueba")
				escribir("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "RCPT TO"):
				i, j := strings.Index(linea, "<"), strings.LastIndex(linea, ">")
				if i >= 0 && j > i {
					mu.Lock()
					destinos = append(destinos, linea[i+1:j])
					mu.Unlock()
				}
				escribir("250 ok")
			case cmd == "DATA":
				escribir("354 dale")
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimRight(l, "\r\n") == "." {
						break
					}
				}
				escribir("250 ok")
			case cmd == "QUIT":
				escribir("221 chau")
				return
			default:
				escribir("250 ok")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), destinos...)
	}
}

// TestLaPruebaVaSiempreAlCorreoDeQuienLaPide es el hallazgo S-4.
//
// El endpoint no acepta destinatario. Uno que lo aceptara sería correo autenticado con las
// credenciales de la empresa y saliendo de su dominio —o sea, pasando SPF y DKIM—: exactamente la
// primitiva que busca un fraude de facturación contra sus 649 proveedores.
func TestLaPruebaVaSiempreAlCorreoDeQuienLaPide(t *testing.T) {
	puerto, recibidos := servidorMinimo(t)
	repo := &repoFalso{
		hay: true,
		fila: Guardado{Host: "127.0.0.1", Puerto: puerto, Seguridad: shared.SeguridadNinguno,
			Remitente: "cxp@valledepazcr.com", Activo: true},
		usuarioNombre: "Ana Rojas", usuarioEmail: "ana@valledepazcr.com",
	}
	svc := servicio(t, repo, cifradorDePrueba(t), shared.SMTP{})

	p, err := svc.Probar(context.Background(), empresaVDP, "u1")
	if err != nil {
		t.Fatalf("la prueba tenía que salir bien: %v", err)
	}
	if p.EnviadoA != "ana@valledepazcr.com" {
		t.Fatalf("la prueba fue a %q; tiene que ir al correo del propio usuario", p.EnviadoA)
	}
	destinos := recibidos()
	if len(destinos) != 1 || destinos[0] != "ana@valledepazcr.com" {
		t.Fatalf("el sobre tenía que llevar solo al usuario: %v", destinos)
	}
	if repo.pruebas != 1 || repo.detallePrueba != "" {
		t.Fatalf("una prueba OK se anota sin error: pruebas=%d detalle=%q", repo.pruebas, repo.detallePrueba)
	}
}

// TestLaPruebaSeAnotaTambienCuandoFalla: sin esto, «nunca se probó» y «se probó y falló» se ven
// igual en la pantalla.
func TestLaPruebaSeAnotaTambienCuandoFalla(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	puerto := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // nadie escuchando

	repo := &repoFalso{
		hay: true,
		fila: Guardado{Host: "127.0.0.1", Puerto: puerto, Seguridad: shared.SeguridadNinguno,
			Remitente: "cxp@valledepazcr.com", Activo: true},
		usuarioNombre: "Ana Rojas", usuarioEmail: "ana@valledepazcr.com",
	}
	svc := servicio(t, repo, cifradorDePrueba(t), shared.SMTP{})

	if _, err := svc.Probar(context.Background(), empresaVDP, "u1"); err == nil {
		t.Fatal("la prueba tenía que fallar")
	}
	if repo.pruebas != 1 {
		t.Fatalf("el fallo tenía que anotarse, pruebas=%d", repo.pruebas)
	}
	if !strings.Contains(repo.detallePrueba, shared.CategoriaHost) {
		t.Fatalf("el detalle técnico tenía que decir la categoría: %q", repo.detallePrueba)
	}
}

// TestSinCorreoDelUsuarioNoHayADondeMandarLaPrueba.
func TestSinCorreoDelUsuarioNoHayADondeMandarLaPrueba(t *testing.T) {
	repo := &repoFalso{hay: true, fila: Guardado{Host: "x", Puerto: 25, Remitente: "a@b.cr", Activo: true}}
	svc := servicio(t, repo, cifradorDePrueba(t), shared.SMTP{})
	if _, err := svc.Probar(context.Background(), empresaVDP, "u1"); !errors.Is(err, ErrSinCorreoDePrueba) {
		t.Fatalf("se esperaba ErrSinCorreoDePrueba, fue %v", err)
	}
}

// TestNoSePuedeProbarEnRafaga: freno simple para que el endpoint no sirva de generador de tráfico.
func TestNoSePuedeProbarEnRafaga(t *testing.T) {
	repo := &repoFalso{usuarioEmail: "ana@valledepazcr.com"}
	svc := servicio(t, repo, cifradorDePrueba(t), shared.SMTP{})
	_, _ = svc.Probar(context.Background(), empresaVDP, "u1") // falla por falta de servidor, pero cuenta
	if _, err := svc.Probar(context.Background(), empresaVDP, "u1"); !errors.Is(err, ErrPruebaMuySeguida) {
		t.Fatalf("se esperaba ErrPruebaMuySeguida, fue %v", err)
	}
	// Y el freno es POR EMPRESA: que una empresa pruebe no puede bloquear a otra.
	if _, err := svc.Probar(context.Background(), empresaCoop, "u1"); errors.Is(err, ErrPruebaMuySeguida) {
		t.Fatal("el freno de una empresa no puede alcanzar a otra")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Validación de borde
// ─────────────────────────────────────────────────────────────────────────────

func TestValidacionDelPUT(t *testing.T) {
	casos := []struct {
		nombre string
		tocar  func(*Input)
		quiere error
	}{
		{"sin host", func(in *Input) { in.Host = "" }, ErrHostRequerido},
		{"puerto fuera de rango", func(in *Input) { in.Puerto = 70000 }, ErrPuertoInvalido},
		{"seguridad inventada", func(in *Input) { in.Seguridad = "SSL" }, ErrSeguridadInvalida},
		{"remitente sin arroba", func(in *Input) { in.Remitente = "cxp-valledepazcr.com" }, ErrRemitenteInvalido},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			repo := &repoFalso{}
			svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
			in := entradaValida("una-clave")
			caso.tocar(&in)
			if err := svc.Guardar(context.Background(), empresaVDP, in, "u1"); !errors.Is(err, caso.quiere) {
				t.Fatalf("se esperaba %v, fue %v", caso.quiere, err)
			}
			if repo.escrituras != 0 {
				t.Fatal("no tenía que escribirse nada")
			}
		})
	}
}

// TestEnProduccionNoSePuedeApuntarALaRedInterna es el hallazgo S-5.
//
// Fuera de producción NO se bloquea, y es a propósito: el servidor por defecto de este proyecto es
// `mailhog:1025`, que vive dentro del compose y resuelve a una IP privada.
func TestEnProduccionNoSePuedeApuntarALaRedInterna(t *testing.T) {
	repo := &repoFalso{}
	prod := NewService(repo, nil, zap.NewNop(), cifradorDePrueba(t), shared.SMTP{}, true)
	in := entradaValida("una-clave")
	in.Host = "127.0.0.1"
	if err := prod.Guardar(context.Background(), empresaVDP, in, "u1"); !errors.Is(err, ErrHostNoPermitido) {
		t.Fatalf("se esperaba ErrHostNoPermitido, fue %v", err)
	}

	dev := servicio(t, &repoFalso{}, cifradorDePrueba(t), shared.SMTP{})
	inDev := entradaValida("una-clave")
	inDev.Host = "127.0.0.1"
	if err := dev.Guardar(context.Background(), empresaVDP, inDev, "u1"); err != nil {
		t.Fatalf("fuera de producción tenía que poder guardarse (MailHog): %v", err)
	}
}

// TestGuardarNoDejaUnaPruebaVieja: si cambia la configuración, el «probado y funcionando» anterior
// describe otra cosa. El repositorio la limpia; acá se deja escrito qué se espera de él.
func TestGuardarNoDejaUnaPruebaVieja(t *testing.T) {
	ayer := time.Now().Add(-24 * time.Hour)
	repo := &repoFalso{hay: true, fila: Guardado{
		Host: "smtp.office365.com", Puerto: 587, Seguridad: shared.SeguridadSTARTTLS,
		Usuario: "cxp@valledepazcr.com", PasswordCifrada: "v1.loquesea",
		Remitente: "cxp@valledepazcr.com", Activo: true, ProbadoEn: &ayer,
	}}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	if err := svc.Guardar(context.Background(), empresaVDP, entradaValida("otra-clave"), "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if repo.guardada == nil {
		t.Fatal("no se guardó nada")
	}
	if repo.guardada.ProbadoEn != nil {
		t.Fatal("el service no tiene que arrastrar la prueba anterior a la fila nueva")
	}
}
