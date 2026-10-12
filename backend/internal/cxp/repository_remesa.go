package cxp

// Acceso a datos de las remesas: buscarlas por su código, listar las de un lote y armar el aviso
// de pago (el detalle de qué cubrió una transferencia).

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// RemesaPorHuella busca una remesa por su código interno, dentro de la empresa.
//
// Devuelve también cuántas facturas VIVAS cubre: una remesa cuyas facturas se anularon todas ya no
// representa un pago que haya que cerrar, y conciliar contra ella no cambiaría nada.
func (r *pgRepository) RemesaPorHuella(ctx context.Context, empresaID, huella string) (Remesa, error) {
	const q = `
		SELECT r.id::text, r.lote_id::text, l.numero, r.proveedor_id::text, p.nombre, r.numero, r.huella,
		       r.monto_crc::text,
		       (SELECT COUNT(*) FROM documento_cxp d WHERE d.remesa_id = r.id)
		FROM remesa_pago r
		JOIN proveedor p ON p.id = r.proveedor_id
		JOIN lote_pago l ON l.id = r.lote_id
		WHERE r.empresa_id = $1::uuid AND r.huella = $2`
	var rm Remesa
	err := r.pool.QueryRow(ctx, q, empresaID, huella).Scan(&rm.ID, &rm.LoteID, &rm.LoteNumero, &rm.ProveedorID,
		&rm.Proveedor, &rm.Numero, &rm.Huella, &rm.MontoCRC, &rm.Facturas)
	if errors.Is(err, pgx.ErrNoRows) {
		return Remesa{}, ErrRemesaNoEncontrada
	}
	if err != nil {
		return Remesa{}, fmt.Errorf("cxp: remesa por huella: %w", err)
	}
	return rm, nil
}

// CerrarRemesa lleva a CONCILIADO todas las facturas de la remesa y devuelve cuántas movió.
//
// Una por una y respetando el flujo (PROGRAMADO → PAGADO → CONCILIADO), igual que el pago suelto:
// saltarse estados dejaría sin registrar que la plata salió. Las que ya estaban conciliadas o
// anuladas no se tocan y por eso el conteo puede ser menor que `Facturas`.
//
// Devuelve además el gasto COMÚN de las facturas cerradas, si todas comparten el mismo: es lo que
// permite clasificar el movimiento bancario sin inventar. Si el pago mezcla gastos distintos, vuelve
// vacío — y el movimiento queda sin clasificar, que es lo honesto: no hay UN gasto que lo describa.
func (r *pgRepository) CerrarRemesa(ctx context.Context, empresaID, remesaID string) (int, string, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, "", "", fmt.Errorf("cxp: begin cerrar remesa: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const avanzar = `
		UPDATE documento_cxp
		   SET estado = CASE WHEN estado = 'PROGRAMADO' THEN 'PAGADO' ELSE estado END,
		       actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND remesa_id = $2::uuid AND estado = 'PROGRAMADO'`
	if _, err := tx.Exec(ctx, avanzar, empresaID, remesaID); err != nil {
		return 0, "", "", fmt.Errorf("cxp: remesa a pagado: %w", err)
	}

	const conciliar = `
		UPDATE documento_cxp SET estado = 'CONCILIADO', actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND remesa_id = $2::uuid AND estado = 'PAGADO'`
	tag, err := tx.Exec(ctx, conciliar, empresaID, remesaID)
	if err != nil {
		return 0, "", "", fmt.Errorf("cxp: remesa a conciliado: %w", err)
	}

	// El gasto común, si lo hay. Dos condiciones: que TODAS tengan clasificación
	// (COUNT(*) = COUNT(columna), porque COUNT de una columna no cuenta los NULL) y que sea la
	// misma. Con una sola distinta, el pago ya no se puede atribuir a un gasto y vuelve vacío.
	//
	// `array_agg(DISTINCT …)[1]` y no MIN(): en PostgreSQL no existe MIN(uuid), y como esto corre
	// detrás de un fake en las pruebas de servicio, el error solo aparecía contra la base real.
	const gasto = `
		SELECT COALESCE(CASE WHEN comun THEN concepto END, ''), COALESCE(CASE WHEN comun THEN clasif END, '')
		FROM (
			SELECT COUNT(*) = COUNT(d.clasificacion_id) AND COUNT(DISTINCT d.clasificacion_id) = 1 AS comun,
			       (array_agg(DISTINCT d.concepto_id))[1]::text      AS concepto,
			       (array_agg(DISTINCT d.clasificacion_id))[1]::text AS clasif
			FROM documento_cxp d
			WHERE d.empresa_id = $1::uuid AND d.remesa_id = $2::uuid
		) x`
	var conceptoID, clasificacionID string
	if err := tx.QueryRow(ctx, gasto, empresaID, remesaID).Scan(&conceptoID, &clasificacionID); err != nil {
		return 0, "", "", fmt.Errorf("cxp: gasto común de la remesa: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, "", "", fmt.Errorf("cxp: commit cerrar remesa: %w", err)
	}
	return int(tag.RowsAffected()), conceptoID, clasificacionID, nil
}

// LotePorID trae un lote de la empresa. Lo necesita el reporte, que arranca por saber CÓMO se
// armó el corte: de eso depende qué es una «transferencia».
func (r *pgRepository) LotePorID(ctx context.Context, empresaID, loteID string) (LotePago, error) {
	const q = `
		SELECT id::text, numero, to_char(fecha_corte, 'YYYY-MM-DD'), estado,
		       to_char(creado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), modo_pago
		FROM lote_pago WHERE empresa_id = $1::uuid AND id = $2::uuid`
	var l LotePago
	err := r.pool.QueryRow(ctx, q, empresaID, loteID).
		Scan(&l.ID, &l.Numero, &l.FechaCorte, &l.Estado, &l.CreadoEn, &l.ModoPago)
	if errors.Is(err, pgx.ErrNoRows) {
		return LotePago{}, ErrLoteNoEncontrado
	}
	if err != nil {
		return LotePago{}, fmt.Errorf("cxp: lote por id: %w", err)
	}
	return l, nil
}

// RemesasDeLote lista los pagos agrupados de un lote, para la pantalla y el reporte.
func (r *pgRepository) RemesasDeLote(ctx context.Context, empresaID, loteID string) ([]Remesa, error) {
	const q = `
		SELECT r.id::text, r.lote_id::text, l.numero, r.proveedor_id::text, p.nombre, r.numero, r.huella,
		       r.monto_crc::text,
		       (SELECT COUNT(*) FROM documento_cxp d WHERE d.remesa_id = r.id)
		FROM remesa_pago r
		JOIN proveedor p ON p.id = r.proveedor_id
		JOIN lote_pago l ON l.id = r.lote_id
		WHERE r.empresa_id = $1::uuid AND r.lote_id = $2::uuid
		ORDER BY r.numero`
	rows, err := r.pool.Query(ctx, q, empresaID, loteID)
	if err != nil {
		return nil, fmt.Errorf("cxp: remesas del lote: %w", err)
	}
	defer rows.Close()
	out := make([]Remesa, 0)
	for rows.Next() {
		var rm Remesa
		if err := rows.Scan(&rm.ID, &rm.LoteID, &rm.LoteNumero, &rm.ProveedorID, &rm.Proveedor, &rm.Numero,
			&rm.Huella, &rm.MontoCRC, &rm.Facturas); err != nil {
			return nil, fmt.Errorf("cxp: scan remesa: %w", err)
		}
		out = append(out, rm)
	}
	return out, rows.Err()
}

// AvisoDeRemesa arma el detalle de UNA transferencia agrupada: qué facturas cubre y por cuánto.
func (r *pgRepository) AvisoDeRemesa(ctx context.Context, empresaID, remesaID string) (AvisoDePago, error) {
	const cab = `
		SELECT r.huella, r.numero, r.monto_crc::text, p.nombre, p.id::text, COALESCE(p.iban, ''),
		       l.numero, to_char(l.fecha_corte, 'YYYY-MM-DD')
		FROM remesa_pago r
		JOIN proveedor p ON p.id = r.proveedor_id
		JOIN lote_pago l ON l.id = r.lote_id
		WHERE r.empresa_id = $1::uuid AND r.id = $2::uuid`
	var av AvisoDePago
	var numero int
	err := r.pool.QueryRow(ctx, cab, empresaID, remesaID).Scan(&av.Huella, &numero, &av.TotalCRC,
		&av.Proveedor, &av.ProveedorID, &av.IBAN, &av.LoteNumero, &av.FechaCorte)
	if errors.Is(err, pgx.ErrNoRows) {
		return AvisoDePago{}, ErrRemesaNoEncontrada
	}
	if err != nil {
		return AvisoDePago{}, fmt.Errorf("cxp: aviso de remesa: %w", err)
	}
	av.Referencia = ReferenciaRemesa(av.LoteNumero, numero)

	lineas, err := r.lineasDeAviso(ctx, empresaID, "d.remesa_id = $2::uuid", remesaID)
	if err != nil {
		return AvisoDePago{}, err
	}
	av.Lineas = lineas
	return av, nil
}

// AvisoDeFactura arma el aviso de un pago suelto. Misma forma que el de la remesa —una sola línea—
// para que la pantalla, el PDF y el correo no tengan que saber cuál de los dos están mostrando.
func (r *pgRepository) AvisoDeFactura(ctx context.Context, empresaID, documentoID string) (AvisoDePago, error) {
	const cab = `
		SELECT COALESCE(d.huella, ''), COALESCE(d.consecutivo, ''), p.nombre, p.id::text,
		       COALESCE(p.iban, ''), COALESCE(l.numero, 0), COALESCE(to_char(l.fecha_corte, 'YYYY-MM-DD'), ''),
		       ` + netoAPagarSQL + `::text
		FROM documento_cxp d
		JOIN proveedor p ON p.id = d.proveedor_id
		LEFT JOIN lote_pago l ON l.id = d.lote_id
		WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid`
	var av AvisoDePago
	var consecutivo string
	err := r.pool.QueryRow(ctx, cab, empresaID, documentoID).Scan(&av.Huella, &consecutivo,
		&av.Proveedor, &av.ProveedorID, &av.IBAN, &av.LoteNumero, &av.FechaCorte, &av.TotalCRC)
	if errors.Is(err, pgx.ErrNoRows) {
		return AvisoDePago{}, ErrDocumentoNoEncontrado
	}
	if err != nil {
		return AvisoDePago{}, fmt.Errorf("cxp: aviso de factura: %w", err)
	}
	// La MISMA referencia que viaja al banco, armada por la misma función: si acá se escribiera a
	// mano, el aviso y el estado de cuenta podrían no coincidir y el proveedor no cruzaría nada.
	av.Referencia = macroDescripcion(consecutivo, "")

	lineas, err := r.lineasDeAviso(ctx, empresaID, "d.id = $2::uuid", documentoID)
	if err != nil {
		return AvisoDePago{}, err
	}
	av.Lineas = lineas
	return av, nil
}

// lineasDeAviso trae el detalle de las facturas que cubre un pago. `cond` sale SIEMPRE de un
// literal del propio paquete (nunca del cliente): acá se concatena SQL.
func (r *pgRepository) lineasDeAviso(ctx context.Context, empresaID, cond, id string) ([]LineaDeAviso, error) {
	q := `
		SELECT COALESCE(d.consecutivo, ''),
		       TRIM(BOTH ' · ' FROM COALESCE(c.nombre, '') || ' · ' || COALESCE(cl.nombre, '')),
		       to_char(d.fecha_emision, 'YYYY-MM-DD'),
		       COALESCE(to_char(d.fecha_vencimiento, 'YYYY-MM-DD'), ''),
		       d.total::text,
		       COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa
		                 WHERE aa.factura_id = d.id AND aa.activo), 0)::text,
		       ` + netoAPagarSQL + `::text
		FROM documento_cxp d
		LEFT JOIN concepto c ON c.id = d.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = d.clasificacion_id
		WHERE d.empresa_id = $1::uuid AND ` + cond + `
		ORDER BY d.fecha_vencimiento NULLS LAST, d.consecutivo`
	rows, err := r.pool.Query(ctx, q, empresaID, id)
	if err != nil {
		return nil, fmt.Errorf("cxp: líneas del aviso: %w", err)
	}
	defer rows.Close()
	out := make([]LineaDeAviso, 0)
	for rows.Next() {
		var l LineaDeAviso
		if err := rows.Scan(&l.Consecutivo, &l.Gasto, &l.FechaEmision, &l.FechaVencimiento,
			&l.TotalCRC, &l.AnticiposCRC, &l.NetoCRC); err != nil {
			return nil, fmt.Errorf("cxp: scan línea del aviso: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
