package bancos

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func (r *pgRepository) UpsertCotizacion(ctx context.Context, empresaID, fecha string, valor decimal.Decimal, fuente string) error {
	const q = `
		INSERT INTO tipo_cambio_cotizacion (empresa_id, fecha, valor, fuente)
		VALUES ($1::uuid, $2::date, $3, $4)
		ON CONFLICT (empresa_id, fecha) DO UPDATE SET valor = EXCLUDED.valor, fuente = EXCLUDED.fuente`
	if _, err := r.pool.Exec(ctx, q, empresaID, fecha, valor, fuente); err != nil {
		return fmt.Errorf("bancos: upsert cotización: %w", err)
	}
	return nil
}

func (r *pgRepository) CotizacionesMes(ctx context.Context, empresaID string, anio, mes int) ([]Cotizacion, error) {
	const q = `
		SELECT to_char(fecha, 'YYYY-MM-DD'), valor::text, fuente
		FROM tipo_cambio_cotizacion
		WHERE empresa_id = $1::uuid AND extract(year from fecha) = $2 AND extract(month from fecha) = $3
		ORDER BY fecha`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("bancos: cotizaciones del mes: %w", err)
	}
	defer rows.Close()
	var out []Cotizacion
	for rows.Next() {
		var c Cotizacion
		if err := rows.Scan(&c.Fecha, &c.Valor, &c.Fuente); err != nil {
			return nil, fmt.Errorf("bancos: scan cotización: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *pgRepository) EstadoTCMes(ctx context.Context, empresaID string, anio, mes int) (string, *string, error) {
	const q = `SELECT estado, valor_congelado::text FROM tipo_cambio_mes WHERE empresa_id = $1::uuid AND anio = $2 AND mes = $3`
	var estado string
	var valor *string
	err := r.pool.QueryRow(ctx, q, empresaID, anio, mes).Scan(&estado, &valor)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, nil // sin registro => provisional por defecto
	}
	if err != nil {
		return "", nil, fmt.Errorf("bancos: estado tc mes: %w", err)
	}
	return estado, valor, nil
}

func (r *pgRepository) AplicarProvisional(ctx context.Context, empresaID string, anio, mes int, tcAntes15, tcDesde15 decimal.Decimal) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("bancos: begin provisional: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const qAntes = `
		UPDATE movimiento_bancario
		SET tc_aplicado = $4, monto_crc = round(monto_original * $4, 2), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND moneda_original = 'USD'
		  AND extract(year from fecha) = $2 AND extract(month from fecha) = $3 AND extract(day from fecha) < 15`
	const qDesde = `
		UPDATE movimiento_bancario
		SET tc_aplicado = $4, monto_crc = round(monto_original * $4, 2), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND moneda_original = 'USD'
		  AND extract(year from fecha) = $2 AND extract(month from fecha) = $3 AND extract(day from fecha) >= 15`

	t1, err := tx.Exec(ctx, qAntes, empresaID, anio, mes, tcAntes15)
	if err != nil {
		return 0, fmt.Errorf("bancos: provisional <15: %w", err)
	}
	t2, err := tx.Exec(ctx, qDesde, empresaID, anio, mes, tcDesde15)
	if err != nil {
		return 0, fmt.Errorf("bancos: provisional >=15: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("bancos: commit provisional: %w", err)
	}
	return int(t1.RowsAffected() + t2.RowsAffected()), nil
}

func (r *pgRepository) CongelarTC(ctx context.Context, empresaID string, anio, mes int, valor decimal.Decimal) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("bancos: begin congelar: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var estado string
	err = tx.QueryRow(ctx, `SELECT estado FROM tipo_cambio_mes WHERE empresa_id = $1::uuid AND anio = $2 AND mes = $3`,
		empresaID, anio, mes).Scan(&estado)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("bancos: estado congelar: %w", err)
	}
	if estado == "CONGELADO" {
		return 0, ErrTCYaCongelado
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO tipo_cambio_mes (empresa_id, anio, mes, valor_congelado, estado, congelado_en)
		VALUES ($1::uuid, $2, $3, $4, 'CONGELADO', now())
		ON CONFLICT (empresa_id, anio, mes)
		DO UPDATE SET valor_congelado = EXCLUDED.valor_congelado, estado = 'CONGELADO', congelado_en = now()`,
		empresaID, anio, mes, valor); err != nil {
		return 0, fmt.Errorf("bancos: set congelado: %w", err)
	}

	// Aplica el TC congelado a TODOS los movimientos USD del mes (retroactivo, RN-12).
	tag, err := tx.Exec(ctx, `
		UPDATE movimiento_bancario
		SET tc_aplicado = $4, monto_crc = round(monto_original * $4, 2), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND moneda_original = 'USD'
		  AND extract(year from fecha) = $2 AND extract(month from fecha) = $3`,
		empresaID, anio, mes, valor)
	if err != nil {
		return 0, fmt.Errorf("bancos: aplicar congelado: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("bancos: commit congelar: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
