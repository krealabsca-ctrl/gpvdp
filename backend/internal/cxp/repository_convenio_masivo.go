package cxp

// Consultas del marcado masivo del convenio (21-oct-2026). El porqué está en convenio_masivo.go.

import (
	"context"
	"fmt"
)

// CandidatosConvenio lista los proveedores que declaran contado en sus comprobantes.
//
// EL CRITERIO ES LO QUE DECLARARON, no lo que la ficha dice. Un proveedor cuya ficha ya está en
// CRÉDITO pero que sigue mandando comprobantes «contado» es exactamente el caso del pedido: hoy
// sus facturas nacerían bloqueadas una por una.
//
// `soloPendientes` deja fuera a los que ya tienen las dos condiciones puestas: son los que ya no
// tienen nada que hacer, y mezclarlos obliga a buscar los que sí entre los que no.
func (r *pgRepository) CandidatosConvenio(ctx context.Context, empresaID string, soloPendientes bool) ([]CandidatoConvenio, error) {
	const q = `
		SELECT p.id::text, p.nombre,
		       COALESCE(p.condicion_pago, ''), COALESCE(p.convenio_manda, 'XML'),
		       COUNT(d.id) FILTER (WHERE d.descripcion ILIKE '%Condición: contado%')::int,
		       COUNT(d.id) FILTER (WHERE d.bloqueado_para_pago)::int,
		       COALESCE(SUM(d.total_crc) FILTER (WHERE d.bloqueado_para_pago), 0)::text
		FROM proveedor p
		LEFT JOIN documento_cxp d ON d.proveedor_id = p.id AND d.empresa_id = $1::uuid
		WHERE p.empresa_id = $1::uuid AND p.activo
		GROUP BY p.id, p.nombre, p.condicion_pago, p.convenio_manda
		HAVING COUNT(d.id) FILTER (WHERE d.descripcion ILIKE '%Condición: contado%') > 0
		-- Los que más facturas de contado traen primero: ahí es donde marcar ahorra más trabajo.
		ORDER BY COUNT(d.id) FILTER (WHERE d.descripcion ILIKE '%Condición: contado%') DESC, p.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: candidatos de convenio: %w", err)
	}
	defer rows.Close()

	out := make([]CandidatoConvenio, 0)
	for rows.Next() {
		var c CandidatoConvenio
		if err := rows.Scan(&c.ProveedorID, &c.Nombre, &c.CondicionPago, &c.ConvenioManda,
			&c.FacturasContado, &c.Bloqueadas, &c.MontoBloqueado); err != nil {
			return nil, fmt.Errorf("cxp: scan candidato: %w", err)
		}
		// Las dos banderas se derivan acá y no en la pantalla: la regla de cuándo el candado se
		// levanta vive en el INSERT del documento, y tenerla escrita en dos lenguajes es la forma
		// de que una de las dos se quede atrás.
		c.YaListo = c.ConvenioManda == "PROVEEDOR" && c.CondicionPago == "CREDITO"
		c.FaltaCredito = c.ConvenioManda == "PROVEEDOR" && c.CondicionPago != "CREDITO"
		if soloPendientes && c.YaListo {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarcarConvenioMasivo pone el convenio a varios proveedores y, si se pidió, completa la segunda
// condición pasando a CRÉDITO los que estaban en CONTADO.
//
// ── UNA SOLA CONSULTA, NO DOS ───────────────────────────────────────────────────────────────
//
// Las dos condiciones del candado se escriben juntas o no se escriben: dejar el convenio puesto y
// la condición sin cambiar es exactamente el estado «a medias» que este cambio viene a eliminar, y
// sería peor llegar a él por un error en medio de la operación que por diseño. El CTE también
// resuelve que haya que saber el estado PREVIO de cada ficha —quién estaba ya en crédito— después
// de haberlo pisado.
//
// Devuelve los nombres en dos cubetas distintas: los que pasó a crédito (un cambio en la condición
// comercial, que hay que decir) y los que quedaron a medias cuando NO se pidió pasarlos, donde el
// aviso de siempre sigue siendo lo correcto.
func (r *pgRepository) MarcarConvenioMasivo(ctx context.Context, empresaID string, ids []string, convenio string, pasarACredito bool) (ResultadoConvenioMasivo, error) {
	res := ResultadoConvenioMasivo{
		SinCredito: []string{}, PasadosACredito: []string{}, PasadosACreditoIDs: []string{},
	}

	rows, err := r.pool.Query(ctx, `
		WITH previo AS (
		    SELECT id, nombre, COALESCE(condicion_pago, '') = 'CREDITO' AS ya_en_credito
		      FROM proveedor
		     WHERE empresa_id = $1::uuid AND id = ANY($2::uuid[]) AND activo
		), act AS (
		    UPDATE proveedor p
		       SET convenio_manda = $3,
		           -- Solo con PROVEEDOR: con XML la condición de la ficha no interviene en el
		           -- candado y cambiarla sería tocar algo que nadie pidió.
		           condicion_pago = CASE WHEN $4::bool AND $3 = 'PROVEEDOR' THEN 'CREDITO'
		                                 ELSE p.condicion_pago END,
		           actualizado_en = now()
		      FROM previo
		     WHERE p.id = previo.id
		    RETURNING p.id
		)
		SELECT previo.id::text, previo.nombre, previo.ya_en_credito
		  FROM previo JOIN act ON act.id = previo.id
		 ORDER BY previo.nombre`,
		empresaID, ids, convenio, pasarACredito)
	if err != nil {
		return ResultadoConvenioMasivo{}, fmt.Errorf("cxp: marcar convenio: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, nombre string
		var yaEnCredito bool
		if err := rows.Scan(&id, &nombre, &yaEnCredito); err != nil {
			return ResultadoConvenioMasivo{}, fmt.Errorf("cxp: scan marcar convenio: %w", err)
		}
		res.Actualizados++
		// Con XML no hay nada que contar: la condición de la ficha no interviene.
		if convenio != "PROVEEDOR" || yaEnCredito {
			continue
		}
		if pasarACredito {
			res.PasadosACredito = append(res.PasadosACredito, nombre)
			res.PasadosACreditoIDs = append(res.PasadosACreditoIDs, id)
		} else {
			res.SinCredito = append(res.SinCredito, nombre)
		}
	}
	return res, rows.Err()
}

// BloqueadasDeProveedores cuenta lo que hoy está trancado para un conjunto de proveedores.
//
// Es lo que se muestra ANTES de liberar: cuántas facturas y por cuánto. Liberar es levantar la
// única barrera que queda contra el doble pago, así que el monto se ve antes de decidir, no
// después.
func (r *pgRepository) BloqueadasDeProveedores(ctx context.Context, empresaID string, ids []string) (int, string, error) {
	var n int
	var monto string
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int, COALESCE(SUM(total_crc), 0)::text
		FROM documento_cxp
		WHERE empresa_id = $1::uuid AND proveedor_id = ANY($2::uuid[]) AND bloqueado_para_pago`,
		empresaID, ids).Scan(&n, &monto)
	if err != nil {
		return 0, "", fmt.Errorf("cxp: bloqueadas de proveedores: %w", err)
	}
	return n, monto, nil
}

// LiberarDeProveedores levanta el candado de todo lo bloqueado de esos proveedores.
//
// Deja la huella igual que la liberación de a una (mig 0097): quién, cuándo y POR QUÉ. Sin el
// motivo, dentro de seis meses nadie distingue la factura que el emisor marcó mal de la que
// alguien liberó por error y se pagó dos veces.
func (r *pgRepository) LiberarDeProveedores(ctx context.Context, empresaID string, ids []string, motivo, usuarioID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE documento_cxp
		SET bloqueado_para_pago = false,
		    desbloqueado_por = NULLIF($4,'')::uuid,
		    desbloqueado_en = now(),
		    desbloqueo_motivo = $3,
		    actualizado_en = now()
		WHERE empresa_id = $1::uuid AND proveedor_id = ANY($2::uuid[]) AND bloqueado_para_pago`,
		empresaID, ids, motivo, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("cxp: liberar de proveedores: %w", err)
	}
	return tag.RowsAffected(), nil
}
