package cxp

// Consultas de la aprobación del corte (mig 0123). El porqué está en lote_aprobacion.go.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// EstadoDelLote devuelve el estado actual, o false si el lote no existe o es de otra empresa.
//
// Se consulta DESPUÉS de un UPDATE que no movió nada, para distinguir «no existe» de «estaba en
// otro estado». Sin esto, las dos cosas darían el mismo error y quien lo recibe no sabría si el
// corte se le perdió o si alguien ya lo aprobó.
func (r *pgRepository) EstadoDelLote(ctx context.Context, empresaID, loteID string) (string, bool, error) {
	var estado string
	err := r.pool.QueryRow(ctx,
		`SELECT estado FROM lote_pago WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, loteID).Scan(&estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("cxp: estado del corte: %w", err)
	}
	return estado, true, nil
}

// MandarAAprobar pasa ABIERTO → EN_APROBACION.
func (r *pgRepository) MandarAAprobar(ctx context.Context, empresaID, loteID, usuarioID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lote_pago
		   SET estado = 'EN_APROBACION',
		       enviado_aprobacion_por = $3::uuid, enviado_aprobacion_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'ABIERTO'`,
		empresaID, loteID, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("cxp: mandar el corte a aprobar: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AprobarLote pasa EN_APROBACION → APROBADO.
//
// NO se borran `devuelto_por`/`devuelto_en`: que el corte haya sido devuelto antes de aprobarse
// es parte de lo que hay que poder reconstruir. Lo que el lote informa después de esto es «se
// devolvió el día X y se aprobó el día Y», que es la verdad.
func (r *pgRepository) AprobarLote(ctx context.Context, empresaID, loteID, usuarioID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lote_pago
		   SET estado = 'APROBADO', aprobado_por = $3::uuid, aprobado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'EN_APROBACION'`,
		empresaID, loteID, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("cxp: aprobar el corte: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DevolverLote pasa EN_APROBACION → ABIERTO dejando el motivo.
//
// Vuelve a ABIERTO y no a un estado propio: devolver es decir «arreglalo y mandámelo de nuevo»,
// así que el corte tiene que quedar exactamente donde tesorería puede trabajarlo. Un estado
// «DEVUELTO» aparte solo agregaría una casilla que hace lo mismo que ABIERTO.
func (r *pgRepository) DevolverLote(ctx context.Context, empresaID, loteID, usuarioID, motivo string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lote_pago
		   SET estado = 'ABIERTO', devuelto_por = $3::uuid, devuelto_en = now(),
		       devolucion_motivo = $4
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'EN_APROBACION'`,
		empresaID, loteID, usuarioID, motivo)
	if err != nil {
		return 0, fmt.Errorf("cxp: devolver el corte: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ExcluirDelLote saca UNA factura del corte y la devuelve a la bandeja.
//
// NO TOCA `estado`. Las facturas entran al lote ya estando en PROGRAMADO —`CrearLote` solo les
// pone `lote_id`— así que quitarles el lote las deja en PROGRAMADO, con su aprobación y su
// validación de área intactas, listas para el próximo corte. Eso es lo que evita el reproceso:
// excluir no es denegar.
//
// El número del corte se guarda ANTES de soltar el `lote_id`, en el mismo UPDATE: después de
// soltarlo ya no hay desde dónde leerlo, y una factura que volvió a la bandeja sin poder decir de
// qué corte salió no explica por qué no se pagó el viernes.
func (r *pgRepository) ExcluirDelLote(ctx context.Context, empresaID, loteID, documentoID, usuarioID, motivo string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE documento_cxp d
		   SET lote_id = NULL,
		       excluido_lote_por = $4::uuid, excluido_lote_en = now(),
		       excluido_lote_motivo = $5,
		       excluido_lote_numero = (SELECT l.numero FROM lote_pago l WHERE l.id = $2::uuid),
		       actualizado_en = now()
		 WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid AND d.id = $3::uuid`,
		empresaID, loteID, documentoID, usuarioID, motivo)
	if err != nil {
		return 0, fmt.Errorf("cxp: sacar la factura del corte: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ExcluidasDelLote lista lo que la gerencia sacó de un corte.
//
// Se busca por `excluido_lote_numero` y no por `lote_id` —que ya es NULL— porque la pregunta es
// del CORTE: qué salió de él. Es lo que va en el cronograma aprobado para poder comparar después
// lo que se aprobó contra lo que se pagó.
func (r *pgRepository) ExcluidasDelLote(ctx context.Context, empresaID, loteID string) ([]FacturaExcluida, error) {
	const q = `
		SELECT d.id::text, COALESCE(d.consecutivo, ''), COALESCE(p.nombre, ''),
		       COALESCE(d.total_crc, 0)::text, COALESCE(d.excluido_lote_motivo, ''),
		       COALESCE(u.nombre, ''), COALESCE(to_char(d.excluido_lote_en, 'YYYY-MM-DD HH24:MI'), ''),
		       COALESCE(d.excluido_lote_numero, 0)
		  FROM documento_cxp d
		  JOIN lote_pago l ON l.id = $2::uuid AND l.empresa_id = $1::uuid
		  LEFT JOIN proveedor p ON p.id = d.proveedor_id
		  LEFT JOIN usuario u ON u.id = d.excluido_lote_por
		 WHERE d.empresa_id = $1::uuid
		   AND d.excluido_lote_numero = l.numero
		   AND d.excluido_lote_en IS NOT NULL
		 ORDER BY d.excluido_lote_en DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: excluidas del corte: %w", err)
	}
	defer rows.Close()

	out := make([]FacturaExcluida, 0, 8)
	for rows.Next() {
		var f FacturaExcluida
		if err := rows.Scan(&f.DocumentoID, &f.Consecutivo, &f.Proveedor, &f.MontoCRC,
			&f.Motivo, &f.ExcluidoPor, &f.ExcluidoEn, &f.LoteNumero); err != nil {
			return nil, fmt.Errorf("cxp: scan excluida: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
