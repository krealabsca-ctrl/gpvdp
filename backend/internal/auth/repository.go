package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RefreshRecord es el registro persistido de un refresh token (solo su hash).
type RefreshRecord struct {
	ID        string
	UsuarioID string
	Revocado  bool
	ExpiraEn  time.Time
}

// Repository abstrae el acceso a datos de identidad/sesión (permite fakes en tests).
type Repository interface {
	UsuarioByEmail(ctx context.Context, email string) (Usuario, error)
	UsuarioByID(ctx context.Context, id string) (Usuario, error)
	ActualizarPassword(ctx context.Context, usuarioID, hash string, debeCambiar bool) error
	Memberships(ctx context.Context, usuarioID string) ([]Membership, error)
	Membership(ctx context.Context, usuarioID, empresaID string) (Membership, error)
	CrearRefresh(ctx context.Context, usuarioID, tokenHash string, expira time.Time) error
	RefreshPorHash(ctx context.Context, tokenHash string) (RefreshRecord, error)
	// RevocarRefresh revoca la sesión y devuelve cuántas filas cambió (0 si ya estaba revocada).
	RevocarRefresh(ctx context.Context, id string) (int64, error)
	// RevocarSesionesDeUsuario revoca TODAS las sesiones vigentes del usuario (al cambiar la
	// contraseña se cierran las demás sesiones abiertas: si la cuenta estaba comprometida, el
	// atacante pierde el refresh que tenía). Devuelve cuántas revocó.
	RevocarSesionesDeUsuario(ctx context.Context, usuarioID string) (int64, error)
	// RegistrarIntentoFallido incrementa el contador de fallos consecutivos y, al llegar al
	// umbral, fija la ventana de bloqueo. Devuelve el estado resultante para decidir la respuesta.
	RegistrarIntentoFallido(ctx context.Context, usuarioID string, umbral int, ventana time.Duration) (intentos int, bloqueadoHasta *time.Time, err error)
	// LimpiarIntentos pone el contador a 0 y quita el bloqueo (login exitoso).
	LimpiarIntentos(ctx context.Context, usuarioID string) error
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository crea el repository respaldado por PostgreSQL (pgx).
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func (r *pgRepository) UsuarioByEmail(ctx context.Context, email string) (Usuario, error) {
	const q = `SELECT id::text, nombre, email, password_hash, activo, debe_cambiar_password, intentos_fallidos, bloqueado_hasta FROM usuario WHERE email = $1`
	var u Usuario
	err := r.pool.QueryRow(ctx, q, email).Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.Activo, &u.DebeCambiarPassword, &u.IntentosFallidos, &u.BloqueadoHasta)
	if errors.Is(err, pgx.ErrNoRows) {
		return Usuario{}, ErrCredenciales
	}
	if err != nil {
		return Usuario{}, fmt.Errorf("auth: usuario por email: %w", err)
	}
	return u, nil
}

func (r *pgRepository) UsuarioByID(ctx context.Context, id string) (Usuario, error) {
	const q = `SELECT id::text, nombre, email, password_hash, activo, debe_cambiar_password, intentos_fallidos, bloqueado_hasta FROM usuario WHERE id = $1::uuid`
	var u Usuario
	err := r.pool.QueryRow(ctx, q, id).Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.Activo, &u.DebeCambiarPassword, &u.IntentosFallidos, &u.BloqueadoHasta)
	if errors.Is(err, pgx.ErrNoRows) {
		return Usuario{}, ErrCredenciales
	}
	if err != nil {
		return Usuario{}, fmt.Errorf("auth: usuario por id: %w", err)
	}
	return u, nil
}

// ActualizarPassword fija un nuevo hash y la bandera de cambio obligatorio.
func (r *pgRepository) ActualizarPassword(ctx context.Context, usuarioID, hash string, debeCambiar bool) error {
	const q = `UPDATE usuario SET password_hash = $2, debe_cambiar_password = $3, actualizado_en = now() WHERE id = $1::uuid`
	if _, err := r.pool.Exec(ctx, q, usuarioID, hash, debeCambiar); err != nil {
		return fmt.Errorf("auth: actualizar password: %w", err)
	}
	return nil
}

func (r *pgRepository) Memberships(ctx context.Context, usuarioID string) ([]Membership, error) {
	const q = `
		SELECT uer.empresa_id::text, e.nombre, uer.rol_id::text, r.codigo
		FROM usuario_empresa_rol uer
		JOIN empresa e ON e.id = uer.empresa_id
		JOIN rol r ON r.id = uer.rol_id
		WHERE uer.usuario_id = $1::uuid AND e.activo = true
		ORDER BY e.nombre`
	rows, err := r.pool.Query(ctx, q, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("auth: memberships: %w", err)
	}
	defer rows.Close()

	var out []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.EmpresaID, &m.EmpresaNombre, &m.RolID, &m.RolCodigo); err != nil {
			return nil, fmt.Errorf("auth: scan membership: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: iterar memberships: %w", err)
	}
	return out, nil
}

func (r *pgRepository) Membership(ctx context.Context, usuarioID, empresaID string) (Membership, error) {
	const q = `
		SELECT uer.empresa_id::text, e.nombre, uer.rol_id::text, r.codigo
		FROM usuario_empresa_rol uer
		JOIN empresa e ON e.id = uer.empresa_id
		JOIN rol r ON r.id = uer.rol_id
		WHERE uer.usuario_id = $1::uuid AND uer.empresa_id = $2::uuid AND e.activo = true`
	var m Membership
	err := r.pool.QueryRow(ctx, q, usuarioID, empresaID).
		Scan(&m.EmpresaID, &m.EmpresaNombre, &m.RolID, &m.RolCodigo)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, ErrSinAcceso
	}
	if err != nil {
		return Membership{}, fmt.Errorf("auth: membership: %w", err)
	}
	return m, nil
}

func (r *pgRepository) CrearRefresh(ctx context.Context, usuarioID, tokenHash string, expira time.Time) error {
	const q = `INSERT INTO sesion (usuario_id, token_hash, expira_en) VALUES ($1::uuid, $2, $3)`
	if _, err := r.pool.Exec(ctx, q, usuarioID, tokenHash, expira); err != nil {
		return fmt.Errorf("auth: crear refresh: %w", err)
	}
	return nil
}

func (r *pgRepository) RefreshPorHash(ctx context.Context, tokenHash string) (RefreshRecord, error) {
	const q = `SELECT id::text, usuario_id::text, revocado, expira_en FROM sesion WHERE token_hash = $1`
	var rec RefreshRecord
	err := r.pool.QueryRow(ctx, q, tokenHash).Scan(&rec.ID, &rec.UsuarioID, &rec.Revocado, &rec.ExpiraEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshRecord{}, ErrRefreshInvalido
	}
	if err != nil {
		return RefreshRecord{}, fmt.Errorf("auth: refresh por hash: %w", err)
	}
	return rec, nil
}

func (r *pgRepository) RevocarRefresh(ctx context.Context, id string) (int64, error) {
	// Condicional (AND revocado = false) para que la rotación sea atómica ante concurrencia.
	const q = `UPDATE sesion SET revocado = true WHERE id = $1::uuid AND revocado = false`
	tag, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return 0, fmt.Errorf("auth: revocar refresh: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *pgRepository) RevocarSesionesDeUsuario(ctx context.Context, usuarioID string) (int64, error) {
	const q = `UPDATE sesion SET revocado = true WHERE usuario_id = $1::uuid AND revocado = false`
	tag, err := r.pool.Exec(ctx, q, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("auth: revocar sesiones del usuario: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *pgRepository) RegistrarIntentoFallido(ctx context.Context, usuarioID string, umbral int, ventana time.Duration) (int, *time.Time, error) {
	// Incremento atómico. Al alcanzar el umbral de fallos consecutivos se fija la ventana de
	// bloqueo; por debajo del umbral, bloqueado_hasta se conserva (arranca en NULL).
	const q = `
		UPDATE usuario
		   SET intentos_fallidos = intentos_fallidos + 1,
		       bloqueado_hasta = CASE
		           WHEN intentos_fallidos + 1 >= $2 THEN now() + make_interval(secs => $3)
		           ELSE bloqueado_hasta
		       END,
		       actualizado_en = now()
		 WHERE id = $1::uuid
		RETURNING intentos_fallidos, bloqueado_hasta`
	var intentos int
	var hasta *time.Time
	if err := r.pool.QueryRow(ctx, q, usuarioID, umbral, ventana.Seconds()).Scan(&intentos, &hasta); err != nil {
		return 0, nil, fmt.Errorf("auth: registrar intento fallido: %w", err)
	}
	return intentos, hasta, nil
}

func (r *pgRepository) LimpiarIntentos(ctx context.Context, usuarioID string) error {
	// Solo escribe si había algo que limpiar (evita un UPDATE inútil en cada login normal).
	const q = `
		UPDATE usuario SET intentos_fallidos = 0, bloqueado_hasta = NULL, actualizado_en = now()
		 WHERE id = $1::uuid AND (intentos_fallidos <> 0 OR bloqueado_hasta IS NOT NULL)`
	if _, err := r.pool.Exec(ctx, q, usuarioID); err != nil {
		return fmt.Errorf("auth: limpiar intentos: %w", err)
	}
	return nil
}
