package bancos

// Consultas de saldos diarios y del checklist de carga. Todas filtran por empresa_id.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// hoyCRSQL es el día de operación de Costa Rica (UTC−6, sin horario de verano). El día hábil
// no es el del servidor ni el UTC: después de las 18:00 CR el UTC ya cambió de fecha.
const hoyCRSQL = `(now() AT TIME ZONE 'America/Costa_Rica')::date`

// SaldosDelDia devuelve una fila por cuenta ACTIVA con su saldo anterior, los movimientos del
// día ya cargados, el saldo capturado (si existe) y hasta cuándo llegan sus movimientos.
//
// El saldo anterior es el último capturado ANTES de la fecha pedida — no necesariamente el del
// día anterior: si nadie capturó el fin de semana, se compara contra el viernes, y los
// movimientos que se suman son los de todo ese tramo. Así el cuadre nunca miente por un hueco
// de captura.
func (r *pgRepository) SaldosDelDia(ctx context.Context, empresaID, fecha string) ([]SaldoDelDia, string, error) {
	const q = `
		WITH cuentas AS (
			SELECT cb.id, cb.alias, cb.moneda, b.nombre AS banco
			FROM cuenta_bancaria cb JOIN banco b ON b.id = cb.banco_id
			WHERE cb.empresa_id = $1::uuid AND cb.activo
		),
		anterior AS (
			SELECT DISTINCT ON (s.cuenta_bancaria_id)
			       s.cuenta_bancaria_id, s.saldo, s.fecha
			FROM saldo_cuenta_diario s
			WHERE s.empresa_id = $1::uuid AND s.fecha < $2::date
			ORDER BY s.cuenta_bancaria_id, s.fecha DESC
		),
		-- Movimientos entre el día del saldo anterior (excluido) y la fecha pedida (incluida).
		-- Sin saldo anterior se toma solo el día pedido: no hay contra qué acumular.
		movs AS (
			SELECT c.id AS cuenta_id,
			       COALESCE(SUM(m.credito), 0) AS entradas,
			       COALESCE(SUM(m.debito), 0) AS salidas
			FROM cuentas c
			LEFT JOIN movimiento_bancario m
			       ON m.cuenta_bancaria_id = c.id AND m.empresa_id = $1::uuid AND m.incluido
			      AND m.fecha > COALESCE((SELECT a.fecha FROM anterior a WHERE a.cuenta_bancaria_id = c.id),
			                             $2::date - INTERVAL '1 day')
			      AND m.fecha <= $2::date
			GROUP BY c.id
		),
		ultimo AS (
			SELECT m.cuenta_bancaria_id, MAX(m.fecha) AS ultima
			FROM movimiento_bancario m
			WHERE m.empresa_id = $1::uuid AND m.incluido
			GROUP BY m.cuenta_bancaria_id
		)
		SELECT c.id::text, c.alias, c.banco, c.moneda,
		       COALESCE(a.saldo::text, ''), COALESCE(to_char(a.fecha, 'YYYY-MM-DD'), ''),
		       mv.entradas::text, mv.salidas::text,
		       COALESCE(s.saldo::text, ''), COALESCE(s.nota, ''),
		       COALESCE(to_char(s.capturado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		       s.revisado_en IS NOT NULL,
		       COALESCE(to_char(s.revisado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		       COALESCE(to_char(u.ultima, 'YYYY-MM-DD'), ''),
		       COALESCE(` + hoyCRSQL + ` - u.ultima, 0),
		       to_char(` + hoyCRSQL + `, 'YYYY-MM-DD')
		FROM cuentas c
		JOIN movs mv ON mv.cuenta_id = c.id
		LEFT JOIN anterior a ON a.cuenta_bancaria_id = c.id
		LEFT JOIN ultimo u ON u.cuenta_bancaria_id = c.id
		LEFT JOIN saldo_cuenta_diario s
		       ON s.cuenta_bancaria_id = c.id AND s.empresa_id = $1::uuid AND s.fecha = $2::date
		ORDER BY c.banco, c.alias`
	rows, err := r.pool.Query(ctx, q, empresaID, fecha)
	if err != nil {
		return nil, "", fmt.Errorf("bancos: saldos del día: %w", err)
	}
	defer rows.Close()
	out := make([]SaldoDelDia, 0, 16)
	hoy := ""
	for rows.Next() {
		var s SaldoDelDia
		if err := rows.Scan(&s.CuentaID, &s.Alias, &s.Banco, &s.Moneda,
			&s.SaldoAnterior, &s.FechaAnterior, &s.EntradasDia, &s.SalidasDia,
			&s.Saldo, &s.Nota, &s.CapturadoEn, &s.Congelado, &s.RevisadoEn,
			&s.UltimoMovimiento, &s.DiasSinCargar, &hoy); err != nil {
			return nil, "", fmt.Errorf("bancos: scan saldo del día: %w", err)
		}
		out = append(out, s)
	}
	return out, hoy, rows.Err()
}

// SerieSaldos devuelve el disponible en colones de los últimos n días hasta la fecha (solo de
// los saldos efectivamente capturados: la serie no inventa días).
func (r *pgRepository) SerieSaldos(ctx context.Context, empresaID, fecha string, dias int) ([]PuntoSaldo, error) {
	const q = `
		SELECT to_char(d.dia, 'YYYY-MM-DD'),
		       COALESCE(SUM(s.saldo) FILTER (WHERE cb.moneda = 'CRC'), 0)::text,
		       COUNT(s.id)::int
		FROM generate_series($2::date - ($3::int - 1), $2::date, INTERVAL '1 day') AS d(dia)
		LEFT JOIN saldo_cuenta_diario s
		       ON s.empresa_id = $1::uuid AND s.fecha = d.dia::date
		LEFT JOIN cuenta_bancaria cb ON cb.id = s.cuenta_bancaria_id
		GROUP BY d.dia ORDER BY d.dia`
	rows, err := r.pool.Query(ctx, q, empresaID, fecha, dias)
	if err != nil {
		return nil, fmt.Errorf("bancos: serie de saldos: %w", err)
	}
	defer rows.Close()
	out := make([]PuntoSaldo, 0, dias)
	for rows.Next() {
		var p PuntoSaldo
		if err := rows.Scan(&p.Fecha, &p.MontoCRC, &p.Capturadas); err != nil {
			return nil, fmt.Errorf("bancos: scan punto de serie: %w", err)
		}
		p.EsHoy = p.Fecha == fecha
		out = append(out, p)
	}
	return out, rows.Err()
}

// GuardarSaldos registra (o corrige) el saldo de varias cuentas en una fecha, en UNA
// transacción: la captura del día es un acto único, no queda a medias.
//
// La cuenta se valida contra la empresa DENTRO del INSERT (tenant-safe: no alcanza con mandar
// un id ajeno). Devuelve cuántas filas se guardaron.
func (r *pgRepository) GuardarSaldos(ctx context.Context, empresaID, fecha string, saldos []SaldoInput, usuarioID string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("bancos: begin saldos: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El DO UPDATE respeta el congelamiento: un saldo ya revisado por Dirección Financiera no
	// se sobrescribe (decisión del 2026-07-31). Para corregirlo hay que descongelarlo, y eso
	// queda auditado.
	const q = `
		INSERT INTO saldo_cuenta_diario (empresa_id, cuenta_bancaria_id, fecha, saldo, nota, capturado_por)
		SELECT $1::uuid, cb.id, $3::date, $4::numeric, NULLIF($5, ''), $6::uuid
		FROM cuenta_bancaria cb
		WHERE cb.id = $2::uuid AND cb.empresa_id = $1::uuid AND cb.activo
		ON CONFLICT (empresa_id, cuenta_bancaria_id, fecha) DO UPDATE
		   SET saldo = EXCLUDED.saldo, nota = EXCLUDED.nota,
		       capturado_por = EXCLUDED.capturado_por, actualizado_en = now()
		   WHERE saldo_cuenta_diario.revisado_en IS NULL`
	// congelado distingue «no se guardó porque está congelado» de «esa cuenta no es tuya».
	const congelado = `
		SELECT EXISTS (SELECT 1 FROM saldo_cuenta_diario
		               WHERE empresa_id = $1::uuid AND cuenta_bancaria_id = $2::uuid
		                 AND fecha = $3::date AND revisado_en IS NOT NULL)`
	n := 0
	for _, s := range saldos {
		tag, err := tx.Exec(ctx, q, empresaID, s.CuentaID, fecha, s.Saldo, s.Nota, usuarioID)
		if err != nil {
			return 0, fmt.Errorf("bancos: guardar saldo: %w", err)
		}
		if tag.RowsAffected() == 0 {
			var esta bool
			if err := tx.QueryRow(ctx, congelado, empresaID, s.CuentaID, fecha).Scan(&esta); err != nil {
				return 0, fmt.Errorf("bancos: verificar congelamiento: %w", err)
			}
			if esta {
				return 0, ErrSaldoCongelado
			}
			return 0, ErrCuentaNoEncontrada
		}
		n += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("bancos: commit saldos: %w", err)
	}
	return n, nil
}

// CargaDelPeriodo devuelve, por cuenta activa, cuántos movimientos tiene el período y hasta
// cuándo llegan: es el checklist que dice si el mes está de verdad completo.
func (r *pgRepository) CargaDelPeriodo(ctx context.Context, empresaID, periodo string) ([]CargaCuenta, error) {
	const q = `
		SELECT cb.id::text, cb.alias, b.nombre, cb.moneda,
		       COUNT(m.id)::int,
		       COALESCE(to_char(MAX(m.fecha), 'YYYY-MM-DD'), ''),
		       COALESCE(` + hoyCRSQL + ` - MAX(m.fecha), 0)
		FROM cuenta_bancaria cb
		JOIN banco b ON b.id = cb.banco_id
		LEFT JOIN movimiento_bancario m
		       ON m.cuenta_bancaria_id = cb.id AND m.empresa_id = $1::uuid AND m.incluido
		      AND to_char(m.fecha, 'YYYY-MM') = $2
		WHERE cb.empresa_id = $1::uuid AND cb.activo
		GROUP BY cb.id, cb.alias, b.nombre, cb.moneda
		ORDER BY 7 DESC, cb.alias`
	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("bancos: carga del período: %w", err)
	}
	defer rows.Close()
	out := make([]CargaCuenta, 0, 16)
	for rows.Next() {
		var c CargaCuenta
		if err := rows.Scan(&c.CuentaID, &c.Alias, &c.Banco, &c.Moneda,
			&c.Movimientos, &c.UltimoMovimiento, &c.DiasSinCargar); err != nil {
			return nil, fmt.Errorf("bancos: scan carga: %w", err)
		}
		c.Estado = estadoCarga(c.Movimientos, c.DiasSinCargar)
		out = append(out, c)
	}
	return out, rows.Err()
}

// decOrCero parsea un monto tolerando vacío (sin captura). Nunca usa float64.
func decOrCero(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}
