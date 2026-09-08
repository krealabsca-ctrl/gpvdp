package rbac

// Traer a esta empresa un rol a medida que ya existe en otra.
//
// El caso real (2026-08-21): un rol «Nómina» creado en Valle de Paz, y la necesidad de darle ese
// mismo rol a alguien en Coopeprofa. Rehacerlo a mano obliga a volver a marcar los permisos uno por
// uno y, sobre todo, deja dos roles que se llaman igual y con el tiempo dejan de ser iguales sin que
// nadie lo note.
//
// ── La guarda que hace que esto sea seguro ───────────────────────────────────
//
// El permiso `admin.roles` se valida sobre la empresa ACTIVA, que es donde se crea el rol. Eso no
// alcanza: leer los permisos de un rol de OTRA empresa es leer datos de esa empresa. Por eso se
// exige además que quien lo trae **tenga membresía en la empresa de origen**. Sin esa condición,
// administrar una empresa alcanzaría para espiar la configuración de las demás.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrEmpresaOrigenSinAcceso indica que quien trae el rol no pertenece a la empresa de origen.
var ErrEmpresaOrigenSinAcceso = errors.New("rbac: no tenés acceso a la empresa de origen")

// RolTraible es un rol a medida de otra empresa que se puede traer a la activa.
type RolTraible struct {
	Codigo         string `json:"codigo"`
	Nombre         string `json:"nombre"`
	EmpresaID      string `json:"empresa_id"`
	EmpresaNombre  string `json:"empresa_nombre"`
	CuantosPermiso int    `json:"cuantos_permisos"`
}

// RolesTraibles lista los roles a medida de las otras empresas DEL ACTOR que todavía no existen en
// la empresa activa. Se excluyen los que ya existen acá: para esos el camino es editarlos, no
// traerlos de nuevo.
func (r *Repository) RolesTraibles(ctx context.Context, empresaDestino, actorID string) ([]RolTraible, error) {
	const q = `
		SELECT ro.codigo, ro.nombre, ro.empresa_id::text, e.nombre,
		       (SELECT count(*) FROM rol_permiso rp
		        WHERE rp.rol_id = ro.id AND rp.empresa_id = ro.empresa_id)::int
		FROM rol ro
		JOIN empresa e ON e.id = ro.empresa_id
		WHERE ro.empresa_id IS NOT NULL
		  AND ro.empresa_id <> $1::uuid
		  AND e.activo
		  -- Solo empresas donde QUIEN PREGUNTA tiene acceso.
		  AND EXISTS (SELECT 1 FROM usuario_empresa_rol uer
		              WHERE uer.usuario_id = $2::uuid AND uer.empresa_id = ro.empresa_id)
		  -- Y que no exista ya acá con ese código.
		  AND NOT EXISTS (SELECT 1 FROM rol r2
		                  WHERE r2.codigo = ro.codigo AND r2.empresa_id = $1::uuid)
		ORDER BY e.nombre, ro.nombre`
	rows, err := r.pool.Query(ctx, q, empresaDestino, actorID)
	if err != nil {
		return nil, fmt.Errorf("rbac: roles traibles: %w", err)
	}
	defer rows.Close()
	out := make([]RolTraible, 0)
	for rows.Next() {
		var it RolTraible
		if err := rows.Scan(&it.Codigo, &it.Nombre, &it.EmpresaID, &it.EmpresaNombre, &it.CuantosPermiso); err != nil {
			return nil, fmt.Errorf("rbac: scan rol traible: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// TraerRol crea en la empresa activa un rol con el mismo código, nombre y permisos que el de la
// empresa de origen. Devuelve el rol creado y cuántos permisos se copiaron.
//
// Es una COPIA, no un vínculo: desde acá los dos roles evolucionan por separado, porque los permisos
// de un rol siempre fueron por empresa (`rol_permiso` lleva `empresa_id` en su clave). Lo que se
// evita es volver a marcarlos a mano, que es donde se cuelan las diferencias.
func (r *Repository) TraerRol(ctx context.Context, empresaDestino, empresaOrigen, codigo, actorID string) (RolItem, int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RolItem{}, 0, fmt.Errorf("rbac: begin traer rol: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Quien trae el rol tiene que pertenecer a la empresa de origen.
	var tieneAcceso bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM usuario_empresa_rol
		                WHERE usuario_id = $1::uuid AND empresa_id = $2::uuid)`,
		actorID, empresaOrigen).Scan(&tieneAcceso); err != nil {
		return RolItem{}, 0, fmt.Errorf("rbac: verificar acceso al origen: %w", err)
	}
	if !tieneAcceso {
		return RolItem{}, 0, ErrEmpresaOrigenSinAcceso
	}

	// 2. El rol de origen, que tiene que ser a medida DE esa empresa.
	var origenID, nombre string
	err = tx.QueryRow(ctx,
		`SELECT id::text, nombre FROM rol WHERE codigo = $1 AND empresa_id = $2::uuid`,
		codigo, empresaOrigen).Scan(&origenID, &nombre)
	if errors.Is(err, pgx.ErrNoRows) {
		return RolItem{}, 0, ErrRolNoEncontrado
	}
	if err != nil {
		return RolItem{}, 0, fmt.Errorf("rbac: buscar rol de origen: %w", err)
	}

	// 3. Crear el rol acá. El índice único parcial de la migración 0067 es lo que permite repetir el
	//    código en otra empresa; si ya existe en ESTA, es un duplicado de verdad.
	var nuevoID string
	err = tx.QueryRow(ctx,
		`INSERT INTO rol (codigo, nombre, empresa_id) VALUES ($1, $2, $3::uuid) RETURNING id::text`,
		codigo, nombre, empresaDestino).Scan(&nuevoID)
	if esUnica(err) {
		return RolItem{}, 0, ErrRolDuplicado
	}
	if err != nil {
		return RolItem{}, 0, fmt.Errorf("rbac: crear rol traído: %w", err)
	}

	// 4. Copiar los permisos que el rol tiene EN SU EMPRESA. Se filtra por `rp.empresa_id` y no solo
	//    por `rol_id`: sin eso se copiarían también concesiones que ese rol tenga en otras empresas.
	tag, err := tx.Exec(ctx,
		`INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
		 SELECT $1::uuid, $2::uuid, rp.permiso_id
		 FROM rol_permiso rp
		 WHERE rp.rol_id = $3::uuid AND rp.empresa_id = $4::uuid
		 ON CONFLICT DO NOTHING`,
		empresaDestino, nuevoID, origenID, empresaOrigen)
	if err != nil {
		return RolItem{}, 0, fmt.Errorf("rbac: copiar permisos: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return RolItem{}, 0, fmt.Errorf("rbac: commit traer rol: %w", err)
	}
	return RolItem{ID: nuevoID, Codigo: codigo, Nombre: nombre, EsBase: false}, int(tag.RowsAffected()), nil
}
