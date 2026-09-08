package inventario

// Traslados entre sedes, reposición y rotación.
//
// El traslado es la operación donde se pierde el control con 15 plazas, y por eso tiene dos pasos:
// lo que sale queda EN TRÁNSITO —fuera de las dos sedes— hasta que alguien lo recibe. Una unidad
// extraviada en el camino queda visible con su responsable en vez de desaparecer del sistema.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func (r *pgRepository) CrearTraslado(ctx context.Context, empresaID string, t TrasladoNuevo, usuarioID string) (Traslado, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Traslado{}, fmt.Errorf("inventario: begin traslado: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	origen, err := nombreSede(ctx, tx, empresaID, t.SedeOrigenID)
	if err != nil {
		return Traslado{}, err
	}
	destino, err := nombreSede(ctx, tx, empresaID, t.SedeDestinoID)
	if err != nil {
		return Traslado{}, err
	}
	n, err := siguienteConsecutivo(ctx, tx, empresaID, "TRASLADO")
	if err != nil {
		return Traslado{}, err
	}
	numero := fmt.Sprintf("TR-%s-%04d", t.Fecha[:4], n)

	var trasladoID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO inv_traslado (empresa_id, numero, sede_origen_id, sede_destino_id, estado,
		                          enviado_en, enviado_por, nota)
		VALUES ($1::uuid, $2, $3::uuid, $4::uuid, 'EN_TRANSITO', $5::date, NULLIF($6,'')::uuid, NULLIF($7,''))
		RETURNING id::text`,
		empresaID, numero, t.SedeOrigenID, t.SedeDestinoID, t.Fecha, usuarioID, t.Nota).Scan(&trasladoID); err != nil {
		return Traslado{}, fmt.Errorf("inventario: crear traslado: %w", err)
	}

	out := Traslado{
		ID: trasladoID, Numero: numero,
		SedeOrigenID: t.SedeOrigenID, SedeOrigen: origen,
		SedeDestinoID: t.SedeDestinoID, SedeDestino: destino,
		Estado: TrasladoEnTransito, EnviadoEn: t.Fecha, Nota: t.Nota,
		Lineas: []TrasladoLinea{},
	}

	for _, l := range t.Lineas {
		modo, nombre, _, err := datosArticulo(ctx, tx, empresaID, l.ArticuloID)
		if err != nil {
			return Traslado{}, err
		}
		linea := TrasladoLinea{ArticuloID: l.ArticuloID, Articulo: nombre}

		if modo == ModoUnidad {
			if l.UnidadNumero == "" {
				return Traslado{}, fmt.Errorf("%w: «%s» se controla por unidad, hace falta decir cuál se traslada", ErrCantidadInvalida, nombre)
			}
			var unidadID, estado, costo string
			var sedeUnidad, sedeUnidadNombre *string
			err := tx.QueryRow(ctx, `
				SELECT u.id::text, u.estado, u.costo_crc::text, u.sede_id::text, s.nombre
				FROM inv_unidad u LEFT JOIN sede s ON s.id = u.sede_id
				WHERE u.empresa_id = $1::uuid AND u.numero = $2 FOR UPDATE OF u`,
				empresaID, l.UnidadNumero).Scan(&unidadID, &estado, &costo, &sedeUnidad, &sedeUnidadNombre)
			if errors.Is(err, pgx.ErrNoRows) {
				return Traslado{}, fmt.Errorf("%w: %s", ErrUnidadNoEncontrada, l.UnidadNumero)
			}
			if err != nil {
				return Traslado{}, fmt.Errorf("inventario: buscar unidad: %w", err)
			}
			if estado != EstadoDisponible && estado != EstadoExhibicion {
				return Traslado{}, &UnidadNoDisponibleError{Numero: l.UnidadNumero, Estado: estado, Quiere: "trasladarla"}
			}
			if sedeUnidad == nil || *sedeUnidad != t.SedeOrigenID {
				return Traslado{}, &UnidadEnOtraSedeError{
					Numero: l.UnidadNumero, Esta: deref(sedeUnidadNombre), Pedida: origen,
				}
			}
			// Sale del origen: `sede_id` en NULL es lo que la deja fuera de las existencias de las
			// DOS sedes mientras viaja, y `sede_destino_id` dice a dónde tenía que llegar.
			if _, err := tx.Exec(ctx, `
				UPDATE inv_unidad SET estado = 'EN_TRANSITO', sede_id = NULL,
				       sede_destino_id = $2::uuid, actualizado_en = now()
				WHERE id = $1::uuid`, unidadID, t.SedeDestinoID); err != nil {
				return Traslado{}, fmt.Errorf("inventario: poner unidad en tránsito: %w", err)
			}
			if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
				EmpresaID: empresaID, ArticuloID: l.ArticuloID, UnidadID: unidadID,
				Tipo: MovTrasladoSalida, Cantidad: 1, SedeID: t.SedeOrigenID,
				SedeContraID: t.SedeDestinoID, Costo: costo, Fecha: t.Fecha,
				TrasladoID: trasladoID, UsuarioID: usuarioID,
			}); err != nil {
				return Traslado{}, err
			}
			linea.UnidadID = unidadID
			linea.UnidadNum = l.UnidadNumero
			linea.Cantidad = 1
		} else {
			hay, err := existenciaDe(ctx, tx, empresaID, l.ArticuloID, t.SedeOrigenID)
			if err != nil {
				return Traslado{}, err
			}
			if hay < l.Cantidad {
				return Traslado{}, &SinExistenciaError{Articulo: nombre, Sede: origen, Hay: hay, Pedido: l.Cantidad}
			}
			costo, err := costoPromedio(ctx, tx, empresaID, l.ArticuloID)
			if err != nil {
				return Traslado{}, err
			}
			if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
				EmpresaID: empresaID, ArticuloID: l.ArticuloID, Tipo: MovTrasladoSalida,
				Cantidad: l.Cantidad, SedeID: t.SedeOrigenID, SedeContraID: t.SedeDestinoID,
				Costo: costo, Fecha: t.Fecha, TrasladoID: trasladoID, UsuarioID: usuarioID,
			}); err != nil {
				return Traslado{}, err
			}
			linea.Cantidad = l.Cantidad
		}
		out.Lineas = append(out.Lineas, linea)
	}

	if err := tx.Commit(ctx); err != nil {
		return Traslado{}, fmt.Errorf("inventario: commit traslado: %w", err)
	}
	return out, nil
}

// RecibirTraslado confirma la llegada: recién acá lo trasladado vuelve a existir, en la sede destino.
func (r *pgRepository) RecibirTraslado(ctx context.Context, empresaID, id, fecha, usuarioID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inventario: begin recibir: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var estado, destinoID, origenID string
	err = tx.QueryRow(ctx, `
		SELECT estado, sede_destino_id::text, sede_origen_id::text FROM inv_traslado
		WHERE empresa_id = $1::uuid AND id = $2::uuid FOR UPDATE`,
		empresaID, id).Scan(&estado, &destinoID, &origenID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTrasladoNoEncontrado
	}
	if err != nil {
		return fmt.Errorf("inventario: buscar traslado: %w", err)
	}
	if estado == TrasladoRecibido {
		return ErrTrasladoYaRecibido
	}

	// Por cada línea que salió, su entrada en el destino. Se recorren los movimientos de salida
	// del traslado: son la lista de lo que efectivamente viajó.
	rows, err := tx.Query(ctx, `
		SELECT articulo_id::text, COALESCE(unidad_id::text, ''), cantidad, costo_unitario_crc::text
		FROM inv_movimiento
		WHERE empresa_id = $1::uuid AND traslado_id = $2::uuid AND tipo = 'TRASLADO_SALIDA'`,
		empresaID, id)
	if err != nil {
		return fmt.Errorf("inventario: líneas del traslado: %w", err)
	}
	type linea struct {
		articulo, unidad, costo string
		cantidad                int
	}
	var lineas []linea
	for rows.Next() {
		var l linea
		if err := rows.Scan(&l.articulo, &l.unidad, &l.cantidad, &l.costo); err != nil {
			rows.Close()
			return fmt.Errorf("inventario: scan línea de traslado: %w", err)
		}
		lineas = append(lineas, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inventario: líneas del traslado: %w", err)
	}

	for _, l := range lineas {
		if l.unidad != "" {
			if _, err := tx.Exec(ctx, `
				UPDATE inv_unidad SET estado = 'DISPONIBLE', sede_id = $2::uuid,
				       sede_destino_id = NULL, actualizado_en = now()
				WHERE id = $1::uuid AND empresa_id = $3::uuid`,
				l.unidad, destinoID, empresaID); err != nil {
				return fmt.Errorf("inventario: recibir unidad: %w", err)
			}
		}
		if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
			EmpresaID: empresaID, ArticuloID: l.articulo, UnidadID: l.unidad,
			Tipo: MovTrasladoEntrada, Cantidad: l.cantidad, SedeID: destinoID,
			SedeContraID: origenID, Costo: l.costo, Fecha: fecha,
			TrasladoID: id, UsuarioID: usuarioID,
		}); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE inv_traslado SET estado = 'RECIBIDO', recibido_en = $3::date,
		       recibido_por = NULLIF($4,'')::uuid
		WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, id, fecha, usuarioID); err != nil {
		return fmt.Errorf("inventario: marcar traslado recibido: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("inventario: commit recibir: %w", err)
	}
	return nil
}

func (r *pgRepository) ListarTraslados(ctx context.Context, empresaID, estado string) ([]Traslado, error) {
	q := `
		SELECT t.id::text, t.numero, t.sede_origen_id::text, so.nombre,
		       t.sede_destino_id::text, sd.nombre, t.estado,
		       to_char(t.enviado_en, 'YYYY-MM-DD'), COALESCE(to_char(t.recibido_en, 'YYYY-MM-DD'), ''),
		       COALESCE(ue.nombre, ''), COALESCE(ur.nombre, ''), COALESCE(t.nota, ''),
		       -- Días en camino: para los recibidos, cuánto tardó; para los que van, cuánto llevan.
		       CASE WHEN t.estado = 'EN_TRANSITO' THEN (CURRENT_DATE - t.enviado_en)::int
		            ELSE COALESCE((t.recibido_en - t.enviado_en)::int, 0) END
		FROM inv_traslado t
		JOIN sede so ON so.id = t.sede_origen_id
		JOIN sede sd ON sd.id = t.sede_destino_id
		LEFT JOIN usuario ue ON ue.id = t.enviado_por
		LEFT JOIN usuario ur ON ur.id = t.recibido_por
		WHERE t.empresa_id = $1::uuid`
	args := []any{empresaID}
	if estado != "" {
		args = append(args, estado)
		q += fmt.Sprintf(` AND t.estado = $%d`, len(args))
	}
	q += ` ORDER BY (t.estado = 'EN_TRANSITO') DESC, t.enviado_en DESC LIMIT 200`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: listar traslados: %w", err)
	}
	defer rows.Close()
	out := []Traslado{}
	ids := []string{}
	for rows.Next() {
		var t Traslado
		if err := rows.Scan(&t.ID, &t.Numero, &t.SedeOrigenID, &t.SedeOrigen,
			&t.SedeDestinoID, &t.SedeDestino, &t.Estado, &t.EnviadoEn, &t.RecibidoEn,
			&t.EnviadoPor, &t.RecibidoPor, &t.Nota, &t.DiasEnCamino); err != nil {
			return nil, fmt.Errorf("inventario: scan traslado: %w", err)
		}
		t.Lineas = []TrasladoLinea{}
		out = append(out, t)
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	// Las líneas se traen de una sola vez y se reparten en memoria: una consulta por traslado
	// serían 200 idas a la base para pintar una tabla.
	lrows, err := r.pool.Query(ctx, `
		SELECT m.traslado_id::text, a.id::text, a.nombre,
		       COALESCE(m.unidad_id::text, ''), COALESCE(u.numero, ''), m.cantidad
		FROM inv_movimiento m
		JOIN inv_articulo a ON a.id = m.articulo_id
		LEFT JOIN inv_unidad u ON u.id = m.unidad_id
		WHERE m.empresa_id = $1::uuid AND m.tipo = 'TRASLADO_SALIDA'
		  AND m.traslado_id = ANY($2::uuid[])
		ORDER BY a.nombre`, empresaID, ids)
	if err != nil {
		return nil, fmt.Errorf("inventario: líneas de traslados: %w", err)
	}
	defer lrows.Close()
	porTraslado := map[string][]TrasladoLinea{}
	for lrows.Next() {
		var id string
		var l TrasladoLinea
		if err := lrows.Scan(&id, &l.ArticuloID, &l.Articulo, &l.UnidadID, &l.UnidadNum, &l.Cantidad); err != nil {
			return nil, fmt.Errorf("inventario: scan línea: %w", err)
		}
		porTraslado[id] = append(porTraslado[id], l)
	}
	if err := lrows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if ls, ok := porTraslado[out[i].ID]; ok {
			out[i].Lineas = ls
		}
	}
	return out, nil
}

// ── Reposición ──────────────────────────────────────────────────────────────

// Reposicion lista lo que está en o por debajo del mínimo, con el consumo medido de las últimas
// semanas. La CANTIDAD sugerida y su explicación las arma el servicio: acá solo se miden hechos.
func (r *pgRepository) Reposicion(ctx context.Context, empresaID, sedeID string, semanas int) ([]SugerenciaPedido, error) {
	q := `
		WITH existencia AS (
			SELECT a.id AS articulo_id, u.sede_id, count(*)::int AS hay
			FROM inv_unidad u JOIN inv_articulo a ON a.id = u.articulo_id
			WHERE u.empresa_id = $1::uuid AND u.sede_id IS NOT NULL
			  AND u.estado IN ` + estadosQueCuentan + `
			GROUP BY a.id, u.sede_id
			UNION ALL
			SELECT a.id, m.sede_id, SUM(` + sqlSignoMovimiento + `)::int
			FROM inv_movimiento m JOIN inv_articulo a ON a.id = m.articulo_id
			WHERE m.empresa_id = $1::uuid AND a.modo_control = 'CANTIDAD' AND m.sede_id IS NOT NULL
			GROUP BY a.id, m.sede_id
		),
		salidas AS (
			SELECT m.articulo_id, m.sede_id, SUM(m.cantidad)::numeric AS unidades
			FROM inv_movimiento m
			WHERE m.empresa_id = $1::uuid AND m.tipo = 'SALIDA'
			  AND m.fecha >= CURRENT_DATE - ($3::int * 7)
			GROUP BY m.articulo_id, m.sede_id
		)
		SELECT a.id::text, a.codigo, a.nombre, n.sede_id::text, s.nombre,
		       COALESCE(a.proveedor_id::text, ''), COALESCE(p.nombre, '(sin proveedor asignado)'),
		       COALESCE(e.hay, 0), n.minimo, n.maximo,
		       (COALESCE(sa.unidades, 0) / $3::numeric)::text,
		       ` + sqlCostoPromedio + `::text
		FROM inv_nivel n
		JOIN inv_articulo a ON a.id = n.articulo_id
		JOIN sede s ON s.id = n.sede_id
		LEFT JOIN proveedor p ON p.id = a.proveedor_id
		LEFT JOIN existencia e ON e.articulo_id = n.articulo_id AND e.sede_id = n.sede_id
		LEFT JOIN salidas sa ON sa.articulo_id = n.articulo_id AND sa.sede_id = n.sede_id
		WHERE n.empresa_id = $1::uuid AND a.activo AND n.minimo > 0
		  -- Solo lo que hace falta pedir: en o por debajo del mínimo.
		  AND COALESCE(e.hay, 0) <= n.minimo
		  -- El filtro de sede va SIEMPRE en el SQL, aunque venga vacío: si el parámetro se pasa pero
		  -- no aparece en la consulta, PostgreSQL no puede inferir su tipo y la consulta entera
		  -- revienta con «could not determine data type of parameter $2».
		  AND ($2 = '' OR n.sede_id = NULLIF($2, '')::uuid)`
	args := []any{empresaID, sedeID, semanas}
	q += ` ORDER BY COALESCE(p.nombre, 'zzz'), a.nombre, s.nombre`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: reposición: %w", err)
	}
	defer rows.Close()
	out := []SugerenciaPedido{}
	for rows.Next() {
		var g SugerenciaPedido
		var costo string
		if err := rows.Scan(&g.ArticuloID, &g.Codigo, &g.Articulo, &g.SedeID, &g.Sede,
			&g.ProveedorID, &g.Proveedor, &g.Hay, &g.Minimo, &g.Maximo,
			&g.ConsumoSemanal, &costo); err != nil {
			return nil, fmt.Errorf("inventario: scan sugerencia: %w", err)
		}
		// Acá NO se calcula la cantidad: eso es una regla de negocio y la decide el servicio, que es
		// el único que sabe si hay historia suficiente para confiar en el consumo. El repositorio
		// solo mide: existencia, consumo y costo.
		g.CostoUnitarioCRC = costo
		out = append(out, g)
	}
	return out, rows.Err()
}

// ── Rotación ────────────────────────────────────────────────────────────────

func (r *pgRepository) Rotacion(ctx context.Context, empresaID, desde, hasta string) ([]RotacionArticulo, error) {
	const q = `
		WITH existencia AS (
			-- El capital que esta pantalla mide es el PROPIO: se llama «capital detenido» y lo
			-- consignado no está detenido, es del proveedor. Si sumara lo consignado, un artículo
			-- que la empresa no pagó encabezaría la lista de plata inmovilizada.
			SELECT a.id AS articulo_id, count(*)::int AS hay, ` + sqlCapitalPropio + ` AS capital,
			       ` + sqlCapitalConsignado + ` AS capital_consignado
			FROM inv_unidad u JOIN inv_articulo a ON a.id = u.articulo_id
			WHERE u.empresa_id = $1::uuid AND u.sede_id IS NOT NULL
			  AND u.estado IN ` + estadosQueCuentan + `
			GROUP BY a.id
			UNION ALL
			SELECT a.id, SUM(` + sqlSignoMovimiento + `)::int,
			       SUM(` + sqlSignoMovimiento + `) * ` + sqlCostoPromedio + `, 0::numeric
			FROM inv_movimiento m JOIN inv_articulo a ON a.id = m.articulo_id
			WHERE m.empresa_id = $1::uuid AND a.modo_control = 'CANTIDAD'
			GROUP BY a.id, a.empresa_id
		),
		salidas AS (
			SELECT m.articulo_id, SUM(m.cantidad)::int AS unidades
			FROM inv_movimiento m
			WHERE m.empresa_id = $1::uuid AND m.tipo = 'SALIDA'
			  AND m.fecha BETWEEN $2::date AND $3::date
			GROUP BY m.articulo_id
		)
		SELECT a.id::text, a.codigo, a.nombre,
		       CASE WHEN cp.nombre IS NULL THEN c.nombre ELSE cp.nombre || ' › ' || c.nombre END,
		       COALESCE(e.hay, 0), COALESCE(sa.unidades, 0), COALESCE(e.capital, 0)::text,
		       COALESCE(e.capital_consignado, 0)::text
		FROM inv_articulo a
		JOIN inv_categoria c ON c.id = a.categoria_id
		LEFT JOIN inv_categoria cp ON cp.id = c.padre_id
		LEFT JOIN (SELECT articulo_id, SUM(hay)::int AS hay, SUM(capital) AS capital,
		                  SUM(capital_consignado) AS capital_consignado
		           FROM existencia GROUP BY articulo_id) e ON e.articulo_id = a.id
		LEFT JOIN salidas sa ON sa.articulo_id = a.id
		WHERE a.empresa_id = $1::uuid AND a.activo
		  AND (COALESCE(e.hay, 0) > 0 OR COALESCE(sa.unidades, 0) > 0)
		ORDER BY COALESCE(e.capital, 0) DESC`

	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("inventario: rotación: %w", err)
	}
	defer rows.Close()

	dias := diasEntre(desde, hasta)
	out := []RotacionArticulo{}
	for rows.Next() {
		var g RotacionArticulo
		if err := rows.Scan(&g.ArticuloID, &g.Codigo, &g.Articulo, &g.Categoria,
			&g.EnStock, &g.Salidas, &g.CapitalCRC, &g.CapitalConsignadoCRC); err != nil {
			return nil, fmt.Errorf("inventario: scan rotación: %w", err)
		}
		out = append(out, conLecturaDeRotacion(g, dias))
	}
	return out, rows.Err()
}

// conLecturaDeRotacion calcula la rotación anualizada y la traduce a una lectura.
//
// La lectura importa más que el número: «0,3×» no le dice nada a nadie, «tres años de stock» sí.
func conLecturaDeRotacion(g RotacionArticulo, dias int) RotacionArticulo {
	if g.Salidas == 0 {
		g.Lectura = RotacionSinSalidas
		return g
	}
	if g.EnStock == 0 {
		// Salió todo: no hay existencia contra la que medir, y eso no es un problema.
		g.Lectura = RotacionSana
		return g
	}
	salidas := decimal.NewFromInt(int64(g.Salidas))
	stock := decimal.NewFromInt(int64(g.EnStock))
	// Rotación anualizada, para que rangos de distinto largo sean comparables.
	factor := decimal.NewFromInt(365).Div(decimal.NewFromInt(int64(maxInt(dias, 1))))
	rot := salidas.Div(stock).Mul(factor)
	g.Rotacion = rot.StringFixed(1)
	// Días que alcanza lo que hay al ritmo del período.
	porDia := salidas.Div(decimal.NewFromInt(int64(maxInt(dias, 1))))
	if porDia.IsPositive() {
		g.DiasDeStock = stock.Div(porDia).StringFixed(0)
	}
	switch {
	case rot.GreaterThanOrEqual(decimal.NewFromInt(3)):
		g.Lectura = RotacionSana
	case rot.GreaterThanOrEqual(decimal.NewFromFloat(1)):
		g.Lectura = RotacionLenta
	default:
		g.Lectura = RotacionDetenida
	}
	return g
}

// diasEntre cuenta los días de un rango AAAA-MM-DD sin traer una librería de fechas.
func diasEntre(desde, hasta string) int {
	d, err1 := parseDia(desde)
	h, err2 := parseDia(hasta)
	if err1 != nil || err2 != nil {
		return 1
	}
	n := int(h.Sub(d).Hours() / 24)
	if n < 1 {
		return 1
	}
	return n
}

// ── Aritmética de montos ────────────────────────────────────────────────────
//
// Con decimal y no con float64: el costo de un cofre en colones no admite errores de redondeo.

// sumarDecimal quedó sin usar cuando el costo total del servicio pasó a releerse del libro con una
// sola expresión SQL, en vez de acumularse acá. Se borra en vez de dejarlo: un helper de dinero sin
// llamadores es una invitación a que el próximo total se vuelva a sumar por su cuenta.

func multiplicarDecimal(a string, n int) string {
	x, err := decimal.NewFromString(a)
	if err != nil {
		x = decimal.Zero
	}
	return x.Mul(decimal.NewFromInt(int64(n))).StringFixed(2)
}
