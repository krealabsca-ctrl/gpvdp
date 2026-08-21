package cxp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const departamentoCols = `id::text, nombre, COALESCE(codigo, ''), COALESCE(centro_costo, ''), activo`

func scanDepartamento(row scanner) (Departamento, error) {
	var d Departamento
	err := row.Scan(&d.ID, &d.Nombre, &d.Codigo, &d.CentroCosto, &d.Activo)
	return d, err
}

// ListarDepartamentos devuelve los departamentos de la empresa (todos o solo activos),
// ordenados por `orden` y nombre.
func (r *pgRepository) ListarDepartamentos(ctx context.Context, empresaID string, soloActivos bool) ([]Departamento, error) {
	conds := []string{"empresa_id = $1::uuid"}
	if soloActivos {
		conds = append(conds, "activo = true")
	}
	q := "SELECT " + departamentoCols + " FROM departamento WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY orden, nombre"
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar departamentos: %w", err)
	}
	defer rows.Close()
	out := make([]Departamento, 0)
	for rows.Next() {
		d, err := scanDepartamento(rows)
		if err != nil {
			return nil, fmt.Errorf("cxp: scan departamento: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CrearDepartamento da de alta un departamento en la empresa.
func (r *pgRepository) CrearDepartamento(ctx context.Context, empresaID string, in DepartamentoInput) (Departamento, error) {
	const q = `
		INSERT INTO departamento (empresa_id, nombre, codigo, centro_costo)
		VALUES ($1::uuid, $2, NULLIF($3, ''), NULLIF($4, ''))
		RETURNING ` + departamentoCols
	d, err := scanDepartamento(r.pool.QueryRow(ctx, q, empresaID, in.Nombre, in.Codigo, in.CentroCosto))
	if esViolacionUnica(err) {
		return Departamento{}, ErrDepartamentoDuplicado
	}
	if err != nil {
		return Departamento{}, fmt.Errorf("cxp: crear departamento: %w", err)
	}
	return d, nil
}

// ActualizarDepartamento modifica un departamento de la empresa.
func (r *pgRepository) ActualizarDepartamento(ctx context.Context, empresaID, id string, in DepartamentoInput) (Departamento, error) {
	const q = `
		UPDATE departamento
		SET nombre = $3, codigo = NULLIF($4, ''), centro_costo = NULLIF($5, ''), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		RETURNING ` + departamentoCols
	d, err := scanDepartamento(r.pool.QueryRow(ctx, q, empresaID, id, in.Nombre, in.Codigo, in.CentroCosto))
	if errors.Is(err, pgx.ErrNoRows) {
		return Departamento{}, ErrDepartamentoNoEncontrado
	}
	if esViolacionUnica(err) {
		return Departamento{}, ErrDepartamentoDuplicado
	}
	if err != nil {
		return Departamento{}, fmt.Errorf("cxp: actualizar departamento: %w", err)
	}
	return d, nil
}

// DesactivarDepartamento da de baja lógica a un departamento (nunca borrado físico).
func (r *pgRepository) DesactivarDepartamento(ctx context.Context, empresaID, id string) error {
	const q = `UPDATE departamento SET activo = false, actualizado_en = now() WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id)
	if err != nil {
		return fmt.Errorf("cxp: desactivar departamento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDepartamentoNoEncontrado
	}
	return nil
}

// EnsureDepartamentos siembra el set base de departamentos por empresa, idempotente
// (no pisa lo que exista: ON CONFLICT por nombre no hace nada). Se llama al arrancar.
func (r *pgRepository) EnsureDepartamentos(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM empresa WHERE activo = true`)
	if err != nil {
		return fmt.Errorf("cxp: listar empresas: %w", err)
	}
	var empresas []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("cxp: scan empresa: %w", err)
		}
		empresas = append(empresas, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	const ins = `
		INSERT INTO departamento (empresa_id, nombre, orden)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (empresa_id, nombre) DO NOTHING`
	for _, empresaID := range empresas {
		for i, nombre := range DepartamentosBase {
			if _, err := r.pool.Exec(ctx, ins, empresaID, nombre, i); err != nil {
				return fmt.Errorf("cxp: sembrar departamento %q: %w", nombre, err)
			}
		}
	}
	return nil
}
