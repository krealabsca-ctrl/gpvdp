package nomina

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Métodos de incapacidades y vacaciones. El SALDO de vacaciones se DERIVA por SQL
// (acumulado por meses de servicio − disfrutado); no hay tabla de saldos que se
// desincronice, igual que los estados de caja chica.

const incapacidadCols = `i.id::text, i.empleado_id::text, e.nombre, i.entidad,
	i.fecha_inicio::text, (i.fecha_inicio + (i.dias - 1))::text, i.dias,
	COALESCE(i.boleta, ''), COALESCE(i.observaciones, ''), i.anulada, i.creado_en::text`

func (r *pgRepository) ListarIncapacidades(ctx context.Context, empresaID string, anio, mes int) ([]Incapacidad, error) {
	// Trae las incapacidades que TOCAN el mes (pueden haber empezado antes y cruzar).
	q := `
		SELECT ` + incapacidadCols + `
		FROM incapacidad i JOIN empleado e ON e.id = i.empleado_id
		WHERE i.empresa_id = $1::uuid
			AND i.fecha_inicio <= (make_date($2, $3, 1) + INTERVAL '1 month - 1 day')::date
			AND (i.fecha_inicio + (i.dias - 1)) >= make_date($2, $3, 1)
		ORDER BY i.fecha_inicio DESC, e.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar incapacidades: %w", err)
	}
	defer rows.Close()
	items := make([]Incapacidad, 0, 16)
	for rows.Next() {
		var i Incapacidad
		if err := rows.Scan(&i.ID, &i.EmpleadoID, &i.EmpleadoNombre, &i.Entidad,
			&i.FechaInicio, &i.FechaFin, &i.Dias, &i.Boleta, &i.Observaciones,
			&i.Anulada, &i.CreadoEn); err != nil {
			return nil, fmt.Errorf("nomina: scan incapacidad: %w", err)
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

func (r *pgRepository) CrearIncapacidad(ctx context.Context, empresaID string, in IncapacidadInput, usuarioID string) (Incapacidad, error) {
	q := `
		WITH ins AS (
			INSERT INTO incapacidad (empresa_id, empleado_id, entidad, fecha_inicio, dias, boleta, observaciones, creado_por)
			SELECT $1::uuid, e.id, $3, $4::date, $5, NULLIF($6, ''), NULLIF($7, ''), $8::uuid
			FROM empleado e WHERE e.id = $2::uuid AND e.empresa_id = $1::uuid
			RETURNING *
		)
		SELECT ` + incapacidadCols + ` FROM ins i JOIN empleado e ON e.id = i.empleado_id`
	var i Incapacidad
	err := r.pool.QueryRow(ctx, q, empresaID, in.EmpleadoID, in.Entidad, in.FechaInicio,
		in.Dias, in.Boleta, in.Observaciones, usuarioID).
		Scan(&i.ID, &i.EmpleadoID, &i.EmpleadoNombre, &i.Entidad, &i.FechaInicio, &i.FechaFin,
			&i.Dias, &i.Boleta, &i.Observaciones, &i.Anulada, &i.CreadoEn)
	if err != nil {
		if esNoRows(err) {
			return Incapacidad{}, ErrEmpleadoNoEncontrado
		}
		return Incapacidad{}, fmt.Errorf("nomina: crear incapacidad: %w", err)
	}
	return i, nil
}

func (r *pgRepository) AnularIncapacidad(ctx context.Context, empresaID, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE incapacidad SET anulada = true WHERE empresa_id = $1::uuid AND id = $2::uuid AND NOT anulada`,
		empresaID, id)
	if err != nil {
		return fmt.Errorf("nomina: anular incapacidad: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrIncapacidadNoEncontrada
	}
	return nil
}

// IncapacidadPorID devuelve el período de una incapacidad viva (para validar la corrida).
func (r *pgRepository) IncapacidadPorID(ctx context.Context, empresaID, id string) (Incapacidad, error) {
	q := `SELECT ` + incapacidadCols + `
		FROM incapacidad i JOIN empleado e ON e.id = i.empleado_id
		WHERE i.empresa_id = $1::uuid AND i.id = $2::uuid`
	var i Incapacidad
	err := r.pool.QueryRow(ctx, q, empresaID, id).
		Scan(&i.ID, &i.EmpleadoID, &i.EmpleadoNombre, &i.Entidad, &i.FechaInicio, &i.FechaFin,
			&i.Dias, &i.Boleta, &i.Observaciones, &i.Anulada, &i.CreadoEn)
	if err != nil {
		if esNoRows(err) {
			return Incapacidad{}, ErrIncapacidadNoEncontrada
		}
		return Incapacidad{}, fmt.Errorf("nomina: incapacidad por id: %w", err)
	}
	return i, nil
}

// IncapacidadesParaCalc devuelve, por empleado, las incapacidades vivas que tocan el mes.
func (r *pgRepository) IncapacidadesParaCalc(ctx context.Context, empresaID string, anio, mes int) (map[string][]IncapacidadCalc, error) {
	q := `
		SELECT i.empleado_id::text, i.id::text, i.entidad, i.fecha_inicio::text, i.dias
		FROM incapacidad i
		WHERE i.empresa_id = $1::uuid AND NOT i.anulada
			AND i.fecha_inicio <= (make_date($2, $3, 1) + INTERVAL '1 month - 1 day')::date
			AND (i.fecha_inicio + (i.dias - 1)) >= make_date($2, $3, 1)
		ORDER BY i.fecha_inicio`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("nomina: incapacidades para cálculo: %w", err)
	}
	defer rows.Close()
	out := map[string][]IncapacidadCalc{}
	for rows.Next() {
		var empleadoID, fecha string
		var c IncapacidadCalc
		if err := rows.Scan(&empleadoID, &c.ID, &c.Entidad, &fecha, &c.Dias); err != nil {
			return nil, fmt.Errorf("nomina: scan incapacidad calc: %w", err)
		}
		if c.FechaInicio, err = time.Parse("2006-01-02", fecha); err != nil {
			return nil, fmt.Errorf("nomina: fecha de incapacidad corrupta: %w", err)
		}
		out[empleadoID] = append(out[empleadoID], c)
	}
	return out, rows.Err()
}

// ---- Vacaciones ----

const vacacionCols = `v.id::text, v.empleado_id::text, e.nombre, v.fecha_inicio::text,
	v.dias::text, COALESCE(v.observaciones, ''), v.anulada, v.creado_en::text`

func (r *pgRepository) ListarVacaciones(ctx context.Context, empresaID, empleadoID string) ([]Vacacion, error) {
	q := `SELECT ` + vacacionCols + `
		FROM vacacion v JOIN empleado e ON e.id = v.empleado_id
		WHERE v.empresa_id = $1::uuid AND ($2 = '' OR v.empleado_id = $2::uuid)
		ORDER BY v.fecha_inicio DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, empleadoID)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar vacaciones: %w", err)
	}
	defer rows.Close()
	items := make([]Vacacion, 0, 16)
	for rows.Next() {
		var v Vacacion
		if err := rows.Scan(&v.ID, &v.EmpleadoID, &v.EmpleadoNombre, &v.FechaInicio,
			&v.Dias, &v.Observaciones, &v.Anulada, &v.CreadoEn); err != nil {
			return nil, fmt.Errorf("nomina: scan vacación: %w", err)
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

// CrearVacacion inserta el disfrute verificando el SALDO EN EL PROPIO INSERT: el WHERE
// recalcula acumulado − disfrutado, así que dos peticiones simultáneas no pueden pasarse
// del saldo (la segunda no inserta ninguna fila).
func (r *pgRepository) CrearVacacion(ctx context.Context, empresaID string, in VacacionInput, diasPorMes decimal.Decimal, usuarioID string) (Vacacion, error) {
	q := `
		WITH ins AS (
			INSERT INTO vacacion (empresa_id, empleado_id, fecha_inicio, dias, observaciones, creado_por)
			SELECT $1::uuid, e.id, $3::date, $4::numeric, NULLIF($5, ''), $6::uuid
			FROM empleado e
			WHERE e.id = $2::uuid AND e.empresa_id = $1::uuid
				AND $4::numeric <= (
					GREATEST(0, (DATE_PART('year', AGE(COALESCE(e.fecha_salida, CURRENT_DATE), e.fecha_ingreso)) * 12
						+ DATE_PART('month', AGE(COALESCE(e.fecha_salida, CURRENT_DATE), e.fecha_ingreso)))::int) * $7::numeric
					- COALESCE((SELECT SUM(v2.dias) FROM vacacion v2
						WHERE v2.empleado_id = e.id AND NOT v2.anulada), 0))
			RETURNING *
		)
		SELECT ` + vacacionCols + ` FROM ins v JOIN empleado e ON e.id = v.empleado_id`
	var v Vacacion
	err := r.pool.QueryRow(ctx, q, empresaID, in.EmpleadoID, in.FechaInicio, in.Dias,
		in.Observaciones, usuarioID, diasPorMes).
		Scan(&v.ID, &v.EmpleadoID, &v.EmpleadoNombre, &v.FechaInicio, &v.Dias,
			&v.Observaciones, &v.Anulada, &v.CreadoEn)
	if err != nil {
		if esNoRows(err) {
			// O el empleado no existe, o no le alcanza el saldo: el servicio ya validó la
			// existencia, así que a esta altura es saldo insuficiente (o una carrera).
			return Vacacion{}, ErrSinSaldoVacaciones
		}
		return Vacacion{}, fmt.Errorf("nomina: crear vacación: %w", err)
	}
	return v, nil
}

func (r *pgRepository) AnularVacacion(ctx context.Context, empresaID, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE vacacion SET anulada = true WHERE empresa_id = $1::uuid AND id = $2::uuid AND NOT anulada`,
		empresaID, id)
	if err != nil {
		return fmt.Errorf("nomina: anular vacación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVacacionNoEncontrada
	}
	return nil
}

// saldoVacacionesSQL calcula el saldo DERIVADO: los meses completos de servicio (hasta la
// salida, si ya salió) por los días que acumula cada mes, menos lo disfrutado.
const saldoVacacionesSQL = `
	SELECT e.id::text, e.nombre, e.identificacion, e.fecha_ingreso::text,
		GREATEST(0, (DATE_PART('year', AGE(COALESCE(e.fecha_salida, CURRENT_DATE), e.fecha_ingreso)) * 12
			+ DATE_PART('month', AGE(COALESCE(e.fecha_salida, CURRENT_DATE), e.fecha_ingreso)))::int) AS meses,
		(GREATEST(0, (DATE_PART('year', AGE(COALESCE(e.fecha_salida, CURRENT_DATE), e.fecha_ingreso)) * 12
			+ DATE_PART('month', AGE(COALESCE(e.fecha_salida, CURRENT_DATE), e.fecha_ingreso)))::int) * $2::numeric)::numeric(8,2)::text AS acumulado,
		COALESCE((SELECT SUM(v.dias) FROM vacacion v
			WHERE v.empleado_id = e.id AND NOT v.anulada), 0)::numeric(8,2)::text AS disfrutado,
		GREATEST(0, (GREATEST(0, (DATE_PART('year', AGE(COALESCE(e.fecha_salida, CURRENT_DATE), e.fecha_ingreso)) * 12
			+ DATE_PART('month', AGE(COALESCE(e.fecha_salida, CURRENT_DATE), e.fecha_ingreso)))::int) * $2::numeric)
			- COALESCE((SELECT SUM(v.dias) FROM vacacion v
				WHERE v.empleado_id = e.id AND NOT v.anulada), 0))::numeric(8,2)::text AS pendiente
	FROM empleado e
	WHERE e.empresa_id = $1::uuid`

// SaldosVacaciones devuelve el saldo de todos los empleados activos (más los que tengan
// días pendientes aunque ya estén de baja, para no perderlos de vista en el finiquito).
func (r *pgRepository) SaldosVacaciones(ctx context.Context, empresaID string, diasPorMes decimal.Decimal) ([]SaldoVacaciones, error) {
	q := saldoVacacionesSQL + ` AND (e.activo OR EXISTS (
			SELECT 1 FROM finiquito f WHERE f.empleado_id = e.id AND f.estado = 'BORRADOR'))
		ORDER BY e.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, diasPorMes)
	if err != nil {
		return nil, fmt.Errorf("nomina: saldos de vacaciones: %w", err)
	}
	defer rows.Close()
	items := make([]SaldoVacaciones, 0, 32)
	for rows.Next() {
		var s SaldoVacaciones
		if err := rows.Scan(&s.EmpleadoID, &s.Nombre, &s.Identificacion, &s.FechaIngreso,
			&s.MesesServicio, &s.Acumulado, &s.Disfrutado, &s.Pendiente); err != nil {
			return nil, fmt.Errorf("nomina: scan saldo: %w", err)
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

// SaldoVacacionesEmpleado devuelve el saldo de un empleado (para precargar el finiquito).
func (r *pgRepository) SaldoVacacionesEmpleado(ctx context.Context, empresaID, empleadoID string, diasPorMes decimal.Decimal) (SaldoVacaciones, error) {
	q := saldoVacacionesSQL + ` AND e.id = $3::uuid`
	var s SaldoVacaciones
	err := r.pool.QueryRow(ctx, q, empresaID, diasPorMes, empleadoID).
		Scan(&s.EmpleadoID, &s.Nombre, &s.Identificacion, &s.FechaIngreso,
			&s.MesesServicio, &s.Acumulado, &s.Disfrutado, &s.Pendiente)
	if err != nil {
		if esNoRows(err) {
			return SaldoVacaciones{}, ErrEmpleadoNoEncontrado
		}
		return SaldoVacaciones{}, fmt.Errorf("nomina: saldo del empleado: %w", err)
	}
	return s, nil
}

// CorridaCerradaDelMes indica si el mes ya tiene una corrida APROBADA o PAGADA de un tipo
// concreto. La ausencia solo se bloquea si está cerrada LA corrida que usaría esos días
// (ver CorridaQueCubre): tras pagar el adelanto del día 15 la empresa igual necesita poder
// registrar la boleta que llega el 16, o pagaría días que la CCSS subsidia.
func (r *pgRepository) CorridaCerradaDelMes(ctx context.Context, empresaID string, anio, mes int, tipo string) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM corrida_nomina
			WHERE empresa_id = $1::uuid AND anio = $2 AND mes = $3
				AND ($4 = '' OR tipo = $4) AND estado IN ('APROBADA', 'PAGADA'))`,
		empresaID, anio, mes, tipo).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("nomina: corrida cerrada del mes: %w", err)
	}
	return existe, nil
}

// EmpleadoParaAusencia devuelve la jornada y el período laborado del empleado, para
// intersecar la ausencia con los días que realmente trabajó y saber qué corrida la usaría.
func (r *pgRepository) EmpleadoParaAusencia(ctx context.Context, empresaID, empleadoID string) (Empleado, error) {
	return r.EmpleadoPorID(ctx, empresaID, empleadoID)
}
