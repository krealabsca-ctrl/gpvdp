package cxc

// Notas de crédito: bajar una deuda sin que entre plata.
//
// Se aplican EXACTAMENTE como un cobro —mismo motor FIFO, misma derivación de estado del
// cargo— y por eso todo lo demás del módulo sigue funcionando sin tocar una línea: el saldo,
// los días de mora, el tramo, el valor esperado de la cola y el aging salen de
// (monto − aplicado) y ya cuentan la nota. Como los cobros viven en otra tabla, las métricas
// de RECAUDO no confunden una condonación con dinero recibido.
//
// El usuario definió que las autoriza el supervisor de piso SIN TOPE. Sin límite de monto, el
// control es la trazabilidad: motivo obligatorio, consecutivo propio, y anulación que devuelve
// los cargos a su saldo con su antigüedad original en vez de borrar el documento.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// NotaCreditoInput es una nota a emitir.
type NotaCreditoInput struct {
	Contrato string
	// CargoID opcional: si viene, la nota se aplica a ESE cargo; si no, FIFO al más viejo.
	CargoID string
	Fecha   string
	Monto   decimal.Decimal
	Motivo  string
}

// NotaCredito es una nota emitida, con a qué cargos fue.
type NotaCredito struct {
	ID           string       `json:"id"`
	Consecutivo  string       `json:"consecutivo"`
	Contrato     string       `json:"contrato"`
	Cliente      string       `json:"cliente"`
	Fecha        string       `json:"fecha"`
	Monto        string       `json:"monto"`
	Motivo       string       `json:"motivo"`
	Estado       string       `json:"estado"`
	Aplicaciones []Aplicacion `json:"aplicaciones"`
	// SinAplicar: la parte de la nota que no encontró cargos abiertos. Se informa en vez de
	// rechazarla: puede ser una condonación adelantada de un cargo que todavía no existe.
	SinAplicar      string `json:"sin_aplicar"`
	CreadaPor       string `json:"creada_por"`
	CreadaEn        string `json:"creada_en"`
	AnuladaPor      string `json:"anulada_por"`
	AnuladaEn       string `json:"anulada_en"`
	AnulacionMotivo string `json:"anulacion_motivo"`
}

// ResumenNotas mide lo condonado en el filtro. Con autorización SIN TOPE, este número ES el
// control: nadie puede vigilar un límite que no existe, pero sí puede ver el acumulado.
type ResumenNotas struct {
	Notas    int    `json:"notas"`
	Monto    string `json:"monto"`
	Anuladas int    `json:"anuladas"`
	// PorUsuario: quién condonó cuánto. Es la pregunta que se hace en una auditoría.
	PorUsuario []NotasPorUsuario `json:"por_usuario"`
}

type NotasPorUsuario struct {
	Usuario string `json:"usuario"`
	Notas   int    `json:"notas"`
	Monto   string `json:"monto"`
}

type ListaNotas struct {
	Resumen ResumenNotas  `json:"resumen"`
	Items   []NotaCredito `json:"items"`
	Total   int           `json:"total"`
}

// FiltrosNotas filtra el listado de notas.
type FiltrosNotas struct {
	Contrato string
	Desde    string
	Hasta    string
	// IncluirAnuladas: por omisión no se muestran (una nota anulada no bajó nada).
	IncluirAnuladas bool
	Page, PageSize  int
}

var (
	ErrNotaNoEncontrada = errors.New("cxc: la nota de crédito no existe en esta empresa")
	ErrNotaYaAnulada    = errors.New("cxc: la nota de crédito ya está anulada")
	ErrMotivoRequerido  = errors.New("cxc: la nota de crédito necesita un motivo: es lo único que explica por qué se bajó la deuda")
)

// EmitirNotaCredito escribe la nota y la aplica a los cargos, TODO en una transacción.
func (r *pgRepository) EmitirNotaCredito(ctx context.Context, empresaID string, in NotaCreditoInput, usuarioID string) (NotaCredito, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return NotaCredito{}, fmt.Errorf("cxc: begin nota de crédito: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var contratoID string
	err = tx.QueryRow(ctx,
		`SELECT id::text FROM contrato_cxc WHERE empresa_id = $1::uuid AND numero = $2`,
		empresaID, in.Contrato).Scan(&contratoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return NotaCredito{}, ErrContratoNoEncontrado
	}
	if err != nil {
		return NotaCredito{}, fmt.Errorf("cxc: buscar contrato: %w", err)
	}

	// El consecutivo se calcula bajo advisory lock para que sea por empresa y SIN HUECOS: una
	// serie con saltos en un documento que justifica plata condonada invita a preguntas que
	// después nadie puede contestar. El lock se libera al cerrar la transacción.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('nota_credito:' || $1))`, empresaID); err != nil {
		return NotaCredito{}, fmt.Errorf("cxc: lock del consecutivo: %w", err)
	}
	var consecutivo int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(max(consecutivo), 0) + 1 FROM nota_credito_cxc WHERE empresa_id = $1::uuid`,
		empresaID).Scan(&consecutivo); err != nil {
		return NotaCredito{}, fmt.Errorf("cxc: consecutivo: %w", err)
	}

	// La aplicación usa el MISMO motor que los cobros: si viene un cargo, va a ese; si no,
	// al más viejo primero. Los cargos se leen con FOR UPDATE.
	cargos, err := cargosAbiertosParaAplicar(ctx, tx, empresaID, contratoID)
	if err != nil {
		return NotaCredito{}, err
	}
	var res ResultadoAplicacion
	if in.CargoID != "" {
		res, err = AplicarADestino(in.Monto, cargos, []string{in.CargoID})
	} else {
		res, err = AplicarFIFO(in.Monto, cargos)
	}
	if err != nil {
		return NotaCredito{}, err
	}

	var notaID string
	err = tx.QueryRow(ctx, `
		INSERT INTO nota_credito_cxc (empresa_id, contrato_id, cargo_id, fecha, monto, motivo,
		                              consecutivo, creado_por)
		VALUES ($1::uuid, $2::uuid, NULLIF($3,'')::uuid, $4::date, $5::numeric, $6, $7, NULLIF($8,'')::uuid)
		RETURNING id::text`,
		empresaID, contratoID, in.CargoID, in.Fecha, in.Monto.String(), in.Motivo,
		consecutivo, usuarioID).Scan(&notaID)
	if err != nil {
		return NotaCredito{}, fmt.Errorf("cxc: insertar nota de crédito: %w", err)
	}

	for _, a := range res.Aplicaciones {
		if _, err := tx.Exec(ctx, `
			INSERT INTO nota_credito_aplicacion (empresa_id, nota_id, cargo_id, monto, parcial)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::numeric, $5)`,
			empresaID, notaID, a.CargoID, a.Monto.String(), a.Parcial); err != nil {
			return NotaCredito{}, fmt.Errorf("cxc: insertar aplicación de nota: %w", err)
		}
		// La misma derivación de estado que usan el cobro y su reversa: ir y volver llega al
		// mismo lugar. El CHECK de la tabla impide pasarse del monto del cargo.
		if _, err := tx.Exec(ctx, `
			UPDATE cargo_cxc
			SET monto_aplicado = monto_aplicado + $3::numeric,
			    estado = CASE
			        WHEN monto_aplicado + $3::numeric >= monto THEN 'SALDADO'
			        WHEN monto_aplicado + $3::numeric > 0 THEN 'PARCIAL'
			        ELSE 'ABIERTO' END,
			    actualizado_en = now()
			WHERE empresa_id = $1::uuid AND id = $2::uuid`,
			empresaID, a.CargoID, a.Monto.String()); err != nil {
			return NotaCredito{}, fmt.Errorf("cxc: actualizar cargo: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return NotaCredito{}, fmt.Errorf("cxc: commit nota de crédito: %w", err)
	}
	return r.NotaCredito(ctx, empresaID, notaID)
}

// AnularNotaCredito deshace la nota: los cargos vuelven a su saldo CON SU ANTIGÜEDAD
// ORIGINAL. No se borra el documento — la nota queda marcada con quién la anuló y por qué.
func (r *pgRepository) AnularNotaCredito(ctx context.Context, empresaID, notaID, motivo, usuarioID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cxc: begin anular nota: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var estado string
	err = tx.QueryRow(ctx,
		`SELECT estado FROM nota_credito_cxc WHERE empresa_id = $1::uuid AND id = $2::uuid FOR UPDATE`,
		empresaID, notaID).Scan(&estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotaNoEncontrada
	}
	if err != nil {
		return fmt.Errorf("cxc: leer nota: %w", err)
	}
	if estado == "ANULADA" {
		return ErrNotaYaAnulada
	}

	// Devolver el saldo. `estado` se recalcula con la misma expresión de siempre, así que un
	// cargo que la nota había saldado vuelve a ABIERTO o PARCIAL según lo que quede.
	if _, err := tx.Exec(ctx, `
		UPDATE cargo_cxc g
		SET monto_aplicado = g.monto_aplicado - a.monto,
		    estado = CASE
		        WHEN g.monto_aplicado - a.monto >= g.monto THEN 'SALDADO'
		        WHEN g.monto_aplicado - a.monto > 0 THEN 'PARCIAL'
		        ELSE 'ABIERTO' END,
		    actualizado_en = now()
		FROM nota_credito_aplicacion a
		WHERE a.nota_id = $2::uuid AND a.empresa_id = $1::uuid AND g.id = a.cargo_id`,
		empresaID, notaID); err != nil {
		return fmt.Errorf("cxc: devolver saldo de los cargos: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM nota_credito_aplicacion WHERE nota_id = $1::uuid AND empresa_id = $2::uuid`,
		notaID, empresaID); err != nil {
		return fmt.Errorf("cxc: borrar aplicaciones de la nota: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE nota_credito_cxc
		SET estado = 'ANULADA', anulada_por = NULLIF($3,'')::uuid, anulada_en = now(), anulacion_motivo = $4
		WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, notaID, usuarioID, motivo); err != nil {
		return fmt.Errorf("cxc: anular nota: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cxc: commit anular nota: %w", err)
	}
	return nil
}

const camposNota = `
	n.id::text, COALESCE('NC-' || lpad(n.consecutivo::text, 6, '0'), ''),
	c.numero, c.cliente_nombre, n.fecha::text, n.monto::text, n.motivo, n.estado,
	COALESCE(u.nombre, ''),
	to_char(n.creado_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI'),
	COALESCE(ua.nombre, ''),
	COALESCE(to_char(n.anulada_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI'), ''),
	n.anulacion_motivo,
	COALESCE((SELECT sum(a.monto) FROM nota_credito_aplicacion a WHERE a.nota_id = n.id), 0)::text`

const desdeNota = `
	FROM nota_credito_cxc n
	JOIN contrato_cxc c ON c.id = n.contrato_id
	LEFT JOIN usuario u ON u.id = n.creado_por
	LEFT JOIN usuario ua ON ua.id = n.anulada_por`

func escanearNota(rows pgx.Rows) (NotaCredito, error) {
	var n NotaCredito
	var aplicado string
	if err := rows.Scan(&n.ID, &n.Consecutivo, &n.Contrato, &n.Cliente, &n.Fecha, &n.Monto,
		&n.Motivo, &n.Estado, &n.CreadaPor, &n.CreadaEn, &n.AnuladaPor, &n.AnuladaEn,
		&n.AnulacionMotivo, &aplicado); err != nil {
		return NotaCredito{}, err
	}
	monto, _ := decimal.NewFromString(n.Monto)
	ap, _ := decimal.NewFromString(aplicado)
	n.SinAplicar = monto.Sub(ap).String()
	n.Aplicaciones = []Aplicacion{}
	return n, nil
}

// NotaCredito devuelve una nota con sus aplicaciones.
func (r *pgRepository) NotaCredito(ctx context.Context, empresaID, notaID string) (NotaCredito, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+camposNota+desdeNota+` WHERE n.empresa_id = $1::uuid AND n.id = $2::uuid`,
		empresaID, notaID)
	if err != nil {
		return NotaCredito{}, fmt.Errorf("cxc: leer nota: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return NotaCredito{}, ErrNotaNoEncontrada
	}
	n, err := escanearNota(rows)
	if err != nil {
		return NotaCredito{}, err
	}
	rows.Close()

	aps, err := r.pool.Query(ctx, `
		SELECT a.cargo_id::text, g.periodo, a.monto::text, a.parcial, g.estado
		FROM nota_credito_aplicacion a
		JOIN cargo_cxc g ON g.id = a.cargo_id
		WHERE a.empresa_id = $1::uuid AND a.nota_id = $2::uuid
		ORDER BY g.vence_en`, empresaID, notaID)
	if err != nil {
		return NotaCredito{}, fmt.Errorf("cxc: aplicaciones de la nota: %w", err)
	}
	defer aps.Close()
	for aps.Next() {
		var a Aplicacion
		var monto string
		if err := aps.Scan(&a.CargoID, &a.Periodo, &monto, &a.Parcial, &a.EstadoFinal); err != nil {
			return NotaCredito{}, err
		}
		a.Monto, _ = decimal.NewFromString(monto)
		n.Aplicaciones = append(n.Aplicaciones, a)
	}
	return n, aps.Err()
}

// ListarNotas trae las notas del filtro con el resumen de lo condonado.
func (r *pgRepository) ListarNotas(ctx context.Context, empresaID string, f FiltrosNotas) (ListaNotas, error) {
	conds := []string{"n.empresa_id = $1::uuid"}
	args := []any{empresaID}
	add := func(v any) int { args = append(args, v); return len(args) }
	if !f.IncluirAnuladas {
		conds = append(conds, "n.estado <> 'ANULADA'")
	}
	if f.Contrato != "" {
		conds = append(conds, fmt.Sprintf("c.numero = $%d", add(f.Contrato)))
	}
	if f.Desde != "" {
		conds = append(conds, fmt.Sprintf("n.fecha >= $%d::date", add(f.Desde)))
	}
	if f.Hasta != "" {
		conds = append(conds, fmt.Sprintf("n.fecha <= $%d::date", add(f.Hasta)))
	}
	where := " WHERE " + join(conds, " AND ")

	var res ResumenNotas
	var monto decimal.Decimal
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int, COALESCE(sum(n.monto), 0),
		       count(*) FILTER (WHERE n.estado = 'ANULADA')::int
		`+desdeNota+where, args...).Scan(&res.Notas, &monto, &res.Anuladas); err != nil {
		return ListaNotas{}, fmt.Errorf("cxc: resumen de notas: %w", err)
	}
	res.Monto = monto.String()

	// Quién condonó cuánto: con autorización sin tope, este desglose ES el control.
	res.PorUsuario = []NotasPorUsuario{}
	filas, err := r.pool.Query(ctx, `
		SELECT COALESCE(u.nombre, 'sin usuario'), count(*)::int, COALESCE(sum(n.monto), 0)::text
		`+desdeNota+where+`
		AND n.estado <> 'ANULADA'
		GROUP BY 1 ORDER BY sum(n.monto) DESC`, args...)
	if err != nil {
		return ListaNotas{}, fmt.Errorf("cxc: notas por usuario: %w", err)
	}
	for filas.Next() {
		var u NotasPorUsuario
		if err := filas.Scan(&u.Usuario, &u.Notas, &u.Monto); err != nil {
			filas.Close()
			return ListaNotas{}, err
		}
		res.PorUsuario = append(res.PorUsuario, u)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return ListaNotas{}, err
	}

	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.pool.Query(ctx,
		`SELECT `+camposNota+desdeNota+where+
			` ORDER BY n.fecha DESC, n.consecutivo DESC`+
			fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return ListaNotas{}, fmt.Errorf("cxc: listar notas: %w", err)
	}
	defer rows.Close()
	out := ListaNotas{Resumen: res, Items: []NotaCredito{}, Total: res.Notas}
	for rows.Next() {
		n, err := escanearNota(rows)
		if err != nil {
			return ListaNotas{}, err
		}
		out.Items = append(out.Items, n)
	}
	return out, rows.Err()
}

func join(xs []string, sep string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += sep
		}
		out += x
	}
	return out
}
