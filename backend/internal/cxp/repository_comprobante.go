package cxp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GuardarComprobante guarda (o reemplaza) el comprobante de una factura PAGADA/CONCILIADA
// de la empresa. Devuelve ErrDocNoPagado si el documento no es elegible.
func (r *pgRepository) GuardarComprobante(ctx context.Context, empresaID, docID, filename, mime string, contenido []byte, usuarioID string) error {
	const q = `
		INSERT INTO comprobante_pago (empresa_id, documento_id, filename, mime, contenido, subido_por)
		SELECT $1::uuid, $2::uuid, $3, $4, $5, $6::uuid
		WHERE EXISTS (
			SELECT 1 FROM documento_cxp
			WHERE id = $2::uuid AND empresa_id = $1::uuid AND estado IN ('PAGADO', 'CONCILIADO'))
		ON CONFLICT (documento_id) DO UPDATE
			SET filename = EXCLUDED.filename, mime = EXCLUDED.mime, contenido = EXCLUDED.contenido,
			    subido_por = EXCLUDED.subido_por, subido_en = now()`
	tag, err := r.pool.Exec(ctx, q, empresaID, docID, filename, mime, contenido, usuarioID)
	if err != nil {
		return fmt.Errorf("cxp: guardar comprobante: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDocNoPagado
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

// ObtenerComprobanteEnvio trae el adjunto + los datos del proveedor y la factura para el correo.
func (r *pgRepository) ObtenerComprobanteEnvio(ctx context.Context, empresaID, docID string) (ComprobanteEnvio, error) {
	const q = `
		SELECT cp.filename, cp.mime, cp.contenido,
		       COALESCE(p.email, ''), p.nombre, COALESCE(d.consecutivo, ''), d.total_crc::text,
		       d.moneda, d.total::text, COALESCE(d.huella, ''), COALESCE(d.descripcion, '')
		FROM comprobante_pago cp
		JOIN documento_cxp d ON d.id = cp.documento_id
		JOIN proveedor p ON p.id = d.proveedor_id
		WHERE cp.empresa_id = $1::uuid AND cp.documento_id = $2::uuid`
	var e ComprobanteEnvio
	err := r.pool.QueryRow(ctx, q, empresaID, docID).
		Scan(&e.Filename, &e.Mime, &e.Contenido, &e.ProveedorEmail, &e.ProveedorNombre, &e.Consecutivo, &e.TotalCRC,
			&e.Moneda, &e.Total, &e.Huella, &e.Descripcion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ComprobanteEnvio{}, ErrComprobanteNoEncontrado
	}
	if err != nil {
		return ComprobanteEnvio{}, fmt.Errorf("cxp: comprobante para envío: %w", err)
	}
	return e, nil
}

// MarcarComprobanteEnviado registra la fecha/hora de envío del comprobante al proveedor.
func (r *pgRepository) MarcarComprobanteEnviado(ctx context.Context, empresaID, docID string) error {
	const q = `UPDATE documento_cxp SET comprobante_enviado_en = now(), actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	_, err := r.pool.Exec(ctx, q, empresaID, docID)
	if err != nil {
		return fmt.Errorf("cxp: marcar comprobante enviado: %w", err)
	}
	return nil
}
