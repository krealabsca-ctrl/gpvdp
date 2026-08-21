package bancos

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository crea el repository del importador respaldado por PostgreSQL.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func (r *pgRepository) ListarCuentas(ctx context.Context, empresaID string, incluirInactivas bool) ([]CuentaListItem, error) {
	// El catálogo necesita ver las desactivadas para poder reactivarlas; el importador y los
	// filtros, no: una cuenta desactivada no debe poder recibir movimientos nuevos.
	const q = `
		SELECT c.id::text, COALESCE(c.alias, ''), b.nombre, COALESCE(c.iban, ''), c.moneda, c.activo
		FROM cuenta_bancaria c
		JOIN banco b ON b.id = c.banco_id
		WHERE c.empresa_id = $1::uuid AND (c.activo = true OR $2)
		ORDER BY c.activo DESC, b.nombre, c.alias`
	rows, err := r.pool.Query(ctx, q, empresaID, incluirInactivas)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar cuentas: %w", err)
	}
	defer rows.Close()
	var out []CuentaListItem
	for rows.Next() {
		var c CuentaListItem
		if err := rows.Scan(&c.ID, &c.Alias, &c.Banco, &c.IBAN, &c.Moneda, &c.Activo); err != nil {
			return nil, fmt.Errorf("bancos: scan cuenta: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("bancos: iterar cuentas: %w", err)
	}
	return out, nil
}

func (r *pgRepository) CuentaByID(ctx context.Context, empresaID, cuentaID string) (Cuenta, error) {
	const q = `
		SELECT id::text, banco_id::text, COALESCE(iban, ''), moneda, COALESCE(alias, '')
		FROM cuenta_bancaria
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND activo = true`
	var c Cuenta
	err := r.pool.QueryRow(ctx, q, empresaID, cuentaID).Scan(&c.ID, &c.BancoID, &c.IBAN, &c.Moneda, &c.Alias)
	if errors.Is(err, pgx.ErrNoRows) {
		return Cuenta{}, ErrCuentaNoEncontrada
	}
	if err != nil {
		return Cuenta{}, fmt.Errorf("bancos: cuenta por id: %w", err)
	}
	return c, nil
}

func (r *pgRepository) CrearImportacion(ctx context.Context, empresaID, cuentaID, hash, nombre string, banco Banco, archivo []byte, usuarioID string) (string, error) {
	const q = `
		INSERT INTO importacion (empresa_id, cuenta_bancaria_id, source_file_hash, nombre_archivo, estado, creado_por, archivo, banco)
		VALUES ($1::uuid, $2::uuid, $3, $4, 'PREVISUALIZADA', $5::uuid, $6, $7)
		RETURNING id::text`
	var id string
	if err := r.pool.QueryRow(ctx, q, empresaID, cuentaID, hash, nombre, usuarioID, archivo, string(banco)).Scan(&id); err != nil {
		return "", fmt.Errorf("bancos: crear importación: %w", err)
	}
	return id, nil
}

func (r *pgRepository) ImportacionArchivo(ctx context.Context, empresaID, importacionID string) (string, []byte, error) {
	const q = `SELECT cuenta_bancaria_id::text, archivo FROM importacion WHERE empresa_id = $1::uuid AND id = $2::uuid`
	var cuentaID string
	var archivo []byte
	err := r.pool.QueryRow(ctx, q, empresaID, importacionID).Scan(&cuentaID, &archivo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrImportacionNoEncontrada
	}
	if err != nil {
		return "", nil, fmt.Errorf("bancos: importación por id: %w", err)
	}
	return cuentaID, archivo, nil
}

func (r *pgRepository) NaturalKeysExistentes(ctx context.Context, empresaID string, keys []string) (map[string]bool, error) {
	out := make(map[string]bool)
	if len(keys) == 0 {
		return out, nil
	}
	const q = `SELECT natural_key FROM movimiento_bancario WHERE empresa_id = $1::uuid AND natural_key = ANY($2)`
	rows, err := r.pool.Query(ctx, q, empresaID, keys)
	if err != nil {
		return nil, fmt.Errorf("bancos: natural keys existentes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("bancos: scan natural key: %w", err)
		}
		out[k] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("bancos: iterar natural keys: %w", err)
	}
	return out, nil
}

func (r *pgRepository) ConfirmarConMovimientos(ctx context.Context, empresaID, cuentaID, importacionID, moneda string, movs []MovimientoParaInsertar) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("bancos: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, importacion_id, fecha, documento, descripcion,
			 debito, credito, moneda_original, monto_original, monto_crc, tc_aplicado,
			 estado_clasificacion, natural_key, indice_ocurrencia)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12,
		        'NO_IDENTIFICADO', $13, $14)
		ON CONFLICT (empresa_id, natural_key) DO NOTHING`

	inserted := 0
	for _, m := range movs {
		tag, err := tx.Exec(ctx, q,
			empresaID, cuentaID, importacionID, m.Fecha, m.Documento, m.Descripcion,
			m.Debito, m.Credito, moneda, m.MontoOriginal, m.MontoCRC, m.TCAplicado,
			m.NaturalKey, m.IndiceOcurrencia)
		if err != nil {
			return 0, fmt.Errorf("bancos: insertar movimiento: %w", err)
		}
		inserted += int(tag.RowsAffected())
	}

	if _, err := tx.Exec(ctx,
		`UPDATE importacion SET estado = 'CONFIRMADA' WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, importacionID); err != nil {
		return 0, fmt.Errorf("bancos: confirmar importación: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("bancos: commit: %w", err)
	}
	return inserted, nil
}

func (r *pgRepository) SetCuentaIBANSiVacio(ctx context.Context, empresaID, cuentaID, iban string) error {
	const q = `
		UPDATE cuenta_bancaria SET iban = $3
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND (iban IS NULL OR iban = '')`
	if _, err := r.pool.Exec(ctx, q, empresaID, cuentaID, iban); err != nil {
		return fmt.Errorf("bancos: memorizar iban: %w", err)
	}
	return nil
}
