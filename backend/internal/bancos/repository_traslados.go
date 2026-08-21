package bancos

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// marcasDeTraslado / marcasDeCobro son las señales de la DESCRIPCIÓN, que en este negocio es
// el mejor indicio: un traslado real dice «TRASLADO 1989 A 1990 FONDOS…» en las dos patas,
// mientras un cobro de plan dice «SINPE MOVIL», «SMO-…» o «PAGO DE …». Sin esto, emparejar
// por monto y fecha convierte cobros de clientes en traslados y los saca del EBITDA.
const marcasDeTraslado = `(traslado|traspaso)`
const marcasDeCobro = `(sinpe *movil|smo-|pago de |dep[oó]sito de |transf(erencia)? de )`

// PropuestasTraslados devuelve los pares candidatos del período CON sus señales, para que el
// servicio los puntúe. Observa hechos; no juzga.
//
// Además de monto (±tolerancia) y fecha (±3 días) en cuentas distintas, mide dos cosas que
// antes faltaban y que producían basura: cuántas veces se repite ese monto en el período (un
// monto que aparece mil veces es una cuota de plan, no un traslado) y cuántas parejas
// posibles tiene el mismo movimiento (un traslado real es único).
func (r *pgRepository) PropuestasTraslados(ctx context.Context, empresaID, periodo string, tolerancia decimal.Decimal) ([]PropuestaTraslado, error) {
	q := `
		WITH movs AS (
			SELECT m.id, m.cuenta_bancaria_id, m.fecha, m.debito, m.credito,
			       COALESCE(m.descripcion, '') AS descripcion,
			       COALESCE(NULLIF(m.debito, 0), m.credito) AS monto
			FROM movimiento_bancario m
			WHERE m.empresa_id = $1::uuid AND m.incluido AND NOT m.es_traslado
			  -- Ventana ampliada: un traslado puede cruzar el borde del mes.
			  AND m.fecha BETWEEN (to_date($2, 'YYYY-MM') - INTERVAL '3 days')
			                  AND (to_date($2, 'YYYY-MM') + INTERVAL '1 month' + INTERVAL '3 days')
		),
		frecuencia AS (
			SELECT monto, COUNT(*) AS veces FROM movs GROUP BY monto
		),
		candidatos AS (
			SELECT d.id AS did, c.id AS cid, d.fecha AS dfecha, c.fecha AS cfecha,
			       d.cuenta_bancaria_id AS dcta, c.cuenta_bancaria_id AS ccta,
			       d.debito, c.credito, d.descripcion AS ddesc, c.descripcion AS cdesc,
			       f.veces,
			       (d.descripcion ~* '` + marcasDeTraslado + `' OR c.descripcion ~* '` + marcasDeTraslado + `') AS dice_traslado,
			       (d.descripcion ~* '` + marcasDeCobro + `' OR c.descripcion ~* '` + marcasDeCobro + `') AS dice_cobro,
			       (d.debito = c.credito) AS monto_exacto,
			       abs(c.fecha - d.fecha) AS dias,
			       (d.debito % 10000 = 0) AS redondo,
			       (d.debito > 1000000) AS alto
			FROM movs d
			JOIN movs c ON c.cuenta_bancaria_id <> d.cuenta_bancaria_id AND c.credito > 0
			           AND abs(d.debito - c.credito) <= $3 * greatest(d.debito, c.credito)
			           AND abs(d.fecha - c.fecha) <= 3
			JOIN frecuencia f ON f.monto = d.debito
			WHERE d.debito > 0 AND to_char(d.fecha, 'YYYY-MM') = $2
			  -- Lo que el criterio va a descartar igual, se descarta ACÁ: si no, el LIMIT de
			  -- abajo se llena de cobros de clientes y puede dejar afuera un traslado real.
			  -- $4 es la misma constante que usa PuntuarTraslado (una sola fuente).
			  AND f.veces < $4
			  AND d.descripcion !~* '` + marcasDeCobro + `'
			  AND c.descripcion !~* '` + marcasDeCobro + `'
		)
		SELECT k.did::text, k.cid::text,
		       to_char(k.dfecha, 'YYYY-MM-DD'), to_char(k.cfecha, 'YYYY-MM-DD'),
		       COALESCE(dc.alias, ''), COALESCE(cc.alias, ''),
		       k.debito::text, k.credito::text, k.ddesc, k.cdesc,
		       k.dice_traslado, k.dice_cobro, k.monto_exacto, k.dias, k.redondo, k.alto,
		       k.veces::int,
		       COUNT(*) OVER (PARTITION BY k.did)::int AS candidatos_del_debito
		FROM candidatos k
		JOIN cuenta_bancaria dc ON dc.id = k.dcta
		JOIN cuenta_bancaria cc ON cc.id = k.ccta
		-- Orden por FUERZA DE LA SEÑAL, no por fecha: así los 500 que se traen siempre
		-- contienen a los traslados plausibles, y lo que se corta es lo que el criterio
		-- descartaría de todos modos.
		ORDER BY k.dice_traslado DESC, k.monto_exacto DESC, k.redondo DESC, k.debito DESC, k.dfecha
		LIMIT 500`
	rows, err := r.pool.Query(ctx, q, empresaID, periodo, tolerancia, vecesParaRecurrente)
	if err != nil {
		return nil, fmt.Errorf("bancos: propuestas traslados: %w", err)
	}
	defer rows.Close()
	var out []PropuestaTraslado
	for rows.Next() {
		var p PropuestaTraslado
		var s SenalesTraslado
		if err := rows.Scan(&p.DebitoID, &p.CreditoID, &p.FechaDebito, &p.FechaCredito,
			&p.CuentaDebito, &p.CuentaCredito, &p.MontoDebito, &p.MontoCredito,
			&p.DescripcionDebito, &p.DescripcionCredito,
			&s.DiceTraslado, &s.DiceCobro, &s.MontoExacto, &s.DiasDiferencia,
			&s.MontoRedondo, &s.MontoAlto, &s.VecesElMonto, &s.CandidatosDelMovimiento); err != nil {
			return nil, fmt.Errorf("bancos: scan propuesta: %w", err)
		}
		p.Puntaje, p.Veredicto, p.Razones = PuntuarTraslado(s)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *pgRepository) MovimientoParaTraslado(ctx context.Context, empresaID, movID string) (MovTraslado, error) {
	const q = `SELECT id::text, cuenta_bancaria_id::text, debito, credito, es_traslado, incluido
	           FROM movimiento_bancario WHERE empresa_id = $1::uuid AND id = $2::uuid`
	var m MovTraslado
	err := r.pool.QueryRow(ctx, q, empresaID, movID).Scan(&m.ID, &m.CuentaID, &m.Debito, &m.Credito, &m.EsTraslado, &m.Incluido)
	if errors.Is(err, pgx.ErrNoRows) {
		return MovTraslado{}, ErrMovimientoNoEncontrado
	}
	if err != nil {
		return MovTraslado{}, fmt.Errorf("bancos: movimiento para traslado: %w", err)
	}
	return m, nil
}

func (r *pgRepository) EmparejarTraslado(ctx context.Context, empresaID, debitoID, creditoID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bancos: begin emparejar: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Cada pata apunta a la otra y queda REVISADO (no bloquea el cierre).
	// Guarda `es_traslado = false`: si una pata ya fue emparejada (carrera o reuso),
	// afecta 0 filas y se aborta — evita emparejamientos huérfanos/asimétricos.
	const q = `
		UPDATE movimiento_bancario
		SET es_traslado = true, par_traslado_id = $3::uuid,
		    estado_clasificacion = 'REVISADO', actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND es_traslado = false`
	t1, err := tx.Exec(ctx, q, empresaID, debitoID, creditoID)
	if err != nil {
		return fmt.Errorf("bancos: emparejar débito: %w", err)
	}
	t2, err := tx.Exec(ctx, q, empresaID, creditoID, debitoID)
	if err != nil {
		return fmt.Errorf("bancos: emparejar crédito: %w", err)
	}
	if t1.RowsAffected() != 1 || t2.RowsAffected() != 1 {
		return ErrTrasladoInvalido // alguna pata no existe o ya estaba emparejada
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bancos: commit emparejar: %w", err)
	}
	return nil
}

func (r *pgRepository) DesemparejarTraslado(ctx context.Context, empresaID, movID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bancos: begin desemparejar: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var par *string
	var esTraslado bool
	err = tx.QueryRow(ctx, `SELECT par_traslado_id::text, es_traslado FROM movimiento_bancario WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, movID).Scan(&par, &esTraslado)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMovimientoNoEncontrado
	}
	if err != nil {
		return fmt.Errorf("bancos: buscar par: %w", err)
	}
	if !esTraslado {
		// No es un traslado emparejado: no tocar (evita revertir una clasificación legítima a NO_IDENTIFICADO).
		return ErrTrasladoInvalido
	}

	// Guarda `es_traslado = true` por seguridad; solo revierte patas de traslado.
	const q = `
		UPDATE movimiento_bancario
		SET es_traslado = false, par_traslado_id = NULL,
		    estado_clasificacion = 'NO_IDENTIFICADO', actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND es_traslado = true`
	if _, err := tx.Exec(ctx, q, empresaID, movID); err != nil {
		return fmt.Errorf("bancos: desemparejar: %w", err)
	}
	if par != nil {
		if _, err := tx.Exec(ctx, q, empresaID, *par); err != nil {
			return fmt.Errorf("bancos: desemparejar par: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bancos: commit desemparejar: %w", err)
	}
	return nil
}

func (r *pgRepository) CerrarPeriodo(ctx context.Context, empresaID string, anio, mes, noIdentificados int, usuarioID string) error {
	const q = `
		INSERT INTO periodo_cierre (empresa_id, anio, mes, no_identificados_al_cierre, cerrado_por)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid)
		ON CONFLICT (empresa_id, anio, mes) DO NOTHING`
	if _, err := r.pool.Exec(ctx, q, empresaID, anio, mes, noIdentificados, usuarioID); err != nil {
		return fmt.Errorf("bancos: cerrar período: %w", err)
	}
	return nil
}

func (r *pgRepository) PeriodoCerrado(ctx context.Context, empresaID string, anio, mes int) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM periodo_cierre WHERE empresa_id = $1::uuid AND anio = $2 AND mes = $3)`
	var existe bool
	if err := r.pool.QueryRow(ctx, q, empresaID, anio, mes).Scan(&existe); err != nil {
		return false, fmt.Errorf("bancos: período cerrado: %w", err)
	}
	return existe, nil
}
