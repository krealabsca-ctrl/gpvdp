package bancos

// SQL del listado de cargas y de la reversa (mig 0085).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// sqlBloqueos son las preguntas de «¿qué se apoya en los movimientos de esta carga?» (y, desde la
// mig 0086, también «¿alguna de sus líneas ya volvió a entrar?», que es lo que frena el deshacer).
//
// Es UNA sola cadena usada por el listado y por la verificación previa a revertir, a propósito: si
// cada camino tuviera su propia copia, el botón de la pantalla y el guardarraíl del servidor
// terminarían contestando cosas distintas, que es el peor final posible para esta función.
//
// Se escribe como una lista de columnas para un `LEFT JOIN LATERAL (...) bl ON true` y asume que
// hay una tabla `i` (importacion) en alcance, con `i.id`, `i.empresa_id` y `i.cuenta_bancaria_id`.
//
// El mes se arma con lpad y no con to_char porque el resultado («2026-09») es el mismo texto que ya
// usa el resto del módulo para un período.
const sqlBloqueos = `
	SELECT
	  (SELECT COUNT(*) FROM cobro_cxc c
	     JOIN movimiento_bancario m ON m.id = c.movimiento_bancario_id
	    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id) AS cobros,
	  (SELECT COUNT(*) FROM cxc_planilla_movimiento pm
	     JOIN movimiento_bancario m ON m.id = pm.movimiento_bancario_id
	    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id) AS planillas,
	  -- Solo los avisos SIN resolver: uno ya contestado es historia, no una dependencia viva.
	  (SELECT COUNT(*) FROM movimiento_reporte_segmentacion rs
	     JOIN movimiento_bancario m ON m.id = rs.movimiento_id
	    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id
	      AND rs.resuelto_en IS NULL) AS avisos,
	  (SELECT COUNT(*) FROM responsabilidad_periodo rp
	     JOIN movimiento_bancario m ON m.id = rp.movimiento_id
	    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id) AS responsabilidades,
	  -- La huella Bancos-CxP: el movimiento ES el pago de esa factura y CxP la pasó a CONCILIADO
	  -- por eso (service_conciliacion_cxp.go, al confirmar la carga). Si se excluye, la factura
	  -- queda conciliada contra plata que ya no cuenta y el barrido no puede repararla: filtra por
	  -- m.incluido, así que nunca la vuelve a examinar.
	  (SELECT COUNT(*) FROM movimiento_bancario m
	    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id
	      AND m.documento_cxp_id IS NOT NULL) AS facturas_cxp,
	  -- Emparejado en cualquiera de los dos sentidos: el par apunta a este, o este apunta al par.
	  (SELECT COUNT(*) FROM movimiento_bancario m
	    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id
	      AND (m.par_traslado_id IS NOT NULL
	           OR EXISTS (SELECT 1 FROM movimiento_bancario o WHERE o.par_traslado_id = m.id))) AS traslados,
	  (SELECT COALESCE(array_agg(DISTINCT lpad(pc.anio::text, 4, '0') || '-' || lpad(pc.mes::text, 2, '0')), ARRAY[]::text[])
	     FROM periodo_cierre pc
	    WHERE pc.empresa_id = i.empresa_id
	      AND EXISTS (SELECT 1 FROM movimiento_bancario m
	                   WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id
	                     AND EXTRACT(YEAR FROM m.fecha) = pc.anio
	                     AND EXTRACT(MONTH FROM m.fecha) = pc.mes)) AS periodos,
	  -- El acta es POR CUENTA: solo bloquea la de la cuenta de esta carga.
	  (SELECT COALESCE(array_agg(DISTINCT lpad(a.anio::text, 4, '0') || '-' || lpad(a.mes::text, 2, '0')), ARRAY[]::text[])
	     FROM acta_conciliacion a
	    WHERE a.empresa_id = i.empresa_id
	      AND a.cuenta_bancaria_id = i.cuenta_bancaria_id
	      AND a.firmado_en IS NOT NULL
	      AND EXISTS (SELECT 1 FROM movimiento_bancario m
	                   WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id
	                     AND EXTRACT(YEAR FROM m.fecha) = a.anio
	                     AND EXTRACT(MONTH FROM m.fecha) = a.mes)) AS actas,
	  -- La otra mitad de «la reversa libera la línea» (mig 0086): cuántas de las filas que ESTA
	  -- reversa apagó ya volvieron a entrar por otra carga, y de qué archivos son.
	  --
	  -- Se compara por natural_key porque es justo la huella que la reversa liberó: la fila nueva
	  -- es otra fila (otro id, otra importación) con la MISMA identidad de movimiento. Y se mira
	  -- solo lo que cuenta plata (NOT n.excluido_por_reversa): si la carga nueva también se
	  -- revirtió, deshacer esta ya no duplica nada y no hay por qué frenarlo.
	  (SELECT COUNT(*) FROM movimiento_bancario m
	    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id
	      AND m.excluido_por_reversa
	      AND EXISTS (SELECT 1 FROM movimiento_bancario n
	                   WHERE n.empresa_id = m.empresa_id AND n.natural_key = m.natural_key
	                     AND n.id <> m.id AND NOT n.excluido_por_reversa)) AS reimportadas,
	  (SELECT COALESCE(array_agg(DISTINCT COALESCE(ni.nombre_archivo, '(sin archivo)')), ARRAY[]::text[])
	     FROM movimiento_bancario m
	     JOIN movimiento_bancario n
	       ON n.empresa_id = m.empresa_id AND n.natural_key = m.natural_key
	      AND n.id <> m.id AND NOT n.excluido_por_reversa
	     LEFT JOIN importacion ni ON ni.id = n.importacion_id
	    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id
	      AND m.excluido_por_reversa) AS cargas_reimport`

func (r *pgRepository) ListarImportaciones(ctx context.Context, empresaID string, f FiltrosImportaciones) (ListaImportaciones, error) {
	res := ListaImportaciones{Items: []ImportacionItem{}, Page: f.Page, PageSize: f.PageSize}

	args := []any{empresaID}
	where := "i.empresa_id = $1::uuid"
	if f.CuentaBancariaID != "" {
		args = append(args, f.CuentaBancariaID)
		where += fmt.Sprintf(" AND i.cuenta_bancaria_id = $%d::uuid", len(args))
	}

	// El total va aparte y sin los LATERAL: es el número que pagina la pantalla, no la página.
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM importacion i WHERE `+where, args...).Scan(&res.Total); err != nil {
		return ListaImportaciones{}, fmt.Errorf("bancos: contar importaciones: %w", err)
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	q := `
		SELECT i.id::text, i.cuenta_bancaria_id::text,
		       COALESCE(b.nombre, ''), COALESCE(cb.alias, ''), cb.moneda,
		       i.nombre_archivo, i.estado,
		       COALESCE(i.creado_por::text, ''), COALESCE(uc.nombre, ''), i.creado_en,
		       i.revertida_en, COALESCE(i.revertida_por::text, ''), COALESCE(ur.nombre, ''), i.motivo_reversa,
		       mv.movs, mv.excluidos, mv.clasificados, mv.debitos, mv.creditos, mv.desde, mv.hasta,
		       bl.cobros, bl.planillas, bl.avisos, bl.responsabilidades, bl.facturas_cxp, bl.traslados,
		       bl.periodos, bl.actas, bl.reimportadas, bl.cargas_reimport
		FROM importacion i
		JOIN cuenta_bancaria cb ON cb.id = i.cuenta_bancaria_id
		JOIN banco b ON b.id = cb.banco_id
		LEFT JOIN usuario uc ON uc.id = i.creado_por
		LEFT JOIN usuario ur ON ur.id = i.revertida_por
		-- LEFT JOIN LATERAL y no subconsultas sueltas: las seis medidas de los movimientos salen de
		-- UNA pasada por la carga. Y una carga de 0 movimientos igual devuelve su fila (con ceros),
		-- que es justo la que confunde al usuario («¿la subí o no?»).
		LEFT JOIN LATERAL (
		    SELECT COUNT(*) AS movs,
		           COUNT(*) FILTER (WHERE NOT m.incluido) AS excluidos,
		           COUNT(*) FILTER (WHERE m.estado_clasificacion <> 'NO_IDENTIFICADO') AS clasificados,
		           COALESCE(SUM(m.debito), 0)  AS debitos,
		           COALESCE(SUM(m.credito), 0) AS creditos,
		           MIN(m.fecha) AS desde, MAX(m.fecha) AS hasta
		    FROM movimiento_bancario m
		    WHERE m.empresa_id = i.empresa_id AND m.importacion_id = i.id
		) mv ON true
		LEFT JOIN LATERAL (` + sqlBloqueos + `) bl ON true
		WHERE ` + where + fmt.Sprintf(`
		ORDER BY i.creado_en DESC, i.id DESC
		LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return ListaImportaciones{}, fmt.Errorf("bancos: listar importaciones: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it ImportacionItem
		var creadoEn time.Time
		var revertidaEn *time.Time
		var desde, hasta *time.Time
		var debitos, creditos decimal.Decimal
		if err := rows.Scan(
			&it.ID, &it.CuentaBancariaID,
			&it.Banco, &it.CuentaAlias, &it.Moneda,
			&it.NombreArchivo, &it.Estado,
			&it.CreadoPor, &it.CreadoPorNombre, &creadoEn,
			&revertidaEn, &it.RevertidaPor, &it.RevertidaPorNombre, &it.MotivoReversa,
			&it.Movimientos, &it.Excluidos, &it.Clasificados, &debitos, &creditos, &desde, &hasta,
			&it.Bloqueos.CobrosCxC, &it.Bloqueos.PlanillasCxC, &it.Bloqueos.AvisosSinResolver,
			&it.Bloqueos.Responsabilidades, &it.Bloqueos.FacturasCxP, &it.Bloqueos.TrasladosEmparejados,
			&it.Bloqueos.PeriodosCerrados, &it.Bloqueos.ActasFirmadas,
			&it.Bloqueos.LineasReimportadas, &it.Bloqueos.CargasQueLasReimportaron,
		); err != nil {
			return ListaImportaciones{}, fmt.Errorf("bancos: scan importación: %w", err)
		}
		it.CreadoEn = creadoEn.Format(time.RFC3339)
		if revertidaEn != nil {
			it.RevertidaEn = revertidaEn.Format(time.RFC3339)
		}
		it.FechaDesde, it.FechaHasta = formatoFecha(desde), formatoFecha(hasta)
		it.TotalDebitos, it.TotalCreditos = debitos.String(), creditos.String()
		if it.Bloqueos.PeriodosCerrados == nil {
			it.Bloqueos.PeriodosCerrados = []string{}
		}
		if it.Bloqueos.ActasFirmadas == nil {
			it.Bloqueos.ActasFirmadas = []string{}
		}
		if it.Bloqueos.CargasQueLasReimportaron == nil {
			it.Bloqueos.CargasQueLasReimportaron = []string{}
		}
		res.Items = append(res.Items, it)
	}
	if err := rows.Err(); err != nil {
		return ListaImportaciones{}, fmt.Errorf("bancos: iterar importaciones: %w", err)
	}
	return res, nil
}

// formatoFecha devuelve «YYYY-MM-DD», o «» cuando la carga no trajo movimientos.
func formatoFecha(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func (r *pgRepository) EstadoDeReversa(ctx context.Context, empresaID, importacionID string) (EstadoDeReversa, error) {
	q := `
		SELECT i.estado, cb.moneda,
		       bl.cobros, bl.planillas, bl.avisos, bl.responsabilidades, bl.facturas_cxp, bl.traslados,
		       bl.periodos, bl.actas, bl.reimportadas, bl.cargas_reimport
		FROM importacion i
		JOIN cuenta_bancaria cb ON cb.id = i.cuenta_bancaria_id
		LEFT JOIN LATERAL (` + sqlBloqueos + `) bl ON true
		WHERE i.empresa_id = $1::uuid AND i.id = $2::uuid`

	var e EstadoDeReversa
	err := r.pool.QueryRow(ctx, q, empresaID, importacionID).Scan(
		&e.Estado, &e.Moneda,
		&e.Bloqueos.CobrosCxC, &e.Bloqueos.PlanillasCxC, &e.Bloqueos.AvisosSinResolver,
		&e.Bloqueos.Responsabilidades, &e.Bloqueos.FacturasCxP, &e.Bloqueos.TrasladosEmparejados,
		&e.Bloqueos.PeriodosCerrados, &e.Bloqueos.ActasFirmadas,
		&e.Bloqueos.LineasReimportadas, &e.Bloqueos.CargasQueLasReimportaron)
	// El filtro por empresa está en el WHERE: una carga de OTRA empresa no existe para esta, y eso
	// es un 404, nunca un 403 (un 403 confirmaría que existe).
	if errors.Is(err, pgx.ErrNoRows) {
		return EstadoDeReversa{}, ErrImportacionNoEncontrada
	}
	if err != nil {
		return EstadoDeReversa{}, fmt.Errorf("bancos: estado de reversa: %w", err)
	}
	return e, nil
}

func (r *pgRepository) RevertirImportacion(ctx context.Context, empresaID, importacionID, usuarioID, motivo string) (CambioDeReversa, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CambioDeReversa{}, fmt.Errorf("bancos: begin tx reversa: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// La carga PRIMERO: su fila es el candado. Dos reversas simultáneas se serializan acá y la
	// segunda ve `estado = 'REVERTIDA'`, no afecta filas y se va con 409 en vez de contar la plata
	// dos veces.
	//
	// `estado_previo = estado` lee el valor VIEJO (así funciona UPDATE en SQL): es de dónde venía la
	// carga, para que deshacer la devuelva exacta.
	const qImp = `
		UPDATE importacion
		   SET estado = 'REVERTIDA', estado_previo = estado,
		       revertida_en = now(), revertida_por = NULLIF($3, '')::uuid, motivo_reversa = $4
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado <> 'REVERTIDA'
		 RETURNING estado, revertida_en`
	var estado string
	var revertidaEn time.Time
	err = tx.QueryRow(ctx, qImp, empresaID, importacionID, usuarioID, motivo).Scan(&estado, &revertidaEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return CambioDeReversa{}, ErrImportacionYaRevertida
	}
	if err != nil {
		return CambioDeReversa{}, fmt.Errorf("bancos: marcar importación revertida: %w", err)
	}

	// NUNCA un DELETE. `AND m.incluido` deja fuera lo que ya estaba excluido por otra razón, y la
	// marca `excluido_por_reversa` es lo que permite que deshacer devuelva EXACTAMENTE estas filas
	// y ni una más.
	const qMovs = `
		WITH tocados AS (
		    UPDATE movimiento_bancario m
		       SET incluido = false, excluido_por_reversa = true, actualizado_en = now()
		     WHERE m.empresa_id = $1::uuid AND m.importacion_id = $2::uuid AND m.incluido
		     RETURNING m.debito, m.credito
		)
		SELECT COUNT(*), COALESCE(SUM(debito), 0), COALESCE(SUM(credito), 0) FROM tocados`
	var cambio CambioDeReversa
	if err := tx.QueryRow(ctx, qMovs, empresaID, importacionID).
		Scan(&cambio.Movimientos, &cambio.Debitos, &cambio.Creditos); err != nil {
		return CambioDeReversa{}, fmt.Errorf("bancos: excluir movimientos de la carga: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CambioDeReversa{}, fmt.Errorf("bancos: commit reversa: %w", err)
	}
	cambio.Estado = estado
	cambio.RevertidaEn = revertidaEn.Format(time.RFC3339)
	return cambio, nil
}

func (r *pgRepository) DeshacerReversaImportacion(ctx context.Context, empresaID, importacionID string) (CambioDeReversa, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CambioDeReversa{}, fmt.Errorf("bancos: begin tx deshacer reversa: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Vuelve al estado del que se revirtió. 'CONFIRMADA' es el respaldo para las reversas hechas
	// antes de que existiera `estado_previo` (y para cualquier fila que lo tenga vacío).
	const qImp = `
		UPDATE importacion
		   SET estado = COALESCE(NULLIF(estado_previo, ''), 'CONFIRMADA'), estado_previo = NULL,
		       revertida_en = NULL, revertida_por = NULL, motivo_reversa = ''
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'REVERTIDA'
		 RETURNING estado`
	var estado string
	err = tx.QueryRow(ctx, qImp, empresaID, importacionID).Scan(&estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return CambioDeReversa{}, ErrImportacionNoRevertida
	}
	if err != nil {
		return CambioDeReversa{}, fmt.Errorf("bancos: desmarcar importación revertida: %w", err)
	}

	// SOLO las que marcó la reversa: una fila excluida por otra corrección tiene que seguir
	// excluida, o deshacer un dedazo volvería a meter plata duplicada a los libros.
	const qMovs = `
		WITH tocados AS (
		    UPDATE movimiento_bancario m
		       SET incluido = true, excluido_por_reversa = false, actualizado_en = now()
		     WHERE m.empresa_id = $1::uuid AND m.importacion_id = $2::uuid AND m.excluido_por_reversa
		     RETURNING m.debito, m.credito
		)
		SELECT COUNT(*), COALESCE(SUM(debito), 0), COALESCE(SUM(credito), 0) FROM tocados`
	var cambio CambioDeReversa
	if err := tx.QueryRow(ctx, qMovs, empresaID, importacionID).
		Scan(&cambio.Movimientos, &cambio.Debitos, &cambio.Creditos); err != nil {
		return CambioDeReversa{}, fmt.Errorf("bancos: re-incluir movimientos de la carga: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CambioDeReversa{}, fmt.Errorf("bancos: commit deshacer reversa: %w", err)
	}
	cambio.Estado = estado
	return cambio, nil
}
