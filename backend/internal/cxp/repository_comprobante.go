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
func (r *pgRepository) GuardarComprobante(ctx context.Context, empresaID, docID, filename, mime string, contenido []byte, usuarioID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cxp: guardar comprobante: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const qArchivo = `
		INSERT INTO comprobante_pago (empresa_id, documento_id, filename, mime, contenido, subido_por)
		SELECT $1::uuid, $2::uuid, $3, $4, $5, $6::uuid
		WHERE EXISTS (
			SELECT 1 FROM documento_cxp
			WHERE id = $2::uuid AND empresa_id = $1::uuid AND estado IN ('PAGADO', 'CONCILIADO'))
		ON CONFLICT (documento_id) DO UPDATE
			SET filename = EXCLUDED.filename, mime = EXCLUDED.mime, contenido = EXCLUDED.contenido,
			    subido_por = EXCLUDED.subido_por, subido_en = now()`
	tag, err := tx.Exec(ctx, qArchivo, empresaID, docID, filename, mime, contenido, usuarioID)
	if err != nil {
		return fmt.Errorf("cxp: guardar comprobante: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDocNoPagado
	}

	const qMarca = `UPDATE documento_cxp SET comprobante_enviado_en = NULL, actualizado_en = now()
	                WHERE empresa_id = $1::uuid AND id = $2::uuid AND comprobante_enviado_en IS NOT NULL`
	if _, err := tx.Exec(ctx, qMarca, empresaID, docID); err != nil {
		return fmt.Errorf("cxp: limpiar la marca de enviado: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cxp: guardar comprobante: %w", err)
	}
	return nil
}

// ObtenerComprobante devuelve el archivo adjunto de una factura (para descargar).
func (r *pgRepository) ObtenerComprobante(ctx context.Context, empresaID, docID string) (Comprobante, error) {
	const q = `SELECT filename, mime, contenido FROM comprobante_pago
	           WHERE empresa_id = $1::uuid AND documento_id = $2::uuid`
	var c Comprobante
	err := r.pool.QueryRow(ctx, q, empresaID, docID).Scan(&c.Filename, &c.Mime, &c.Contenido)
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
		       COALESCE(ap.email, ''), COALESCE(ap.nombre, '')
		FROM comprobante_pago cp
		JOIN documento_cxp d ON d.id = cp.documento_id
		JOIN proveedor p ON p.id = d.proveedor_id
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
			&e.AprobadorEmail, &e.AprobadorNombre)
	if errors.Is(err, pgx.ErrNoRows) {
		return ComprobanteEnvio{}, ErrComprobanteNoEncontrado
	}
	if err != nil {
		return ComprobanteEnvio{}, fmt.Errorf("cxp: comprobante para envío: %w", err)
	}
	return e, nil
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
