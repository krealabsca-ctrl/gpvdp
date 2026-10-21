package cxp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// GuardarComprobante guarda (o reemplaza) el comprobante de una factura PAGADA/CONCILIADA
// de la empresa. Devuelve ErrDocNoPagado si el documento no es elegible.
//
// AL REEMPLAZAR, BORRA LA MARCA DE ENVIADO. `documento_cxp.comprobante_enviado_en` significa «el
// proveedor ya tiene el PDF que está adjunto AHORA»; si se sube otro archivo, esa afirmación deja
// de ser cierta y la pantalla mostraba «Enviado» sobre un comprobante que nadie recibió. El
// histórico del envío viejo no se pierde: vive en `comprobante_envio`.
//
// Las dos escrituras van en UNA transacción. Si el adjunto se reemplazara y la marca quedara, el
// defecto vuelve; y si se borrara la marca sin llegar a guardar el archivo, la factura perdería su
// envío por nada.
// `tipo` distingue los dos documentos del expediente (mig 0103): COMPROBANTE es el recibo del
// banco y FACTURA el documento del proveedor. La unicidad es por (documento, tipo), así que
// adjuntar uno NO pisa al otro.
func (r *pgRepository) GuardarComprobante(ctx context.Context, empresaID, docID, tipo, filename, mime string, contenido []byte, usuarioID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cxp: guardar comprobante: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// LA FACTURA SE PUEDE ADJUNTAR EN CUALQUIER ESTADO; el comprobante solo si ya se pagó.
	//
	// No es una excepción: son documentos de momentos distintos. La factura llega al principio y
	// es lo que se revisa para decidir el pago —exigir que ya esté pagada para poder verla deja
	// sin respaldo justo el tramo donde se aprueba—. El recibo del banco, en cambio, no existe
	// antes de que el dinero salga, y aceptarlo antes sería aceptar un papel que no puede existir.
	const qArchivo = `
		INSERT INTO comprobante_pago (empresa_id, documento_id, tipo, filename, mime, contenido, subido_por)
		SELECT $1::uuid, $2::uuid, $7, $3, $4, $5, $6::uuid
		WHERE EXISTS (
			SELECT 1 FROM documento_cxp
			WHERE id = $2::uuid AND empresa_id = $1::uuid
			  AND ($7 = 'FACTURA' OR estado IN ('PAGADO', 'CONCILIADO')))
		ON CONFLICT (documento_id, tipo) DO UPDATE
			SET filename = EXCLUDED.filename, mime = EXCLUDED.mime, contenido = EXCLUDED.contenido,
			    subido_por = EXCLUDED.subido_por, subido_en = now()`
	tag, err := tx.Exec(ctx, qArchivo, empresaID, docID, filename, mime, contenido, usuarioID, tipo)
	if err != nil {
		return fmt.Errorf("cxp: guardar comprobante: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDocNoPagado
	}

	// La marca de enviado solo la borra el COMPROBANTE. Significa «el proveedor ya tiene el recibo
	// que está adjunto ahora»; adjuntar la factura no la vuelve mentira, y borrarla ahí marcaría
	// como no enviado un comprobante que sí salió.
	if tipo != AdjuntoFactura {
		const qMarca = `UPDATE documento_cxp SET comprobante_enviado_en = NULL, actualizado_en = now()
		                WHERE empresa_id = $1::uuid AND id = $2::uuid AND comprobante_enviado_en IS NOT NULL`
		if _, err := tx.Exec(ctx, qMarca, empresaID, docID); err != nil {
			return fmt.Errorf("cxp: limpiar la marca de enviado: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cxp: guardar comprobante: %w", err)
	}
	return nil
}

// ObtenerComprobante devuelve uno de los dos archivos adjuntos de una factura (para descargar).
func (r *pgRepository) ObtenerComprobante(ctx context.Context, empresaID, docID, tipo string) (Comprobante, error) {
	const q = `SELECT filename, mime, contenido FROM comprobante_pago
	           WHERE empresa_id = $1::uuid AND documento_id = $2::uuid AND tipo = $3`
	var c Comprobante
	err := r.pool.QueryRow(ctx, q, empresaID, docID, tipo).Scan(&c.Filename, &c.Mime, &c.Contenido)
	if errors.Is(err, pgx.ErrNoRows) {
		return Comprobante{}, ErrComprobanteNoEncontrado
	}
	if err != nil {
		return Comprobante{}, fmt.Errorf("cxp: obtener comprobante: %w", err)
	}
	return c, nil
}

// ObtenerComprobanteEnvio trae el adjunto + los datos del proveedor, la factura y el aprobador.
//
// EL APROBADOR sale de `documento_cxp_aprobacion` unida a `usuario`, y es un LEFT JOIN a propósito:
// una factura sin aprobación registrada (o con un aprobador sin correo) tiene que poder mandarse
// igual, sin copia. Si fuera un JOIN normal, el comprobante dejaría de salir por una copia que es
// accesoria.
//
// Se toma EL ÚLTIMO por `aprobado_en`. Hoy los 99 documentos aprobados del sistema tienen
// exactamente un aprobador, así que la regla nunca se ejerció; está pendiente de confirmación qué
// hacer el día que una factura lleve firma mancomunada (¿se copia a todos o solo al último?).
func (r *pgRepository) ObtenerComprobanteEnvio(ctx context.Context, empresaID, docID string) (ComprobanteEnvio, error) {
	const q = `
		SELECT cp.filename, cp.mime, cp.contenido, cp.subido_en,
		       COALESCE(p.email, ''), p.nombre, COALESCE(d.consecutivo, ''), d.total_crc::text,
		       d.moneda, d.total::text, COALESCE(d.huella, ''), COALESCE(d.descripcion, ''),
		       COALESCE(ap.email, ''), COALESCE(ap.nombre, ''),
		       COALESCE(to_char(d.fecha_emision, 'DD/MM/YYYY'), ''),
		       COALESCE(p.telefono, ''),
		       -- LA REFERENCIA QUE DE VERDAD VIAJA AL BANCO (19-oct-2026).
		       --
		       -- Si la factura se pagó dentro de una REMESA, lo que vuelve en el estado de cuenta es
		       -- el código de la remesa, no el de la factura. El correo mandaba el de la factura, así
		       -- que el proveedor buscaba un código que su banco nunca trae — y esa frase («la
		       -- referencia con la que aparece en el banco es…») existe justamente para que lo
		       -- encuentre. Medido: 9 facturas dentro de remesa, todas con huella propia distinta a
		       -- la de su remesa.
		       COALESCE(rcxp.huella, d.huella, ''),
		       CASE WHEN rcxp.id IS NOT NULL THEN ` + referenciaRemesaSQL + ` ELSE '' END,
		       -- Cuántas facturas cubre el pago: decide si el correo habla en singular o lista.
		       CASE WHEN rcxp.id IS NOT NULL
		            THEN (SELECT count(*) FROM documento_cxp dr WHERE dr.remesa_id = rcxp.id)
		            ELSE 1 END
		FROM comprobante_pago cp
		JOIN documento_cxp d ON d.id = cp.documento_id
		JOIN proveedor p ON p.id = d.proveedor_id
		LEFT JOIN remesa_pago rcxp ON rcxp.id = d.remesa_id
		LEFT JOIN lote_pago lrem ON lrem.id = rcxp.lote_id
		LEFT JOIN LATERAL (
			SELECT u.email, u.nombre
			FROM documento_cxp_aprobacion a
			JOIN usuario u ON u.id = a.usuario_id
			WHERE a.documento_id = cp.documento_id AND a.empresa_id = cp.empresa_id
			ORDER BY a.aprobado_en DESC
			LIMIT 1
		) ap ON true
		WHERE cp.empresa_id = $1::uuid AND cp.documento_id = $2::uuid`
	var e ComprobanteEnvio
	err := r.pool.QueryRow(ctx, q, empresaID, docID).
		Scan(&e.Filename, &e.Mime, &e.Contenido, &e.SubidoEn, &e.ProveedorEmail, &e.ProveedorNombre,
			&e.Consecutivo, &e.TotalCRC, &e.Moneda, &e.Total, &e.Huella, &e.Descripcion,
			&e.AprobadorEmail, &e.AprobadorNombre,
			&e.FechaEmision, &e.ProveedorTelefono,
			&e.HuellaQueViaja, &e.ReferenciaRemesa, &e.FacturasDelPago)
	if errors.Is(err, pgx.ErrNoRows) {
		return ComprobanteEnvio{}, ErrComprobanteNoEncontrado
	}
	if err != nil {
		return ComprobanteEnvio{}, fmt.Errorf("cxp: comprobante para envío: %w", err)
	}
	// Las facturas hermanas solo se piden cuando el pago cubre más de una: en el caso normal —una
	// factura, un pago— sería una consulta de más en cada envío.
	if e.FacturasDelPago > 1 {
		if e.Facturas, err = r.facturasDelPago(ctx, empresaID, docID); err != nil {
			return ComprobanteEnvio{}, err
		}
	}
	return e, nil
}

// GuardarTelefonoProveedorDeDocumento deja el número en la ficha del proveedor de esa factura.
//
// Solo lo escribe si estaba VACÍO: el número de la ficha es el contacto oficial y un envío suelto
// —a un celular que alguien pasó por ahí— no debería reemplazarlo sin que nadie lo decida.
func (r *pgRepository) GuardarTelefonoProveedorDeDocumento(ctx context.Context, empresaID, docID, telefono string) error {
	const q = `
		UPDATE proveedor p
		   SET telefono = $3, actualizado_en = now()
		  FROM documento_cxp d
		 WHERE d.id = $2::uuid AND d.empresa_id = $1::uuid
		   AND p.id = d.proveedor_id AND p.empresa_id = $1::uuid
		   AND COALESCE(p.telefono, '') = ''`
	if _, err := r.pool.Exec(ctx, q, empresaID, docID, telefono); err != nil {
		return fmt.Errorf("cxp: guardar teléfono del proveedor: %w", err)
	}
	return nil
}

// facturasDelPago lista TODAS las facturas que cubre el mismo pago agrupado, con lo que el
// proveedor necesita para reconocerlas en sus propios registros.
//
// Sin esto, el correo de un pago de cinco facturas decía «su factura fue pagada» en singular y sin
// decir cuál: el proveedor recibía un monto que no calzaba con ninguna de las suyas.
func (r *pgRepository) facturasDelPago(ctx context.Context, empresaID, docID string) ([]FacturaDelPago, error) {
	const q = `
		SELECT COALESCE(d.consecutivo, ''), COALESCE(to_char(d.fecha_emision, 'DD/MM/YYYY'), ''),
		       d.total::text, d.moneda
		FROM documento_cxp d
		WHERE d.empresa_id = $1::uuid
		  AND d.remesa_id = (SELECT remesa_id FROM documento_cxp WHERE id = $2::uuid)
		ORDER BY d.fecha_emision, d.consecutivo`
	rows, err := r.pool.Query(ctx, q, empresaID, docID)
	if err != nil {
		return nil, fmt.Errorf("cxp: facturas del pago: %w", err)
	}
	defer rows.Close()
	out := make([]FacturaDelPago, 0, 8)
	for rows.Next() {
		var f FacturaDelPago
		if err := rows.Scan(&f.Consecutivo, &f.FechaEmision, &f.Total, &f.Moneda); err != nil {
			return nil, fmt.Errorf("cxp: scan factura del pago: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// RegistrarEnvio escribe UNA fila de bitácora por intento —incluidos los que fallaron— y, solo si
// salió bien, mueve `documento_cxp.comprobante_enviado_en`.
//
// Las dos cosas van en la MISMA transacción porque responden a la misma pregunta desde dos lados:
// si la marca se moviera sin quedar la fila, la pantalla diría «enviado» sin poder decir a quién; y
// si quedara la fila OK sin mover la marca, el botón seguiría ofreciendo un envío que ya se hizo.
// (Por eso esta función reemplazó a `MarcarComprobanteEnviado`: eran dos escrituras que no pueden
// divergir.)
//
// Devuelve si ESTE envío fue un REENVÍO —o sea, si ya había un OK anterior del mismo documento— y
// la hora que quedó registrada.
func (r *pgRepository) RegistrarEnvio(ctx context.Context, empresaID, docID string, reg RegistroEnvio) (bool, time.Time, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("cxp: registrar envío: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// «Reenvío» se mide ANTES de insertar: es «ya había uno bueno antes de este».
	const qPrevio = `SELECT EXISTS (
		SELECT 1 FROM comprobante_envio
		WHERE empresa_id = $1::uuid AND documento_id = $2::uuid AND resultado = 'OK')`
	var reenvio bool
	if err := tx.QueryRow(ctx, qPrevio, empresaID, docID).Scan(&reenvio); err != nil {
		return false, time.Time{}, fmt.Errorf("cxp: registrar envío: %w", err)
	}

	const qIns = `
		INSERT INTO comprobante_envio (empresa_id, documento_id, destinatario, copia, remitente,
			origen, archivo, comprobante_subido_en, resultado, error_categoria, error, enviado_por)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, '')::uuid)
		RETURNING enviado_en`
	var enviadoEn time.Time
	err = tx.QueryRow(ctx, qIns, empresaID, docID, reg.Destinatario, reg.Copia, reg.Remitente,
		reg.Origen, reg.Archivo, reg.ComprobanteSubidoEn, reg.Resultado, reg.ErrorCategoria,
		reg.Error, reg.UsuarioID).Scan(&enviadoEn)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("cxp: registrar envío: %w", err)
	}

	if reg.Resultado == EnvioOK {
		const qMarca = `UPDATE documento_cxp SET comprobante_enviado_en = $3, actualizado_en = now()
		                WHERE empresa_id = $1::uuid AND id = $2::uuid`
		if _, err := tx.Exec(ctx, qMarca, empresaID, docID, enviadoEn); err != nil {
			return false, time.Time{}, fmt.Errorf("cxp: marcar comprobante enviado: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, time.Time{}, fmt.Errorf("cxp: registrar envío: %w", err)
	}
	return reenvio, enviadoEn, nil
}

// ListarEnvios devuelve la bitácora de un documento, del más reciente al más viejo.
//
// `reenvio` y `mismo_archivo` se DERIVAN, no se guardan: el primer OK es el envío y los siguientes
// son reenvíos, y «el archivo que se mandó es el que está adjunto hoy» se responde comparando con
// el adjunto vigente. Guardarlos sería dejar que se desincronicen de los datos que los definen.
func (r *pgRepository) ListarEnvios(ctx context.Context, empresaID, docID string) ([]EnvioComprobante, error) {
	const q = `
		SELECT e.id::text, e.enviado_en, e.destinatario, e.copia, e.remitente, e.origen, e.archivo,
		       e.resultado, e.error_categoria, COALESCE(e.error, ''), COALESCE(u.nombre, ''),
		       (COUNT(*) FILTER (WHERE e.resultado = 'OK')
		          OVER (PARTITION BY e.documento_id ORDER BY e.enviado_en, e.id
		                ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING)) > 0 AS reenvio,
		       (e.comprobante_subido_en IS NOT NULL AND cp.subido_en IS NOT NULL
		        AND e.comprobante_subido_en = cp.subido_en) AS mismo_archivo
		FROM comprobante_envio e
		LEFT JOIN usuario u ON u.id = e.enviado_por
		LEFT JOIN comprobante_pago cp ON cp.documento_id = e.documento_id
		WHERE e.empresa_id = $1::uuid AND e.documento_id = $2::uuid
		ORDER BY e.enviado_en DESC, e.id DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, docID)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar envíos del comprobante: %w", err)
	}
	defer rows.Close()
	out := make([]EnvioComprobante, 0, 8)
	for rows.Next() {
		var e EnvioComprobante
		var errTexto string
		if err := rows.Scan(&e.ID, &e.EnviadoEn, &e.Destinatario, &e.Copia, &e.Remitente, &e.Origen,
			&e.Archivo, &e.Resultado, &e.ErrorCategoria, &errTexto, &e.EnviadoPor,
			&e.Reenvio, &e.MismoArchivo); err != nil {
			return nil, fmt.Errorf("cxp: listar envíos del comprobante: %w", err)
		}
		if errTexto != "" {
			texto := errTexto
			e.Error = &texto
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cxp: listar envíos del comprobante: %w", err)
	}
	return out, nil
}

// GuardarEmailProveedorDeDocumento deja en la ficha el correo que se escribió al reenviar.
//
// Solo si estaba VACÍO, igual que el teléfono: un envío puntual al contacto de Contabilidad no
// debería reemplazar el correo oficial del proveedor sin que nadie lo decida.
func (r *pgRepository) GuardarEmailProveedorDeDocumento(ctx context.Context, empresaID, docID, email string) error {
	const q = `
		UPDATE proveedor p
		   SET email = $3, actualizado_en = now()
		  FROM documento_cxp d
		 WHERE d.id = $2::uuid AND d.empresa_id = $1::uuid
		   AND p.id = d.proveedor_id AND p.empresa_id = $1::uuid
		   AND COALESCE(p.email, '') = ''`
	if _, err := r.pool.Exec(ctx, q, empresaID, docID, email); err != nil {
		return fmt.Errorf("cxp: guardar correo del proveedor: %w", err)
	}
	return nil
}
