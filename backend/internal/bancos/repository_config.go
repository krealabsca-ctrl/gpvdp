package bancos

// Parámetros de negocio configurables por empresa (Fase D). Hoy: tolerancia de
// traslado. Diseñado para crecer (otros parámetros por empresa) sin nueva tabla.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func (r *pgRepository) ToleranciaTraslado(ctx context.Context, empresaID string) (decimal.Decimal, error) {
	var pct decimal.Decimal
	err := r.pool.QueryRow(ctx,
		`SELECT tolerancia_traslado FROM empresa WHERE id = $1::uuid`, empresaID).Scan(&pct)
	if errors.Is(err, pgx.ErrNoRows) {
		return ToleranciaTrasladoDefault, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("bancos: leer tolerancia: %w", err)
	}
	return pct, nil
}

func (r *pgRepository) ActualizarTolerancia(ctx context.Context, empresaID string, pct decimal.Decimal) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE empresa SET tolerancia_traslado = $2 WHERE id = $1::uuid`, empresaID, pct)
	if err != nil {
		return fmt.Errorf("bancos: actualizar tolerancia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEmpresaNoEncontrada
	}
	return nil
}
