package bancos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

func (r *pgRepository) ListarReglas(ctx context.Context, empresaID string) ([]Regla, error) {
	// Trae TODAS las reglas (activas y pausadas): la UI las gestiona y el motor
	// filtra con soloActivas() antes de clasificar.
	const q = `
		SELECT r.id::text, r.nombre, r.aplica_a, r.concepto_id::text, co.nombre,
		       r.clasificacion_id::text, cl.nombre, r.prioridad, r.activo, r.aciertos,
		       COALESCE(array_agg(p.texto) FILTER (WHERE p.texto IS NOT NULL), '{}')
		FROM regla_clasificacion r
		JOIN concepto co ON co.id = r.concepto_id
		JOIN clasificacion cl ON cl.id = r.clasificacion_id
		LEFT JOIN palabra_clave p ON p.regla_id = r.id
		WHERE r.empresa_id = $1::uuid
		GROUP BY r.id, co.nombre, cl.nombre
		ORDER BY r.prioridad DESC, r.id`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar reglas: %w", err)
	}
	defer rows.Close()
	var out []Regla
	for rows.Next() {
		var reg Regla
		if err := rows.Scan(&reg.ID, &reg.Nombre, &reg.AplicaA, &reg.ConceptoID, &reg.Concepto,
			&reg.ClasificacionID, &reg.Clasificacion, &reg.Prioridad, &reg.Activo, &reg.Aciertos, &reg.Palabras); err != nil {
			return nil, fmt.Errorf("bancos: scan regla: %w", err)
		}
		out = append(out, reg)
	}
	return out, rows.Err()
}

// condicionesMovimientos arma el WHERE (y sus argumentos) de la hoja de trabajo.
//
// Vive aparte a propósito: la lista y el RESUMEN de la selección tienen que mirar
// exactamente el mismo conjunto. Si cada uno armara su filtro, tarde o temprano el
// encabezado diría un número y la tabla mostraría otro — el defecto que ya corregimos
// en el tablero de CxP. Alias obligatorio de la tabla: `m`.
func condicionesMovimientos(empresaID string, f FiltrosMovimientos) (string, []any) {
	conds := []string{"m.empresa_id = $1::uuid"}
	args := []any{empresaID}
	addArg := func(val any) int { args = append(args, val); return len(args) }

	if f.Desde != "" {
		conds = append(conds, fmt.Sprintf("m.fecha >= $%d", addArg(f.Desde)))
	}
	if f.Hasta != "" {
		conds = append(conds, fmt.Sprintf("m.fecha <= $%d", addArg(f.Hasta)))
	}
	if f.Periodo != "" {
		conds = append(conds, fmt.Sprintf("to_char(m.fecha, 'YYYY-MM') = $%d", addArg(f.Periodo)))
	}
	// Varios períodos a la vez (un trimestre, un semestre, meses sueltos).
	if len(f.Periodos) > 0 {
		conds = append(conds, fmt.Sprintf("to_char(m.fecha, 'YYYY-MM') = ANY($%d::text[])", addArg(f.Periodos)))
	}
	if f.ConceptoID != "" {
		conds = append(conds, fmt.Sprintf("m.concepto_id = $%d::uuid", addArg(f.ConceptoID)))
	}
	// Varios conceptos a la vez.
	if len(f.ConceptoIDs) > 0 {
		conds = append(conds, fmt.Sprintf("m.concepto_id = ANY($%d::uuid[])", addArg(f.ConceptoIDs)))
	}
	if f.ClasificacionID != "" {
		conds = append(conds, fmt.Sprintf("m.clasificacion_id = $%d::uuid", addArg(f.ClasificacionID)))
	}
	// Varias clasificaciones a la vez (el nivel fino del reporte).
	if len(f.ClasificacionIDs) > 0 {
		conds = append(conds, fmt.Sprintf("m.clasificacion_id = ANY($%d::uuid[])", addArg(f.ClasificacionIDs)))
	}
	// La cuenta se resuelve directo sobre el movimiento; el banco, por subconsulta a sus cuentas,
	// para no obligar al JOIN de `cuenta_bancaria` en las consultas que no lo traen (los totales).
	if f.CuentaID != "" {
		conds = append(conds, fmt.Sprintf("m.cuenta_bancaria_id = $%d::uuid", addArg(f.CuentaID)))
	}
	if f.BancoID != "" {
		conds = append(conds, fmt.Sprintf(
			"m.cuenta_bancaria_id IN (SELECT id FROM cuenta_bancaria WHERE banco_id = $%d::uuid)",
			addArg(f.BancoID)))
	}
	switch f.Estado {
	case "":
		// sin filtro de estado
	case "CLASIFICADO":
		// pseudo-estado: todo lo que ya tiene clasificación (AUTO o REVISADO)
		conds = append(conds, "m.estado_clasificacion <> 'NO_IDENTIFICADO'")
	default:
		conds = append(conds, fmt.Sprintf("m.estado_clasificacion = $%d", addArg(f.Estado)))
	}
	switch f.Tipo {
	case "DEBITO":
		conds = append(conds, "m.debito > 0")
	case "CREDITO":
		conds = append(conds, "m.credito > 0")
	}
	// Traslados: el que trabaja la pestaña de traslados quiere ver SOLO traslados
	// (o solo lo que no lo es) sin tener que filtrar a ojo.
	switch f.Traslado {
	case "si":
		conds = append(conds, "m.es_traslado = true")
	case "no":
		conds = append(conds, "m.es_traslado = false")
	}
	if f.Q != "" {
		n := addArg(f.Q)
		conds = append(conds, fmt.Sprintf("(m.descripcion ILIKE '%%'||$%d||'%%' OR m.documento ILIKE '%%'||$%d||'%%')", n, n))
	}
	return strings.Join(conds, " AND "), args
}

func (r *pgRepository) ListarMovimientos(ctx context.Context, empresaID string, f FiltrosMovimientos) (ListaMovimientos, error) {
	where, args := condicionesMovimientos(empresaID, f)
	addArg := func(val any) int { args = append(args, val); return len(args) }

	// Totales + conteo sobre el conjunto filtrado (sin paginar).
	//
	// Los montos salen de `monto_crc`, NO de `debito`/`credito`: esas dos columnas vienen en la
	// moneda de la CUENTA, así que sumarlas mezclaba dólares con colones. Con los datos reales
	// el total de débitos daba ₡486 123 505,52 cuando en colones son ₡511 492 482,56: un número
	// que no era ni una cosa ni la otra. Es el mismo criterio que ya usaba ResumenFiltro.
	//
	// Y como `monto_crc` vale 0 mientras la cuenta en dólares no tenga tipo de cambio del mes,
	// se cuenta aparte lo que quedó sin convertir: si no, el total en colones se queda corto
	// EN SILENCIO, que es justo el defecto que se estaba arreglando.
	var totDeb, totCred, usdSinConvertir decimal.Decimal
	var total, sinTC int
	aggQ := `SELECT COALESCE(SUM(CASE WHEN m.debito  > 0 THEN m.monto_crc ELSE 0 END), 0),
	                COALESCE(SUM(CASE WHEN m.credito > 0 THEN m.monto_crc ELSE 0 END), 0),
	                COUNT(*),
	                COUNT(*) FILTER (WHERE m.moneda_original <> 'CRC' AND m.tc_aplicado IS NULL),
	                COALESCE(SUM(m.monto_original) FILTER (WHERE m.moneda_original <> 'CRC' AND m.tc_aplicado IS NULL), 0)
	         FROM movimiento_bancario m WHERE ` + where
	if err := r.pool.QueryRow(ctx, aggQ, args...).
		Scan(&totDeb, &totCred, &total, &sinTC, &usdSinConvertir); err != nil {
		return ListaMovimientos{}, fmt.Errorf("bancos: totales movimientos: %w", err)
	}

	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 100
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	limIdx := addArg(pageSize)
	offIdx := addArg((page - 1) * pageSize)

	listQ := `
		SELECT m.id::text, m.fecha, COALESCE(m.documento,''), COALESCE(m.descripcion,''),
		       m.debito, m.credito, m.moneda_original, m.monto_crc,
		       m.concepto_id::text, COALESCE(co.nombre,''),
		       m.clasificacion_id::text, COALESCE(cl.nombre,''),
		       m.estado_clasificacion, m.confianza, m.es_traslado,
		       COALESCE(b.nombre,''), COALESCE(cb.alias,'')
		FROM movimiento_bancario m
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE ` + where + fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", ordenSQL(f.Orden), limIdx, offIdx)

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return ListaMovimientos{}, fmt.Errorf("bancos: listar movimientos: %w", err)
	}
	defer rows.Close()

	items := make([]MovimientoRow, 0, pageSize)
	for rows.Next() {
		var (
			row       MovimientoRow
			fecha     time.Time
			deb, cred decimal.Decimal
			mcrc      decimal.Decimal
			confianza decimal.NullDecimal
		)
		if err := rows.Scan(&row.ID, &fecha, &row.Documento, &row.Descripcion,
			&deb, &cred, &row.Moneda, &mcrc,
			&row.ConceptoID, &row.Concepto,
			&row.ClasificacionID, &row.Clasificacion,
			&row.Estado, &confianza, &row.EsTraslado,
			&row.Banco, &row.Cuenta); err != nil {
			return ListaMovimientos{}, fmt.Errorf("bancos: scan movimiento: %w", err)
		}
		row.Fecha = fecha.Format("2006-01-02")
		row.Debito = deb.String()
		row.Credito = cred.String()
		row.MontoCRC = mcrc.String()
		if confianza.Valid {
			s := confianza.Decimal.String()
			row.Confianza = &s
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return ListaMovimientos{}, fmt.Errorf("bancos: iterar movimientos: %w", err)
	}

	return ListaMovimientos{
		Totales: Totales{
			TotalDebitos:      totDeb.String(),
			TotalCreditos:     totCred.String(),
			Diferencia:        totCred.Sub(totDeb).String(),
			SinTipoCambio:     sinTC,
			MontoSinConvertir: usdSinConvertir.String(),
		},
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (r *pgRepository) movimientosParaClasificar(ctx context.Context, empresaID string, extraCond string, extraArgs ...any) ([]MovParaClasificar, error) {
	q := `SELECT id::text, COALESCE(descripcion,''), (debito > 0)
	      FROM movimiento_bancario
	      WHERE empresa_id = $1::uuid AND estado_clasificacion = 'NO_IDENTIFICADO'` + extraCond
	args := append([]any{empresaID}, extraArgs...)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("bancos: movimientos a clasificar: %w", err)
	}
	defer rows.Close()
	var out []MovParaClasificar
	for rows.Next() {
		var m MovParaClasificar
		if err := rows.Scan(&m.ID, &m.Descripcion, &m.EsDebito); err != nil {
			return nil, fmt.Errorf("bancos: scan mov a clasificar: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *pgRepository) MovimientosDeImportacion(ctx context.Context, empresaID, importacionID string) ([]MovParaClasificar, error) {
	return r.movimientosParaClasificar(ctx, empresaID, " AND importacion_id = $2::uuid", importacionID)
}

func (r *pgRepository) MovimientosNoIdentificados(ctx context.Context, empresaID string) ([]MovParaClasificar, error) {
	return r.movimientosParaClasificar(ctx, empresaID, "")
}

func (r *pgRepository) AplicarClasificaciones(ctx context.Context, empresaID string, updates []MovClasifUpdate) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("bancos: begin tx clasif: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Solo toca movimientos aún NO_IDENTIFICADO (no pisa una revisión manual).
	// es_traslado: un par emparejado SIEMPRE es traslado; si no hay par, decide el concepto.
	const q = `
		UPDATE movimiento_bancario
		SET concepto_id = $3::uuid, clasificacion_id = $4::uuid,
		    estado_clasificacion = 'AUTO', confianza = $5,
		    es_traslado = (par_traslado_id IS NOT NULL)
		                  OR EXISTS (SELECT 1 FROM concepto
		                             WHERE id = $3::uuid AND empresa_id = $1::uuid
		                               AND (nombre ILIKE '%traslado%' OR nombre ILIKE '%overnight%')),
		    actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado_clasificacion = 'NO_IDENTIFICADO'`
	n := 0
	aciertos := map[string]int{} // regla_id -> movimientos realmente clasificados
	for _, u := range updates {
		tag, err := tx.Exec(ctx, q, empresaID, u.MovID, u.ConceptoID, u.ClasificacionID, u.Confianza)
		if err != nil {
			return 0, fmt.Errorf("bancos: aplicar clasificación: %w", err)
		}
		if tag.RowsAffected() > 0 && u.ReglaID != "" {
			aciertos[u.ReglaID] += int(tag.RowsAffected())
		}
		n += int(tag.RowsAffected())
	}
	for reglaID, cnt := range aciertos {
		if _, err := tx.Exec(ctx,
			`UPDATE regla_clasificacion SET aciertos = aciertos + $3
			 WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, reglaID, cnt); err != nil {
			return 0, fmt.Errorf("bancos: sumar aciertos: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("bancos: commit clasif: %w", err)
	}
	return n, nil
}

func (r *pgRepository) ReclasificarMovimiento(ctx context.Context, empresaID, movID, conceptoID, clasificacionID string) error {
	// EXISTS asegura que la clasificación pertenezca a la empresa y corresponda al concepto (tenant-safe).
	// es_traslado: un par emparejado SIEMPRE es traslado (clasificarlo no rompe el par); sin par, decide el concepto.
	const q = `
		UPDATE movimiento_bancario
		SET concepto_id = $3::uuid, clasificacion_id = $4::uuid,
		    estado_clasificacion = 'REVISADO', confianza = 100,
		    es_traslado = (par_traslado_id IS NOT NULL)
		                  OR EXISTS (SELECT 1 FROM concepto
		                             WHERE id = $3::uuid AND empresa_id = $1::uuid
		                               AND (nombre ILIKE '%traslado%' OR nombre ILIKE '%overnight%')),
		    actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		  AND EXISTS (SELECT 1 FROM clasificacion
		              WHERE id = $4::uuid AND empresa_id = $1::uuid AND concepto_id = $3::uuid)`
	tag, err := r.pool.Exec(ctx, q, empresaID, movID, conceptoID, clasificacionID)
	if err != nil {
		return fmt.Errorf("bancos: reclasificar: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Distinguir: si el movimiento existe, entonces la clasificación no es válida para la empresa/concepto.
		var existe bool
		if e := r.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM movimiento_bancario WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
			empresaID, movID).Scan(&existe); e == nil && existe {
			return ErrClasificacionInvalida
		}
		return ErrMovimientoNoEncontrado
	}
	return nil
}

func (r *pgRepository) CrearRegla(ctx context.Context, empresaID string, nr NuevaRegla) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("bancos: begin tx regla: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	prioridad := nr.Prioridad
	if prioridad == 0 {
		prioridad = 100
	}
	// INSERT solo si concepto y clasificación pertenecen a la empresa y son coherentes (tenant-safe).
	const qr = `
		INSERT INTO regla_clasificacion (empresa_id, nombre, aplica_a, concepto_id, clasificacion_id, prioridad)
		SELECT $1::uuid, $2, $3, $4::uuid, $5::uuid, $6
		WHERE EXISTS (SELECT 1 FROM concepto WHERE id = $4::uuid AND empresa_id = $1::uuid)
		  AND EXISTS (SELECT 1 FROM clasificacion WHERE id = $5::uuid AND empresa_id = $1::uuid AND concepto_id = $4::uuid)
		RETURNING id::text`
	if err := tx.QueryRow(ctx, qr, empresaID, nr.Nombre, nr.AplicaA, nr.ConceptoID, nr.ClasificacionID, prioridad).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || esViolacionFK(err) {
			return "", ErrClasificacionInvalida
		}
		return "", fmt.Errorf("bancos: crear regla: %w", err)
	}
	for _, p := range nr.Palabras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO palabra_clave (regla_id, texto) VALUES ($1::uuid, $2)`, id, p); err != nil {
			return "", fmt.Errorf("bancos: crear palabra clave: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("bancos: commit regla: %w", err)
	}
	return id, nil
}

func (r *pgRepository) CrearConcepto(ctx context.Context, empresaID, nombre string, visibleCxP bool) (Concepto, error) {
	const q = `INSERT INTO concepto (empresa_id, nombre, visible_cxp) VALUES ($1::uuid, $2, $3)
	           RETURNING id::text, nombre, visible_cxp`
	var c Concepto
	err := r.pool.QueryRow(ctx, q, empresaID, nombre, visibleCxP).Scan(&c.ID, &c.Nombre, &c.VisibleCxP)
	if esViolacionUnica(err) {
		return Concepto{}, ErrCatalogoDuplicado
	}
	if err != nil {
		return Concepto{}, fmt.Errorf("bancos: crear concepto: %w", err)
	}
	return c, nil
}

func (r *pgRepository) CrearClasificacion(ctx context.Context, empresaID, conceptoID, nombre, cuentaContable string) (ClasificacionItem, error) {
	// INSERT solo si el concepto pertenece a la empresa (tenant-safe): 0 filas => concepto ajeno o inexistente.
	const q = `
		INSERT INTO clasificacion (empresa_id, concepto_id, nombre, cuenta_contable_futura)
		SELECT $1::uuid, $2::uuid, $3, NULLIF($4, '')
		WHERE EXISTS (SELECT 1 FROM concepto WHERE id = $2::uuid AND empresa_id = $1::uuid)
		RETURNING id::text, nombre`
	var ci ClasificacionItem
	err := r.pool.QueryRow(ctx, q, empresaID, conceptoID, nombre, cuentaContable).Scan(&ci.ID, &ci.Nombre)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClasificacionItem{}, ErrConceptoNoEncontrado
	}
	if esViolacionUnica(err) {
		return ClasificacionItem{}, ErrCatalogoDuplicado
	}
	if err != nil {
		return ClasificacionItem{}, fmt.Errorf("bancos: crear clasificación: %w", err)
	}
	ci.ConceptoID = conceptoID
	return ci, nil
}

func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *pgRepository) ListarConceptos(ctx context.Context, empresaID string, soloCxP bool) ([]Concepto, error) {
	// soloCxP: vista de contabilidad — únicamente los conceptos marcados visibles para CxP.
	const q = `SELECT id::text, nombre, visible_cxp, naturaleza, naturaleza_declarada FROM concepto
	           WHERE empresa_id = $1::uuid AND activo = true AND (NOT $2::bool OR visible_cxp)
	           ORDER BY nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, soloCxP)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar conceptos: %w", err)
	}
	defer rows.Close()
	var out []Concepto
	for rows.Next() {
		var c Concepto
		if err := rows.Scan(&c.ID, &c.Nombre, &c.VisibleCxP, &c.Naturaleza, &c.NaturalezaDeclarada); err != nil {
			return nil, fmt.Errorf("bancos: scan concepto: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *pgRepository) ListarClasificaciones(ctx context.Context, empresaID string, soloCxP bool) ([]ClasificacionItem, error) {
	// soloCxP: las clasificaciones heredan la visibilidad de su concepto.
	const q = `
		SELECT c.id::text, c.concepto_id::text, co.nombre, c.nombre
		FROM clasificacion c
		JOIN concepto co ON co.id = c.concepto_id
		WHERE c.empresa_id = $1::uuid AND c.activo = true AND (NOT $2::bool OR co.visible_cxp)
		ORDER BY co.nombre, c.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, soloCxP)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar clasificaciones: %w", err)
	}
	defer rows.Close()
	var out []ClasificacionItem
	for rows.Next() {
		var c ClasificacionItem
		if err := rows.Scan(&c.ID, &c.ConceptoID, &c.Concepto, &c.Nombre); err != nil {
			return nil, fmt.Errorf("bancos: scan clasificación: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// esViolacionFK detecta una violación de llave foránea (p. ej. clasificación que no
// pertenece al concepto), para traducirla a un error de dominio 422.
func esViolacionFK(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
