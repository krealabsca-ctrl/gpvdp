package cxp

// Consultas de la nota de crédito (mig 0106). El porqué está en nota_credito.go.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// AplicarNotaCredito escribe la rebaja verificando las dos sumas EN LA MISMA TRANSACCIÓN y con la
// nota bloqueada.
//
// Las dos son sumas entre filas, así que no caben en un CHECK, y no se validan leyendo primero y
// escribiendo después: entre las dos consultas otra aplicación puede consumir la misma nota y las
// dos pasarían su chequeo por separado para dejarla sobreaplicada. Es la misma disciplina del
// pago parcial y del anticipo.
func (r *pgRepository) AplicarNotaCredito(ctx context.Context, empresaID, notaID, facturaID string, monto decimal.Decimal, usuarioID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("cxp: begin nota de crédito: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// La nota, bloqueada: nadie más puede consumirla mientras se decide.
	var totalNota decimal.Decimal
	if err := tx.QueryRow(ctx, `
		SELECT d.total FROM documento_cxp d
		WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid AND d.tipo = $3
		FOR UPDATE`, empresaID, notaID, TipoNotaCredito).Scan(&totalNota); err != nil {
		return "", fmt.Errorf("cxp: nota de crédito: %w", err)
	}

	var yaAplicado decimal.Decimal
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(monto), 0) FROM cxp_nota_credito_aplicacion
		WHERE empresa_id = $1::uuid AND nota_id = $2::uuid AND activo`,
		empresaID, notaID).Scan(&yaAplicado); err != nil {
		return "", fmt.Errorf("cxp: aplicado de la nota: %w", err)
	}
	if yaAplicado.Add(monto).GreaterThan(totalNota) {
		return "", ErrNotaExcedeSuSaldo
	}

	// Lo que la factura todavía debe. `netoAPagarSQL` ya resta las notas vivas, así que comparar
	// contra él alcanza y no hace falta recomponer nada.
	var neto decimal.Decimal
	if err := tx.QueryRow(ctx,
		`SELECT `+netoAPagarSQL+` FROM documento_cxp d WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid`,
		empresaID, facturaID).Scan(&neto); err != nil {
		return "", fmt.Errorf("cxp: neto de la factura: %w", err)
	}
	if monto.GreaterThan(neto) {
		return "", ErrNotaExcedeFactura
	}

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO cxp_nota_credito_aplicacion (empresa_id, nota_id, factura_id, monto, aplicado_por)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, NULLIF($5,'')::uuid)
		RETURNING id::text`,
		empresaID, notaID, facturaID, monto, usuarioID).Scan(&id); err != nil {
		return "", fmt.Errorf("cxp: insertar aplicación de nota: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("cxp: commit nota de crédito: %w", err)
	}
	return id, nil
}

// NotasDeCreditoDisponibles lista las notas de un proveedor que todavía tienen saldo.
//
// Se filtra por proveedor porque una nota rebaja lo que le debemos a QUIEN la emitió: ofrecer las
// de otro proveedor invita al error que el servicio después rechaza, y una lista que propone lo
// que no se puede hacer es peor que una lista corta.
func (r *pgRepository) NotasDeCreditoDisponibles(ctx context.Context, empresaID, proveedorID string) ([]NotaCreditoSaldo, error) {
	const q = `
		SELECT d.id::text, COALESCE(d.clave,''), COALESCE(d.consecutivo,''),
		       COALESCE(p.nombre,''), d.proveedor_id::text,
		       COALESCE(to_char(d.fecha_emision,'YYYY-MM-DD'),''), COALESCE(d.moneda,'CRC'),
		       d.total::text,
		       COALESCE(ap.aplicado, 0)::text,
		       (d.total - COALESCE(ap.aplicado, 0))::text
		FROM documento_cxp d
		LEFT JOIN proveedor p ON p.id = d.proveedor_id
		LEFT JOIN LATERAL (
		    SELECT SUM(nc.monto) AS aplicado FROM cxp_nota_credito_aplicacion nc
		    WHERE nc.nota_id = d.id AND nc.activo
		) ap ON true
		WHERE d.empresa_id = $1::uuid AND d.tipo = $2
		  AND ($3 = '' OR d.proveedor_id = NULLIF($3,'')::uuid)
		  AND d.total > COALESCE(ap.aplicado, 0)
		ORDER BY d.fecha_emision DESC, d.id`
	rows, err := r.pool.Query(ctx, q, empresaID, TipoNotaCredito, proveedorID)
	if err != nil {
		return nil, fmt.Errorf("cxp: notas de crédito disponibles: %w", err)
	}
	defer rows.Close()
	out := make([]NotaCreditoSaldo, 0)
	for rows.Next() {
		var n NotaCreditoSaldo
		if err := rows.Scan(&n.ID, &n.Clave, &n.Consecutivo, &n.Proveedor, &n.ProveedorID,
			&n.Fecha, &n.Moneda, &n.Total, &n.Aplicado, &n.Saldo); err != nil {
			return nil, fmt.Errorf("cxp: scan nota: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// NotasAplicadasAFactura lista las rebajas vivas sobre una factura.
func (r *pgRepository) NotasAplicadasAFactura(ctx context.Context, empresaID, facturaID string) ([]AplicacionNotaCredito, error) {
	const q = `
		SELECT nc.id::text, nc.nota_id::text, nc.factura_id::text, nc.monto::text,
		       COALESCE(n.consecutivo,''), COALESCE(n.clave,''),
		       to_char(nc.aplicado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM cxp_nota_credito_aplicacion nc
		JOIN documento_cxp n ON n.id = nc.nota_id
		WHERE nc.empresa_id = $1::uuid AND nc.factura_id = $2::uuid AND nc.activo
		ORDER BY nc.aplicado_en`
	rows, err := r.pool.Query(ctx, q, empresaID, facturaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: notas aplicadas: %w", err)
	}
	defer rows.Close()
	out := make([]AplicacionNotaCredito, 0)
	for rows.Next() {
		var a AplicacionNotaCredito
		if err := rows.Scan(&a.ID, &a.NotaID, &a.FacturaID, &a.Monto, &a.Consecutivo, &a.Clave,
			&a.AplicadoEn); err != nil {
			return nil, fmt.Errorf("cxp: scan aplicación de nota: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ReversarNotaCredito deshace una aplicación. No borra: deja la huella.
func (r *pgRepository) ReversarNotaCredito(ctx context.Context, empresaID, aplicacionID, usuarioID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE cxp_nota_credito_aplicacion
		SET activo = false, reversado_por = NULLIF($3,'')::uuid, reversado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND activo`,
		empresaID, aplicacionID, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("cxp: reversar nota de crédito: %w", err)
	}
	return tag.RowsAffected(), nil
}
