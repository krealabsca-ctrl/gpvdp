package cxp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const documentoCols = `d.id::text, d.proveedor_id::text, COALESCE(p.nombre, ''), d.clave, COALESCE(d.consecutivo, ''), d.tipo,
	to_char(d.fecha_emision, 'YYYY-MM-DD'), d.moneda, d.subtotal::text, d.iva::text, d.retencion::text,
	d.total::text, d.total_crc::text, d.estado, to_char(d.fecha_pago_programada, 'YYYY-MM-DD'),
	COALESCE(d.huella, ''), COALESCE(d.descripcion, ''),
	to_char(d.fecha_vencimiento, 'YYYY-MM-DD'),
	COALESCE(d.concepto_id::text, ''), COALESCE(c.nombre, ''),
	COALESCE(d.clasificacion_id::text, ''), COALESCE(cl.nombre, ''),
	COALESCE(d.subclasificacion_id::text, ''), COALESCE(sc.nombre, ''),
	COALESCE(d.lote_id::text, ''), COALESCE(lp.numero::text, ''),
	(cpz.id IS NOT NULL), to_char(d.comprobante_enviado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), d.clasif_auto,
	d.prioridad, COALESCE(d.nota_revision, ''),
	COALESCE(d.departamento_id::text, ''), COALESCE(dep.nombre, ''),
	COALESCE(d.validado_depto_por::text, ''), to_char(d.validado_depto_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
	COALESCE(d.validacion_respaldo, ''), COALESCE(NULLIF(uv.nombre, ''), uv.email, ''),
	COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa WHERE aa.factura_id = d.id AND aa.activo), 0)::numeric(14,2)::text,
	EXISTS (SELECT 1 FROM documento_cxp a2 WHERE a2.empresa_id = d.empresa_id AND a2.proveedor_id = d.proveedor_id
	        AND a2.tipo = 'ANTICIPO' AND a2.estado IN ('PAGADO','CONCILIADO') AND a2.moneda = 'CRC'
	        AND (a2.total_crc - COALESCE((SELECT SUM(x.monto_crc) FROM anticipo_aplicacion x WHERE x.anticipo_id = a2.id AND x.activo), 0)) > 0),
	` + contabilidadOrigenSQL + `, COALESCE(d.contabilidad_motivo, ''), COALESCE(d.contabilidad_marcado_por::text, ''),
	d.requiere_validacion, COALESCE(d.validacion_motivo, '')`

// contabilidadOrigenSQL resuelve la marca «de Contabilidad» en UNA expresión, y la resuelve acá
// —en el SELECT común— para que la Bandeja, el detalle y el candado de aprobación lean todos el
// mismo valor. Si cada uno la calculara, la pantalla mostraría una cosa y el candado haría otra.
//
// Precedencia: el override de la factura manda sobre el catálogo (así una factura de un rubro
// marcado puede volver a exigir validación de área, y viceversa). Dentro del catálogo se prefiere
// lo más específico: clasificación antes que concepto.
//
// La HERENCIA del catálogo solo se consulta mientras la factura está en el tramo donde la marca
// decide algo (RECIBIDO/REVISADO/VALIDADO_DEPTO). Una vez aprobada, el pasado no se reescribe: si
// se consultara igual, marcar hoy a un proveedor haría que una factura pagada el mes pasado
// —validada por su área, con su acta— apareciera de golpe como «de Contabilidad, sin validación de
// área». El hecho de que una factura aprobada SÍ pasó por esta vía queda sellado en su propia
// columna al aprobarla (ver `SellarContabilidad`), no inferido del catálogo de hoy.
const contabilidadOrigenSQL = `CASE
		WHEN d.es_contabilidad IS TRUE  THEN 'FACTURA'
		WHEN d.es_contabilidad IS FALSE THEN ''
		WHEN d.estado NOT IN ('RECIBIDO', 'REVISADO', 'VALIDADO_DEPTO') THEN ''
		WHEN p.es_contabilidad          THEN 'PROVEEDOR'
		WHEN COALESCE(cl.es_contabilidad, false) THEN 'CLASIFICACION'
		WHEN COALESCE(c.es_contabilidad, false)  THEN 'CONCEPTO'
		ELSE ''
	END`

// documentoFrom es el FROM común: proveedor (obligatorio) + catálogo de gasto (opcional, 3 niveles) + lote + comprobante.
const documentoFrom = `FROM documento_cxp d
	JOIN proveedor p ON p.id = d.proveedor_id
	LEFT JOIN concepto c ON c.id = d.concepto_id
	LEFT JOIN clasificacion cl ON cl.id = d.clasificacion_id
	LEFT JOIN subclasificacion sc ON sc.id = d.subclasificacion_id
	LEFT JOIN lote_pago lp ON lp.id = d.lote_id
	LEFT JOIN comprobante_pago cpz ON cpz.documento_id = d.id
	LEFT JOIN departamento dep ON dep.id = d.departamento_id
	LEFT JOIN usuario uv ON uv.id = d.validado_depto_por`

func scanDocumento(row scanner) (Documento, error) {
	var d Documento
	err := row.Scan(&d.ID, &d.ProveedorID, &d.Proveedor, &d.Clave, &d.Consecutivo, &d.Tipo, &d.FechaEmision,
		&d.Moneda, &d.Subtotal, &d.IVA, &d.Retencion, &d.Total, &d.TotalCRC, &d.Estado,
		&d.FechaPagoProgramada, &d.Huella, &d.Descripcion,
		&d.FechaVencimiento, &d.ConceptoID, &d.Concepto, &d.ClasificacionID, &d.Clasificacion,
		&d.SubclasificacionID, &d.Subclasificacion, &d.LoteID, &d.LoteNumero,
		&d.TieneComprobante, &d.ComprobanteEnviado, &d.ClasifAuto, &d.Prioridad, &d.NotaRevision,
		&d.DepartamentoID, &d.Departamento, &d.ValidadoDeptoPor, &d.ValidadoDeptoEn, &d.ValidacionRespaldo,
		&d.ValidadoDeptoPorNombre, &d.AnticiposAplicados, &d.ProveedorAnticipoDisponible,
		&d.ContabilidadOrigen, &d.ContabilidadMotivo, &d.ContabilidadMarcadoPor,
		&d.RequiereValidacion, &d.ValidacionMotivo)
	if err == nil {
		d.NetoCRC = netoCRC(d.TotalCRC, d.AnticiposAplicados)
		// Derivado del origen, nunca consultado aparte: así el booleano y el «por qué» no pueden
		// contradecirse.
		d.EsContabilidad = d.ContabilidadOrigen != ""
	}
	return d, err
}

// netoCRC = total_crc − anticipos aplicados (nunca negativo). Ambos en formato decimal string.
func netoCRC(totalCRC, aplicados string) string {
	t, err := decimal.NewFromString(totalCRC)
	if err != nil {
		return totalCRC
	}
	a, err := decimal.NewFromString(aplicados)
	if err != nil {
		a = decimal.Zero
	}
	neto := t.Sub(a)
	if neto.IsNegative() {
		neto = decimal.Zero
	}
	return neto.StringFixed(2)
}

func (r *pgRepository) CrearDocumento(ctx context.Context, empresaID string, in DocumentoInput, totalCRC decimal.Decimal, tc *decimal.Decimal, usuarioID string) (Documento, error) {
	// INSERT solo si el proveedor pertenece a la empresa (tenant-safe): 0 filas => proveedor ajeno.
	// Nace pre-clasificado con la memoria de gasto del proveedor (clasif_auto = true) si existe.
	const q = `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, moneda, subtotal, iva, retencion, total, tc_aplicado, total_crc, descripcion, creado_por, fecha_vencimiento, tipo,
			 concepto_id, clasificacion_id, subclasificacion_id, clasif_auto, departamento_id)
		SELECT $1::uuid, p.id,
		       COALESCE(NULLIF($3, ''), 'INT-' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 10)),
		       NULLIF($4, ''), $5::date, $6, $7, $8, $9, $10, $11, $12, NULLIF($13, ''), $14::uuid,
		       COALESCE(NULLIF($15, '')::date,
		                CASE WHEN p.condicion_pago = 'CREDITO' THEN $5::date + p.plazo_credito_dias END),
		       COALESCE(NULLIF($16, ''), 'CXP'),
		       p.gasto_concepto_id, p.gasto_clasificacion_id, p.gasto_subclasificacion_id, (p.gasto_concepto_id IS NOT NULL),
		       -- Enrutamiento automático: hereda el departamento (centro de costo) del proveedor.
		       (SELECT dep.id FROM departamento dep WHERE dep.empresa_id = $1::uuid AND dep.nombre = p.departamento AND dep.activo)
		FROM proveedor p WHERE p.id = $2::uuid AND p.empresa_id = $1::uuid
		RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, in.ProveedorID, in.Clave, in.Consecutivo, in.FechaEmision,
		in.Moneda, in.Subtotal, in.IVA, in.Retencion, in.Total, tc, totalCRC, in.Descripcion, usuarioID, in.Vencimiento, in.Tipo).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Documento{}, ErrProveedorNoEncontrado
	}
	if esViolacionUnica(err) {
		return Documento{}, ErrDocumentoDuplicado
	}
	if err != nil {
		return Documento{}, fmt.Errorf("cxp: crear documento: %w", err)
	}
	return r.DocumentoPorID(ctx, empresaID, id)
}

func (r *pgRepository) DocumentoPorID(ctx context.Context, empresaID, id string) (Documento, error) {
	const q = `SELECT ` + documentoCols + ` ` + documentoFrom + `
		WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid`
	d, err := scanDocumento(r.pool.QueryRow(ctx, q, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Documento{}, ErrDocumentoNoEncontrado
	}
	if err != nil {
		return Documento{}, fmt.Errorf("cxp: documento por id: %w", err)
	}
	return d, nil
}

func (r *pgRepository) ListarDocumentos(ctx context.Context, empresaID string, f FiltrosDocumentos) (ListaDocumentos, error) {
	conds := []string{"d.empresa_id = $1::uuid"}
	args := []any{empresaID}
	if f.Estado != "" {
		args = append(args, f.Estado)
		conds = append(conds, fmt.Sprintf("d.estado = $%d", len(args)))
	}
	if f.ProveedorID != "" {
		args = append(args, f.ProveedorID)
		conds = append(conds, fmt.Sprintf("d.proveedor_id = $%d::uuid", len(args)))
	}
	if len(f.Estados) > 0 {
		args = append(args, f.Estados)
		conds = append(conds, fmt.Sprintf("d.estado = ANY($%d)", len(args)))
	}
	if f.Q != "" {
		args = append(args, "%"+f.Q+"%")
		conds = append(conds, fmt.Sprintf("(p.nombre ILIKE $%d OR d.consecutivo ILIKE $%d OR d.clave ILIKE $%d)", len(args), len(args), len(args)))
	}
	if f.ConceptoID != "" {
		args = append(args, f.ConceptoID)
		conds = append(conds, fmt.Sprintf("d.concepto_id = $%d::uuid", len(args)))
	}
	if f.ClasificacionID != "" {
		args = append(args, f.ClasificacionID)
		conds = append(conds, fmt.Sprintf("d.clasificacion_id = $%d::uuid", len(args)))
	}
	if f.MontoMin != "" {
		args = append(args, f.MontoMin)
		conds = append(conds, fmt.Sprintf("d.total_crc >= $%d::numeric", len(args)))
	}
	if f.MontoMax != "" {
		args = append(args, f.MontoMax)
		conds = append(conds, fmt.Sprintf("d.total_crc <= $%d::numeric", len(args)))
	}
	if f.LoteID != "" {
		args = append(args, f.LoteID)
		conds = append(conds, fmt.Sprintf("d.lote_id = $%d::uuid", len(args)))
	}
	switch f.LoteFiltro {
	case "sin":
		conds = append(conds, "d.lote_id IS NULL")
	case "con":
		conds = append(conds, "d.lote_id IS NOT NULL")
	}
	// Cartera viva: MISMA frontera que el tablero (una sola constante, así el conteo de un
	// tramo y su listado no pueden desincronizarse).
	if f.Abierta {
		conds = append(conds, "d.estado NOT IN "+estadosCerrados)
	}
	// Tramo de vencimiento: mismos cortes que el panel del dashboard (día de Costa Rica),
	// para que al hacer clic en «+90 días» salgan exactamente esas facturas.
	if cond := condVencimiento(f.Vencimiento); cond != "" {
		conds = append(conds, cond)
	}
	// Scoping por área del validador: nil = ve todo; no-nil (aun vacío) = solo esos departamentos.
	if f.DepartamentoIDs != nil {
		args = append(args, f.DepartamentoIDs)
		conds = append(conds, fmt.Sprintf("d.departamento_id = ANY($%d)", len(args)))
	}
	// Marca «de Contabilidad». Se compara contra la MISMA expresión que devuelve el SELECT, así
	// que el filtro y lo que muestra la fila no pueden discrepar.
	switch f.Contabilidad {
	case "si":
		conds = append(conds, "("+contabilidadOrigenSQL+") <> ''")
	case "no":
		conds = append(conds, "("+contabilidadOrigenSQL+") = ''")
	}
	// Cola del área: solo lo que un área tiene que confirmar. Lo que todavía no se evaluó
	// (requiere_validacion NULL) cuenta como pendiente de confirmar — es lo conservador: mejor que
	// alguien la mire a que se pague sin que nadie la haya decidido.
	switch f.RequiereValidacion {
	case "si":
		conds = append(conds, "COALESCE(d.requiere_validacion, true)")
	case "no":
		conds = append(conds, "d.requiere_validacion IS FALSE")
	}
	// Fase de la Bandeja: se resuelve con la MISMA expresión que cuenta el encabezado
	// (`faseBandejaSQL`), no con una lista de estados equivalente. Una fase ya no se puede
	// expresar como «estos estados»: «Por aprobar» junta lo que el área validó con lo que nunca
	// necesitó pasar por el área, y las dos cosas están en estados distintos.
	if fasesBandeja[f.Fase] {
		args = append(args, f.Fase)
		conds = append(conds, fmt.Sprintf("(%s) = $%d", faseBandejaSQL, len(args)))
	}
	where := strings.Join(conds, " AND ")

	// El conteo usa el mismo FROM que el listado: los filtros pueden referenciar proveedor (q).
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) "+documentoFrom+" WHERE "+where, args...).Scan(&total); err != nil {
		return ListaDocumentos{}, fmt.Errorf("cxp: contar documentos: %w", err)
	}
	if f.PageSize <= 0 || f.PageSize > 500 {
		f.PageSize = 100
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	orderBy := "d.fecha_emision DESC, d.id"
	if f.Orden == "vencimiento" {
		// Bandeja: prioridad interna primero (AA, luego A), después lo que vence primero.
		orderBy = "CASE d.prioridad WHEN 'AA' THEN 0 WHEN 'A' THEN 1 ELSE 2 END, d.fecha_vencimiento ASC NULLS LAST, d.id"
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	listQ := "SELECT " + documentoCols + " " + documentoFrom + " WHERE " + where +
		fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderBy, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return ListaDocumentos{}, fmt.Errorf("cxp: listar documentos: %w", err)
	}
	defer rows.Close()
	items := make([]Documento, 0, f.PageSize)
	for rows.Next() {
		d, err := scanDocumento(rows)
		if err != nil {
			return ListaDocumentos{}, fmt.Errorf("cxp: scan documento: %w", err)
		}
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return ListaDocumentos{}, fmt.Errorf("cxp: iterar documentos: %w", err)
	}
	return ListaDocumentos{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, nil
}

func (r *pgRepository) CambiarEstado(ctx context.Context, empresaID, id, de, a string) (int64, error) {
	const q = `UPDATE documento_cxp SET estado = $4, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = $3`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, de, a)
	if err != nil {
		return 0, fmt.Errorf("cxp: cambiar estado: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CambiarEstadoMulti cambia el estado si el actual está en `de` (acciones que salen de varios estados,
// p. ej. anular desde RECIBIDO/REVISADO/APROBADO/PROGRAMADO).
func (r *pgRepository) CambiarEstadoMulti(ctx context.Context, empresaID, id string, de []string, a string) (int64, error) {
	const q = `UPDATE documento_cxp SET estado = $4, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = ANY($3)`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, de, a)
	if err != nil {
		return 0, fmt.Errorf("cxp: cambiar estado multi: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AprenderCondicionPago completa las condiciones de pago de un proveedor que sigue en el
// valor por defecto (Contado/0) usando lo que dicen sus facturas. No pisa lo fijado a mano.
func (r *pgRepository) AprenderCondicionPago(ctx context.Context, empresaID, proveedorID, condicion string, plazoDias int) error {
	const q = `
		UPDATE proveedor
		SET condicion_pago = $3, plazo_credito_dias = $4
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		  AND condicion_pago = 'CONTADO' AND plazo_credito_dias = 0
		  AND $3 = 'CREDITO' AND $4 > 0`
	if _, err := r.pool.Exec(ctx, q, empresaID, proveedorID, condicion, plazoDias); err != nil {
		return fmt.Errorf("cxp: aprender condición de pago: %w", err)
	}
	return nil
}

// GuardarGastoDefault fija la memoria de gasto del proveedor (para auto-clasificar sus próximas
// facturas). Tenant-safe: solo si concepto/clasif/subclasif pertenecen a la empresa (los valida
// el UPDATE previo de Clasificar; aquí solo se copian los mismos ids).
func (r *pgRepository) GuardarGastoDefault(ctx context.Context, empresaID, proveedorID, conceptoID, clasificacionID, subclasificacionID string) error {
	const q = `
		UPDATE proveedor
		SET gasto_concepto_id = NULLIF($3, '')::uuid, gasto_clasificacion_id = NULLIF($4, '')::uuid,
		    gasto_subclasificacion_id = NULLIF($5, '')::uuid
		WHERE empresa_id = $1::uuid AND id = $2::uuid`
	if _, err := r.pool.Exec(ctx, q, empresaID, proveedorID, conceptoID, clasificacionID, subclasificacionID); err != nil {
		return fmt.Errorf("cxp: guardar gasto default: %w", err)
	}
	return nil
}

// AsignarPrioridad fija la prioridad interna de pago (AA / A / "" normal).
func (r *pgRepository) AsignarPrioridad(ctx context.Context, empresaID, id, prioridad string) (int64, error) {
	const q = `UPDATE documento_cxp SET prioridad = $3, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, prioridad)
	if err != nil {
		return 0, fmt.Errorf("cxp: asignar prioridad: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GuardarNotaRevision registra el motivo al denegar/anular/liquidar (la contrapartida del archivo).
func (r *pgRepository) GuardarNotaRevision(ctx context.Context, empresaID, id, nota string) error {
	const q = `UPDATE documento_cxp SET nota_revision = NULLIF($3, ''), actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	if _, err := r.pool.Exec(ctx, q, empresaID, id, nota); err != nil {
		return fmt.Errorf("cxp: guardar nota revisión: %w", err)
	}
	return nil
}

// RegistrarGastoProveedor acumula la categoría usada con el proveedor (gastos frecuentes).
// Solo conceptos visibles para CxP (el catálogo bancario sensible no entra a la memoria).
func (r *pgRepository) RegistrarGastoProveedor(ctx context.Context, empresaID, proveedorID, conceptoID, clasificacionID, subclasificacionID string) error {
	const q = `
		INSERT INTO proveedor_gasto (empresa_id, proveedor_id, concepto_id, clasificacion_id, subclasificacion_id)
		SELECT $1::uuid, $2::uuid, $3::uuid, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid
		WHERE EXISTS (SELECT 1 FROM proveedor WHERE id = $2::uuid AND empresa_id = $1::uuid)
		  AND EXISTS (SELECT 1 FROM concepto WHERE id = $3::uuid AND empresa_id = $1::uuid AND visible_cxp)
		ON CONFLICT (proveedor_id, concepto_id, clasificacion_id, subclasificacion_id)
			DO UPDATE SET usos = proveedor_gasto.usos + 1, ultimo_uso = now()`
	if _, err := r.pool.Exec(ctx, q, empresaID, proveedorID, conceptoID, clasificacionID, subclasificacionID); err != nil {
		return fmt.Errorf("cxp: registrar gasto proveedor: %w", err)
	}
	return nil
}

// GastosDeProveedor lista las categorías frecuentes del proveedor (más usadas primero).
func (r *pgRepository) GastosDeProveedor(ctx context.Context, empresaID, proveedorID string) ([]GastoFrecuente, error) {
	const q = `
		SELECT pg.concepto_id::text, c.nombre,
		       COALESCE(pg.clasificacion_id::text, ''), COALESCE(cl.nombre, ''),
		       COALESCE(pg.subclasificacion_id::text, ''), COALESCE(sc.nombre, ''), pg.usos
		FROM proveedor_gasto pg
		JOIN concepto c ON c.id = pg.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = pg.clasificacion_id
		LEFT JOIN subclasificacion sc ON sc.id = pg.subclasificacion_id
		WHERE pg.empresa_id = $1::uuid AND pg.proveedor_id = $2::uuid
		ORDER BY pg.usos DESC, pg.ultimo_uso DESC
		LIMIT 8`
	rows, err := r.pool.Query(ctx, q, empresaID, proveedorID)
	if err != nil {
		return nil, fmt.Errorf("cxp: gastos de proveedor: %w", err)
	}
	defer rows.Close()
	out := make([]GastoFrecuente, 0, 8)
	for rows.Next() {
		var g GastoFrecuente
		if err := rows.Scan(&g.ConceptoID, &g.Concepto, &g.ClasificacionID, &g.Clasificacion,
			&g.SubclasificacionID, &g.Subclasificacion, &g.Usos); err != nil {
			return nil, fmt.Errorf("cxp: scan gasto frecuente: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// AsignarTipo fija el tipo de factura (CXP/ANTICIPO/VIATICOS/REINTEGRO) de un documento.
// Solo antes de la validación de área (RECIBIDO/REVISADO): re-tipificar después sería una vía
// para evadir el control (reclasificar a VIATICOS y desviar por la ruta sin pago).
func (r *pgRepository) AsignarTipo(ctx context.Context, empresaID, id, tipo string) (int64, error) {
	const q = `UPDATE documento_cxp SET tipo = $3, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado IN ('RECIBIDO', 'REVISADO')`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, tipo)
	if err != nil {
		return 0, fmt.Errorf("cxp: asignar tipo: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *pgRepository) Programar(ctx context.Context, empresaID, id, fecha, huella string) (int64, error) {
	const q = `UPDATE documento_cxp
	           SET estado = 'PROGRAMADO', fecha_pago_programada = $3::date, huella = $4, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'APROBADO'`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, fecha, huella)
	if err != nil {
		return 0, fmt.Errorf("cxp: programar: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Clasificar asigna concepto/clasificación de gasto a un documento. Tenant-safe: solo aplica si
// el concepto y la clasificación (cuando se envían) pertenecen a la empresa. Devuelve filas afectadas.
func (r *pgRepository) Clasificar(ctx context.Context, empresaID, id, conceptoID, clasificacionID, subclasificacionID string) (int64, error) {
	const q = `
		UPDATE documento_cxp
		SET concepto_id = NULLIF($3, '')::uuid, clasificacion_id = NULLIF($4, '')::uuid,
		    subclasificacion_id = NULLIF($5, '')::uuid, clasif_auto = false, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		  AND ($3 = '' OR EXISTS (SELECT 1 FROM concepto WHERE id = $3::uuid AND empresa_id = $1::uuid AND visible_cxp))
		  AND ($4 = '' OR EXISTS (SELECT 1 FROM clasificacion WHERE id = $4::uuid AND empresa_id = $1::uuid))
		  AND ($5 = '' OR EXISTS (SELECT 1 FROM subclasificacion WHERE id = $5::uuid AND empresa_id = $1::uuid))`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, conceptoID, clasificacionID, subclasificacionID)
	if err != nil {
		return 0, fmt.Errorf("cxp: clasificar documento: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *pgRepository) RegistrarAprobacion(ctx context.Context, empresaID, docID, usuarioID, rol string) error {
	const q = `INSERT INTO documento_cxp_aprobacion (empresa_id, documento_id, usuario_id, rol)
	           VALUES ($1::uuid, $2::uuid, $3::uuid, $4)`
	_, err := r.pool.Exec(ctx, q, empresaID, docID, usuarioID, rol)
	if esViolacionUnica(err) {
		return ErrYaAprobado
	}
	if err != nil {
		return fmt.Errorf("cxp: registrar aprobación: %w", err)
	}
	return nil
}

func (r *pgRepository) RolesAprobaciones(ctx context.Context, empresaID, docID string) ([]string, error) {
	const q = `SELECT rol FROM documento_cxp_aprobacion WHERE empresa_id = $1::uuid AND documento_id = $2::uuid`
	rows, err := r.pool.Query(ctx, q, empresaID, docID)
	if err != nil {
		return nil, fmt.Errorf("cxp: roles aprobaciones: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var rol string
		if err := rows.Scan(&rol); err != nil {
			return nil, fmt.Errorf("cxp: scan rol aprobación: %w", err)
		}
		out = append(out, rol)
	}
	return out, rows.Err()
}
