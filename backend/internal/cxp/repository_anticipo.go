package cxp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// AnticiposDisponibles lista los anticipos pagados del proveedor con saldo a favor (> 0).
// v1: solo colones (CRC). Ordena por fecha de pago (o emisión) ascendente.
func (r *pgRepository) AnticiposDisponibles(ctx context.Context, empresaID, proveedorID string) ([]AnticipoSaldo, error) {
	const q = `
		SELECT a.id::text, COALESCE(a.consecutivo, ''),
		       to_char(COALESCE(a.fecha_pago_programada, a.fecha_emision), 'YYYY-MM-DD'),
		       a.total_crc::text,
		       COALESCE(SUM(aa.monto_crc), 0)::text,
		       (a.total_crc - COALESCE(SUM(aa.monto_crc), 0))::text
		FROM documento_cxp a
		LEFT JOIN anticipo_aplicacion aa ON aa.anticipo_id = a.id AND aa.activo
		WHERE a.empresa_id = $1::uuid AND a.proveedor_id = $2::uuid AND a.tipo = 'ANTICIPO'
		  AND a.estado IN ('PAGADO', 'CONCILIADO') AND a.moneda = 'CRC'
		GROUP BY a.id
		HAVING (a.total_crc - COALESCE(SUM(aa.monto_crc), 0)) > 0
		ORDER BY COALESCE(a.fecha_pago_programada, a.fecha_emision), a.id`
	rows, err := r.pool.Query(ctx, q, empresaID, proveedorID)
	if err != nil {
		return nil, fmt.Errorf("cxp: anticipos disponibles: %w", err)
	}
	defer rows.Close()
	out := make([]AnticipoSaldo, 0)
	for rows.Next() {
		var a AnticipoSaldo
		if err := rows.Scan(&a.ID, &a.Consecutivo, &a.FechaPago, &a.TotalCRC, &a.Aplicado, &a.Saldo); err != nil {
			return nil, fmt.Errorf("cxp: scan anticipo disponible: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AnticiposEmpresa lista los anticipos vivos de la empresa (billetera global) con el nombre del
// proveedor y su estado: los PAGADOS con saldo son aplicables; los que aún están en el flujo
// (RECIBIDO…PROGRAMADO) se listan como "en trámite" para que el usuario vea que ya existen.
// Excluye los archivados (denegado/anulado/liquidada). Ordena por proveedor y fecha.
func (r *pgRepository) AnticiposEmpresa(ctx context.Context, empresaID string) ([]AnticipoSaldo, error) {
	const q = `
		SELECT a.id::text, COALESCE(a.consecutivo, ''),
		       to_char(COALESCE(a.fecha_pago_programada, a.fecha_emision), 'YYYY-MM-DD'),
		       a.total_crc::text,
		       COALESCE(SUM(aa.monto_crc), 0)::text,
		       (a.total_crc - COALESCE(SUM(aa.monto_crc), 0))::text,
		       a.proveedor_id::text, p.nombre, a.estado
		FROM documento_cxp a
		JOIN proveedor p ON p.id = a.proveedor_id
		LEFT JOIN anticipo_aplicacion aa ON aa.anticipo_id = a.id AND aa.activo
		WHERE a.empresa_id = $1::uuid AND a.tipo = 'ANTICIPO' AND a.moneda = 'CRC'
		  AND a.estado NOT IN ('DENEGADO', 'ANULADO', 'LIQUIDADA')
		GROUP BY a.id, p.nombre
		HAVING (a.total_crc - COALESCE(SUM(aa.monto_crc), 0)) > 0
		ORDER BY p.nombre, COALESCE(a.fecha_pago_programada, a.fecha_emision), a.id`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: anticipos empresa: %w", err)
	}
	defer rows.Close()
	out := make([]AnticipoSaldo, 0)
	for rows.Next() {
		var a AnticipoSaldo
		if err := rows.Scan(&a.ID, &a.Consecutivo, &a.FechaPago, &a.TotalCRC, &a.Aplicado, &a.Saldo,
			&a.ProveedorID, &a.Proveedor, &a.Estado); err != nil {
			return nil, fmt.Errorf("cxp: scan anticipo empresa: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaldoAnticipo devuelve el saldo disponible (total_crc − aplicaciones activas) de un anticipo.
func (r *pgRepository) SaldoAnticipo(ctx context.Context, empresaID, anticipoID string) (decimal.Decimal, error) {
	const q = `
		SELECT (a.total_crc - COALESCE((SELECT SUM(x.monto_crc) FROM anticipo_aplicacion x WHERE x.anticipo_id = a.id AND x.activo), 0))::text
		FROM documento_cxp a
		WHERE a.id = $2::uuid AND a.empresa_id = $1::uuid AND a.tipo = 'ANTICIPO'`
	var s string
	err := r.pool.QueryRow(ctx, q, empresaID, anticipoID).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, ErrNoEsAnticipo
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("cxp: saldo anticipo: %w", err)
	}
	return decimal.NewFromString(s)
}

// querier abstrae pool y transacción: la misma guarda sirve para una aplicación sola o en lote.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// sqlAplicarAnticipo inserta la aplicación SOLO si el monto cabe a la vez en el saldo del anticipo
// y en el neto restante de la factura (guarda atómica anti-carrera). 0 filas => monto inválido.
// Dentro de una transacción cada INSERT ve los anteriores, así que los topes son acumulativos.
const sqlAplicarAnticipo = `
	INSERT INTO anticipo_aplicacion (empresa_id, anticipo_id, factura_id, monto_crc, aplicado_por)
	SELECT $1::uuid, $2::uuid, $3::uuid, $4::numeric, $5::uuid
	WHERE $4::numeric > 0
	  AND $4::numeric <= (SELECT a.total_crc - COALESCE((SELECT SUM(x.monto_crc) FROM anticipo_aplicacion x WHERE x.anticipo_id = $2::uuid AND x.activo), 0)
	                      FROM documento_cxp a WHERE a.id = $2::uuid AND a.empresa_id = $1::uuid)
	  AND $4::numeric <= (SELECT f.total_crc - COALESCE((SELECT SUM(y.monto_crc) FROM anticipo_aplicacion y WHERE y.factura_id = $3::uuid AND y.activo), 0)
	                      FROM documento_cxp f WHERE f.id = $3::uuid AND f.empresa_id = $1::uuid)
	RETURNING id::text`

func aplicarAnticipoEn(ctx context.Context, q querier, empresaID, anticipoID, facturaID string, monto decimal.Decimal, usuarioID string) (string, error) {
	var id string
	err := q.QueryRow(ctx, sqlAplicarAnticipo, empresaID, anticipoID, facturaID, monto, usuarioID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMontoAplicacionInvalido
	}
	if err != nil {
		return "", fmt.Errorf("cxp: aplicar anticipo: %w", err)
	}
	return id, nil
}

// AplicarAnticipo aplica un solo anticipo a la factura.
func (r *pgRepository) AplicarAnticipo(ctx context.Context, empresaID, anticipoID, facturaID string, monto decimal.Decimal, usuarioID string) (string, error) {
	return aplicarAnticipoEn(ctx, r.pool, empresaID, anticipoID, facturaID, monto, usuarioID)
}

// AplicarAnticiposLote aplica VARIOS anticipos a la misma factura en una sola transacción:
// si alguno no cabe, no se aplica ninguno (todo-o-nada; es plata del proveedor).
func (r *pgRepository) AplicarAnticiposLote(ctx context.Context, empresaID, facturaID string, apps []AplicacionInput, usuarioID string) error {
	if len(apps) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cxp: aplicar anticipos en lote: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, a := range apps {
		if _, err := aplicarAnticipoEn(ctx, tx, empresaID, a.AnticipoID, facturaID, a.Monto, usuarioID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cxp: confirmar aplicación en lote: %w", err)
	}
	return nil
}

// ReversarAplicacion desactiva una aplicación (soft) de la factura indicada, solo si esa
// factura aún no fue pagada. Verifica pertenencia (empresa + factura) para tenant-safety.
func (r *pgRepository) ReversarAplicacion(ctx context.Context, empresaID, facturaID, aplicacionID, usuarioID string) error {
	const q = `
		UPDATE anticipo_aplicacion aa
		SET activo = false, reversado_por = $4::uuid, reversado_en = now()
		WHERE aa.id = $3::uuid AND aa.empresa_id = $1::uuid AND aa.factura_id = $2::uuid AND aa.activo
		  AND EXISTS (SELECT 1 FROM documento_cxp f WHERE f.id = aa.factura_id AND f.estado NOT IN ('PAGADO', 'CONCILIADO'))`
	tag, err := r.pool.Exec(ctx, q, empresaID, facturaID, aplicacionID, usuarioID)
	if err != nil {
		return fmt.Errorf("cxp: reversar aplicación: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	// 0 filas: distinguir "no existe/ya reversada" de "factura ya pagada".
	var existeActiva bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM anticipo_aplicacion WHERE id = $3::uuid AND empresa_id = $1::uuid AND factura_id = $2::uuid AND activo)`,
		empresaID, facturaID, aplicacionID).Scan(&existeActiva); err != nil {
		return fmt.Errorf("cxp: verificar aplicación: %w", err)
	}
	if existeActiva {
		return ErrReversaNoPermitida
	}
	return ErrAplicacionNoEncontrada
}

// AplicacionesDeFactura lista los anticipos aplicados (activos) a una factura.
func (r *pgRepository) AplicacionesDeFactura(ctx context.Context, empresaID, facturaID string) ([]AplicacionAnticipo, error) {
	const q = `
		SELECT aa.id::text, aa.anticipo_id::text, COALESCE(a.consecutivo, ''), aa.monto_crc::text,
		       COALESCE(NULLIF(u.nombre, ''), u.email, ''),
		       to_char(aa.aplicado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM anticipo_aplicacion aa
		JOIN documento_cxp a ON a.id = aa.anticipo_id
		LEFT JOIN usuario u ON u.id = aa.aplicado_por
		WHERE aa.empresa_id = $1::uuid AND aa.factura_id = $2::uuid AND aa.activo
		ORDER BY aa.aplicado_en`
	rows, err := r.pool.Query(ctx, q, empresaID, facturaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: aplicaciones de factura: %w", err)
	}
	defer rows.Close()
	out := make([]AplicacionAnticipo, 0)
	for rows.Next() {
		var a AplicacionAnticipo
		if err := rows.Scan(&a.ID, &a.AnticipoID, &a.AnticipoConsecutivo, &a.MontoCRC, &a.AplicadoPorNombre, &a.AplicadoEn); err != nil {
			return nil, fmt.Errorf("cxp: scan aplicación: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
