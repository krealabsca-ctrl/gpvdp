package cxp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// DocumentoDeClave busca la factura a la que pertenece un XML, por su clave de 50 dígitos.
//
// `(empresa_id, clave)` es UNIQUE, así que o hay una o no hay ninguna. El filtro por empresa no es
// decorativo: el mismo proveedor le factura a Valle de Paz y a Memorial Pets, y una clave de otra
// empresa NO puede recibir el detalle de ésta.
func (r *pgRepository) DocumentoDeClave(ctx context.Context, empresaID, clave string) (DocumentoBreve, error) {
	const q = `SELECT d.id, d.consecutivo, d.moneda, d.detalle_lineas, COALESCE(p.nombre,'')
	           FROM documento_cxp d
	           LEFT JOIN proveedor p ON p.id = d.proveedor_id
	           WHERE d.empresa_id = $1::uuid AND d.clave = $2`
	var b DocumentoBreve
	err := r.pool.QueryRow(ctx, q, empresaID, clave).
		Scan(&b.ID, &b.Consecutivo, &b.Moneda, &b.Lineas, &b.Proveedor)
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentoBreve{}, ErrDocumentoNoEncontrado
	}
	if err != nil {
		return DocumentoBreve{}, fmt.Errorf("cxp: documento por clave: %w", err)
	}
	return b, nil
}

// GuardarDetalle reemplaza las líneas de una factura por las del XML.
//
// REEMPLAZA, no agrega: cargar dos veces el mismo archivo tiene que dejar la factura igual, y una
// factura corregida por el proveedor (misma clave, otro detalle) tiene que quedar con el detalle
// nuevo y no con los dos mezclados. El DELETE va dentro de la misma transacción que el INSERT, así
// que una carga que falle a la mitad no deja la factura sin detalle.
//
// El `DELETE` acá sí es físico y es correcto que lo sea: `cxp_linea` no es una tabla financiera de
// registro —es un ESPEJO del XML, que es el original y sigue existiendo—. No se le debe dinero a
// nadie por una fila de esta tabla.
func (r *pgRepository) GuardarDetalle(ctx context.Context, empresaID, documentoID string, lineas []LineaDetalle) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cxp: begin guardar detalle: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El filtro por empresa viaja en el DELETE y en el INSERT, no solo en la búsqueda previa.
	const del = `DELETE FROM cxp_linea WHERE empresa_id = $1::uuid AND documento_id = $2::uuid`
	if _, err := tx.Exec(ctx, del, empresaID, documentoID); err != nil {
		return fmt.Errorf("cxp: limpiar detalle: %w", err)
	}

	const ins = `INSERT INTO cxp_linea (
	       empresa_id, documento_id, linea, numero_linea,
	       cabys, codigo_comercial, detalle, detalle_norm, unidad_medida,
	       cantidad, precio_unitario, monto_total, descuento, subtotal, impuesto, monto_total_linea)
	   VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9,
	           $10::numeric, $11::numeric, $12::numeric, $13::numeric, $14::numeric, $15::numeric, $16::numeric)`
	for _, l := range lineas {
		if _, err := tx.Exec(ctx, ins,
			empresaID, documentoID, l.Linea, l.NumeroLinea,
			l.CABYS, l.CodigoComercial, l.Detalle, l.DetalleNorm, l.UnidadMedida,
			num(l.Cantidad), num(l.PrecioUnitario), num(l.MontoTotal), num(l.Descuento),
			num(l.SubTotal), num(l.Impuesto), num(l.MontoTotalLinea),
		); err != nil {
			return fmt.Errorf("cxp: insertar línea %d: %w", l.Linea, err)
		}
	}

	const marcar = `UPDATE documento_cxp SET detalle_lineas = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`
	if _, err := tx.Exec(ctx, marcar, empresaID, documentoID, len(lineas)); err != nil {
		return fmt.Errorf("cxp: marcar detalle: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cxp: commit detalle: %w", err)
	}
	return nil
}

// num pasa el puntero a lo que pgx entiende: nil se escribe como NULL.
//
// Se devuelve `any` a propósito. Pasar el `*decimal.Decimal` directo funciona para el caso con
// valor y escribe la cadena vacía —no NULL— para el nil en algunas versiones del driver, que es
// justo lo que esta columna no puede tener.
func num(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return d.String()
}

// LineasDeDocumento devuelve el detalle de una factura, en orden.
func (r *pgRepository) LineasDeDocumento(ctx context.Context, empresaID, documentoID string) ([]LineaGuardada, error) {
	const q = `SELECT linea, numero_linea, cabys, codigo_comercial, detalle, unidad_medida,
	                  COALESCE(cantidad::text,''), COALESCE(precio_unitario::text,''),
	                  COALESCE(descuento::text,''), COALESCE(impuesto::text,''),
	                  COALESCE(monto_total_linea::text,'')
	           FROM cxp_linea
	           WHERE empresa_id = $1::uuid AND documento_id = $2::uuid
	           ORDER BY linea`
	rows, err := r.pool.Query(ctx, q, empresaID, documentoID)
	if err != nil {
		return nil, fmt.Errorf("cxp: líneas de documento: %w", err)
	}
	defer rows.Close()

	out := []LineaGuardada{}
	for rows.Next() {
		var l LineaGuardada
		if err := rows.Scan(&l.Linea, &l.NumeroLinea, &l.CABYS, &l.CodigoComercial, &l.Detalle,
			&l.UnidadMedida, &l.Cantidad, &l.PrecioUnitario, &l.Descuento, &l.Impuesto, &l.Total); err != nil {
			return nil, fmt.Errorf("cxp: scan línea: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
