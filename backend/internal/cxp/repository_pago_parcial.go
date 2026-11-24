package cxp

// Consultas del pago parcial (mig 0105). El porqué está en pago_parcial.go.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// RegistrarPagoParcial escribe la aplicación comprobando las tres invariantes EN LA MISMA
// TRANSACCIÓN y con la fila del movimiento bloqueada.
//
// NO se valida leyendo primero y escribiendo después: entre las dos consultas otro registro puede
// aplicar el mismo débito, y las dos sumas pasarían su chequeo por separado para dejar el
// movimiento sobreaplicado. Es la misma razón por la que la guarda de excluir un movimiento vive
// en su WHERE.
func (r *pgRepository) RegistrarPagoParcial(ctx context.Context, empresaID, documentoID, movimientoID string, monto decimal.Decimal, moneda, motivo, usuarioID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("cxp: begin pago parcial: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El movimiento, bloqueado: nadie más puede aplicarlo mientras se decide.
	var debito decimal.Decimal
	var yaApuntaAAlgo bool
	if err := tx.QueryRow(ctx, `
		SELECT m.debito, (m.documento_cxp_id IS NOT NULL OR m.remesa_id IS NOT NULL)
		FROM movimiento_bancario m
		WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid AND m.incluido AND m.debito > 0
		FOR UPDATE`, empresaID, movimientoID).Scan(&debito, &yaApuntaAAlgo); err != nil {
		return "", fmt.Errorf("cxp: movimiento del parcial: %w", err)
	}
	// Invariante 3.
	if yaApuntaAAlgo {
		return "", ErrMovimientoYaAplicado
	}

	// Invariante 2: lo aplicado desde este movimiento no puede pasarse de su débito.
	var aplicadoDelMov decimal.Decimal
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(monto), 0) FROM pago_parcial
		WHERE empresa_id = $1::uuid AND movimiento_id = $2::uuid AND activo`,
		empresaID, movimientoID).Scan(&aplicadoDelMov); err != nil {
		return "", fmt.Errorf("cxp: aplicado del movimiento: %w", err)
	}
	if aplicadoDelMov.Add(monto).GreaterThan(debito) {
		return "", ErrParcialExcedeMovimiento
	}

	// Invariante 1: lo aplicado a esta factura no puede pasarse de lo que todavía debe. El neto ya
	// descuenta los parciales vivos, así que comparar contra él alcanza.
	var neto decimal.Decimal
	if err := tx.QueryRow(ctx,
		`SELECT `+netoAPagarSQL+` FROM documento_cxp d WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid`,
		empresaID, documentoID).Scan(&neto); err != nil {
		return "", fmt.Errorf("cxp: neto de la factura: %w", err)
	}
	if monto.GreaterThan(neto) {
		return "", ErrParcialExcedeFactura
	}

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO pago_parcial (empresa_id, documento_id, movimiento_id, monto, moneda, motivo, aplicado_por)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, NULLIF($7,'')::uuid)
		RETURNING id::text`,
		empresaID, documentoID, movimientoID, monto, moneda, motivo, usuarioID).Scan(&id); err != nil {
		return "", fmt.Errorf("cxp: insertar pago parcial: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("cxp: commit pago parcial: %w", err)
	}
	return id, nil
}

// SaldoDeFactura devuelve el neto original, lo ya aplicado y lo que falta, con el detalle.
func (r *pgRepository) SaldoDeFactura(ctx context.Context, empresaID, documentoID string) (SaldoDeFactura, error) {
	out := SaldoDeFactura{DocumentoID: documentoID, Parciales: make([]PagoParcial, 0)}

	// `netoAPagarSQL` ya resta los parciales, así que ES el saldo. El neto original se recompone
	// sumándole lo aplicado: así los tres números que se muestran cierran entre sí y quien los lee
	// puede verificar la resta.
	if err := r.pool.QueryRow(ctx, `
		SELECT `+netoAPagarSQL+`::text,
		       COALESCE((SELECT SUM(pp.monto) FROM pago_parcial pp
		                 WHERE pp.documento_id = d.id AND pp.activo), 0)::text,
		       COALESCE(d.moneda, 'CRC')
		FROM documento_cxp d WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid`,
		empresaID, documentoID).Scan(&out.Saldo, &out.Pagado, &out.Moneda); err != nil {
		return SaldoDeFactura{}, fmt.Errorf("cxp: saldo de la factura: %w", err)
	}
	saldo, _ := decimal.NewFromString(out.Saldo)
	pagado, _ := decimal.NewFromString(out.Pagado)
	out.Neto = saldo.Add(pagado).StringFixed(2)

	rows, err := r.pool.Query(ctx, `
		SELECT pp.id::text, pp.documento_id::text, pp.movimiento_id::text, pp.monto::text,
		       pp.moneda, pp.motivo,
		       to_char(pp.aplicado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       to_char(m.fecha, 'YYYY-MM-DD'), COALESCE(m.descripcion,''), COALESCE(cb.alias,'')
		FROM pago_parcial pp
		JOIN movimiento_bancario m ON m.id = pp.movimiento_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		WHERE pp.empresa_id = $1::uuid AND pp.documento_id = $2::uuid AND pp.activo
		ORDER BY m.fecha, pp.aplicado_en`, empresaID, documentoID)
	if err != nil {
		return SaldoDeFactura{}, fmt.Errorf("cxp: parciales de la factura: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p PagoParcial
		if err := rows.Scan(&p.ID, &p.DocumentoID, &p.MovimientoID, &p.Monto, &p.Moneda, &p.Motivo,
			&p.AplicadoEn, &p.Fecha, &p.Descripcion, &p.Cuenta); err != nil {
			return SaldoDeFactura{}, fmt.Errorf("cxp: scan parcial: %w", err)
		}
		out.Parciales = append(out.Parciales, p)
	}
	return out, rows.Err()
}

// MandarACobrarElSaldo devuelve la factura al circuito de pago por lo que todavía debe.
//
// La saca del lote anterior (`lote_id = NULL`): el corte donde estaba describe un pago que ya se
// hizo —por la parte que entró— y el saldo es un pago distinto, que va a salir en otro corte con
// su propia macro. Dejarla colgando del lote viejo haría que ese lote dijera contener algo que ya
// no corresponde a lo que se le subió al banco.
func (r *pgRepository) MandarACobrarElSaldo(ctx context.Context, empresaID, documentoID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE documento_cxp d SET estado = 'PROGRAMADO', lote_id = NULL, actualizado_en = now()
		WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid
		  AND d.estado IN ('PAGADO', 'PROGRAMADO')
		  AND `+netoAPagarSQL+` > 0
		  AND EXISTS (SELECT 1 FROM pago_parcial pp WHERE pp.documento_id = d.id AND pp.activo)`,
		empresaID, documentoID)
	if err != nil {
		return 0, fmt.Errorf("cxp: mandar a cobrar el saldo: %w", err)
	}
	return tag.RowsAffected(), nil
}
