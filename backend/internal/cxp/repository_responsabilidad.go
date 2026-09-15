package cxp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// esIDInvalido reconoce un UUID mal formado (22P02). Un id con basura tiene que leerse como «no
// existe», no como una falla del servidor: quien manda un id inventado merece un 404, no un 500.
func esIDInvalido(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}

// Acceso a datos de las responsabilidades mensuales (migración 0082).
//
// Todo filtra por empresa_id, y encima de eso va el `Alcance`, que decide qué subconjunto ve la
// persona. El cero de `Alcance` NO VE NADA a propósito: quien olvide resolverlo se queda con una
// lista vacía —evidente y reportable— en vez de con acceso total, que sería invisible.

// FiltrosResponsabilidad acota el listado de acuerdos.
type FiltrosResponsabilidad struct {
	Estado  string
	Tipo    string
	Q       string
	Alcance Alcance
}

// FiltrosPeriodo acota el listado de meses.
type FiltrosPeriodo struct {
	Periodo           string
	Estado            string
	ResponsabilidadID string
	Alcance           Alcance
}

// ResponsabilidadInput es lo que llega a crear o editar un acuerdo.
type ResponsabilidadInput struct {
	Nombre          string
	Contraparte     string
	ProveedorID     string
	Tipo            string
	Periodicidad    string
	DiaVencimiento  int
	MesAncla        int
	Moneda          string
	MontoEsperado   string
	MontoTipo       string
	RespaldoTipo    string
	RespaldoArchivo string
	EsperaFactura   bool
	Deducible       bool
	ClasificacionID string
	DepartamentoID  string
	Notas           string
	TitularID       string
	SuplenteID      string
}

// PeriodoNuevo es una fila del mes ya calculada por el service (fecha y monto resueltos).
type PeriodoNuevo struct {
	ResponsabilidadID string
	Periodo           string
	VenceEn           string
	MontoEsperado     string
	Moneda            string
}

// CierrePeriodo describe cómo se resuelve un mes.
type CierrePeriodo struct {
	Estado       string
	CumplidaCon  string
	DocumentoID  string
	MovimientoID string
	AcuseArchivo string
	Motivo       string
}

const responsabilidadCols = `
	r.id::text, r.nombre, r.contraparte,
	COALESCE(r.proveedor_id::text, ''), COALESCE(p.nombre, ''),
	r.tipo, r.periodicidad, r.dia_vencimiento, COALESCE(r.mes_ancla, 0),
	r.moneda, r.monto_esperado::text, r.monto_tipo,
	r.respaldo_tipo, r.respaldo_archivo, r.espera_factura, r.deducible,
	COALESCE(r.clasificacion_id::text, ''), COALESCE(cl.nombre, ''),
	COALESCE(r.departamento_id::text, ''), COALESCE(dp.nombre, ''),
	r.estado, r.motivo_estado, r.notas,
	COALESCE(tit.usuario_id::text, ''), COALESCE(ut.nombre, ''),
	COALESCE(sup.usuario_id::text, ''), COALESCE(us.nombre, ''),
	to_char(r.creado_en, 'YYYY-MM-DD')`

const responsabilidadFrom = `
	FROM responsabilidad_cxp r
	LEFT JOIN proveedor p     ON p.id  = r.proveedor_id
	LEFT JOIN clasificacion cl ON cl.id = r.clasificacion_id
	LEFT JOIN departamento dp ON dp.id = r.departamento_id
	LEFT JOIN responsabilidad_responsable tit
	       ON tit.responsabilidad_id = r.id AND tit.papel = 'TITULAR'
	LEFT JOIN usuario ut ON ut.id = tit.usuario_id
	LEFT JOIN responsabilidad_responsable sup
	       ON sup.responsabilidad_id = r.id AND sup.papel = 'SUPLENTE'
	LEFT JOIN usuario us ON us.id = sup.usuario_id`

func escanearResponsabilidad(rows pgx.Rows) (Responsabilidad, error) {
	var r Responsabilidad
	err := rows.Scan(&r.ID, &r.Nombre, &r.Contraparte,
		&r.ProveedorID, &r.ProveedorNombre,
		&r.Tipo, &r.Periodicidad, &r.DiaVencimiento, &r.MesAncla,
		&r.Moneda, &r.MontoEsperado, &r.MontoTipo,
		&r.RespaldoTipo, &r.RespaldoArchivo, &r.EsperaFactura, &r.Deducible,
		&r.ClasificacionID, &r.ClasificacionNombre,
		&r.DepartamentoID, &r.DepartamentoNombre,
		&r.Estado, &r.MotivoEstado, &r.Notas,
		&r.TitularID, &r.TitularNombre,
		&r.SuplenteID, &r.SuplenteNombre,
		&r.CreadoEn)
	return r, err
}

// Alcance es lo que una persona puede ver. Se resuelve en el service a partir de los permisos y
// viaja hasta acá para que el recorte se aplique del lado del servidor, no escondiendo botones.
type Alcance struct {
	// Todo: ve todas las responsabilidades de la empresa (permiso `cxp.responsabilidades.ver`).
	Todo bool
	// ClasificacionIDs: las partidas asignadas a su rol (`rol_clasificacion_consulta`).
	ClasificacionIDs []string
	// UsuarioID: además ve aquellas donde es titular o suplente.
	UsuarioID string
}

// condicionDeAlcance arma el recorte, que es el mismo en los dos listados. Vive en una función
// para que no se pueda aplicar en uno y olvidar en el otro: un alcance que se respeta en la lista
// de acuerdos pero no en la del mes no es un alcance.
//
// Las dos piezas se combinan con O, no con Y. El encargado de servicios públicos ve lo de SUS
// partidas, Y ADEMÁS aquello de lo que lo nombraron titular aunque sea de otra partida. Con Y
// vería solo la intersección — es decir, casi nada— y el módulo no le serviría.
func condicionDeAlcance(a Alcance, args []any, alias string) (string, []any) {
	if a.Todo {
		return "", args
	}
	var partes []string
	if len(a.ClasificacionIDs) > 0 {
		args = append(args, a.ClasificacionIDs)
		partes = append(partes, fmt.Sprintf("%s.clasificacion_id = ANY($%d::uuid[])", alias, len(args)))
	}
	if a.UsuarioID != "" {
		args = append(args, a.UsuarioID)
		partes = append(partes, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM responsabilidad_responsable rr
			 WHERE rr.responsabilidad_id = %s.id AND rr.usuario_id = $%d::uuid)`, alias, len(args)))
	}
	if len(partes) == 0 {
		// Sin partidas asignadas y sin nada a su nombre: no ve nada. NUNCA «sin filtro» — ese es
		// el error que convierte un permiso restringido en acceso total.
		return "false", args
	}
	return "(" + strings.Join(partes, " OR ") + ")", args
}

func (r *pgRepository) ListarResponsabilidades(ctx context.Context, empresaID string, f FiltrosResponsabilidad) ([]Responsabilidad, error) {
	args := []any{empresaID}
	conds := []string{"r.empresa_id = $1::uuid"}

	if f.Estado != "" {
		args = append(args, f.Estado)
		conds = append(conds, fmt.Sprintf("r.estado = $%d", len(args)))
	}
	if f.Tipo != "" {
		args = append(args, f.Tipo)
		conds = append(conds, fmt.Sprintf("r.tipo = $%d", len(args)))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, "%"+q+"%")
		conds = append(conds, fmt.Sprintf("(r.nombre ILIKE $%d OR r.contraparte ILIKE $%d)", len(args), len(args)))
	}
	alcance, args := condicionDeAlcance(f.Alcance, args, "r")
	if alcance != "" {
		conds = append(conds, alcance)
	}

	q := "SELECT " + responsabilidadCols + responsabilidadFrom +
		" WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY r.estado, r.dia_vencimiento, r.nombre"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar responsabilidades: %w", err)
	}
	defer rows.Close()
	out := []Responsabilidad{}
	for rows.Next() {
		x, err := escanearResponsabilidad(rows)
		if err != nil {
			return nil, fmt.Errorf("cxp: scan responsabilidad: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *pgRepository) ResponsabilidadPorID(ctx context.Context, empresaID, id string) (Responsabilidad, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+responsabilidadCols+responsabilidadFrom+" WHERE r.empresa_id = $1::uuid AND r.id = $2::uuid",
		empresaID, id)
	if err != nil {
		if esIDInvalido(err) {
			return Responsabilidad{}, ErrResponsabilidadNoEncontrada
		}
		return Responsabilidad{}, fmt.Errorf("cxp: responsabilidad por id: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return Responsabilidad{}, ErrResponsabilidadNoEncontrada
	}
	x, err := escanearResponsabilidad(rows)
	if err != nil {
		return Responsabilidad{}, fmt.Errorf("cxp: scan responsabilidad: %w", err)
	}
	return x, nil
}

// CrearResponsabilidad inserta el acuerdo y sus responsables en UNA transacción: un acuerdo sin
// titular es un acuerdo sin dueño, que es la forma en que se olvidan las cosas.
func (r *pgRepository) CrearResponsabilidad(ctx context.Context, empresaID string, in ResponsabilidadInput, usuarioID string) (Responsabilidad, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Responsabilidad{}, fmt.Errorf("cxp: abrir transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO responsabilidad_cxp (
			empresa_id, nombre, contraparte, proveedor_id, tipo, periodicidad,
			dia_vencimiento, mes_ancla, moneda, monto_esperado, monto_tipo,
			respaldo_tipo, respaldo_archivo, espera_factura, deducible,
			clasificacion_id, departamento_id, notas, creado_por)
		VALUES ($1::uuid, $2, $3, NULLIF($4,'')::uuid, $5, $6,
		        $7, NULLIF($8,0), $9, $10::numeric, $11,
		        $12, $13, $14, $15,
		        NULLIF($16,'')::uuid, NULLIF($17,'')::uuid, $18, NULLIF($19,'')::uuid)
		RETURNING id::text`,
		empresaID, in.Nombre, in.Contraparte, in.ProveedorID, in.Tipo, in.Periodicidad,
		in.DiaVencimiento, in.MesAncla, in.Moneda, in.MontoEsperado, in.MontoTipo,
		in.RespaldoTipo, in.RespaldoArchivo, in.EsperaFactura, in.Deducible,
		in.ClasificacionID, in.DepartamentoID, in.Notas, usuarioID).Scan(&id)
	if err != nil {
		return Responsabilidad{}, traducirErrorResponsabilidad(err)
	}
	if err := fijarResponsablesTx(ctx, tx, empresaID, id, in.TitularID, in.SuplenteID); err != nil {
		return Responsabilidad{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Responsabilidad{}, fmt.Errorf("cxp: confirmar responsabilidad: %w", err)
	}
	return r.ResponsabilidadPorID(ctx, empresaID, id)
}

func (r *pgRepository) ActualizarResponsabilidad(ctx context.Context, empresaID, id string, in ResponsabilidadInput) (Responsabilidad, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Responsabilidad{}, fmt.Errorf("cxp: abrir transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ct, err := tx.Exec(ctx, `
		UPDATE responsabilidad_cxp SET
			nombre=$3, contraparte=$4, proveedor_id=NULLIF($5,'')::uuid, tipo=$6, periodicidad=$7,
			dia_vencimiento=$8, mes_ancla=NULLIF($9,0), moneda=$10, monto_esperado=$11::numeric,
			monto_tipo=$12, respaldo_tipo=$13, respaldo_archivo=$14, espera_factura=$15,
			deducible=$16, clasificacion_id=NULLIF($17,'')::uuid,
			departamento_id=NULLIF($18,'')::uuid, notas=$19, actualizado_en=now()
		WHERE empresa_id=$1::uuid AND id=$2::uuid`,
		empresaID, id, in.Nombre, in.Contraparte, in.ProveedorID, in.Tipo, in.Periodicidad,
		in.DiaVencimiento, in.MesAncla, in.Moneda, in.MontoEsperado, in.MontoTipo,
		in.RespaldoTipo, in.RespaldoArchivo, in.EsperaFactura, in.Deducible,
		in.ClasificacionID, in.DepartamentoID, in.Notas)
	if err != nil {
		return Responsabilidad{}, traducirErrorResponsabilidad(err)
	}
	if ct.RowsAffected() == 0 {
		return Responsabilidad{}, ErrResponsabilidadNoEncontrada
	}
	if err := fijarResponsablesTx(ctx, tx, empresaID, id, in.TitularID, in.SuplenteID); err != nil {
		return Responsabilidad{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Responsabilidad{}, fmt.Errorf("cxp: confirmar responsabilidad: %w", err)
	}
	return r.ResponsabilidadPorID(ctx, empresaID, id)
}

// fijarResponsablesTx reemplaza titular y suplente. Se borran y se reponen porque la tabla es un
// puente sin historia propia: quién era el titular hace dos años vive en auditoria_evento, que es
// append-only, no acá.
func fijarResponsablesTx(ctx context.Context, tx pgx.Tx, empresaID, respID, titularID, suplenteID string) error {
	if _, err := tx.Exec(ctx,
		"DELETE FROM responsabilidad_responsable WHERE empresa_id=$1::uuid AND responsabilidad_id=$2::uuid",
		empresaID, respID); err != nil {
		return fmt.Errorf("cxp: limpiar responsables: %w", err)
	}
	for _, x := range []struct{ id, papel string }{{titularID, "TITULAR"}, {suplenteID, "SUPLENTE"}} {
		if x.id == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO responsabilidad_responsable (empresa_id, responsabilidad_id, usuario_id, papel)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4)`, empresaID, respID, x.id, x.papel); err != nil {
			return traducirErrorResponsabilidad(err)
		}
	}
	return nil
}

func (r *pgRepository) CambiarEstadoResponsabilidad(ctx context.Context, empresaID, id, estado, motivo string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE responsabilidad_cxp SET estado=$3, motivo_estado=$4, actualizado_en=now()
		 WHERE empresa_id=$1::uuid AND id=$2::uuid`, empresaID, id, estado, motivo)
	if err != nil {
		return traducirErrorResponsabilidad(err)
	}
	if ct.RowsAffected() == 0 {
		return ErrResponsabilidadNoEncontrada
	}
	return nil
}

// ResponsabilidadesActivas devuelve las que hay que considerar al abrir un mes. El filtro de
// periodicidad NO va acá: lo resuelve el service con AplicaEnPeriodo, que está probado sin base.
func (r *pgRepository) ResponsabilidadesActivas(ctx context.Context, empresaID string) ([]Responsabilidad, error) {
	// Alcance{Todo: true} explícito: abrir el mes es una operación de la EMPRESA, no de quien la
	// dispara, y tiene que considerar todas las responsabilidades activas aunque quien apriete el
	// botón solo pueda ver algunas. El permiso para apretarlo se verifica en la ruta.
	return r.ListarResponsabilidades(ctx, empresaID, FiltrosResponsabilidad{
		Estado:  RespActiva,
		Alcance: Alcance{Todo: true},
	})
}

// AbrirMes inserta los períodos del mes. ES IDEMPOTENTE: el ON CONFLICT se apoya en el
// UNIQUE (responsabilidad_id, periodo) de la migración, así que llamarlo dos veces no duplica
// nada. Devuelve cuántas filas se crearon DE VERDAD — no cuántas se intentaron — para que la
// pantalla pueda decir «ya estaba abierto» en vez de fingir que hizo algo.
func (r *pgRepository) AbrirMes(ctx context.Context, empresaID string, filas []PeriodoNuevo) (int, error) {
	if len(filas) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("cxp: abrir transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	creadas := 0
	for _, f := range filas {
		ct, err := tx.Exec(ctx, `
			INSERT INTO responsabilidad_periodo
			   (empresa_id, responsabilidad_id, periodo, vence_en, monto_esperado, moneda)
			VALUES ($1::uuid, $2::uuid, $3, $4::date, $5::numeric, $6)
			ON CONFLICT (responsabilidad_id, periodo) DO NOTHING`,
			empresaID, f.ResponsabilidadID, f.Periodo, f.VenceEn, f.MontoEsperado, f.Moneda)
		if err != nil {
			return 0, fmt.Errorf("cxp: abrir mes: %w", err)
		}
		creadas += int(ct.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("cxp: confirmar apertura del mes: %w", err)
	}
	return creadas, nil
}

const periodoCols = `
	pe.id::text, pe.responsabilidad_id::text, r.nombre, r.contraparte,
	pe.periodo, to_char(pe.vence_en,'YYYY-MM-DD'), pe.monto_esperado::text, pe.moneda, r.monto_tipo,
	pe.estado, COALESCE(pe.cumplida_con,''),
	COALESCE(pe.documento_id::text,''), COALESCE(pe.movimiento_id::text,''),
	pe.acuse_archivo, pe.motivo, r.respaldo_tipo, r.deducible,
	COALESCE(ut.nombre,''), COALESCE(us.nombre,''),
	COALESCE(uc.nombre,''), COALESCE(to_char(pe.cerrado_en,'YYYY-MM-DD'),'')`

const periodoFrom = `
	FROM responsabilidad_periodo pe
	JOIN responsabilidad_cxp r ON r.id = pe.responsabilidad_id
	LEFT JOIN responsabilidad_responsable tit
	       ON tit.responsabilidad_id = r.id AND tit.papel = 'TITULAR'
	LEFT JOIN usuario ut ON ut.id = tit.usuario_id
	LEFT JOIN responsabilidad_responsable sup
	       ON sup.responsabilidad_id = r.id AND sup.papel = 'SUPLENTE'
	LEFT JOIN usuario us ON us.id = sup.usuario_id
	LEFT JOIN usuario uc ON uc.id = pe.cerrado_por`

func (r *pgRepository) ListarPeriodos(ctx context.Context, empresaID string, f FiltrosPeriodo) ([]PeriodoResponsabilidad, error) {
	args := []any{empresaID}
	conds := []string{"pe.empresa_id = $1::uuid"}

	if f.Periodo != "" {
		args = append(args, f.Periodo)
		conds = append(conds, fmt.Sprintf("pe.periodo = $%d", len(args)))
	}
	if f.Estado != "" {
		args = append(args, f.Estado)
		conds = append(conds, fmt.Sprintf("pe.estado = $%d", len(args)))
	}
	if f.ResponsabilidadID != "" {
		args = append(args, f.ResponsabilidadID)
		conds = append(conds, fmt.Sprintf("pe.responsabilidad_id = $%d::uuid", len(args)))
	}
	alcance, args := condicionDeAlcance(f.Alcance, args, "r")
	if alcance != "" {
		conds = append(conds, alcance)
	}

	q := "SELECT " + periodoCols + periodoFrom +
		" WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY pe.vence_en, r.nombre"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar períodos: %w", err)
	}
	defer rows.Close()
	out := []PeriodoResponsabilidad{}
	for rows.Next() {
		var p PeriodoResponsabilidad
		if err := rows.Scan(&p.ID, &p.ResponsabilidadID, &p.Nombre, &p.Contraparte,
			&p.Periodo, &p.VenceEn, &p.MontoEsperado, &p.Moneda, &p.MontoTipo,
			&p.Estado, &p.CumplidaCon, &p.DocumentoID, &p.MovimientoID,
			&p.AcuseArchivo, &p.Motivo, &p.RespaldoTipo, &p.Deducible,
			&p.TitularNombre, &p.SuplenteNombre,
			&p.CerradoPorNombre, &p.CerradoEn); err != nil {
			return nil, fmt.Errorf("cxp: scan período: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CerrarPeriodo resuelve un mes. Solo toca los que siguen PENDIENTE: cerrar dos veces el mismo mes
// sobreescribiría en silencio quién lo cerró y con qué, y esa es la traza que después se audita.
func (r *pgRepository) CerrarPeriodo(ctx context.Context, empresaID, id string, c CierrePeriodo, usuarioID string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE responsabilidad_periodo SET
			estado=$3, cumplida_con=NULLIF($4,''), documento_id=NULLIF($5,'')::uuid,
			movimiento_id=NULLIF($6,'')::uuid, acuse_archivo=$7, motivo=$8,
			cerrado_por=NULLIF($9,'')::uuid, cerrado_en=now()
		WHERE empresa_id=$1::uuid AND id=$2::uuid AND estado='PENDIENTE'`,
		empresaID, id, c.Estado, c.CumplidaCon, c.DocumentoID, c.MovimientoID,
		c.AcuseArchivo, c.Motivo, usuarioID)
	if err != nil {
		return traducirErrorResponsabilidad(err)
	}
	if ct.RowsAffected() == 0 {
		// O no existe en esta empresa, o ya estaba resuelto. Se distingue con una lectura, para
		// que el mensaje al usuario no diga «no existe» cuando en realidad ya estaba cerrado.
		var estado string
		err := r.pool.QueryRow(ctx,
			"SELECT estado FROM responsabilidad_periodo WHERE empresa_id=$1::uuid AND id=$2::uuid",
			empresaID, id).Scan(&estado)
		if err != nil {
			return ErrPeriodoNoEncontrado
		}
		return ErrPeriodoYaCerrado
	}
	return nil
}

// ReabrirPeriodo devuelve un mes a PENDIENTE, borrando la prueba y dejando el motivo de la
// reapertura. Es lo que corrige un cierre equivocado sin borrar la fila.
func (r *pgRepository) ReabrirPeriodo(ctx context.Context, empresaID, id, motivo string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE responsabilidad_periodo SET
			estado='PENDIENTE', cumplida_con=NULL, documento_id=NULL, movimiento_id=NULL,
			acuse_archivo='', motivo=$3, cerrado_por=NULL, cerrado_en=NULL
		WHERE empresa_id=$1::uuid AND id=$2::uuid AND estado <> 'PENDIENTE'`,
		empresaID, id, motivo)
	if err != nil {
		return traducirErrorResponsabilidad(err)
	}
	if ct.RowsAffected() == 0 {
		return ErrPeriodoNoEncontrado
	}
	return nil
}

// IDsConFilaEnElMes devuelve qué responsabilidades YA tienen su fila del mes. El service resta
// contra las que deberían tenerla y de ahí sale «SIN ABRIR», que es el estado que atrapa el olvido
// del olvido: si nadie abrió el mes, nada figura vencido y todo parece en orden.
func (r *pgRepository) IDsConFilaEnElMes(ctx context.Context, empresaID, periodo string) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT responsabilidad_id::text FROM responsabilidad_periodo WHERE empresa_id=$1::uuid AND periodo=$2",
		empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("cxp: ids con fila en el mes: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cxp: scan id del mes: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// PartidasDelRol devuelve las clasificaciones que el rol del usuario puede consultar.
//
// Reutiliza `rol_clasificacion_consulta`, la misma tabla con la que Bancos resuelve la consulta por
// segmento (migración 0077). Ojo con el matiz, que no es menor: el alcance va por ROL, no por
// persona — y como `usuario_empresa_rol` es único por empresa+usuario, cada quien tiene uno solo.
// Para que «el encargado de servicios públicos vea los suyos» hay que darle un rol con esas
// partidas; lo que es personal de cada quien se resuelve por titular/suplente, no por acá.
func (r *pgRepository) PartidasDelRol(ctx context.Context, empresaID, usuarioID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT rcc.clasificacion_id::text
		  FROM usuario_empresa_rol uer
		  JOIN rol_clasificacion_consulta rcc
		    ON rcc.rol_id = uer.rol_id AND rcc.empresa_id = uer.empresa_id
		  JOIN clasificacion cl ON cl.id = rcc.clasificacion_id AND cl.activo = true
		 WHERE uer.empresa_id = $1::uuid AND uer.usuario_id = $2::uuid`, empresaID, usuarioID)
	if err != nil {
		if esIDInvalido(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("cxp: partidas del rol: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cxp: scan partida del rol: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// UltimaFechaBanco dice hasta cuándo alcanzan los movimientos importados. Es lo que permite
// distinguir «esto está vencido» de «todavía no puedo saberlo». Devuelve "" si no hay ninguno.
func (r *pgRepository) UltimaFechaBanco(ctx context.Context, empresaID string) (string, error) {
	var fecha *string
	err := r.pool.QueryRow(ctx,
		"SELECT to_char(MAX(fecha),'YYYY-MM-DD') FROM movimiento_bancario WHERE empresa_id=$1::uuid",
		empresaID).Scan(&fecha)
	if err != nil {
		return "", fmt.Errorf("cxp: última fecha de banco: %w", err)
	}
	if fecha == nil {
		return "", nil
	}
	return *fecha, nil
}

// traducirErrorResponsabilidad convierte los frenos de la base en errores de dominio, para que el
// usuario lea por qué no se pudo y no un mensaje de Postgres.
func traducirErrorResponsabilidad(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "uq_periodo_documento"), strings.Contains(msg, "uq_periodo_movimiento"):
		return ErrPruebaYaUsada
	case strings.Contains(msg, "responsabilidad_sin_respaldo_no_deducible"):
		return ErrSinComprobanteNoDeducible
	case strings.Contains(msg, "responsabilidad_respaldo_con_archivo"):
		return ErrRespaldoSinArchivo
	case strings.Contains(msg, "periodo_no_aplica_con_motivo"):
		return ErrMotivoObligatorio
	case strings.Contains(msg, "periodo_cumplida_con_prueba"),
		strings.Contains(msg, "periodo_pendiente_sin_prueba"):
		return ErrPruebaObligatoria
	case strings.Contains(msg, "responsabilidad_ancla_si_no_es_mensual"),
		strings.Contains(msg, "responsabilidad_cxp_periodicidad_check"):
		return ErrPeriodicidadInvalida
	case strings.Contains(msg, "uq_responsabilidad_nombre"):
		return errors.New("cxp: ya existe una responsabilidad con ese nombre")
	case esIDInvalido(err):
		return ErrResponsabilidadNoEncontrada
	}
	return fmt.Errorf("cxp: guardar responsabilidad: %w", err)
}
