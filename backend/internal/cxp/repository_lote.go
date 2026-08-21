package cxp

import (
	"context"
	"fmt"
)

// CrearLote crea un lote de pago para la fecha de corte y le asigna las facturas indicadas que
// estén PROGRAMADO y sin lote (tenant-safe). Devuelve el lote con número, cantidad y total.
func (r *pgRepository) CrearLote(ctx context.Context, empresaID, fechaCorte string, ids []string, usuarioID string) (LotePago, error) {
	if len(ids) == 0 {
		return LotePago{}, ErrSinDocumentos
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return LotePago{}, fmt.Errorf("cxp: begin lote: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lote LotePago
	const insLote = `
		INSERT INTO lote_pago (empresa_id, numero, fecha_corte, creado_por)
		VALUES ($1::uuid,
		        (SELECT COALESCE(MAX(numero), 0) + 1 FROM lote_pago WHERE empresa_id = $1::uuid),
		        $2::date, $3::uuid)
		RETURNING id::text, numero, to_char(fecha_corte, 'YYYY-MM-DD'), estado,
		          to_char(creado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF')`
	if err := tx.QueryRow(ctx, insLote, empresaID, fechaCorte, usuarioID).
		Scan(&lote.ID, &lote.Numero, &lote.FechaCorte, &lote.Estado, &lote.CreadoEn); err != nil {
		return LotePago{}, fmt.Errorf("cxp: crear lote: %w", err)
	}

	const asignar = `
		UPDATE documento_cxp SET lote_id = $3::uuid, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND estado = 'PROGRAMADO' AND lote_id IS NULL AND id = ANY($2::uuid[])`
	tag, err := tx.Exec(ctx, asignar, empresaID, ids, lote.ID)
	if err != nil {
		return LotePago{}, fmt.Errorf("cxp: asignar lote: %w", err)
	}
	lote.Cantidad = int(tag.RowsAffected())

	// Total del lote = suma de NETOS (descontando anticipos aplicados): es lo que se va a pagar.
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(GREATEST(d.total_crc - COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa WHERE aa.factura_id = d.id AND aa.activo), 0), 0)), 0)::text
		 FROM documento_cxp d WHERE d.lote_id = $1::uuid`,
		lote.ID).Scan(&lote.TotalCRC); err != nil {
		return LotePago{}, fmt.Errorf("cxp: total lote: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return LotePago{}, fmt.Errorf("cxp: commit lote: %w", err)
	}
	return lote, nil
}

// ListarLotes lista los lotes de la empresa con cantidad y total (más reciente primero).
func (r *pgRepository) ListarLotes(ctx context.Context, empresaID string) ([]LotePago, error) {
	const q = `
		SELECT l.id::text, l.numero, to_char(l.fecha_corte, 'YYYY-MM-DD'), l.estado,
		       COUNT(d.id)::int,
		       COALESCE(SUM(GREATEST(d.total_crc - COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa WHERE aa.factura_id = d.id AND aa.activo), 0), 0)), 0)::text,
		       to_char(l.creado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       COUNT(d.id) FILTER (WHERE d.estado IN ('PAGADO', 'CONCILIADO'))::int,
		       COUNT(d.id) FILTER (WHERE d.estado = 'REBOTADA')::int,
		       COUNT(d.id) FILTER (WHERE d.estado = 'PROGRAMADO')::int
		FROM lote_pago l
		LEFT JOIN documento_cxp d ON d.lote_id = l.id
		WHERE l.empresa_id = $1::uuid
		GROUP BY l.id, l.numero, l.fecha_corte, l.estado, l.creado_en
		ORDER BY l.creado_en DESC`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar lotes: %w", err)
	}
	defer rows.Close()
	out := make([]LotePago, 0)
	for rows.Next() {
		var l LotePago
		if err := rows.Scan(&l.ID, &l.Numero, &l.FechaCorte, &l.Estado, &l.Cantidad, &l.TotalCRC, &l.CreadoEn,
			&l.Pagadas, &l.Rebotadas, &l.Pendientes); err != nil {
			return nil, fmt.Errorf("cxp: scan lote: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DocumentosParaPagoPorLote devuelve las líneas de pago (para la macro) de todas las facturas del lote.
func (r *pgRepository) DocumentosParaPagoPorLote(ctx context.Context, empresaID, loteID string) ([]PagoRow, error) {
	// El monto que va al banco es el NETO: total − retención − anticipos aplicados. Sin restar
	// los anticipos se le pagaría al proveedor el total de una factura ya neteada (doble pago).
	const q = `
		SELECT COALESCE(p.identificacion, ''), p.nombre, COALESCE(p.iban, ''), d.moneda,
		       GREATEST(d.total - d.retencion - COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa WHERE aa.factura_id = d.id AND aa.activo), 0), 0)::text,
		       COALESCE(d.huella, ''), COALESCE(d.consecutivo, ''), d.id::text
		FROM documento_cxp d
		JOIN proveedor p ON p.id = d.proveedor_id
		WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid
		ORDER BY p.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: documentos de lote: %w", err)
	}
	defer rows.Close()
	var out []PagoRow
	for rows.Next() {
		var pr PagoRow
		if err := rows.Scan(&pr.Cedula, &pr.Nombre, &pr.IBAN, &pr.Moneda, &pr.MontoNeto,
			&pr.Descripcion, &pr.Consecutivo, &pr.DocumentoID); err != nil {
			return nil, fmt.Errorf("cxp: scan pago lote: %w", err)
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// ProgramarAprobados programa en bloque las facturas APROBADAS del corte (fecha de pago = corte)
// y les genera su huella. Se usa al crear el lote para que "aprobada" pueda ir directo al corte.
func (r *pgRepository) ProgramarAprobados(ctx context.Context, empresaID string, ids []string, fecha string) (int64, error) {
	const q = `
		UPDATE documento_cxp
		SET estado = 'PROGRAMADO', fecha_pago_programada = $3::date,
		    huella = 'CXP-' || UPPER(LEFT(REPLACE(id::text, '-', ''), 12)), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = ANY($2::uuid[]) AND estado = 'APROBADO'`
	tag, err := r.pool.Exec(ctx, q, empresaID, ids, fecha)
	if err != nil {
		return 0, fmt.Errorf("cxp: programar aprobados: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Reintentar saca una factura REBOTADA de su lote y la vuelve a PROGRAMADO (para un nuevo corte).
func (r *pgRepository) Reintentar(ctx context.Context, empresaID, id string) (int64, error) {
	const q = `UPDATE documento_cxp SET estado = 'PROGRAMADO', lote_id = NULL, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'REBOTADA'`
	tag, err := r.pool.Exec(ctx, q, empresaID, id)
	if err != nil {
		return 0, fmt.Errorf("cxp: reintentar: %w", err)
	}
	return tag.RowsAffected(), nil
}
