package bancos

// Consultas del análisis de partidas en el tiempo.

import (
	"context"
	"fmt"
)

// SaludMeses devuelve, por mes del rango, cuántos movimientos hay y qué porcentaje tiene su
// partida asignada. Es lo que permite decir «este mes no es comparable» antes de comparar.
func (r *pgRepository) SaludMeses(ctx context.Context, empresaID, desde, hasta string) ([]SaludMes, error) {
	// `generate_series` para que los meses SIN movimientos también aparezcan: un mes vacío es
	// información (no se cargó el estado de cuenta), y omitirlo lo esconde.
	const q = `
		WITH meses AS (
			SELECT to_char(gs, 'YYYY-MM') AS periodo
			FROM generate_series(
				to_date($2, 'YYYY-MM'),
				to_date($3, 'YYYY-MM'),
				interval '1 month') gs
		)
		SELECT m.periodo,
		       COALESCE(x.movs, 0)::int,
		       COALESCE(x.pct, 0)::text
		FROM meses m
		LEFT JOIN (
			SELECT to_char(fecha, 'YYYY-MM') AS periodo,
			       count(*) AS movs,
			       round(100.0 * count(*) FILTER (WHERE clasificacion_id IS NOT NULL) / count(*), 1) AS pct
			FROM movimiento_bancario
			WHERE empresa_id = $1::uuid AND incluido
			GROUP BY 1
		) x ON x.periodo = m.periodo
		ORDER BY m.periodo`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: salud de meses: %w", err)
	}
	defer rows.Close()
	out := []SaludMes{}
	for rows.Next() {
		var s SaludMes
		if err := rows.Scan(&s.Periodo, &s.Movs, &s.PctClasificado); err != nil {
			return nil, fmt.Errorf("bancos: scan salud de mes: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SeriePorPartida devuelve, para cada Concepto › Clasificación, su monto mes a mes en el rango.
//
// El monto sale de la MISMA definición de ingreso/gasto que usa el EBITDA (la naturaleza declarada
// en el concepto, no el signo del movimiento) y excluye los traslados emparejados. Si esta consulta
// midiera distinto que el tablero, las dos pantallas discreparían sobre el mismo gasto.
func (r *pgRepository) SeriePorPartida(ctx context.Context, empresaID, desde, hasta string) ([]TendenciaPartida, error) {
	const q = `
		WITH meses AS (
			SELECT to_char(gs, 'YYYY-MM') AS periodo
			FROM generate_series(
				to_date($2, 'YYYY-MM'),
				to_date($3, 'YYYY-MM'),
				interval '1 month') gs
		),
		partidas AS (
			SELECT DISTINCT m.concepto_id, m.clasificacion_id
			FROM movimiento_bancario m
			WHERE m.empresa_id = $1::uuid AND m.incluido
			  AND NOT m.es_traslado
			  AND m.clasificacion_id IS NOT NULL
			  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3
		),
		datos AS (
			SELECT m.concepto_id, m.clasificacion_id,
			       to_char(m.fecha, 'YYYY-MM') AS periodo,
			       -- El monto de la partida en su propio sentido, con la MISMA expresión que usa el
			       -- EBITDA del tablero: un gasto suma como gasto y un ingreso como ingreso, y una
			       -- devolución dentro de la partida resta.
			       ` + sqlMontoEnSuSentido + ` AS monto,
			       count(*) AS movs
			FROM movimiento_bancario m
			` + joinConcepto + `
			WHERE m.empresa_id = $1::uuid AND m.incluido
			  AND NOT m.es_traslado
			  AND m.clasificacion_id IS NOT NULL
			  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3
			GROUP BY 1, 2, 3
		)
		SELECT p.concepto_id::text, co.nombre, p.clasificacion_id::text, cl.nombre,
		       COALESCE(co.naturaleza, 'NEUTRO'), co.naturaleza_declarada,
		       ms.periodo,
		       COALESCE(d.monto, 0)::text,
		       COALESCE(d.movs, 0)::int
		FROM partidas p
		CROSS JOIN meses ms
		JOIN concepto co ON co.id = p.concepto_id
		JOIN clasificacion cl ON cl.id = p.clasificacion_id
		LEFT JOIN datos d
		       ON d.concepto_id = p.concepto_id
		      AND d.clasificacion_id = p.clasificacion_id
		      AND d.periodo = ms.periodo
		ORDER BY co.nombre, cl.nombre, ms.periodo`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: serie por partida: %w", err)
	}
	defer rows.Close()

	// Las filas vienen ordenadas por partida y período: se agrupan en una pasada.
	out := []TendenciaPartida{}
	for rows.Next() {
		var conceptoID, concepto, clasifID, clasif, naturaleza, periodo, monto string
		var declarada bool
		var movs int
		if err := rows.Scan(&conceptoID, &concepto, &clasifID, &clasif, &naturaleza, &declarada, &periodo, &monto, &movs); err != nil {
			return nil, fmt.Errorf("bancos: scan serie por partida: %w", err)
		}
		n := len(out)
		if n == 0 || out[n-1].ClasificacionID != clasifID || out[n-1].ConceptoID != conceptoID {
			out = append(out, TendenciaPartida{
				ConceptoID: conceptoID, Concepto: concepto,
				ClasificacionID: clasifID, Clasificacion: clasif,
				Naturaleza: naturaleza, NaturalezaDeclarada: declarada,
				Serie: []PuntoPartida{},
			})
			n = len(out)
		}
		out[n-1].Serie = append(out[n-1].Serie, PuntoPartida{Periodo: periodo, Monto: monto, Movs: movs})
	}
	return out, rows.Err()
}
