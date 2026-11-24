package cxp

// Lo que hace falta para el aviso de pago y que NO está en las filas que van al banco: el
// concepto que propone el catálogo, los consecutivos de las facturas, y el texto que alguien
// escribió. El porqué está en aviso_pago.go y en la migración 0117.

import (
	"context"
	"fmt"
	"strings"
)

// conceptoDelAvisoSQL es la hoja del catálogo de gasto, la más específica que exista.
//
// Se prefiere la CLASIFICACIÓN sobre el concepto porque es la que nombra la cosa comprada
// («Suministro de Gas»); el concepto es la familia («Gastos») y en el cuadro no dice nada.
const conceptoDelAvisoSQL = `COALESCE(NULLIF(cl.nombre, ''), NULLIF(c.nombre, ''), '')`

// EncabezadoDelAviso resuelve los dos nombres de la línea de contexto: de qué empresa sale el
// papel y quién lo emitió.
//
// Ninguno está en el token —lleva ids y el correo, no nombres— y un aviso que se manda por fuera
// del sistema sin decir de quién sale no se puede citar después. Si el usuario no se encuentra,
// devuelve vacío en vez de fallar: perder la línea de contexto es molesto, no poder emitir el
// papel es un problema.
func (r *pgRepository) EncabezadoDelAviso(ctx context.Context, empresaID, usuarioID string) (string, string, error) {
	const q = `
		SELECT e.nombre,
		       COALESCE((SELECT u.nombre FROM usuario u WHERE u.id = NULLIF($2, '')::uuid), '')
		FROM empresa e WHERE e.id = $1::uuid`
	var empresa, usuario string
	if err := r.pool.QueryRow(ctx, q, empresaID, usuarioID).Scan(&empresa, &usuario); err != nil {
		return "", "", fmt.Errorf("cxp: encabezado del aviso: %w", err)
	}
	return empresa, usuario, nil
}

// FirmasDelLote arma las cuatro líneas del papel desde lo que el sistema REGISTRÓ.
//
// Nadie las teclea: es la diferencia con el Excel que este papel reemplaza, donde los cuatro
// nombres se escribían a mano. Acá el papel no puede afirmar que alguien aprobó algo que no aprobó.
//
// CADA LÍNEA SALE DE DONDE DE VERDAD ESTÁ EL DATO, que no es el mismo lugar para las cuatro:
//
//   - Solicitado y Pagado salen de la FICHA DEL CORTE (`creado_por`, `cerrado_por`): el corte lo
//     arma una persona y lo cierra una, y eso ya está en su propia fila.
//   - Revisado y Aprobado salen de la AUDITORÍA de las facturas que lo componen, porque son pasos
//     de cada factura y no del corte. Un corte de cien facturas suele pasar por varios revisores:
//     por eso se agregan los nombres distintos con « / », como en el papel que se usaba.
//
// Devuelve SIEMPRE las cuatro, en orden, aunque alguna quede vacía. Un papel al que le falta la
// línea de «Aprobado por» se lee distinto de uno donde esa línea está en blanco.
func (r *pgRepository) FirmasDelLote(ctx context.Context, empresaID, loteID string) ([]Firma, string, error) {
	// `fecha_pago` es el cierre del corte: cuándo salió el dinero. Mientras no se cierre va vacía,
	// y el papel lo muestra así en vez de poner la fecha de hoy —que sería afirmar que ya se pagó—.
	const q = `
		WITH facturas AS (
		    SELECT d.id FROM documento_cxp d
		     WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid
		), pasos AS (
		    SELECT a.accion,
		           string_agg(DISTINCT COALESCE(NULLIF(u.nombre, ''), u.email), ' / ') AS nombres,
		           to_char(MAX(a.ts) AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD') AS fecha
		      FROM auditoria_evento a
		      JOIN facturas f ON f.id = a.entidad_id
		      LEFT JOIN usuario u ON u.id = a.usuario_id
		     WHERE a.empresa_id = $1::uuid AND a.entidad = 'documento_cxp'
		       AND a.accion IN ('REVISAR_DOCUMENTO', 'APROBAR_DOCUMENTO')
		     GROUP BY a.accion
		)
		SELECT
		    COALESCE((SELECT COALESCE(NULLIF(uc.nombre, ''), uc.email) FROM usuario uc WHERE uc.id = l.creado_por), ''),
		    COALESCE(to_char(l.creado_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD'), ''),
		    COALESCE((SELECT nombres FROM pasos WHERE accion = 'REVISAR_DOCUMENTO'), ''),
		    COALESCE((SELECT fecha   FROM pasos WHERE accion = 'REVISAR_DOCUMENTO'), ''),
		    COALESCE((SELECT nombres FROM pasos WHERE accion = 'APROBAR_DOCUMENTO'), ''),
		    COALESCE((SELECT fecha   FROM pasos WHERE accion = 'APROBAR_DOCUMENTO'), ''),
		    COALESCE((SELECT COALESCE(NULLIF(uz.nombre, ''), uz.email) FROM usuario uz WHERE uz.id = l.cerrado_por), ''),
		    COALESCE(to_char(l.cerrado_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD'), '')
		FROM lote_pago l
		WHERE l.empresa_id = $1::uuid AND l.id = $2::uuid`
	var solN, solF, revN, revF, aprN, aprF, pagN, pagF string
	err := r.pool.QueryRow(ctx, q, empresaID, loteID).
		Scan(&solN, &solF, &revN, &revF, &aprN, &aprF, &pagN, &pagF)
	if err != nil {
		return nil, "", fmt.Errorf("cxp: firmas del corte: %w", err)
	}
	return []Firma{
		{Rol: RolSolicitado, Nombre: solN, Fecha: solF},
		{Rol: RolRevisado, Nombre: revN, Fecha: revF},
		{Rol: RolAprobado, Nombre: aprN, Fecha: aprF},
		{Rol: RolPagado, Nombre: pagN, Fecha: pagF},
	}, pagF, nil
}

// DatosDeAvisoDelLote trae, por proveedor, el concepto propuesto y los consecutivos del corte.
//
// EL CONCEPTO SOLO SE PROPONE SI ES UNO SOLO. Si el proveedor trae facturas de rubros distintos,
// el campo va vacío para que lo escriba una persona: poner el del primero diría que todo el pago
// es de ese rubro cuando la mitad es de otro. Es la misma regla que ya gobierna la nomenclatura
// del pago agrupado (`codigoPagoSQL`), y por la misma razón.
func (r *pgRepository) DatosDeAvisoDelLote(ctx context.Context, empresaID, loteID string) (map[string]DatosDelAviso, error) {
	const q = `
		SELECT d.proveedor_id::text,
		       CASE WHEN COUNT(DISTINCT ` + conceptoDelAvisoSQL + `) = 1
		            THEN MIN(` + conceptoDelAvisoSQL + `) ELSE '' END,
		       COALESCE(string_agg(NULLIF(d.consecutivo, ''), ' ' ORDER BY d.consecutivo), '')
		FROM documento_cxp d
		LEFT JOIN concepto c ON c.id = d.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = d.clasificacion_id
		WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid
		  AND NOT d.bloqueado_para_pago
		GROUP BY d.proveedor_id`
	rows, err := r.pool.Query(ctx, q, empresaID, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: datos del aviso: %w", err)
	}
	defer rows.Close()
	out := map[string]DatosDelAviso{}
	for rows.Next() {
		var provID, concepto, consecutivos string
		if err := rows.Scan(&provID, &concepto, &consecutivos); err != nil {
			return nil, fmt.Errorf("cxp: scan datos del aviso: %w", err)
		}
		d := DatosDelAviso{ConceptoPropuesto: strings.TrimSpace(concepto)}
		if c := strings.TrimSpace(consecutivos); c != "" {
			d.Consecutivos = strings.Fields(c)
		}
		out[provID] = d
	}
	return out, rows.Err()
}

// TextosDeAvisoDelLote trae lo que alguien escribió para ese corte, por proveedor.
//
// Devuelve PUNTEROS para distinguir «no se tocó» de «se borró a propósito»: con cadenas pelados
// las dos cosas serían "" y una corrección que vacía el campo volvería a llenarse sola con la
// propuesta en la siguiente descarga.
func (r *pgRepository) TextosDeAvisoDelLote(ctx context.Context, empresaID, loteID string) (map[string]TextoAviso, error) {
	const q = `
		SELECT proveedor_id::text, concepto, documentos
		FROM aviso_pago_texto
		WHERE empresa_id = $1::uuid AND lote_id = $2::uuid`
	rows, err := r.pool.Query(ctx, q, empresaID, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: textos del aviso: %w", err)
	}
	defer rows.Close()
	out := map[string]TextoAviso{}
	for rows.Next() {
		var provID string
		var t TextoAviso
		if err := rows.Scan(&provID, &t.Concepto, &t.Documentos); err != nil {
			return nil, fmt.Errorf("cxp: scan textos del aviso: %w", err)
		}
		out[provID] = t
	}
	return out, rows.Err()
}

// GuardarTextoDeAviso deja el texto corregido de un proveedor en un corte.
//
// El proveedor tiene que estar EN ESE CORTE: sin esa comprobación se podrían sembrar textos de
// proveedores ajenos al corte, que no se verían en ninguna pantalla y aparecerían solos el día
// que ese proveedor entrara a un corte con el mismo id. Devuelve si encontró a quién aplicarlo.
func (r *pgRepository) GuardarTextoDeAviso(ctx context.Context, empresaID, loteID, proveedorID string, t TextoAviso, usuarioID string) (bool, error) {
	const q = `
		INSERT INTO aviso_pago_texto (empresa_id, lote_id, proveedor_id, concepto, documentos, actualizado_por)
		SELECT $1::uuid, $2::uuid, $3::uuid, $4, $5, $6::uuid
		 WHERE EXISTS (SELECT 1 FROM documento_cxp d
		                WHERE d.empresa_id = $1::uuid AND d.lote_id = $2::uuid
		                  AND d.proveedor_id = $3::uuid AND NOT d.bloqueado_para_pago)
		ON CONFLICT (empresa_id, lote_id, proveedor_id) DO UPDATE
		   SET concepto        = EXCLUDED.concepto,
		       documentos      = EXCLUDED.documentos,
		       actualizado_por = EXCLUDED.actualizado_por,
		       actualizado_en  = now()`
	tag, err := r.pool.Exec(ctx, q, empresaID, loteID, proveedorID, t.Concepto, t.Documentos, usuarioID)
	if err != nil {
		return false, fmt.Errorf("cxp: guardar texto del aviso: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
