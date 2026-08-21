package cxp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// netoAPagarSQL es LA definición de lo que sale del banco por una factura: total menos la
// retención menos los anticipos aplicados. La comparten el archivo de pago y la verificación de
// la huella, para que no puedan divergir.
const netoAPagarSQL = `GREATEST(d.total - d.retencion - COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa WHERE aa.factura_id = d.id AND aa.activo), 0), 0)`

// NetoAPagar devuelve el neto de un documento (mismo cálculo que el archivo de pago).
func (r *pgRepository) NetoAPagar(ctx context.Context, empresaID, docID string) (string, error) {
	const q = `SELECT ` + netoAPagarSQL + `::text FROM documento_cxp d
		WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid`
	var neto string
	err := r.pool.QueryRow(ctx, q, empresaID, docID).Scan(&neto)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrDocumentoNoEncontrado
	}
	if err != nil {
		return "", fmt.Errorf("cxp: neto a pagar: %w", err)
	}
	return neto, nil
}

// DocumentosParaPago devuelve los documentos PROGRAMADOS listos para el archivo de pago.
// fecha "" = todos los programados; con fecha, solo los con pago programado hasta esa fecha.
func (r *pgRepository) DocumentosParaPago(ctx context.Context, empresaID, fecha string) ([]PagoRow, error) {
	const q = `
		SELECT COALESCE(p.identificacion, ''), p.nombre, COALESCE(p.iban, ''), d.moneda,
		       ` + netoAPagarSQL + `::text,
		       COALESCE(d.huella, ''), COALESCE(d.consecutivo, ''), d.id::text
		FROM documento_cxp d
		JOIN proveedor p ON p.id = d.proveedor_id
		WHERE d.empresa_id = $1::uuid AND d.estado = 'PROGRAMADO'
		  AND (NULLIF($2, '')::date IS NULL OR d.fecha_pago_programada <= NULLIF($2, '')::date)
		ORDER BY p.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, fecha)
	if err != nil {
		return nil, fmt.Errorf("cxp: documentos para pago: %w", err)
	}
	defer rows.Close()
	var out []PagoRow
	for rows.Next() {
		var pr PagoRow
		if err := rows.Scan(&pr.Cedula, &pr.Nombre, &pr.IBAN, &pr.Moneda, &pr.MontoNeto,
			&pr.Descripcion, &pr.Consecutivo, &pr.DocumentoID); err != nil {
			return nil, fmt.Errorf("cxp: scan pago: %w", err)
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// DocumentosParaPagoPorIDs devuelve las líneas de pago de los documentos indicados que
// estén PROGRAMADO (los demás ids se ignoran). Filtra por empresa (tenant-safe).
func (r *pgRepository) DocumentosParaPagoPorIDs(ctx context.Context, empresaID string, ids []string) ([]PagoRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const q = `
		SELECT COALESCE(p.identificacion, ''), p.nombre, COALESCE(p.iban, ''), d.moneda,
		       ` + netoAPagarSQL + `::text,
		       COALESCE(d.huella, ''), COALESCE(d.consecutivo, ''), d.id::text
		FROM documento_cxp d
		JOIN proveedor p ON p.id = d.proveedor_id
		WHERE d.empresa_id = $1::uuid AND d.estado = 'PROGRAMADO' AND d.id = ANY($2::uuid[])
		ORDER BY p.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, ids)
	if err != nil {
		return nil, fmt.Errorf("cxp: documentos para pago por ids: %w", err)
	}
	defer rows.Close()
	var out []PagoRow
	for rows.Next() {
		var pr PagoRow
		if err := rows.Scan(&pr.Cedula, &pr.Nombre, &pr.IBAN, &pr.Moneda, &pr.MontoNeto,
			&pr.Descripcion, &pr.Consecutivo, &pr.DocumentoID); err != nil {
			return nil, fmt.Errorf("cxp: scan pago por id: %w", err)
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// DocumentoPorHuella busca un documento por su huella, solo si está PROGRAMADO o PAGADO.
func (r *pgRepository) DocumentoPorHuella(ctx context.Context, empresaID, huella string) (Documento, error) {
	const q = `SELECT ` + documentoCols + ` ` + documentoFrom + `
		WHERE d.empresa_id = $1::uuid AND d.huella = $2 AND d.estado IN ('PROGRAMADO', 'PAGADO')`
	d, err := scanDocumento(r.pool.QueryRow(ctx, q, empresaID, huella))
	if errors.Is(err, pgx.ErrNoRows) {
		return Documento{}, ErrDocumentoNoEncontrado
	}
	if err != nil {
		return Documento{}, fmt.Errorf("cxp: documento por huella: %w", err)
	}
	return d, nil
}
