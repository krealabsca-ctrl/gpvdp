package cxp

import (
	"context"
	"fmt"
)

// RecepcionConXML es una factura que entró por el buzón y cuyo XML sigue guardado.
type RecepcionConXML struct {
	RecepcionID string
	DocumentoID string
	XML         []byte
}

// RecepcionesSinDetalle lista las facturas que entraron por el buzón y todavía no tienen su
// detalle guardado, con el XML que ya está en la base.
//
// POR QUÉ EXISTE. El detalle de línea se empezó a guardar el 9 de octubre; todo lo que entró por
// el buzón antes de esa fecha tiene su `xml_crudo` guardado y la tabla de líneas vacía. Pedirle al
// usuario que vuelva a bajar esos XML del correo para subirlos a mano sería pedirle que traiga
// algo que el sistema ya tiene.
//
// `d.detalle_lineas = 0` es la condición, y no «no tiene filas en cxp_linea»: es el mismo contador
// que escribe GuardarDetalle, así que una factura ya procesada no se vuelve a tocar.
func (r *pgRepository) RecepcionesSinDetalle(ctx context.Context, empresaID string, limite int) ([]RecepcionConXML, error) {
	const q = `SELECT rc.id::text, rc.documento_id::text, rc.xml_crudo
	           FROM cxp_recepcion rc
	           JOIN documento_cxp d ON d.id = rc.documento_id AND d.empresa_id = rc.empresa_id
	           WHERE rc.empresa_id = $1::uuid
	             AND rc.xml_crudo IS NOT NULL
	             AND d.detalle_lineas = 0
	           ORDER BY rc.creado_en
	           LIMIT $2`
	rows, err := r.pool.Query(ctx, q, empresaID, limite)
	if err != nil {
		return nil, fmt.Errorf("cxp: recepciones sin detalle: %w", err)
	}
	defer rows.Close()

	out := []RecepcionConXML{}
	for rows.Next() {
		var x RecepcionConXML
		if err := rows.Scan(&x.RecepcionID, &x.DocumentoID, &x.XML); err != nil {
			return nil, fmt.Errorf("cxp: scan recepción sin detalle: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ContarRecepcionesSinDetalle dice cuántas quedan, para que la pantalla ofrezca el relleno solo
// cuando hay algo que rellenar y diga cuántas son antes de empezar.
func (r *pgRepository) ContarRecepcionesSinDetalle(ctx context.Context, empresaID string) (int, error) {
	const q = `SELECT count(*)
	           FROM cxp_recepcion rc
	           JOIN documento_cxp d ON d.id = rc.documento_id AND d.empresa_id = rc.empresa_id
	           WHERE rc.empresa_id = $1::uuid AND rc.xml_crudo IS NOT NULL AND d.detalle_lineas = 0`
	var n int
	if err := r.pool.QueryRow(ctx, q, empresaID).Scan(&n); err != nil {
		return 0, fmt.Errorf("cxp: contar recepciones sin detalle: %w", err)
	}
	return n, nil
}
