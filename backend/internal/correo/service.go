package correo

// La lógica del correo saliente por empresa: leer, guardar, probar y —lo que usan los demás
// módulos— RESOLVER con qué servidor se manda en el momento del envío.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// Service es la lógica del correo saliente.
type Service struct {
	repo  Repository
	audit *shared.Audit
	log   *zap.Logger
	// cif puede ser nil: es lo que pasa en una instalación sin CIFRADO_SECRET. No se aborta el
	// arranque por eso —el correo global sigue funcionando—, el fallo se cobra al guardar una
	// contraseña (422 nombrando la variable) y al enviar por una empresa que ya tiene una guardada.
	cif *shared.Cifrador
	// global es el correo del proceso (SMTP_*). Es la caída cuando la empresa no configuró el suyo.
	global shared.SMTP
	// produccion activa el guardarraíl de red interna. En desarrollo NO puede activarse: el
	// servidor por defecto de este proyecto es `mailhog:1025`, que vive dentro del compose.
	produccion bool

	// mu/ultimaPrueba: freno del endpoint de prueba, por empresa. Es estado de proceso y NO es un
	// caché de credenciales: nada de lo que hay acá se usa para enviar (ver SMTPDe).
	mu           sync.Mutex
	ultimaPrueba map[string]time.Time
}

// NewService construye el servicio.
func NewService(repo Repository, audit *shared.Audit, log *zap.Logger, cif *shared.Cifrador, global shared.SMTP, produccion bool) *Service {
	return &Service{repo: repo, audit: audit, log: log, cif: cif, global: global,
		produccion: produccion, ultimaPrueba: map[string]time.Time{}}
}

// reCorreo es el mismo formato que exige el CHECK de la base (correo_saliente_remitente_check). Se
// valida acá además de allá para que el usuario lea «el remitente no es una dirección válida» en
// vez de un error de base de datos.
var reCorreo = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// ─────────────────────────────────────────────────────────────────────────────
// Leer
// ─────────────────────────────────────────────────────────────────────────────

// Obtener devuelve lo que la pantalla puede mostrar. La contraseña NO SALE: lo único que se dice
// de ella es si hay una guardada.
func (s *Service) Obtener(ctx context.Context, empresaID string) (Config, error) {
	cfg := Config{
		Puerto:            587,
		Seguridad:         shared.SeguridadSTARTTLS,
		CifradoDisponible: s.cif.Configurado(),
	}
	g, err := s.repo.Obtener(ctx, empresaID)
	if err != nil {
		if !errors.Is(err, ErrConfigNoEncontrada) {
			return Config{}, err
		}
		// Sin fila propia la empresa no está «sin correo»: está mandando por el global. Decirlo es
		// la diferencia entre una pantalla que informa y una que asusta.
		cfg.OrigenVigente, cfg.RemitenteVigente = OrigenNinguno, ""
		if s.global.Configurado() {
			cfg.OrigenVigente, cfg.RemitenteVigente = OrigenGlobal, s.global.Remitente
		}
		return cfg, nil
	}
	cfg.Configurado = true
	cfg.Host, cfg.Puerto, cfg.Seguridad = g.Host, g.Puerto, g.Seguridad
	cfg.Usuario, cfg.TienePassword = g.Usuario, g.PasswordCifrada != ""
	cfg.Remitente, cfg.RemitenteNombre, cfg.Activo = g.Remitente, g.RemitenteNombre, g.Activo
	cfg.ProbadoEn = g.ProbadoEn
	if g.ProbadoError != "" {
		detalle := g.ProbadoError
		cfg.ProbadoError = &detalle
	}
	cfg.ActualizadoPor = g.ActualizadoPor
	actualizado := g.ActualizadoEn
	cfg.ActualizadoEn = &actualizado
	cfg.OrigenVigente, cfg.RemitenteVigente = OrigenEmpresa, g.Remitente
	if !g.Activo {
		cfg.OrigenVigente, cfg.RemitenteVigente = OrigenNinguno, ""
		if s.global.Configurado() {
			cfg.OrigenVigente, cfg.RemitenteVigente = OrigenGlobal, s.global.Remitente
		}
	}
	return cfg, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Guardar
// ─────────────────────────────────────────────────────────────────────────────

// Guardar crea o reemplaza la configuración de la empresa activa y cifra la contraseña.
//
// Es TODO O NADA: si la contraseña no se puede cifrar, no se guarda ni el resto de los campos.
// Guardar «casi todo» dejaría una configuración que parece puesta y no manda.
func (s *Service) Guardar(ctx context.Context, empresaID string, in Input, usuarioID string) error {
	g := Guardado{
		Host:            strings.TrimSpace(in.Host),
		Puerto:          in.Puerto,
		Seguridad:       strings.ToUpper(strings.TrimSpace(in.Seguridad)),
		Usuario:         strings.TrimSpace(in.Usuario),
		Remitente:       strings.TrimSpace(in.Remitente),
		RemitenteNombre: strings.TrimSpace(in.RemitenteNombre),
		Activo:          in.Activo,
	}
	if g.Puerto == 0 {
		g.Puerto = 587
	}
	if g.Seguridad == "" {
		g.Seguridad = shared.SeguridadSTARTTLS
	}
	if g.Host == "" {
		return ErrHostRequerido
	}
	if g.Puerto < 1 || g.Puerto > 65535 {
		return ErrPuertoInvalido
	}
	switch g.Seguridad {
	case shared.SeguridadNinguno, shared.SeguridadSTARTTLS, shared.SeguridadTLS:
	default:
		return ErrSeguridadInvalida
	}
	if !reCorreo.MatchString(g.Remitente) {
		return ErrRemitenteInvalido
	}
	if err := s.validarHost(ctx, g.Host); err != nil {
		return err
	}

	actual, err := s.repo.Obtener(ctx, empresaID)
	if err != nil && !errors.Is(err, ErrConfigNoEncontrada) {
		return err
	}

	cifrada, err := s.resolverPassword(empresaID, in, actual, g)
	if err != nil {
		return err
	}
	// Sin usuario no hay autenticación, así que una contraseña guardada sería un secreto huérfano:
	// se descarta en vez de quedar dando vueltas en la base.
	if g.Usuario == "" {
		cifrada = ""
	}
	if g.Usuario != "" && cifrada == "" {
		return ErrPasswordRequerida
	}

	if err := s.repo.Guardar(ctx, empresaID, g, cifrada, usuarioID); err != nil {
		return err
	}
	// La auditoría registra QUÉ se configuró y si la contraseña cambió — nunca su valor.
	// `auditoria_evento` es append-only: un secreto escrito ahí no se puede ni borrar.
	if s.audit != nil {
		s.audit.Registrar(ctx, shared.Evento{
			EmpresaID: &empresaID, Entidad: "correo_saliente", Accion: "GUARDAR_CORREO_SALIENTE",
			UsuarioID: &usuarioID,
			ValorNuevo: map[string]any{
				"host": g.Host, "puerto": g.Puerto, "seguridad": g.Seguridad, "usuario": g.Usuario,
				"remitente": g.Remitente, "activo": g.Activo,
				"password_cambiada": in.Password != nil,
			},
		})
	}
	return nil
}

// resolverPassword decide qué blob se guarda, con las tres semánticas del campo `password`.
func (s *Service) resolverPassword(empresaID string, in Input, actual, nuevo Guardado) (string, error) {
	if in.Password == nil {
		// Conservar. Pero solo si el servidor es EL MISMO: la contraseña guardada está atada a
		// (empresa, host, puerto, usuario) y cambiar cualquiera de esos la vuelve indescifrable a
		// propósito. Ver ErrPasswordRequeridaPorCambio.
		if actual.PasswordCifrada == "" {
			return "", nil
		}
		if cambioDeServidor(actual, nuevo) {
			return "", ErrPasswordRequeridaPorCambio
		}
		return actual.PasswordCifrada, nil
	}
	if in.Password.Vacio() {
		return "", nil // borrar: queda sin autenticación
	}
	if !s.cif.Configurado() {
		// 422 nombrando la variable, NUNCA un 500, y sobre todo NUNCA guardar en claro.
		return "", fmt.Errorf("correo: no se puede guardar la contraseña del correo: %w", shared.ErrClaveAusente)
	}
	cifrada, err := s.cif.Cifrar(*in.Password, aad(empresaID, nuevo.Host, nuevo.Puerto, nuevo.Usuario)...)
	if err != nil {
		return "", fmt.Errorf("correo: cifrar la contraseña del correo: %w", err)
	}
	return cifrada, nil
}

// cambioDeServidor dice si lo que se está guardando apunta a otro lado que lo guardado.
//
// `seguridad` entra en la comparación aunque no forme parte del atado criptográfico: pasar de
// STARTTLS a NINGUNO es pedirle al sistema que presente la misma contraseña por un canal sin
// cifrar, y eso también exige que alguien la vuelva a escribir a conciencia.
func cambioDeServidor(actual, nuevo Guardado) bool {
	return !strings.EqualFold(actual.Host, nuevo.Host) ||
		actual.Puerto != nuevo.Puerto ||
		actual.Usuario != nuevo.Usuario ||
		actual.Seguridad != nuevo.Seguridad
}

// aad arma el contexto autenticado del cifrado. El PRIMER elemento es siempre el empresa_id DEL
// CONTEXTO de la petición (el que el token acuñó contra la membresía), nunca el que el repositorio
// leyó de la propia fila. (Hallazgo S-7.)
func aad(empresaID, host string, puerto int, usuario string) []string {
	return []string{empresaID, strings.ToLower(host), strconv.Itoa(puerto), usuario}
}

// validarHost impide, EN PRODUCCIÓN, que el correo se apunte a la red interna del servidor.
//
// Sin esto, el par «guardar + probar» convierte el backend en un escáner de puertos de su propia
// red: la diferencia entre «conexión rechazada» y «tiempo agotado» ya es información. No se aplica
// fuera de producción porque el servidor por defecto de este proyecto es `mailhog:1025`, que vive
// dentro del compose y tiene IP privada. (Hallazgo S-5.)
func (s *Service) validarHost(ctx context.Context, host string) error {
	if !s.produccion {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if ipInterna(ip) {
			return ErrHostNoPermitido
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		// Un host que no resuelve no se bloquea acá: el envío va a fallar solo, con
		// HOST_INALCANZABLE, y ahí el mensaje es más útil que «no permitido».
		return nil
	}
	for _, ip := range ips {
		if ipInterna(ip.IP) {
			return ErrHostNoPermitido
		}
	}
	return nil
}

func ipInterna(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// ─────────────────────────────────────────────────────────────────────────────
// Resolver (lo que usan los demás módulos al enviar)
// ─────────────────────────────────────────────────────────────────────────────

// SMTPDe devuelve el servidor con el que esta empresa manda AHORA, y de dónde salió.
//
// Se resuelve en cada envío, sin caché: un cliente o un mailer memorizado en el proceso y mal
// llaveado mandaría el correo de Memorial Pets con las credenciales y el remitente de Valle de
// Paz, que es el mismo defecto que la migración 0084 viene a arreglar, reintroducido por la puerta
// de atrás. Son tres empresas y envíos manuales: no hay nada que optimizar. (Hallazgo S-8.)
//
// El `origen` vuelve incluso cuando hay error, porque la bitácora tiene que poder decir CONTRA QUÉ
// se intentó.
func (s *Service) SMTPDe(ctx context.Context, empresaID string) (shared.SMTP, string, error) {
	g, err := s.repo.Obtener(ctx, empresaID)
	if errors.Is(err, ErrConfigNoEncontrada) || (err == nil && !g.Activo) {
		// La empresa no configuró el suyo (o lo apagó): rige el global. Apagar es reversible y no
		// destruye la credencial guardada; por eso no hay DELETE en esta pantalla.
		if !s.global.Configurado() {
			return shared.SMTP{}, OrigenGlobal, shared.ErrSMTPNoConfigurado
		}
		return s.global, OrigenGlobal, nil
	}
	if err != nil {
		return shared.SMTP{}, OrigenGlobal, err
	}

	cfg := shared.SMTP{
		Host: g.Host, Puerto: g.Puerto, Seguridad: g.Seguridad, Usuario: g.Usuario,
		Remitente: g.Remitente, RemitenteNombre: g.RemitenteNombre,
	}
	if g.PasswordCifrada != "" {
		// FALLA RUIDOSA, NUNCA CAÍDA SILENCIOSA AL GLOBAL. Si la empresa configuró su buzón y la
		// contraseña no se puede leer, mandar igual por el buzón global significaría que Coopeprofa
		// le escribe a sus proveedores desde Valle de Paz — el bug original, ahora en silencio y sin
		// que nadie lo note. Mejor un 422 que dice exactamente qué pasó.
		if !s.cif.Configurado() {
			return shared.SMTP{}, OrigenEmpresa,
				fmt.Errorf("correo: la empresa tiene una contraseña guardada y no se puede leer: %w", shared.ErrClaveAusente)
		}
		clara, err := s.cif.Descifrar(g.PasswordCifrada, aad(empresaID, g.Host, g.Puerto, g.Usuario)...)
		if err != nil {
			return shared.SMTP{}, OrigenEmpresa,
				fmt.Errorf("correo: la contraseña guardada del correo de esta empresa no se puede leer: %w", err)
		}
		cfg.Password = clara
	}
	return cfg, OrigenEmpresa, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Probar
// ─────────────────────────────────────────────────────────────────────────────

// Probar manda un correo de prueba AL CORREO DE QUIEN LA PIDE.
//
// El destino no se elige, y es una decisión de seguridad: un endpoint que manda a una dirección
// arbitraria, autenticado con las credenciales de la empresa y desde su dominio, es correo que
// pasa SPF y DKIM de la empresa — exactamente la primitiva que busca un fraude de facturación
// contra sus proveedores. Mandándolo siempre a quien aprieta el botón se comprueba lo mismo (que
// las credenciales sirven) y el relay desaparece. (Hallazgo S-4.)
//
// Prueba LO GUARDADO, no lo que hay en el formulario: la contraseña almacenada es la que se va a
// usar de verdad al enviar. El flujo es guardar y después probar.
func (s *Service) Probar(ctx context.Context, empresaID, usuarioID string) (Prueba, error) {
	if err := s.frenar(empresaID); err != nil {
		return Prueba{}, err
	}
	nombre, destino, err := s.repo.CorreoUsuario(ctx, usuarioID)
	if err != nil {
		return Prueba{}, err
	}
	if strings.TrimSpace(destino) == "" {
		return Prueba{}, ErrSinCorreoDePrueba
	}

	cfg, origen, err := s.SMTPDe(ctx, empresaID)
	if err != nil {
		s.anotarPrueba(ctx, empresaID, origen, err)
		return Prueba{}, err
	}
	asunto, cuerpo := textoDePrueba(nombre, cfg, origen)
	if err := shared.Enviar(ctx, cfg, shared.Sobre{Para: destino, Asunto: asunto, Cuerpo: cuerpo}); err != nil {
		s.anotarPrueba(ctx, empresaID, origen, err)
		return Prueba{}, err
	}
	s.anotarPrueba(ctx, empresaID, origen, nil)
	return Prueba{
		OK: true, Servidor: cfg.Addr(), Remitente: cfg.Remitente, Origen: origen, EnviadoA: destino,
		ProbadoEn:   time.Now().Format(time.RFC3339),
		Descripcion: "se envió un correo de prueba a tu propia dirección",
	}, nil
}

// frenar aplica la espera entre pruebas por empresa.
func (s *Service) frenar(empresaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ultima, ok := s.ultimaPrueba[empresaID]; ok && time.Since(ultima) < EsperaEntrePruebas {
		return ErrPruebaMuySeguida
	}
	s.ultimaPrueba[empresaID] = time.Now()
	return nil
}

// anotarPrueba guarda el resultado en la fila de la empresa. Si el correo salió por el global no
// hay fila que anotar, y no se inventa una: la configuración global no es de ninguna empresa.
//
// El DETALLE TÉCNICO (categoría, código SMTP y servidor) se guarda acá y solo acá, porque esta
// tabla se lee detrás de admin.correo. Nunca el texto crudo del servidor: ese no sale del
// transporte. (Hallazgos S-2 y S-3.)
func (s *Service) anotarPrueba(ctx context.Context, empresaID, origen string, errEnvio error) {
	if origen != OrigenEmpresa {
		return
	}
	detalle := ""
	if errEnvio != nil {
		detalle = detalleDe(errEnvio)
	}
	if err := s.repo.MarcarPrueba(ctx, empresaID, detalle); err != nil && s.log != nil {
		s.log.Error("correo: no se pudo anotar el resultado de la prueba", zap.Error(err))
	}
}

// detalleDe arma el texto técnico del fallo. Un ErrorSMTP trae su propio detalle; cualquier otro
// error de este paquete es un centinela con texto propio, que tampoco contiene secretos.
func detalleDe(err error) string {
	var e *shared.ErrorSMTP
	if errors.As(err, &e) {
		return e.Detalle()
	}
	return err.Error()
}

// textoDePrueba arma el correo de prueba. Es fijo y vive en el código a propósito: no es una
// comunicación con un tercero, así que no es una plantilla de `plantilla_correo`.
func textoDePrueba(nombre string, cfg shared.SMTP, origen string) (string, string) {
	asunto := "Prueba del correo saliente"
	var b strings.Builder
	if nombre != "" {
		fmt.Fprintf(&b, "Hola %s:\n\n", nombre)
	}
	b.WriteString("Si estás leyendo este correo, el servidor de salida quedó bien configurado.\n\n")
	fmt.Fprintf(&b, "Servidor: %s\n", cfg.Addr())
	fmt.Fprintf(&b, "Remitente: %s\n", cfg.Remitente)
	fmt.Fprintf(&b, "Origen de la configuración: %s\n", origen)
	b.WriteString("\nEste mensaje lo generó el sistema a pedido tuyo. No hace falta responderlo.\n")
	return asunto, b.String()
}
