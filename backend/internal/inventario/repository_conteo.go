package inventario

// Acceso a datos del conteo cíclico.
//
// Las dos operaciones que importan:
//
//  · **Abrir** congela la foto del sistema en cada línea. Todo en una transacción: una hoja con la
//    mitad de las líneas congeladas y la otra mitad no sería incomparable.
//  · **Cerrar** convierte cada diferencia explicada en su movimiento de ajuste, con el motivo del
//    conteo. Si eso quedara para después, el sistema seguiría afirmando un número que la bodega ya
//    desmintió.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// AbrirConteo crea la hoja y la llena con lo que el sistema dice que hay en esa sede.
func (r *pgRepository) AbrirConteo(ctx context.Context, empresaID string, c ConteoNuevo, usuarioID string) (Conteo, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Conteo{}, fmt.Errorf("inventario: begin conteo: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sede, err := nombreSede(ctx, tx, empresaID, c.SedeID)
	if err != nil {
		return Conteo{}, err
	}
	n, err := siguienteConsecutivo(ctx, tx, empresaID, "CONTEO")
	if err != nil {
		return Conteo{}, err
	}
	numero := fmt.Sprintf("CT-%s-%04d", c.Fecha[:4], n)

	var conteoID string
	err = tx.QueryRow(ctx, `
		INSERT INTO inv_conteo (empresa_id, numero, sede_id, categoria_id, estado,
		                        abierto_en, abierto_por, nota)
		VALUES ($1::uuid, $2, $3::uuid, NULLIF($4,'')::uuid, 'ABIERTO', $5::date,
		        NULLIF($6,'')::uuid, NULLIF($7,''))
		RETURNING id::text`,
		empresaID, numero, c.SedeID, c.CategoriaID, c.Fecha, usuarioID, c.Nota).Scan(&conteoID)
	if esViolacionUnica(err) {
		// El índice parcial de una sola hoja abierta por sede.
		return Conteo{}, ErrConteoAbiertoEnLaSede
	}
	if err != nil {
		return Conteo{}, fmt.Errorf("inventario: crear conteo: %w", err)
	}

	// Las líneas de modo UNIDAD: una por ficha presente en la sede. La pregunta es «¿está este
	// cofre?», así que la cantidad del sistema es 1 por ficha.
	filtroCat := ` AND ($2 = '' OR a.categoria_id = NULLIF($2,'')::uuid OR c.padre_id = NULLIF($2,'')::uuid)`
	tagU, err := tx.Exec(ctx, `
		INSERT INTO inv_conteo_linea (empresa_id, conteo_id, articulo_id, unidad_id, cantidad_sistema)
		SELECT $3::uuid, $4::uuid, u.articulo_id, u.id, 1
		FROM inv_unidad u
		JOIN inv_articulo a ON a.id = u.articulo_id
		JOIN inv_categoria c ON c.id = a.categoria_id
		WHERE u.empresa_id = $3::uuid AND u.sede_id = $1::uuid
		  AND u.estado IN `+estadosQueCuentan+filtroCat,
		c.SedeID, c.CategoriaID, empresaID, conteoID)
	if err != nil {
		return Conteo{}, fmt.Errorf("inventario: líneas de unidades: %w", err)
	}

	// Las de modo CANTIDAD: una por artículo con existencia distinta de cero en la sede.
	tagC, err := tx.Exec(ctx, `
		INSERT INTO inv_conteo_linea (empresa_id, conteo_id, articulo_id, cantidad_sistema)
		SELECT $3::uuid, $4::uuid, m.articulo_id, SUM(`+sqlSignoMovimiento+`)::int
		FROM inv_movimiento m
		JOIN inv_articulo a ON a.id = m.articulo_id
		JOIN inv_categoria c ON c.id = a.categoria_id
		WHERE m.empresa_id = $3::uuid AND m.sede_id = $1::uuid
		  AND a.modo_control = 'CANTIDAD'`+filtroCat+`
		GROUP BY m.articulo_id
		HAVING SUM(`+sqlSignoMovimiento+`) > 0`,
		c.SedeID, c.CategoriaID, empresaID, conteoID)
	if err != nil {
		return Conteo{}, fmt.Errorf("inventario: líneas de cantidades: %w", err)
	}

	if tagU.RowsAffected()+tagC.RowsAffected() == 0 {
		// Una hoja vacía no es una hoja: se rechaza en vez de dejar un conteo que nunca va a cerrar.
		return Conteo{}, ErrConteoSinLineas
	}
	if err := tx.Commit(ctx); err != nil {
		return Conteo{}, fmt.Errorf("inventario: commit conteo: %w", err)
	}

	return Conteo{
		ID: conteoID, Numero: numero, SedeID: c.SedeID, Sede: sede,
		CategoriaID: c.CategoriaID, Estado: ConteoAbierto, AbiertoEn: c.Fecha,
		Nota: c.Nota, Lineas: int(tagU.RowsAffected() + tagC.RowsAffected()),
		Filas: []ConteoLinea{},
	}, nil
}

// selectConteoLinea proyecta una línea con todo lo que la pantalla necesita, en un solo lugar.
const selectConteoLinea = `
	SELECT l.id::text, a.id::text, a.codigo, a.nombre,
	       CASE WHEN cp.nombre IS NULL THEN cat.nombre ELSE cp.nombre || ' › ' || cat.nombre END,
	       a.modo_control, COALESCE(l.unidad_id::text, ''), COALESCE(u.numero, ''),
	       l.cantidad_sistema, l.cantidad_contada, COALESCE(l.motivo, ''),
	       CASE WHEN a.modo_control = 'UNIDAD' THEN COALESCE(u.costo_crc, 0)
	            ELSE ` + sqlCostoPromedio + ` END::text
	FROM inv_conteo_linea l
	JOIN inv_articulo a ON a.id = l.articulo_id
	JOIN inv_categoria cat ON cat.id = a.categoria_id
	LEFT JOIN inv_categoria cp ON cp.id = cat.padre_id
	LEFT JOIN inv_unidad u ON u.id = l.unidad_id`

func escanearLineas(rows pgx.Rows) ([]ConteoLinea, error) {
	out := []ConteoLinea{}
	for rows.Next() {
		var l ConteoLinea
		var contada *int
		var costo string
		if err := rows.Scan(&l.ID, &l.ArticuloID, &l.Codigo, &l.Articulo, &l.Categoria,
			&l.ModoControl, &l.UnidadID, &l.UnidadNumero, &l.CantidadSistema, &contada,
			&l.Motivo, &costo); err != nil {
			return nil, fmt.Errorf("inventario: scan línea de conteo: %w", err)
		}
		l.Estado = estadoDeLinea(contada, l.CantidadSistema, l.Motivo)
		l.CostoUnitarioCRC = costo
		if contada == nil {
			// -1 y no 0: el cliente tiene que poder distinguir «sin contar» de «contó cero».
			l.CantidadContada = -1
		} else {
			l.CantidadContada = *contada
			l.Diferencia = *contada - l.CantidadSistema
			cu, err := decimal.NewFromString(costo)
			if err != nil {
				cu = decimal.Zero
			}
			l.ImpactoCRC = cu.Mul(decimal.NewFromInt(int64(l.Diferencia))).StringFixed(2)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ConteoPorID trae la hoja con sus líneas.
func (r *pgRepository) ConteoPorID(ctx context.Context, empresaID, id string) (Conteo, error) {
	var c Conteo
	var categoriaID, categoria, cerradoEn, abiertoPor, cerradoPor, nota *string
	err := r.pool.QueryRow(ctx, `
		SELECT ct.id::text, ct.numero, ct.sede_id::text, s.nombre,
		       ct.categoria_id::text, cat.nombre, ct.estado,
		       to_char(ct.abierto_en, 'YYYY-MM-DD'), to_char(ct.cerrado_en, 'YYYY-MM-DD'),
		       ua.nombre, uc.nombre, ct.nota
		FROM inv_conteo ct
		JOIN sede s ON s.id = ct.sede_id
		LEFT JOIN inv_categoria cat ON cat.id = ct.categoria_id
		LEFT JOIN usuario ua ON ua.id = ct.abierto_por
		LEFT JOIN usuario uc ON uc.id = ct.cerrado_por
		WHERE ct.empresa_id = $1::uuid AND ct.id = $2::uuid`,
		empresaID, id).Scan(&c.ID, &c.Numero, &c.SedeID, &c.Sede, &categoriaID, &categoria,
		&c.Estado, &c.AbiertoEn, &cerradoEn, &abiertoPor, &cerradoPor, &nota)
	if errors.Is(err, pgx.ErrNoRows) {
		return Conteo{}, ErrConteoNoEncontrado
	}
	if err != nil {
		return Conteo{}, fmt.Errorf("inventario: conteo por id: %w", err)
	}
	c.CategoriaID = deref(categoriaID)
	c.Categoria = deref(categoria)
	c.CerradoEn = deref(cerradoEn)
	c.AbiertoPor = deref(abiertoPor)
	c.CerradoPor = deref(cerradoPor)
	c.Nota = deref(nota)

	rows, err := r.pool.Query(ctx,
		selectConteoLinea+` WHERE l.empresa_id = $1::uuid AND l.conteo_id = $2::uuid
		ORDER BY 5, a.nombre, u.numero`, empresaID, id)
	if err != nil {
		return Conteo{}, fmt.Errorf("inventario: líneas del conteo: %w", err)
	}
	defer rows.Close()
	filas, err := escanearLineas(rows)
	if err != nil {
		return Conteo{}, err
	}
	c.Filas = filas
	return c, nil
}

// ListarConteos lista las hojas, opcionalmente por estado o sede.
func (r *pgRepository) ListarConteos(ctx context.Context, empresaID, estado, sedeID string) ([]Conteo, error) {
	q := `
		SELECT ct.id::text, ct.numero, ct.sede_id::text, s.nombre,
		       COALESCE(ct.categoria_id::text, ''), COALESCE(cat.nombre, ''), ct.estado,
		       to_char(ct.abierto_en, 'YYYY-MM-DD'), COALESCE(to_char(ct.cerrado_en, 'YYYY-MM-DD'), ''),
		       COALESCE(ua.nombre, ''), COALESCE(uc.nombre, ''), COALESCE(ct.nota, ''),
		       (SELECT count(*)::int FROM inv_conteo_linea l WHERE l.conteo_id = ct.id),
		       (SELECT count(*)::int FROM inv_conteo_linea l
		        WHERE l.conteo_id = ct.id AND l.cantidad_contada IS NOT NULL),
		       (SELECT count(*)::int FROM inv_conteo_linea l
		        WHERE l.conteo_id = ct.id AND l.cantidad_contada IS NOT NULL
		          AND l.cantidad_contada <> l.cantidad_sistema),
		       (SELECT count(*)::int FROM inv_conteo_linea l
		        WHERE l.conteo_id = ct.id AND l.cantidad_contada IS NOT NULL
		          AND l.cantidad_contada <> l.cantidad_sistema
		          AND COALESCE(l.motivo, '') = '')
		FROM inv_conteo ct
		JOIN sede s ON s.id = ct.sede_id
		LEFT JOIN inv_categoria cat ON cat.id = ct.categoria_id
		LEFT JOIN usuario ua ON ua.id = ct.abierto_por
		LEFT JOIN usuario uc ON uc.id = ct.cerrado_por
		WHERE ct.empresa_id = $1::uuid`
	args := []any{empresaID}
	if estado != "" {
		args = append(args, estado)
		q += fmt.Sprintf(` AND ct.estado = $%d`, len(args))
	}
	if sedeID != "" {
		args = append(args, sedeID)
		q += fmt.Sprintf(` AND ct.sede_id = $%d::uuid`, len(args))
	}
	q += ` ORDER BY (ct.estado = 'ABIERTO') DESC, ct.abierto_en DESC LIMIT 100`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: listar conteos: %w", err)
	}
	defer rows.Close()
	out := []Conteo{}
	for rows.Next() {
		var c Conteo
		if err := rows.Scan(&c.ID, &c.Numero, &c.SedeID, &c.Sede, &c.CategoriaID, &c.Categoria,
			&c.Estado, &c.AbiertoEn, &c.CerradoEn, &c.AbiertoPor, &c.CerradoPor, &c.Nota,
			&c.Lineas, &c.Contadas, &c.ConDiferencia, &c.SinExplicar); err != nil {
			return nil, fmt.Errorf("inventario: scan conteo: %w", err)
		}
		c.Filas = []ConteoLinea{}
		c.PuedeCerrarse = c.Estado == ConteoAbierto && c.Contadas == c.Lineas && c.SinExplicar == 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// GuardarLineaConteo anota lo contado y su motivo. Solo sobre hojas abiertas.
func (r *pgRepository) GuardarLineaConteo(ctx context.Context, empresaID, conteoID, lineaID string, contada int, motivo, usuarioID string) error {
	// El estado de la hoja se comprueba en el mismo UPDATE: si se leyera antes por separado, entre
	// la lectura y la escritura alguien podría cerrarla y el dato entraría en una hoja cerrada.
	const q = `
		UPDATE inv_conteo_linea l
		SET cantidad_contada = $4, motivo = NULLIF($5, ''), contado_en = now(),
		    contado_por = NULLIF($6,'')::uuid
		WHERE l.empresa_id = $1::uuid AND l.id = $3::uuid AND l.conteo_id = $2::uuid
		  AND EXISTS (SELECT 1 FROM inv_conteo ct
		              WHERE ct.id = $2::uuid AND ct.empresa_id = $1::uuid AND ct.estado = 'ABIERTO')`
	tag, err := r.pool.Exec(ctx, q, empresaID, conteoID, lineaID, contada, motivo, usuarioID)
	if err != nil {
		return fmt.Errorf("inventario: guardar línea de conteo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// O la línea no es de esta hoja, o la hoja ya no está abierta. Se distingue para poder
		// decirlo: son dos problemas con soluciones distintas.
		var estado string
		err := r.pool.QueryRow(ctx,
			`SELECT estado FROM inv_conteo WHERE empresa_id = $1::uuid AND id = $2::uuid`,
			empresaID, conteoID).Scan(&estado)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConteoNoEncontrado
		}
		if err == nil && estado != ConteoAbierto {
			return ErrConteoYaCerrado
		}
		return ErrLineaNoEncontrada
	}
	return nil
}

// CerrarConteo cierra la hoja y convierte cada diferencia explicada en su movimiento de ajuste.
//
// Devuelve cuántos ajustes generó: es el número que la pantalla necesita para decir qué pasó de
// verdad, en lugar de un «listo» que no dice nada.
func (r *pgRepository) CerrarConteo(ctx context.Context, empresaID, conteoID, fecha, usuarioID string) (CierreConteo, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CierreConteo{}, fmt.Errorf("inventario: begin cierre: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var estado, sedeID, numero string
	err = tx.QueryRow(ctx, `
		SELECT estado, sede_id::text, numero FROM inv_conteo
		WHERE empresa_id = $1::uuid AND id = $2::uuid FOR UPDATE`,
		empresaID, conteoID).Scan(&estado, &sedeID, &numero)
	if errors.Is(err, pgx.ErrNoRows) {
		return CierreConteo{}, ErrConteoNoEncontrado
	}
	if err != nil {
		return CierreConteo{}, fmt.Errorf("inventario: buscar conteo: %w", err)
	}
	if estado != ConteoAbierto {
		return CierreConteo{}, ErrConteoYaCerrado
	}

	// Guarda 1: nada sin contar. Cerrar con líneas en blanco daría por verificado lo que nadie miró.
	var total, contadas int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)::int, count(cantidad_contada)::int
		FROM inv_conteo_linea WHERE empresa_id = $1::uuid AND conteo_id = $2::uuid`,
		empresaID, conteoID).Scan(&total, &contadas); err != nil {
		return CierreConteo{}, fmt.Errorf("inventario: avance del conteo: %w", err)
	}
	if contadas < total {
		return CierreConteo{}, &SinContarError{Cuantas: total - contadas, Total: total}
	}

	// Guarda 2: ninguna diferencia muda.
	rows, err := tx.Query(ctx, `
		SELECT a.nombre FROM inv_conteo_linea l
		JOIN inv_articulo a ON a.id = l.articulo_id
		WHERE l.empresa_id = $1::uuid AND l.conteo_id = $2::uuid
		  AND l.cantidad_contada <> l.cantidad_sistema AND COALESCE(l.motivo, '') = ''
		ORDER BY a.nombre`, empresaID, conteoID)
	if err != nil {
		return CierreConteo{}, fmt.Errorf("inventario: diferencias sin explicar: %w", err)
	}
	var mudas []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return CierreConteo{}, fmt.Errorf("inventario: scan diferencia: %w", err)
		}
		mudas = append(mudas, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return CierreConteo{}, err
	}
	if len(mudas) > 0 {
		return CierreConteo{}, &DiferenciasSinExplicarError{Cuantas: len(mudas), Articulos: mudas}
	}

	// Las diferencias explicadas se vuelven movimientos.
	drows, err := tx.Query(ctx, `
		SELECT l.articulo_id::text, COALESCE(l.unidad_id::text, ''), a.modo_control,
		       l.cantidad_sistema, l.cantidad_contada, COALESCE(l.motivo, ''),
		       CASE WHEN a.modo_control = 'UNIDAD' THEN COALESCE(u.costo_crc, 0)
		            ELSE `+sqlCostoPromedio+` END::text
		FROM inv_conteo_linea l
		JOIN inv_articulo a ON a.id = l.articulo_id
		LEFT JOIN inv_unidad u ON u.id = l.unidad_id
		WHERE l.empresa_id = $1::uuid AND l.conteo_id = $2::uuid
		  AND l.cantidad_contada <> l.cantidad_sistema`, empresaID, conteoID)
	if err != nil {
		return CierreConteo{}, fmt.Errorf("inventario: diferencias del conteo: %w", err)
	}
	type dif struct {
		articulo, unidad, modo, motivo, costo string
		sistema, contada                      int
	}
	var difs []dif
	for drows.Next() {
		var d dif
		if err := drows.Scan(&d.articulo, &d.unidad, &d.modo, &d.sistema, &d.contada,
			&d.motivo, &d.costo); err != nil {
			drows.Close()
			return CierreConteo{}, fmt.Errorf("inventario: scan diferencia: %w", err)
		}
		difs = append(difs, d)
	}
	drows.Close()
	if err := drows.Err(); err != nil {
		return CierreConteo{}, err
	}

	out := CierreConteo{Numero: numero}

	// El impacto lo calcula UNA sola función, a partir de las unidades que el movimiento generado
	// mueve de verdad. Cuando cada rama lo sumaba por su cuenta, la rama de unidad se saltaba la
	// suma: un cofre de ₡423.750 que no apareció valía ₡0 en el resumen del cierre, mientras la
	// línea en pantalla sí mostraba su costo. Dos capas calculando el mismo número terminan
	// diciendo cosas distintas.
	sumarImpacto := func(costo string, unidades int) {
		c, err := decimal.NewFromString(costo)
		if err != nil {
			return
		}
		out.impacto = out.impacto.Add(c.Mul(decimal.NewFromInt(int64(unidades))))
	}

	for _, d := range difs {
		motivo := fmt.Sprintf("conteo %s: %s", numero, d.motivo)

		if d.modo == ModoUnidad {
			// En modo unidad la pregunta es de presencia: si no apareció, se da de baja. Un ajuste
			// de cantidad dejaría la ficha «disponible» para un cofre que no está en la bodega.
			if d.contada == 0 {
				if _, err := tx.Exec(ctx, `
					UPDATE inv_unidad SET estado = 'NO_APARECIO', actualizado_en = now()
					WHERE id = $1::uuid AND empresa_id = $2::uuid`, d.unidad, empresaID); err != nil {
					return CierreConteo{}, fmt.Errorf("inventario: dar de baja unidad no encontrada: %w", err)
				}
				if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
					EmpresaID: empresaID, ArticuloID: d.articulo, UnidadID: d.unidad,
					Tipo: MovBaja, Cantidad: 1, SedeID: sedeID, Costo: d.costo,
					Fecha: fecha, Motivo: motivo, UsuarioID: usuarioID,
				}); err != nil {
					return CierreConteo{}, err
				}
				out.Bajas++
				sumarImpacto(d.costo, -1)
			}
			continue
		}

		// Modo cantidad: el signo de la diferencia elige el tipo.
		tipo, cantidad := MovAjusteMas, d.contada-d.sistema
		if cantidad < 0 {
			tipo, cantidad = MovAjusteMenos, -cantidad
		}
		if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
			EmpresaID: empresaID, ArticuloID: d.articulo, Tipo: tipo, Cantidad: cantidad,
			SedeID: sedeID, Costo: d.costo, Fecha: fecha, Motivo: motivo, UsuarioID: usuarioID,
		}); err != nil {
			return CierreConteo{}, err
		}
		if tipo == MovAjusteMas {
			out.AjustesQueSuman++
		} else {
			out.AjustesQueRestan++
		}
		sumarImpacto(d.costo, d.contada-d.sistema)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE inv_conteo SET estado = 'CERRADO', cerrado_en = $3::date,
		       cerrado_por = NULLIF($4,'')::uuid
		WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, conteoID, fecha, usuarioID); err != nil {
		return CierreConteo{}, fmt.Errorf("inventario: cerrar conteo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CierreConteo{}, fmt.Errorf("inventario: commit cierre: %w", err)
	}
	out.ImpactoCRC = out.impacto.StringFixed(2)
	out.Lineas = total
	return out, nil
}

// AnularConteo descarta una hoja sin generar ningún ajuste.
//
// Existe porque la alternativa es peor: una hoja abierta que nadie va a terminar bloquea la sede
// para siempre (solo puede haber una), y forzar a cerrarla generaría ajustes falsos.
func (r *pgRepository) AnularConteo(ctx context.Context, empresaID, conteoID, motivo string) error {
	const q = `
		UPDATE inv_conteo SET estado = 'ANULADO',
		       nota = COALESCE(nota || ' · ', '') || 'anulado: ' || $3
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'ABIERTO'`
	tag, err := r.pool.Exec(ctx, q, empresaID, conteoID, motivo)
	if err != nil {
		return fmt.Errorf("inventario: anular conteo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConteoYaCerrado
	}
	return nil
}

// PlanDeConteo dice qué toca contar: por sede y clase, con cuánto hace que no se cuenta.
//
// La clase sale del capital acumulado (A hasta el 80 %, B hasta el 95 %, C el resto), calculado por
// sede: la clase de un artículo depende de cuánto pesa DONDE está.
func (r *pgRepository) PlanDeConteo(ctx context.Context, empresaID string) ([]PlanConteo, error) {
	const q = `
		WITH existencia AS (
			-- Acá el capital SÍ incluye lo consignado, al contrario de la rotación, y es a
			-- propósito: el conteo mide presencia física, y perder un cofre del proveedor es el
			-- PEOR caso, no el más leve. Si al usarlo nace la cuenta por pagar, perderlo obliga a
			-- pagarlo igual pero sin haber cobrado nada. Contarlo menos seguido por no ser capital
			-- propio sería exactamente al revés de lo que conviene.
			SELECT a.id AS articulo_id, u.sede_id, count(*)::int AS hay,
			       COALESCE(SUM(u.costo_crc), 0) AS capital
			FROM inv_unidad u JOIN inv_articulo a ON a.id = u.articulo_id
			WHERE u.empresa_id = $1::uuid AND u.sede_id IS NOT NULL
			  AND u.estado IN ` + estadosQueCuentan + `
			GROUP BY a.id, u.sede_id
			UNION ALL
			SELECT a.id, m.sede_id, SUM(` + sqlSignoMovimiento + `)::int,
			       SUM(` + sqlSignoMovimiento + `) * ` + sqlCostoPromedio + `
			FROM inv_movimiento m JOIN inv_articulo a ON a.id = m.articulo_id
			WHERE m.empresa_id = $1::uuid AND a.modo_control = 'CANTIDAD' AND m.sede_id IS NOT NULL
			GROUP BY a.id, a.empresa_id, m.sede_id
			HAVING SUM(` + sqlSignoMovimiento + `) > 0
		),
		-- El capital acumulado dentro de cada sede, de mayor a menor: es lo que define la clase.
		acumulado AS (
			SELECT e.articulo_id, e.sede_id, e.capital,
			       100.0 * SUM(e.capital) OVER (PARTITION BY e.sede_id ORDER BY e.capital DESC,
			                                    e.articulo_id ROWS UNBOUNDED PRECEDING)
			         / NULLIF(SUM(e.capital) OVER (PARTITION BY e.sede_id), 0) AS pct
			FROM existencia e
		),
		clasificado AS (
			SELECT articulo_id, sede_id, capital,
			       CASE WHEN pct <= $2 THEN 'A' WHEN pct <= $3 THEN 'B' ELSE 'C' END AS clase
			FROM acumulado
		)
		-- El último conteo va como SUBCONSULTA y no como JOIN.
		--
		-- Con un join, un artículo con 3 líneas de conteo (una por unidad) produce 3 filas, y tanto
		-- el conteo de artículos como la suma del capital quedan multiplicados por 3: el plan mostraba
		-- 7 artículos y 4,5 M donde había 4 artículos y 1,5 M. Verificado con datos reales.
		SELECT c.sede_id::text, s.nombre, c.clase,
		       count(*)::int, SUM(c.capital)::text,
		       COALESCE(to_char(max(c.ultimo), 'YYYY-MM-DD'), ''),
		       COALESCE((CURRENT_DATE - max(c.ultimo))::int, -1)
		FROM (
			SELECT cl2.*,
			       (SELECT max(ct.cerrado_en) FROM inv_conteo_linea l
			        JOIN inv_conteo ct ON ct.id = l.conteo_id
			        WHERE l.articulo_id = cl2.articulo_id AND ct.sede_id = cl2.sede_id
			          AND ct.estado = 'CERRADO' AND ct.empresa_id = $1::uuid) AS ultimo
			FROM clasificado cl2
		) c
		JOIN sede s ON s.id = c.sede_id
		GROUP BY c.sede_id, s.nombre, c.clase
		ORDER BY s.nombre, c.clase`

	rows, err := r.pool.Query(ctx, q, empresaID, corteA, corteB)
	if err != nil {
		return nil, fmt.Errorf("inventario: plan de conteo: %w", err)
	}
	defer rows.Close()
	out := []PlanConteo{}
	for rows.Next() {
		var p PlanConteo
		if err := rows.Scan(&p.SedeID, &p.Sede, &p.Clase, &p.Articulos, &p.CapitalCRC,
			&p.UltimoConteo, &p.DiasDesde); err != nil {
			return nil, fmt.Errorf("inventario: scan plan: %w", err)
		}
		p.ClaseTexto = EtiquetaClase(p.Clase)
		p.CadaCuantosDias = DiasEntreConteos(p.Clase)
		nunca := p.UltimoConteo == ""
		if nunca {
			p.DiasDesde = 0
		}
		p.Estado = estadoDelPlan(nunca, p.DiasDesde, p.CadaCuantosDias)
		out = append(out, p)
	}
	return out, rows.Err()
}
