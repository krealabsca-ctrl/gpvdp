package cxc

// Configuración del módulo: los parámetros clave/valor, los tramos de mora con su
// probabilidad, el factor por forma de pago, las sedes y la frontera de datos
// (qué sede ve cada usuario).
//
// Por qué existe: todo esto se sembró con valores de arranque y NO se podía cambiar sin un
// UPDATE a mano en la base. El más grave era el factor del canal de asociaciones —el
// dominante— porque multiplica el orden de la cola de cobro; y la asignación de sedes, sin
// la cual un operador sin `cxc.ver_todas_sedes` veía la cola vacía y nadie podía arreglarlo.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ParametroItem es una fila de configuración clave/valor con su rastro de cambio.
//
// `Editable`, `LeidoPor` y `Nota` los llena el SERVICIO, no la base: son conocimiento del
// motor (quién lee cada parámetro), no un dato guardado. Un parámetro que nadie lee se
// muestra bloqueado con el motivo, para que la pantalla no prometa un cambio que no ocurre.
type ParametroItem struct {
	Clave          string   `json:"clave"`
	Valor          string   `json:"valor"`
	Descripcion    string   `json:"descripcion"`
	ActualizadoEn  string   `json:"actualizado_en"`
	ActualizadoPor string   `json:"actualizado_por"`
	Editable       bool     `json:"editable"`
	LeidoPor       string   `json:"leido_por"`
	Nota           string   `json:"nota"`
	Tipo           string   `json:"tipo"`
	Opciones       []string `json:"opciones"`
}

// TramoConfig es un tramo de mora editable. `Contratos` dice cuántos caen hoy en él: sin
// ese número, cambiar una probabilidad es a ciegas.
type TramoConfig struct {
	Codigo        string `json:"codigo"`
	Etiqueta      string `json:"etiqueta"`
	DiasMin       int    `json:"dias_min"`
	DiasMax       int    `json:"dias_max"`
	Orden         int    `json:"orden"`
	Prob          string `json:"prob_recuperacion"`
	Estrategia    string `json:"estrategia"`
	CanalSugerido string `json:"canal_sugerido"`
	Contratos     int    `json:"contratos"`
	Vencido       string `json:"vencido"`
}

// FormaPagoConfig es una forma de pago con su factor de recuperación editable.
type FormaPagoConfig struct {
	ID            string `json:"id"`
	Nombre        string `json:"nombre"`
	Factor        string `json:"factor_recuperacion"`
	EsAsociacion  bool   `json:"es_asociacion"`
	EsDomiciliado bool   `json:"es_domiciliado"`
	Activa        bool   `json:"activa"`
	Contratos     int    `json:"contratos"`
}

// SedeConfig es una sede operativa con lo que cuelga de ella.
type SedeConfig struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	RazonSocial string `json:"razon_social"`
	Plaza       string `json:"plaza"`
	Activa      bool   `json:"activa"`
	Contratos   int    `json:"contratos"`
	Usuarios    int    `json:"usuarios"`
}

// UsuarioSedes es la frontera de datos de un usuario: qué cartera puede ver.
type UsuarioSedes struct {
	UsuarioID string `json:"usuario_id"`
	Nombre    string `json:"nombre"`
	Email     string `json:"email"`
	Rol       string `json:"rol"`
	// VeTodasSedes: su rol tiene cxc.ver_todas_sedes, así que la asignación no lo limita.
	VeTodasSedes bool `json:"ve_todas_sedes"`
	// PuedeVerCxC: sin cxc.ver el usuario no entra al módulo; asignarle sedes no sirve.
	PuedeVerCxC bool     `json:"puede_ver_cxc"`
	SedeIDs     []string `json:"sede_ids"`
}

// ConfigCxC es todo lo que la pantalla de parámetros necesita, en una sola llamada.
type ConfigCxC struct {
	Parametros []ParametroItem   `json:"parametros"`
	Tramos     []TramoConfig     `json:"tramos"`
	FormasPago []FormaPagoConfig `json:"formas_pago"`
	Sedes      []SedeConfig      `json:"sedes"`
	Usuarios   []UsuarioSedes    `json:"usuarios"`
}

var (
	ErrTramoNoEncontrado     = errors.New("cxc: el tramo no existe en esta empresa")
	ErrTramosSeTraslapan     = errors.New("cxc: los rangos de días no pueden traslaparse con otro tramo")
	ErrFormaPagoNoEncontrada = errors.New("cxc: la forma de pago no existe en esta empresa")
	ErrSedeNoEncontrada      = errors.New("cxc: la sede no existe en esta empresa")
	ErrSedeDuplicada         = errors.New("cxc: ya existe una sede con ese nombre")
	ErrUsuarioSinAcceso      = errors.New("cxc: ese usuario no pertenece a la empresa activa")
)

func (r *pgRepository) ConfigCxC(ctx context.Context, empresaID string) (ConfigCxC, error) {
	out := ConfigCxC{
		Parametros: []ParametroItem{}, Tramos: []TramoConfig{},
		FormasPago: []FormaPagoConfig{}, Sedes: []SedeConfig{}, Usuarios: []UsuarioSedes{},
	}
	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"

	rows, err := r.pool.Query(ctx, `
		SELECT p.clave, p.valor, p.descripcion,
		       to_char(p.actualizado_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI'),
		       COALESCE(u.nombre, '')
		FROM cxc_parametro p
		LEFT JOIN usuario u ON u.id = p.actualizado_por
		WHERE p.empresa_id = $1::uuid
		ORDER BY p.clave`, empresaID)
	if err != nil {
		return ConfigCxC{}, fmt.Errorf("cxc: parámetros: %w", err)
	}
	for rows.Next() {
		var p ParametroItem
		if err := rows.Scan(&p.Clave, &p.Valor, &p.Descripcion, &p.ActualizadoEn, &p.ActualizadoPor); err != nil {
			rows.Close()
			return ConfigCxC{}, err
		}
		out.Parametros = append(out.Parametros, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ConfigCxC{}, err
	}

	// Tramos + cuántos contratos caen hoy en cada uno y cuánto vencido representan. Ese
	// número es el que permite decidir si una probabilidad tiene sentido o es un supuesto.
	rows, err = r.pool.Query(ctx, `
		WITH saldo AS (
			SELECT g.contrato_id,
			       sum(CASE WHEN g.vence_en < `+hoy+` THEN g.monto - g.monto_aplicado ELSE 0 END) AS vencido,
			       min(g.vence_en) AS mas_viejo
			FROM cargo_cxc g
			WHERE g.empresa_id = $1::uuid AND g.estado IN ('ABIERTO','PARCIAL')
			GROUP BY g.contrato_id
		),
		clasificado AS (
			SELECT (`+hoy+` - s.mas_viejo) AS dias, s.vencido
			FROM saldo s
			JOIN contrato_cxc c ON c.id = s.contrato_id
			WHERE c.empresa_id = $1::uuid AND c.estado = 'ACTIVO' AND s.vencido > 0
		)
		SELECT t.codigo, t.etiqueta, t.dias_min, t.dias_max, t.orden,
		       t.prob_recuperacion::text, t.estrategia, t.canal_sugerido,
		       COALESCE(x.n, 0)::int, COALESCE(x.vencido, 0)::text
		FROM cxc_tramo t
		LEFT JOIN LATERAL (
			SELECT count(*) AS n, sum(cl.vencido) AS vencido
			FROM clasificado cl WHERE cl.dias BETWEEN t.dias_min AND t.dias_max
		) x ON true
		WHERE t.empresa_id = $1::uuid
		ORDER BY t.orden`, empresaID)
	if err != nil {
		return ConfigCxC{}, fmt.Errorf("cxc: tramos: %w", err)
	}
	for rows.Next() {
		var t TramoConfig
		if err := rows.Scan(&t.Codigo, &t.Etiqueta, &t.DiasMin, &t.DiasMax, &t.Orden,
			&t.Prob, &t.Estrategia, &t.CanalSugerido, &t.Contratos, &t.Vencido); err != nil {
			rows.Close()
			return ConfigCxC{}, err
		}
		out.Tramos = append(out.Tramos, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ConfigCxC{}, err
	}

	rows, err = r.pool.Query(ctx, `
		SELECT f.id::text, f.nombre, f.factor_recuperacion::text, f.es_asociacion, f.es_domiciliado,
		       f.activa, (SELECT count(*) FROM contrato_cxc c WHERE c.forma_pago_id = f.id)::int
		FROM cxc_forma_pago f WHERE f.empresa_id = $1::uuid ORDER BY f.nombre`, empresaID)
	if err != nil {
		return ConfigCxC{}, fmt.Errorf("cxc: formas de pago: %w", err)
	}
	for rows.Next() {
		var f FormaPagoConfig
		if err := rows.Scan(&f.ID, &f.Nombre, &f.Factor, &f.EsAsociacion, &f.EsDomiciliado, &f.Activa, &f.Contratos); err != nil {
			rows.Close()
			return ConfigCxC{}, err
		}
		out.FormasPago = append(out.FormasPago, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ConfigCxC{}, err
	}

	rows, err = r.pool.Query(ctx, `
		SELECT s.id::text, s.nombre, COALESCE(s.razon_social,''), COALESCE(s.plaza,''), s.activa,
		       (SELECT count(*) FROM contrato_cxc c WHERE c.sede_id = s.id)::int,
		       (SELECT count(*) FROM cxc_usuario_sede us WHERE us.sede_id = s.id)::int
		FROM cxc_sede s WHERE s.empresa_id = $1::uuid ORDER BY s.activa DESC, s.nombre`, empresaID)
	if err != nil {
		return ConfigCxC{}, fmt.Errorf("cxc: sedes: %w", err)
	}
	for rows.Next() {
		var s SedeConfig
		if err := rows.Scan(&s.ID, &s.Nombre, &s.RazonSocial, &s.Plaza, &s.Activa, &s.Contratos, &s.Usuarios); err != nil {
			rows.Close()
			return ConfigCxC{}, err
		}
		out.Sedes = append(out.Sedes, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ConfigCxC{}, err
	}

	// Usuarios de la empresa con sus sedes. `ve_todas_sedes` y `puede_ver_cxc` se resuelven
	// contra la matriz de permisos del rol: la pantalla tiene que poder decir «a este no hace
	// falta asignarle nada» y «a este no le sirve, no entra al módulo».
	rows, err = r.pool.Query(ctx, `
		SELECT u.id::text, u.nombre, u.email, r.codigo,
		       EXISTS (SELECT 1 FROM rol_permiso rp JOIN permiso pe ON pe.id = rp.permiso_id
		               WHERE rp.empresa_id = uer.empresa_id AND rp.rol_id = r.id AND pe.codigo = 'cxc.ver_todas_sedes'),
		       EXISTS (SELECT 1 FROM rol_permiso rp JOIN permiso pe ON pe.id = rp.permiso_id
		               WHERE rp.empresa_id = uer.empresa_id AND rp.rol_id = r.id AND pe.codigo = 'cxc.ver'),
		       COALESCE((SELECT array_agg(us.sede_id::text) FROM cxc_usuario_sede us
		                 WHERE us.empresa_id = uer.empresa_id AND us.usuario_id = u.id), '{}')
		FROM usuario_empresa_rol uer
		JOIN usuario u ON u.id = uer.usuario_id
		JOIN rol r ON r.id = uer.rol_id
		WHERE uer.empresa_id = $1::uuid AND u.activo = true
		ORDER BY u.nombre`, empresaID)
	if err != nil {
		return ConfigCxC{}, fmt.Errorf("cxc: usuarios y sedes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		u := UsuarioSedes{SedeIDs: []string{}}
		if err := rows.Scan(&u.UsuarioID, &u.Nombre, &u.Email, &u.Rol, &u.VeTodasSedes, &u.PuedeVerCxC, &u.SedeIDs); err != nil {
			return ConfigCxC{}, err
		}
		out.Usuarios = append(out.Usuarios, u)
	}
	return out, rows.Err()
}

// GuardarParametros hace upsert de las claves recibidas. Solo toca las que vienen: no
// reescribe el resto de la configuración.
func (r *pgRepository) GuardarParametros(ctx context.Context, empresaID string, valores map[string]string, usuarioID string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("cxc: begin parámetros: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	n := 0
	for clave, valor := range valores {
		tag, err := tx.Exec(ctx, `
			INSERT INTO cxc_parametro (empresa_id, clave, valor, actualizado_en, actualizado_por)
			VALUES ($1::uuid, $2, $3, now(), NULLIF($4,'')::uuid)
			ON CONFLICT (empresa_id, clave) DO UPDATE
			SET valor = EXCLUDED.valor, actualizado_en = now(), actualizado_por = EXCLUDED.actualizado_por
			WHERE cxc_parametro.valor <> EXCLUDED.valor`,
			empresaID, clave, valor, usuarioID)
		if err != nil {
			return 0, fmt.Errorf("cxc: guardar parámetro %s: %w", clave, err)
		}
		n += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("cxc: commit parámetros: %w", err)
	}
	return n, nil
}

// CambioTramo son los campos editables de un tramo. nil = no se toca.
type CambioTramo struct {
	Prob          *string
	Estrategia    *string
	CanalSugerido *string
	DiasMin       *int
	DiasMax       *int
}

func (r *pgRepository) ActualizarTramo(ctx context.Context, empresaID, codigo string, c CambioTramo) error {
	sets := []string{}
	args := []any{empresaID, codigo}
	add := func(expr string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf(expr, len(args)))
	}
	if c.Prob != nil {
		add("prob_recuperacion = $%d::numeric", *c.Prob)
	}
	if c.Estrategia != nil {
		add("estrategia = $%d", *c.Estrategia)
	}
	if c.CanalSugerido != nil {
		add("canal_sugerido = $%d", *c.CanalSugerido)
	}
	if c.DiasMin != nil {
		add("dias_min = $%d", *c.DiasMin)
	}
	if c.DiasMax != nil {
		add("dias_max = $%d", *c.DiasMax)
	}
	if len(sets) == 0 {
		return nil
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE cxc_tramo SET `+strings.Join(sets, ", ")+
			` WHERE empresa_id = $1::uuid AND codigo = $2`, args...)
	// La base impide que dos tramos se traslapen (EXCLUDE gist): sin esa regla un mismo
	// saldo tendría dos probabilidades y la cola sería irreproducible.
	if esExclusion(err) {
		return ErrTramosSeTraslapan
	}
	if err != nil {
		return fmt.Errorf("cxc: actualizar tramo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTramoNoEncontrado
	}
	return nil
}

func (r *pgRepository) ActualizarFormaPago(ctx context.Context, empresaID, id string, factor *string, activa *bool) error {
	sets := []string{}
	args := []any{empresaID, id}
	add := func(expr string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf(expr, len(args)))
	}
	if factor != nil {
		add("factor_recuperacion = $%d::numeric", *factor)
	}
	if activa != nil {
		add("activa = $%d", *activa)
	}
	if len(sets) == 0 {
		return nil
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE cxc_forma_pago SET `+strings.Join(sets, ", ")+
			` WHERE empresa_id = $1::uuid AND id = $2::uuid`, args...)
	if err != nil {
		return fmt.Errorf("cxc: actualizar forma de pago: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrFormaPagoNoEncontrada
	}
	return nil
}

func (r *pgRepository) CrearSede(ctx context.Context, empresaID, nombre, razonSocial, plaza string) (SedeConfig, error) {
	var s SedeConfig
	err := r.pool.QueryRow(ctx, `
		INSERT INTO cxc_sede (empresa_id, nombre, razon_social, plaza)
		VALUES ($1::uuid, $2, NULLIF($3,''), NULLIF($4,''))
		RETURNING id::text, nombre, COALESCE(razon_social,''), COALESCE(plaza,''), activa`,
		empresaID, nombre, razonSocial, plaza).
		Scan(&s.ID, &s.Nombre, &s.RazonSocial, &s.Plaza, &s.Activa)
	if esViolacionUnicaPG(err) {
		return SedeConfig{}, ErrSedeDuplicada
	}
	if err != nil {
		return SedeConfig{}, fmt.Errorf("cxc: crear sede: %w", err)
	}
	return s, nil
}

func (r *pgRepository) ActualizarSede(ctx context.Context, empresaID, id string, nombre *string, activa *bool) error {
	sets := []string{}
	args := []any{empresaID, id}
	add := func(expr string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf(expr, len(args)))
	}
	if nombre != nil {
		add("nombre = $%d", *nombre)
	}
	if activa != nil {
		add("activa = $%d", *activa)
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "actualizado_en = now()")
	tag, err := r.pool.Exec(ctx,
		`UPDATE cxc_sede SET `+strings.Join(sets, ", ")+
			` WHERE empresa_id = $1::uuid AND id = $2::uuid`, args...)
	if esViolacionUnicaPG(err) {
		return ErrSedeDuplicada
	}
	if err != nil {
		return fmt.Errorf("cxc: actualizar sede: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSedeNoEncontrada
	}
	return nil
}

// AsignarSedes reemplaza el conjunto de sedes de un usuario. Es un reemplazo y no un
// agregado a propósito: la pantalla manda la lista completa que el supervisor ve marcada,
// así que quitar una casilla tiene que quitar el acceso.
func (r *pgRepository) AsignarSedes(ctx context.Context, empresaID, usuarioID string, sedeIDs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cxc: begin asignar sedes: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El usuario tiene que pertenecer a la empresa activa: si no, se le estaría dando
	// acceso a la cartera de una empresa que no es la suya.
	var pertenece bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM usuario_empresa_rol WHERE empresa_id = $1::uuid AND usuario_id = $2::uuid)`,
		empresaID, usuarioID).Scan(&pertenece); err != nil {
		return fmt.Errorf("cxc: verificar usuario: %w", err)
	}
	if !pertenece {
		return ErrUsuarioSinAcceso
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM cxc_usuario_sede WHERE empresa_id = $1::uuid AND usuario_id = $2::uuid`,
		empresaID, usuarioID); err != nil {
		return fmt.Errorf("cxc: limpiar sedes del usuario: %w", err)
	}
	if len(sedeIDs) > 0 {
		// Las sedes tienen que ser de esta empresa: el INSERT filtra por eso en vez de
		// confiar en lo que mandó el cliente.
		if _, err := tx.Exec(ctx, `
			INSERT INTO cxc_usuario_sede (empresa_id, usuario_id, sede_id)
			SELECT $1::uuid, $2::uuid, s.id
			FROM cxc_sede s
			WHERE s.empresa_id = $1::uuid AND s.id = ANY($3::uuid[])`,
			empresaID, usuarioID, sedeIDs); err != nil {
			return fmt.Errorf("cxc: asignar sedes: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cxc: commit asignar sedes: %w", err)
	}
	return nil
}

func esViolacionUnicaPG(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func esExclusion(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23P01"
}

var _ = pgx.ErrNoRows
