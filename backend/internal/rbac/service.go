package rbac

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gpvdp/erp/internal/shared"
)

// cacheTTL: los permisos resueltos por (empresa, rol) se cachean brevemente para
// no golpear la BD en cada request. Editar la matriz invalida la empresa afectada,
// así que el cambio surte efecto de inmediato para esa empresa.
const cacheTTL = 60 * time.Second

type entrada struct {
	permisos map[string]struct{}
	expira   time.Time
}

// Service resuelve permisos (con caché) y administra la matriz.
type Service struct {
	repo  *Repository
	audit *shared.Audit
	mu    sync.RWMutex
	cache map[string]entrada // clave: empresaID|rolCodigo
	now   func() time.Time
}

func NewService(repo *Repository, audit *shared.Audit) *Service {
	return &Service{repo: repo, audit: audit, cache: map[string]entrada{}, now: time.Now}
}

// EnsureDefaults siembra catálogo + matriz por defecto (idempotente).
func (s *Service) EnsureDefaults(ctx context.Context) error { return s.repo.EnsureDefaults(ctx) }

// Tiene indica si (empresa, rol) tiene el permiso. ADMIN siempre true (bypass).
func (s *Service) Tiene(ctx context.Context, empresaID, rolCodigo, permiso string) (bool, error) {
	if rolCodigo == RolAdmin {
		return true, nil
	}
	set, err := s.permisosSet(ctx, empresaID, rolCodigo)
	if err != nil {
		return false, err
	}
	_, ok := set[permiso]
	return ok, nil
}

// PermisosDe devuelve la lista de permisos efectivos (para /me). ADMIN = todos.
func (s *Service) PermisosDe(ctx context.Context, empresaID, rolCodigo string) ([]string, error) {
	if rolCodigo == RolAdmin {
		return codigos(), nil
	}
	set, err := s.permisosSet(ctx, empresaID, rolCodigo)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for _, p := range Catalogo { // orden estable del catálogo
		if _, ok := set[p.Codigo]; ok {
			out = append(out, p.Codigo)
		}
	}
	return out, nil
}

func (s *Service) permisosSet(ctx context.Context, empresaID, rolCodigo string) (map[string]struct{}, error) {
	key := empresaID + "|" + rolCodigo
	s.mu.RLock()
	if e, ok := s.cache[key]; ok && s.now().Before(e.expira) {
		s.mu.RUnlock()
		return e.permisos, nil
	}
	s.mu.RUnlock()

	codes, err := s.repo.PermisosDeRol(ctx, empresaID, rolCodigo)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		set[c] = struct{}{}
	}
	s.mu.Lock()
	s.cache[key] = entrada{permisos: set, expira: s.now().Add(cacheTTL)}
	s.mu.Unlock()
	return set, nil
}

// invalidarEmpresa limpia el caché de todos los roles de una empresa.
func (s *Service) invalidarEmpresa(empresaID string) {
	s.mu.Lock()
	for k := range s.cache {
		if strings.HasPrefix(k, empresaID+"|") {
			delete(s.cache, k)
		}
	}
	s.mu.Unlock()
}

// ---- Administración de la matriz ----

func (s *Service) Catalogo() []PermisoDef { return Catalogo }

func (s *Service) Roles(ctx context.Context, empresaID string) ([]RolItem, error) {
	return s.repo.RolesVisibles(ctx, empresaID)
}

func (s *Service) Matriz(ctx context.Context, empresaID string) ([]MatrizGrant, error) {
	return s.repo.Matriz(ctx, empresaID)
}

// SetPermisosDeRol reemplaza los permisos de un rol (valida contra el catálogo).
func (s *Service) SetPermisosDeRol(ctx context.Context, empresaID, rolCodigo string, permisos []string, usuarioID string) error {
	if rolCodigo == RolAdmin {
		return ErrRolBaseProtegido // ADMIN es superusuario; su matriz no se edita
	}
	valido := map[string]struct{}{}
	for _, p := range Catalogo {
		valido[p.Codigo] = struct{}{}
	}
	for _, p := range permisos {
		if _, ok := valido[p]; !ok {
			return ErrPermisoInvalido
		}
	}
	if err := s.repo.SetPermisosDeRol(ctx, empresaID, rolCodigo, permisos); err != nil {
		return err
	}
	s.invalidarEmpresa(empresaID)
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "rol_permiso", Accion: "SET_PERMISOS_ROL", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"rol": rolCodigo, "permisos": permisos},
	})
	return nil
}

// CrearRol crea un rol a medida (código derivado del nombre) con permisos mínimos.
func (s *Service) CrearRol(ctx context.Context, empresaID, nombre, usuarioID string) (RolItem, error) {
	codigo := codigoDesdeNombre(nombre)
	it, err := s.repo.CrearRol(ctx, empresaID, codigo, nombre, PermisosRolNuevo)
	if err != nil {
		return RolItem{}, err
	}
	s.invalidarEmpresa(empresaID)
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "rol", EntidadID: &it.ID, Accion: "CREAR_ROL", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"codigo": codigo, "nombre": nombre},
	})
	return it, nil
}

// codigoDesdeNombre arma un código estable a partir del nombre ("Solo Bancos" → "CUSTOM_SOLO_BANCOS").
func codigoDesdeNombre(nombre string) string {
	up := strings.ToUpper(strings.TrimSpace(nombre))
	var b strings.Builder
	b.WriteString("CUSTOM_")
	for _, r := range up {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		}
	}
	return b.String()
}
