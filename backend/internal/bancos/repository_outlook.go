package bancos

// Consultas del outlook anual. El porqué está en outlook.go.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// ingresoYGastoPorMesSQL trae los dos netos de CADA mes del año.
//
// Por mes y no acumulado: el outlook necesita saber cuáles meses están cerrados para sacar el
// ritmo, y un total del año no permite distinguirlos. Usa `sqlIngresoNeto` y `sqlGastoNeto`, la
// definición única del módulo, para que esta pantalla no pueda discrepar del EBITDA del tablero.
var ingresoYGastoPorMesSQL = `
	SELECT to_char(m.fecha, 'YYYY-MM') AS periodo,
	       ` + sqlIngresoNeto + `::text, ` + sqlGastoNeto + `::text
	FROM movimiento_bancario m
	` + joinConcepto + `
	WHERE m.empresa_id = $1::uuid AND m.incluido
	  AND EXTRACT(YEAR FROM m.fecha)::int = $2
	GROUP BY 1`

// RealPorMesDelAnio trae ingreso y gasto netos de cada mes del año.
func (r *pgRepository) RealPorMesDelAnio(ctx context.Context, empresaID string, anio int) (map[string][2]decimal.Decimal, error) {
	rows, err := r.pool.Query(ctx, ingresoYGastoPorMesSQL, empresaID, anio)
	if err != nil {
		return nil, fmt.Errorf("bancos: real por mes del año: %w", err)
	}
	defer rows.Close()

	out := map[string][2]decimal.Decimal{}
	for rows.Next() {
		var periodo, ing, gas string
		if err := rows.Scan(&periodo, &ing, &gas); err != nil {
			return nil, fmt.Errorf("bancos: scan real del mes: %w", err)
		}
		i, err1 := decimal.NewFromString(ing)
		g, err2 := decimal.NewFromString(gas)
		if err1 != nil || err2 != nil {
			continue
		}
		out[periodo] = [2]decimal.Decimal{i, g}
	}
	return out, rows.Err()
}

// metasPorMesDelAnioSQL junta las DOS mitades de la meta en una sola pasada.
//
// El ingreso sale de `presupuesto_ingreso` (total del mes: `clasificacion_id` NULO) y el gasto de
// los totales por departamento de `presupuesto_departamento`. En los dos casos se toma SOLO el
// total, nunca el desglose: son dos vistas del mismo dinero y sumarlas contaría doble.
//
// FULL OUTER JOIN y no INNER: un mes puede tener meta de gasto y no de ingreso, o al revés, y un
// INNER borraría justo los meses a medio presupuestar —que son los que hay que ver—.
const metasPorMesDelAnioSQL = `
	WITH ing AS (
		SELECT periodo, SUM(monto_crc) AS monto
		FROM presupuesto_ingreso
		WHERE empresa_id = $1::uuid AND clasificacion_id IS NULL
		  AND periodo LIKE $2 || '-%'
		GROUP BY periodo
	), gas AS (
		SELECT periodo, SUM(monto_crc) AS monto
		FROM presupuesto_departamento
		WHERE empresa_id = $1::uuid AND clasificacion_id IS NULL
		  AND periodo LIKE $2 || '-%'
		GROUP BY periodo
	)
	SELECT COALESCE(ing.periodo, gas.periodo),
	       COALESCE(ing.monto, 0)::text,
	       COALESCE(gas.monto, 0)::text
	FROM ing FULL OUTER JOIN gas ON gas.periodo = ing.periodo`

// MetasPorMesDelAnio trae la meta de ingreso y el presupuesto de gasto de cada mes.
func (r *pgRepository) MetasPorMesDelAnio(ctx context.Context, empresaID string, anio int) (map[string][2]decimal.Decimal, error) {
	rows, err := r.pool.Query(ctx, metasPorMesDelAnioSQL, empresaID, fmt.Sprintf("%04d", anio))
	if err != nil {
		return nil, fmt.Errorf("bancos: metas por mes del año: %w", err)
	}
	defer rows.Close()

	out := map[string][2]decimal.Decimal{}
	for rows.Next() {
		var periodo, ing, gas string
		if err := rows.Scan(&periodo, &ing, &gas); err != nil {
			return nil, fmt.Errorf("bancos: scan meta del mes: %w", err)
		}
		i, err1 := decimal.NewFromString(ing)
		g, err2 := decimal.NewFromString(gas)
		if err1 != nil || err2 != nil {
			continue
		}
		out[periodo] = [2]decimal.Decimal{i, g}
	}
	return out, rows.Err()
}
