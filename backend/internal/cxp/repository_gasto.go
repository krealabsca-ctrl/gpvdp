package cxp

import (
	"context"
	"fmt"
)

// baseGastoSQL es el universo del análisis: una línea de factura con el tipo de cambio ya resuelto.
//
// `tc` queda en NULL cuando la factura es en moneda extranjera y NO trae tipo de cambio. Ese NULL
// se propaga a propósito: una multiplicación por NULL da NULL y la línea no entra en ninguna suma,
// que es lo correcto —no se sabe cuánto costó en colones— pero además se CUENTA aparte y se
// muestra. Si se hubiera usado COALESCE(tc, 0) la línea valdría cero y bajaría el gasto del ítem
// sin que nada lo delate.
//
// ANULADO y DENEGADO quedan fuera: no son gasto. El resto de los estados sí entra, incluido
// RECIBIDO, porque la factura ya existe y el gasto ya se incurrió aunque todavía no se haya pagado.
const baseGastoSQL = `
	SELECT l.cabys, l.detalle, l.detalle_norm, l.unidad_medida, l.cantidad,
	       l.precio_unitario, l.monto_total_linea, l.documento_id, l.linea,
	       d.proveedor_id, d.fecha_emision,
	       CASE WHEN d.moneda = 'CRC' THEN 1::numeric
	            WHEN d.tc_aplicado > 0 THEN d.tc_aplicado
	       END AS tc
	FROM cxp_linea l
	JOIN documento_cxp d ON d.id = l.documento_id AND d.empresa_id = l.empresa_id
	WHERE l.empresa_id = $1::uuid
	  AND d.estado NOT IN ('ANULADO', 'DENEGADO')
	  AND ($2 = '' OR d.fecha_emision >= $2::date)
	  AND ($3 = '' OR d.fecha_emision <= $3::date)
	  AND ($4 = '' OR d.proveedor_id = $4::uuid)
	  AND ($5 = '' OR l.detalle_norm LIKE '%' || $5 || '%' OR l.cabys LIKE $5 || '%')`

// ItemsDeGasto agrupa el detalle por CABYS.
func (r *pgRepository) ItemsDeGasto(ctx context.Context, empresaID string, f FiltrosGasto) ([]ItemGasto, string, int, error) {
	// La comparación de precios se apaga cuando hay más de una unidad de medida: el precio del
	// kilo contra el de la unidad daría una "variación" de miles por ciento que no significa nada
	// y mandaría a negociar un ítem que nunca tuvo problema. La unidad vacía no cuenta como una
	// unidad distinta —muchos emisores no la declaran— pero tampoco impide comparar.
	q := `WITH base AS (` + baseGastoSQL + `
	), g AS (
	  SELECT cabys,
	         mode() WITHIN GROUP (ORDER BY detalle)        AS descripcion,
	         count(DISTINCT detalle_norm)                  AS descripciones,
	         COALESCE(sum(monto_total_linea * tc), 0)      AS gasto,
	         count(*)                                      AS lineas,
	         count(DISTINCT documento_id)                  AS facturas,
	         count(DISTINCT proveedor_id)                  AS proveedores,
	         COALESCE(mode() WITHIN GROUP (ORDER BY NULLIF(unidad_medida,'')), '') AS unidad,
	         count(DISTINCT NULLIF(unidad_medida,''))      AS unidades,
	         COALESCE(sum(cantidad), 0)                    AS cantidad,
	         min(precio_unitario * tc)                     AS precio_min,
	         max(precio_unitario * tc)                     AS precio_max,
	         (array_remove(array_agg(
	             CASE WHEN tc IS NOT NULL THEN precio_unitario * tc END
	             ORDER BY fecha_emision DESC, linea DESC), NULL))[1] AS precio_ultimo,
	         max(fecha_emision)                            AS ultima_compra,
	         count(*) FILTER (WHERE tc IS NULL)            AS sin_convertir
	  FROM base
	  GROUP BY cabys
	)
	SELECT cabys, descripcion, descripciones, gasto::text, lineas, facturas, proveedores,
	       unidad, unidades,
	       CASE WHEN unidades <= 1 THEN COALESCE(cantidad::text,'') ELSE '' END,
	       CASE WHEN unidades <= 1 THEN COALESCE(precio_min::text,'') ELSE '' END,
	       CASE WHEN unidades <= 1 THEN COALESCE(precio_max::text,'') ELSE '' END,
	       CASE WHEN unidades <= 1 THEN COALESCE(precio_ultimo::text,'') ELSE '' END,
	       CASE WHEN unidades <= 1 AND precio_min > 0 AND precio_max > precio_min
	            THEN round((precio_max - precio_min) / precio_min * 100, 1)::text
	            ELSE '' END,
	       COALESCE(ultima_compra::text,''), sin_convertir,
	       COALESCE(sum(gasto) OVER (), 0)::text, count(*) OVER ()
	FROM g
	ORDER BY gasto DESC, lineas DESC
	LIMIT $6`

	limite := f.Limite
	if limite <= 0 || limite > 500 {
		limite = 200
	}
	rows, err := r.pool.Query(ctx, q, empresaID, f.Desde, f.Hasta, f.ProveedorID, f.Texto, limite)
	if err != nil {
		return nil, "", 0, fmt.Errorf("cxp: items de gasto: %w", err)
	}
	defer rows.Close()

	out := []ItemGasto{}
	total := "0"
	cuantos := 0
	for rows.Next() {
		var it ItemGasto
		if err := rows.Scan(&it.CABYS, &it.Descripcion, &it.Descripciones, &it.GastoCRC,
			&it.Lineas, &it.Facturas, &it.Proveedores, &it.Unidad, &it.Unidades,
			&it.Cantidad, &it.PrecioMin, &it.PrecioMax, &it.PrecioUltimo, &it.VariacionPct,
			&it.UltimaCompra, &it.SinConvertir, &total, &cuantos); err != nil {
			return nil, "", 0, fmt.Errorf("cxp: scan item de gasto: %w", err)
		}
		out = append(out, it)
	}
	return out, total, cuantos, rows.Err()
}

// CoberturaDeGasto dice sobre qué parte del gasto se hizo el análisis.
//
// Dos preguntas distintas en una sola pasada: cuántas FACTURAS del filtro tienen detalle cargado
// (y cuánto dinero representan), y cuántas LÍNEAS de las que sí hay quedaron sin CABYS o sin poder
// convertirse. La primera dice si el análisis es representativo; la segunda, si lo que muestra
// está completo.
func (r *pgRepository) CoberturaDeGasto(ctx context.Context, empresaID string, f FiltrosGasto) (CoberturaGasto, error) {
	const qDocs = `
	  SELECT count(*),
	         count(*) FILTER (WHERE detalle_lineas > 0),
	         COALESCE(sum(CASE WHEN moneda = 'CRC' THEN total
	                           WHEN tc_aplicado > 0 THEN total * tc_aplicado END), 0)::text,
	         COALESCE(sum(CASE WHEN detalle_lineas > 0 THEN
	                        CASE WHEN moneda = 'CRC' THEN total
	                             WHEN tc_aplicado > 0 THEN total * tc_aplicado END END), 0)::text
	  FROM documento_cxp
	  WHERE empresa_id = $1::uuid
	    AND estado NOT IN ('ANULADO', 'DENEGADO')
	    AND ($2 = '' OR fecha_emision >= $2::date)
	    AND ($3 = '' OR fecha_emision <= $3::date)
	    AND ($4 = '' OR proveedor_id = $4::uuid)`

	var c CoberturaGasto
	if err := r.pool.QueryRow(ctx, qDocs, empresaID, f.Desde, f.Hasta, f.ProveedorID).
		Scan(&c.Facturas, &c.FacturasConDet, &c.MontoTotal, &c.MontoConDetalle); err != nil {
		return CoberturaGasto{}, fmt.Errorf("cxp: cobertura de gasto: %w", err)
	}

	// El porcentaje se calcula en la base, sobre numeric, y no en Go a partir de los strings: ese
	// camino obligaría a convertir dinero a float para una división.
	const qPct = `SELECT CASE WHEN $2::numeric > 0
	                          THEN round($1::numeric / $2::numeric * 100, 1)::text
	                          ELSE '0' END`
	if err := r.pool.QueryRow(ctx, qPct, c.MontoConDetalle, c.MontoTotal).Scan(&c.PorcentajeMonto); err != nil {
		return CoberturaGasto{}, fmt.Errorf("cxp: porcentaje de cobertura: %w", err)
	}

	qLineas := `WITH base AS (` + baseGastoSQL + `)
	  SELECT count(*) FILTER (WHERE cabys = ''), count(*) FILTER (WHERE tc IS NULL) FROM base`
	if err := r.pool.QueryRow(ctx, qLineas, empresaID, f.Desde, f.Hasta, f.ProveedorID, f.Texto).
		Scan(&c.LineasSinCABYS, &c.LineasSinConvert); err != nil {
		return CoberturaGasto{}, fmt.Errorf("cxp: líneas sin clasificar: %w", err)
	}
	return c, nil
}

// ComprasDeItem lista cada compra de un CABYS, de la más reciente a la más vieja.
//
// Es la vista que permite contestar "¿por qué varió el precio?": se ve la fecha, el proveedor y la
// factura de cada compra, y desde ahí se puede ir al documento.
func (r *pgRepository) ComprasDeItem(ctx context.Context, empresaID, cabys string, f FiltrosGasto) ([]CompraDeItem, error) {
	q := `WITH base AS (` + baseGastoSQL + `
	)
	SELECT b.fecha_emision::text, COALESCE(p.nombre,''), b.proveedor_id::text,
	       b.documento_id::text, COALESCE(d.consecutivo,''), b.detalle, d.moneda,
	       COALESCE(b.cantidad::text,''), b.unidad_medida,
	       COALESCE((b.precio_unitario * b.tc)::text,''),
	       COALESCE((b.monto_total_linea * b.tc)::text,'')
	FROM base b
	JOIN documento_cxp d ON d.id = b.documento_id
	LEFT JOIN proveedor p ON p.id = b.proveedor_id
	WHERE b.cabys = $6
	ORDER BY b.fecha_emision DESC, b.linea
	LIMIT 500`

	rows, err := r.pool.Query(ctx, q, empresaID, f.Desde, f.Hasta, f.ProveedorID, f.Texto, cabys)
	if err != nil {
		return nil, fmt.Errorf("cxp: compras de item: %w", err)
	}
	defer rows.Close()

	out := []CompraDeItem{}
	for rows.Next() {
		var c CompraDeItem
		if err := rows.Scan(&c.Fecha, &c.Proveedor, &c.ProveedorID, &c.DocumentoID, &c.Consecutivo,
			&c.Detalle, &c.Moneda, &c.Cantidad, &c.Unidad, &c.PrecioUnitario, &c.Total); err != nil {
			return nil, fmt.Errorf("cxp: scan compra: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
