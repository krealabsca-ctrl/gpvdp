package bancos

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// CuadreArbolRow es una fila plana (concepto × clasificación) con montos y conteos
// por lado (crédito = ingreso, débito = gasto). Base para armar el árbol del cuadre.
type CuadreArbolRow struct {
	ConceptoID      string
	Concepto        string
	ClasificacionID string
	Clasificacion   string
	CreditoSum      decimal.Decimal
	CreditoCnt      int
	DebitoSum       decimal.Decimal
	DebitoCnt       int
}

// CuadreArbol agrega por concepto y clasificación, separando crédito/débito y con
// conteo de movimientos. Mismo filtro que Cuadre (incluido = true, monto en CRC).
func (r *pgRepository) CuadreArbol(ctx context.Context, empresaID, periodo string) ([]CuadreArbolRow, error) {
	const q = `
		SELECT COALESCE(m.concepto_id::text, ''), COALESCE(co.nombre, '(sin concepto)'),
		       COALESCE(m.clasificacion_id::text, ''), COALESCE(cl.nombre, '(sin clasificación)'),
		       COALESCE(SUM(CASE WHEN m.credito > 0 THEN m.monto_crc ELSE 0 END), 0),
		       COUNT(*) FILTER (WHERE m.credito > 0),
		       COALESCE(SUM(CASE WHEN m.debito  > 0 THEN m.monto_crc ELSE 0 END), 0),
		       COUNT(*) FILTER (WHERE m.debito  > 0)
		FROM movimiento_bancario m
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		-- El cuadre muestra TODO (incluye traslados/overnight: así se ve que cuadran,
		-- débito ≈ crédito). El EBITDA es lo único que excluye es_traslado.
		WHERE m.empresa_id = $1::uuid AND to_char(m.fecha, 'YYYY-MM') = $2 AND m.incluido = true
		GROUP BY m.concepto_id, co.nombre, m.clasificacion_id, cl.nombre
		ORDER BY co.nombre, cl.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("bancos: cuadre árbol: %w", err)
	}
	defer rows.Close()
	var out []CuadreArbolRow
	for rows.Next() {
		var row CuadreArbolRow
		if err := rows.Scan(&row.ConceptoID, &row.Concepto, &row.ClasificacionID, &row.Clasificacion,
			&row.CreditoSum, &row.CreditoCnt, &row.DebitoSum, &row.DebitoCnt); err != nil {
			return nil, fmt.Errorf("bancos: scan cuadre árbol: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
