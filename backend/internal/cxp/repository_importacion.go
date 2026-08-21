package cxp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ClavesExistentes devuelve qué claves (50 díg.) ya existen como documento de la empresa.
func (r *pgRepository) ClavesExistentes(ctx context.Context, empresaID string, claves []string) (map[string]bool, error) {
	out := make(map[string]bool, len(claves))
	if len(claves) == 0 {
		return out, nil
	}
	const q = `SELECT clave FROM documento_cxp WHERE empresa_id = $1::uuid AND clave = ANY($2)`
	rows, err := r.pool.Query(ctx, q, empresaID, claves)
	if err != nil {
		return nil, fmt.Errorf("cxp: claves existentes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("cxp: scan clave: %w", err)
		}
		out[c] = true
	}
	return out, rows.Err()
}

// ProveedorIDPorIdentificacion busca un proveedor activo de la empresa por su identificación (cédula).
func (r *pgRepository) ProveedorIDPorIdentificacion(ctx context.Context, empresaID, identificacion string) (string, bool, error) {
	if identificacion == "" {
		return "", false, nil
	}
	const q = `SELECT id::text FROM proveedor WHERE empresa_id = $1::uuid AND identificacion = $2 AND activo = true LIMIT 1`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, identificacion).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("cxp: proveedor por identificación: %w", err)
	}
	return id, true, nil
}
