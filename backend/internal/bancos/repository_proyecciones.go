package bancos

// Fase C — Proyecciones: sendas diarias de ingresos, líneas de ingreso y persistencia de escenarios.
//
// Ingreso = la MISMA definición que el KPI y la tendencia (naturaleza.go): lo que el usuario declaró
// como concepto de ingreso, no «todo crédito que no sea traslado». Con la definición vieja la senda
// incluía ahorro, reservas y aportes entre empresas, así que el cierre de mes se proyectaba contra
// un ingreso que no era ingreso.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

func (r *pgRepository) SendaIngresos(ctx context.Context, empresaID, periodo string) ([]DiaMonto, error) {
	q := `
		SELECT EXTRACT(DAY FROM m.fecha)::int, ` + sqlIngresoNeto + `
		FROM movimiento_bancario m
		` + joinConcepto + `
		WHERE m.empresa_id = $1::uuid AND m.incluido = true AND to_char(m.fecha, 'YYYY-MM') = $2
		GROUP BY 1
		HAVING ` + sqlIngresoNeto + ` <> 0
		ORDER BY 1`
	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("bancos: senda ingresos: %w", err)
	}
	defer rows.Close()
	var out []DiaMonto
	for rows.Next() {
		var dm DiaMonto
		if err := rows.Scan(&dm.Dia, &dm.Monto); err != nil {
			return nil, fmt.Errorf("bancos: scan senda: %w", err)
		}
		out = append(out, dm)
	}
	return out, rows.Err()
}

func (r *pgRepository) SendasIngresosRango(ctx context.Context, empresaID string, periodos []string) ([]SendaMes, error) {
	if len(periodos) == 0 {
		return nil, nil
	}
	q := `
		SELECT to_char(m.fecha, 'YYYY-MM'), EXTRACT(DAY FROM m.fecha)::int, ` + sqlIngresoNeto + `
		FROM movimiento_bancario m
		` + joinConcepto + `
		WHERE m.empresa_id = $1::uuid AND m.incluido = true AND to_char(m.fecha, 'YYYY-MM') = ANY($2)
		GROUP BY 1, 2
		ORDER BY 1, 2`
	rows, err := r.pool.Query(ctx, q, empresaID, periodos)
	if err != nil {
		return nil, fmt.Errorf("bancos: sendas rango: %w", err)
	}
	defer rows.Close()
	porPeriodo := map[string]*SendaMes{}
	var orden []string
	for rows.Next() {
		var p string
		var dm DiaMonto
		if err := rows.Scan(&p, &dm.Dia, &dm.Monto); err != nil {
			return nil, fmt.Errorf("bancos: scan senda rango: %w", err)
		}
		if porPeriodo[p] == nil {
			porPeriodo[p] = &SendaMes{Periodo: p}
			orden = append(orden, p)
		}
		porPeriodo[p].Dias = append(porPeriodo[p].Dias, dm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("bancos: iterar sendas: %w", err)
	}
	out := make([]SendaMes, 0, len(orden))
	for _, p := range orden {
		out = append(out, *porPeriodo[p])
	}
	return out, nil
}

func (r *pgRepository) IngresosPorClasificacion(ctx context.Context, empresaID, periodo string) ([]LineaIngreso, error) {
	const q = `
		SELECT COALESCE(cl.id::text, ''),
		       CASE WHEN cl.id IS NULL THEN '(sin clasificar)'
		            ELSE co.nombre || ' › ' || cl.nombre END,
		       SUM(m.monto_crc)
		FROM movimiento_bancario m
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		LEFT JOIN concepto co ON co.id = cl.concepto_id
		WHERE m.empresa_id = $1::uuid AND m.incluido = true AND to_char(m.fecha, 'YYYY-MM') = $2
		  AND m.credito > 0 AND NOT m.es_traslado
		GROUP BY 1, 2
		ORDER BY 3 DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("bancos: ingresos por línea: %w", err)
	}
	defer rows.Close()
	var out []LineaIngreso
	for rows.Next() {
		var l LineaIngreso
		var monto decimal.Decimal
		if err := rows.Scan(&l.ClasificacionID, &l.Nombre, &monto); err != nil {
			return nil, fmt.Errorf("bancos: scan línea ingreso: %w", err)
		}
		l.Real = monto.String()
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *pgRepository) GuardarEscenario(ctx context.Context, empresaID string, e EscenarioNuevo) (string, error) {
	const q = `
		INSERT INTO proyeccion_escenario
			(empresa_id, periodo, metodo, metodo_efectivo, meta_crecimiento_pct,
			 lineas_ingreso, dia_calculo, real_acumulado, cierre_proyectado, meta_monto, creado_por)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::uuid)
		RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, e.Periodo, e.Metodo, e.MetodoEfectivo, e.MetaPct,
		e.LineasIngreso, e.DiaCalculo, e.RealAcumulado, e.CierreProyectado, e.MetaMonto, e.CreadoPor).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("bancos: guardar escenario: %w", err)
	}
	return id, nil
}

func (r *pgRepository) ListarEscenarios(ctx context.Context, empresaID, periodo string) ([]EscenarioGuardado, error) {
	// real_cierre: los ingresos reales del período proyectado (para medir precisión
	// cuando el mes ya cerró; el frontend decide cuándo mostrarla).
	conds := "e.empresa_id = $1::uuid"
	args := []any{empresaID}
	if periodo != "" {
		conds += " AND e.periodo = $2"
		args = append(args, periodo)
	}
	q := `
		SELECT e.id::text, e.periodo, e.metodo, e.metodo_efectivo, e.meta_crecimiento_pct,
		       e.dia_calculo, e.real_acumulado, e.cierre_proyectado, e.meta_monto,
		       to_char(e.creado_en, 'YYYY-MM-DD'),
		       COALESCE((SELECT SUM(CASE WHEN m.credito > 0 AND NOT m.es_traslado THEN m.monto_crc ELSE 0 END)
		                 FROM movimiento_bancario m
		                 WHERE m.empresa_id = e.empresa_id AND m.incluido = true
		                   AND to_char(m.fecha, 'YYYY-MM') = e.periodo), 0)
		FROM proyeccion_escenario e
		WHERE ` + conds + `
		ORDER BY e.creado_en DESC
		LIMIT 50`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar escenarios: %w", err)
	}
	defer rows.Close()
	var out []EscenarioGuardado
	for rows.Next() {
		var e EscenarioGuardado
		var metaPct, realAcum, cierre, meta, realCierre decimal.Decimal
		if err := rows.Scan(&e.ID, &e.Periodo, &e.Metodo, &e.MetodoEfectivo, &metaPct,
			&e.DiaCalculo, &realAcum, &cierre, &meta, &e.CreadoEn, &realCierre); err != nil {
			return nil, fmt.Errorf("bancos: scan escenario: %w", err)
		}
		e.MetaPct = metaPct.String()
		e.RealAcumulado = realAcum.String()
		e.CierreProyectado = cierre.String()
		e.MetaMonto = meta.String()
		e.RealCierre = realCierre.String()
		out = append(out, e)
	}
	return out, rows.Err()
}
