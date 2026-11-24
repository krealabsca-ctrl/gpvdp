package conta

// Consultas del asiento y del enlace bancario (mig 0111). El porqué está en asiento.go.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// ── El enlace cuenta bancaria → cuenta contable ─────────────────────────────────────────────

// filasBancoSQL trae cada cuenta bancaria de la empresa con su cuenta contable, si ya se decidió,
// y con cuántos movimientos vivos la usan.
//
// El conteo sale de una subconsulta y no de un JOIN con GROUP BY: son 20.082 movimientos y 16
// cuentas, así que agrupar para contar obliga a recorrerlos todos.
const filasBancoSQL = `
	SELECT cb.id::text, COALESCE(cb.alias,''), COALESCE(b.nombre,''), COALESCE(cb.moneda,''),
	       COALESCE(cb.iban,''),
	       (SELECT COUNT(*) FROM movimiento_bancario m
	         WHERE m.cuenta_bancaria_id = cb.id AND m.incluido)::int,
	       mcb.cuenta_id::text, COALESCE(cc.codigo,''), COALESCE(cc.nombre,''),
	       EXISTS (SELECT 1 FROM mapeo_cuenta_bancaria o
	                WHERE o.cuenta_id = mcb.cuenta_id AND o.id <> mcb.id
	                  AND o.empresa_id = mcb.empresa_id)
	FROM cuenta_bancaria cb
	LEFT JOIN banco b ON b.id = cb.banco_id
	LEFT JOIN mapeo_cuenta_bancaria mcb
	       ON mcb.cuenta_bancaria_id = cb.id AND mcb.empresa_id = $1::uuid
	LEFT JOIN cuenta_contable cc ON cc.id = mcb.cuenta_id
	WHERE cb.empresa_id = $1::uuid
	ORDER BY 6 DESC, cb.alias`

// FilasBanco lista las cuentas bancarias con su enlace contable.
func (r *pgRepository) FilasBanco(ctx context.Context, empresaID string) ([]FilaBancoContable, error) {
	rows, err := r.pool.Query(ctx, filasBancoSQL, empresaID)
	if err != nil {
		return nil, fmt.Errorf("conta: listar cuentas bancarias: %w", err)
	}
	defer rows.Close()

	out := make([]FilaBancoContable, 0)
	for rows.Next() {
		var f FilaBancoContable
		if err := rows.Scan(&f.CuentaBancariaID, &f.Alias, &f.Banco, &f.Moneda, &f.IBAN,
			&f.Movimientos, &f.CuentaID, &f.CuentaCodigo, &f.CuentaNombre, &f.Compartida); err != nil {
			return nil, fmt.Errorf("conta: scan cuenta bancaria: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GuardarEnlaceBanco crea o reemplaza el enlace de una cuenta bancaria.
func (r *pgRepository) GuardarEnlaceBanco(ctx context.Context, empresaID, cuentaBancariaID, cuentaID, usuarioID string) error {
	const q = `
		INSERT INTO mapeo_cuenta_bancaria (empresa_id, cuenta_bancaria_id, cuenta_id, decidido_por)
		VALUES ($1::uuid, $2::uuid, $3::uuid, NULLIF($4,'')::uuid)
		ON CONFLICT (empresa_id, cuenta_bancaria_id) DO UPDATE
		SET cuenta_id = EXCLUDED.cuenta_id, decidido_por = EXCLUDED.decidido_por,
		    decidido_en = now(), actualizado_en = now()`
	if _, err := r.pool.Exec(ctx, q, empresaID, cuentaBancariaID, cuentaID, usuarioID); err != nil {
		return traducirMapeo(err)
	}
	return nil
}

// BorrarEnlaceBanco deshace el enlace.
func (r *pgRepository) BorrarEnlaceBanco(ctx context.Context, empresaID, cuentaBancariaID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM mapeo_cuenta_bancaria WHERE empresa_id = $1::uuid AND cuenta_bancaria_id = $2::uuid`,
		empresaID, cuentaBancariaID)
	if err != nil {
		if idInvalido(err) {
			return ErrCuentaBancariaNoMapeada
		}
		return fmt.Errorf("conta: borrar el enlace bancario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCuentaBancariaNoMapeada
	}
	return nil
}

// ── El asiento ──────────────────────────────────────────────────────────────────────────────

// CrearAsiento escribe la cabecera y sus líneas en UNA transacción.
//
// TIENE QUE SER UNA SOLA TRANSACCIÓN, y no por prolijidad: el disparador que verifica la partida
// doble es DIFERIDO y corre al COMMIT. Partir la escritura en dos transacciones haría que la
// primera cierre con el asiento a medias, y ahí la base lo rechaza —correctamente—.
func (r *pgRepository) CrearAsiento(ctx context.Context, a Asiento, lineas []LineaValidada, usuarioID string) (Asiento, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Asiento{}, fmt.Errorf("conta: abrir transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El consecutivo sale de un UPDATE … RETURNING: toma el lock de la fila y serializa a los que
	// pidan número a la vez. Un MAX(numero)+1 tendría carrera y el libro saldría con repetidos.
	var numero int64
	const qNum = `
		INSERT INTO conta_consecutivo (empresa_id, ambito, siguiente) VALUES ($1::uuid, 'asiento', 2)
		ON CONFLICT (empresa_id, ambito) DO UPDATE SET siguiente = conta_consecutivo.siguiente + 1
		RETURNING siguiente - 1`
	if err := tx.QueryRow(ctx, qNum, a.EmpresaID).Scan(&numero); err != nil {
		return Asiento{}, fmt.Errorf("conta: tomar el consecutivo: %w", err)
	}

	var id string
	const qCab = `
		INSERT INTO asiento (empresa_id, numero, fecha, glosa, origen_tipo, origen_id, estado, creado_por)
		VALUES ($1::uuid, $2, $3::date, $4, $5, NULLIF($6,'')::uuid, $7, NULLIF($8,'')::uuid)
		RETURNING id::text`
	var origen string
	if a.OrigenID != nil {
		origen = *a.OrigenID
	}
	if err := tx.QueryRow(ctx, qCab, a.EmpresaID, numero, a.Fecha, a.Glosa,
		a.OrigenTipo, origen, a.Estado, usuarioID).Scan(&id); err != nil {
		return Asiento{}, traducirAsiento(err)
	}

	const qLin = `
		INSERT INTO asiento_linea (asiento_id, cuenta_id, debe, haber, detalle, orden)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)`
	for _, l := range lineas {
		if _, err := tx.Exec(ctx, qLin, id, l.CuentaID, l.Debe, l.Haber, l.Detalle, l.Orden); err != nil {
			return Asiento{}, traducirAsiento(err)
		}
	}

	// Acá es donde la base comprueba la partida doble. Un asiento descuadrado falla EN EL COMMIT,
	// no antes.
	if err := tx.Commit(ctx); err != nil {
		return Asiento{}, traducirAsiento(err)
	}
	a.ID = id
	a.Numero = numero
	return a, nil
}

// AnularAsiento lo deja sin efecto. No se borra: borrar deja un hueco en la numeración.
func (r *pgRepository) AnularAsiento(ctx context.Context, empresaID, id, motivo, usuarioID string) error {
	const q = `
		UPDATE asiento SET estado = 'ANULADO', anulado_en = now(),
		       anulado_por = NULLIF($4,'')::uuid, anulado_motivo = $3
		WHERE id = $2::uuid AND empresa_id = $1::uuid AND estado <> 'ANULADO'`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, motivo, usuarioID)
	if err != nil {
		if idInvalido(err) {
			return ErrAsientoNoEncontrado
		}
		return fmt.Errorf("conta: anular el asiento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Dos causas distintas con el mismo síntoma. Se distinguen porque «ya estaba anulado» es
		// información útil y «no existe» es un error del llamador.
		var estado string
		err := r.pool.QueryRow(ctx,
			`SELECT estado FROM asiento WHERE id = $2::uuid AND empresa_id = $1::uuid`,
			empresaID, id).Scan(&estado)
		if err == nil && estado == EstadoAnulado {
			return ErrAsientoYaAnulado
		}
		return ErrAsientoNoEncontrado
	}
	return nil
}

// diarioSQL es el libro diario: los asientos con sus totales, sin las líneas.
//
// Los totales salen de una subconsulta LATERAL y no de un GROUP BY sobre el JOIN: con el JOIN
// habría que agrupar también por todas las columnas de la cabecera, y el filtro por fecha dejaría
// de poder usar el índice.
const diarioSQL = `
	SELECT a.id::text, a.numero, a.fecha::text, a.glosa, a.origen_tipo, a.origen_id::text,
	       a.estado, COALESCE(a.anulado_motivo,''), COALESCE(u.nombre,''), a.creado_en,
	       t.debe::text, t.haber::text
	FROM asiento a
	LEFT JOIN usuario u ON u.id = a.creado_por
	CROSS JOIN LATERAL (
	    SELECT COALESCE(sum(debe),0) AS debe, COALESCE(sum(haber),0) AS haber
	    FROM asiento_linea WHERE asiento_id = a.id
	) t
	WHERE a.empresa_id = $1::uuid`

// Diario lista los asientos.
func (r *pgRepository) Diario(ctx context.Context, empresaID string, f FiltroDiario) ([]Asiento, error) {
	q := diarioSQL
	args := []any{empresaID}
	if f.Desde != "" {
		args = append(args, f.Desde)
		q += fmt.Sprintf(" AND a.fecha >= $%d::date", len(args))
	}
	if f.Hasta != "" {
		args = append(args, f.Hasta)
		q += fmt.Sprintf(" AND a.fecha <= $%d::date", len(args))
	}
	if f.Estado != "" {
		args = append(args, f.Estado)
		q += fmt.Sprintf(" AND a.estado = $%d", len(args))
	}
	if f.OrigenTipo != "" {
		args = append(args, f.OrigenTipo)
		q += fmt.Sprintf(" AND a.origen_tipo = $%d", len(args))
	}
	// Por número descendente: lo último asentado es lo que alguien está mirando.
	q += " ORDER BY a.numero DESC"
	if f.Limite > 0 {
		args = append(args, f.Limite)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if f.Desplaz > 0 {
		args = append(args, f.Desplaz)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		if fechaInvalida(err) {
			return nil, ErrFechaInvalida
		}
		return nil, fmt.Errorf("conta: leer el diario: %w", err)
	}
	defer rows.Close()

	out := make([]Asiento, 0)
	for rows.Next() {
		var a Asiento
		var origen *string
		if err := rows.Scan(&a.ID, &a.Numero, &a.Fecha, &a.Glosa, &a.OrigenTipo, &origen,
			&a.Estado, &a.AnuladoMotivo, &a.CreadoPorNombre, &a.CreadoEn,
			&a.Debe, &a.Haber); err != nil {
			return nil, fmt.Errorf("conta: scan asiento: %w", err)
		}
		a.OrigenID = origen
		a.EmpresaID = empresaID
		out = append(out, a)
	}
	return out, rows.Err()
}

// Asiento devuelve uno con todas sus líneas.
func (r *pgRepository) Asiento(ctx context.Context, empresaID, id string) (Asiento, error) {
	var a Asiento
	var origen *string
	err := r.pool.QueryRow(ctx, diarioSQL+` AND a.id = $2::uuid`, empresaID, id).Scan(
		&a.ID, &a.Numero, &a.Fecha, &a.Glosa, &a.OrigenTipo, &origen,
		&a.Estado, &a.AnuladoMotivo, &a.CreadoPorNombre, &a.CreadoEn, &a.Debe, &a.Haber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || idInvalido(err) {
			return Asiento{}, ErrAsientoNoEncontrado
		}
		return Asiento{}, fmt.Errorf("conta: leer el asiento: %w", err)
	}
	a.OrigenID = origen
	a.EmpresaID = empresaID

	rows, err := r.pool.Query(ctx, `
		SELECT l.id::text, l.cuenta_id::text, c.codigo, c.nombre,
		       l.debe::text, l.haber::text, l.moneda_origen,
		       COALESCE(l.monto_origen::text,''), COALESCE(l.tc_aplicado::text,''),
		       l.detalle, l.orden
		FROM asiento_linea l JOIN cuenta_contable c ON c.id = l.cuenta_id
		WHERE l.asiento_id = $1::uuid ORDER BY l.orden, l.id`, id)
	if err != nil {
		return Asiento{}, fmt.Errorf("conta: leer las líneas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l LineaAsiento
		if err := rows.Scan(&l.ID, &l.CuentaID, &l.CuentaCodigo, &l.CuentaNombre,
			&l.Debe, &l.Haber, &l.MonedaOrigen, &l.MontoOrigen, &l.TCAplicado,
			&l.Detalle, &l.Orden); err != nil {
			return Asiento{}, fmt.Errorf("conta: scan línea: %w", err)
		}
		l.AsientoID = id
		a.Lineas = append(a.Lineas, l)
	}
	return a, rows.Err()
}

// Mayor arma el movimiento de una cuenta con su saldo corrido.
//
// EL SALDO INICIAL SE CALCULA APARTE, sumando todo lo anterior a `desde`. Sin eso, el mayor de un
// mes arrancaría en cero y el saldo final sería el del mes, no el de la cuenta: el número que
// alguien copiaría a un balance estaría mal y parecería bien.
func (r *pgRepository) Mayor(ctx context.Context, empresaID string, f FiltroMayor) (Mayor, error) {
	cuenta, err := r.cuentaPorID(ctx, empresaID, f.CuentaID)
	if err != nil {
		return Mayor{}, err
	}
	m := Mayor{Cuenta: cuenta}

	// Los asientos ANULADOS no cuentan: están en el libro para que se vea que existieron, pero no
	// mueven saldo. Incluirlos duplicaría todo lo que se corrigió.
	const base = `
		FROM asiento_linea l JOIN asiento a ON a.id = l.asiento_id
		WHERE a.empresa_id = $1::uuid AND l.cuenta_id = $2::uuid AND a.estado <> 'ANULADO'`

	inicial := decimal.Zero
	if f.Desde != "" {
		var d, h decimal.Decimal
		err := r.pool.QueryRow(ctx,
			`SELECT COALESCE(sum(l.debe),0), COALESCE(sum(l.haber),0) `+base+` AND a.fecha < $3::date`,
			empresaID, f.CuentaID, f.Desde).Scan(&d, &h)
		if err != nil {
			if fechaInvalida(err) {
				return Mayor{}, ErrFechaInvalida
			}
			return Mayor{}, fmt.Errorf("conta: saldo inicial: %w", err)
		}
		inicial = saldoSegunNaturaleza(d, h, cuenta.Naturaleza)
	}
	m.SaldoInicial = inicial.StringFixed(2)

	q := `SELECT a.id::text, a.numero, a.fecha::text, a.glosa, l.detalle, l.debe, l.haber ` + base
	args := []any{empresaID, f.CuentaID}
	if f.Desde != "" {
		args = append(args, f.Desde)
		q += fmt.Sprintf(" AND a.fecha >= $%d::date", len(args))
	}
	if f.Hasta != "" {
		args = append(args, f.Hasta)
		q += fmt.Sprintf(" AND a.fecha <= $%d::date", len(args))
	}
	q += " ORDER BY a.fecha, a.numero, l.orden"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		if fechaInvalida(err) {
			return Mayor{}, ErrFechaInvalida
		}
		return Mayor{}, fmt.Errorf("conta: leer el mayor: %w", err)
	}
	defer rows.Close()

	saldo := inicial
	totalDebe, totalHaber := decimal.Zero, decimal.Zero
	m.Renglones = make([]RenglonMayor, 0)
	for rows.Next() {
		var g RenglonMayor
		var d, h decimal.Decimal
		if err := rows.Scan(&g.AsientoID, &g.Numero, &g.Fecha, &g.Glosa, &g.Detalle, &d, &h); err != nil {
			return Mayor{}, fmt.Errorf("conta: scan renglón: %w", err)
		}
		totalDebe = totalDebe.Add(d)
		totalHaber = totalHaber.Add(h)
		saldo = saldo.Add(saldoSegunNaturaleza(d, h, cuenta.Naturaleza))
		g.Debe, g.Haber, g.Saldo = d.StringFixed(2), h.StringFixed(2), saldo.StringFixed(2)
		m.Renglones = append(m.Renglones, g)
	}
	if err := rows.Err(); err != nil {
		return Mayor{}, err
	}
	m.TotalDebe = totalDebe.StringFixed(2)
	m.TotalHaber = totalHaber.StringFixed(2)
	m.SaldoFinal = saldo.StringFixed(2)
	return m, nil
}

func (r *pgRepository) cuentaPorID(ctx context.Context, empresaID, id string) (Cuenta, error) {
	var c Cuenta
	err := r.pool.QueryRow(ctx, `
		SELECT c.id::text, c.empresa_id::text, c.codigo, c.nombre, c.clase, c.naturaleza,
		       c.acepta_movimiento, c.nivel
		FROM cuenta_contable c
		WHERE c.id = $2::uuid AND (c.empresa_id = $1::uuid OR c.empresa_id IS NULL)`,
		empresaID, id).Scan(&c.ID, &c.EmpresaID, &c.Codigo, &c.Nombre, &c.Clase,
		&c.Naturaleza, &c.AceptaMovimiento, &c.Nivel)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || idInvalido(err) {
			return Cuenta{}, ErrCuentaNoEncontrada
		}
		return Cuenta{}, fmt.Errorf("conta: leer la cuenta: %w", err)
	}
	return c, nil
}

// fechaInvalida reconoce el error de Postgres al recibir algo que no es fecha.
//
// Sin esto, `?desde=hola` revienta en la base y sale como 500 «error interno», que se registra
// como un incidente del propio sistema. Ya pasó una vez, en el módulo de incidentes.
func fechaInvalida(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		// 22007 invalid_datetime_format · 22008 datetime_field_overflow (p. ej. 2026-02-31)
		return pgErr.SQLState() == "22007" || pgErr.SQLState() == "22008"
	}
	return false
}

// traducirAsiento convierte el rechazo de la base en un error de dominio.
//
// EL ORDEN IMPORTA, por lo mismo que en traducirMapeo: `traducir()` atiende los check_violation
// y devolvería el texto del disparador sin identificarlo, dejando muertos los centinelas de acá.
func traducirAsiento(err error) error {
	msg := errorDePostgres(err)
	switch {
	case strings.Contains(msg, "no cuadra"):
		return noCuadra{msg}
	case strings.Contains(msg, "al debe y otra al haber"):
		return ErrAsientoSinLineas
	case strings.Contains(msg, "agrupa a otras"):
		return deAgrupacion{msg}
	// El disparador del cierre (mig 0113) ya dice el mes y qué hacer. Sin esta rama, su texto
	// llegaba a la pantalla envuelto en «revertir la generación: ERROR: … (SQLSTATE 23514)»,
	// que es ruido alrededor de la única frase útil.
	case strings.Contains(msg, "ya está cerrado"):
		return periodoCerrado{msg}
	}
	if e := traducir(err); !errors.Is(e, err) {
		return e
	}
	if msg != "" {
		return fmt.Errorf("conta: %s", msg)
	}
	return fmt.Errorf("conta: guardar el asiento: %w", err)
}

// noCuadra lleva el texto del disparador —que dice el debe, el haber y la DIFERENCIA— y responde
// que sí a ErrAsientoDescuadrado. Mismo motivo que deAgrupacion: un centinela pelado le quita a
// quien carga el asiento el único dato con el que puede corregirlo.
type noCuadra struct{ msg string }

func (e noCuadra) Error() string { return "conta: " + e.msg }
func (e noCuadra) Unwrap() error { return ErrAsientoDescuadrado }
