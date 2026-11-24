package conta

// Consultas del balance de comprobación y del período contable (mig 0113). El porqué está en
// periodo.go.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// balanceSQL arma el balance de comprobación de un rango.
//
// ── POR QUÉ DOS AGREGACIONES Y NO UNA ───────────────────────────────────────────────────────
//
// El saldo ANTERIOR y los movimientos DEL RANGO son dos preguntas distintas sobre las mismas
// filas, y separarlas con FILTER las contesta en una sola pasada. Hacerlo con dos consultas
// obligaría a unirlas en Go por cuenta, que es donde se cometen los errores de alineación.
//
// Se parte de `cuenta_contable` con LEFT JOIN y no de las líneas: una cuenta con saldo anterior
// y sin movimientos del mes tiene que aparecer igual —es justamente la que alguien busca para
// saber qué quedó quieto—.
const balanceSQL = `
	SELECT c.id::text, c.codigo, c.nombre, c.clase, c.naturaleza,
	       COALESCE(sum(l.debe)  FILTER (WHERE a.fecha < $2::date), 0)::text,
	       COALESCE(sum(l.haber) FILTER (WHERE a.fecha < $2::date), 0)::text,
	       COALESCE(sum(l.debe)  FILTER (WHERE a.fecha >= $2::date AND a.fecha <= $3::date), 0)::text,
	       COALESCE(sum(l.haber) FILTER (WHERE a.fecha >= $2::date AND a.fecha <= $3::date), 0)::text
	FROM cuenta_contable c
	LEFT JOIN asiento_linea l ON l.cuenta_id = c.id
	LEFT JOIN asiento a ON a.id = l.asiento_id
	                   AND a.empresa_id = $1::uuid
	                   AND a.estado <> 'ANULADO'
	                   AND a.fecha <= $3::date
	WHERE (c.empresa_id = $1::uuid OR c.empresa_id IS NULL)
	  AND c.acepta_movimiento
	GROUP BY c.id, c.codigo, c.nombre, c.clase, c.naturaleza
	-- Solo las cuentas que tienen algo que mostrar: un plan de 258 cuentas con 240 en cero no es
	-- un balance, es un catálogo con números al lado.
	--
	-- LOS FILTER SON LOS MISMOS DEL SELECT, Y ESO NO ES ESTÉTICA. Un sum(l.debe) pelado cuenta
	-- líneas cuyo asiento NO pasó el LEFT JOIN —anulado, de otra empresa, o de fecha posterior al
	-- corte—: esas filas salían con las seis columnas en blanco, y una cuenta del grupo que solo
	-- usó otra empresa aparecía en este balance. La fila no traía sus montos, pero su presencia
	-- ya delataba movimiento ajeno.
	HAVING COALESCE(sum(l.debe)  FILTER (WHERE a.fecha < $2::date), 0)
	     - COALESCE(sum(l.haber) FILTER (WHERE a.fecha < $2::date), 0) <> 0
	    OR COALESCE(sum(l.debe)  FILTER (WHERE a.fecha >= $2::date AND a.fecha <= $3::date), 0) <> 0
	    OR COALESCE(sum(l.haber) FILTER (WHERE a.fecha >= $2::date AND a.fecha <= $3::date), 0) <> 0
	ORDER BY c.codigo`

// Balance arma el balance de comprobación del rango.
func (r *pgRepository) Balance(ctx context.Context, empresaID, desde, hasta string) ([]RenglonBalance, error) {
	rows, err := r.pool.Query(ctx, balanceSQL, empresaID, desde, hasta)
	if err != nil {
		if fechaInvalida(err) {
			return nil, ErrFechaInvalida
		}
		return nil, fmt.Errorf("conta: balance de comprobación: %w", err)
	}
	defer rows.Close()

	out := make([]RenglonBalance, 0)
	for rows.Next() {
		var g RenglonBalance
		var antDebe, antHaber decimal.Decimal
		if err := rows.Scan(&g.CuentaID, &g.Codigo, &g.Nombre, &g.Clase, &g.Naturaleza,
			&antDebe, &antHaber, &g.Debe, &g.Haber); err != nil {
			return nil, fmt.Errorf("conta: scan del balance: %w", err)
		}
		// El saldo anterior va NETO y en la columna de su signo: una cuenta no arrastra «debe
		// 900 y haber 400» del pasado, arrastra un saldo de 500 deudor.
		g.SaldoAnteriorDebe, g.SaldoAnteriorHaber = repartirPorSigno(antDebe.Sub(antHaber))

		debe, err := decimal.NewFromString(g.Debe)
		if err != nil {
			return nil, fmt.Errorf("conta: debe de %s: %w", g.Codigo, err)
		}
		haber, err := decimal.NewFromString(g.Haber)
		if err != nil {
			return nil, fmt.Errorf("conta: haber de %s: %w", g.Codigo, err)
		}
		final := antDebe.Sub(antHaber).Add(debe).Sub(haber)
		g.SaldoFinalDebe, g.SaldoFinalHaber = repartirPorSigno(final)
		g.Debe, g.Haber = debe.StringFixed(2), haber.StringFixed(2)
		out = append(out, g)
	}
	return out, rows.Err()
}

// repartirPorSigno pone un saldo neto en la columna que le corresponde.
//
// Positivo va al debe, negativo al haber (en valor absoluto). Mostrar un «−500» en la columna del
// debe obliga a traducir mentalmente cada fila, y en un balance de cien renglones eso es donde se
// empiezan a leer mal los números.
func repartirPorSigno(neto decimal.Decimal) (string, string) {
	if neto.IsNegative() {
		return "0.00", neto.Neg().StringFixed(2)
	}
	return neto.StringFixed(2), "0.00"
}

// HayApertura dice si existe un asiento de apertura vivo.
func (r *pgRepository) HayApertura(ctx context.Context, empresaID string) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM asiento
		                WHERE empresa_id = $1::uuid AND origen_tipo = 'APERTURA'
		                  AND estado <> 'ANULADO')`, empresaID).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("conta: buscar la apertura: %w", err)
	}
	return existe, nil
}

// ── El período contable ─────────────────────────────────────────────────────────────────────

// periodosSQL trae el estado de cada mes CON lo que el libro tiene hoy.
//
// Los dos lados juntos a propósito: el estado dice si se puede escribir y los totales dicen si
// conviene cerrar. Pedirlos por separado dejaría la pantalla mostrando «abierto» sin saber que
// ese mes no cuadra.
const periodosSQL = `
	WITH meses AS (
		SELECT DISTINCT EXTRACT(YEAR FROM fecha)::int AS anio,
		                EXTRACT(MONTH FROM fecha)::int AS mes
		FROM asiento WHERE empresa_id = $1::uuid AND estado <> 'ANULADO'
		UNION
		SELECT anio, mes FROM periodo_contable WHERE empresa_id = $1::uuid
	)
	SELECT m.anio, m.mes,
	       COALESCE(p.estado, 'ABIERTO'),
	       p.cerrado_en, COALESCE(u.nombre, ''),
	       COALESCE(p.debe_al_cierre::text, ''), COALESCE(p.haber_al_cierre::text, ''),
	       COALESCE(p.asientos_al_cierre, 0),
	       p.reabierto_en, COALESCE(p.reabierto_motivo, ''),
	       COALESCE(t.asientos, 0), COALESCE(t.debe, 0)::text, COALESCE(t.haber, 0)::text
	FROM meses m
	LEFT JOIN periodo_contable p
	       ON p.empresa_id = $1::uuid AND p.anio = m.anio AND p.mes = m.mes
	LEFT JOIN usuario u ON u.id = p.cerrado_por
	LEFT JOIN LATERAL (
		SELECT count(DISTINCT a.id)::int AS asientos,
		       sum(l.debe) AS debe, sum(l.haber) AS haber
		FROM asiento a LEFT JOIN asiento_linea l ON l.asiento_id = a.id
		WHERE a.empresa_id = $1::uuid AND a.estado <> 'ANULADO'
		  AND EXTRACT(YEAR FROM a.fecha)::int = m.anio
		  AND EXTRACT(MONTH FROM a.fecha)::int = m.mes
	) t ON true
	ORDER BY m.anio DESC, m.mes DESC`

// Periodos lista los meses con su estado y lo que tienen.
func (r *pgRepository) Periodos(ctx context.Context, empresaID string) ([]PeriodoContable, error) {
	rows, err := r.pool.Query(ctx, periodosSQL, empresaID)
	if err != nil {
		return nil, fmt.Errorf("conta: listar períodos: %w", err)
	}
	defer rows.Close()

	out := make([]PeriodoContable, 0)
	for rows.Next() {
		var p PeriodoContable
		var debe, haber decimal.Decimal
		if err := rows.Scan(&p.Anio, &p.Mes, &p.Estado,
			&p.CerradoEn, &p.CerradoPorNombre,
			&p.DebeAlCierre, &p.HaberAlCierre, &p.AsientosAlCierre,
			&p.ReabiertoEn, &p.ReabiertoMotivo,
			&p.Asientos, &debe, &haber); err != nil {
			return nil, fmt.Errorf("conta: scan del período: %w", err)
		}
		p.Debe, p.Haber = debe.StringFixed(2), haber.StringFixed(2)
		p.Cuadra = debe.Equal(haber)
		out = append(out, p)
	}
	return out, rows.Err()
}

// CerrarPeriodo congela el mes, guardando con qué números se cerró.
func (r *pgRepository) CerrarPeriodo(ctx context.Context, empresaID string, anio, mes int, debe, haber decimal.Decimal, asientos int, usuarioID string) error {
	const q = `
		INSERT INTO periodo_contable
		  (empresa_id, anio, mes, estado, cerrado_en, cerrado_por,
		   debe_al_cierre, haber_al_cierre, asientos_al_cierre)
		VALUES ($1::uuid, $2, $3, 'CERRADO', now(), NULLIF($4,'')::uuid, $5, $6, $7)
		ON CONFLICT (empresa_id, anio, mes) DO UPDATE
		SET estado = 'CERRADO', cerrado_en = now(), cerrado_por = EXCLUDED.cerrado_por,
		    debe_al_cierre = EXCLUDED.debe_al_cierre,
		    haber_al_cierre = EXCLUDED.haber_al_cierre,
		    asientos_al_cierre = EXCLUDED.asientos_al_cierre
		WHERE periodo_contable.estado <> 'CERRADO'`
	tag, err := r.pool.Exec(ctx, q, empresaID, anio, mes, usuarioID, debe, haber, asientos)
	if err != nil {
		return fmt.Errorf("conta: cerrar el período: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPeriodoCerradoYa
	}
	return nil
}

// ReabrirPeriodo descongela el mes, dejando escrito por qué.
func (r *pgRepository) ReabrirPeriodo(ctx context.Context, empresaID string, anio, mes int, motivo, usuarioID string) error {
	const q = `
		UPDATE periodo_contable
		SET estado = 'ABIERTO', reabierto_en = now(),
		    reabierto_por = NULLIF($4,'')::uuid, reabierto_motivo = $5
		WHERE empresa_id = $1::uuid AND anio = $2 AND mes = $3 AND estado = 'CERRADO'`
	tag, err := r.pool.Exec(ctx, q, empresaID, anio, mes, usuarioID, motivo)
	if err != nil {
		return fmt.Errorf("conta: reabrir el período: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPeriodoNoCerrado
	}
	return nil
}

// TotalesDelMes devuelve lo que el libro tiene en un mes, para decidir si se puede cerrar.
func (r *pgRepository) TotalesDelMes(ctx context.Context, empresaID string, anio, mes int) (decimal.Decimal, decimal.Decimal, int, error) {
	const q = `
		SELECT COALESCE(sum(l.debe), 0), COALESCE(sum(l.haber), 0), count(DISTINCT a.id)::int
		FROM asiento a LEFT JOIN asiento_linea l ON l.asiento_id = a.id
		WHERE a.empresa_id = $1::uuid AND a.estado <> 'ANULADO'
		  AND EXTRACT(YEAR FROM a.fecha)::int = $2
		  AND EXTRACT(MONTH FROM a.fecha)::int = $3`
	var debe, haber decimal.Decimal
	var n int
	if err := r.pool.QueryRow(ctx, q, empresaID, anio, mes).Scan(&debe, &haber, &n); err != nil {
		return decimal.Zero, decimal.Zero, 0, fmt.Errorf("conta: totales del mes: %w", err)
	}
	return debe, haber, n, nil
}
