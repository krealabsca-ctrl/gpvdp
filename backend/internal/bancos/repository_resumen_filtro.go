package bancos

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// ResumenFiltroRow es una fila del desglose (padre × hijo) de la selección activa.
//
// Movs se cuenta APARTE de los conteos por lado. Podría derivarse de débitos + créditos,
// pero eso asumiría que todo movimiento tiene exactamente un lado; si alguna importación
// dejara un monto en cero, el encabezado dejaría de cuadrar con la lista sin avisar.
type ResumenFiltroRow struct {
	PadreID, Padre string
	HijoID, Hijo   string
	CreditoSum     decimal.Decimal
	DebitoSum      decimal.Decimal
	Movs           int
}

// ResumenFiltro agrega los movimientos de la SELECCIÓN ACTIVA de la hoja de trabajo,
// separando débito y crédito, con su conteo, en dos niveles (padre → hijo).
//
// Dos decisiones que importan y que no son obvias:
//
//   - Los montos salen de `monto_crc`, NO de las columnas `debito`/`credito`. Esas
//     vienen en la moneda de la cuenta: sumar dólares con colones daría un número
//     falso (hay 99 movimientos en USD). El resumen se expresa en colones.
//   - El WHERE lo arma `condicionesMovimientos`, el MISMO que la lista. Así el
//     encabezado y la tabla no pueden contradecirse nunca.
//
// El agrupamiento cambia según el área de trabajo (ver agrupamientoResumen).
func (r *pgRepository) ResumenFiltro(ctx context.Context, empresaID string, f FiltrosMovimientos, agrupar string) ([]ResumenFiltroRow, error) {
	where, args := condicionesMovimientos(empresaID, f)
	sel := agrupamientoResumen(agrupar)

	q := `
		SELECT ` + sel.padreID + `, ` + sel.padreNombre + `,
		       ` + sel.hijoID + `, ` + sel.hijoNombre + `,
		       COALESCE(SUM(CASE WHEN m.credito > 0 THEN m.monto_crc ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN m.debito  > 0 THEN m.monto_crc ELSE 0 END), 0),
		       COUNT(*)
		FROM movimiento_bancario m
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE ` + where + `
		GROUP BY ` + sel.groupBy + `
		ORDER BY ` + sel.orderBy

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("bancos: resumen de la selección: %w", err)
	}
	defer rows.Close()
	var out []ResumenFiltroRow
	for rows.Next() {
		var row ResumenFiltroRow
		if err := rows.Scan(&row.PadreID, &row.Padre, &row.HijoID, &row.Hijo,
			&row.CreditoSum, &row.DebitoSum, &row.Movs); err != nil {
			return nil, fmt.Errorf("bancos: scan resumen de la selección: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// agrupamiento son las expresiones SQL de un desglose de dos niveles.
// Todas son literales de código (whitelist): nunca llega texto del cliente al SQL.
type agrupamiento struct {
	padreID, padreNombre string
	hijoID, hijoNombre   string
	groupBy, orderBy     string
}

// agrupamientoResumen elige el desglose útil según el área de trabajo:
//
//   - "cuenta": Banco → cuenta. Es el único que sirve en «Por clasificar», donde
//     todos los movimientos son «(sin concepto)» por definición.
//   - "concepto" (default): Concepto → Clasificación, igual que el Cuadre.
func agrupamientoResumen(agrupar string) agrupamiento {
	if agrupar == "cuenta" {
		return agrupamiento{
			padreID:     "COALESCE(b.id::text, '')",
			padreNombre: "COALESCE(b.nombre, '(sin banco)')",
			hijoID:      "COALESCE(cb.id::text, '')",
			hijoNombre:  "COALESCE(cb.alias, '(sin cuenta)')",
			groupBy:     "b.id, b.nombre, cb.id, cb.alias",
			orderBy:     "b.nombre, cb.alias",
		}
	}
	return agrupamiento{
		padreID:     "COALESCE(m.concepto_id::text, '')",
		padreNombre: "COALESCE(co.nombre, '(sin concepto)')",
		hijoID:      "COALESCE(m.clasificacion_id::text, '')",
		hijoNombre:  "COALESCE(cl.nombre, '(sin clasificación)')",
		groupBy:     "m.concepto_id, co.nombre, m.clasificacion_id, cl.nombre",
		orderBy:     "co.nombre, cl.nombre",
	}
}
