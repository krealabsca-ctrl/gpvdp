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

// CuentasDelAlcance devuelve las cuentas donde el segmento recibe plata, para el filtro.
//
// Sale de los movimientos del propio alcance: así el desplegable ofrece solo lo que el equipo puede
// ver, y no hace falta darle acceso al catálogo de cuentas de la empresa.
func (r *pgRepository) CuentasDelAlcance(ctx context.Context, empresaID string, alcance []string) ([]CuentaDelSegmento, error) {
	if len(alcance) == 0 {
		return []CuentaDelSegmento{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT cb.id::text, COALESCE(b.nombre,''), COALESCE(cb.alias,'')
		FROM movimiento_bancario m
		JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE m.empresa_id = $1::uuid AND m.credito > 0 AND m.clasificacion_id = ANY($2::uuid[])
		ORDER BY 2, 3`, empresaID, alcance)
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
func (r *pgRepository) UltimaFechaCargada(ctx context.Context, empresaID string) (string, error) {
	var fecha *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT max(fecha) FROM movimiento_bancario WHERE empresa_id = $1::uuid`, empresaID).Scan(&fecha)
	if err != nil {
		return "", fmt.Errorf("bancos: última fecha cargada: %w", err)
	}
	if fecha == nil {
		return "", nil
	}
	return fecha.Format("2006-01-02"), nil
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

// MovimientoEnAlcance dice si el movimiento existe en la empresa Y está en el alcance dado.
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
// Devuelve por separado los que están en el alcance del rol —que se pueden mostrar completos— y
// cuántos hay FUERA de él, sin traer ni un dato de esos: la existencia es todo lo que se divulga.
//
// El monto se compara contra `credito` y no contra `monto_crc`: el equipo tiene el recibo del
// depósito en colones tal como lo hizo, y `monto_crc` de una cuenta en dólares es una conversión
// que nunca va a coincidir con lo que la persona escribe.
func (r *pgRepository) BuscarPorFechaYMonto(
	ctx context.Context, empresaID, fecha string, monto decimal.Decimal, alcance []string,
) (mios []MovimientoRow, fuera int, err error) {
	// El alcance vacío ni se consulta: sin partidas asignadas no hay pantalla desde donde buscar.
	if len(alcance) == 0 {
		return nil, 0, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT m.id::text, m.fecha, COALESCE(m.documento,''), COALESCE(m.descripcion,''),
		       m.debito, m.credito, m.moneda_original, m.monto_crc,
		       m.concepto_id::text, COALESCE(co.nombre,''),
		       m.clasificacion_id::text, COALESCE(cl.nombre,''),
		       m.estado_clasificacion, m.es_traslado,
		       COALESCE(b.nombre,''), COALESCE(cb.alias,''),
		       (m.clasificacion_id = ANY($4::uuid[])) AS es_mio
		FROM movimiento_bancario m
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE m.empresa_id = $1::uuid AND m.fecha = $2::date AND m.credito = $3
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
			esMio     bool
		)
		if err := rows.Scan(&row.ID, &f, &row.Documento, &row.Descripcion,
			&deb, &cred, &row.Moneda, &mcrc,
			&row.ConceptoID, &row.Concepto, &row.ClasificacionID, &row.Clasificacion,
			&row.Estado, &row.EsTraslado, &row.Banco, &row.Cuenta, &esMio); err != nil {
			return nil, 0, fmt.Errorf("bancos: scan búsqueda de faltante: %w", err)
		}
		// Lo ajeno se CUENTA y se descarta acá mismo, en el repositorio: así no queda un
		// `MovimientoRow` con datos de otra partida viajando por el servicio, donde un descuido
		// futuro lo podría serializar.
		if !esMio {
			fuera++
			continue
		}
		row.Fecha = f.Format("2006-01-02")
		row.Debito = deb.String()
		row.Credito = cred.String()
		row.MontoCRC = mcrc.String()
		mios = append(mios, row)
	}
	return mios, fuera, rows.Err()
}

// EngancharFaltante busca el ÚNICO crédito de esa fecha y ese monto en la empresa.
//
// Sirve para que el aviso de faltante llegue con el movimiento ya identificado y quien clasifica no
// lo tenga que buscar. Si hay más de uno devuelve "" a propósito: dos depósitos idénticos el mismo
// día son indistinguibles y adivinar cuál es sería peor que no enganchar ninguno.
func (r *pgRepository) EngancharFaltante(ctx context.Context, empresaID, fecha string, monto decimal.Decimal) (string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text FROM movimiento_bancario
		WHERE empresa_id = $1::uuid AND fecha = $2::date AND credito = $3
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
	rows, err := r.pool.Query(ctx, `
		SELECT rs.id::text, rs.motivo, COALESCE(u.nombre, u.email), rs.creado_en,
		       COALESCE(m.id::text,''), m.fecha, COALESCE(m.descripcion,''), COALESCE(m.monto_crc,0),
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
		)
		if err := rows.Scan(&rep.ID, &rep.Motivo, &rep.Usuario, &creado,
			&rep.MovimientoID, &fecha, &rep.Descripcion, &monto,
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
// La pantalla del equipo lo usa para no ofrecer «avisar» dos veces sobre el mismo movimiento —y
// para mostrar el motivo que ya se escribió, que es lo que evita el tercer aviso idéntico—.
func (r *pgRepository) ReportesDeMovimientos(ctx context.Context, empresaID string, movIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(movIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT movimiento_id::text, motivo
		FROM movimiento_reporte_segmentacion
		WHERE empresa_id = $1::uuid AND resuelto_en IS NULL AND movimiento_id = ANY($2::uuid[])`,
		empresaID, movIDs)
	if err != nil {
		return nil, fmt.Errorf("bancos: reportes de movimientos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, motivo string
		if err := rows.Scan(&id, &motivo); err != nil {
			return nil, fmt.Errorf("bancos: scan reporte de movimiento: %w", err)
		}
		out[id] = motivo
	}
	return out, rows.Err()
}
