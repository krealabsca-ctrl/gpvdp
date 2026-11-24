package bancos

// Consultas del presupuesto de INGRESO (mig 0116). El porqué está en presupuesto_ingreso.go.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// MetasDeIngreso trae las metas cargadas en el rango, las dos clases juntas.
//
// `clasificacion_id` vacío = el total del mes; con valor = la meta de esa línea. Quien suma elige
// cuál usa: mezclarlas contaría el mismo colón dos veces.
func (r *pgRepository) MetasDeIngreso(ctx context.Context, empresaID, desde, hasta string) ([]MetaIngreso, error) {
	const q = `
		SELECT p.periodo, p.monto_crc::text, COALESCE(p.nota, ''),
		       COALESCE(p.clasificacion_id::text, ''), COALESCE(cl.nombre, ''), COALESCE(co.nombre, '')
		FROM presupuesto_ingreso p
		LEFT JOIN clasificacion cl ON cl.id = p.clasificacion_id
		LEFT JOIN concepto co ON co.id = cl.concepto_id
		WHERE p.empresa_id = $1::uuid AND p.periodo BETWEEN $2 AND $3
		ORDER BY p.periodo, cl.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: metas de ingreso: %w", err)
	}
	defer rows.Close()
	out := []MetaIngreso{}
	for rows.Next() {
		var m MetaIngreso
		if err := rows.Scan(&m.Periodo, &m.Monto, &m.Nota,
			&m.ClasificacionID, &m.Clasificacion, &m.Concepto); err != nil {
			return nil, fmt.Errorf("bancos: scan meta de ingreso: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GuardarMetaIngreso escribe (o reemplaza) la meta de un mes, total o de una línea.
//
// Un monto en CERO es legítimo: significa «este mes no se espera ingreso por acá», que es distinto
// de no haberlo definido. Por eso la forma de quitar una meta es borrarla, no ponerla en cero.
func (r *pgRepository) GuardarMetaIngreso(ctx context.Context, empresaID, clasifID, periodo, monto, nota, usuarioID string) error {
	// Dos ON CONFLICT porque la unicidad son dos índices PARCIALES —uno para el total y otro para
	// la línea—: un solo `ON CONFLICT (a,b,c)` no calza con un índice parcial y Postgres lo
	// rechaza. Es la misma trampa que ya tenía el presupuesto de gasto.
	const total = `
		INSERT INTO presupuesto_ingreso (empresa_id, periodo, monto_crc, nota, creado_por)
		VALUES ($1::uuid, $2, $3::numeric, COALESCE($4, ''), NULLIF($5, '')::uuid)
		ON CONFLICT (empresa_id, periodo) WHERE clasificacion_id IS NULL
		DO UPDATE SET monto_crc = EXCLUDED.monto_crc, nota = EXCLUDED.nota, actualizado_en = now()`
	const porLinea = `
		INSERT INTO presupuesto_ingreso (empresa_id, clasificacion_id, periodo, monto_crc, nota, creado_por)
		VALUES ($1::uuid, $6::uuid, $2, $3::numeric, COALESCE($4, ''), NULLIF($5, '')::uuid)
		ON CONFLICT (empresa_id, periodo, clasificacion_id) WHERE clasificacion_id IS NOT NULL
		DO UPDATE SET monto_crc = EXCLUDED.monto_crc, nota = EXCLUDED.nota, actualizado_en = now()`

	var err error
	if clasifID == "" {
		_, err = r.pool.Exec(ctx, total, empresaID, periodo, monto, nota, usuarioID)
	} else {
		_, err = r.pool.Exec(ctx, porLinea, empresaID, periodo, monto, nota, usuarioID, clasifID)
	}
	if err != nil {
		return fmt.Errorf("bancos: guardar meta de ingreso: %w", err)
	}
	return nil
}

// BorrarMetaIngreso quita la meta de un mes (vuelve a «no definida»).
func (r *pgRepository) BorrarMetaIngreso(ctx context.Context, empresaID, clasifID, periodo string) error {
	// `IS NOT DISTINCT FROM` y no `=`: con NULL, `=` nunca es verdadero y el borrado del total no
	// encontraría su fila.
	const q = `
		DELETE FROM presupuesto_ingreso
		 WHERE empresa_id = $1::uuid AND periodo = $2
		   AND clasificacion_id IS NOT DISTINCT FROM NULLIF($3, '')::uuid`
	if _, err := r.pool.Exec(ctx, q, empresaID, periodo, clasifID); err != nil {
		return fmt.Errorf("bancos: borrar meta de ingreso: %w", err)
	}
	return nil
}

// NaturalezaDeClasificacion devuelve la naturaleza declarada del concepto de esa partida.
//
// Es lo que impide poner una meta de INGRESO sobre una partida de gasto: sin esa comprobación, el
// resultado presupuestado restaría el mismo monto dos veces —una como gasto autorizado y otra
// como ingreso esperado— y el número saldría plausible.
func (r *pgRepository) NaturalezaDeClasificacion(ctx context.Context, empresaID, clasifID string) (string, error) {
	const q = `
		SELECT co.naturaleza
		FROM clasificacion cl JOIN concepto co ON co.id = cl.concepto_id
		WHERE cl.id = $2::uuid AND cl.empresa_id = $1::uuid`
	var nat string
	if err := r.pool.QueryRow(ctx, q, empresaID, clasifID).Scan(&nat); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrClasificacionNoEncontrada
		}
		return "", fmt.Errorf("bancos: naturaleza de la clasificación: %w", err)
	}
	return nat, nil
}

// ingresoRealPorLineaSQL mide el ingreso real de cada línea en el rango.
//
// USA `sqlIngresoNeto`, la definición única del módulo: si acá se escribiera «crédito que no es
// traslado», el control diría un número y el EBITDA del tablero otro, y nadie sabría cuál creer.
// El neto es lo correcto además porque una devolución dentro de un concepto de ingreso lo BAJA —no
// aparece como gasto—.
var ingresoRealPorLineaSQL = `
	SELECT cl.id::text, cl.nombre, co.nombre,
	       ` + sqlIngresoNeto + `::text,
	       COUNT(*) FILTER (WHERE co.naturaleza = 'INGRESO' AND NOT m.es_traslado)
	FROM movimiento_bancario m
	JOIN clasificacion cl ON cl.id = m.clasificacion_id
	JOIN concepto co ON co.id = cl.concepto_id
	WHERE m.empresa_id = $1::uuid AND m.incluido
	  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3
	  AND co.naturaleza = 'INGRESO'
	GROUP BY cl.id, cl.nombre, co.nombre
	HAVING ` + sqlIngresoNeto + ` <> 0
	ORDER BY 4 DESC`

// IngresoRealPorLinea trae lo que de verdad entró, por línea.
func (r *pgRepository) IngresoRealPorLinea(ctx context.Context, empresaID, desde, hasta string) ([]FilaIngreso, error) {
	rows, err := r.pool.Query(ctx, ingresoRealPorLineaSQL, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: ingreso real por línea: %w", err)
	}
	defer rows.Close()
	out := []FilaIngreso{}
	for rows.Next() {
		var f FilaIngreso
		if err := rows.Scan(&f.ClasificacionID, &f.Clasificacion, &f.Concepto, &f.Real, &f.Movs); err != nil {
			return nil, fmt.Errorf("bancos: scan ingreso por línea: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ingresoYGastoRealSQL devuelve los dos totales del rango de una sola pasada.
//
// Juntos y no en dos consultas: el resultado presupuestado los resta, y si cada uno viniera de su
// propia consulta con su propio filtro, una diferencia de criterio entre las dos se vería como un
// desvío del negocio.
var ingresoYGastoRealSQL = `
	SELECT ` + sqlIngresoNeto + `::text, ` + sqlGastoNeto + `::text
	FROM movimiento_bancario m
	` + joinConcepto + `
	WHERE m.empresa_id = $1::uuid AND m.incluido
	  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3`

// IngresoYGastoReal trae los dos netos del rango.
func (r *pgRepository) IngresoYGastoReal(ctx context.Context, empresaID, desde, hasta string) (decimal.Decimal, decimal.Decimal, error) {
	var ing, gas string
	if err := r.pool.QueryRow(ctx, ingresoYGastoRealSQL, empresaID, desde, hasta).Scan(&ing, &gas); err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("bancos: ingreso y gasto real: %w", err)
	}
	i, err := decimal.NewFromString(ing)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("bancos: ingreso real inválido: %w", err)
	}
	g, err := decimal.NewFromString(gas)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("bancos: gasto real inválido: %w", err)
	}
	return i, g, nil
}

// ingresoHistoricoSQL trae lo que entró en los mismos meses de un año atrás, por línea.
//
// ES LA AYUDA QUE HACE USABLE LA PANTALLA: con 41 líneas de ingreso, una planilla vacía se queda
// vacía. Proponer desde el histórico convierte el trabajo en ajustar en vez de escribir, que es lo
// que de verdad hace una empresa al presupuestar.
//
// Se propone, no se guarda: el número que queda es el que el usuario confirma.
var ingresoHistoricoSQL = `
	SELECT cl.id::text, cl.nombre, co.nombre, ` + sqlIngresoNeto + `::text
	FROM movimiento_bancario m
	JOIN clasificacion cl ON cl.id = m.clasificacion_id
	JOIN concepto co ON co.id = cl.concepto_id
	WHERE m.empresa_id = $1::uuid AND m.incluido
	  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3
	  AND co.naturaleza = 'INGRESO'
	GROUP BY cl.id, cl.nombre, co.nombre
	HAVING ` + sqlIngresoNeto + ` > 0
	ORDER BY 4 DESC`

// IngresoHistorico trae la base para proponer una meta.
func (r *pgRepository) IngresoHistorico(ctx context.Context, empresaID, desde, hasta string) ([]FilaIngreso, error) {
	rows, err := r.pool.Query(ctx, ingresoHistoricoSQL, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: ingreso histórico: %w", err)
	}
	defer rows.Close()
	out := []FilaIngreso{}
	for rows.Next() {
		var f FilaIngreso
		if err := rows.Scan(&f.ClasificacionID, &f.Clasificacion, &f.Concepto, &f.Real); err != nil {
			return nil, fmt.Errorf("bancos: scan ingreso histórico: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
