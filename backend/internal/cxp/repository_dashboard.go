package cxp

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// HistorialDocumento devuelve la línea de tiempo de un documento (eventos de auditoría,
// más antiguo primero) con el nombre del usuario que ejecutó cada acción.
func (r *pgRepository) HistorialDocumento(ctx context.Context, empresaID, docID string) ([]EventoHistorial, error) {
	const q = `
		SELECT a.accion,
		       COALESCE(NULLIF(u.nombre, ''), u.email, 'sistema'),
		       to_char(a.ts, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       COALESCE(a.valor_nuevo->>'nota', '')
		FROM auditoria_evento a
		LEFT JOIN usuario u ON u.id = a.usuario_id
		WHERE a.empresa_id = $1::uuid AND a.entidad = 'documento_cxp' AND a.entidad_id = $2::uuid
		ORDER BY a.ts`
	rows, err := r.pool.Query(ctx, q, empresaID, docID)
	if err != nil {
		return nil, fmt.Errorf("cxp: historial documento: %w", err)
	}
	defer rows.Close()
	out := make([]EventoHistorial, 0)
	for rows.Next() {
		var e EventoHistorial
		if err := rows.Scan(&e.Accion, &e.Usuario, &e.Fecha, &e.Nota); err != nil {
			return nil, fmt.Errorf("cxp: scan historial: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ResumenBandeja agrega conteo y monto por fase de la Bandeja CxP:
// rec=Recibidas · val=Por validar (área) · apr=Por aprobar (ya validada) · pag=Por pagar
// (aprobado o programado sin lote) · bco=En banco (en lote o rebotada) · pgd=Pagadas · arc=Archivo.
func (r *pgRepository) ResumenBandeja(ctx context.Context, empresaID string, deptIDs []string) ([]FaseBandeja, error) {
	// deptIDs nil = todas las áreas; no-nil (aun vacío) = solo esos departamentos.
	deptCond := ""
	args := []any{empresaID}
	if deptIDs != nil {
		args = append(args, deptIDs)
		deptCond = " AND d.departamento_id = ANY($2)"
	}
	// La fase sale de `faseBandejaSQL` — la MISMA expresión con la que el listado resuelve el
	// filtro `fase`. Por eso el número de una pestaña y las filas que se abren al hacerle clic
	// traen exactamente los mismos documentos.
	q := `
		SELECT fase, COUNT(*)::int, COALESCE(SUM(total_crc), 0)::text FROM (
			SELECT d.total_crc, ` + faseBandejaSQL + ` AS fase
			FROM documento_cxp d
			JOIN proveedor p ON p.id = d.proveedor_id
			LEFT JOIN concepto c ON c.id = d.concepto_id
			LEFT JOIN clasificacion cl ON cl.id = d.clasificacion_id
			WHERE d.empresa_id = $1::uuid` + deptCond + `) t
		GROUP BY fase`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cxp: resumen bandeja: %w", err)
	}
	defer rows.Close()
	out := make([]FaseBandeja, 0, 6)
	for rows.Next() {
		var f FaseBandeja
		if err := rows.Scan(&f.Fase, &f.Cantidad, &f.Monto); err != nil {
			return nil, fmt.Errorf("cxp: scan fase bandeja: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ── Dashboard CxP ──────────────────────────────────────────────────────────────

// hoyCR es el día calendario de Costa Rica (no UTC): el día hábil real de la operación.
const hoyCR = `(now() AT TIME ZONE 'America/Costa_Rica')::date`

// estadosCerrados son los que ya salieron de la cartera. REBOTADA NO está: el banco la
// devolvió, sigue debiéndose y hay que reintentarla.
const estadosCerrados = `('PAGADO', 'CONCILIADO', 'DENEGADO', 'ANULADO', 'LIQUIDADA')`

// accionesDePago son los eventos de auditoría que fechan un pago efectivo. Es la ÚNICA
// fuente válida de «cuándo se pagó»: actualizado_en se reescribe con cualquier edición
// posterior (adjuntar comprobante, reclasificar) y arrastraría la factura a otro mes.
const accionesDePago = `('PAGAR_DOCUMENTO', 'CONCILIAR_AUTO', 'CONCILIAR_DOCUMENTO')`

// retencionCRCSQL es la retención expresada en colones: en un documento en dólares la
// retención está en dólares, así que se convierte con el TC aplicado. Se redondea a dos
// decimales para no arrastrar los 4 del tipo de cambio a los montos del tablero.
const retencionCRCSQL = `ROUND(d.retencion * COALESCE(d.tc_aplicado, 1), 2)`

// netoCRCSQL es lo que de verdad sale del banco por una factura, en colones: total menos
// retención (se entera a Hacienda) menos anticipos ya aplicados (si no, doble pago). Misma
// fórmula que el archivo de pago (ver DocumentosParaPagoPorLote).
const netoCRCSQL = `GREATEST(d.total_crc - ` + retencionCRCSQL + `
	- COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa
	            WHERE aa.factura_id = d.id AND aa.activo), 0), 0)`

// condVencimiento traduce un tramo del panel de vencimientos a una condición SQL sobre
// d.fecha_vencimiento. Es la MISMA escala que tramosCartera: si el tablero dice «1 934
// facturas con +90 días», el listado filtrado debe traer esas 1 934 y no otras. Devuelve ""
// para un valor desconocido o vacío (sin filtro). No interpola datos del usuario: el valor
// se compara contra una lista cerrada.
func condVencimiento(tramo string) string {
	switch tramo {
	case "vencido":
		return "d.fecha_vencimiento < " + hoyCR
	case TramoV90:
		return "d.fecha_vencimiento < " + hoyCR + " - 90"
	case TramoV61:
		return "d.fecha_vencimiento >= " + hoyCR + " - 90 AND d.fecha_vencimiento < " + hoyCR + " - 60"
	case TramoV31:
		return "d.fecha_vencimiento >= " + hoyCR + " - 60 AND d.fecha_vencimiento < " + hoyCR + " - 30"
	case TramoV1:
		return "d.fecha_vencimiento >= " + hoyCR + " - 30 AND d.fecha_vencimiento < " + hoyCR
	case TramoSemana:
		return "d.fecha_vencimiento BETWEEN " + hoyCR + " AND " + hoyCR + " + 7"
	case TramoMes:
		return "d.fecha_vencimiento > " + hoyCR + " + 7 AND d.fecha_vencimiento <= " + hoyCR + " + 30"
	case TramoFuturo:
		return "d.fecha_vencimiento > " + hoyCR + " + 30"
	case TramoSinFecha:
		return "d.fecha_vencimiento IS NULL"
	default:
		return ""
	}
}

// condDepto agrega el filtro por área cuando el usuario no ve toda la empresa (mismo
// alcance que la Bandeja). deptIDs nil = ve todo; no-nil (aun vacío) = solo esos.
func condDepto(deptIDs []string, args []any) (string, []any) {
	if deptIDs == nil {
		return "", args
	}
	args = append(args, deptIDs)
	return fmt.Sprintf(" AND d.departamento_id = ANY($%d)", len(args)), args
}

// cteAbiertos es la cartera viva de la empresa con su neto y su proveedor, base de todos
// los cortes de la cartera (una sola definición para que ningún panel discrepe).
func cteAbiertos(deptCond string) string {
	return `WITH abiertos AS (
		SELECT d.id, d.fecha_vencimiento, COALESCE(d.prioridad, '') AS prioridad, d.estado,
		       d.departamento_id, d.concepto_id, d.total_crc,
		       p.id AS proveedor_id, p.nombre AS proveedor,
		       ` + retencionCRCSQL + ` AS retencion_crc,
		       COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa
		                 WHERE aa.factura_id = d.id AND aa.activo), 0) AS anticipos_crc,
		       ` + netoCRCSQL + ` AS neto
		FROM documento_cxp d
		JOIN proveedor p ON p.id = d.proveedor_id
		WHERE d.empresa_id = $1::uuid AND d.estado NOT IN ` + estadosCerrados + deptCond + `)`
}

// DashboardCxP arma el tablero: cartera a hoy (stock) + movimiento del período (flujo).
// periodo viene como YYYY-MM; deptIDs recorta al área del usuario (nil = toda la empresa).
func (r *pgRepository) DashboardCxP(ctx context.Context, empresaID, periodo string, deptIDs []string) (DashboardCxP, error) {
	d := DashboardCxP{Periodo: periodo}
	d.PorEstado = []ConteoEstado{} // nunca nil → JSON [] para empresas sin documentos
	d.Cartera.Tramos = []TramoVencimiento{}
	d.Cartera.TopProveedores = []ProveedorCartera{}
	d.Movimiento.Serie = []PuntoMesCxP{}
	d.AlcanceLimitado = deptIDs != nil

	if err := r.carteraCxP(ctx, empresaID, deptIDs, &d); err != nil {
		return DashboardCxP{}, err
	}
	if err := r.tramosCartera(ctx, empresaID, deptIDs, &d.Cartera); err != nil {
		return DashboardCxP{}, err
	}
	if err := r.topProveedoresCartera(ctx, empresaID, deptIDs, &d.Cartera); err != nil {
		return DashboardCxP{}, err
	}
	if err := r.movimientoCxP(ctx, empresaID, periodo, deptIDs, &d.Movimiento); err != nil {
		return DashboardCxP{}, err
	}
	if err := r.estadosCxP(ctx, empresaID, deptIDs, &d); err != nil {
		return DashboardCxP{}, err
	}
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM proveedor WHERE empresa_id = $1::uuid AND activo = true`,
		empresaID).Scan(&d.ProveedoresActivos); err != nil {
		return DashboardCxP{}, fmt.Errorf("cxp: proveedores activos: %w", err)
	}
	return d, nil
}

// carteraCxP saca en UNA foto todos los cortes de la cartera abierta (evita que dos
// consultas separadas se contradigan si alguien mueve un documento en medio).
func (r *pgRepository) carteraCxP(ctx context.Context, empresaID string, deptIDs []string, d *DashboardCxP) error {
	args := []any{empresaID}
	deptCond, args := condDepto(deptIDs, args)
	q := cteAbiertos(deptCond) + `
		SELECT to_char(` + hoyCR + `, 'YYYY-MM-DD'),
			COUNT(*), COALESCE(SUM(neto), 0)::text, COALESCE(SUM(total_crc), 0)::text,
			COALESCE(SUM(retencion_crc), 0)::text, COALESCE(SUM(anticipos_crc), 0)::text,
			COUNT(*) FILTER (WHERE fecha_vencimiento < ` + hoyCR + `),
			COALESCE(SUM(neto) FILTER (WHERE fecha_vencimiento < ` + hoyCR + `), 0)::text,
			COUNT(*) FILTER (WHERE fecha_vencimiento BETWEEN ` + hoyCR + ` AND ` + hoyCR + ` + 7),
			COALESCE(SUM(neto) FILTER (WHERE fecha_vencimiento BETWEEN ` + hoyCR + ` AND ` + hoyCR + ` + 7), 0)::text,
			COUNT(*) FILTER (WHERE estado = 'REBOTADA'),
			COALESCE(SUM(neto) FILTER (WHERE estado = 'REBOTADA'), 0)::text,
			COUNT(*) FILTER (WHERE prioridad = 'AA'),
			COALESCE(SUM(neto) FILTER (WHERE prioridad = 'AA'), 0)::text,
			COUNT(*) FILTER (WHERE prioridad = 'AA' AND fecha_vencimiento < ` + hoyCR + `),
			GREATEST(COALESCE(MAX(` + hoyCR + ` - fecha_vencimiento), 0), 0),
			COUNT(*) FILTER (WHERE departamento_id IS NULL),
			COALESCE(SUM(neto) FILTER (WHERE departamento_id IS NULL), 0)::text,
			COUNT(*) FILTER (WHERE concepto_id IS NULL),
			COALESCE(SUM(neto) FILTER (WHERE concepto_id IS NULL), 0)::text
		FROM abiertos`
	c := &d.Cartera
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&d.Hoy,
		&c.Abierta.Cantidad, &c.Abierta.Monto, &c.Bruto, &c.Retencion, &c.Anticipos,
		&c.Vencido.Cantidad, &c.Vencido.Monto,
		&c.VenceSemana.Cantidad, &c.VenceSemana.Monto,
		&c.Rebotadas.Cantidad, &c.Rebotadas.Monto,
		&c.PrioridadAA.Cantidad, &c.PrioridadAA.Monto, &c.AAVencidas, &c.DiasMasAntigua,
		&c.SinDepartamento.Cantidad, &c.SinDepartamento.Monto,
		&c.SinClasificar.Cantidad, &c.SinClasificar.Monto); err != nil {
		return fmt.Errorf("cxp: cartera del dashboard: %w", err)
	}
	return nil
}

// ordenTramos fija la presentación: del vencimiento más viejo al más lejano.
var ordenTramos = []string{TramoV90, TramoV61, TramoV31, TramoV1, TramoSemana, TramoMes, TramoFuturo, TramoSinFecha}

// tramosVencidos marca qué tramos ya están vencidos (para el color de la UI).
var tramosVencidos = map[string]bool{TramoV90: true, TramoV61: true, TramoV31: true, TramoV1: true}

// tramosCartera agrupa la cartera abierta por antigüedad de vencimiento (monto neto).
func (r *pgRepository) tramosCartera(ctx context.Context, empresaID string, deptIDs []string, c *CarteraCxP) error {
	args := []any{empresaID}
	deptCond, args := condDepto(deptIDs, args)
	q := cteAbiertos(deptCond) + `
		SELECT CASE
			WHEN fecha_vencimiento IS NULL THEN '` + TramoSinFecha + `'
			WHEN fecha_vencimiento < ` + hoyCR + ` - 90 THEN '` + TramoV90 + `'
			WHEN fecha_vencimiento < ` + hoyCR + ` - 60 THEN '` + TramoV61 + `'
			WHEN fecha_vencimiento < ` + hoyCR + ` - 30 THEN '` + TramoV31 + `'
			WHEN fecha_vencimiento < ` + hoyCR + ` THEN '` + TramoV1 + `'
			WHEN fecha_vencimiento <= ` + hoyCR + ` + 7 THEN '` + TramoSemana + `'
			WHEN fecha_vencimiento <= ` + hoyCR + ` + 30 THEN '` + TramoMes + `'
			ELSE '` + TramoFuturo + `' END AS tramo,
			COUNT(*), COALESCE(SUM(neto), 0)::text
		FROM abiertos GROUP BY 1`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("cxp: tramos de vencimiento: %w", err)
	}
	defer rows.Close()
	porClave := map[string]TramoVencimiento{}
	for rows.Next() {
		var t TramoVencimiento
		if err := rows.Scan(&t.Clave, &t.Cantidad, &t.Monto); err != nil {
			return fmt.Errorf("cxp: scan tramo: %w", err)
		}
		t.Vencido = tramosVencidos[t.Clave]
		porClave[t.Clave] = t
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cxp: iterar tramos: %w", err)
	}
	// Orden fijo y tramos vacíos incluidos: el escalón que quedó en cero se ve igual.
	for _, clave := range ordenTramos {
		t, ok := porClave[clave]
		if !ok {
			t = TramoVencimiento{Clave: clave, Vencido: tramosVencidos[clave], Monto: "0.00"}
		}
		c.Tramos = append(c.Tramos, t)
	}
	return nil
}

// topProveedoresCartera devuelve los 5 proveedores con más saldo abierto (concentración).
func (r *pgRepository) topProveedoresCartera(ctx context.Context, empresaID string, deptIDs []string, c *CarteraCxP) error {
	args := []any{empresaID}
	deptCond, args := condDepto(deptIDs, args)
	// Agrupa por ID (no por nombre): dos proveedores homónimos son dos acreedores distintos.
	q := cteAbiertos(deptCond) + `
		SELECT proveedor, COUNT(*), COALESCE(SUM(neto), 0)::text,
			COUNT(*) FILTER (WHERE fecha_vencimiento < ` + hoyCR + `)
		FROM abiertos GROUP BY proveedor_id, proveedor ORDER BY SUM(neto) DESC, proveedor LIMIT 5`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("cxp: concentración por proveedor: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p ProveedorCartera
		if err := rows.Scan(&p.Nombre, &p.Cantidad, &p.Monto, &p.Vencidos); err != nil {
			return fmt.Errorf("cxp: scan proveedor de cartera: %w", err)
		}
		c.TopProveedores = append(c.TopProveedores, p)
	}
	return rows.Err()
}

// movimientoCxP calcula lo recibido y lo pagado EN EL PERÍODO, más la serie mensual.
func (r *pgRepository) movimientoCxP(ctx context.Context, empresaID, periodo string, deptIDs []string, m *MovimientoCxP) error {
	// Recibidas: por fecha de EMISIÓN de la factura (no por la fecha de carga al sistema,
	// que se mueve con las importaciones masivas y no dice nada del período).
	args := []any{empresaID, periodo}
	deptCond, args := condDepto(deptIDs, args)
	q := `
		SELECT COUNT(*), COALESCE(SUM(d.total_crc), 0)::text
		FROM documento_cxp d
		WHERE d.empresa_id = $1::uuid AND to_char(d.fecha_emision, 'YYYY-MM') = $2` + deptCond
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&m.Recibidas.Cantidad, &m.Recibidas.Monto); err != nil {
		return fmt.Errorf("cxp: recibidas del período: %w", err)
	}

	// Pagadas: por la fecha REAL del pago (evento de auditoría) y ciclo emisión → pago.
	args = []any{empresaID, periodo}
	deptCond, args = condDepto(deptIDs, args)
	q = `
		SELECT COUNT(*), COALESCE(SUM(d.total_crc), 0)::text,
			COALESCE(ROUND(AVG((ev.ts AT TIME ZONE 'America/Costa_Rica')::date - d.fecha_emision)::numeric, 1), 0)::text
		FROM documento_cxp d
		JOIN LATERAL (
			SELECT MIN(a.ts) AS ts FROM auditoria_evento a
			WHERE a.empresa_id = d.empresa_id
			  AND a.entidad = 'documento_cxp' AND a.entidad_id = d.id
			  AND a.accion IN ` + accionesDePago + `
		) ev ON ev.ts IS NOT NULL
		WHERE d.empresa_id = $1::uuid AND d.estado IN ('PAGADO', 'CONCILIADO')
		  AND to_char(ev.ts AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM') = $2` + deptCond
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&m.Pagadas.Cantidad, &m.Pagadas.Monto, &m.CicloDias); err != nil {
		return fmt.Errorf("cxp: pagadas del período: %w", err)
	}

	// Pagados sin evento de pago: no se pueden fechar (vienen de una carga histórica). Se
	// reporta el número para que nadie lea las «pagadas del mes» como si fueran todas.
	args = []any{empresaID}
	deptCond, args = condDepto(deptIDs, args)
	q = `
		SELECT COUNT(*) FROM documento_cxp d
		WHERE d.empresa_id = $1::uuid AND d.estado IN ('PAGADO', 'CONCILIADO')
		  AND NOT EXISTS (
			SELECT 1 FROM auditoria_evento a
			WHERE a.empresa_id = d.empresa_id
			  AND a.entidad = 'documento_cxp' AND a.entidad_id = d.id
			  AND a.accion IN ` + accionesDePago + `)` + deptCond
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&m.PagadasSinEvento); err != nil {
		return fmt.Errorf("cxp: pagadas sin evento: %w", err)
	}

	return r.serieRecibidas(ctx, empresaID, periodo, deptIDs, m)
}

// mesesSerieCxP son los meses de la serie de recibidas (la maqueta aprobada muestra 7).
const mesesSerieCxP = 7

// serieRecibidas devuelve las facturas recibidas por mes de emisión, en los últimos
// mesesSerieCxP períodos hasta el elegido (los meses sin facturas quedan en cero).
func (r *pgRepository) serieRecibidas(ctx context.Context, empresaID, periodo string, deptIDs []string, m *MovimientoCxP) error {
	meses, err := ultimosPeriodos(periodo, mesesSerieCxP)
	if err != nil {
		return err
	}
	desde := meses[0] + "-01"
	hasta := periodo + "-01"
	args := []any{empresaID, desde, hasta}
	deptCond, args := condDepto(deptIDs, args)
	q := `
		SELECT to_char(d.fecha_emision, 'YYYY-MM'), COUNT(*), COALESCE(SUM(d.total_crc), 0)::text
		FROM documento_cxp d
		WHERE d.empresa_id = $1::uuid
		  AND d.fecha_emision >= $2::date
		  AND d.fecha_emision < ($3::date + INTERVAL '1 month')` + deptCond + `
		GROUP BY 1`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("cxp: serie de recibidas: %w", err)
	}
	defer rows.Close()
	porMes := map[string]PuntoMesCxP{}
	for rows.Next() {
		var p PuntoMesCxP
		if err := rows.Scan(&p.Periodo, &p.Cantidad, &p.Monto); err != nil {
			return fmt.Errorf("cxp: scan punto de serie: %w", err)
		}
		porMes[p.Periodo] = p
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cxp: iterar serie: %w", err)
	}
	for _, mes := range meses {
		p, ok := porMes[mes]
		if !ok {
			p = PuntoMesCxP{Periodo: mes, Monto: "0.00"}
		}
		p.EnCurso = mes == periodo
		m.Serie = append(m.Serie, p)
	}
	return nil
}

// estadosCxP devuelve el universo completo de documentos por estado (incluidos los
// terminales: rebotadas, anuladas, denegadas y liquidadas nunca desaparecen del tablero).
func (r *pgRepository) estadosCxP(ctx context.Context, empresaID string, deptIDs []string, d *DashboardCxP) error {
	args := []any{empresaID}
	deptCond, args := condDepto(deptIDs, args)
	q := `
		SELECT d.estado, COUNT(*), COALESCE(SUM(d.total_crc), 0)::text
		FROM documento_cxp d WHERE d.empresa_id = $1::uuid` + deptCond + `
		GROUP BY d.estado`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("cxp: dashboard por estado: %w", err)
	}
	defer rows.Close()
	total := decimal.Zero
	for rows.Next() {
		var c ConteoEstado
		if err := rows.Scan(&c.Estado, &c.Cantidad, &c.Monto); err != nil {
			return fmt.Errorf("cxp: scan estado: %w", err)
		}
		d.PorEstado = append(d.PorEstado, c)
		d.TotalDocumentos += c.Cantidad
		total = total.Add(decOrZeroSilent(c.Monto))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cxp: iterar estados: %w", err)
	}
	d.TotalMonto = total.StringFixed(2)
	return nil
}
