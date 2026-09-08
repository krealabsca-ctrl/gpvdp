package bancos

// Consultas de las dimensiones (departamento y sede) y del presupuesto por departamento.

import (
	"context"
	"fmt"
)

// ── Catálogo de sedes ────────────────────────────────────────────────────────

// ListarSedes devuelve las sedes de la empresa. Con `incluirInactivas` trae también las dadas de
// baja, que hacen falta para leer historia vieja sin que aparezca un hueco.
func (r *pgRepository) ListarSedes(ctx context.Context, empresaID string, incluirInactivas bool) ([]Sede, error) {
	const q = `SELECT id::text, nombre, COALESCE(codigo, ''), activo
	           FROM sede
	           WHERE empresa_id = $1::uuid AND ($2::bool OR activo)
	           ORDER BY orden, nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, incluirInactivas)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar sedes: %w", err)
	}
	defer rows.Close()
	out := []Sede{}
	for rows.Next() {
		var s Sede
		if err := rows.Scan(&s.ID, &s.Nombre, &s.Codigo, &s.Activo); err != nil {
			return nil, fmt.Errorf("bancos: scan sede: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CrearSede agrega una sede. Devuelve ErrCatalogoDuplicado si el nombre ya existe en la empresa.
func (r *pgRepository) CrearSede(ctx context.Context, empresaID, nombre, codigo string) (Sede, error) {
	const q = `INSERT INTO sede (empresa_id, nombre, codigo,
	                             orden)
	           VALUES ($1::uuid, $2, NULLIF($3, ''),
	                   COALESCE((SELECT MAX(orden) + 1 FROM sede WHERE empresa_id = $1::uuid), 0))
	           RETURNING id::text, nombre, COALESCE(codigo, ''), activo`
	var s Sede
	err := r.pool.QueryRow(ctx, q, empresaID, nombre, codigo).
		Scan(&s.ID, &s.Nombre, &s.Codigo, &s.Activo)
	if esViolacionUnica(err) {
		return Sede{}, ErrCatalogoDuplicado
	}
	if err != nil {
		return Sede{}, fmt.Errorf("bancos: crear sede: %w", err)
	}
	return s, nil
}

// ActualizarSede renombra o cambia el código de una sede.
func (r *pgRepository) ActualizarSede(ctx context.Context, empresaID, sedeID, nombre, codigo string) error {
	const q = `UPDATE sede SET nombre = $3, codigo = NULLIF($4, ''), actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, sedeID, nombre, codigo)
	if esViolacionUnica(err) {
		return ErrCatalogoDuplicado
	}
	if err != nil {
		return fmt.Errorf("bancos: actualizar sede: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSedeNoEncontrada
	}
	return nil
}

// CambiarActivoSede da de baja (o revive) una sede. Nunca se borra físicamente: si una sede tiene
// historia, borrarla dejaría movimientos apuntando a la nada.
func (r *pgRepository) CambiarActivoSede(ctx context.Context, empresaID, sedeID string, activo bool) error {
	const q = `UPDATE sede SET activo = $3, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, sedeID, activo)
	if err != nil {
		return fmt.Errorf("bancos: cambiar activo de sede: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSedeNoEncontrada
	}
	return nil
}

// ── Los defaults por partida ─────────────────────────────────────────────────

// AsignarDimensionesClasificacion pone (o quita) el departamento y la sede POR DEFECTO de una
// partida. Un id vacío significa «quitar»: se guarda NULL y esa partida vuelve a «sin asignar».
//
// Devuelve cuántos movimientos quedan afectados por el cambio, para poder decírselo al usuario. No
// se actualiza ni un movimiento: el valor se resuelve al leer, y ese es justamente el punto.
func (r *pgRepository) AsignarDimensionesClasificacion(ctx context.Context, empresaID, clasifID, deptoID, sedeID string) (int, error) {
	const q = `UPDATE clasificacion
	           SET departamento_id = NULLIF($3, '')::uuid,
	               sede_id = NULLIF($4, '')::uuid
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, clasifID, deptoID, sedeID)
	if err != nil {
		return 0, fmt.Errorf("bancos: asignar dimensiones a la clasificación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, ErrClasificacionNoEncontrada
	}

	// Cuántos movimientos hereda: es el número que hace visible el alcance de un clic.
	var n int
	const conteo = `SELECT count(*) FROM movimiento_bancario
	                WHERE empresa_id = $1::uuid AND clasificacion_id = $2::uuid AND incluido`
	if err := r.pool.QueryRow(ctx, conteo, empresaID, clasifID).Scan(&n); err != nil {
		return 0, fmt.Errorf("bancos: contar movimientos de la clasificación: %w", err)
	}
	return n, nil
}

// AsignarDimensionesMovimiento escribe la EXCEPCIÓN de un movimiento puntual.
func (r *pgRepository) AsignarDimensionesMovimiento(ctx context.Context, empresaID, movID, deptoID, sedeID string) error {
	const q = `UPDATE movimiento_bancario
	           SET departamento_id = NULLIF($3, '')::uuid,
	               sede_id = NULLIF($4, '')::uuid,
	               actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, movID, deptoID, sedeID)
	if err != nil {
		return fmt.Errorf("bancos: asignar dimensiones al movimiento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMovimientoNoEncontrado
	}
	return nil
}

// ── El reporte: gasto real por dimensión ─────────────────────────────────────

// GastoPorDimension devuelve el gasto del rango agrupado por departamento o por sede.
//
// Solo GASTO: la naturaleza del concepto decide (la misma definición que el EBITDA, ver
// naturaleza.go). Un traslado de fondos o un ingreso no tienen «departamento que gastó», y meterlos
// haría que un departamento con muchos depósitos apareciera gastando de más.
//
// `agruparPor` es "departamento" o "sede" — validado en el service, nunca viene del cliente crudo.
func (r *pgRepository) GastoPorDimension(ctx context.Context, empresaID, desde, hasta, agruparPor string) ([]GastoDimension, error) {
	idExpr, nombreExpr := sqlDepartamentoEfectivoID, sqlDepartamentoNombre
	if agruparPor == AgruparPorSede {
		idExpr, nombreExpr = sqlSedeEfectivaID, sqlSedeNombre
	}

	q := `
		SELECT COALESCE(` + idExpr + `::text, ''),
		       ` + nombreExpr + `,
		       count(*)::int,
		       COALESCE(SUM(CASE WHEN m.debito > 0 THEN m.monto_crc ELSE -m.monto_crc END), 0)::text
		FROM movimiento_bancario m
		` + joinClasifParaDimensiones + `
		LEFT JOIN concepto co ON co.id = m.concepto_id
		` + joinDimensiones + `
		WHERE m.empresa_id = $1::uuid AND m.incluido
		  AND NOT m.es_traslado
		  AND co.naturaleza = 'GASTO'
		  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3
		GROUP BY 1, 2
		ORDER BY 4 DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: gasto por dimensión: %w", err)
	}
	defer rows.Close()
	out := []GastoDimension{}
	for rows.Next() {
		var g GastoDimension
		if err := rows.Scan(&g.ID, &g.Nombre, &g.Movs, &g.Gasto); err != nil {
			return nil, fmt.Errorf("bancos: scan gasto por dimensión: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// PartidasDeDimension devuelve, para UN departamento (o sede), en qué partidas gastó. Es el paso
// siguiente obligado: saber que Logística se pasó no sirve si no se puede ver en qué.
func (r *pgRepository) PartidasDeDimension(ctx context.Context, empresaID, desde, hasta, agruparPor, dimID string) ([]GastoPartidaDimension, error) {
	idExpr := sqlDepartamentoEfectivoID
	if agruparPor == AgruparPorSede {
		idExpr = sqlSedeEfectivaID
	}
	// El id vacío pide justamente lo que NO tiene dimensión asignada.
	condicion := idExpr + ` = $4::uuid`
	if dimID == "" {
		condicion = idExpr + ` IS NULL`
	}

	// El id de la partida viaja en la respuesta: la pantalla necesita saber A QUÉ clasificación le
	// está poniendo el departamento, y resolverlo por nombre desde el cliente obliga a cargar el
	// catálogo completo y falla con dos partidas homónimas en conceptos distintos.
	//
	// El subpresupuesto se busca AFUERA del agregado, en un segundo nivel sobre el CTE. Colgar el
	// subquery del SELECT agregado no compila: PostgreSQL lo rechaza con «subquery uses ungrouped
	// column», porque agrupar por `COALESCE(cl.id::text, '')` no le prueba que `cl.id` esté agrupado.
	// Se suma solo sobre las filas CON clasificación: las de clasificacion_id NULL son el total del
	// departamento y contarlas acá duplicaría el dinero.
	q := `
		WITH gasto AS (
			SELECT COALESCE(cl.id::text, '')            AS clasificacion_id,
			       COALESCE(co.nombre, '(sin concepto)') AS concepto,
			       COALESCE(cl.nombre, '(sin clasificar)') AS clasificacion,
			       count(*)::int                        AS movs,
			       COALESCE(SUM(CASE WHEN m.debito > 0 THEN m.monto_crc ELSE -m.monto_crc END), 0) AS gasto,
			       ` + sqlOrigenDimension + ` AS origen,
			       m.empresa_id                         AS empresa_id,
			       ` + sqlDepartamentoEfectivoID + `    AS departamento_id
			FROM movimiento_bancario m
			` + joinClasifParaDimensiones + `
			LEFT JOIN concepto co ON co.id = m.concepto_id
			` + joinDimensiones + `
			WHERE m.empresa_id = $1::uuid AND m.incluido
			  AND NOT m.es_traslado
			  AND co.naturaleza = 'GASTO'
			  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3
			  AND ` + condicion + `
			GROUP BY 1, 2, 3, 6, 7, 8
		)
		SELECT g.clasificacion_id, g.concepto, g.clasificacion, g.movs, g.gasto::text, g.origen,
		       COALESCE((SELECT SUM(pp.monto_crc) FROM presupuesto_departamento pp
		                 WHERE pp.empresa_id = g.empresa_id
		                   AND pp.departamento_id = g.departamento_id
		                   AND pp.clasificacion_id::text = g.clasificacion_id
		                   AND pp.periodo BETWEEN $2 AND $3), 0)::text
		FROM gasto g
		ORDER BY g.gasto DESC`

	args := []any{empresaID, desde, hasta}
	if dimID != "" {
		args = append(args, dimID)
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("bancos: partidas de la dimensión: %w", err)
	}
	defer rows.Close()
	out := []GastoPartidaDimension{}
	for rows.Next() {
		var g GastoPartidaDimension
		if err := rows.Scan(&g.ClasificacionID, &g.Concepto, &g.Clasificacion, &g.Movs, &g.Gasto,
			&g.Origen, &g.Subpresupuesto); err != nil {
			return nil, fmt.Errorf("bancos: scan partida de dimensión: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ── Presupuesto ──────────────────────────────────────────────────────────────

// PresupuestoDelRango devuelve lo presupuestado por departamento en el rango de meses.
func (r *pgRepository) PresupuestoDelRango(ctx context.Context, empresaID, desde, hasta string) ([]PresupuestoLinea, error) {
	// Vienen las dos clases de fila. `clasificacion_id` vacío = el total del departamento; con valor =
	// el subpresupuesto de esa partida. Quien suma decide cuál usa: mezclarlas contaría doble.
	const q = `
		SELECT p.departamento_id::text, d.nombre, p.periodo, p.monto_crc::text, COALESCE(p.nota, ''),
		       COALESCE(p.clasificacion_id::text, ''), COALESCE(cl.nombre, ''), COALESCE(co.nombre, '')
		FROM presupuesto_departamento p
		JOIN departamento d ON d.id = p.departamento_id
		LEFT JOIN clasificacion cl ON cl.id = p.clasificacion_id
		LEFT JOIN concepto co ON co.id = cl.concepto_id
		WHERE p.empresa_id = $1::uuid AND p.periodo BETWEEN $2 AND $3
		ORDER BY d.orden, d.nombre, p.periodo, cl.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: presupuesto del rango: %w", err)
	}
	defer rows.Close()
	out := []PresupuestoLinea{}
	for rows.Next() {
		var l PresupuestoLinea
		if err := rows.Scan(&l.DepartamentoID, &l.Departamento, &l.Periodo, &l.Monto, &l.Nota,
			&l.ClasificacionID, &l.Clasificacion, &l.Concepto); err != nil {
			return nil, fmt.Errorf("bancos: scan línea de presupuesto: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GuardarPresupuesto escribe (o reemplaza) el monto de un departamento en un mes. Un monto en cero
// es legítimo: significa «este departamento no tiene presupuesto este mes», que es distinto de no
// haberlo definido.
func (r *pgRepository) GuardarPresupuesto(ctx context.Context, empresaID, deptoID, clasifID, periodo, monto, nota, usuarioID string) error {
	// Dos ON CONFLICT distintos porque la unicidad son dos índices PARCIALES: uno para el total
	// (clasificacion_id NULL) y otro para el subpresupuesto. Un solo `ON CONFLICT (a,b,c)` no calza
	// con un índice parcial y Postgres lo rechaza.
	const total = `
		INSERT INTO presupuesto_departamento (empresa_id, departamento_id, periodo, monto_crc, nota, creado_por)
		VALUES ($1::uuid, $2::uuid, $3, $4::numeric, NULLIF($5, ''), NULLIF($6, '')::uuid)
		ON CONFLICT (empresa_id, departamento_id, periodo) WHERE clasificacion_id IS NULL
		DO UPDATE SET monto_crc = EXCLUDED.monto_crc, nota = EXCLUDED.nota, actualizado_en = now()`
	const porPartida = `
		INSERT INTO presupuesto_departamento (empresa_id, departamento_id, clasificacion_id, periodo, monto_crc, nota, creado_por)
		VALUES ($1::uuid, $2::uuid, $7::uuid, $3, $4::numeric, NULLIF($5, ''), NULLIF($6, '')::uuid)
		ON CONFLICT (empresa_id, departamento_id, periodo, clasificacion_id) WHERE clasificacion_id IS NOT NULL
		DO UPDATE SET monto_crc = EXCLUDED.monto_crc, nota = EXCLUDED.nota, actualizado_en = now()`

	var err error
	if clasifID == "" {
		_, err = r.pool.Exec(ctx, total, empresaID, deptoID, periodo, monto, nota, usuarioID)
	} else {
		_, err = r.pool.Exec(ctx, porPartida, empresaID, deptoID, periodo, monto, nota, usuarioID, clasifID)
	}
	if err != nil {
		return fmt.Errorf("bancos: guardar presupuesto: %w", err)
	}
	return nil
}

// BorrarPresupuesto quita la línea de un departamento en un mes (vuelve a «no definido»).
func (r *pgRepository) BorrarPresupuesto(ctx context.Context, empresaID, deptoID, clasifID, periodo string) error {
	// `clasificacion_id IS NOT DISTINCT FROM` y no `=`: con NULL, `=` nunca es verdadero y el borrado
	// del total no encontraría su fila.
	const q = `DELETE FROM presupuesto_departamento
	           WHERE empresa_id = $1::uuid AND departamento_id = $2::uuid AND periodo = $3
	             AND clasificacion_id IS NOT DISTINCT FROM NULLIF($4, '')::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, deptoID, periodo, clasifID)
	if err != nil {
		return fmt.Errorf("bancos: borrar presupuesto: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPresupuestoNoEncontrado
	}
	return nil
}

// DepartamentosActivos devuelve el catálogo de departamentos de la empresa. Vive en el paquete de
// CxP (migración 0026) pero es de toda la empresa: Bancos lo LEE, no lo administra.
func (r *pgRepository) DepartamentosActivos(ctx context.Context, empresaID string) ([]Departamento, error) {
	const q = `SELECT id::text, nombre, COALESCE(codigo, ''), activo
	           FROM departamento WHERE empresa_id = $1::uuid AND activo
	           ORDER BY orden, nombre`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: departamentos: %w", err)
	}
	defer rows.Close()
	out := []Departamento{}
	for rows.Next() {
		var d Departamento
		if err := rows.Scan(&d.ID, &d.Nombre, &d.Codigo, &d.Activo); err != nil {
			return nil, fmt.Errorf("bancos: scan departamento: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
