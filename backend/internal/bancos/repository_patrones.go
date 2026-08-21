package bancos

// Datos para el descubridor de patrones. Filtra por empresa_id como todo el módulo.

import (
	"context"
	"fmt"
)

// LineasSinClasificar trae los movimientos NO_IDENTIFICADO de la empresa (opcionalmente de un
// período) para agruparlos por forma.
func (r *pgRepository) LineasSinClasificar(ctx context.Context, empresaID, periodo string) ([]LineaSinClasificar, error) {
	q := `
		SELECT m.descripcion, m.debito > 0, (m.debito + m.credito)
		FROM movimiento_bancario m
		WHERE m.empresa_id = $1::uuid AND m.incluido
		  AND m.estado_clasificacion = 'NO_IDENTIFICADO'`
	args := []any{empresaID}
	if periodo != "" {
		q += ` AND to_char(m.fecha, 'YYYY-MM') = $2`
		args = append(args, periodo)
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("bancos: líneas sin clasificar: %w", err)
	}
	defer rows.Close()
	out := make([]LineaSinClasificar, 0, 1024)
	for rows.Next() {
		var l LineaSinClasificar
		if err := rows.Scan(&l.Descripcion, &l.EsDebito, &l.Monto); err != nil {
			return nil, fmt.Errorf("bancos: scan línea sin clasificar: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DescripcionesEmpresa trae las descripciones de TODOS los movimientos incluidos. Sirven para
// medir el alcance de una palabra clave propuesta: una regla solo pisa los NO_IDENTIFICADO,
// pero si la palabra también calza con movimientos ya clasificados es demasiado genérica y
// mañana atraparía lo que no debe.
func (r *pgRepository) DescripcionesEmpresa(ctx context.Context, empresaID string) ([]string, error) {
	const q = `
		SELECT m.descripcion FROM movimiento_bancario m
		WHERE m.empresa_id = $1::uuid AND m.incluido`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: descripciones de la empresa: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, 1024)
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("bancos: scan descripción: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
