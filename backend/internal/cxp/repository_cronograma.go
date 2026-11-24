package cxp

// Lo que el cronograma necesita de la base (mig 0122). El porqué está en cronograma.go.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// LineasDelCronograma trae un renglón por proveedor y moneda, con de dónde sale el dinero.
//
// AGRUPA POR PROVEEDOR, igual que el cuadro que reemplaza: a quien recibe el cronograma le importa
// a quién se le paga y cuánto, no cuántas facturas lo componen.
//
// La cuenta y la marca de reserva salen de la cuenta asignada a la factura; si varias facturas del
// mismo proveedor salieran de cuentas distintas, se agrupan por cuenta también —son pagos
// distintos y el cuadro tiene que poder mostrarlos separados—.
func (r *pgRepository) LineasDelCronograma(ctx context.Context, empresaID, loteID string) ([]FilaCronogramaCruda, error) {
	// El monto es el NETO (lo que de verdad sale del banco), igual que el archivo de pago y el
	// tablero: el total no sale nunca, siempre va menos retención, anticipos, notas de crédito y
	// parciales. Un cronograma armado sobre el total prometería más plata de la que sale.
	q := `
		SELECT p.id::text, p.nombre, COALESCE(d.moneda, 'CRC'),
		       SUM(` + netoCRCSQL + `)::text,
		       COALESCE(cb.alias, ''), COALESCE(b.nombre, ''),
		       COALESCE(bool_or(cb.es_reserva), false),
		       bool_or(d.por_fuera),
		       bool_or(d.cuenta_pago_id IS NOT NULL)
		FROM documento_cxp d` + JoinsNetoSQL + `
		JOIN proveedor p ON p.id = d.proveedor_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = d.cuenta_pago_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid
		  AND NOT d.bloqueado_para_pago
		GROUP BY p.id, p.nombre, d.moneda, cb.alias, b.nombre
		ORDER BY p.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: líneas del cronograma: %w", err)
	}
	defer rows.Close()

	out := make([]FilaCronogramaCruda, 0, 32)
	for rows.Next() {
		var f FilaCronogramaCruda
		var monto string
		if err := rows.Scan(&f.ProveedorID, &f.Proveedor, &f.Moneda, &monto,
			&f.Cuenta, &f.Banco, &f.EsReserva, &f.PorFuera, &f.TieneCuenta); err != nil {
			return nil, fmt.Errorf("cxp: scan línea del cronograma: %w", err)
		}
		d, err := decimal.NewFromString(monto)
		if err != nil {
			return nil, fmt.Errorf("cxp: monto del cronograma (%q): %w", monto, err)
		}
		f.Monto = d
		out = append(out, f)
	}
	return out, rows.Err()
}

// TCDelCronograma lee el tipo de cambio guardado con el corte. Cero si nadie lo escribió.
func (r *pgRepository) TCDelCronograma(ctx context.Context, empresaID, loteID string) (decimal.Decimal, error) {
	const q = `SELECT COALESCE(tc_cronograma, 0)::text FROM lote_pago
	            WHERE empresa_id = $1::uuid AND id = $2::uuid`
	var s string
	err := r.pool.QueryRow(ctx, q, empresaID, loteID).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, ErrLoteNoEncontrado
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("cxp: tipo de cambio del cronograma: %w", err)
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("cxp: tipo de cambio inválido (%q): %w", s, err)
	}
	return d, nil
}

// GuardarTCDelCronograma deja el tipo de cambio que escribió tesorería.
//
// Se guarda CON EL CORTE y no se recalcula: reimprimir el cronograma del mes pasado tiene que dar
// las mismas cifras que dio entonces, aunque el dólar haya cambiado.
func (r *pgRepository) GuardarTCDelCronograma(ctx context.Context, empresaID, loteID string, tc decimal.Decimal) error {
	const q = `UPDATE lote_pago SET tc_cronograma = $3::numeric
	            WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, loteID, tc.String())
	if err != nil {
		return fmt.Errorf("cxp: guardar tipo de cambio del cronograma: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLoteNoEncontrado
	}
	return nil
}

// AsignarCuentaDePago dice de qué cuenta sale lo que se le paga a un proveedor en ese corte.
//
// Se asigna por PROVEEDOR y no por factura: así es como se decide en la práctica —«a este le
// pagamos de BP Negocios»— y así lo muestra el cuadro. Internamente marca todas sus facturas del
// corte, que es lo que después suma el cronograma.
func (r *pgRepository) AsignarCuentaDePago(ctx context.Context, empresaID, loteID, proveedorID, cuentaID string) (int, error) {
	// La cuenta tiene que ser de la MISMA empresa: sin esa comprobación se podría asignar una
	// cuenta ajena y el cronograma diría que el dinero sale de un banco de otra empresa.
	const q = `
		UPDATE documento_cxp d
		   SET cuenta_pago_id = NULLIF($4, '')::uuid, actualizado_en = now()
		 WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid AND d.proveedor_id = $3::uuid
		   AND ($4 = '' OR EXISTS (SELECT 1 FROM cuenta_bancaria c
		                            WHERE c.id = $4::uuid AND c.empresa_id = $1::uuid))`
	tag, err := r.pool.Exec(ctx, q, empresaID, loteID, proveedorID, cuentaID)
	if err != nil {
		return 0, fmt.Errorf("cxp: asignar cuenta de pago: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// MarcarPorFuera saca (o devuelve) del cronograma lo que se le paga a un proveedor en ese corte.
//
// Deja autor y fecha: sacar un pago del cronograma es una decisión, no un atributo de la factura.
// En el archivo real son ₡40,9 M de ₡51,9 M, así que sin saber quién lo decidió no se puede
// preguntar por qué el cronograma comunicó la quinta parte.
func (r *pgRepository) MarcarPorFuera(ctx context.Context, empresaID, loteID, proveedorID string, porFuera bool, usuarioID string) (int, error) {
	const q = `
		UPDATE documento_cxp
		   SET por_fuera = $4,
		       por_fuera_por = CASE WHEN $4 THEN $5::uuid ELSE NULL END,
		       por_fuera_en  = CASE WHEN $4 THEN now() ELSE NULL END,
		       actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND lote_id = $2::uuid AND proveedor_id = $3::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, loteID, proveedorID, porFuera, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("cxp: marcar por fuera: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
