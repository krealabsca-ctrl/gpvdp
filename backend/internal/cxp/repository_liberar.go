package cxp

import (
	"context"
	"fmt"
)

// LiberarParaPago levanta el candado de las facturas indicadas, dejando la huella.
//
// EL UPDATE LLEVA LA CONDICIÓN ADENTRO (`AND bloqueado_para_pago`): entre que la pantalla pintó
// la lista y alguien confirma pasan minutos, y en ese rato otro pudo liberarla. Si la condición
// se revisara antes en Go, la segunda liberación pisaría la huella de la primera —otro motivo,
// otro usuario, otra fecha— y la historia de quién lo autorizó se perdería.
//
// Devuelve SOLO las que de verdad cambiaron. Las que no vuelven es porque no estaban bloqueadas,
// no existen o son de otra empresa; el service las nombra una por una.
// El `proveedor_id` del WHERE no es redundante con el filtro de empresa: la ruta cuelga de UN
// proveedor, así que los ids que llegan en el cuerpo tienen que ser suyos. Sin esa condición,
// quien pueda liberar las facturas de un proveedor cualquiera podría mandar en el cuerpo los ids
// de otro y liberarlas desde ahí. El alcance lo pone el servidor, nunca el cliente.
func (r *pgRepository) LiberarParaPago(ctx context.Context, empresaID, proveedorID string, ids []string, motivo, usuarioID string) ([]FacturaLiberada, error) {
	const q = `
		UPDATE documento_cxp d
		   SET bloqueado_para_pago = false,
		       desbloqueado_por    = $3::uuid,
		       desbloqueado_en     = now(),
		       desbloqueo_motivo   = $4,
		       -- El motivo original del bloqueo NO se borra: es lo que explica por qué estuvo
		       -- trancada, y sin él la huella diría quién la liberó pero no de qué.
		       actualizado_en      = now()
		 WHERE d.empresa_id = $1::uuid
		   AND d.proveedor_id = $5::uuid
		   AND d.id = ANY($2::uuid[])
		   AND d.bloqueado_para_pago
		RETURNING d.id::text, COALESCE(d.consecutivo, ''), d.total::text, d.moneda,
		          COALESCE((SELECT p.nombre FROM proveedor p WHERE p.id = d.proveedor_id), '')`

	rows, err := r.pool.Query(ctx, q, empresaID, ids, usuarioID, motivo, proveedorID)
	if err != nil {
		return nil, fmt.Errorf("cxp: liberar para pago: %w", err)
	}
	defer rows.Close()

	out := []FacturaLiberada{}
	for rows.Next() {
		var f FacturaLiberada
		if err := rows.Scan(&f.DocumentoID, &f.Consecutivo, &f.Total, &f.Moneda, &f.Proveedor); err != nil {
			return nil, fmt.Errorf("cxp: scan liberada: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FacturasDeContado lista las facturas de un proveedor que quedaron trancadas por el candado.
//
// Es lo que se muestra al marcar «manda el convenio» en su ficha: cuántas son, cuáles y por
// cuánto. Ordenadas de la más vieja a la más nueva, que es el orden en que hay que pagarlas.
func (r *pgRepository) FacturasDeContado(ctx context.Context, empresaID, proveedorID string) ([]FacturaDeContado, error) {
	const q = `SELECT id::text, COALESCE(consecutivo, ''), fecha_emision::text,
	                  total::text, moneda, COALESCE(bloqueo_motivo, '')
	           FROM documento_cxp
	           WHERE empresa_id = $1::uuid AND proveedor_id = $2::uuid AND bloqueado_para_pago
	           ORDER BY fecha_emision, consecutivo`
	rows, err := r.pool.Query(ctx, q, empresaID, proveedorID)
	if err != nil {
		return nil, fmt.Errorf("cxp: facturas de contado: %w", err)
	}
	defer rows.Close()

	out := []FacturaDeContado{}
	for rows.Next() {
		var f FacturaDeContado
		if err := rows.Scan(&f.DocumentoID, &f.Consecutivo, &f.FechaEmision, &f.Total, &f.Moneda, &f.Motivo); err != nil {
			return nil, fmt.Errorf("cxp: scan factura de contado: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// EstadoDeBloqueo dice, para los ids pedidos, cuáles existen en la empresa y cuáles siguen
// bloqueados. Sirve para explicar los descartes con precisión en vez de decir «no se pudo».
func (r *pgRepository) EstadoDeBloqueo(ctx context.Context, empresaID string, ids []string) (map[string]bool, error) {
	const q = `SELECT id::text, bloqueado_para_pago FROM documento_cxp
	           WHERE empresa_id = $1::uuid AND id = ANY($2::uuid[])`
	rows, err := r.pool.Query(ctx, q, empresaID, ids)
	if err != nil {
		return nil, fmt.Errorf("cxp: estado de bloqueo: %w", err)
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var id string
		var bloq bool
		if err := rows.Scan(&id, &bloq); err != nil {
			return nil, fmt.Errorf("cxp: scan estado de bloqueo: %w", err)
		}
		out[id] = bloq
	}
	return out, rows.Err()
}
