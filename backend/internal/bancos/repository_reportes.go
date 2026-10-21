package bancos

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

func (r *pgRepository) Cuadre(ctx context.Context, empresaID, periodo string) ([]CuadreRow, error) {
	const q = `
		SELECT COALESCE(m.concepto_id::text, ''), COALESCE(co.nombre, '(sin concepto)'),
		       COALESCE(SUM(CASE WHEN m.credito > 0 THEN m.monto_crc ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN m.debito  > 0 THEN m.monto_crc ELSE 0 END), 0)
		FROM movimiento_bancario m
		LEFT JOIN concepto co ON co.id = m.concepto_id
		WHERE m.empresa_id = $1::uuid AND to_char(m.fecha, 'YYYY-MM') = $2 AND m.incluido = true
		GROUP BY m.concepto_id, co.nombre
		ORDER BY co.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("bancos: cuadre: %w", err)
	}
	defer rows.Close()
	var out []CuadreRow
	for rows.Next() {
		var (
			row       CuadreRow
			cred, deb decimal.Decimal
		)
		if err := rows.Scan(&row.ConceptoID, &row.Concepto, &cred, &deb); err != nil {
			return nil, fmt.Errorf("bancos: scan cuadre: %w", err)
		}
		row.TotalCreditos = cred.String()
		row.TotalDebitos = deb.String()
		out = append(out, row)
	}
	return out, rows.Err()
}

// TotalesPeriodo devuelve el ingreso y el gasto del período SEGÚN LA NATURALEZA DEL CONCEPTO
// (ver naturaleza.go), más lo que quedó fuera del EBITDA por no estar declarado.
//
// Antes definía ingreso = «crédito que no es traslado» y gasto = «débito que no es traslado», y así
// el ahorro, las reservas, los préstamos, los aportes entre empresas y lo sin clasificar entraban al
// EBITDA. En agosto 2026 eso inflaba los gastos de Valle de Paz en ₡35,3 millones.
func (r *pgRepository) TotalesPeriodo(ctx context.Context, empresaID, periodo string) (TotalesEbitda, error) {
	return r.TotalesPeriodoHastaDia(ctx, empresaID, periodo, 0)
}

// TotalesPeriodoHastaDia suma el período CORTANDO al día indicado. `dia = 0` = el mes entero.
//
// Existe para comparar a paridad: un mes en curso contra el mismo tramo del mes anterior. Mientras
// el tablero comparó un mes a medias contra meses enteros, el porcentaje no medía el negocio sino
// cuántos días habían pasado — medido sobre los datos reales, setiembre (cargado hasta el día 14)
// salía «−62 % vs agosto» cuando a paridad de día la caída es del 35 %. Una alarma que aparece
// todos los meses por construcción deja de leerse.
func (r *pgRepository) TotalesPeriodoHastaDia(ctx context.Context, empresaID, periodo string, dia int) (TotalesEbitda, error) {
	q := `
		SELECT ` + sqlIngresoNeto + `, ` + sqlGastoNeto + `,
			COUNT(*) FILTER (WHERE m.estado_clasificacion = 'NO_IDENTIFICADO'),
			` + sqlFueraDelEbitda + `, ` + sqlMovsFueraDelEbitda + `
		FROM movimiento_bancario m
		` + joinConcepto + `
		WHERE m.empresa_id = $1::uuid AND to_char(m.fecha, 'YYYY-MM') = $2 AND m.incluido = true
		  -- $3 = 0 trae el mes entero, que es el caso de un mes ya cerrado: cortarlo por un día
		  -- dejaría afuera plata que sí ocurrió.
		  AND ($3 = 0 OR extract(day from m.fecha) <= $3)`
	var t TotalesEbitda
	if err := r.pool.QueryRow(ctx, q, empresaID, periodo, dia).
		Scan(&t.Ingresos, &t.Gastos, &t.NoIdentificados, &t.MontoFueraEbitda, &t.MovsFueraEbitda); err != nil {
		return TotalesEbitda{}, fmt.Errorf("bancos: totales período: %w", err)
	}
	return t, nil
}

// UltimoDiaCargado devuelve el día del mes del movimiento más reciente del período (0 si no hay
// ninguno).
//
// El tablero corta por el día de HOY —decisión del Director Financiero: «sería al 19, porque si
// algo no se ha cargado se termine de cargar»— y este número es lo que hace que esa decisión
// funcione. Sin él, un estado de cuenta atrasado se ve igual que una caída de ventas: los días sin
// cargar entran al promedio como ceros y nadie puede distinguir las dos cosas desde la pantalla.
func (r *pgRepository) UltimoDiaCargado(ctx context.Context, empresaID, periodo string) (int, error) {
	const q = `
		SELECT COALESCE(MAX(extract(day from fecha))::int, 0)
		FROM movimiento_bancario
		WHERE empresa_id = $1::uuid AND to_char(fecha, 'YYYY-MM') = $2 AND incluido = true`
	var dia int
	if err := r.pool.QueryRow(ctx, q, empresaID, periodo).Scan(&dia); err != nil {
		return 0, fmt.Errorf("bancos: último día cargado: %w", err)
	}
	return dia, nil
}

// ConceptosSinNaturaleza cuenta los conceptos EN USO (con movimientos) cuya naturaleza NADIE
// declaró todavía.
//
// El criterio es `NOT naturaleza_declarada`, no `naturaleza = 'NEUTRO'` (migración 0064). Antes
// contaba el valor NEUTRO, y así declarar «Ahorro» como NEUTRO —la respuesta correcta— no bajaba el
// aviso: la única forma de apagarlo era meter al EBITDA algo que no debe entrar. El aviso empujaba
// al error que se quería evitar.
//
// Se limita a los que tienen movimientos porque son los únicos que afectan el número: un concepto
// creado y nunca usado no deja el EBITDA corto, y contarlo convertiría el aviso en ruido permanente.
func (r *pgRepository) ConceptosSinNaturaleza(ctx context.Context, empresaID string) (int, error) {
	const q = `
		SELECT COUNT(*) FROM concepto co
		WHERE co.empresa_id = $1::uuid AND co.activo AND NOT co.naturaleza_declarada
		  AND EXISTS (SELECT 1 FROM movimiento_bancario m
		              WHERE m.empresa_id = co.empresa_id AND m.concepto_id = co.id
		                AND m.incluido AND NOT m.es_traslado)`
	var n int
	if err := r.pool.QueryRow(ctx, q, empresaID).Scan(&n); err != nil {
		return 0, fmt.Errorf("bancos: conceptos sin naturaleza: %w", err)
	}
	return n, nil
}

// TotalesEbitda es el resultado de TotalesPeriodo. Lleva lo que quedó FUERA a propósito: un total
// que omite algo sin decirlo es un total que miente por silencio.
type TotalesEbitda struct {
	Ingresos decimal.Decimal
	Gastos   decimal.Decimal
	// NoIdentificados: movimientos sin clasificar del período (bloquean el cierre, §19).
	NoIdentificados int
	// MontoFueraEbitda / MovsFueraEbitda: lo que no entró porque su concepto es NEUTRO o no tiene
	// concepto. Sirve para avisar «faltan conceptos por declarar», no para corregir el número.
	MontoFueraEbitda decimal.Decimal
	MovsFueraEbitda  int
}
