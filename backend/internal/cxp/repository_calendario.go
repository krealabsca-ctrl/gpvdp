package cxp

// Las dos consultas del calendario de pagos. El porqué está en calendario.go.

import (
	"context"
	"fmt"
)

// estadosVivosDelCalendario son los que todavía esperan plata.
//
// DENEGADO y ANULADO quedan fuera —no se van a pagar— y también LIQUIDADA, PAGADO y CONCILIADO,
// que ya salieron. Se nombra la lista y no se usa un `NOT IN` con los cerrados: un estado nuevo
// tiene que entrar acá a propósito, no colarse por omisión.
const estadosVivosDelCalendario = `('RECIBIDO','REVISADO','VALIDADO_DEPTO','APROBADO','PROGRAMADO','REBOTADA')`

// FilaProgramada es una fecha de pago con lo que sale ese día, tal como viene de la base.
type FilaProgramada struct {
	Fecha    string
	Facturas int
	TotalCRC string
}

// ProgramadoPorFecha trae lo que tiene fecha de pago, agrupado por día.
//
// El monto es el NETO, igual que el cronograma y el archivo de pago: el total nunca sale del
// banco. Un calendario armado sobre el total prometería más plata de la que va a salir.
func (r *pgRepository) ProgramadoPorFecha(ctx context.Context, empresaID string) ([]FilaProgramada, error) {
	q := `
		SELECT to_char(d.fecha_pago_programada, 'YYYY-MM-DD'),
		       COUNT(*)::int,
		       COALESCE(SUM(` + netoCRCSQL + `), 0)::text
		FROM documento_cxp d` + JoinsNetoSQL + `
		WHERE d.empresa_id = $1::uuid
		  AND d.fecha_pago_programada IS NOT NULL
		  AND d.estado IN ` + estadosVivosDelCalendario + `
		GROUP BY d.fecha_pago_programada
		ORDER BY d.fecha_pago_programada`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: programado por fecha: %w", err)
	}
	defer rows.Close()

	out := make([]FilaProgramada, 0, 16)
	for rows.Next() {
		var f FilaProgramada
		if err := rows.Scan(&f.Fecha, &f.Facturas, &f.TotalCRC); err != nil {
			return nil, fmt.Errorf("cxp: scan programado: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FilaVencida es un estado del flujo con lo vencido que está parado ahí.
type FilaVencida struct {
	Estado   string
	Facturas int
	TotalCRC string
}

// VencidoPorEstado trae lo vencido SIN fecha de pago, agrupado por dónde está trabado.
//
// Excluye lo que ya tiene fecha: eso vive en el otro bloque y contarlo dos veces haría que los
// dos totales no sumen la cartera.
func (r *pgRepository) VencidoPorEstado(ctx context.Context, empresaID string) ([]FilaVencida, error) {
	q := `
		SELECT d.estado, COUNT(*)::int, COALESCE(SUM(` + netoCRCSQL + `), 0)::text
		FROM documento_cxp d` + JoinsNetoSQL + `
		WHERE d.empresa_id = $1::uuid
		  AND d.fecha_pago_programada IS NULL
		  AND d.fecha_vencimiento IS NOT NULL
		  AND d.fecha_vencimiento < CURRENT_DATE
		  AND d.estado IN ` + estadosVivosDelCalendario + `
		GROUP BY d.estado`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: vencido por estado: %w", err)
	}
	defer rows.Close()

	out := make([]FilaVencida, 0, 8)
	for rows.Next() {
		var f FilaVencida
		if err := rows.Scan(&f.Estado, &f.Facturas, &f.TotalCRC); err != nil {
			return nil, fmt.Errorf("cxp: scan vencido: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
