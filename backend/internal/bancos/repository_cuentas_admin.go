package bancos

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// BancoItem es un banco registrado por la empresa (catálogo + selector de cuentas).
type BancoItem struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	// Activo: un banco desactivado sigue existiendo (sus cuentas y movimientos también),
	// pero no se ofrece para crear cuentas nuevas.
	Activo bool `json:"activo"`
}

func (r *pgRepository) ListarBancos(ctx context.Context, empresaID string, incluirInactivos bool) ([]BancoItem, error) {
	// El catálogo pide los inactivos para poder reactivarlos; los selectores, no.
	const q = `SELECT id::text, nombre, activo FROM banco
		WHERE empresa_id = $1::uuid AND (activo = true OR $2)
		ORDER BY activo DESC, nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, incluirInactivos)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar bancos: %w", err)
	}
	defer rows.Close()
	var out []BancoItem
	for rows.Next() {
		var b BancoItem
		if err := rows.Scan(&b.ID, &b.Nombre, &b.Activo); err != nil {
			return nil, fmt.Errorf("bancos: scan banco: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *pgRepository) CrearBanco(ctx context.Context, empresaID, nombre string) (BancoItem, error) {
	// Evita duplicar por nombre (case-insensitive) dentro de la empresa: 0 filas => duplicado.
	const q = `
		INSERT INTO banco (empresa_id, nombre)
		SELECT $1::uuid, $2
		WHERE NOT EXISTS (
			SELECT 1 FROM banco WHERE empresa_id = $1::uuid AND activo = true AND lower(nombre) = lower($2)
		)
		RETURNING id::text, nombre`
	var b BancoItem
	err := r.pool.QueryRow(ctx, q, empresaID, nombre).Scan(&b.ID, &b.Nombre)
	if errors.Is(err, pgx.ErrNoRows) {
		return BancoItem{}, ErrCatalogoDuplicado
	}
	if err != nil {
		return BancoItem{}, fmt.Errorf("bancos: crear banco: %w", err)
	}
	return b, nil
}

func (r *pgRepository) RenombrarBanco(ctx context.Context, empresaID, bancoID, nombre string) error {
	const q = `UPDATE banco SET nombre = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid AND activo = true`
	tag, err := r.pool.Exec(ctx, q, empresaID, bancoID, nombre)
	if err != nil {
		return fmt.Errorf("bancos: renombrar banco: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBancoNoEncontrado
	}
	return nil
}

func (r *pgRepository) CrearCuenta(ctx context.Context, empresaID, bancoID, alias, iban, moneda string) (CuentaListItem, error) {
	// INSERT solo si el banco pertenece a la empresa (tenant-safe). UNIQUE(empresa_id, iban) => 23505.
	const q = `
		WITH nueva AS (
			INSERT INTO cuenta_bancaria (empresa_id, banco_id, alias, iban, moneda)
			SELECT $1::uuid, $2::uuid, NULLIF($3, ''), NULLIF($4, ''), $5
			WHERE EXISTS (SELECT 1 FROM banco WHERE id = $2::uuid AND empresa_id = $1::uuid AND activo = true)
			RETURNING id, banco_id, alias, iban, moneda
		)
		SELECT n.id::text, COALESCE(n.alias, ''), b.nombre, COALESCE(n.iban, ''), n.moneda
		FROM nueva n JOIN banco b ON b.id = n.banco_id`
	var c CuentaListItem
	err := r.pool.QueryRow(ctx, q, empresaID, bancoID, alias, iban, moneda).Scan(&c.ID, &c.Alias, &c.Banco, &c.IBAN, &c.Moneda)
	if errors.Is(err, pgx.ErrNoRows) {
		return CuentaListItem{}, ErrBancoNoEncontrado
	}
	if esViolacionUnica(err) {
		return CuentaListItem{}, ErrCatalogoDuplicado
	}
	if err != nil {
		return CuentaListItem{}, fmt.Errorf("bancos: crear cuenta: %w", err)
	}
	return c, nil
}

func (r *pgRepository) RenombrarCuenta(ctx context.Context, empresaID, cuentaID, alias string) error {
	const q = `UPDATE cuenta_bancaria SET alias = NULLIF($3, '') WHERE empresa_id = $1::uuid AND id = $2::uuid AND activo = true`
	tag, err := r.pool.Exec(ctx, q, empresaID, cuentaID, alias)
	if err != nil {
		return fmt.Errorf("bancos: renombrar cuenta: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCuentaNoEncontrada
	}
	return nil
}
