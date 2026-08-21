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
	q := `
		SELECT ` + sqlIngresoNeto + `, ` + sqlGastoNeto + `,
			COUNT(*) FILTER (WHERE m.estado_clasificacion = 'NO_IDENTIFICADO'),
			` + sqlFueraDelEbitda + `, ` + sqlMovsFueraDelEbitda + `
		FROM movimiento_bancario m
		` + joinConcepto + `
		WHERE m.empresa_id = $1::uuid AND to_char(m.fecha, 'YYYY-MM') = $2 AND m.incluido = true`
	var t TotalesEbitda
	if err := r.pool.QueryRow(ctx, q, empresaID, periodo).
		Scan(&t.Ingresos, &t.Gastos, &t.NoIdentificados, &t.MontoFueraEbitda, &t.MovsFueraEbitda); err != nil {
		return TotalesEbitda{}, fmt.Errorf("bancos: totales período: %w", err)
	}
	return t, nil
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
