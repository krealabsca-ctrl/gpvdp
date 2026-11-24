package bancos

// El calendario real de la empresa. El porqué está en periodos.go.

import (
	"context"
	"fmt"
)

// PeriodosConMovimiento devuelve los meses «YYYY-MM» con al menos un movimiento, del más viejo al
// más nuevo.
//
// SIN TOPE, a propósito: poner un límite acá sería reponer en el servidor el mismo cap que esta
// función viene a quitar de la pantalla. El resultado es una fila por mes cargado —21 en la
// empresa más antigua del grupo, medido el 16-nov-2026—, no por movimiento.
//
// Cuenta los movimientos EXCLUIDOS también (una carga revertida, un duplicado marcado). El mes
// existe y se cargó; que una fila esté fuera de los libros no borra el mes del calendario, y
// esconderlo dejaría al usuario sin poder mirar justamente el mes que quiere revisar.
func (r *pgRepository) PeriodosConMovimiento(ctx context.Context, empresaID string) ([]string, error) {
	const q = `
		SELECT to_char(fecha, 'YYYY-MM') AS periodo
		  FROM movimiento_bancario
		 WHERE empresa_id = $1::uuid
		 GROUP BY 1
		 ORDER BY 1`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: períodos con movimiento: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, 24)
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("bancos: scan período: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
