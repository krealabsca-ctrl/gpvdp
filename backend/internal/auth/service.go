package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Service concentra la lógica de autenticación y selección de empresa.
// No conoce Gin ni HTTP (arquitectura por capas).
type Service struct {
	repo       Repository
	secret     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewService construye el servicio de auth con sus dependencias explícitas.
func NewService(repo Repository, secret string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{repo: repo, secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// Parámetros del bloqueo por cuenta ante fuerza bruta (complementa el límite por IP).
const (
	// maxIntentosLogin: fallos CONSECUTIVOS antes de bloquear la cuenta.
	maxIntentosLogin = 10
	// ventanaBloqueo: cuánto dura el bloqueo una vez alcanzado el umbral.
	ventanaBloqueo = 15 * time.Minute
)

// Login valida credenciales y emite tokens. El access token de login NO trae empresa.
//
// Seguridad:
//   - Timing uniforme: todos los caminos de fallo pagan un bcrypt (real o dummy), así el
//     tiempo de respuesta no revela si el email existe (anti-enumeración).
//   - Bloqueo por cuenta: tras `maxIntentosLogin` fallos seguidos, la cuenta queda bloqueada
//     `ventanaBloqueo`; un login exitoso reinicia el contador.
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	u, err := s.repo.UsuarioByEmail(ctx, email)
	if err != nil {
		VerifyDummy(password) // iguala el tiempo con el de un usuario real (usuario inexistente)
		return LoginResult{}, ErrCredenciales
	}
	// Cuenta bloqueada: se rechaza aunque la contraseña fuera correcta. Se paga el bcrypt dummy
	// para no distinguir por timing de otros caminos.
	if u.BloqueadoHasta != nil && u.BloqueadoHasta.After(time.Now()) {
		VerifyDummy(password)
		return LoginResult{}, ErrCuentaBloqueada
	}
	if !u.Activo {
		VerifyDummy(password) // no se revela el estado de la cuenta por timing ni por mensaje
		return LoginResult{}, ErrCredenciales
	}
	if !VerifyPassword(u.PasswordHash, password) {
		intentos, hasta, rerr := s.repo.RegistrarIntentoFallido(ctx, u.ID, maxIntentosLogin, ventanaBloqueo)
		if rerr == nil && hasta != nil && hasta.After(time.Now()) && intentos >= maxIntentosLogin {
			return LoginResult{}, ErrCuentaBloqueada
		}
		return LoginResult{}, ErrCredenciales
	}
	// Login correcto: se limpia cualquier bloqueo o contador de fallos previo.
	if err := s.repo.LimpiarIntentos(ctx, u.ID); err != nil {
		return LoginResult{}, err
	}

	memberships, err := s.repo.Memberships(ctx, u.ID)
	if err != nil {
		return LoginResult{}, err
	}

	access, err := MintAccessToken(s.secret, s.accessTTL, u.ID, u.Email, "", "")
	if err != nil {
		return LoginResult{}, err
	}
	refresh, err := s.nuevoRefresh(ctx, u.ID)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{AccessToken: access, RefreshToken: refresh, Usuario: u, Empresas: memberships}, nil
}

// SelectEmpresa emite un access token scopeado a una empresa, verificando pertenencia.
func (s *Service) SelectEmpresa(ctx context.Context, usuarioID, empresaID string) (string, Membership, error) {
	m, err := s.repo.Membership(ctx, usuarioID, empresaID)
	if err != nil {
		return "", Membership{}, err // ErrSinAcceso cuando no pertenece
	}
	u, err := s.repo.UsuarioByID(ctx, usuarioID)
	if err != nil {
		return "", Membership{}, err
	}
	if !u.Activo {
		return "", Membership{}, ErrUsuarioInactivo
	}
	access, err := MintAccessToken(s.secret, s.accessTTL, usuarioID, u.Email, m.EmpresaID, m.RolCodigo)
	if err != nil {
		return "", Membership{}, err
	}
	return access, m, nil
}

// Refresh rota el refresh token y emite un nuevo access token.
// Si empresaID != "", el nuevo access token queda scopeado (verifica pertenencia).
func (s *Service) Refresh(ctx context.Context, refreshToken, empresaID string) (string, string, error) {
	rec, err := s.repo.RefreshPorHash(ctx, hashToken(refreshToken))
	if err != nil {
		return "", "", err // ErrRefreshInvalido
	}
	if rec.Revocado || rec.ExpiraEn.Before(time.Now()) {
		return "", "", ErrRefreshInvalido
	}

	u, err := s.repo.UsuarioByID(ctx, rec.UsuarioID)
	if err != nil {
		// Usuario borrado con refresh aún vigente => sesión inválida (no error interno).
		if errors.Is(err, ErrCredenciales) {
			return "", "", ErrRefreshInvalido
		}
		return "", "", err
	}
	if !u.Activo {
		return "", "", ErrUsuarioInactivo
	}

	empClaim, rolClaim := "", ""
	if empresaID != "" {
		m, err := s.repo.Membership(ctx, rec.UsuarioID, empresaID)
		if err != nil {
			return "", "", err // ErrSinAcceso
		}
		empClaim, rolClaim = m.EmpresaID, m.RolCodigo
	}

	// Rotación atómica: revoca el viejo condicionalmente. Si otra petición ya lo
	// rotó (filas afectadas != 1), se trata como reuso y no se emite token nuevo (evita TOCTOU).
	n, err := s.repo.RevocarRefresh(ctx, rec.ID)
	if err != nil {
		return "", "", err
	}
	if n != 1 {
		return "", "", ErrRefreshInvalido
	}
	newRefresh, err := s.nuevoRefresh(ctx, rec.UsuarioID)
	if err != nil {
		return "", "", err
	}
	access, err := MintAccessToken(s.secret, s.accessTTL, u.ID, u.Email, empClaim, rolClaim)
	if err != nil {
		return "", "", err
	}
	return access, newRefresh, nil
}

// Me devuelve el estado de la sesión actual a partir de los claims.
func (s *Service) Me(ctx context.Context, claims *Claims) (MeResult, error) {
	u, err := s.repo.UsuarioByID(ctx, claims.UsuarioID())
	if err != nil {
		return MeResult{}, err
	}
	memberships, err := s.repo.Memberships(ctx, claims.UsuarioID())
	if err != nil {
		return MeResult{}, err
	}
	return MeResult{
		Usuario:             u,
		Empresas:            memberships,
		EmpresaActivaID:     claims.EmpresaID,
		Rol:                 claims.Rol,
		DebeCambiarPassword: u.DebeCambiarPassword,
	}, nil
}

// CambiarPassword cambia la contraseña del usuario autenticado, verificando la actual.
// Al hacerlo se limpia la bandera de cambio obligatorio.
func (s *Service) CambiarPassword(ctx context.Context, usuarioID, actual, nueva string) error {
	u, err := s.repo.UsuarioByID(ctx, usuarioID)
	if err != nil {
		return err
	}
	if !VerifyPassword(u.PasswordHash, actual) {
		return ErrCredenciales
	}
	if len(nueva) < 8 {
		return ErrPasswordDebil
	}
	hash, err := HashPassword(nueva)
	if err != nil {
		return err
	}
	if err := s.repo.ActualizarPassword(ctx, usuarioID, hash, false); err != nil {
		return err
	}
	// Al cambiar la contraseña se revocan las DEMÁS sesiones abiertas: si la cuenta estaba
	// comprometida, el atacante pierde el refresh token que tuviera. La sesión actual sigue con
	// su access token vigente (≤ ACCESS_TTL) y luego tendrá que volver a iniciar sesión.
	if _, err := s.repo.RevocarSesionesDeUsuario(ctx, usuarioID); err != nil {
		return err
	}
	return nil
}

// nuevoRefresh genera, persiste y devuelve un refresh token en claro (se guarda solo su hash).
func (s *Service) nuevoRefresh(ctx context.Context, usuarioID string) (string, error) {
	token, hash, err := generarRefreshToken()
	if err != nil {
		return "", err
	}
	if err := s.repo.CrearRefresh(ctx, usuarioID, hash, time.Now().Add(s.refreshTTL)); err != nil {
		return "", err
	}
	return token, nil
}

func generarRefreshToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: generar refresh: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
