package nomina

// Consultas agregadas del dashboard de RRHH. Todas filtran por empresa_id y solo miran
// corridas VIVAS (no ANULADA): el mes tiene a lo sumo un adelanto y una liquidación vivos.

import (
	"context"
	"fmt"
)

// ResumenNominaMes agrega los totales del mes (borrador incluido: el dashboard sirve para
// decidir antes de aprobar). El bruto excluye las colillas de ADELANTO para no contar dos
// veces el mismo salario del mes en la jornada mensual.
func (r *pgRepository) ResumenNominaMes(ctx context.Context, empresaID string, anio, mes int) (ResumenMes, error) {
	const q = `
		SELECT
			COALESCE(SUM(cl.bruto) FILTER (WHERE cl.tratamiento <> 'ADELANTO'), 0)::text,
			COALESCE(SUM(cl.base_ccss), 0)::text,
			COALESCE(SUM(cl.patronal), 0)::text,
			COALESCE(SUM(cl.prov_aguinaldo), 0)::text,
			COALESCE(SUM(cl.prov_vacaciones), 0)::text,
			COALESCE(SUM(cl.prov_cesantia), 0)::text,
			COALESCE(SUM(cl.neto), 0)::text,
			COALESCE(SUM(cl.neto) FILTER (WHERE c.tipo = 'LIQUIDACION'), 0)::text,
			COUNT(DISTINCT cl.empleado_id)
		FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
		WHERE cl.empresa_id = $1::uuid AND c.anio = $2 AND c.mes = $3 AND c.estado <> 'ANULADA'`
	var m ResumenMes
	if err := r.pool.QueryRow(ctx, q, empresaID, anio, mes).Scan(&m.Bruto, &m.BaseCCSS, &m.Patronal,
		&m.ProvAguinaldo, &m.ProvVacaciones, &m.ProvCesantia, &m.Neto, &m.NetoLiquidacion,
		&m.Empleados); err != nil {
		return ResumenMes{}, fmt.Errorf("nomina: resumen del mes: %w", err)
	}
	return m, nil
}

// TendenciaCostoNomina devuelve el costo real (bruto sin adelantos + patronal + provisiones)
// de los meses del rango [desde, hasta] (fechas 'YYYY-MM-01'), solo los que tienen corrida.
func (r *pgRepository) TendenciaCostoNomina(ctx context.Context, empresaID, desde, hasta string) ([]CostoMes, error) {
	const q = `
		SELECT c.anio, c.mes,
			COALESCE(SUM(cl.bruto) FILTER (WHERE cl.tratamiento <> 'ADELANTO'), 0)
			+ COALESCE(SUM(cl.patronal), 0)
			+ COALESCE(SUM(cl.prov_aguinaldo + cl.prov_vacaciones + cl.prov_cesantia), 0)
		FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
		WHERE cl.empresa_id = $1::uuid AND c.estado <> 'ANULADA'
			AND make_date(c.anio, c.mes, 1) BETWEEN $2::date AND $3::date
		GROUP BY c.anio, c.mes
		ORDER BY c.anio, c.mes`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("nomina: tendencia de costo: %w", err)
	}
	defer rows.Close()
	items := make([]CostoMes, 0, 12)
	for rows.Next() {
		var c CostoMes
		if err := rows.Scan(&c.Anio, &c.Mes, &c.Costo); err != nil {
			return nil, fmt.Errorf("nomina: scan tendencia: %w", err)
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// CorridasVivasDelMes devuelve las corridas no anuladas del mes (tipo, estado, id).
func (r *pgRepository) CorridasVivasDelMes(ctx context.Context, empresaID string, anio, mes int) ([]EstadoCorridaMes, error) {
	const q = `
		SELECT id::text, tipo, estado FROM corrida_nomina
		WHERE empresa_id = $1::uuid AND anio = $2 AND mes = $3 AND estado <> 'ANULADA'
		ORDER BY tipo`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("nomina: corridas vivas del mes: %w", err)
	}
	defer rows.Close()
	items := make([]EstadoCorridaMes, 0, 2)
	for rows.Next() {
		var c EstadoCorridaMes
		if err := rows.Scan(&c.ID, &c.Tipo, &c.Estado); err != nil {
			return nil, fmt.Errorf("nomina: scan corrida viva: %w", err)
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// HeadcountPorDepartamento cuenta los empleados activos por departamento.
func (r *pgRepository) HeadcountPorDepartamento(ctx context.Context, empresaID string) ([]DashboardDepto, error) {
	const q = `
		SELECT COALESCE(d.nombre, 'Sin departamento'), COUNT(*)
		FROM empleado e LEFT JOIN departamento d ON d.id = e.departamento_id
		WHERE e.empresa_id = $1::uuid AND e.activo
		GROUP BY 1
		ORDER BY 2 DESC, 1`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: headcount por departamento: %w", err)
	}
	defer rows.Close()
	items := make([]DashboardDepto, 0, 8)
	for rows.Next() {
		var d DashboardDepto
		if err := rows.Scan(&d.Departamento, &d.Empleados); err != nil {
			return nil, fmt.Errorf("nomina: scan headcount: %w", err)
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

// AvisosNominaMes reúne los hechos que alimentan las alertas: empleados activos sin IBAN
// (bloquean el archivo SINPE), deducciones con saldo vivo, conceptos de ingreso excluidos
// de CCSS (y cuántos sin base legal — control del guardarraíl) e incapacidades del mes.
func (r *pgRepository) AvisosNominaMes(ctx context.Context, empresaID string, anio, mes int) (AvisosNomina, error) {
	var a AvisosNomina
	const qIBAN = `
		SELECT COUNT(*), COALESCE(ARRAY_AGG(nombre ORDER BY nombre), '{}')
		FROM empleado
		WHERE empresa_id = $1::uuid AND activo AND COALESCE(iban, '') = ''`
	if err := r.pool.QueryRow(ctx, qIBAN, empresaID).Scan(&a.SinIBAN, &a.NombresSinIBAN); err != nil {
		return AvisosNomina{}, fmt.Errorf("nomina: empleados sin IBAN: %w", err)
	}
	const qDeds = `
		SELECT COUNT(*), COALESCE(SUM(de.saldo_restante), 0)::text
		FROM deduccion_empleado de JOIN empleado e ON e.id = de.empleado_id
		WHERE de.empresa_id = $1::uuid AND de.activo AND e.activo
			AND de.saldo_restante IS NOT NULL AND de.saldo_restante > 0`
	if err := r.pool.QueryRow(ctx, qDeds, empresaID).Scan(&a.DeduccionesActivas, &a.SaldoDeducciones); err != nil {
		return AvisosNomina{}, fmt.Errorf("nomina: deducciones con saldo: %w", err)
	}
	const qConceptos = `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE COALESCE(base_legal, '') = '')
		FROM concepto_nomina
		WHERE empresa_id = $1::uuid AND activo AND tipo = 'INGRESO' AND NOT afecta_ccss`
	if err := r.pool.QueryRow(ctx, qConceptos, empresaID).Scan(&a.ConceptosNoAfectos, &a.SinBaseLegal); err != nil {
		return AvisosNomina{}, fmt.Errorf("nomina: conceptos no afectos: %w", err)
	}
	const qIncap = `
		SELECT COUNT(*)
		FROM incapacidad i
		WHERE i.empresa_id = $1::uuid AND NOT i.anulada
			AND i.fecha_inicio <= make_date($2, $3, 1) + INTERVAL '1 month' - INTERVAL '1 day'
			AND (i.fecha_inicio + (i.dias - 1) * INTERVAL '1 day') >= make_date($2, $3, 1)`
	if err := r.pool.QueryRow(ctx, qIncap, empresaID, anio, mes).Scan(&a.IncapacidadesMes); err != nil {
		return AvisosNomina{}, fmt.Errorf("nomina: incapacidades del mes: %w", err)
	}
	return a, nil
}
