package cxp

import (
	"context"
	"fmt"
	"strings"
)

// estadosConComprobante son los dos en los que una factura admite recibo. Está acá como lista y no
// escrito dentro de cada consulta para que las tres —la que resuelve destinos, la que escribe y la
// que cuenta lo que falta— no puedan discrepar sobre qué es «ya pagada».
var estadosConComprobante = []string{"PAGADO", "CONCILIADO"}

// DestinosPorHuella resuelve de un viaje los códigos leídos en los nombres de archivo.
//
// Los dos caminos van en la MISMA consulta por UNION ALL: los códigos de factura contra
// `documento_cxp.huella` y los de remesa contra `remesa_pago.huella`, trayendo en ese caso todas
// las facturas que el pago agrupado cubre. Separarlos en dos viajes obligaría a saber de antemano
// de qué tipo es cada código, que es justo lo que esta función existe para averiguar.
//
// Devuelve TODAS las facturas del código, pagadas o no, y las reparte acá: la diferencia entre
// «ese código no existe» y «ese pago todavía no está marcado como pagado» son dos problemas con
// dos soluciones distintas, y la pantalla tiene que poder decir cuál es.
func (r *pgRepository) DestinosPorHuella(ctx context.Context, empresaID string, codigos []string) (map[string]HuellaDestino, error) {
	out := make(map[string]HuellaDestino, len(codigos))
	if len(codigos) == 0 {
		return out, nil
	}
	const q = `
		WITH cods AS (SELECT DISTINCT unnest($2::text[]) AS cod)
		SELECT c.cod, false AS es_remesa, d.id::text, COALESCE(d.consecutivo, ''), p.nombre,
		       d.total::text, d.moneda, d.estado, (cp.documento_id IS NOT NULL) AS ya_tiene
		FROM cods c
		JOIN documento_cxp d ON d.empresa_id = $1::uuid AND d.huella = c.cod
		JOIN proveedor p ON p.id = d.proveedor_id
		LEFT JOIN comprobante_pago cp ON cp.documento_id = d.id
		UNION ALL
		SELECT c.cod, true, d.id::text, COALESCE(d.consecutivo, ''), p.nombre,
		       d.total::text, d.moneda, d.estado, (cp.documento_id IS NOT NULL)
		FROM cods c
		JOIN remesa_pago rm ON rm.empresa_id = $1::uuid AND rm.huella = c.cod
		JOIN documento_cxp d ON d.remesa_id = rm.id AND d.empresa_id = $1::uuid
		JOIN proveedor p ON p.id = d.proveedor_id
		LEFT JOIN comprobante_pago cp ON cp.documento_id = d.id
		ORDER BY 1, 4, 3`
	rows, err := r.pool.Query(ctx, q, empresaID, codigos)
	if err != nil {
		return nil, fmt.Errorf("cxp: resolver códigos de comprobante: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cod, estado, moneda string
		var esRemesa bool
		var d DestinoComprobante
		if err := rows.Scan(&cod, &esRemesa, &d.DocumentoID, &d.Consecutivo, &d.Proveedor,
			&d.Total, &moneda, &estado, &d.YaTiene); err != nil {
			return nil, fmt.Errorf("cxp: resolver códigos de comprobante: %w", err)
		}
		d.Moneda = moneda
		h := out[cod]
		h.EsRemesa = esRemesa
		if estado == "PAGADO" || estado == "CONCILIADO" {
			h.Pagadas = append(h.Pagadas, d)
		} else {
			h.SinPagar++
		}
		out[cod] = h
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cxp: resolver códigos de comprobante: %w", err)
	}
	return out, nil
}

// GuardarComprobantesCarpeta adjunta varios archivos de una corrida en UNA transacción.
//
// EL INSERT VUELVE A VERIFICAR EL ESTADO, igual que el de a uno: entre previsualizar y confirmar
// pasan minutos y la factura pudo anularse o volver atrás. Lo que la revalidación deja afuera se
// devuelve en `Rechazados` en vez de callarse — un conteo que dice «87 adjuntados» cuando fueron
// 85 es peor que uno que dice 85 y nombra las dos, porque nadie vuelve a mirar las que faltan.
//
// Todo o nada: si una escritura revienta, no queda media carpeta adjunta y la corrida se repite
// completa. Media carpeta obligaría a averiguar cuáles entraron, que es exactamente el trabajo
// manual que esto vino a eliminar.
//
// Al reemplazar un comprobante se BORRA la marca de enviado, por la misma razón que en el camino de
// a uno: `comprobante_enviado_en` significa «el proveedor tiene el PDF que está adjunto AHORA», y
// con otro archivo esa afirmación deja de ser cierta.
func (r *pgRepository) GuardarComprobantesCarpeta(ctx context.Context, empresaID string, items []ItemCarpeta, usuarioID string) (ResultadoCarpeta, error) {
	var res ResultadoCarpeta
	res.Rechazados = []string{}
	if len(items) == 0 {
		return res, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("cxp: adjuntar comprobantes: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// La carpeta de una corrida trae RECIBOS DEL BANCO, así que todo lo que entra por acá es
	// COMPROBANTE. El tipo va explícito —aunque sea el default de la columna— porque el
	// `ON CONFLICT` tiene que nombrar el índice entero (documento_id, tipo) desde la mig 0103, y
	// dejarlo implícito hacía que Postgres no encontrara restricción que calzara.
	const qArchivo = `
		INSERT INTO comprobante_pago (empresa_id, documento_id, tipo, filename, mime, contenido, subido_por)
		SELECT $1::uuid, $2::uuid, 'COMPROBANTE', $3, $4, $5, $6::uuid
		WHERE EXISTS (
			SELECT 1 FROM documento_cxp
			WHERE id = $2::uuid AND empresa_id = $1::uuid AND estado = ANY($7::text[]))
		ON CONFLICT (documento_id, tipo) DO UPDATE
			SET filename = EXCLUDED.filename, mime = EXCLUDED.mime, contenido = EXCLUDED.contenido,
			    subido_por = EXCLUDED.subido_por, subido_en = now()`
	const qMarca = `UPDATE documento_cxp SET comprobante_enviado_en = NULL, actualizado_en = now()
	                WHERE empresa_id = $1::uuid AND id = $2::uuid AND comprobante_enviado_en IS NOT NULL`

	for _, it := range items {
		escribio := false
		for _, docID := range it.DocumentoID {
			tag, err := tx.Exec(ctx, qArchivo, empresaID, docID, it.Nombre, it.Mime, it.Contenido,
				usuarioID, estadosConComprobante)
			if err != nil {
				return ResultadoCarpeta{}, fmt.Errorf("cxp: adjuntar %q: %w", it.Nombre, err)
			}
			if tag.RowsAffected() == 0 {
				res.Rechazados = append(res.Rechazados, it.Nombre)
				continue
			}
			if _, err := tx.Exec(ctx, qMarca, empresaID, docID); err != nil {
				return ResultadoCarpeta{}, fmt.Errorf("cxp: limpiar la marca de enviado: %w", err)
			}
			res.Facturas++
			escribio = true
		}
		if escribio {
			res.Archivos++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ResultadoCarpeta{}, fmt.Errorf("cxp: adjuntar comprobantes: %w", err)
	}
	return res, nil
}

// PagosSinComprobante lista las transferencias ya pagadas y si tienen su recibo guardado.
//
// UNA FILA POR TRANSFERENCIA, no por factura: es lo que corresponde uno a uno con un archivo bajado
// del banco, y por eso el pago agrupado se cuenta una sola vez aunque cubra cinco facturas. Contar
// por factura daría «de 100 pagos me faltan 140», que no se puede comparar con nada.
//
// Las facturas sin remesa se agrupan por su propia huella y las agrupadas por la de la remesa; las
// dos mitades salen del mismo UNION para que el total sea el número de transferencias del lote.
//
// `con_recibo` se cuenta en vez de responder sí/no porque existe el caso intermedio: alguien
// adjuntó el recibo a mano a UNA de las cinco facturas de una remesa. Un booleano tendría que
// elegir entre decir «ya está» —y el pago quedaría medio documentado para siempre— o «falta», y
// mandaría a subir de nuevo algo que ya está. El número dice qué pasó.
func (r *pgRepository) PagosSinComprobante(ctx context.Context, empresaID, loteID string, soloFaltantes bool) ([]PagoConRecibo, error) {
	args := []any{empresaID, estadosConComprobante}
	filtroLote := ""
	if strings.TrimSpace(loteID) != "" {
		args = append(args, loteID)
		filtroLote = fmt.Sprintf(" AND d.lote_id = $%d::uuid", len(args))
	}
	q := fmt.Sprintf(`
		WITH pagos AS (
			SELECT rcxp.huella AS codigo, true AS es_remesa, %s AS referencia, p.nombre AS proveedor,
			       lrem.id::text AS lote_id, lrem.numero AS lote_numero,
			       count(*) AS facturas, sum(d.total_crc) AS monto,
			       count(*) FILTER (WHERE cp.documento_id IS NOT NULL) AS con_recibo
			FROM documento_cxp d
			JOIN remesa_pago rcxp ON rcxp.id = d.remesa_id
			JOIN lote_pago lrem ON lrem.id = rcxp.lote_id
			JOIN proveedor p ON p.id = d.proveedor_id
			LEFT JOIN comprobante_pago cp ON cp.documento_id = d.id
			WHERE d.empresa_id = $1::uuid AND d.estado = ANY($2::text[])%[2]s
			GROUP BY 1, 2, 3, 4, 5, 6
			UNION ALL
			SELECT d.huella, false, 'FC ' || lpad(right(regexp_replace(COALESCE(d.consecutivo, ''), '\D', '', 'g'), 6), 6, '0'),
			       p.nombre, COALESCE(l.id::text, ''), COALESCE(l.numero, 0),
			       1, d.total_crc,
			       (CASE WHEN cp.documento_id IS NOT NULL THEN 1 ELSE 0 END)
			FROM documento_cxp d
			JOIN proveedor p ON p.id = d.proveedor_id
			LEFT JOIN lote_pago l ON l.id = d.lote_id
			LEFT JOIN comprobante_pago cp ON cp.documento_id = d.id
			WHERE d.empresa_id = $1::uuid AND d.estado = ANY($2::text[])
			  AND d.remesa_id IS NULL AND d.huella IS NOT NULL AND d.huella <> ''%[2]s
		)
		SELECT codigo, es_remesa, referencia, proveedor, lote_id, lote_numero,
		       facturas, monto::text, con_recibo
		FROM pagos
		%[3]s
		ORDER BY lote_numero DESC, referencia`,
		referenciaRemesaSQL, filtroLote, filtroSinRecibo(soloFaltantes))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cxp: pagos sin comprobante: %w", err)
	}
	defer rows.Close()
	out := make([]PagoConRecibo, 0, 64)
	for rows.Next() {
		var p PagoConRecibo
		if err := rows.Scan(&p.Codigo, &p.EsRemesa, &p.Referencia, &p.Proveedor, &p.LoteID,
			&p.LoteNumero, &p.Facturas, &p.MontoCRC, &p.ConRecibo); err != nil {
			return nil, fmt.Errorf("cxp: pagos sin comprobante: %w", err)
		}
		p.NombreSugerido = NombreSugerido(p.Referencia, p.Codigo)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cxp: pagos sin comprobante: %w", err)
	}
	return out, nil
}

// filtroSinRecibo recorta el UNION ya armado cuando se piden solo los faltantes.
//
// Va AFUERA y no dentro de cada mitad: una filtra sobre un agregado (necesitaría HAVING) y la otra
// sobre una fila suelta (necesitaría WHERE). Escrito dos veces, son dos definiciones de «le falta
// el recibo» que pueden separarse con el primer cambio; escrito una, no.
func filtroSinRecibo(solo bool) string {
	if !solo {
		return ""
	}
	return "WHERE con_recibo < facturas"
}
