package bancos

// Consultas del análisis de partidas en el tiempo.

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
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

// SerieDiariaPorPartida devuelve, por partida, los días CON movimiento dentro del rango de meses.
//
// Mismos filtros y misma expresión de monto que `SeriePorPartida`: si difirieran, la vista diaria
// y la mensual de la misma pantalla mostrarían totales distintos del mismo gasto.
//
// No hay `generate_series` de días a propósito: los días sin movimiento no se devuelven. Rellenar
// con ceros multiplicaría por 30 el tamaño de la respuesta para dibujar una línea en el suelo.
func (r *pgRepository) SerieDiariaPorPartida(ctx context.Context, empresaID, desde, hasta string, clasificaciones []string) ([]SerieDiariaPartida, error) {
	const q = `
		SELECT cl.id::text, cl.nombre, COALESCE(co.nombre, '(sin concepto)'),
		       m.fecha,
		       ` + sqlMontoEnSuSentido + ` AS monto,
		       count(*)::int AS movs
		FROM movimiento_bancario m
		` + joinConcepto + `
		JOIN clasificacion cl ON cl.id = m.clasificacion_id
		WHERE m.empresa_id = $1::uuid AND m.incluido
		  AND NOT m.es_traslado
		  AND m.clasificacion_id = ANY($4::uuid[])
		  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3
		GROUP BY cl.id, cl.nombre, co.nombre, m.fecha
		ORDER BY cl.nombre, m.fecha`

	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta, clasificaciones)
	if err != nil {
		return nil, fmt.Errorf("bancos: serie diaria por partida: %w", err)
	}
	defer rows.Close()

	// Se arma agrupando en Go y no con un array en SQL: la consulta queda legible y el volumen es
	// el de unas pocas partidas por un rango de meses.
	orden := []string{}
	porID := map[string]*SerieDiariaPartida{}
	for rows.Next() {
		var (
			id, nombre, concepto string
			fecha                time.Time
			monto                decimal.Decimal
			movs                 int
		)
		if err := rows.Scan(&id, &nombre, &concepto, &fecha, &monto, &movs); err != nil {
			return nil, fmt.Errorf("bancos: scan serie diaria: %w", err)
		}
		p, ok := porID[id]
		if !ok {
			p = &SerieDiariaPartida{
				ClasificacionID: id, Clasificacion: nombre, Concepto: concepto,
				Total: "0", Dias: []DiaDePartida{},
			}
			porID[id] = p
			orden = append(orden, id)
		}
		p.Dias = append(p.Dias, DiaDePartida{
			Fecha: fecha.Format("2006-01-02"),
			Monto: monto.String(),
			Movs:  movs,
		})
		total, err := decimal.NewFromString(p.Total)
		if err != nil {
			return nil, fmt.Errorf("bancos: total de la serie diaria de %s: %w", nombre, err)
		}
		p.Total = total.Add(monto).String()
		p.Movs += movs
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]SerieDiariaPartida, 0, len(orden))
	for _, id := range orden {
		out = append(out, *porID[id])
	}
	return out, nil
}
