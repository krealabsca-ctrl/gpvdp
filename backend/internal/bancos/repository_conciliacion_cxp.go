package bancos

// Consultas del barrido de huellas Bancos↔CxP. Filtran por empresa_id.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// MovimientosConHuella devuelve los débitos que traen el prefijo de la huella en su descripción
// y todavía no están enlazados a una factura. Solo débitos: la huella marca un PAGO.
// importacionID vacío = toda la empresa; con id, solo lo recién importado.
func (r *pgRepository) MovimientosConHuella(ctx context.Context, empresaID, prefijo, importacionID string) ([]MovimientoConHuella, error) {
	q := `
		SELECT m.id::text, to_char(m.fecha, 'YYYY-MM-DD'), m.descripcion, m.debito::text, cb.alias
		FROM movimiento_bancario m
		JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		WHERE m.empresa_id = $1::uuid AND m.incluido AND m.debito > 0
		  AND m.documento_cxp_id IS NULL
		  AND m.descripcion ILIKE '%' || $2 || '%'`
	args := []any{empresaID, prefijo}
	if importacionID != "" {
		q += ` AND m.importacion_id = $3::uuid`
		args = append(args, importacionID)
	}
	q += ` ORDER BY m.fecha, m.id`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("bancos: movimientos con huella: %w", err)
	}
	defer rows.Close()
	out := make([]MovimientoConHuella, 0, 32)
	for rows.Next() {
		var m MovimientoConHuella
		if err := rows.Scan(&m.ID, &m.Fecha, &m.Descripcion, &m.Debito, &m.Cuenta); err != nil {
			return nil, fmt.Errorf("bancos: scan movimiento con huella: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// EnlazarPagoCxP deja el rastro del movimiento que ES el pago de la factura y, si el documento
// traía clasificación de gasto, la aplica al movimiento (deja de ser No identificado).
//
// Respeta lo que ya está: NO pisa una clasificación hecha a mano ni por el motor. Devuelve
// (clasificado, enlazado): enlazado=false significa que alguien lo enlazó antes (carrera), y no
// es un error.
//
// El CTE `previo` fija el estado ANTES del UPDATE (y bloquea la fila): sin eso, el RETURNING
// vería el estado nuevo y reportaría como «clasificado por el enlace» algo que ya venía del motor.
func (r *pgRepository) EnlazarPagoCxP(ctx context.Context, empresaID, movimientoID, documentoID, conceptoID, clasificacionID string) (bool, bool, error) {
	const q = `
		WITH previo AS (
			SELECT id, estado_clasificacion
			FROM movimiento_bancario
			WHERE empresa_id = $1::uuid AND id = $2::uuid AND documento_cxp_id IS NULL
			FOR UPDATE
		),
		-- El par (concepto, clasificación) se DERIVA de la clasificación en vez de copiarse:
		-- movimiento_bancario tiene FK compuesta y el documento podría traer un par que no
		-- corresponde. Sin clasificación válida, se clasifica solo por concepto.
		elegido AS (
			SELECT cl.id AS clasif_id, cl.concepto_id
			FROM clasificacion cl WHERE cl.id = NULLIF($5, '')::uuid
			UNION ALL
			SELECT NULL::uuid, NULLIF($4, '')::uuid
			WHERE NOT EXISTS (SELECT 1 FROM clasificacion WHERE id = NULLIF($5, '')::uuid)
		),
		plan AS (
			SELECT p.id, e.clasif_id, e.concepto_id,
			       (e.concepto_id IS NOT NULL AND p.estado_clasificacion = 'NO_IDENTIFICADO') AS aplica
			FROM previo p CROSS JOIN elegido e
		)
		UPDATE movimiento_bancario m
		SET documento_cxp_id = $3::uuid,
		    concepto_id = CASE WHEN pl.aplica THEN pl.concepto_id ELSE m.concepto_id END,
		    clasificacion_id = CASE WHEN pl.aplica THEN pl.clasif_id ELSE m.clasificacion_id END,
		    estado_clasificacion = CASE WHEN pl.aplica THEN 'AUTO' ELSE m.estado_clasificacion END,
		    confianza = CASE WHEN pl.aplica THEN 100 ELSE m.confianza END,
		    actualizado_en = now()
		FROM plan pl
		WHERE m.id = pl.id
		RETURNING pl.aplica`
	var clasificado bool
	err := r.pool.QueryRow(ctx, q, empresaID, movimientoID, documentoID, conceptoID, clasificacionID).Scan(&clasificado)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("bancos: enlazar pago CxP: %w", err)
	}
	return clasificado, true, nil
}
