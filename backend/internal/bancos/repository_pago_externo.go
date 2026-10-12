package bancos

// Acceso a datos del sello «aplicado en el sistema anterior».

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// DebitosParaCruzar trae los movimientos de SALIDA que todavía no tienen sello ni enlace a una
// factura del ERP: son los únicos que un reporte externo puede explicar.
//
// Se traen todos de una y el cruce se hace en memoria. Con 3.437 débitos sin marcar el conjunto
// entra holgado, y consultar uno por uno por cada renglón del reporte serían 50.000 viajes.
func (r *pgRepository) DebitosParaCruzar(ctx context.Context, empresaID string) ([]DebitoCandidato, error) {
	const q = `
		SELECT m.id::text, to_char(m.fecha, 'YYYY-MM-DD'),
		       COALESCE(m.documento, ''), COALESCE(m.descripcion, ''), m.debito::text
		FROM movimiento_bancario m
		WHERE m.empresa_id = $1::uuid
		  AND m.incluido
		  AND m.debito > 0
		  -- Ya enlazado al ERP: ese movimiento tiene dueño y no se re-explica desde afuera.
		  AND m.documento_cxp_id IS NULL
		  AND m.remesa_id IS NULL
		  -- Ya sellado por una carga anterior: no se marca dos veces.
		  AND NOT EXISTS (
		      SELECT 1 FROM pago_aplicado_externo p WHERE p.movimiento_bancario_id = m.id
		  )
		ORDER BY m.fecha, m.id`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: débitos para cruzar: %w", err)
	}
	defer rows.Close()

	out := make([]DebitoCandidato, 0, 4096)
	for rows.Next() {
		var c DebitoCandidato
		var monto string
		if err := rows.Scan(&c.ID, &c.Fecha, &c.Documento, &c.Descripcion, &monto); err != nil {
			return nil, fmt.Errorf("bancos: scan débito candidato: %w", err)
		}
		d, err := decimal.NewFromString(monto)
		if err != nil {
			return nil, fmt.Errorf("bancos: débito %s con monto ilegible %q: %w", c.ID, monto, err)
		}
		c.Monto = d
		out = append(out, c)
	}
	return out, rows.Err()
}

// SellarPagosExternos escribe las marcas de UNA carga, en una transacción.
//
// Todo o nada: una carga que se escribe a medias deja movimientos sellados y otros no, sin forma de
// saber cuáles —y la reversa por `carga_id` solo encontraría los que alcanzaron a entrar.
func (r *pgRepository) SellarPagosExternos(ctx context.Context, empresaID, usuarioID string, sellos []SelloPagoExterno) (cargaID string, n int, err error) {
	if len(sellos) == 0 {
		return "", 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("bancos: abrir transacción del sello: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El id de la carga lo genera Postgres, que ya sabe: traerse una dependencia nueva solo para
	// esto sería agrandar el stack por un uuid.
	var carga string
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&carga); err != nil {
		return "", 0, fmt.Errorf("bancos: generar el id de la carga: %w", err)
	}

	const ins = `
		INSERT INTO pago_aplicado_externo
			(empresa_id, movimiento_bancario_id, referencia, documento, tercero,
			 fecha_aplicado, monto_reportado, nota, carga_id, cargado_por)
		SELECT $1::uuid, m.id, $3, $4, $5, NULLIF($6, '')::date, NULLIF($7, '')::numeric, $8,
		       $9::uuid, NULLIF($10, '')::uuid
		FROM movimiento_bancario m
		-- El movimiento se vuelve a verificar DENTRO del INSERT: entre la previsualización y la
		-- confirmación pueden haber pasado minutos, y en ese rato alguien pudo enlazarlo a una
		-- factura o revertir su carga. Sin esto se sellaría un movimiento que ya tiene dueño.
		WHERE m.id = $2::uuid AND m.empresa_id = $1::uuid
		  AND m.incluido AND m.debito > 0
		  AND m.documento_cxp_id IS NULL AND m.remesa_id IS NULL
		ON CONFLICT (movimiento_bancario_id) DO NOTHING`

	for _, s := range sellos {
		tag, err := tx.Exec(ctx, ins, empresaID, s.MovimientoID, s.Referencia, s.Documento,
			s.Tercero, s.FechaAplicado, s.MontoReportado, s.Nota, carga, usuarioID)
		if err != nil {
			return "", 0, fmt.Errorf("bancos: sellar el movimiento %s: %w", s.MovimientoID, err)
		}
		n += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, fmt.Errorf("bancos: confirmar los sellos: %w", err)
	}
	return carga, n, nil
}

// SellosDeMovimientos devuelve el sello de cada movimiento pedido, para pintarlo en la lista.
func (r *pgRepository) SellosDeMovimientos(ctx context.Context, empresaID string, movIDs []string) (map[string]SelloVisible, error) {
	out := map[string]SelloVisible{}
	if len(movIDs) == 0 {
		return out, nil
	}
	const q = `
		SELECT p.movimiento_bancario_id::text, p.referencia, p.documento, p.tercero,
		       COALESCE(to_char(p.fecha_aplicado, 'YYYY-MM-DD'), ''),
		       COALESCE(p.monto_reportado::text, '')
		FROM pago_aplicado_externo p
		WHERE p.empresa_id = $1::uuid AND p.movimiento_bancario_id = ANY($2::uuid[])`
	rows, err := r.pool.Query(ctx, q, empresaID, movIDs)
	if err != nil {
		return nil, fmt.Errorf("bancos: sellos de movimientos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s SelloVisible
		if err := rows.Scan(&id, &s.Referencia, &s.Documento, &s.Tercero, &s.FechaAplicado, &s.MontoReportado); err != nil {
			return nil, fmt.Errorf("bancos: scan sello: %w", err)
		}
		out[id] = s
	}
	return out, rows.Err()
}

// RevertirCargaDeSellos borra las marcas de una carga entera y devuelve cuántas quitó.
//
// Una importación equivocada puede sellar miles de movimientos. Sin reversa por carga habría que
// deshacerlos uno por uno, y nadie lo haría: quedarían marcados como resueltos para siempre.
func (r *pgRepository) RevertirCargaDeSellos(ctx context.Context, empresaID, cargaID string) (int, error) {
	const q = `DELETE FROM pago_aplicado_externo WHERE empresa_id = $1::uuid AND carga_id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, cargaID)
	if err != nil {
		return 0, fmt.Errorf("bancos: revertir la carga de sellos: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// ResumenSellos cuenta cuánto hay sellado, para el encabezado de la pantalla.
func (r *pgRepository) ResumenSellos(ctx context.Context, empresaID string) (ResumenPagosExternos, error) {
	var res ResumenPagosExternos
	const q = `
		SELECT COUNT(*)::int,
		       COALESCE(SUM(m.debito), 0)::text,
		       COUNT(DISTINCT p.carga_id)::int
		FROM pago_aplicado_externo p
		JOIN movimiento_bancario m ON m.id = p.movimiento_bancario_id
		WHERE p.empresa_id = $1::uuid AND m.incluido`
	if err := r.pool.QueryRow(ctx, q, empresaID).Scan(&res.Sellados, &res.MontoSellado, &res.Cargas); err != nil {
		return res, fmt.Errorf("bancos: resumen de sellos: %w", err)
	}
	// Lo que queda pendiente de verdad: el número que el Director quiere ver bajar.
	const qp = `
		SELECT COUNT(*)::int, COALESCE(SUM(m.debito), 0)::text
		FROM movimiento_bancario m
		WHERE m.empresa_id = $1::uuid AND m.incluido AND m.debito > 0
		  AND m.documento_cxp_id IS NULL AND m.remesa_id IS NULL
		  AND NOT EXISTS (SELECT 1 FROM pago_aplicado_externo p WHERE p.movimiento_bancario_id = m.id)`
	if err := r.pool.QueryRow(ctx, qp, empresaID).Scan(&res.Pendientes, &res.MontoPendiente); err != nil {
		return res, fmt.Errorf("bancos: pendientes sin explicar: %w", err)
	}
	return res, nil
}
