package cxp

// Marca «de Contabilidad»: las facturas que no tienen área operativa que las valide.
//
// Se marca en cuatro lugares y todos escriben la MISMA columna `es_contabilidad` de su tabla; la
// resolución (quién gana) vive en un solo lugar: `contabilidadOrigenSQL` del SELECT común.

import (
	"context"
	"fmt"
)

// MarcarDocumentoContabilidad fija el override de UNA factura.
//
// `valor` es de tres estados: true fuerza «de Contabilidad», false fuerza que la valide el área
// (aunque el proveedor o el rubro estén marcados), y nil vuelve a heredar del catálogo.
//
// Solo antes de aprobar: después la marca ya no cambia nada del flujo y reescribirla solo
// ensuciaría el rastro de por qué esa factura se aprobó sin pasar por el área.
func (r *pgRepository) MarcarDocumentoContabilidad(ctx context.Context, empresaID, docID string, valor *bool, motivo, usuarioID string) (int64, error) {
	const q = `
		UPDATE documento_cxp
		SET es_contabilidad = $3,
		    contabilidad_motivo = NULLIF($4, ''),
		    contabilidad_marcado_por = NULLIF($5, '')::uuid,
		    contabilidad_marcado_en = now(),
		    actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		  AND estado IN ('RECIBIDO', 'REVISADO', 'VALIDADO_DEPTO')`
	tag, err := r.pool.Exec(ctx, q, empresaID, docID, valor, motivo, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("cxp: marcar documento de contabilidad: %w", err)
	}
	return tag.RowsAffected(), nil
}

// SellarContabilidad graba en la factura el hecho de que se aprobó por la vía de Contabilidad.
//
// Se llama al aprobar, y solo cuando la marca venía HEREDADA del proveedor o del rubro: convierte
// esa herencia —que se recalcula al leer y podría cambiar mañana— en un hecho fijo de esta factura.
// Sin esto, desmarcar el proveedor haría que una factura ya pagada dejara de decir por qué se
// aprobó sin validación de área, y el rastro se perdería justo en el documento que lo necesita.
//
// No lleva `contabilidad_marcado_por`: nadie tomó una decisión sobre ESTA factura, se heredó. Poner
// al aprobador ahí lo haría parecer el marcador y rompería la regla de segregación.
func (r *pgRepository) SellarContabilidad(ctx context.Context, empresaID, docID, motivo string) error {
	const q = `
		UPDATE documento_cxp
		SET es_contabilidad = true,
		    contabilidad_motivo = COALESCE(NULLIF(contabilidad_motivo, ''), NULLIF($3, '')),
		    contabilidad_marcado_en = COALESCE(contabilidad_marcado_en, now())
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND es_contabilidad IS NULL`
	if _, err := r.pool.Exec(ctx, q, empresaID, docID, motivo); err != nil {
		return fmt.Errorf("cxp: sellar marca de contabilidad: %w", err)
	}
	return nil
}

// MarcarProveedorContabilidad marca (o desmarca) al proveedor. Es la marca que captura el
// «siempre»: a partir de acá sus facturas nacen sin necesidad de validación de área.
func (r *pgRepository) MarcarProveedorContabilidad(ctx context.Context, empresaID, proveedorID string, valor bool) (int64, error) {
	const q = `UPDATE proveedor SET es_contabilidad = $3, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, proveedorID, valor)
	if err != nil {
		return 0, fmt.Errorf("cxp: marcar proveedor de contabilidad: %w", err)
	}
	return tag.RowsAffected(), nil
}

// MarcarConceptoContabilidad marca (o desmarca) un rubro completo del catálogo.
func (r *pgRepository) MarcarConceptoContabilidad(ctx context.Context, empresaID, conceptoID string, valor bool) (int64, error) {
	const q = `UPDATE concepto SET es_contabilidad = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, conceptoID, valor)
	if err != nil {
		return 0, fmt.Errorf("cxp: marcar concepto de contabilidad: %w", err)
	}
	return tag.RowsAffected(), nil
}

// MarcarClasificacionContabilidad marca (o desmarca) el nivel fino del catálogo.
func (r *pgRepository) MarcarClasificacionContabilidad(ctx context.Context, empresaID, clasificacionID string, valor bool) (int64, error) {
	const q = `UPDATE clasificacion SET es_contabilidad = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, clasificacionID, valor)
	if err != nil {
		return 0, fmt.Errorf("cxp: marcar clasificación de contabilidad: %w", err)
	}
	return tag.RowsAffected(), nil
}

// MarcasContabilidad lista lo que está marcado hoy, para que la pantalla de configuración muestre
// el cuadro completo en vez de obligar a abrir proveedor por proveedor.
//
// Incluye lo INACTIVO a propósito, señalado como tal: desactivar un proveedor o un rubro NO le quita
// la marca, y filtrarlos escondía excepciones que seguían vigentes —un proveedor inactivo puede
// tener facturas abiertas—. Una excepción que nadie ve es una excepción que nadie audita.
func (r *pgRepository) MarcasContabilidad(ctx context.Context, empresaID string) (MarcasContabilidad, error) {
	var m MarcasContabilidad
	m.Proveedores = []MarcaContabilidad{}
	m.Conceptos = []MarcaContabilidad{}
	m.Clasificaciones = []MarcaContabilidad{}

	const qProv = `SELECT id::text, nombre, activo FROM proveedor
	               WHERE empresa_id = $1::uuid AND es_contabilidad ORDER BY activo DESC, nombre`
	rows, err := r.pool.Query(ctx, qProv, empresaID)
	if err != nil {
		return m, fmt.Errorf("cxp: marcas de proveedores: %w", err)
	}
	for rows.Next() {
		var x MarcaContabilidad
		if err := rows.Scan(&x.ID, &x.Nombre, &x.Activo); err != nil {
			rows.Close()
			return m, fmt.Errorf("cxp: scan marca proveedor: %w", err)
		}
		m.Proveedores = append(m.Proveedores, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return m, err
	}

	const qCon = `SELECT id::text, nombre, activo FROM concepto
	              WHERE empresa_id = $1::uuid AND es_contabilidad ORDER BY activo DESC, nombre`
	rows, err = r.pool.Query(ctx, qCon, empresaID)
	if err != nil {
		return m, fmt.Errorf("cxp: marcas de conceptos: %w", err)
	}
	for rows.Next() {
		var x MarcaContabilidad
		if err := rows.Scan(&x.ID, &x.Nombre, &x.Activo); err != nil {
			rows.Close()
			return m, fmt.Errorf("cxp: scan marca concepto: %w", err)
		}
		m.Conceptos = append(m.Conceptos, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return m, err
	}

	// La clasificación se nombra con su concepto: el mismo nombre puede existir en dos rubros.
	const qCla = `SELECT cl.id::text, cl.nombre, co.nombre, cl.activo
	              FROM clasificacion cl JOIN concepto co ON co.id = cl.concepto_id
	              WHERE cl.empresa_id = $1::uuid AND cl.es_contabilidad
	              ORDER BY cl.activo DESC, co.nombre, cl.nombre`
	rows, err = r.pool.Query(ctx, qCla, empresaID)
	if err != nil {
		return m, fmt.Errorf("cxp: marcas de clasificaciones: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var x MarcaContabilidad
		if err := rows.Scan(&x.ID, &x.Nombre, &x.Concepto, &x.Activo); err != nil {
			return m, fmt.Errorf("cxp: scan marca clasificación: %w", err)
		}
		m.Clasificaciones = append(m.Clasificaciones, x)
	}
	return m, rows.Err()
}
