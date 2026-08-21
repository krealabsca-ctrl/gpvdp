package cxp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ListarSubclasificaciones lista las subclasificaciones activas de la empresa; si se indica
// clasificacionID, solo las de esa clasificación.
func (r *pgRepository) ListarSubclasificaciones(ctx context.Context, empresaID, clasificacionID string) ([]Subclasificacion, error) {
	conds := []string{"empresa_id = $1::uuid", "activo = true"}
	args := []any{empresaID}
	if clasificacionID != "" {
		args = append(args, clasificacionID)
		conds = append(conds, fmt.Sprintf("clasificacion_id = $%d::uuid", len(args)))
	}
	q := "SELECT id::text, clasificacion_id::text, nombre FROM subclasificacion WHERE " +
		strings.Join(conds, " AND ") + " ORDER BY nombre"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar subclasificaciones: %w", err)
	}
	defer rows.Close()
	out := make([]Subclasificacion, 0)
	for rows.Next() {
		var s Subclasificacion
		if err := rows.Scan(&s.ID, &s.ClasificacionID, &s.Nombre); err != nil {
			return nil, fmt.Errorf("cxp: scan subclasificacion: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CrearSubclasificacion da de alta una subclasificación (tenant-safe: la clasificación padre debe
// ser de la empresa). Si ya existe con ese nombre, devuelve la existente (idempotente).
func (r *pgRepository) CrearSubclasificacion(ctx context.Context, empresaID, clasificacionID, nombre string) (Subclasificacion, error) {
	const q = `
		INSERT INTO subclasificacion (empresa_id, clasificacion_id, nombre)
		SELECT $1::uuid, $2::uuid, $3
		WHERE EXISTS (SELECT 1 FROM clasificacion WHERE id = $2::uuid AND empresa_id = $1::uuid)
		RETURNING id::text, clasificacion_id::text, nombre`
	var s Subclasificacion
	err := r.pool.QueryRow(ctx, q, empresaID, clasificacionID, nombre).Scan(&s.ID, &s.ClasificacionID, &s.Nombre)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subclasificacion{}, ErrCatalogoInvalido // clasificación ajena o inexistente
	}
	if esViolacionUnica(err) {
		const sel = `SELECT id::text, clasificacion_id::text, nombre FROM subclasificacion WHERE clasificacion_id = $1::uuid AND nombre = $2`
		if e := r.pool.QueryRow(ctx, sel, clasificacionID, nombre).Scan(&s.ID, &s.ClasificacionID, &s.Nombre); e == nil {
			return s, nil
		}
		return Subclasificacion{}, ErrCatalogoInvalido
	}
	if err != nil {
		return Subclasificacion{}, fmt.Errorf("cxp: crear subclasificacion: %w", err)
	}
	return s, nil
}
