package rbac

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errores de dominio RBAC.
var (
	ErrRolNoEncontrado  = errors.New("rbac: rol no encontrado")
	ErrRolDuplicado     = errors.New("rbac: ya existe un rol con ese nombre")
	ErrPermisoInvalido  = errors.New("rbac: permiso desconocido")
	ErrRolBaseProtegido = errors.New("rbac: los roles base no se pueden eliminar")
)

// Repository es el acceso a datos de RBAC.
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// PermisosDeRol devuelve los códigos de permiso concedidos a (empresa, rolCodigo).
func (r *Repository) PermisosDeRol(ctx context.Context, empresaID, rolCodigo string) ([]string, error) {
	const q = `
		SELECT p.codigo
		FROM rol_permiso rp
		JOIN rol r ON r.id = rp.rol_id
		JOIN permiso p ON p.id = rp.permiso_id
		WHERE rp.empresa_id = $1::uuid AND r.codigo = $2`
	rows, err := r.pool.Query(ctx, q, empresaID, rolCodigo)
	if err != nil {
		return nil, fmt.Errorf("rbac: permisos de rol: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("rbac: scan permiso: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RolItem es un rol para la matriz (base o a medida de la empresa activa).
type RolItem struct {
	ID      string `json:"id"`
	Codigo  string `json:"codigo"`
	Nombre  string `json:"nombre"`
	EsBase  bool   `json:"es_base"`
	EsAdmin bool   `json:"es_admin"`
}

// RolesVisibles devuelve los roles base (empresa_id NULL) + los a medida de la empresa.
func (r *Repository) RolesVisibles(ctx context.Context, empresaID string) ([]RolItem, error) {
	const q = `
		SELECT id::text, codigo, nombre, (empresa_id IS NULL)
		FROM rol
		WHERE empresa_id IS NULL OR empresa_id = $1::uuid
		ORDER BY (empresa_id IS NULL) DESC, nombre`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("rbac: roles visibles: %w", err)
	}
	defer rows.Close()
	var out []RolItem
	for rows.Next() {
		var it RolItem
		if err := rows.Scan(&it.ID, &it.Codigo, &it.Nombre, &it.EsBase); err != nil {
			return nil, fmt.Errorf("rbac: scan rol: %w", err)
		}
		it.EsAdmin = it.Codigo == RolAdmin
		out = append(out, it)
	}
	return out, rows.Err()
}

// MatrizGrant es una concesión (rol × permiso) de la empresa.
type MatrizGrant struct {
	RolCodigo     string `json:"rol_codigo"`
	PermisoCodigo string `json:"permiso_codigo"`
}

// Matriz devuelve todas las concesiones de la empresa (para pintar la matriz).
func (r *Repository) Matriz(ctx context.Context, empresaID string) ([]MatrizGrant, error) {
	const q = `
		SELECT r.codigo, p.codigo
		FROM rol_permiso rp
		JOIN rol r ON r.id = rp.rol_id
		JOIN permiso p ON p.id = rp.permiso_id
		WHERE rp.empresa_id = $1::uuid`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("rbac: matriz: %w", err)
	}
	defer rows.Close()
	var out []MatrizGrant
	for rows.Next() {
		var g MatrizGrant
		if err := rows.Scan(&g.RolCodigo, &g.PermisoCodigo); err != nil {
			return nil, fmt.Errorf("rbac: scan grant: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SetPermisosDeRol reemplaza el conjunto de permisos de un rol en una empresa
// (transaccional: borra los actuales e inserta los nuevos válidos).
func (r *Repository) SetPermisosDeRol(ctx context.Context, empresaID, rolCodigo string, permisos []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rbac: begin set permisos: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var rolID string
	err = tx.QueryRow(ctx,
		`SELECT id::text FROM rol WHERE codigo = $1 AND (empresa_id IS NULL OR empresa_id = $2::uuid)`,
		rolCodigo, empresaID).Scan(&rolID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRolNoEncontrado
	}
	if err != nil {
		return fmt.Errorf("rbac: buscar rol: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM rol_permiso WHERE empresa_id = $1::uuid AND rol_id = $2::uuid`, empresaID, rolID); err != nil {
		return fmt.Errorf("rbac: limpiar permisos: %w", err)
	}
	const ins = `
		INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
		SELECT $1::uuid, $2::uuid, p.id FROM permiso p WHERE p.codigo = $3`
	for _, code := range permisos {
		if _, err := tx.Exec(ctx, ins, empresaID, rolID, code); err != nil {
			return fmt.Errorf("rbac: conceder %s: %w", code, err)
		}
	}
	return tx.Commit(ctx)
}

// CrearRol crea un rol a medida para la empresa y le da los permisos mínimos.
func (r *Repository) CrearRol(ctx context.Context, empresaID, codigo, nombre string, permisos []string) (RolItem, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RolItem{}, fmt.Errorf("rbac: begin crear rol: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO rol (codigo, nombre, empresa_id) VALUES ($1, $2, $3::uuid) RETURNING id::text`,
		codigo, nombre, empresaID).Scan(&id)
	if esUnica(err) {
		return RolItem{}, ErrRolDuplicado
	}
	if err != nil {
		return RolItem{}, fmt.Errorf("rbac: crear rol: %w", err)
	}
	for _, code := range permisos {
		if _, err := tx.Exec(ctx,
			`INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id) SELECT $1::uuid, $2::uuid, p.id FROM permiso p WHERE p.codigo = $3`,
			empresaID, id, code); err != nil {
			return RolItem{}, fmt.Errorf("rbac: permiso inicial: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RolItem{}, fmt.Errorf("rbac: commit crear rol: %w", err)
	}
	return RolItem{ID: id, Codigo: codigo, Nombre: nombre, EsBase: false}, nil
}

// EnsureDefaults siembra el catálogo de permisos y la matriz por defecto de los
// roles base, para cada empresa, de forma idempotente (nunca pisa ediciones).
func (r *Repository) EnsureDefaults(ctx context.Context) error {
	// 1) Catálogo de permisos (upsert por código; actualiza metadatos).
	for i, p := range Catalogo {
		if _, err := r.pool.Exec(ctx, `
			INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico, orden)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (codigo) DO UPDATE
			SET modulo = EXCLUDED.modulo, nombre = EXCLUDED.nombre,
			    descripcion = EXCLUDED.descripcion, critico = EXCLUDED.critico, orden = EXCLUDED.orden`,
			p.Codigo, p.Modulo, p.Nombre, p.Descripcion, p.Critico, i); err != nil {
			return fmt.Errorf("rbac: sembrar permiso %s: %w", p.Codigo, err)
		}
	}
	// 2) Matriz por defecto por empresa × rol base (solo si la empresa aún no tiene
	//    NINGUNA concesión para ese rol — así no se pisa lo que un admin ya configuró).
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM empresa WHERE activo = true`)
	if err != nil {
		return fmt.Errorf("rbac: listar empresas: %w", err)
	}
	var empresas []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("rbac: scan empresa: %w", err)
		}
		empresas = append(empresas, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, empresaID := range empresas {
		for rolCodigo, permisos := range MatrizDefault {
			var ya int
			if err := r.pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM rol_permiso rp JOIN rol r ON r.id = rp.rol_id
				WHERE rp.empresa_id = $1::uuid AND r.codigo = $2`, empresaID, rolCodigo).Scan(&ya); err != nil {
				return fmt.Errorf("rbac: contar grants: %w", err)
			}
			if ya > 0 {
				continue // la empresa ya tiene matriz configurada para este rol
			}
			for _, code := range permisos {
				if _, err := r.pool.Exec(ctx, `
					INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
					SELECT $1::uuid, r.id, p.id FROM rol r, permiso p
					WHERE r.codigo = $2 AND r.empresa_id IS NULL AND p.codigo = $3
					ON CONFLICT DO NOTHING`, empresaID, rolCodigo, code); err != nil {
					return fmt.Errorf("rbac: default %s/%s: %w", rolCodigo, code, err)
				}
			}
		}
	}
	return nil
}

func esUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
