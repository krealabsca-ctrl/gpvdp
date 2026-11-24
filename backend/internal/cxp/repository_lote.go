package cxp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CrearLote crea un lote de pago para la fecha de corte y le asigna las facturas indicadas que
// estén PROGRAMADO y sin lote (tenant-safe). Devuelve el lote con número, cantidad y total.
//
// `modo` decide cuántas transferencias salen: una por factura (el de siempre) o una por proveedor
// que cubre todas sus facturas del lote (ver crearRemesas).
func (r *pgRepository) CrearLote(ctx context.Context, empresaID, fechaCorte, modo string, ids []string, usuarioID string) (LotePago, error) {
	if len(ids) == 0 {
		return LotePago{}, ErrSinDocumentos
	}
	if !ModoPagoValido(modo) {
		return LotePago{}, ErrModoPagoInvalido
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return LotePago{}, fmt.Errorf("cxp: begin lote: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lote LotePago
	const insLote = `
		INSERT INTO lote_pago (empresa_id, numero, fecha_corte, creado_por, modo_pago)
		VALUES ($1::uuid,
		        (SELECT COALESCE(MAX(numero), 0) + 1 FROM lote_pago WHERE empresa_id = $1::uuid),
		        $2::date, $3::uuid, $4)
		RETURNING id::text, numero, to_char(fecha_corte, 'YYYY-MM-DD'), estado,
		          to_char(creado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), modo_pago`
	if err := tx.QueryRow(ctx, insLote, empresaID, fechaCorte, usuarioID, modo).
		Scan(&lote.ID, &lote.Numero, &lote.FechaCorte, &lote.Estado, &lote.CreadoEn, &lote.ModoPago); err != nil {
		return LotePago{}, fmt.Errorf("cxp: crear lote: %w", err)
	}

	const asignar = `
		UPDATE documento_cxp SET lote_id = $3::uuid, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND estado = 'PROGRAMADO' AND lote_id IS NULL AND id = ANY($2::uuid[])`
	tag, err := tx.Exec(ctx, asignar, empresaID, ids, lote.ID)
	if err != nil {
		return LotePago{}, fmt.Errorf("cxp: asignar lote: %w", err)
	}
	lote.Cantidad = int(tag.RowsAffected())

	// Lo que se pidió y NO entró. Este UPDATE filtra por estado y por «sin lote», y hasta el
	// 25-set-2026 lo que no calzaba desaparecía sin una palabra: el lote se creaba igual, con cero
	// facturas, y la macro se bajaba vacía. Así nació el lote #17.
	if lote.Cantidad < len(ids) {
		fuera, err := motivosFueraDelLote(ctx, tx, empresaID, ids, lote.ID)
		if err != nil {
			return LotePago{}, err
		}
		// Si no entró NINGUNA, no queda lote: un lote vacío no es un corte, es basura que después
		// hay que distinguir de los buenos. Se aborta y se dice, factura por factura, por qué.
		if lote.Cantidad == 0 {
			return LotePago{}, &LoteVacioError{Fuera: fuera}
		}
		// Parcial: el lote se crea con lo que sí entró —cortar 9 de 10 es trabajo hecho— pero la
		// décima viaja en la respuesta con su motivo, o se paga de menos sin que nadie se entere.
		lote.Fuera = fuera
	}

	// Un pago por proveedor: se agrupan las facturas que SÍ entraron y cada grupo recibe su
	// referencia. Va DENTRO de la transacción del lote porque un lote por proveedor sin sus remesas
	// es un lote que no se puede pagar: la macro no sabría qué escribir.
	if modo == ModoProveedor {
		if err := crearRemesas(ctx, tx, empresaID, lote.ID); err != nil {
			return LotePago{}, err
		}
	}

	// Total del lote = suma de NETOS (descontando anticipos aplicados): es lo que se va a pagar.
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(GREATEST(d.total_crc - COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa WHERE aa.factura_id = d.id AND aa.activo), 0), 0)), 0)::text
		 FROM documento_cxp d WHERE d.lote_id = $1::uuid`,
		lote.ID).Scan(&lote.TotalCRC); err != nil {
		return LotePago{}, fmt.Errorf("cxp: total lote: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return LotePago{}, fmt.Errorf("cxp: commit lote: %w", err)
	}
	return lote, nil
}

// crearRemesas agrupa las facturas YA asignadas al lote: una remesa por proveedor.
//
// Se corre después del UPDATE de asignación y dentro de la misma transacción, así que agrupa lo que
// de verdad entró —no lo que se pidió—. Un proveedor cuya única factura quedó afuera no genera una
// remesa vacía.
//
// El uuid se genera en el SELECT y se usa para las dos cosas (la clave y el código), porque el
// código SE DERIVA del id: así no hay un segundo UPDATE que pueda quedar a medias y dejar remesas
// sin código, que es lo mismo que un pago sin referencia.
//
// El monto sale de netoAPagarSQL, la MISMA expresión que arma el archivo de pago y que verifica la
// conciliación. Si la remesa sumara por su cuenta, el día que cambie la regla del neto el banco
// debitaría una cifra y la conciliación esperaría otra.
func crearRemesas(ctx context.Context, tx pgx.Tx, empresaID, loteID string) error {
	const q = `
		WITH grupos AS (
			SELECT gen_random_uuid() AS id,
			       d.proveedor_id,
			       ROW_NUMBER() OVER (ORDER BY MIN(p.nombre), d.proveedor_id) AS numero,
			       SUM(` + netoAPagarSQL + `) AS monto
			FROM documento_cxp d
			JOIN proveedor p ON p.id = d.proveedor_id
			WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid
			  -- MISMA exclusión que el archivo de pago. Una factura bloqueada puede estar en el
			  -- lote (el UPDATE de asignación filtra por estado, no por el bloqueo) y la macro no
			  -- la paga; si la remesa la sumara, el banco debitaría menos de lo que dice la remesa
			  -- y la conciliación lo marcaría como monto distinto para siempre.
			  AND NOT d.bloqueado_para_pago
			GROUP BY d.proveedor_id
		), insertadas AS (
			INSERT INTO remesa_pago (id, empresa_id, lote_id, proveedor_id, numero, huella, monto_crc)
			SELECT g.id, $1::uuid, $2::uuid, g.proveedor_id, g.numero,
			       'CXPR-' || UPPER(LEFT(REPLACE(g.id::text, '-', ''), 12)), g.monto
			FROM grupos g
			RETURNING id, proveedor_id
		)
		UPDATE documento_cxp d
		   SET remesa_id = i.id, actualizado_en = now()
		  FROM insertadas i
		 WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid AND d.proveedor_id = i.proveedor_id
		   -- Y tampoco se marca la bloqueada: decir que la cubre una remesa que no la sumó sería
		   -- afirmar que se pagó algo que no se pagó.
		   AND NOT d.bloqueado_para_pago`
	if _, err := tx.Exec(ctx, q, empresaID, loteID); err != nil {
		return fmt.Errorf("cxp: crear remesas: %w", err)
	}
	return nil
}

// motivosFueraDelLote dice, factura por factura, por qué una de las pedidas no entró al lote.
//
// Se corre DENTRO de la transacción y después del UPDATE, así que lee el estado ya actualizado: una
// factura recién asignada a ESTE lote no figura, y las que quedaron afuera traen la razón de verdad
// y no una reconstruida de memoria. Un id que no existe —o que es de otra empresa— cae en el
// `LEFT JOIN` y se reporta como inexistente: para esta empresa, no existe.
func motivosFueraDelLote(ctx context.Context, tx pgx.Tx, empresaID string, ids []string, loteID string) ([]DocumentoFueraDelLote, error) {
	const q = `
		SELECT pedido.id::text,
		       COALESCE(d.consecutivo, ''), COALESCE(p.nombre, ''), COALESCE(d.estado, ''),
		       CASE
		         WHEN d.id IS NULL THEN 'no existe en esta empresa'
		         WHEN d.bloqueado_para_pago
		           THEN 'está bloqueada para pago' ||
		                COALESCE(NULLIF(': ' || d.bloqueo_motivo, ': '), '')
		         WHEN d.lote_id IS NOT NULL
		           THEN 'ya está en el lote #' || COALESCE(lp.numero::text, '?')
		         WHEN d.estado <> 'PROGRAMADO'
		           THEN 'está en ' || d.estado || ': solo se puede cortar lo aprobado o lo ya programado'
		         ELSE 'no se pudo programar'
		       END
		FROM unnest($2::uuid[]) AS pedido(id)
		LEFT JOIN documento_cxp d ON d.id = pedido.id AND d.empresa_id = $1::uuid
		LEFT JOIN proveedor p ON p.id = d.proveedor_id
		LEFT JOIN lote_pago lp ON lp.id = d.lote_id
		WHERE d.lote_id IS DISTINCT FROM $3::uuid
		ORDER BY 3, 2`
	rows, err := tx.Query(ctx, q, empresaID, ids, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: motivos fuera del lote: %w", err)
	}
	defer rows.Close()
	fuera := []DocumentoFueraDelLote{}
	for rows.Next() {
		var f DocumentoFueraDelLote
		if err := rows.Scan(&f.DocumentoID, &f.Consecutivo, &f.Proveedor, &f.Estado, &f.Motivo); err != nil {
			return nil, fmt.Errorf("cxp: scan motivo fuera del lote: %w", err)
		}
		fuera = append(fuera, f)
	}
	return fuera, rows.Err()
}

// ListarLotes lista los lotes de la empresa con cantidad y total (más reciente primero).
func (r *pgRepository) ListarLotes(ctx context.Context, empresaID string) ([]LotePago, error) {
	const q = `
		SELECT l.id::text, l.numero, to_char(l.fecha_corte, 'YYYY-MM-DD'), l.estado,
		       COUNT(d.id)::int,
		       COALESCE(SUM(GREATEST(d.total_crc - COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa WHERE aa.factura_id = d.id AND aa.activo), 0), 0)), 0)::text,
		       to_char(l.creado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       COUNT(d.id) FILTER (WHERE d.estado IN ('PAGADO', 'CONCILIADO'))::int,
		       COUNT(d.id) FILTER (WHERE d.estado = 'REBOTADA')::int,
		       COUNT(d.id) FILTER (WHERE d.estado = 'PROGRAMADO')::int,
		       -- El corte (mig 0104). Van como texto vacío y no como NULL para que el cliente no
		       -- tenga que distinguir dos formas de «no está cerrado».
		       COALESCE(to_char(l.cerrado_en,   'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		       COALESCE(to_char(l.reabierto_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		       COALESCE(l.reapertura_motivo, '')
		FROM lote_pago l
		LEFT JOIN documento_cxp d ON d.lote_id = l.id
		WHERE l.empresa_id = $1::uuid
		GROUP BY l.id, l.numero, l.fecha_corte, l.estado, l.creado_en,
		         l.cerrado_en, l.reabierto_en, l.reapertura_motivo
		ORDER BY l.creado_en DESC`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar lotes: %w", err)
	}
	defer rows.Close()
	out := make([]LotePago, 0)
	for rows.Next() {
		var l LotePago
		if err := rows.Scan(&l.ID, &l.Numero, &l.FechaCorte, &l.Estado, &l.Cantidad, &l.TotalCRC, &l.CreadoEn,
			&l.Pagadas, &l.Rebotadas, &l.Pendientes,
			&l.CerradoEn, &l.ReabiertoEn, &l.ReaperturaMotivo); err != nil {
			return nil, fmt.Errorf("cxp: scan lote: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DocumentosParaPagoPorLote devuelve las líneas de pago (para la macro) del lote.
//
// UNA LÍNEA = UNA TRANSFERENCIA, no una factura. En el modo por proveedor eso significa una línea
// por remesa, con el monto agrupado: es la razón de ser del modo. Si acá se devolviera una línea
// por factura, el lote diría «un pago por proveedor» y el banco haría cinco igual.
func (r *pgRepository) DocumentosParaPagoPorLote(ctx context.Context, empresaID, loteID string) ([]PagoRow, error) {
	modo, numero, err := r.modoYNumeroDeLote(ctx, empresaID, loteID)
	if err != nil {
		return nil, err
	}
	if modo == ModoProveedor {
		return r.pagosPorRemesa(ctx, empresaID, loteID, numero)
	}
	// El monto que va al banco es el NETO: total − retención − anticipos aplicados. Sin restar
	// los anticipos se le pagaría al proveedor el total de una factura ya neteada (doble pago).
	const q = `
		SELECT COALESCE(p.identificacion, ''), p.nombre, COALESCE(p.iban, ''), d.moneda,
		       GREATEST(d.total - d.retencion - COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa WHERE aa.factura_id = d.id AND aa.activo), 0), 0)::text,
		       COALESCE(d.huella, ''), COALESCE(d.consecutivo, ''), d.id::text,
		       -- La nomenclatura interna («A8», «A9»): sale del catálogo de gasto de la factura.
		       ` + codigoPagoSQL + `,
		       p.id::text
		FROM documento_cxp d
		JOIN proveedor p ON p.id = d.proveedor_id
		LEFT JOIN concepto c ON c.id = d.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = d.clasificacion_id
		WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid
		  AND NOT d.bloqueado_para_pago
		ORDER BY p.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: documentos de lote: %w", err)
	}
	defer rows.Close()
	var out []PagoRow
	for rows.Next() {
		var pr PagoRow
		if err := rows.Scan(&pr.Cedula, &pr.Nombre, &pr.IBAN, &pr.Moneda, &pr.MontoNeto,
			&pr.Descripcion, &pr.Consecutivo, &pr.DocumentoID, &pr.CodigoPago, &pr.ProveedorID); err != nil {
			return nil, fmt.Errorf("cxp: scan pago lote: %w", err)
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// modoYNumeroDeLote lee cómo se armó el lote y su número. El número hace falta para la referencia
// legible de la remesa («REM 0043-07»), que es lo que una persona busca en el estado de cuenta.
func (r *pgRepository) modoYNumeroDeLote(ctx context.Context, empresaID, loteID string) (string, int64, error) {
	var modo string
	var numero int64
	err := r.pool.QueryRow(ctx,
		`SELECT modo_pago, numero FROM lote_pago WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, loteID).Scan(&modo, &numero)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrLoteNoEncontrado
	}
	if err != nil {
		return "", 0, fmt.Errorf("cxp: modo del lote: %w", err)
	}
	return modo, numero, nil
}

// pagosPorRemesa arma la macro de un lote agrupado: una línea por proveedor.
//
// La remesa SIN facturas vivas no se paga. Puede pasar si entre el corte y la bajada de la macro
// alguien anuló lo único que cubría: transferir igual sería mandar plata sin nada que la respalde.
func (r *pgRepository) pagosPorRemesa(ctx context.Context, empresaID, loteID string, numeroLote int64) ([]PagoRow, error) {
	const q = `
		SELECT COALESCE(p.identificacion, ''), p.nombre, COALESCE(p.iban, ''),
		       -- La moneda sale de las facturas que cubre. El modo agrupado exige que sean todas la
		       -- misma: un pago no puede salir mitad en colones y mitad en dólares.
		       MIN(d.moneda), r.monto_crc::text, r.huella, r.numero,
		       -- La nomenclatura de un pago AGRUPADO solo existe si todas las facturas que cubre
		       -- comparten el mismo código. Si mezcla A8 y A9, el pago no es ni una cosa ni la
		       -- otra y va sin código: etiquetarlo con el de la primera diría que ese débito es
		       -- todo comisiones cuando la mitad es planilla, que es justo lo que se quiere evitar.
		       CASE WHEN COUNT(DISTINCT ` + codigoPagoSQL + `) = 1
		            THEN MIN(` + codigoPagoSQL + `) ELSE '' END,
		       p.id::text
		FROM remesa_pago r
		JOIN proveedor p ON p.id = r.proveedor_id
		JOIN documento_cxp d ON d.remesa_id = r.id AND NOT d.bloqueado_para_pago
		LEFT JOIN concepto c ON c.id = d.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = d.clasificacion_id
		WHERE r.empresa_id = $1::uuid AND r.lote_id = $2::uuid
		GROUP BY r.id, r.numero, r.huella, r.monto_crc, p.id, p.identificacion, p.nombre, p.iban
		HAVING COUNT(d.id) > 0
		ORDER BY r.numero`
	rows, err := r.pool.Query(ctx, q, empresaID, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: pagos por remesa: %w", err)
	}
	defer rows.Close()
	var out []PagoRow
	for rows.Next() {
		var pr PagoRow
		var numero int
		if err := rows.Scan(&pr.Cedula, &pr.Nombre, &pr.IBAN, &pr.Moneda, &pr.MontoNeto,
			&pr.Descripcion, &numero, &pr.CodigoPago, &pr.ProveedorID); err != nil {
			return nil, fmt.Errorf("cxp: scan pago por remesa: %w", err)
		}
		// La referencia legible reemplaza al «FC 000487» de una factura suelta: acá no hay UNA
		// factura que nombrar, y nombrar solo a la primera sería mentir sobre lo que cubre el pago.
		pr.Referencia = ReferenciaRemesa(numeroLote, numero)
		out = append(out, pr)
	}
	return out, rows.Err()
}

// ProgramarAprobados programa en bloque las facturas APROBADAS del corte (fecha de pago = corte)
// y les genera su huella. Se usa al crear el lote para que "aprobada" pueda ir directo al corte.
func (r *pgRepository) ProgramarAprobados(ctx context.Context, empresaID string, ids []string, fecha string) (int64, error) {
	const q = `
		UPDATE documento_cxp
		SET estado = 'PROGRAMADO', fecha_pago_programada = $3::date,
		    huella = 'CXP-' || UPPER(LEFT(REPLACE(id::text, '-', ''), 12)), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = ANY($2::uuid[]) AND estado = 'APROBADO'
		  AND NOT bloqueado_para_pago`
	tag, err := r.pool.Exec(ctx, q, empresaID, ids, fecha)
	if err != nil {
		return 0, fmt.Errorf("cxp: programar aprobados: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Reintentar saca una factura REBOTADA de su lote y la vuelve a PROGRAMADO (para un nuevo corte).
//
// LA GUARDA DEL LOTE CERRADO VA EN EL WHERE, no en una lectura previa (mig 0104). Leer el estado
// del lote y después actualizar deja una ventana entre las dos consultas: si alguien cierra el
// lote justo ahí, la factura sale igual de un lote cerrado y el corte deja de cuadrar con la
// macro que se subió al banco. Es la misma razón por la que la guarda de excluir un movimiento
// vive en su WHERE.
func (r *pgRepository) Reintentar(ctx context.Context, empresaID, id string) (int64, error) {
	const q = `UPDATE documento_cxp d SET estado = 'PROGRAMADO', lote_id = NULL, actualizado_en = now()
	           WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid AND d.estado = 'REBOTADA'
	             AND NOT EXISTS (SELECT 1 FROM lote_pago l
	                             WHERE l.id = d.lote_id AND l.estado = 'CERRADO')`
	tag, err := r.pool.Exec(ctx, q, empresaID, id)
	if err != nil {
		return 0, fmt.Errorf("cxp: reintentar: %w", err)
	}
	return tag.RowsAffected(), nil
}

// LoteDeDocumentoEstaCerrado dice si la factura cuelga de un lote cerrado.
//
// Existe solo para EXPLICAR un reintento que no movió nada. La decisión ya la tomó el WHERE de
// arriba; esto contesta «¿por qué no se movió?», porque «0 filas» se ve igual cuando la factura no
// está rebotada que cuando su lote está cerrado, y son dos arreglos distintos.
func (r *pgRepository) LoteDeDocumentoEstaCerrado(ctx context.Context, empresaID, id string) (bool, error) {
	var cerrado bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM documento_cxp d
		               JOIN lote_pago l ON l.id = d.lote_id
		               WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid AND l.estado = 'CERRADO')`,
		empresaID, id).Scan(&cerrado)
	if err != nil {
		return false, fmt.Errorf("cxp: lote del documento cerrado: %w", err)
	}
	return cerrado, nil
}

// CerrarLote da el corte: ABIERTO → CERRADO.
//
// El estado va en el WHERE para que cerrar dos veces no pise la fecha ni el autor del cierre
// bueno. Devuelve 0 filas si el lote ya estaba cerrado o no existe, y el servicio distingue cuál
// de las dos cosas fue.
func (r *pgRepository) CerrarLote(ctx context.Context, empresaID, loteID, usuarioID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lote_pago SET estado = 'CERRADO', cerrado_por = $3::uuid, cerrado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'ABIERTO'`,
		empresaID, loteID, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("cxp: cerrar lote: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ReabrirLote vuelve CERRADO → ABIERTO dejando el motivo.
//
// NO se borra `cerrado_en` ni `cerrado_por`: que el lote estuvo cerrado alguna vez es parte de lo
// que hay que poder reconstruir. Lo que el lote informa después de esto es «se cerró el día X y se
// reabrió el día Y porque Z».
func (r *pgRepository) ReabrirLote(ctx context.Context, empresaID, loteID, usuarioID, motivo string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lote_pago
		SET estado = 'ABIERTO', reabierto_por = $3::uuid, reabierto_en = now(), reapertura_motivo = $4
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'CERRADO'`,
		empresaID, loteID, usuarioID, motivo)
	if err != nil {
		return 0, fmt.Errorf("cxp: reabrir lote: %w", err)
	}
	return tag.RowsAffected(), nil
}

// LoteEstaCerrado explica un cierre/reapertura que no movió nada: distingue «no existe» de «ya
// estaba en ese estado».
func (r *pgRepository) LoteEstaCerrado(ctx context.Context, empresaID, loteID string) (existe, cerrado bool, err error) {
	var estado string
	err = r.pool.QueryRow(ctx,
		`SELECT estado FROM lote_pago WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, loteID).Scan(&estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("cxp: estado del lote: %w", err)
	}
	return true, estado == "CERRADO", nil
}
