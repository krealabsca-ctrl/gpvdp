package rbac

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ListarUsuarios devuelve los usuarios con acceso a la empresa + su rol en ella.
func (r *Repository) ListarUsuarios(ctx context.Context, empresaID string) ([]UsuarioAdmin, error) {
	const q = `
		SELECT u.id::text, u.nombre, u.email, u.activo, u.debe_cambiar_password, r.codigo, r.nombre
		FROM usuario_empresa_rol uer
		JOIN usuario u ON u.id = uer.usuario_id
		JOIN rol r ON r.id = uer.rol_id
		WHERE uer.empresa_id = $1::uuid
		ORDER BY u.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("rbac: listar usuarios: %w", err)
	}
	defer rows.Close()
	out := make([]UsuarioAdmin, 0)
	for rows.Next() {
		var u UsuarioAdmin
		if err := rows.Scan(&u.ID, &u.Nombre, &u.Email, &u.Activo, &u.DebeCambiar, &u.RolCodigo, &u.RolNombre); err != nil {
			return nil, fmt.Errorf("rbac: scan usuario: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UsuarioIDPorEmail busca un usuario por correo (para crear-o-vincular).
func (r *Repository) UsuarioIDPorEmail(ctx context.Context, email string) (string, bool, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM usuario WHERE email = $1`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("rbac: usuario por email: %w", err)
	}
	return id, true, nil
}

// CrearUsuario da de alta un usuario con contraseña temporal (debe cambiarla al ingresar).
func (r *Repository) CrearUsuario(ctx context.Context, nombre, email, hash string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO usuario (nombre, email, password_hash, debe_cambiar_password) VALUES ($1, $2, $3, true) RETURNING id::text`,
		nombre, email, hash).Scan(&id)
	if esUnica(err) {
		return "", ErrEmailDuplicado
	}
	if err != nil {
		return "", fmt.Errorf("rbac: crear usuario: %w", err)
	}
	return id, nil
}

// AsignarRolEmpresa vincula (o cambia el rol de) un usuario en una empresa. Resuelve el rol por
// código (base o a medida de la empresa). 0 filas = código de rol inválido.
func (r *Repository) AsignarRolEmpresa(ctx context.Context, empresaID, usuarioID, rolCodigo string) error {
	const q = `
		WITH rid AS (
			SELECT id FROM rol
			WHERE codigo = $3 AND (empresa_id IS NULL OR empresa_id = $1::uuid)
			ORDER BY (empresa_id IS NOT NULL) DESC LIMIT 1
		)
		INSERT INTO usuario_empresa_rol (empresa_id, usuario_id, rol_id)
		SELECT $1::uuid, $2::uuid, rid.id FROM rid
		ON CONFLICT (empresa_id, usuario_id) DO UPDATE SET rol_id = EXCLUDED.rol_id`
	tag, err := r.pool.Exec(ctx, q, empresaID, usuarioID, rolCodigo)
	if err != nil {
		return fmt.Errorf("rbac: asignar rol a usuario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRolNoEncontrado
	}
	return nil
}

// ActualizarUsuario cambia nombre y estado (globales), solo si el usuario tiene acceso a la empresa
// activa (tenant-safe: un admin no toca usuarios de otras empresas).
func (r *Repository) ActualizarUsuario(ctx context.Context, empresaID, usuarioID, nombre string, activo bool) error {
	const q = `
		UPDATE usuario SET nombre = $3, activo = $4, actualizado_en = now()
		WHERE id = $2::uuid
		  AND EXISTS (SELECT 1 FROM usuario_empresa_rol WHERE usuario_id = $2::uuid AND empresa_id = $1::uuid)`
	tag, err := r.pool.Exec(ctx, q, empresaID, usuarioID, nombre, activo)
	if err != nil {
		return fmt.Errorf("rbac: actualizar usuario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUsuarioNoEncontrado
	}
	return nil
}

// SetPasswordTemporal fija una contraseña temporal (debe cambiarla al ingresar), tenant-safe.
func (r *Repository) SetPasswordTemporal(ctx context.Context, empresaID, usuarioID, hash string) error {
	const q = `
		UPDATE usuario SET password_hash = $3, debe_cambiar_password = true, actualizado_en = now()
		WHERE id = $2::uuid
		  AND EXISTS (SELECT 1 FROM usuario_empresa_rol WHERE usuario_id = $2::uuid AND empresa_id = $1::uuid)`
	tag, err := r.pool.Exec(ctx, q, empresaID, usuarioID, hash)
	if err != nil {
		return fmt.Errorf("rbac: set password temporal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUsuarioNoEncontrado
	}
	return nil
}

// QuitarAcceso desvincula al usuario de la empresa activa (no borra el usuario global).
func (r *Repository) QuitarAcceso(ctx context.Context, empresaID, usuarioID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM usuario_empresa_rol WHERE empresa_id = $1::uuid AND usuario_id = $2::uuid`, empresaID, usuarioID)
	if err != nil {
		return fmt.Errorf("rbac: quitar acceso: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUsuarioNoEncontrado
	}
	return nil
}

// AplicarPermisosFaltantes otorga a los roles base los permisos de la matriz por defecto que
// aún no tengan concedidos en la empresa (nunca quita nada). Devuelve cuántos se agregaron.
// Útil cuando se suman permisos nuevos al catálogo después de sembrar la empresa.
func (r *Repository) AplicarPermisosFaltantes(ctx context.Context, empresaID string) (int, error) {
	total := 0
	for rolCodigo, permisos := range MatrizDefault {
		for _, code := range permisos {
			tag, err := r.pool.Exec(ctx, `
				INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
				SELECT $1::uuid, r.id, p.id
				FROM rol r, permiso p
				WHERE r.codigo = $2 AND (r.empresa_id IS NULL OR r.empresa_id = $1::uuid) AND p.codigo = $3
				ON CONFLICT DO NOTHING`, empresaID, rolCodigo, code)
			if err != nil {
				return total, fmt.Errorf("rbac: aplicar permiso faltante %s/%s: %w", rolCodigo, code, err)
			}
			total += int(tag.RowsAffected())
		}
	}
	return total, nil
}
