package nomina

// El autoservicio de vacaciones: lo que ve el colaborador y lo que ve la jefatura.
//
// TODO lo de este archivo existe para una sola cosa: que alguien pueda pedir o aprobar días libres
// sin que se le abra la ficha de nadie. El alcance NO viene en la petición —lo resuelve el
// servidor a partir del usuario autenticado— y un alcance vacío CIERRA: sin empleado ligado no hay
// «mis vacaciones», y sin jefatura cargada no hay equipo que mirar.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// EmpleadoDeUsuario dice qué empleado ES este usuario en esta empresa.
//
// Devuelve ErrSinEmpleadoLigado cuando no hay ninguno, y eso NO es un error del sistema: es una
// persona que entra al ERP y no está en planilla, o una ficha a la que todavía no le asignaron su
// usuario. La diferencia con un fallo importa, porque la pantalla tiene que decirle qué pedirle a
// RRHH en vez de mostrar un error técnico.
func (r *pgRepository) EmpleadoDeUsuario(ctx context.Context, empresaID, usuarioID string) (string, error) {
	const q = `SELECT id::text FROM empleado
	           WHERE empresa_id = $1::uuid AND usuario_id = $2::uuid AND activo`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, usuarioID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSinEmpleadoLigado
	}
	if err != nil {
		return "", fmt.Errorf("nomina: empleado del usuario: %w", err)
	}
	return id, nil
}

// DepartamentosDeJefe devuelve los departamentos que este usuario tiene a cargo.
//
// Una lista VACÍA es un alcance vacío y cierra todo: quien tiene el permiso pero no está cargado
// como jefatura de ningún departamento no ve a nadie. Es lo contrario de lo que haría un filtro
// mal escrito —«sin departamentos, no filtro»— que le mostraría la empresa entera.
func (r *pgRepository) DepartamentosDeJefe(ctx context.Context, empresaID, usuarioID string) ([]string, error) {
	const q = `SELECT departamento_id::text FROM rrhh_jefatura
	           WHERE empresa_id = $1::uuid AND usuario_id = $2::uuid`
	rows, err := r.pool.Query(ctx, q, empresaID, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("nomina: departamentos a cargo: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, 4)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("nomina: departamentos a cargo: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SaldoDeEmpleado es el saldo de UNA persona, con la misma fórmula que usa la pantalla de RRHH.
//
// Reusa `saldoVacacionesSQL` y no una copia: el saldo de vacaciones es un derecho laboral y dos
// fórmulas que puedan separarse acabarían dándole al colaborador un número distinto del que ve su
// jefatura.
func (r *pgRepository) SaldoDeEmpleado(ctx context.Context, empresaID, empleadoID string, diasPorMes decimal.Decimal) (SaldoVacaciones, error) {
	q := saldoVacacionesSQL + ` AND e.id = $3::uuid`
	var s SaldoVacaciones
	err := r.pool.QueryRow(ctx, q, empresaID, diasPorMes, empleadoID).
		Scan(&s.EmpleadoID, &s.Nombre, &s.Identificacion, &s.FechaIngreso,
			&s.MesesServicio, &s.Acumulado, &s.Disfrutado, &s.Pendiente, &s.Reservado)
	if errors.Is(err, pgx.ErrNoRows) {
		return SaldoVacaciones{}, ErrEmpleadoNoEncontrado
	}
	if err != nil {
		return SaldoVacaciones{}, fmt.Errorf("nomina: saldo del empleado: %w", err)
	}
	return s, nil
}

// SaldosDeDepartamentos es el saldo del equipo de una jefatura.
//
// Con la lista de departamentos vacía devuelve vacío SIN consultar: el `= ANY('{}')` de Postgres ya
// daría cero filas, pero dejarlo explícito evita que un cambio futuro lo convierta por descuido en
// «sin filtro».
func (r *pgRepository) SaldosDeDepartamentos(ctx context.Context, empresaID string, deptos []string, diasPorMes decimal.Decimal) ([]SaldoVacaciones, error) {
	if len(deptos) == 0 {
		return []SaldoVacaciones{}, nil
	}
	q := saldoVacacionesSQL + ` AND e.activo AND e.departamento_id = ANY($3::uuid[]) ORDER BY e.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, diasPorMes, deptos)
	if err != nil {
		return nil, fmt.Errorf("nomina: saldos del equipo: %w", err)
	}
	defer rows.Close()
	out := make([]SaldoVacaciones, 0, 16)
	for rows.Next() {
		var s SaldoVacaciones
		if err := rows.Scan(&s.EmpleadoID, &s.Nombre, &s.Identificacion, &s.FechaIngreso,
			&s.MesesServicio, &s.Acumulado, &s.Disfrutado, &s.Pendiente, &s.Reservado); err != nil {
			return nil, fmt.Errorf("nomina: saldos del equipo: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// solicitudCols es lo ÚNICO que viaja de una solicitud. La lista está escrita entera y a mano, sin
// `SELECT *` ni el `vacacionCols` de RRHH, porque es la frontera entre quien aprueba días libres y
// los datos de planilla: acá no hay salario, ni deducciones, ni identificación. Lo que no está en
// esta lista no puede filtrarse por descuido.
const solicitudCols = `v.id::text, v.empleado_id::text, e.nombre,
	COALESCE(d.nombre, ''), v.fecha_inicio::text,
	to_char(v.fecha_inicio + ((v.dias - 1) || ' day')::interval, 'YYYY-MM-DD'),
	v.dias::text, COALESCE(v.observaciones, ''), v.estado, v.anulada,
	v.creado_en::text, COALESCE(us.nombre, ''), COALESCE(ua.nombre, ''),
	COALESCE(v.aprobada_en::text, ''), COALESCE(v.motivo_rechazo, '')`

const solicitudFrom = `
	FROM vacacion v
	JOIN empleado e ON e.id = v.empleado_id
	LEFT JOIN departamento d ON d.id = e.departamento_id
	LEFT JOIN usuario us ON us.id = v.solicitada_por
	LEFT JOIN usuario ua ON ua.id = v.aprobada_por`

func escanearSolicitudes(rows pgx.Rows) ([]SolicitudVacacion, error) {
	out := make([]SolicitudVacacion, 0, 16)
	for rows.Next() {
		var s SolicitudVacacion
		if err := rows.Scan(&s.ID, &s.EmpleadoID, &s.EmpleadoNombre, &s.Departamento,
			&s.FechaInicio, &s.FechaFin, &s.Dias, &s.Observaciones, &s.Estado, &s.Anulada,
			&s.CreadoEn, &s.SolicitadaPor, &s.AprobadaPor, &s.AprobadaEn, &s.MotivoRechazo); err != nil {
			return nil, fmt.Errorf("nomina: scan solicitud: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// MisVacaciones son las de UN empleado, en cualquier estado, de la más reciente a la más vieja.
func (r *pgRepository) MisVacaciones(ctx context.Context, empresaID, empleadoID string) ([]SolicitudVacacion, error) {
	q := `SELECT ` + solicitudCols + solicitudFrom + `
		WHERE v.empresa_id = $1::uuid AND v.empleado_id = $2::uuid
		ORDER BY v.fecha_inicio DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, empleadoID)
	if err != nil {
		return nil, fmt.Errorf("nomina: mis vacaciones: %w", err)
	}
	defer rows.Close()
	return escanearSolicitudes(rows)
}

// VacacionesDeDepartamentos son las del equipo de una jefatura.
//
// `soloPendientes` deja las que esperan decisión, que es con lo que la jefatura entra a la
// pantalla. Sin él se ve el calendario completo, que es con lo que decide si puede dar más días
// esa semana.
func (r *pgRepository) VacacionesDeDepartamentos(ctx context.Context, empresaID string, deptos []string, soloPendientes bool) ([]SolicitudVacacion, error) {
	if len(deptos) == 0 {
		return []SolicitudVacacion{}, nil
	}
	filtro := ""
	if soloPendientes {
		filtro = ` AND v.estado = '` + VacacionSolicitada + `' AND NOT v.anulada`
	}
	q := `SELECT ` + solicitudCols + solicitudFrom + `
		WHERE v.empresa_id = $1::uuid AND e.departamento_id = ANY($2::uuid[])` + filtro + `
		ORDER BY v.fecha_inicio DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, deptos)
	if err != nil {
		return nil, fmt.Errorf("nomina: vacaciones del equipo: %w", err)
	}
	defer rows.Close()
	return escanearSolicitudes(rows)
}

// CambiarEstadoVacacion mueve una solicitud y deja escrito quién lo hizo.
//
// `desde` es el estado que la petición espera encontrar, y es lo que hace la operación segura
// cuando dos personas actúan a la vez: si la jefatura aprueba mientras RRHH ya confirmó, la
// segunda escritura no toca ninguna fila y devuelve ErrTransicionVacacionInvalida en vez de
// pisar el trabajo de la otra.
//
// `deptos` es el alcance: con la lista vacía no se puede cambiar NADA. El id de la vacación llega
// del cliente, así que el filtro por departamento es lo único que impide que alguien apruebe las
// vacaciones de un departamento ajeno pasando un id que no le corresponde.
func (r *pgRepository) CambiarEstadoVacacion(ctx context.Context, empresaID, vacacionID, desde, hacia, usuarioID, motivo string, deptos []string) error {
	// El motivo viaja SOLO cuando se usa. Pasarlo siempre dejaba un parámetro que no aparece en
	// la sentencia al aprobar, y Postgres no puede inferirle el tipo: la aprobación fallaba
	// entera con «could not determine data type of parameter $6».
	args := []any{empresaID, vacacionID, desde, hacia, usuarioID}
	sets := `estado = $4`
	switch hacia {
	case VacacionAprobada:
		sets += `, aprobada_por = $5::uuid, aprobada_en = now(), motivo_rechazo = NULL`
	case VacacionRechazada:
		args = append(args, motivo)
		sets += fmt.Sprintf(`, aprobada_por = $5::uuid, aprobada_en = now(), motivo_rechazo = NULLIF($%d, '')`, len(args))
	case VacacionConfirmada:
		sets += `, confirmada_por = $5::uuid, confirmada_en = now()`
	default:
		return ErrTransicionVacacionInvalida
	}
	alcance := ""
	if deptos != nil {
		// nil = sin recorte (RRHH, que ya tiene su propio permiso). Lista vacía = cierra.
		args = append(args, deptos)
		alcance = fmt.Sprintf(` AND EXISTS (SELECT 1 FROM empleado e
			WHERE e.id = vacacion.empleado_id AND e.departamento_id = ANY($%d::uuid[]))`, len(args))
	}
	q := `UPDATE vacacion SET ` + sets + `
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = $3 AND NOT anulada` + alcance
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("nomina: cambiar estado de la vacación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTransicionVacacionInvalida
	}
	return nil
}

// VacacionDeEmpleado dice de quién es una vacación, para que el service pueda comprobar que el
// colaborador solo cancela las suyas.
func (r *pgRepository) VacacionDeEmpleado(ctx context.Context, empresaID, vacacionID string) (empleadoID, estado string, err error) {
	const q = `SELECT empleado_id::text, estado FROM vacacion
	           WHERE empresa_id = $1::uuid AND id = $2::uuid AND NOT anulada`
	err = r.pool.QueryRow(ctx, q, empresaID, vacacionID).Scan(&empleadoID, &estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrVacacionNoEncontrada
	}
	if err != nil {
		return "", "", fmt.Errorf("nomina: vacación del empleado: %w", err)
	}
	return empleadoID, estado, nil
}

// ---- Jefaturas ----

// ListarJefaturas devuelve quién tiene a cargo cada departamento.
func (r *pgRepository) ListarJefaturas(ctx context.Context, empresaID string) ([]Jefatura, error) {
	const q = `
		SELECT j.departamento_id::text, COALESCE(d.nombre, ''), j.usuario_id::text,
		       COALESCE(u.nombre, ''), COALESCE(u.email, ''), j.rol
		FROM rrhh_jefatura j
		JOIN departamento d ON d.id = j.departamento_id
		JOIN usuario u ON u.id = j.usuario_id
		WHERE j.empresa_id = $1::uuid
		ORDER BY d.nombre, j.rol, u.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar jefaturas: %w", err)
	}
	defer rows.Close()
	out := make([]Jefatura, 0, 16)
	for rows.Next() {
		var j Jefatura
		if err := rows.Scan(&j.DepartamentoID, &j.Departamento, &j.UsuarioID,
			&j.UsuarioNombre, &j.UsuarioEmail, &j.Rol); err != nil {
			return nil, fmt.Errorf("nomina: listar jefaturas: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// AsignarJefatura pone (o cambia el rol de) una jefatura. El departamento tiene que ser de la
// empresa: sin esa comprobación, un id de otra empresa crearía una jefatura cruzada.
func (r *pgRepository) AsignarJefatura(ctx context.Context, empresaID, departamentoID, usuarioID, rol, porUsuarioID string) error {
	const q = `
		INSERT INTO rrhh_jefatura (empresa_id, departamento_id, usuario_id, rol, creado_por)
		SELECT $1::uuid, $2::uuid, $3::uuid, $4, $5::uuid
		WHERE EXISTS (SELECT 1 FROM departamento WHERE id = $2::uuid AND empresa_id = $1::uuid)
		ON CONFLICT (empresa_id, departamento_id, usuario_id) DO UPDATE SET rol = EXCLUDED.rol`
	tag, err := r.pool.Exec(ctx, q, empresaID, departamentoID, usuarioID, rol, porUsuarioID)
	if err != nil {
		return fmt.Errorf("nomina: asignar jefatura: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDepartamentoNoEncontrado
	}
	return nil
}

// QuitarJefatura saca a alguien de un departamento.
func (r *pgRepository) QuitarJefatura(ctx context.Context, empresaID, departamentoID, usuarioID string) error {
	const q = `DELETE FROM rrhh_jefatura
	           WHERE empresa_id = $1::uuid AND departamento_id = $2::uuid AND usuario_id = $3::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, departamentoID, usuarioID)
	if err != nil {
		return fmt.Errorf("nomina: quitar jefatura: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJefaturaNoEncontrada
	}
	return nil
}

// LigarUsuarioAEmpleado conecta una ficha con el usuario que la persona usa para entrar.
//
// `usuarioID` vacío DESLIGA, que es lo que hay que hacer cuando alguien se va: si la ficha
// conservara el vínculo, el día que ese correo se reasigne a otra persona heredaría las
// vacaciones de quien se fue.
func (r *pgRepository) LigarUsuarioAEmpleado(ctx context.Context, empresaID, empleadoID, usuarioID string) error {
	const q = `UPDATE empleado SET usuario_id = NULLIF($3, '')::uuid
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, empleadoID, usuarioID)
	if err != nil {
		return fmt.Errorf("nomina: ligar usuario al empleado: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEmpleadoNoEncontrado
	}
	return nil
}
