package cxp

// Validación de área POR RIESGO (migraciones 0061/0062).
//
// La regla es al revés de como estaba: nada requiere que el área confirme la conformidad, salvo que
// la factura dispare un criterio de riesgo. El veredicto se calcula UNA vez —al revisar— y se
// guarda en la factura junto con el motivo; no se recalcula al leer. Si mañana se sube el umbral,
// una factura que ya pasó por validación no puede dejar de decir que la necesitaba.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// Motivos por los que una factura sí requiere validación de área.
const (
	MotivoMonto           = "MONTO"
	MotivoProveedorNuevo  = "PROVEEDOR_NUEVO"
	MotivoDesvio          = "DESVIO"
	MotivoValidacionVacio = ""
)

// EtiquetaMotivoValidacion explica el motivo en una frase, para la pantalla y la auditoría.
func EtiquetaMotivoValidacion(m string) string {
	switch m {
	case MotivoMonto:
		return "supera el umbral de monto"
	case MotivoProveedorNuevo:
		return "proveedor nuevo o esporádico"
	case MotivoDesvio:
		return "se aparta del histórico de este proveedor"
	default:
		return ""
	}
}

// EvaluarValidacion decide si la factura necesita validación de área y guarda el veredicto.
//
// El cálculo va entero en SQL —una sola ida— porque necesita agregados del histórico del proveedor
// que sería absurdo traer a Go. Devuelve el motivo ("" = no la requiere) para que el servicio pueda
// contarlo en la auditoría.
//
// La factura de un proveedor NUEVO se detecta contando su historial SIN incluir la propia factura:
// si no, un proveedor con una sola factura (la que se está evaluando) daría 1 y no 0.
func (r *pgRepository) EvaluarValidacion(ctx context.Context, empresaID, docID string) (string, error) {
	const q = `
		WITH params AS (
		    SELECT MAX(valor) FILTER (WHERE clave = 'VALIDACION_UMBRAL_MONTO')::numeric      AS umbral,
		           MAX(valor) FILTER (WHERE clave = 'VALIDACION_PROVEEDOR_NUEVO_MAX')::int   AS max_nuevo,
		           MAX(valor) FILTER (WHERE clave = 'VALIDACION_DESVIO_PCT')::numeric        AS desvio_pct,
		           MAX(valor) FILTER (WHERE clave = 'VALIDACION_DESVIO_PISO_MONTO')::numeric AS desvio_piso
		    FROM cxp_parametro WHERE empresa_id = $1::uuid
		),
		doc AS (
		    SELECT id, proveedor_id, total_crc FROM documento_cxp
		    WHERE empresa_id = $1::uuid AND id = $2::uuid
		),
		hist AS (
		    SELECT COUNT(*) AS facturas, AVG(total_crc) AS promedio
		    FROM documento_cxp h, doc
		    WHERE h.empresa_id = $1::uuid AND h.proveedor_id = doc.proveedor_id
		      AND h.tipo = 'CXP' AND h.id <> doc.id
		),
		veredicto AS (
		    SELECT doc.id,
		           CASE
		               WHEN doc.total_crc > COALESCE(p.umbral, 0) THEN 'MONTO'
		               WHEN COALESCE(h.facturas, 0) <= COALESCE(p.max_nuevo, 0) THEN 'PROVEEDOR_NUEVO'
		               WHEN COALESCE(p.desvio_pct, 0) > 0 AND COALESCE(h.promedio, 0) > 0
		                    AND doc.total_crc > COALESCE(p.desvio_piso, 0)
		                    AND ABS(doc.total_crc - h.promedio) > h.promedio * p.desvio_pct / 100 THEN 'DESVIO'
		               ELSE ''
		           END AS motivo
		    FROM doc CROSS JOIN params p CROSS JOIN hist h
		)
		UPDATE documento_cxp d
		SET requiere_validacion = (v.motivo <> ''),
		    validacion_motivo   = NULLIF(v.motivo, '')
		FROM veredicto v
		WHERE d.id = v.id
		RETURNING COALESCE(d.validacion_motivo, '')`
	var motivo string
	if err := r.pool.QueryRow(ctx, q, empresaID, docID).Scan(&motivo); err != nil {
		return "", fmt.Errorf("cxp: evaluar validación por riesgo: %w", err)
	}
	return motivo, nil
}

// ParametrosValidacion trae los umbrales vigentes de la empresa, para mostrarlos y editarlos.
func (r *pgRepository) ParametrosValidacion(ctx context.Context, empresaID string) ([]ParametroCxP, error) {
	const q = `SELECT clave, valor, descripcion FROM cxp_parametro
	           WHERE empresa_id = $1::uuid ORDER BY clave`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: parámetros de validación: %w", err)
	}
	defer rows.Close()
	out := []ParametroCxP{}
	for rows.Next() {
		var p ParametroCxP
		if err := rows.Scan(&p.Clave, &p.Valor, &p.Descripcion); err != nil {
			return nil, fmt.Errorf("cxp: scan parámetro: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// EfectoValidacion mide a cuánto gasto le está pidiendo confirmación la regla vigente.
//
// Sin este número la pantalla de umbrales es un formulario a ciegas: subir el umbral de ₡100.000 a
// ₡1.000.000 se ve igual de inofensivo en los dos casos, y no lo es. Se mide sobre las facturas YA
// EVALUADAS (las que pasaron por revisión), que son las únicas de las que se sabe el veredicto.
func (r *pgRepository) EfectoValidacion(ctx context.Context, empresaID string) (EfectoValidacion, error) {
	const q = `
		SELECT COALESCE(validacion_motivo, '')            AS motivo,
		       COUNT(*)::int                              AS cantidad,
		       COALESCE(SUM(total_crc), 0)::text          AS monto
		FROM documento_cxp
		WHERE empresa_id = $1::uuid AND requiere_validacion IS NOT NULL
		GROUP BY COALESCE(validacion_motivo, '')`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return EfectoValidacion{}, fmt.Errorf("cxp: efecto de la validación: %w", err)
	}
	defer rows.Close()
	out := EfectoValidacion{PorMotivo: []EfectoMotivo{}}
	total, requieren := decimal.Zero, decimal.Zero
	for rows.Next() {
		var m EfectoMotivo
		if err := rows.Scan(&m.Motivo, &m.Cantidad, &m.Monto); err != nil {
			return EfectoValidacion{}, fmt.Errorf("cxp: scan efecto: %w", err)
		}
		monto, err := decimal.NewFromString(m.Monto)
		if err != nil {
			return EfectoValidacion{}, fmt.Errorf("cxp: monto del efecto (%s): %w", m.Motivo, err)
		}
		out.Total += m.Cantidad
		total = total.Add(monto)
		if m.Motivo != MotivoValidacionVacio {
			m.Etiqueta = EtiquetaMotivoValidacion(m.Motivo)
			out.PorMotivo = append(out.PorMotivo, m)
			out.Requieren += m.Cantidad
			requieren = requieren.Add(monto)
		}
	}
	if err := rows.Err(); err != nil {
		return EfectoValidacion{}, fmt.Errorf("cxp: efecto de la validación: %w", err)
	}
	out.TotalMonto = total.StringFixed(2)
	out.RequierenMonto = requieren.StringFixed(2)
	return out, nil
}

// GuardarParametroValidacion actualiza un umbral. Solo actualiza los que ya existen: la clave la
// define la migración, no el cliente, así que un nombre inventado no crea un parámetro fantasma.
func (r *pgRepository) GuardarParametroValidacion(ctx context.Context, empresaID, clave, valor, usuarioID string) (int64, error) {
	const q = `UPDATE cxp_parametro
	           SET valor = $3, actualizado_en = now(), actualizado_por = NULLIF($4,'')::uuid
	           WHERE empresa_id = $1::uuid AND clave = $2`
	tag, err := r.pool.Exec(ctx, q, empresaID, clave, valor, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("cxp: guardar parámetro de validación: %w", err)
	}
	return tag.RowsAffected(), nil
}
