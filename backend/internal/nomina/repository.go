package nomina

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Repository abstrae el acceso a datos de RRHH / Nómina.
type Repository interface {
	// Empleados
	ListarEmpleados(ctx context.Context, empresaID string, f FiltrosEmpleado) ([]Empleado, error)
	EmpleadoPorID(ctx context.Context, empresaID, id string) (Empleado, error)
	CrearEmpleado(ctx context.Context, empresaID string, in EmpleadoInput) (Empleado, error)
	ActualizarEmpleado(ctx context.Context, empresaID, id string, in EmpleadoInput) (Empleado, error)
	DesactivarEmpleado(ctx context.Context, empresaID, id, fechaSalida string) error

	// Parámetros por año (versionados)
	ParametrosPorAnio(ctx context.Context, empresaID string, anio int) (Parametros, error)
	GuardarParametros(ctx context.Context, empresaID string, anio int, in ParametrosInput) (Parametros, error)

	// Conceptos de ingreso/deducción
	ListarConceptos(ctx context.Context, empresaID string) ([]ConceptoNomina, error)
	ConceptoPorID(ctx context.Context, empresaID, id string) (ConceptoNomina, error)
	CrearConcepto(ctx context.Context, empresaID string, in ConceptoInput) (ConceptoNomina, error)
	ActualizarConcepto(ctx context.Context, empresaID, id string, in ConceptoInput) (ConceptoNomina, error)
	DesactivarConcepto(ctx context.Context, empresaID, id string) error
	// EnsureConceptos siembra el catálogo base en TODAS las empresas (idempotente).
	EnsureConceptos(ctx context.Context) error

	// Deducciones recurrentes por empleado
	ListarDeducciones(ctx context.Context, empresaID, empleadoID string) ([]DeduccionEmpleado, error)
	CrearDeduccion(ctx context.Context, empresaID, empleadoID string, in DeduccionInput) (DeduccionEmpleado, error)
	ActualizarDeduccion(ctx context.Context, empresaID, empleadoID, id string, in DeduccionInput) (DeduccionEmpleado, error)
	DesactivarDeduccion(ctx context.Context, empresaID, empleadoID, id string) error

	// Corrida quincenal (Etapa 2)
	ListarCorridas(ctx context.Context, empresaID string, anio int) ([]Corrida, error)
	CorridaPorID(ctx context.Context, empresaID, id string) (Corrida, error)
	CrearCorrida(ctx context.Context, empresaID string, anio, mes int, tipo, fechaPago string, parametros []byte, usuarioID string) (Corrida, error)
	LineasCorrida(ctx context.Context, empresaID, corridaID string) ([]LineaCorrida, error)
	// GuardarLineas reemplaza las colillas y totales de una corrida EN BORRADOR (tx todo-o-nada).
	GuardarLineas(ctx context.Context, empresaID, corridaID string, lineas []LineaCorrida, totales TotalesCorrida, parametros []byte) error
	NovedadesCorrida(ctx context.Context, empresaID, corridaID string) ([]NovedadCorrida, error)
	ReemplazarNovedades(ctx context.Context, empresaID, corridaID string, novedades []novedadValidada) error
	NovedadesParaCalc(ctx context.Context, empresaID, corridaID string) (map[string][]IngresoCalc, error)
	DeduccionesParaCalc(ctx context.Context, empresaID string) (map[string][]DeduccionCalc, error)
	AdelantosPagadosDelMes(ctx context.Context, empresaID string, anio, mes int) (map[string]decimal.Decimal, error)
	// RentaRetenidaPrimeraQuincena: lo retenido el día 15 a los empleados quincenales, para
	// que la 2ª quincena cobre solo el ajuste del mes real.
	RentaRetenidaPrimeraQuincena(ctx context.Context, empresaID string, anio, mes int) (map[string]decimal.Decimal, error)
	// LineasPlanillaDelMes agrega las bases y cargas de todas las colillas congeladas del
	// mes por empleado (la planilla CCSS reporta el salario mensual, no media quincena).
	LineasPlanillaDelMes(ctx context.Context, empresaID string, anio, mes int) ([]LineaCorrida, error)
	// EmpleadosParaCorrida: elegibles del período con su fracción de mes laborada.
	EmpleadosParaCorrida(ctx context.Context, empresaID string, anio, mes int, tipo string) ([]EmpleadoCorrida, error)
	ExisteAdelantoBorrador(ctx context.Context, empresaID string, anio, mes int) (bool, error)
	LiquidacionCerradaDelMes(ctx context.Context, empresaID string, anio, mes int) (bool, error)
	TieneNetoNegativo(ctx context.Context, empresaID, corridaID string) (bool, error)
	AdelantosSinColilla(ctx context.Context, empresaID string, anio, mes int, liquidacionID string) (bool, error)
	// AprobarCorrida y AnularCorrida llevan la guarda cruzada ADELANTO↔LIQUIDACIÓN en el
	// propio UPDATE (atómica): 0 filas = guarda rechazada, el servicio desambigua.
	AprobarCorrida(ctx context.Context, empresaID, id, usuarioID string) (int64, error)
	// PagarCorrida marca PAGADA y descuenta los saldos de las deducciones aplicadas (misma tx).
	PagarCorrida(ctx context.Context, empresaID, id, usuarioID string) (int64, error)
	AnularCorrida(ctx context.Context, empresaID, id string) (int64, error)

	// Finiquito (Etapa 3)
	ListarFiniquitos(ctx context.Context, empresaID string) ([]Finiquito, error)
	FiniquitoPorID(ctx context.Context, empresaID, id string) (Finiquito, error)
	// GuardarFiniquito inserta (id vacío) o actualiza EN BORRADOR el snapshot calculado.
	GuardarFiniquito(ctx context.Context, empresaID, id string, in FiniquitoInput, res ResultadoFiniquito, salarioPromedio, diasVacaciones decimal.Decimal, usuarioID string) (Finiquito, error)
	// AprobarFiniquito lleva locking optimista: exige que motivo/fecha/días sigan iguales.
	AprobarFiniquito(ctx context.Context, empresaID, id, usuarioID string, motivo, fechaSalida, diasVacaciones string) (int64, error)
	// PagarFiniquito: PAGADO + descuento de saldos + cierre de deducciones + baja de la ficha (tx).
	PagarFiniquito(ctx context.Context, empresaID, id, usuarioID string) (int64, error)
	AnularFiniquito(ctx context.Context, empresaID, id string) (int64, error)
	SalarioPromedioEmpleado(ctx context.Context, empresaID, empleadoID string) (decimal.Decimal, error)
	AdelantoPendienteEmpleado(ctx context.Context, empresaID, empleadoID string, anio, mes int) (decimal.Decimal, error)
	ProvisionesEmpleado(ctx context.Context, empresaID, empleadoID string) (decimal.Decimal, error)
	ProvisionesAnio(ctx context.Context, empresaID string, anio int) ([]ProvisionEmpleadoAnio, error)

	// Archivo de pago SINPE (bitácora con consecutivo por empresa)
	LineasParaArchivo(ctx context.Context, empresaID, corridaID string) ([]LineaArchivoPago, int, error)
	// FiniquitosDelMes: los congelados con salida en el mes, para sumarlos a la planilla
	// CCSS y al archivo SINPE de la liquidación (el cese también se paga y se reporta).
	FiniquitosDelMes(ctx context.Context, empresaID string, anio, mes int) ([]FiniquitoDelMes, error)

	// Dashboard de RRHH (Etapa 5): agregados del mes, tendencia y hechos para las alertas.
	ResumenNominaMes(ctx context.Context, empresaID string, anio, mes int) (ResumenMes, error)
	TendenciaCostoNomina(ctx context.Context, empresaID, desde, hasta string) ([]CostoMes, error)
	CorridasVivasDelMes(ctx context.Context, empresaID string, anio, mes int) ([]EstadoCorridaMes, error)
	HeadcountPorDepartamento(ctx context.Context, empresaID string) ([]DashboardDepto, error)
	AvisosNominaMes(ctx context.Context, empresaID string, anio, mes int) (AvisosNomina, error)
	RegistrarArchivoPago(ctx context.Context, empresaID, corridaID string, registros int, total decimal.Decimal, usuarioID string) (int, error)

	// Incapacidades y vacaciones (Etapa 4)
	ListarIncapacidades(ctx context.Context, empresaID string, anio, mes int) ([]Incapacidad, error)
	IncapacidadPorID(ctx context.Context, empresaID, id string) (Incapacidad, error)
	CrearIncapacidad(ctx context.Context, empresaID string, in IncapacidadInput, usuarioID string) (Incapacidad, error)
	AnularIncapacidad(ctx context.Context, empresaID, id string) error
	// IncapacidadesParaCalc: las vivas que tocan el mes, por empleado, para la corrida.
	IncapacidadesParaCalc(ctx context.Context, empresaID string, anio, mes int) (map[string][]IncapacidadCalc, error)
	ListarVacaciones(ctx context.Context, empresaID, empleadoID string) ([]Vacacion, error)
	// CrearVacacion valida el saldo dentro del propio INSERT (anti-carrera).
	CrearVacacion(ctx context.Context, empresaID string, in VacacionInput, diasPorMes decimal.Decimal, usuarioID string) (Vacacion, error)
	AnularVacacion(ctx context.Context, empresaID, id string) error
	// Saldos DERIVADOS de vacaciones (acumulado por meses de servicio − disfrutado).
	SaldosVacaciones(ctx context.Context, empresaID string, diasPorMes decimal.Decimal) ([]SaldoVacaciones, error)
	SaldoVacacionesEmpleado(ctx context.Context, empresaID, empleadoID string, diasPorMes decimal.Decimal) (SaldoVacaciones, error)

	// Notificaciones (boleta de pago y aviso de vacaciones)
	CorreosEmpleados(ctx context.Context, empresaID string) (map[string]string, error)
	VacacionParaAviso(ctx context.Context, empresaID, vacacionID string) (VacacionAviso, error)
	// CorridaCerradaDelMes: si tipo viene vacío mira cualquier corrida del mes; con tipo,
	// solo esa (la ausencia se bloquea únicamente si la corrida que usaría esos días ya
	// está congelada).
	CorridaCerradaDelMes(ctx context.Context, empresaID string, anio, mes int, tipo string) (bool, error)
}

// FiltrosEmpleado filtra el listado de empleados.
type FiltrosEmpleado struct {
	Q      string // nombre o identificación (ILIKE)
	Estado string // "activo" | "inactivo" | "" (todos)
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository crea el repository de nómina respaldado por PostgreSQL.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

type scanner interface{ Scan(dest ...any) error }

// ---- Empleados ----

const empleadoCols = `e.id::text, e.nombre, e.tipo_identificacion, e.identificacion,
	COALESCE(e.email, ''), COALESCE(e.telefono, ''), COALESCE(e.iban, ''),
	COALESCE(e.departamento_id::text, ''), COALESCE(d.nombre, ''), COALESCE(e.puesto, ''),
	e.fecha_ingreso::text, COALESCE(e.fecha_salida::text, ''), e.salario_base::text, e.jornada,
	e.hijos, e.conyuge, e.activo,
	(SELECT COUNT(*) FROM deduccion_empleado de WHERE de.empleado_id = e.id AND de.activo)`

const empleadoFrom = ` FROM empleado e LEFT JOIN departamento d ON d.id = e.departamento_id`

func scanEmpleado(row scanner) (Empleado, error) {
	var e Empleado
	err := row.Scan(&e.ID, &e.Nombre, &e.TipoIdentificacion, &e.Identificacion, &e.Email,
		&e.Telefono, &e.IBAN, &e.DepartamentoID, &e.DepartamentoNombre, &e.Puesto,
		&e.FechaIngreso, &e.FechaSalida, &e.SalarioBase, &e.Jornada,
		&e.Hijos, &e.Conyuge, &e.Activo, &e.DeduccionesActivas)
	return e, err
}

func (r *pgRepository) ListarEmpleados(ctx context.Context, empresaID string, f FiltrosEmpleado) ([]Empleado, error) {
	conds := []string{"e.empresa_id = $1::uuid"}
	args := []any{empresaID}
	if f.Q != "" {
		args = append(args, "%"+f.Q+"%")
		conds = append(conds, fmt.Sprintf("(e.nombre ILIKE $%d OR e.identificacion ILIKE $%d)", len(args), len(args)))
	}
	switch f.Estado {
	case "activo":
		conds = append(conds, "e.activo = true")
	case "inactivo":
		conds = append(conds, "e.activo = false")
	}
	q := "SELECT " + empleadoCols + empleadoFrom + " WHERE " + strings.Join(conds, " AND ") + " ORDER BY e.nombre"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar empleados: %w", err)
	}
	defer rows.Close()
	items := make([]Empleado, 0, 32)
	for rows.Next() {
		e, err := scanEmpleado(rows)
		if err != nil {
			return nil, fmt.Errorf("nomina: scan empleado: %w", err)
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

func (r *pgRepository) EmpleadoPorID(ctx context.Context, empresaID, id string) (Empleado, error) {
	q := "SELECT " + empleadoCols + empleadoFrom + " WHERE e.empresa_id = $1::uuid AND e.id = $2::uuid"
	e, err := scanEmpleado(r.pool.QueryRow(ctx, q, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Empleado{}, ErrEmpleadoNoEncontrado
	}
	if err != nil {
		return Empleado{}, fmt.Errorf("nomina: empleado por id: %w", err)
	}
	return e, nil
}

func (r *pgRepository) CrearEmpleado(ctx context.Context, empresaID string, in EmpleadoInput) (Empleado, error) {
	// El departamento se valida contra la empresa (subconsulta): un id ajeno queda NULL.
	const q = `
		WITH ins AS (
			INSERT INTO empleado (empresa_id, nombre, tipo_identificacion, identificacion, email, telefono, iban,
				departamento_id, puesto, fecha_ingreso, salario_base, jornada, hijos, conyuge)
			VALUES ($1::uuid, $2, COALESCE(NULLIF($3, ''), 'CEDULA'), $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
				(SELECT id FROM departamento WHERE id = NULLIF($8, '')::uuid AND empresa_id = $1::uuid),
				NULLIF($9, ''), $10::date, $11::numeric, COALESCE(NULLIF($12, ''), 'MENSUAL'), $13, $14)
			RETURNING *
		)
		SELECT ` + empleadoCols + ` FROM ins e LEFT JOIN departamento d ON d.id = e.departamento_id`
	e, err := scanEmpleado(r.pool.QueryRow(ctx, q, empresaID, in.Nombre, in.TipoIdentificacion, in.Identificacion,
		in.Email, in.Telefono, in.IBAN, in.DepartamentoID, in.Puesto, in.FechaIngreso, in.SalarioBase, in.Jornada,
		in.Hijos, in.Conyuge))
	if esViolacionUnica(err) {
		return Empleado{}, ErrEmpleadoDuplicado
	}
	if err != nil {
		return Empleado{}, fmt.Errorf("nomina: crear empleado: %w", err)
	}
	return e, nil
}

func (r *pgRepository) ActualizarEmpleado(ctx context.Context, empresaID, id string, in EmpleadoInput) (Empleado, error) {
	const q = `
		WITH upd AS (
			UPDATE empleado SET nombre = $3, tipo_identificacion = COALESCE(NULLIF($4, ''), 'CEDULA'),
				identificacion = $5, email = NULLIF($6, ''), telefono = NULLIF($7, ''), iban = NULLIF($8, ''),
				departamento_id = (SELECT dd.id FROM departamento dd WHERE dd.id = NULLIF($9, '')::uuid AND dd.empresa_id = $1::uuid),
				puesto = NULLIF($10, ''), fecha_ingreso = $11::date, salario_base = $12::numeric,
				jornada = COALESCE(NULLIF($13, ''), 'MENSUAL'), hijos = $14, conyuge = $15
			WHERE empresa_id = $1::uuid AND id = $2::uuid
			RETURNING *
		)
		SELECT ` + empleadoCols + ` FROM upd e LEFT JOIN departamento d ON d.id = e.departamento_id`
	e, err := scanEmpleado(r.pool.QueryRow(ctx, q, empresaID, id, in.Nombre, in.TipoIdentificacion, in.Identificacion,
		in.Email, in.Telefono, in.IBAN, in.DepartamentoID, in.Puesto, in.FechaIngreso, in.SalarioBase, in.Jornada,
		in.Hijos, in.Conyuge))
	if errors.Is(err, pgx.ErrNoRows) {
		return Empleado{}, ErrEmpleadoNoEncontrado
	}
	if esViolacionUnica(err) {
		return Empleado{}, ErrEmpleadoDuplicado
	}
	if err != nil {
		return Empleado{}, fmt.Errorf("nomina: actualizar empleado: %w", err)
	}
	return e, nil
}

func (r *pgRepository) DesactivarEmpleado(ctx context.Context, empresaID, id, fechaSalida string) error {
	const q = `UPDATE empleado SET activo = false, fecha_salida = COALESCE(NULLIF($3, '')::date, CURRENT_DATE)
		WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, fechaSalida)
	if err != nil {
		return fmt.Errorf("nomina: desactivar empleado: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEmpleadoNoEncontrado
	}
	return nil
}

// ---- Parámetros ----

func scanParametros(row scanner) (Parametros, error) {
	var p Parametros
	var cargas, renta []byte
	err := row.Scan(&p.ID, &p.Anio, &cargas, &renta, &p.INSRiesgosPct, &p.AplicaINA,
		&p.AdelantoPct, &p.AdelantoBase, &p.Redondeo, &p.ProvisionBase,
		&p.AguinaldoPct, &p.VacacionesPct, &p.CesantiaPct, &p.VacacionesDiasMes,
		&p.HorasJornadaMes, &p.FactorHoraExtra)
	if err != nil {
		return Parametros{}, err
	}
	if err := json.Unmarshal(cargas, &p.Cargas); err != nil {
		return Parametros{}, fmt.Errorf("nomina: cargas corruptas: %w", err)
	}
	if err := json.Unmarshal(renta, &p.Renta); err != nil {
		return Parametros{}, fmt.Errorf("nomina: tramos corruptos: %w", err)
	}
	p.Origen = "EMPRESA"
	return p, nil
}

const parametrosCols = `id::text, anio, cargas, tramos_renta, ins_riesgos_pct::text, aplica_ina,
	adelanto_pct::text, adelanto_base, redondeo, provision_base,
	aguinaldo_pct::text, vacaciones_pct::text, cesantia_pct::text, vacaciones_dias_mes::text,
	horas_jornada_mes::text, factor_hora_extra::text`

func (r *pgRepository) ParametrosPorAnio(ctx context.Context, empresaID string, anio int) (Parametros, error) {
	q := "SELECT " + parametrosCols + " FROM nomina_parametros WHERE empresa_id = $1::uuid AND anio = $2"
	p, err := scanParametros(r.pool.QueryRow(ctx, q, empresaID, anio))
	if errors.Is(err, pgx.ErrNoRows) {
		return Parametros{}, ErrParametrosNoEncontrados
	}
	if err != nil {
		return Parametros{}, fmt.Errorf("nomina: parámetros por año: %w", err)
	}
	return p, nil
}

func (r *pgRepository) GuardarParametros(ctx context.Context, empresaID string, anio int, in ParametrosInput) (Parametros, error) {
	cargas, err := json.Marshal(in.Cargas)
	if err != nil {
		return Parametros{}, fmt.Errorf("nomina: serializar cargas: %w", err)
	}
	renta, err := json.Marshal(in.Renta)
	if err != nil {
		return Parametros{}, fmt.Errorf("nomina: serializar tramos: %w", err)
	}
	q := `
		INSERT INTO nomina_parametros (empresa_id, anio, cargas, tramos_renta, ins_riesgos_pct, aplica_ina,
			adelanto_pct, adelanto_base, redondeo, provision_base, aguinaldo_pct, vacaciones_pct, cesantia_pct,
			vacaciones_dias_mes, horas_jornada_mes, factor_hora_extra)
		VALUES ($1::uuid, $2, $3::jsonb, $4::jsonb, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			COALESCE(NULLIF($15, '')::numeric, 240), COALESCE(NULLIF($16, '')::numeric, 1.5))
		ON CONFLICT (empresa_id, anio) DO UPDATE SET cargas = EXCLUDED.cargas, tramos_renta = EXCLUDED.tramos_renta,
			ins_riesgos_pct = EXCLUDED.ins_riesgos_pct, aplica_ina = EXCLUDED.aplica_ina,
			adelanto_pct = EXCLUDED.adelanto_pct, adelanto_base = EXCLUDED.adelanto_base,
			redondeo = EXCLUDED.redondeo, provision_base = EXCLUDED.provision_base,
			aguinaldo_pct = EXCLUDED.aguinaldo_pct, vacaciones_pct = EXCLUDED.vacaciones_pct,
			cesantia_pct = EXCLUDED.cesantia_pct, vacaciones_dias_mes = EXCLUDED.vacaciones_dias_mes,
			-- Si el request no trae estos campos, se CONSERVA lo que la empresa tenía: guardar
			-- los otros parámetros no puede reiniciar en silencio el valor de la hora extra.
			horas_jornada_mes = COALESCE(NULLIF($15, '')::numeric, nomina_parametros.horas_jornada_mes),
			factor_hora_extra = COALESCE(NULLIF($16, '')::numeric, nomina_parametros.factor_hora_extra)
		RETURNING ` + parametrosCols
	p, err := scanParametros(r.pool.QueryRow(ctx, q, empresaID, anio, cargas, renta, in.INSRiesgosPct,
		in.AplicaINA, in.AdelantoPct, in.AdelantoBase, in.Redondeo, in.ProvisionBase,
		in.AguinaldoPct, in.VacacionesPct, in.CesantiaPct, in.VacacionesDiasMes,
		in.HorasJornadaMes, in.FactorHoraExtra))
	if err != nil {
		return Parametros{}, fmt.Errorf("nomina: guardar parámetros: %w", err)
	}
	return p, nil
}

// ---- Conceptos ----

const conceptoCols = `id::text, nombre, tipo, afecta_ccss, afecta_renta, afecta_aguinaldo,
	COALESCE(base_legal, ''), de_sistema, activo, por_horas`

func scanConcepto(row scanner) (ConceptoNomina, error) {
	var c ConceptoNomina
	err := row.Scan(&c.ID, &c.Nombre, &c.Tipo, &c.AfectaCCSS, &c.AfectaRenta, &c.AfectaAguinaldo,
		&c.BaseLegal, &c.DeSistema, &c.Activo, &c.PorHoras)
	return c, err
}

func (r *pgRepository) ListarConceptos(ctx context.Context, empresaID string) ([]ConceptoNomina, error) {
	q := "SELECT " + conceptoCols + ` FROM concepto_nomina WHERE empresa_id = $1::uuid
		ORDER BY de_sistema DESC, tipo, nombre`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar conceptos: %w", err)
	}
	defer rows.Close()
	items := make([]ConceptoNomina, 0, 16)
	for rows.Next() {
		c, err := scanConcepto(rows)
		if err != nil {
			return nil, fmt.Errorf("nomina: scan concepto: %w", err)
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (r *pgRepository) ConceptoPorID(ctx context.Context, empresaID, id string) (ConceptoNomina, error) {
	q := "SELECT " + conceptoCols + " FROM concepto_nomina WHERE empresa_id = $1::uuid AND id = $2::uuid"
	c, err := scanConcepto(r.pool.QueryRow(ctx, q, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ConceptoNomina{}, ErrConceptoNoEncontrado
	}
	if err != nil {
		return ConceptoNomina{}, fmt.Errorf("nomina: concepto por id: %w", err)
	}
	return c, nil
}

func (r *pgRepository) CrearConcepto(ctx context.Context, empresaID string, in ConceptoInput) (ConceptoNomina, error) {
	q := `INSERT INTO concepto_nomina (empresa_id, nombre, tipo, afecta_ccss, afecta_renta, afecta_aguinaldo, base_legal)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, NULLIF($7, ''))
		RETURNING ` + conceptoCols
	c, err := scanConcepto(r.pool.QueryRow(ctx, q, empresaID, in.Nombre, in.Tipo,
		in.AfectaCCSS, in.AfectaRenta, in.AfectaAguinaldo, in.BaseLegal))
	if esViolacionUnica(err) {
		return ConceptoNomina{}, ErrConceptoDuplicado
	}
	if err != nil {
		return ConceptoNomina{}, fmt.Errorf("nomina: crear concepto: %w", err)
	}
	return c, nil
}

func (r *pgRepository) ActualizarConcepto(ctx context.Context, empresaID, id string, in ConceptoInput) (ConceptoNomina, error) {
	// El WHERE re-verifica de_sistema=false: el guardarraíl no depende solo del servicio.
	q := `UPDATE concepto_nomina SET nombre = $3, tipo = $4, afecta_ccss = $5, afecta_renta = $6,
			afecta_aguinaldo = $7, base_legal = NULLIF($8, '')
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND NOT de_sistema
		RETURNING ` + conceptoCols
	c, err := scanConcepto(r.pool.QueryRow(ctx, q, empresaID, id, in.Nombre, in.Tipo,
		in.AfectaCCSS, in.AfectaRenta, in.AfectaAguinaldo, in.BaseLegal))
	if errors.Is(err, pgx.ErrNoRows) {
		return ConceptoNomina{}, ErrConceptoNoEncontrado
	}
	if esViolacionUnica(err) {
		return ConceptoNomina{}, ErrConceptoDuplicado
	}
	if err != nil {
		return ConceptoNomina{}, fmt.Errorf("nomina: actualizar concepto: %w", err)
	}
	return c, nil
}

func (r *pgRepository) DesactivarConcepto(ctx context.Context, empresaID, id string) error {
	const q = `UPDATE concepto_nomina SET activo = false
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND NOT de_sistema`
	tag, err := r.pool.Exec(ctx, q, empresaID, id)
	if err != nil {
		return fmt.Errorf("nomina: desactivar concepto: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConceptoNoEncontrado
	}
	return nil
}

func (r *pgRepository) EnsureConceptos(ctx context.Context) error {
	const q = `
		INSERT INTO concepto_nomina (empresa_id, nombre, tipo, afecta_ccss, afecta_renta, afecta_aguinaldo, base_legal, de_sistema)
		SELECT e.id, c.nombre, c.tipo, c.ccss, c.renta, c.aguinaldo, NULLIF(c.base_legal, ''), c.de_sistema
		FROM empresa e
		CROSS JOIN (SELECT * FROM unnest($1::text[], $2::text[], $3::bool[], $4::bool[], $5::bool[], $6::text[], $7::bool[])
			AS t(nombre, tipo, ccss, renta, aguinaldo, base_legal, de_sistema)) c
		ON CONFLICT (empresa_id, nombre) DO NOTHING`
	n := len(ConceptosBase)
	nombres, tipos, bases := make([]string, n), make([]string, n), make([]string, n)
	ccss, renta, agui, sist := make([]bool, n), make([]bool, n), make([]bool, n), make([]bool, n)
	for i, c := range ConceptosBase {
		nombres[i], tipos[i], bases[i] = c.Nombre, c.Tipo, c.BaseLegal
		ccss[i], renta[i], agui[i], sist[i] = c.AfectaCCSS, c.AfectaRenta, c.AfectaAguinaldo, c.DeSistema
	}
	if _, err := r.pool.Exec(ctx, q, nombres, tipos, ccss, renta, agui, bases, sist); err != nil {
		return fmt.Errorf("nomina: sembrar conceptos base: %w", err)
	}
	return nil
}

// ---- Deducciones recurrentes ----

const deduccionCols = `de.id::text, de.empleado_id::text, de.concepto_id::text, c.nombre, de.etiqueta,
	de.cuota::text, COALESCE(de.saldo_total::text, ''), COALESCE(de.saldo_restante::text, ''), de.prioridad,
	de.frecuencia, de.activo`

func scanDeduccion(row scanner) (DeduccionEmpleado, error) {
	var d DeduccionEmpleado
	err := row.Scan(&d.ID, &d.EmpleadoID, &d.ConceptoID, &d.ConceptoNombre, &d.Etiqueta,
		&d.Cuota, &d.SaldoTotal, &d.SaldoRestante, &d.Prioridad, &d.Frecuencia, &d.Activo)
	return d, err
}

func (r *pgRepository) ListarDeducciones(ctx context.Context, empresaID, empleadoID string) ([]DeduccionEmpleado, error) {
	q := "SELECT " + deduccionCols + ` FROM deduccion_empleado de
		JOIN concepto_nomina c ON c.id = de.concepto_id
		WHERE de.empresa_id = $1::uuid AND de.empleado_id = $2::uuid
		ORDER BY de.activo DESC, de.prioridad, de.etiqueta`
	rows, err := r.pool.Query(ctx, q, empresaID, empleadoID)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar deducciones: %w", err)
	}
	defer rows.Close()
	items := make([]DeduccionEmpleado, 0, 8)
	for rows.Next() {
		d, err := scanDeduccion(rows)
		if err != nil {
			return nil, fmt.Errorf("nomina: scan deducción: %w", err)
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

func (r *pgRepository) CrearDeduccion(ctx context.Context, empresaID, empleadoID string, in DeduccionInput) (DeduccionEmpleado, error) {
	// saldo_restante nace igual al saldo_total (o NULL si es recurrente sin tope).
	q := `
		WITH ins AS (
			INSERT INTO deduccion_empleado (empresa_id, empleado_id, concepto_id, etiqueta, cuota, saldo_total, saldo_restante, prioridad, frecuencia)
			SELECT $1::uuid, e.id, $3::uuid, $4, $5::numeric, $6::numeric, $6::numeric, $7, COALESCE(NULLIF($8, ''), 'MENSUAL')
			FROM empleado e WHERE e.id = $2::uuid AND e.empresa_id = $1::uuid
			RETURNING *
		)
		SELECT ` + strings.ReplaceAll(deduccionCols, "de.", "ins.") + ` FROM ins JOIN concepto_nomina c ON c.id = ins.concepto_id`
	d, err := scanDeduccion(r.pool.QueryRow(ctx, q, empresaID, empleadoID, in.ConceptoID, in.Etiqueta,
		in.Cuota, in.SaldoTotal, in.Prioridad, in.Frecuencia))
	if errors.Is(err, pgx.ErrNoRows) {
		return DeduccionEmpleado{}, ErrEmpleadoNoEncontrado
	}
	if err != nil {
		return DeduccionEmpleado{}, fmt.Errorf("nomina: crear deducción: %w", err)
	}
	return d, nil
}

func (r *pgRepository) ActualizarDeduccion(ctx context.Context, empresaID, empleadoID, id string, in DeduccionInput) (DeduccionEmpleado, error) {
	// Si cambia el saldo_total se reinicia el restante; si no cambia, se conserva el avance.
	q := `
		WITH upd AS (
			UPDATE deduccion_empleado SET etiqueta = $4, cuota = $5::numeric, prioridad = $6,
				saldo_restante = CASE WHEN saldo_total IS DISTINCT FROM $7::numeric THEN $7::numeric ELSE saldo_restante END,
				saldo_total = $7::numeric, frecuencia = COALESCE(NULLIF($8, ''), frecuencia)
			WHERE empresa_id = $1::uuid AND empleado_id = $2::uuid AND id = $3::uuid
			RETURNING *
		)
		SELECT ` + strings.ReplaceAll(deduccionCols, "de.", "upd.") + ` FROM upd JOIN concepto_nomina c ON c.id = upd.concepto_id`
	d, err := scanDeduccion(r.pool.QueryRow(ctx, q, empresaID, empleadoID, id, in.Etiqueta, in.Cuota, in.Prioridad, in.SaldoTotal, in.Frecuencia))
	if errors.Is(err, pgx.ErrNoRows) {
		return DeduccionEmpleado{}, ErrDeduccionNoEncontrada
	}
	if err != nil {
		return DeduccionEmpleado{}, fmt.Errorf("nomina: actualizar deducción: %w", err)
	}
	return d, nil
}

func (r *pgRepository) DesactivarDeduccion(ctx context.Context, empresaID, empleadoID, id string) error {
	const q = `UPDATE deduccion_empleado SET activo = false
		WHERE empresa_id = $1::uuid AND empleado_id = $2::uuid AND id = $3::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, empleadoID, id)
	if err != nil {
		return fmt.Errorf("nomina: desactivar deducción: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeduccionNoEncontrada
	}
	return nil
}

func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// esNoRows distingue "no encontrado" de un error real de base de datos.
func esNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
