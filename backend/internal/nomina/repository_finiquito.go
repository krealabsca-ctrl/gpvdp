package nomina

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Métodos de finiquito y reportes del Repository (implementados por pgRepository).

const finiquitoCols = `f.id::text, f.empleado_id::text, e.nombre, e.identificacion, e.fecha_ingreso::text,
	f.motivo, f.fecha_salida::text, f.estado, f.dias_vacaciones::text, f.salario_promedio::text,
	f.salario_diario::text, f.anios_servicio, f.preaviso::text, f.cesantia::text, f.vacaciones::text,
	f.aguinaldo::text, f.base_ccss::text, f.ccss_obrero::text, f.renta::text, f.patronal::text,
	f.descuentos::text, f.total::text, f.detalle, f.dias_vacaciones_manual,
	f.creado_en::text, COALESCE(f.aprobado_en::text, ''), COALESCE(f.pagado_en::text, '')`

const finiquitoFrom = ` FROM finiquito f JOIN empleado e ON e.id = f.empleado_id`

func scanFiniquito(row scanner) (Finiquito, error) {
	var fi Finiquito
	var detalle []byte
	err := row.Scan(&fi.ID, &fi.EmpleadoID, &fi.EmpleadoNombre, &fi.Identificacion, &fi.FechaIngreso,
		&fi.Motivo, &fi.FechaSalida, &fi.Estado, &fi.DiasVacaciones, &fi.SalarioPromedio,
		&fi.SalarioDiario, &fi.AniosServicio, &fi.Preaviso, &fi.Cesantia, &fi.Vacaciones,
		&fi.Aguinaldo, &fi.BaseCCSS, &fi.CCSSObrero, &fi.Renta, &fi.Patronal,
		&fi.Descuentos, &fi.Total, &detalle, &fi.DiasManual,
		&fi.CreadoEn, &fi.AprobadoEn, &fi.PagadoEn)
	if err != nil {
		return Finiquito{}, err
	}
	if err := json.Unmarshal(detalle, &fi.Detalle); err != nil {
		return Finiquito{}, fmt.Errorf("nomina: detalle de finiquito corrupto: %w", err)
	}
	return fi, nil
}

func (r *pgRepository) ListarFiniquitos(ctx context.Context, empresaID string) ([]Finiquito, error) {
	q := "SELECT " + finiquitoCols + finiquitoFrom + ` WHERE f.empresa_id = $1::uuid
		ORDER BY f.creado_en DESC`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar finiquitos: %w", err)
	}
	defer rows.Close()
	items := make([]Finiquito, 0, 8)
	for rows.Next() {
		fi, err := scanFiniquito(rows)
		if err != nil {
			return nil, fmt.Errorf("nomina: scan finiquito: %w", err)
		}
		items = append(items, fi)
	}
	return items, rows.Err()
}

func (r *pgRepository) FiniquitoPorID(ctx context.Context, empresaID, id string) (Finiquito, error) {
	q := "SELECT " + finiquitoCols + finiquitoFrom + " WHERE f.empresa_id = $1::uuid AND f.id = $2::uuid"
	fi, err := scanFiniquito(r.pool.QueryRow(ctx, q, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Finiquito{}, ErrFiniquitoNoEncontrado
	}
	if err != nil {
		return Finiquito{}, fmt.Errorf("nomina: finiquito por id: %w", err)
	}
	return fi, nil
}

// GuardarFiniquito inserta (id vacío) o actualiza EN BORRADOR el snapshot calculado.
func (r *pgRepository) GuardarFiniquito(ctx context.Context, empresaID, id string, in FiniquitoInput, res ResultadoFiniquito, salarioPromedio, diasVacaciones decimal.Decimal, usuarioID string) (Finiquito, error) {
	detalle, err := json.Marshal(res.Detalle)
	if err != nil {
		return Finiquito{}, fmt.Errorf("nomina: serializar finiquito: %w", err)
	}
	if id == "" {
		q := `
			WITH ins AS (
				INSERT INTO finiquito (empresa_id, empleado_id, motivo, fecha_salida, dias_vacaciones,
					salario_promedio, salario_diario, anios_servicio, preaviso, cesantia, vacaciones,
					aguinaldo, base_ccss, ccss_obrero, renta, descuentos, total, detalle,
					dias_vacaciones_manual, creado_por, patronal)
				SELECT $1::uuid, e.id, $3, $4::date, $5::numeric, $6::numeric, $7::numeric, $8,
					$9::numeric, $10::numeric, $11::numeric, $12::numeric, $13::numeric, $14::numeric,
					$15::numeric, $16::numeric, $17::numeric, $18::jsonb, $20, $19::uuid, $21::numeric
				FROM empleado e WHERE e.id = $2::uuid AND e.empresa_id = $1::uuid
				RETURNING *
			)
			SELECT ` + finiquitoCols + ` FROM ins f JOIN empleado e ON e.id = f.empleado_id`
		fi, err := scanFiniquito(r.pool.QueryRow(ctx, q, empresaID, in.EmpleadoID, in.Motivo, in.FechaSalida,
			diasVacaciones, salarioPromedio, res.SalarioDiario, res.AniosServicio, res.Preaviso, res.Cesantia,
			res.Vacaciones, res.Aguinaldo, res.BaseCCSS, res.CCSSObrero, res.Renta,
			res.Descuentos, res.Total, detalle, usuarioID, in.DiasManual, res.Patronal))
		if esViolacionUnica(err) {
			return Finiquito{}, ErrFiniquitoDuplicado
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return Finiquito{}, ErrEmpleadoNoEncontrado
		}
		if err != nil {
			return Finiquito{}, fmt.Errorf("nomina: crear finiquito: %w", err)
		}
		return fi, nil
	}
	q := `
		WITH upd AS (
			UPDATE finiquito SET motivo = $3, fecha_salida = $4::date, dias_vacaciones = $5::numeric,
				salario_promedio = $6::numeric, salario_diario = $7::numeric, anios_servicio = $8,
				preaviso = $9::numeric, cesantia = $10::numeric, vacaciones = $11::numeric,
				aguinaldo = $12::numeric, base_ccss = $13::numeric, ccss_obrero = $14::numeric,
				renta = $15::numeric, descuentos = $16::numeric, total = $17::numeric, detalle = $18::jsonb,
				dias_vacaciones_manual = $19, patronal = $20::numeric
			WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'BORRADOR'
			RETURNING *
		)
		SELECT ` + finiquitoCols + ` FROM upd f JOIN empleado e ON e.id = f.empleado_id`
	fi, err := scanFiniquito(r.pool.QueryRow(ctx, q, empresaID, id, in.Motivo, in.FechaSalida,
		diasVacaciones, salarioPromedio, res.SalarioDiario, res.AniosServicio, res.Preaviso, res.Cesantia,
		res.Vacaciones, res.Aguinaldo, res.BaseCCSS, res.CCSSObrero, res.Renta,
		res.Descuentos, res.Total, detalle, in.DiasManual, res.Patronal))
	if errors.Is(err, pgx.ErrNoRows) {
		return Finiquito{}, ErrFiniquitoNoEditable
	}
	if err != nil {
		return Finiquito{}, fmt.Errorf("nomina: actualizar finiquito: %w", err)
	}
	return fi, nil
}

// AprobarFiniquito congela el borrador con LOCKING OPTIMISTA: el UPDATE exige que el
// motivo, la fecha de salida y los días de vacaciones sigan siendo los que se usaron para
// recalcular. Si otra sesión los editó en medio, no se congela un cálculo obsoleto.
func (r *pgRepository) AprobarFiniquito(ctx context.Context, empresaID, id, usuarioID string, motivo, fechaSalida, diasVacaciones string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE finiquito SET estado = 'APROBADO', aprobado_por = $3::uuid, aprobado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'BORRADOR'
			AND motivo = $4 AND fecha_salida = $5::date AND dias_vacaciones = $6::numeric`,
		empresaID, id, usuarioID, motivo, fechaSalida, diasVacaciones)
	if err != nil {
		return 0, fmt.Errorf("nomina: aprobar finiquito: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PagarFiniquito marca PAGADO y, en la MISMA transacción: descuenta los saldos de las
// deducciones aplicadas, desactiva las deducciones del empleado y da de baja la ficha
// con la fecha de salida del finiquito.
func (r *pgRepository) PagarFiniquito(ctx context.Context, empresaID, id, usuarioID string) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("nomina: tx pagar finiquito: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE finiquito SET estado = 'PAGADO', pagado_por = $3::uuid, pagado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'APROBADO'`,
		empresaID, id, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("nomina: pagar finiquito: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, nil
	}
	// Saldos de deducciones aplicadas en el finiquito (desde el detalle jsonb).
	if _, err := tx.Exec(ctx, `
		UPDATE deduccion_empleado de
		SET saldo_restante = GREATEST(0, de.saldo_restante - d.monto)
		FROM (
			SELECT (elem->>'deduccion_id')::uuid AS ded_id, SUM((elem->>'monto')::numeric) AS monto
			FROM finiquito f
			CROSS JOIN LATERAL jsonb_array_elements(f.detalle) elem
			WHERE f.empresa_id = $1::uuid AND f.id = $2::uuid
				AND elem->>'tipo' = 'DEDUCCION' AND COALESCE(elem->>'deduccion_id', '') <> ''
			GROUP BY 1
		) d
		WHERE de.id = d.ded_id AND de.empresa_id = $1::uuid AND de.saldo_restante IS NOT NULL`,
		empresaID, id); err != nil {
		return 0, fmt.Errorf("nomina: descontar saldos del finiquito: %w", err)
	}
	// El vínculo laboral termina: deducciones recurrentes fuera y ficha de baja.
	if _, err := tx.Exec(ctx, `
		UPDATE deduccion_empleado SET activo = false
		WHERE empresa_id = $1::uuid AND empleado_id = (SELECT empleado_id FROM finiquito WHERE id = $2::uuid)`,
		empresaID, id); err != nil {
		return 0, fmt.Errorf("nomina: cerrar deducciones: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE empleado SET activo = false,
			fecha_salida = (SELECT fecha_salida FROM finiquito WHERE id = $2::uuid)
		WHERE empresa_id = $1::uuid AND id = (SELECT empleado_id FROM finiquito WHERE id = $2::uuid)`,
		empresaID, id); err != nil {
		return 0, fmt.Errorf("nomina: baja del empleado: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("nomina: commit pagar finiquito: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AnularFiniquito anula un BORRADOR o APROBADO, salvo que una liquidación APROBADA/PAGADA
// del mes de la salida se haya apoyado en él (omitió al empleado porque el finiquito
// asumía su adelanto): anularlo dejaría ese adelanto sin descontar ni cotizar. Guarda
// atómica en el propio UPDATE.
func (r *pgRepository) AnularFiniquito(ctx context.Context, empresaID, id string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE finiquito AS f SET estado = 'ANULADO'
		WHERE f.empresa_id = $1::uuid AND f.id = $2::uuid AND f.estado IN ('BORRADOR', 'APROBADO')
			AND NOT EXISTS (
				SELECT 1 FROM corrida_nomina c
				WHERE c.empresa_id = f.empresa_id AND c.tipo = 'LIQUIDACION'
					AND c.estado IN ('APROBADA', 'PAGADA')
					AND c.anio = EXTRACT(YEAR FROM f.fecha_salida)
					AND c.mes = EXTRACT(MONTH FROM f.fecha_salida)
					AND NOT EXISTS (SELECT 1 FROM corrida_linea cl
						WHERE cl.corrida_id = c.id AND cl.empleado_id = f.empleado_id))`,
		empresaID, id)
	if err != nil {
		return 0, fmt.Errorf("nomina: anular finiquito: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ---- Insumos del cálculo ----

// SalarioPromedioEmpleado: promedio de la base salarial (base_ccss) de las últimas 6
// liquidaciones PAGADAS — el "salario promedio real" de la maqueta (incluye comisiones y
// bonos). Sin historial, cae al salario base de la ficha.
func (r *pgRepository) SalarioPromedioEmpleado(ctx context.Context, empresaID, empleadoID string) (decimal.Decimal, error) {
	var s string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT AVG(x.base_ccss) FROM (
				SELECT cl.base_ccss FROM corrida_linea cl
				JOIN corrida_nomina c ON c.id = cl.corrida_id
				WHERE c.empresa_id = $1::uuid AND cl.empleado_id = $2::uuid
					AND c.tipo = 'LIQUIDACION' AND c.estado = 'PAGADA'
				ORDER BY c.anio DESC, c.mes DESC LIMIT 6) x),
			(SELECT salario_base FROM empleado WHERE empresa_id = $1::uuid AND id = $2::uuid)
		)::numeric(14,2)::text`,
		empresaID, empleadoID).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) || s == "" {
		return decimal.Zero, ErrEmpleadoNoEncontrado
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("nomina: salario promedio: %w", err)
	}
	return decimal.NewFromString(s)
}

// AdelantoPendienteEmpleado: adelanto APROBADO/PAGADO del mes que ninguna liquidación
// viva del mes descontó (dinero ya entregado al empleado que el finiquito debe recuperar).
// Solo los adelantos verdaderos: el pago del día 15 de un empleado QUINCENAL es salario
// devengado (tratamiento QUINCENA_1), no un anticipo — no se le recupera en el cese.
func (r *pgRepository) AdelantoPendienteEmpleado(ctx context.Context, empresaID, empleadoID string, anio, mes int) (decimal.Decimal, error) {
	var s string
	err := r.pool.QueryRow(ctx, `
		SELECT GREATEST(0,
			COALESCE((SELECT cl.neto FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
				WHERE c.empresa_id = $1::uuid AND cl.empleado_id = $2::uuid AND c.anio = $3 AND c.mes = $4
					AND c.tipo = 'ADELANTO' AND c.estado IN ('APROBADA', 'PAGADA')
					AND cl.tratamiento = 'ADELANTO'), 0)
			- COALESCE((SELECT cl.adelanto FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
				WHERE c.empresa_id = $1::uuid AND cl.empleado_id = $2::uuid AND c.anio = $3 AND c.mes = $4
					AND c.tipo = 'LIQUIDACION' AND c.estado IN ('APROBADA', 'PAGADA')), 0)
		)::numeric(14,2)::text`,
		empresaID, empleadoID, anio, mes).Scan(&s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("nomina: adelanto pendiente: %w", err)
	}
	return decimal.NewFromString(s)
}

// ProvisionesEmpleado: acumulado histórico provisionado (corridas PAGADAS) para el
// comparativo "calculado vs provisionado" de la maqueta.
func (r *pgRepository) ProvisionesEmpleado(ctx context.Context, empresaID, empleadoID string) (decimal.Decimal, error) {
	var s string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cl.prov_aguinaldo + cl.prov_vacaciones + cl.prov_cesantia), 0)::numeric(14,2)::text
		FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
		WHERE c.empresa_id = $1::uuid AND cl.empleado_id = $2::uuid
			AND c.tipo = 'LIQUIDACION' AND c.estado = 'PAGADA'`,
		empresaID, empleadoID).Scan(&s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("nomina: provisiones del empleado: %w", err)
	}
	return decimal.NewFromString(s)
}

// ProvisionEmpleadoAnio es la fila del reporte de provisiones acumuladas del año.
type ProvisionEmpleadoAnio struct {
	EmpleadoID     string `json:"empleado_id"`
	Nombre         string `json:"nombre"`
	Identificacion string `json:"identificacion"`
	Meses          int    `json:"meses"`
	Aguinaldo      string `json:"aguinaldo"`
	Vacaciones     string `json:"vacaciones"`
	Cesantia       string `json:"cesantia"`
	Total          string `json:"total"`
}

// ProvisionesAnio: acumulado por empleado de las liquidaciones PAGADAS del año.
func (r *pgRepository) ProvisionesAnio(ctx context.Context, empresaID string, anio int) ([]ProvisionEmpleadoAnio, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cl.empleado_id::text, cl.nombre, cl.identificacion, COUNT(*),
			SUM(cl.prov_aguinaldo)::text, SUM(cl.prov_vacaciones)::text, SUM(cl.prov_cesantia)::text,
			SUM(cl.prov_aguinaldo + cl.prov_vacaciones + cl.prov_cesantia)::text
		FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
		WHERE c.empresa_id = $1::uuid AND c.anio = $2 AND c.tipo = 'LIQUIDACION' AND c.estado = 'PAGADA'
		GROUP BY cl.empleado_id, cl.nombre, cl.identificacion
		ORDER BY cl.nombre`,
		empresaID, anio)
	if err != nil {
		return nil, fmt.Errorf("nomina: provisiones del año: %w", err)
	}
	defer rows.Close()
	items := make([]ProvisionEmpleadoAnio, 0, 32)
	for rows.Next() {
		var p ProvisionEmpleadoAnio
		if err := rows.Scan(&p.EmpleadoID, &p.Nombre, &p.Identificacion, &p.Meses,
			&p.Aguinaldo, &p.Vacaciones, &p.Cesantia, &p.Total); err != nil {
			return nil, fmt.Errorf("nomina: scan provisión: %w", err)
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// ---- Archivo de pago SINPE ----

// LineaArchivoPago es una fila del archivo de pago (solo empleados con IBAN).
type LineaArchivoPago struct {
	TipoIdentificacion string
	Identificacion     string
	Nombre             string
	IBAN               string
	Neto               string
}

// LineasParaArchivo devuelve las colillas pagables (con IBAN y neto > 0) de la corrida.
func (r *pgRepository) LineasParaArchivo(ctx context.Context, empresaID, corridaID string) ([]LineaArchivoPago, int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(e.tipo_identificacion, 'CEDULA'), cl.identificacion, cl.nombre,
			COALESCE(cl.iban, ''), cl.neto::text
		FROM corrida_linea cl JOIN empleado e ON e.id = cl.empleado_id
		WHERE cl.empresa_id = $1::uuid AND cl.corrida_id = $2::uuid AND cl.neto > 0
		ORDER BY cl.nombre`,
		empresaID, corridaID)
	if err != nil {
		return nil, 0, fmt.Errorf("nomina: líneas para archivo: %w", err)
	}
	defer rows.Close()
	items := make([]LineaArchivoPago, 0, 32)
	sinIBAN := 0
	for rows.Next() {
		var l LineaArchivoPago
		if err := rows.Scan(&l.TipoIdentificacion, &l.Identificacion, &l.Nombre, &l.IBAN, &l.Neto); err != nil {
			return nil, 0, fmt.Errorf("nomina: scan línea archivo: %w", err)
		}
		if l.IBAN == "" {
			sinIBAN++
			continue
		}
		items = append(items, l)
	}
	return items, sinIBAN, rows.Err()
}

// FiniquitoDelMes es un finiquito congelado (APROBADO/PAGADO) cuya fecha de salida cae en
// el mes. Alimenta los dos reportes del cierre: la planilla CCSS (las vacaciones pagadas al
// cese son salario y cotizan) y el archivo SINPE de la liquidación (el neto a depositar).
type FiniquitoDelMes struct {
	ID                 string
	Nombre             string
	TipoIdentificacion string
	Identificacion     string
	IBAN               string
	BaseCCSS           string
	CCSSObrero         string
	Patronal           string
	Total              string
	Estado             string
}

// FiniquitosDelMes devuelve los finiquitos congelados con salida en el mes indicado.
func (r *pgRepository) FiniquitosDelMes(ctx context.Context, empresaID string, anio, mes int) ([]FiniquitoDelMes, error) {
	const q = `
		SELECT f.id::text, e.nombre, COALESCE(e.tipo_identificacion, 'CEDULA'), e.identificacion,
			COALESCE(e.iban, ''), f.base_ccss::text, f.ccss_obrero::text, f.patronal::text,
			f.total::text, f.estado
		FROM finiquito f JOIN empleado e ON e.id = f.empleado_id
		WHERE f.empresa_id = $1::uuid AND f.estado IN ('APROBADO', 'PAGADO')
			AND EXTRACT(YEAR FROM f.fecha_salida) = $2 AND EXTRACT(MONTH FROM f.fecha_salida) = $3
		ORDER BY e.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("nomina: finiquitos del mes: %w", err)
	}
	defer rows.Close()
	items := make([]FiniquitoDelMes, 0, 4)
	for rows.Next() {
		var f FiniquitoDelMes
		if err := rows.Scan(&f.ID, &f.Nombre, &f.TipoIdentificacion, &f.Identificacion, &f.IBAN,
			&f.BaseCCSS, &f.CCSSObrero, &f.Patronal, &f.Total, &f.Estado); err != nil {
			return nil, fmt.Errorf("nomina: scan finiquito del mes: %w", err)
		}
		items = append(items, f)
	}
	return items, rows.Err()
}

// RegistrarArchivoPago asigna el consecutivo del archivo de la corrida. Es IDEMPOTENTE:
// una corrida tiene UN consecutivo (volver a descargar el archivo no quema números ni
// rompe la conciliación por huella); solo se refrescan registros y total. Reintenta una
// vez ante la carrera del UNIQUE (empresa, consecutivo).
func (r *pgRepository) RegistrarArchivoPago(ctx context.Context, empresaID, corridaID string, registros int, total decimal.Decimal, usuarioID string) (int, error) {
	const q = `
		INSERT INTO nomina_archivo_pago (empresa_id, corrida_id, consecutivo, registros, total, creado_por)
		VALUES ($1::uuid, $2::uuid,
			(SELECT COALESCE(MAX(consecutivo), 0) + 1 FROM nomina_archivo_pago WHERE empresa_id = $1::uuid),
			$3, $4, $5::uuid)
		ON CONFLICT (empresa_id, corrida_id) DO UPDATE
			SET registros = EXCLUDED.registros, total = EXCLUDED.total
		RETURNING consecutivo`
	var consecutivo int
	err := r.pool.QueryRow(ctx, q, empresaID, corridaID, registros, total, usuarioID).Scan(&consecutivo)
	if esViolacionUnica(err) {
		err = r.pool.QueryRow(ctx, q, empresaID, corridaID, registros, total, usuarioID).Scan(&consecutivo)
	}
	if err != nil {
		return 0, fmt.Errorf("nomina: registrar archivo de pago: %w", err)
	}
	return consecutivo, nil
}
