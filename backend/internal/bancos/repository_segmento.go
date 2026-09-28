package bancos

// Consulta por segmento — acceso a datos (mig 0077).
//
// Todo filtra por `empresa_id`, incluidas las consultas que podrían resolverse solo por id: el
// alcance de un rol es exactamente el dato que no puede cruzarse entre empresas.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

// idInvalido reconoce el uuid mal escrito que llegó de una URL o de un cuerpo: es un «no existe»,
// no un 500. Postgres lo reporta como error de sintaxis al castear.
func idInvalido(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid input syntax")
}

// AlcanceDeUsuario devuelve las clasificaciones que el usuario puede consultar.
//
// Sale del ROL: un usuario tiene UN rol por empresa (`usuario_empresa_rol` es único por
// empresa+usuario), así que el alcance es el de su rol y no hace falta unir nada.
//
// Devuelve un slice vacío —no un error— cuando al rol no le asignaron partidas: es un estado
// legítimo del negocio (la mayoría de los roles no consulta por segmento) y quien lo llama tiene
// que tratarlo como «no ve nada», nunca como «sin filtro».
func (r *pgRepository) AlcanceDeUsuario(ctx context.Context, empresaID, usuarioID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT rcc.clasificacion_id::text
		FROM usuario_empresa_rol uer
		JOIN rol_clasificacion_consulta rcc
		  ON rcc.rol_id = uer.rol_id AND rcc.empresa_id = uer.empresa_id
		JOIN clasificacion cl ON cl.id = rcc.clasificacion_id AND cl.activo = true
		WHERE uer.empresa_id = $1::uuid AND uer.usuario_id = $2::uuid`, empresaID, usuarioID)
	if err != nil {
		if idInvalido(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("bancos: alcance del usuario: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("bancos: scan alcance: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("bancos: iterar alcance: %w", err)
	}
	return out, nil
}

// PartidasDelAlcance describe las partidas del alcance, para que la pantalla pueda decir de qué es
// lo que muestra sin que el cliente cruce el catálogo.
func (r *pgRepository) PartidasDelAlcance(ctx context.Context, empresaID string, ids []string) ([]PartidaDelSegmento, error) {
	if len(ids) == 0 {
		return []PartidaDelSegmento{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT cl.id::text, cl.nombre, co.nombre
		FROM clasificacion cl
		JOIN concepto co ON co.id = cl.concepto_id
		WHERE cl.empresa_id = $1::uuid AND cl.id = ANY($2::uuid[])
		ORDER BY co.nombre, cl.nombre`, empresaID, ids)
	if err != nil {
		return nil, fmt.Errorf("bancos: partidas del alcance: %w", err)
	}
	defer rows.Close()
	out := []PartidaDelSegmento{}
	for rows.Next() {
		var p PartidaDelSegmento
		if err := rows.Scan(&p.ClasificacionID, &p.Clasificacion, &p.Concepto); err != nil {
			return nil, fmt.Errorf("bancos: scan partida del alcance: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// cuentasDelSegmentoSQL es la DEFINICIÓN de «cuentas del segmento» (decisión del Director
// Financiero, 22-set-2026). Vive escrita UNA sola vez porque la usan dos consultas que no pueden
// discrepar: el desplegable de cuentas (CuentasDelAlcance) y el «cargado hasta»
// (CargaDeCuentasDelSegmento). Si cada una la escribiera a su modo, el filtro ofrecería una cuenta
// de la que el encabezado después no sabría decir hasta cuándo está cargada.
//
// La usaban cinco hasta el 23-set-2026; las otras tres eran las que mostraban o dejaban avisar sobre
// créditos SIN clasificar, que el Director Financiero mandó quitar de esta pantalla.
//
// Cuentas del segmento = las cuentas bancarias de la empresa que tienen AL MENOS UN crédito
// INCLUIDO clasificado en el alcance del usuario, en todo el histórico.
//
//   - Es DERIVADA, no se configura: si una partida empieza a recibir plata en otra cuenta, esa
//     cuenta entra sola en cuanto se clasifica el primer crédito.
//   - «Incluido»: una cuenta cuyo único crédito de la partida era un duplicado revertido no es una
//     cuenta donde entra la plata del equipo.
//   - «En todo el histórico», no en el período filtrado: la pregunta es «¿en qué cuentas puede caer
//     mi plata?», y un mes sin depósitos en una cuenta no la saca del segmento.
//   - Alcance vacío → CERO cuentas (`= ANY('{}')` no calza con nada). Igual ninguna consulta llega
//     acá con él: todas cortan antes.
//
// Devuelve un subselect de ids para usar con `IN (...)`. Espera la empresa en `$1`; `alcance` es
// el placeholder del arreglo de clasificaciones (p. ej. "$2"). El alias `seg` es propio para no
// correlacionarse por accidente con la `m` de la consulta de afuera.
func cuentasDelSegmentoSQL(alcance string) string {
	return `SELECT seg.cuenta_bancaria_id FROM movimiento_bancario seg
	        WHERE seg.empresa_id = $1::uuid AND seg.incluido AND seg.credito > 0
	          AND seg.clasificacion_id = ANY(` + alcance + `::uuid[])`
}

// CuentasDelAlcance devuelve las cuentas del segmento (ver cuentasDelSegmentoSQL), para el filtro.
//
// Sale de los movimientos del propio alcance: así el desplegable ofrece solo lo que el equipo puede
// ver, y no hace falta darle acceso al catálogo de cuentas de la empresa.
//
// Solo cuentas con movimientos INCLUIDOS: la pregunta que contesta el desplegable es «¿de qué
// cuentas entra mi plata?», y una cuenta que aporta cero —porque lo único que tenía era el
// estado de cuenta importado por error y ya excluido— es una respuesta falsa a esa pregunta.
func (r *pgRepository) CuentasDelAlcance(ctx context.Context, empresaID string, alcance []string) ([]CuentaDelSegmento, error) {
	if len(alcance) == 0 {
		return []CuentaDelSegmento{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT cb.id::text, COALESCE(b.nombre,''), COALESCE(cb.alias,'')
		FROM cuenta_bancaria cb
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE cb.empresa_id = $1::uuid AND cb.id IN (`+cuentasDelSegmentoSQL("$2")+`)
		ORDER BY 2, 3, 1`, empresaID, alcance)
	if err != nil {
		return nil, fmt.Errorf("bancos: cuentas del alcance: %w", err)
	}
	defer rows.Close()
	out := []CuentaDelSegmento{}
	for rows.Next() {
		var c CuentaDelSegmento
		if err := rows.Scan(&c.ID, &c.Banco, &c.Cuenta); err != nil {
			return nil, fmt.Errorf("bancos: scan cuenta del alcance: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UltimaFechaCargada es la fecha del último movimiento importado de la EMPRESA.
//
// Deliberadamente sin recortar por alcance: contesta «¿ya cargaron el banco?», que es una pregunta
// sobre la operación y no sobre el segmento de nadie. Devuelve "" si la empresa no tiene ni un
// movimiento.
//
// Solo movimientos INCLUIDOS: este dato acompaña al veredicto «no existe» y es lo que separa «no
// entró» de «todavía no lo han cargado». Una fecha sostenida únicamente por una importación que
// después se excluyó entera afirmaría «estamos al día» sobre datos que el sistema ya declaró que
// no cuentan, y eso frena un aviso de faltante legítimo.
func (r *pgRepository) UltimaFechaCargada(ctx context.Context, empresaID string) (string, error) {
	var fecha *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT max(fecha) FROM movimiento_bancario WHERE empresa_id = $1::uuid AND incluido`, empresaID).Scan(&fecha)
	if err != nil {
		return "", fmt.Errorf("bancos: última fecha cargada: %w", err)
	}
	if fecha == nil {
		return "", nil
	}
	return fecha.Format("2006-01-02"), nil
}

// CargaDeCuentasDelSegmento dice, para cada cuenta del segmento, hasta qué día está importada: el
// máximo de `fecha` de sus movimientos INCLUIDOS (decisión del Director Financiero, 22-set-2026).
//
// Por cuenta se mira TODO lo incluido de la cuenta —débitos, créditos, cualquier partida—, no solo
// lo del segmento: la pregunta es «¿ya cargaron este banco?», y una partida que no tuvo depósitos el
// 10 no hace que el banco esté cargado solo hasta el 9. `incluido` por la misma razón que
// UltimaFechaCargada: una importación revertida no prueba que el banco esté al día.
//
// Viene ordenada de la más atrasada a la más al día (desempate por banco, cuenta e id). Con alcance
// vacío devuelve la lista vacía SIN consultar: cero cuentas, así que no hay fecha que afirmar.
func (r *pgRepository) CargaDeCuentasDelSegmento(ctx context.Context, empresaID string, alcance []string) ([]CuentaCargadaHasta, error) {
	if len(alcance) == 0 {
		return []CuentaCargadaHasta{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT cb.id::text, COALESCE(b.nombre,''), COALESCE(cb.alias,''), max(m.fecha)
		FROM movimiento_bancario m
		JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id AND cb.empresa_id = $1::uuid
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE m.empresa_id = $1::uuid AND m.incluido
		  AND m.cuenta_bancaria_id IN (`+cuentasDelSegmentoSQL("$2")+`)
		GROUP BY cb.id, b.nombre, cb.alias
		ORDER BY max(m.fecha), 2, 3, 1`, empresaID, alcance)
	if err != nil {
		return nil, fmt.Errorf("bancos: carga de las cuentas del segmento: %w", err)
	}
	defer rows.Close()
	out := []CuentaCargadaHasta{}
	for rows.Next() {
		var (
			c     CuentaCargadaHasta
			fecha time.Time
		)
		// max(fecha) no puede venir nulo: toda cuenta del segmento tiene, por definición, al menos
		// un crédito incluido.
		if err := rows.Scan(&c.ID, &c.Banco, &c.Cuenta, &fecha); err != nil {
			return nil, fmt.Errorf("bancos: scan carga de cuenta: %w", err)
		}
		c.CargadoHasta = fecha.Format("2006-01-02")
		out = append(out, c)
	}
	return out, rows.Err()
}

// RolesDeConsulta lista los roles de la empresa que TIENEN `bancos.ver_mi_segmento`.
//
// Son los que la columna del catálogo puede ofrecer. Asignarle una partida a un rol que no puede
// abrir la pantalla no falla, pero es una marca que no hace nada: mejor no ofrecerlo.
//
// Se incluyen los roles base (`empresa_id IS NULL`) porque un rol base también recibe permisos por
// empresa en `rol_permiso`, y es ahí donde se decide si consulta o no.
func (r *pgRepository) RolesDeConsulta(ctx context.Context, empresaID string) ([]RolDeConsulta, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ro.id::text, ro.codigo, ro.nombre,
		       (SELECT count(*) FROM usuario_empresa_rol uer
		         WHERE uer.rol_id = ro.id AND uer.empresa_id = $1::uuid)
		FROM rol ro
		JOIN rol_permiso rp ON rp.rol_id = ro.id AND rp.empresa_id = $1::uuid
		JOIN permiso pe ON pe.id = rp.permiso_id AND pe.codigo = 'bancos.ver_mi_segmento'
		WHERE ro.empresa_id IS NULL OR ro.empresa_id = $1::uuid
		ORDER BY ro.nombre`, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: roles de consulta: %w", err)
	}
	defer rows.Close()
	out := []RolDeConsulta{}
	for rows.Next() {
		var ro RolDeConsulta
		if err := rows.Scan(&ro.ID, &ro.Codigo, &ro.Nombre, &ro.Usuarios); err != nil {
			return nil, fmt.Errorf("bancos: scan rol de consulta: %w", err)
		}
		out = append(out, ro)
	}
	return out, rows.Err()
}

// AsignacionesConsulta devuelve el alcance completo de la empresa, ya resuelto a nombres.
//
// Es una sola lista para las dos puertas: la columna del catálogo la agrupa por partida, la vista
// de Seguridad la agrupa por rol. Un solo dato, así las dos pantallas no pueden discrepar.
func (r *pgRepository) AsignacionesConsulta(ctx context.Context, empresaID string) ([]AsignacionConsulta, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cl.id::text, cl.nombre, co.id::text, co.nombre, ro.id::text, ro.nombre
		FROM rol_clasificacion_consulta rcc
		JOIN clasificacion cl ON cl.id = rcc.clasificacion_id
		JOIN concepto co ON co.id = cl.concepto_id
		JOIN rol ro ON ro.id = rcc.rol_id
		WHERE rcc.empresa_id = $1::uuid
		ORDER BY co.nombre, cl.nombre, ro.nombre`, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: asignaciones de consulta: %w", err)
	}
	defer rows.Close()
	out := []AsignacionConsulta{}
	for rows.Next() {
		var a AsignacionConsulta
		if err := rows.Scan(&a.ClasificacionID, &a.Clasificacion, &a.ConceptoID, &a.Concepto,
			&a.RolID, &a.RolNombre); err != nil {
			return nil, fmt.Errorf("bancos: scan asignación: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GuardarConsultaDePartida reemplaza el conjunto de roles que consultan UNA partida.
//
// Reemplazar y no acumular: la pantalla manda la lista completa de la fila, que es cómo se ve y
// cómo se piensa («esta partida la ven estos»). Se hace en una transacción para que no exista un
// instante con la partida sin nadie.
//
// Devuelve ErrClasificacionNoEncontrada si la partida no es de esta empresa, y
// ErrRolDeConsultaNoEncontrado si alguno de los roles no lo es: pasar un id de otra empresa no
// puede ser un 500 ni, mucho menos, un guardado.
func (r *pgRepository) GuardarConsultaDePartida(ctx context.Context, empresaID, clasificacionID string, rolIDs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bancos: abrir tx alcance: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existe bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM clasificacion WHERE id = $2::uuid AND empresa_id = $1::uuid)`,
		empresaID, clasificacionID).Scan(&existe)
	if err != nil {
		if idInvalido(err) {
			return ErrClasificacionNoEncontrada
		}
		return fmt.Errorf("bancos: verificar clasificación: %w", err)
	}
	if !existe {
		return ErrClasificacionNoEncontrada
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM rol_clasificacion_consulta
		WHERE empresa_id = $1::uuid AND clasificacion_id = $2::uuid`, empresaID, clasificacionID); err != nil {
		return fmt.Errorf("bancos: limpiar alcance de la partida: %w", err)
	}

	for _, rolID := range rolIDs {
		// El rol tiene que ser de esta empresa o base: el INSERT ... SELECT lo verifica en la misma
		// sentencia, así que un rol ajeno no inserta nada y se distingue del que sí insertó.
		ct, err := tx.Exec(ctx, `
			INSERT INTO rol_clasificacion_consulta (empresa_id, rol_id, clasificacion_id)
			SELECT $1::uuid, ro.id, $3::uuid
			FROM rol ro
			WHERE ro.id = $2::uuid AND (ro.empresa_id IS NULL OR ro.empresa_id = $1::uuid)
			ON CONFLICT DO NOTHING`, empresaID, rolID, clasificacionID)
		if err != nil {
			if idInvalido(err) {
				return ErrRolDeConsultaNoEncontrado
			}
			return fmt.Errorf("bancos: asignar rol al alcance: %w", err)
		}
		// Cero filas = el rol no existe o es de otra empresa. `ON CONFLICT DO NOTHING` también
		// afecta cero filas, pero acá no puede tapar nada: el DELETE de arriba dejó la partida sin
		// asignaciones, así que un conflicto solo puede venir del mismo rol repetido en la lista
		// —y el servicio la deduplica antes de llamar—.
		if ct.RowsAffected() == 0 {
			return ErrRolDeConsultaNoEncontrado
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bancos: confirmar alcance: %w", err)
	}
	return nil
}

// MovimientoEnAlcance dice si el movimiento existe en la empresa Y el equipo lo puede VER en su
// pantalla, que es la condición para poder avisar sobre él: un crédito de una partida del alcance.
//
// Es exactamente el mismo predicado que arma la lista (condicionesMovimientos con Alcance): se puede
// avisar sobre lo que se ve, y sobre nada más.
//
// Entre el 22 y el 23-set-2026 aceptaba además el crédito SIN clasificar de una cuenta del segmento,
// porque la pestaña «Todavía sin partida» lo mostraba. Quitada la pestaña, se quitó acá también: si
// no, mandando ids a mano se podría confirmar la existencia de un movimiento que la pantalla ya no
// enseña, y la guarda estaría permitiendo justo lo que el permiso dice que no.
//
// Con alcance vacío devuelve false sin consultar: es la misma regla que el listado —sin alcance no
// se ve nada—, y acá además evita que un `ANY(ARRAY[]::uuid[])` parezca un descuido.
func (r *pgRepository) MovimientoEnAlcance(ctx context.Context, empresaID, movID string, alcance []string) (bool, error) {
	if len(alcance) == 0 {
		return false, nil
	}
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM movimiento_bancario m
		  WHERE m.id = $2::uuid AND m.empresa_id = $1::uuid
		    AND m.credito > 0
		    AND m.clasificacion_id = ANY($3::uuid[])
		)`, empresaID, movID, alcance).Scan(&ok)
	if err != nil {
		if idInvalido(err) {
			return false, nil
		}
		return false, fmt.Errorf("bancos: movimiento en alcance: %w", err)
	}
	return ok, nil
}

// BuscarPorFechaYMonto responde si existe un crédito de esa fecha y ese monto exactos (mig 0078).
//
// Separa las coincidencias en DOS grupos:
//
//   - `mios`: de una partida del alcance. Se devuelven completos, porque el usuario ya los ve.
//   - `fuera`: todo lo demás —otra partida, o todavía sin partida—. Se CUENTA y no se trae ni un
//     dato: la existencia es todo lo que se divulga.
//
// Lo sin clasificar tuvo su propio grupo entre el 22 y el 23-set-2026, mientras existió la pestaña
// «Todavía sin partida»; ahora suma a `fuera` como el resto de lo que el usuario no ve.
//
// El grupo se decide con un CASE y no con `clasificacion_id = ANY(...)` escaneado a un bool: esa
// comparación da NULL con un crédito sin clasificar —el caso más común, 563 en setiembre— y escanear
// NULL en un bool era el 500 de «Falta un movimiento». Dentro del CASE el NULL no hace daño: un WHEN
// que no es verdadero cae al ELSE, que es justamente donde va lo que no es suyo.
//
// El monto se compara contra `credito` y no contra `monto_crc`: el equipo tiene el recibo del
// depósito en colones tal como lo hizo, y `monto_crc` de una cuenta en dólares es una conversión
// que nunca va a coincidir con lo que la persona escribe.
//
// Solo movimientos INCLUIDOS. Es la consulta donde un duplicado hace el daño más silencioso: el
// equipo busca su depósito, encuentra el fantasma, lee «existe, está en tu partida» y deja de
// buscar, mientras el depósito de verdad puede seguir faltando. Si la única coincidencia es un
// excluido, la respuesta correcta es «no existe», porque esa plata no entró a los libros.
func (r *pgRepository) BuscarPorFechaYMonto(
	ctx context.Context, empresaID, fecha string, monto decimal.Decimal, alcance []string,
) (mios []MovimientoRow, fuera int, err error) {
	// El alcance vacío ni se consulta: sin partidas asignadas no hay pantalla desde donde buscar.
	if len(alcance) == 0 {
		return nil, 0, nil
	}
	// `m.credito > 0` en el WHERE aunque el servicio ya exige un monto positivo: «solo créditos» es la
	// regla de esta pantalla, y no puede depender de quién llame.
	rows, err := r.pool.Query(ctx, `
		SELECT m.id::text, m.fecha, COALESCE(m.documento,''), COALESCE(m.descripcion,''),
		       m.debito, m.credito, m.moneda_original, m.monto_crc,
		       m.concepto_id::text, COALESCE(co.nombre,''),
		       m.clasificacion_id::text, COALESCE(cl.nombre,''),
		       m.estado_clasificacion, m.es_traslado, m.incluido,
		       COALESCE(b.nombre,''), COALESCE(cb.alias,''),
		       CASE
		         WHEN m.clasificacion_id = ANY($4::uuid[]) THEN 'MIO'
		         ELSE 'FUERA'
		       END AS grupo
		FROM movimiento_bancario m
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE m.empresa_id = $1::uuid AND m.incluido AND m.credito > 0
		  AND m.fecha = $2::date AND m.credito = $3
		ORDER BY m.id`, empresaID, fecha, monto, alcance)
	if err != nil {
		if idInvalido(err) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("bancos: buscar por fecha y monto: %w", err)
	}
	defer rows.Close()
	mios = []MovimientoRow{}
	for rows.Next() {
		var (
			row       MovimientoRow
			f         time.Time
			deb, cred decimal.Decimal
			mcrc      decimal.Decimal
			grupo     string
		)
		if err := rows.Scan(&row.ID, &f, &row.Documento, &row.Descripcion,
			&deb, &cred, &row.Moneda, &mcrc,
			&row.ConceptoID, &row.Concepto, &row.ClasificacionID, &row.Clasificacion,
			&row.Estado, &row.EsTraslado, &row.Incluido,
			&row.Banco, &row.Cuenta, &grupo); err != nil {
			return nil, 0, fmt.Errorf("bancos: scan búsqueda de faltante: %w", err)
		}
		// Lo ajeno se CUENTA y se descarta acá mismo, en el repositorio: así no queda un
		// `MovimientoRow` con datos que el usuario no ve viajando por el servicio, donde un descuido
		// futuro lo podría serializar.
		if grupo != "MIO" {
			fuera++
			continue
		}
		row.Fecha = f.Format("2006-01-02")
		row.Debito = deb.String()
		row.Credito = cred.String()
		row.MontoCRC = mcrc.String()
		row.ConsecutivoLargo = ConsecutivoLargo(row.Banco, row.Descripcion)
		mios = append(mios, row)
	}
	return mios, fuera, rows.Err()
}

// EngancharFaltante busca el ÚNICO crédito de esa fecha y ese monto en la empresa.
//
// Sirve para que el aviso de faltante llegue con el movimiento ya identificado y quien clasifica no
// lo tenga que buscar. Si hay más de uno devuelve "" a propósito: dos depósitos idénticos el mismo
// día son indistinguibles y adivinar cuál es sería peor que no enganchar ninguno.
//
// Solo movimientos INCLUIDOS: si no, el duplicado excluido convierte un enganche exitoso en un
// empate y el aviso viaja sin movimiento, que es justo el trabajo manual que esta función existe
// para evitar. (Hay empates legítimos —depósitos idénticos de verdad— y esos siguen dando "".)
func (r *pgRepository) EngancharFaltante(ctx context.Context, empresaID, fecha string, monto decimal.Decimal) (string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text FROM movimiento_bancario
		WHERE empresa_id = $1::uuid AND incluido
		  AND fecha = $2::date AND credito = $3
		LIMIT 2`, empresaID, fecha, monto)
	if err != nil {
		if idInvalido(err) {
			return "", nil
		}
		return "", fmt.Errorf("bancos: enganchar faltante: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", fmt.Errorf("bancos: scan enganche: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(ids) != 1 {
		return "", nil
	}
	return ids[0], nil
}

// CrearReporteFaltante registra el aviso de un movimiento que el equipo espera y no ve.
//
// `movimientoID` vacío = no se pudo enganchar (no existe, o hay varios idénticos): el aviso viaja
// con la fecha, el monto y la referencia, que es con lo que quien clasifica lo va a buscar.
func (r *pgRepository) CrearReporteFaltante(
	ctx context.Context, empresaID, usuarioID, fecha string, monto decimal.Decimal,
	referencia, motivo, movimientoID string,
) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO movimiento_reporte_segmentacion
		  (empresa_id, usuario_id, fecha_esperada, monto_esperado, referencia, motivo, movimiento_id)
		VALUES ($1::uuid, $2::uuid, $3::date, $4, NULLIF($5,''), $6, NULLIF($7,'')::uuid)
		RETURNING id::text`, empresaID, usuarioID, fecha, monto, referencia, motivo, movimientoID).Scan(&id)
	if err != nil {
		// Dos índices distintos pueden rechazar esto y el mensaje NO es el mismo. Si se enganchó un
		// movimiento que ya tenía aviso abierto, lo que hay que decir es «ya está en revisión»
		// —quizá lo reportó otra persona—; «ya avisaste vos» sería falso.
		if esViolacionDelIndice(err, "uq_reporte_abierto_por_movimiento") {
			return "", ErrReporteYaAbierto
		}
		if esViolacionUnica(err) {
			return "", ErrFaltanteYaAvisado
		}
		return "", fmt.Errorf("bancos: crear aviso de faltante: %w", err)
	}
	return id, nil
}

// esViolacionDelIndice distingue CUÁL restricción única se violó, para no dar un mensaje que
// afirma algo falso sobre quién hizo qué.
func esViolacionDelIndice(err error, nombre string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == nombre
}

// CrearReporteSegmentacion registra el aviso del equipo.
//
// Devuelve ErrReporteYaAbierto si ese movimiento ya tiene un aviso sin resolver: lo impone un
// índice único parcial, así que dos avisos simultáneos tampoco pasan.
func (r *pgRepository) CrearReporteSegmentacion(ctx context.Context, empresaID, movID, usuarioID, motivo string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO movimiento_reporte_segmentacion (empresa_id, movimiento_id, usuario_id, motivo)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
		RETURNING id::text`, empresaID, movID, usuarioID, motivo).Scan(&id)
	if err != nil {
		if esViolacionUnica(err) {
			return "", ErrReporteYaAbierto
		}
		return "", fmt.Errorf("bancos: crear reporte de segmentación: %w", err)
	}
	return id, nil
}

// ListarReportesSegmentacion devuelve los avisos con el movimiento y su partida de HOY.
//
// La partida se lee en vivo y no se copia al reporte a propósito: quien resuelve tiene que juzgar
// cómo está clasificado el movimiento AHORA, no cómo estaba cuando alguien avisó.
func (r *pgRepository) ListarReportesSegmentacion(ctx context.Context, empresaID string, soloPendientes bool) ([]ReporteSegmentacion, error) {
	cond := ""
	if soloPendientes {
		cond = " AND rs.resuelto_en IS NULL"
	}
	// LEFT JOIN con el movimiento, no JOIN: un aviso de FALTANTE puede no tener movimiento
	// enganchado (mig 0078), y un JOIN lo dejaría fuera de la cola en silencio — el aviso existiría
	// en la base y nadie lo vería nunca.
	//
	// Por eso `incluido` acá NO filtra: se TRAE. «El movimiento se excluyó por duplicado» suele ser
	// LA respuesta al aviso, no algo que haya que ocultarle a quien lo resuelve; y con un LEFT JOIN
	// filtrar ni siquiera sacaría el aviso de la cola, lo dejaría con todos los campos del
	// movimiento vacíos y `EsFaltante` en false: una fila muda que no se entiende ni se resuelve.
	// El COALESCE es obligatorio: sin movimiento enganchado la columna viene NULL.
	rows, err := r.pool.Query(ctx, `
		SELECT rs.id::text, rs.motivo, COALESCE(u.nombre, u.email), rs.creado_en,
		       COALESCE(m.id::text,''), m.fecha, COALESCE(m.descripcion,''), COALESCE(m.monto_crc,0),
		       COALESCE(m.incluido, true),
		       COALESCE(b.nombre,''), COALESCE(cb.alias,''),
		       COALESCE(co.nombre,''), COALESCE(cl.nombre,''),
		       COALESCE(rs.resolucion,''), COALESCE(rs.respuesta,''),
		       COALESCE(ur.nombre, ur.email, ''), rs.resuelto_en,
		       rs.fecha_esperada, rs.monto_esperado, COALESCE(rs.referencia,'')
		FROM movimiento_reporte_segmentacion rs
		LEFT JOIN movimiento_bancario m ON m.id = rs.movimiento_id
		JOIN usuario u ON u.id = rs.usuario_id
		LEFT JOIN usuario ur ON ur.id = rs.resuelto_por
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE rs.empresa_id = $1::uuid`+cond+`
		ORDER BY rs.resuelto_en NULLS FIRST, rs.creado_en DESC`, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar reportes de segmentación: %w", err)
	}
	defer rows.Close()
	out := []ReporteSegmentacion{}
	for rows.Next() {
		var (
			rep           ReporteSegmentacion
			creado        time.Time
			fecha         *time.Time
			monto         decimal.Decimal
			resueltoEn    *time.Time
			fechaEsperada *time.Time
			montoEsperado decimal.NullDecimal
			movIncluido   bool
		)
		if err := rows.Scan(&rep.ID, &rep.Motivo, &rep.Usuario, &creado,
			&rep.MovimientoID, &fecha, &rep.Descripcion, &monto, &movIncluido,
			&rep.Banco, &rep.Cuenta, &rep.Concepto, &rep.Clasificacion,
			&rep.Resolucion, &rep.Respuesta, &rep.ResueltoPor, &resueltoEn,
			&fechaEsperada, &montoEsperado, &rep.Referencia); err != nil {
			return nil, fmt.Errorf("bancos: scan reporte: %w", err)
		}
		rep.CreadoEn = creado.Format(time.RFC3339)
		if fecha != nil {
			rep.Fecha = fecha.Format("2006-01-02")
		}
		rep.MontoCRC = monto.String()
		// Sin movimiento enganchado no hay nada que marcar: el COALESCE de arriba devolvió true.
		rep.MovExcluido = rep.MovimientoID != "" && !movIncluido
		rep.Pendiente = resueltoEn == nil
		if resueltoEn != nil {
			rep.ResueltoEn = resueltoEn.Format(time.RFC3339)
		}
		// Un aviso de FALTANTE: lo que el equipo esperaba. La pantalla lo muestra en lugar del
		// movimiento cuando no hay movimiento que mostrar.
		rep.EsFaltante = fechaEsperada != nil
		if fechaEsperada != nil {
			rep.FechaEsperada = fechaEsperada.Format("2006-01-02")
		}
		if montoEsperado.Valid {
			rep.MontoEsperado = montoEsperado.Decimal.String()
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

// ResolverReporteSegmentacion cierra el aviso. `resolucion` ya viene validada por el servicio.
func (r *pgRepository) ResolverReporteSegmentacion(ctx context.Context, empresaID, reporteID, usuarioID, resolucion, respuesta string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE movimiento_reporte_segmentacion
		SET resuelto_en = now(), resuelto_por = $3::uuid, resolucion = $4, respuesta = NULLIF($5,'')
		WHERE id = $2::uuid AND empresa_id = $1::uuid AND resuelto_en IS NULL`,
		empresaID, reporteID, usuarioID, resolucion, respuesta)
	if err != nil {
		if idInvalido(err) {
			return ErrReporteNoEncontrado
		}
		return fmt.Errorf("bancos: resolver reporte: %w", err)
	}
	// Cero filas cubre los dos casos y en los dos la respuesta es la misma: no hay nada que
	// resolver. O el reporte no es de esta empresa, o alguien ya lo resolvió mientras esta pantalla
	// estaba abierta —que con tres personas trabajando la misma cola pasa—.
	if ct.RowsAffected() == 0 {
		return ErrReporteNoEncontrado
	}
	return nil
}

// ReportesDeMovimientos dice cuáles de los movimientos dados ya tienen un reporte ABIERTO.
//
// La pantalla del equipo lo usa para no ofrecer «avisar» dos veces sobre el mismo movimiento (el
// índice de un solo aviso abierto por movimiento respondería 409) —y, si el aviso es suyo, para
// mostrarle el motivo que ya escribió, que es lo que evita el tercer aviso idéntico—.
//
// El motivo se recorta ACÁ, en la consulta (ver AvisoAbiertoDeFila): solo sale de la base si el
// aviso es de `usuarioID` y no es un faltante (`fecha_esperada` nula). El de otra persona, o el de un
// faltante que el servidor enganchó a este movimiento, dice que existe y nada más.
func (r *pgRepository) ReportesDeMovimientos(ctx context.Context, empresaID, usuarioID string, movIDs []string) (map[string]AvisoAbiertoDeFila, error) {
	out := map[string]AvisoAbiertoDeFila{}
	if len(movIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT movimiento_id::text,
		       (usuario_id = $3::uuid AND fecha_esperada IS NULL),
		       CASE WHEN usuario_id = $3::uuid AND fecha_esperada IS NULL THEN motivo ELSE '' END
		FROM movimiento_reporte_segmentacion
		WHERE empresa_id = $1::uuid AND resuelto_en IS NULL AND movimiento_id = ANY($2::uuid[])`,
		empresaID, movIDs, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("bancos: reportes de movimientos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id string
			a  AvisoAbiertoDeFila
		)
		if err := rows.Scan(&id, &a.Propio, &a.Motivo); err != nil {
			return nil, fmt.Errorf("bancos: scan reporte de movimiento: %w", err)
		}
		out[id] = a
	}
	return out, rows.Err()
}

// AvisosResueltosDeMovimientos devuelve, para cada movimiento dado, el último aviso RESUELTO que hizo
// `usuarioID` sobre él (sin contar faltantes).
//
// Es la otra mitad de ReportesDeMovimientos: antes, al resolverse un aviso el botón volvía a salir
// y la respuesta no se veía en ningún lado. Solo los de quien pregunta (22-set-2026, decisión
// conservadora hasta que el Director Financiero decida si se abre): el motivo y la respuesta del
// aviso de otra persona son de ella, y un faltante resuelto vive en «Mis avisos».
func (r *pgRepository) AvisosResueltosDeMovimientos(ctx context.Context, empresaID, usuarioID string, movIDs []string) (map[string]AvisoResuelto, error) {
	out := map[string]AvisoResuelto{}
	if len(movIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (movimiento_id)
		       movimiento_id::text, motivo, resolucion, COALESCE(respuesta,''), resuelto_en
		FROM movimiento_reporte_segmentacion
		WHERE empresa_id = $1::uuid AND resuelto_en IS NOT NULL AND movimiento_id = ANY($2::uuid[])
		  AND usuario_id = $3::uuid AND fecha_esperada IS NULL
		ORDER BY movimiento_id, resuelto_en DESC, id`,
		empresaID, movIDs, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("bancos: avisos resueltos de movimientos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id       string
			a        AvisoResuelto
			resuelto time.Time
		)
		if err := rows.Scan(&id, &a.Motivo, &a.Resolucion, &a.Respuesta, &resuelto); err != nil {
			return nil, fmt.Errorf("bancos: scan aviso resuelto: %w", err)
		}
		a.ResueltoEn = resuelto.Format(time.RFC3339)
		out[id] = a
	}
	return out, rows.Err()
}

// MisAvisos devuelve los avisos que hizo ESTE usuario en ESTA empresa, abiertos y resueltos,
// paginados con el total real. `page` y `pageSize` ya vienen normalizados por el servicio.
//
// No se recorta por alcance a propósito: el caso que lo justifica es el aviso resuelto
// reclasificando el movimiento a OTRA partida, que lo saca del alcance justo cuando hay respuesta.
// El recorte es otro y más estricto: `empresa_id` y `usuario_id` salen del token, así que nadie ve
// los avisos de otra persona ni los de otra empresa.
//
// Lo que se trae es lo que el usuario vio al avisar (ver MiAviso), y el JOIN con el movimiento se
// hace SOLO para los avisos sobre un movimiento visto. En un faltante (`fecha_esperada` no nula) el
// movimiento enganchado lo eligió el servidor y el usuario nunca lo vio —era de otra partida—: la
// condición del LEFT JOIN lo deja afuera, así que ni su documento ni su cuenta pueden viajar.
//
// Orden: primero los que siguen en revisión (el más nuevo arriba), después los resueltos (el
// último respondido arriba); desempate por id para que el paginado no repita ni pierda.
func (r *pgRepository) MisAvisos(ctx context.Context, empresaID, usuarioID string, page, pageSize int) (ListaMisAvisos, error) {
	res := ListaMisAvisos{Items: []MiAviso{}, Page: page, PageSize: pageSize}
	err := r.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE resuelto_en IS NULL)
		FROM movimiento_reporte_segmentacion
		WHERE empresa_id = $1::uuid AND usuario_id = $2::uuid`, empresaID, usuarioID).
		Scan(&res.Total, &res.Abiertos)
	if err != nil {
		if idInvalido(err) {
			return res, nil
		}
		return ListaMisAvisos{}, fmt.Errorf("bancos: contar mis avisos: %w", err)
	}
	res.Resueltos = res.Total - res.Abiertos

	rows, err := r.pool.Query(ctx, `
		SELECT rs.id::text, (rs.fecha_esperada IS NOT NULL), rs.motivo, rs.creado_en,
		       COALESCE(m.fecha, rs.fecha_esperada),
		       COALESCE(m.documento,''),
		       COALESCE(m.credito, rs.monto_esperado),
		       COALESCE(m.moneda_original,''),
		       COALESCE(b.nombre,''), COALESCE(cb.alias,''),
		       COALESCE(rs.referencia,''),
		       COALESCE(rs.resolucion,''), COALESCE(rs.respuesta,''), rs.resuelto_en
		FROM movimiento_reporte_segmentacion rs
		LEFT JOIN movimiento_bancario m
		  ON rs.fecha_esperada IS NULL AND m.id = rs.movimiento_id AND m.empresa_id = rs.empresa_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE rs.empresa_id = $1::uuid AND rs.usuario_id = $2::uuid
		ORDER BY (rs.resuelto_en IS NULL) DESC, COALESCE(rs.resuelto_en, rs.creado_en) DESC, rs.id
		LIMIT $3 OFFSET $4`, empresaID, usuarioID, pageSize, (page-1)*pageSize)
	if err != nil {
		return ListaMisAvisos{}, fmt.Errorf("bancos: listar mis avisos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			a          MiAviso
			creado     time.Time
			fecha      *time.Time
			monto      decimal.NullDecimal
			resueltoEn *time.Time
		)
		if err := rows.Scan(&a.ID, &a.EsFaltante, &a.Motivo, &creado,
			&fecha, &a.Documento, &monto, &a.Moneda, &a.Banco, &a.Cuenta, &a.Referencia,
			&a.Resolucion, &a.Respuesta, &resueltoEn); err != nil {
			return ListaMisAvisos{}, fmt.Errorf("bancos: scan mi aviso: %w", err)
		}
		a.CreadoEn = creado.Format(time.RFC3339)
		if fecha != nil {
			a.Fecha = fecha.Format("2006-01-02")
		}
		if monto.Valid {
			a.Monto = monto.Decimal.String()
		}
		a.Estado = AvisoEnRevision
		if resueltoEn != nil {
			a.Estado = AvisoResueltoEstado
			a.ResueltoEn = resueltoEn.Format(time.RFC3339)
		}
		res.Items = append(res.Items, a)
	}
	if err := rows.Err(); err != nil {
		return ListaMisAvisos{}, fmt.Errorf("bancos: iterar mis avisos: %w", err)
	}
	return res, nil
}
