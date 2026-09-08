package inventario

// Acceso a datos: catálogo y existencias.
//
// ── De dónde sale una existencia ─────────────────────────────────────────────
//
// Según el modo de control, la verdad vive en un lugar distinto, y eso es deliberado:
//
//   · **Modo UNIDAD** → las FICHAS. Una fila de `inv_unidad` ES un cofre; su `sede_id` y su
//     `estado` dicen dónde está y si cuenta como existencia. No hay contador que mantener: el
//     objeto y su registro son lo mismo.
//   · **Modo CANTIDAD** → la SUMA del libro de movimientos. No hay objeto que apuntar, así que la
//     existencia es la historia sumada.
//
// En los dos casos no existe ningún campo «cantidad_actual» que alguien tenga que recordar
// actualizar, que es lo que se desincroniza. Las escrituras que tocan ficha y libro a la vez van
// SIEMPRE en la misma transacción; si se separaran, ahí sí aparecerían dos verdades.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio sobre PostgreSQL.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// estadosQueCuentan son los estados de unidad que SÍ son existencia en su sede.
//
// En tránsito no cuenta a propósito: la unidad salió del origen y todavía nadie la recibió, así que
// no está en ninguna de las dos sedes. Usada, dañada, devuelta y la que el conteo no encontró
// salieron para siempre. Es una lista de lo que SÍ cuenta, no de lo que se descarta: así un estado
// nuevo queda fuera de la existencia hasta que alguien decida a mano que debe entrar.
const estadosQueCuentan = `('DISPONIBLE','RESERVADA','EXHIBICION')`

// ── Categorías ──────────────────────────────────────────────────────────────

func (r *pgRepository) ListarCategorias(ctx context.Context, empresaID string, incluirInactivas bool) ([]Categoria, error) {
	q := `
		SELECT c.id::text, COALESCE(c.padre_id::text, ''), COALESCE(p.nombre, ''), c.nombre, c.activo,
		       (SELECT count(*)::int FROM inv_articulo a
		        WHERE a.categoria_id = c.id AND a.empresa_id = c.empresa_id AND a.activo)
		FROM inv_categoria c
		LEFT JOIN inv_categoria p ON p.id = c.padre_id
		WHERE c.empresa_id = $1::uuid`
	if !incluirInactivas {
		q += ` AND c.activo`
	}
	q += ` ORDER BY COALESCE(p.nombre, c.nombre), (c.padre_id IS NOT NULL), c.orden, c.nombre`

	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("inventario: listar categorías: %w", err)
	}
	defer rows.Close()
	out := []Categoria{}
	for rows.Next() {
		var c Categoria
		if err := rows.Scan(&c.ID, &c.PadreID, &c.Padre, &c.Nombre, &c.Activo, &c.Articulos); err != nil {
			return nil, fmt.Errorf("inventario: scan categoría: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *pgRepository) CrearCategoria(ctx context.Context, empresaID, padreID, nombre string) (Categoria, error) {
	const q = `
		INSERT INTO inv_categoria (empresa_id, padre_id, nombre, orden)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3,
		        COALESCE((SELECT max(orden)+1 FROM inv_categoria
		                  WHERE empresa_id = $1::uuid AND padre_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid), 0))
		RETURNING id::text, nombre, activo`
	var c Categoria
	err := r.pool.QueryRow(ctx, q, empresaID, padreID, nombre).Scan(&c.ID, &c.Nombre, &c.Activo)
	if esViolacionUnica(err) {
		return Categoria{}, ErrDuplicado
	}
	if err != nil {
		return Categoria{}, fmt.Errorf("inventario: crear categoría: %w", err)
	}
	c.PadreID = padreID
	return c, nil
}

func (r *pgRepository) ActualizarCategoria(ctx context.Context, empresaID, id, nombre string, activo bool) error {
	const q = `UPDATE inv_categoria SET nombre = $3, activo = $4
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, nombre, activo)
	if esViolacionUnica(err) {
		return ErrDuplicado
	}
	if err != nil {
		return fmt.Errorf("inventario: actualizar categoría: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCategoriaNoEncontrada
	}
	return nil
}

// ── Artículos ───────────────────────────────────────────────────────────────

// selectArticulo es la proyección de un artículo con sus nombres resueltos, en un solo lugar para
// que la lista y el detalle no puedan devolver formas distintas.
const selectArticulo = `
	SELECT a.id::text, a.codigo, a.nombre, a.categoria_id::text,
	       CASE WHEN cp.nombre IS NULL THEN c.nombre ELSE cp.nombre || ' › ' || c.nombre END,
	       a.modo_control, a.unidad_medida,
	       COALESCE(a.proveedor_id::text, ''), COALESCE(p.nombre, ''),
	       COALESCE(a.clasificacion_id::text, ''), COALESCE(cl.nombre, ''),
	       a.activo, COALESCE(a.nota, '')
	FROM inv_articulo a
	JOIN inv_categoria c ON c.id = a.categoria_id
	LEFT JOIN inv_categoria cp ON cp.id = c.padre_id
	LEFT JOIN proveedor p ON p.id = a.proveedor_id
	LEFT JOIN clasificacion cl ON cl.id = a.clasificacion_id`

func escanearArticulo(rows pgx.Rows) (Articulo, error) {
	var a Articulo
	err := rows.Scan(&a.ID, &a.Codigo, &a.Nombre, &a.CategoriaID, &a.Categoria,
		&a.ModoControl, &a.UnidadMedida, &a.ProveedorID, &a.Proveedor,
		&a.ClasificacionID, &a.Clasificacion, &a.Activo, &a.Nota)
	return a, err
}

func (r *pgRepository) ListarArticulos(ctx context.Context, empresaID string, f FiltroArticulos) ([]Articulo, error) {
	q := selectArticulo + ` WHERE a.empresa_id = $1::uuid`
	args := []any{empresaID}
	if !f.IncluirInactivos {
		q += ` AND a.activo`
	}
	if f.CategoriaID != "" {
		args = append(args, f.CategoriaID)
		q += fmt.Sprintf(` AND (a.categoria_id = $%d::uuid OR c.padre_id = $%d::uuid)`, len(args), len(args))
	}
	if f.ModoControl != "" {
		args = append(args, f.ModoControl)
		q += fmt.Sprintf(` AND a.modo_control = $%d`, len(args))
	}
	if q2 := strings.TrimSpace(f.Q); q2 != "" {
		args = append(args, "%"+q2+"%")
		q += fmt.Sprintf(` AND (a.nombre ILIKE $%d OR a.codigo ILIKE $%d)`, len(args), len(args))
	}
	q += ` ORDER BY 5, a.nombre`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: listar artículos: %w", err)
	}
	defer rows.Close()
	out := []Articulo{}
	for rows.Next() {
		a, err := escanearArticulo(rows)
		if err != nil {
			return nil, fmt.Errorf("inventario: scan artículo: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *pgRepository) ArticuloPorID(ctx context.Context, empresaID, id string) (Articulo, error) {
	rows, err := r.pool.Query(ctx, selectArticulo+` WHERE a.empresa_id = $1::uuid AND a.id = $2::uuid`, empresaID, id)
	if err != nil {
		return Articulo{}, fmt.Errorf("inventario: artículo por id: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return Articulo{}, ErrArticuloNoEncontrado
	}
	a, err := escanearArticulo(rows)
	if err != nil {
		return Articulo{}, fmt.Errorf("inventario: scan artículo: %w", err)
	}
	return a, nil
}

func (r *pgRepository) CrearArticulo(ctx context.Context, empresaID string, a ArticuloNuevo, usuarioID string) (string, error) {
	const q = `
		INSERT INTO inv_articulo (empresa_id, categoria_id, codigo, nombre, modo_control,
		                          unidad_medida, proveedor_id, clasificacion_id, activo, nota, creado_por)
		SELECT $1::uuid, $2::uuid, $3, $4, $5, COALESCE(NULLIF($6,''), 'unidad'),
		       NULLIF($7,'')::uuid, NULLIF($8,'')::uuid, true, NULLIF($9,''), NULLIF($10,'')::uuid
		-- La categoría tiene que ser de esta empresa: si no, el artículo quedaría colgado de una
		-- categoría ajena y el filtro por categoría lo dejaría invisible.
		WHERE EXISTS (SELECT 1 FROM inv_categoria WHERE id = $2::uuid AND empresa_id = $1::uuid)
		RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, a.CategoriaID, a.Codigo, a.Nombre, a.ModoControl,
		a.UnidadMedida, a.ProveedorID, a.ClasificacionID, a.Nota, usuarioID).Scan(&id)
	if esViolacionUnica(err) {
		return "", ErrDuplicado
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCategoriaNoEncontrada
	}
	if err != nil {
		return "", fmt.Errorf("inventario: crear artículo: %w", err)
	}
	return id, nil
}

func (r *pgRepository) ActualizarArticulo(ctx context.Context, empresaID, id string, a ArticuloNuevo) error {
	const q = `
		UPDATE inv_articulo SET codigo = $3, nombre = $4, categoria_id = $5::uuid,
		       unidad_medida = COALESCE(NULLIF($6,''), 'unidad'),
		       proveedor_id = NULLIF($7,'')::uuid, clasificacion_id = NULLIF($8,'')::uuid,
		       activo = $9, nota = NULLIF($10,''), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		  AND EXISTS (SELECT 1 FROM inv_categoria WHERE id = $5::uuid AND empresa_id = $1::uuid)`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, a.Codigo, a.Nombre, a.CategoriaID,
		a.UnidadMedida, a.ProveedorID, a.ClasificacionID, a.Activo, a.Nota)
	if esViolacionUnica(err) {
		return ErrDuplicado
	}
	if err != nil {
		return fmt.Errorf("inventario: actualizar artículo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrArticuloNoEncontrado
	}
	return nil
}

// ── Niveles (mínimo y máximo por sede) ──────────────────────────────────────

func (r *pgRepository) FijarNivel(ctx context.Context, empresaID, articuloID, sedeID string, minimo, maximo int) error {
	const q = `
		INSERT INTO inv_nivel (empresa_id, articulo_id, sede_id, minimo, maximo)
		SELECT $1::uuid, $2::uuid, $3::uuid, $4, $5
		WHERE EXISTS (SELECT 1 FROM inv_articulo WHERE id = $2::uuid AND empresa_id = $1::uuid)
		  AND EXISTS (SELECT 1 FROM sede WHERE id = $3::uuid AND empresa_id = $1::uuid)
		ON CONFLICT (empresa_id, articulo_id, sede_id)
		DO UPDATE SET minimo = EXCLUDED.minimo, maximo = EXCLUDED.maximo, actualizado_en = now()`
	tag, err := r.pool.Exec(ctx, q, empresaID, articuloID, sedeID, minimo, maximo)
	if err != nil {
		return fmt.Errorf("inventario: fijar nivel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrArticuloNoEncontrado
	}
	return nil
}

func (r *pgRepository) NivelesDeArticulo(ctx context.Context, empresaID, articuloID string) ([]NivelSede, error) {
	// Se listan TODAS las sedes, no solo las que tienen nivel: la pantalla necesita poder fijar el
	// mínimo de una sede que todavía no lo tiene.
	const q = `
		SELECT s.id::text, s.nombre, COALESCE(n.minimo, 0), COALESCE(n.maximo, 0)
		FROM sede s
		LEFT JOIN inv_nivel n ON n.sede_id = s.id AND n.articulo_id = $2::uuid AND n.empresa_id = $1::uuid
		WHERE s.empresa_id = $1::uuid AND s.activo
		ORDER BY s.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, articuloID)
	if err != nil {
		return nil, fmt.Errorf("inventario: niveles del artículo: %w", err)
	}
	defer rows.Close()
	out := []NivelSede{}
	for rows.Next() {
		var n NivelSede
		if err := rows.Scan(&n.SedeID, &n.Sede, &n.Minimo, &n.Maximo); err != nil {
			return nil, fmt.Errorf("inventario: scan nivel: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ── Existencias ─────────────────────────────────────────────────────────────

// sqlCostoPromedio es el costo unitario de un artículo controlado por cantidad: promedio ponderado
// de sus entradas. Se calcula por ARTÍCULO y no por sede porque una sede puede haber recibido todo
// por traslado y no tener ninguna entrada propia; el costo del objeto no cambia por mudarse.
const sqlCostoPromedio = `
	COALESCE((SELECT ` + sqlPromedioPonderado + `
	          FROM inv_movimiento e
	          WHERE e.empresa_id = a.empresa_id AND e.articulo_id = a.id AND e.tipo = 'ENTRADA'), 0)`

// De quién es el capital. Una unidad consignada está en la bodega y se puede vender, pero el dinero
// es del proveedor hasta que se usa: sumarla al capital propio dice que la empresa tiene invertido
// algo que no invirtió.
//
// Las dos expresiones viven acá y las usan las tres consultas que valorizan inventario (existencias,
// rotación y el capital que define la clase ABC del conteo). Antes el consignado se estimaba en Go
// multiplicando el PROMEDIO de la fila por la cantidad de consignadas: con tres cofres de ₡100.000,
// ₡200.000 y ₡900.000 donde solo el último era del proveedor, la pantalla reportaba ₡400.000 en vez
// de ₡900.000. El FILTER lo calcula exacto y en el mismo lugar donde se lee el costo.
const (
	sqlCapitalPropio     = `COALESCE(SUM(u.costo_crc) FILTER (WHERE NOT u.es_consignada), 0)`
	sqlCapitalConsignado = `COALESCE(SUM(u.costo_crc) FILTER (WHERE u.es_consignada), 0)`
)

func (r *pgRepository) Existencias(ctx context.Context, empresaID string, f FiltroExistencias) ([]ExistenciaSede, error) {
	// Dos ramas porque la existencia se lee de dos lugares distintos según el modo (ver la cabecera
	// del archivo). Se unen acá para que la pantalla reciba una sola lista comparable.
	q := `
		WITH por_unidad AS (
			SELECT a.id AS articulo_id, u.sede_id, count(*)::int AS cantidad,
			       COALESCE(SUM(u.costo_crc), 0) AS valor,
			       CASE WHEN count(*) > 0 THEN COALESCE(SUM(u.costo_crc), 0) / count(*) ELSE 0 END AS costo_unit,
			       count(*) FILTER (WHERE u.es_consignada)::int AS consignadas,
			       ` + sqlCapitalConsignado + ` AS valor_consignado
			FROM inv_unidad u
			JOIN inv_articulo a ON a.id = u.articulo_id
			WHERE u.empresa_id = $1::uuid AND u.sede_id IS NOT NULL
			  AND u.estado IN ` + estadosQueCuentan + `
			GROUP BY a.id, u.sede_id
		),
		por_cantidad AS (
			SELECT a.id AS articulo_id, m.sede_id,
			       SUM(` + sqlSignoMovimiento + `)::int AS cantidad,
			       0::numeric AS valor, ` + sqlCostoPromedio + ` AS costo_unit,
			       0 AS consignadas, 0::numeric AS valor_consignado
			FROM inv_movimiento m
			JOIN inv_articulo a ON a.id = m.articulo_id
			WHERE m.empresa_id = $1::uuid AND a.modo_control = 'CANTIDAD' AND m.sede_id IS NOT NULL
			GROUP BY a.id, a.empresa_id, m.sede_id
			HAVING SUM(` + sqlSignoMovimiento + `) <> 0
		),
		todo AS (
			SELECT articulo_id, sede_id, cantidad, valor, costo_unit, consignadas, valor_consignado FROM por_unidad
			UNION ALL
			SELECT articulo_id, sede_id, cantidad, cantidad * costo_unit, costo_unit, consignadas, valor_consignado FROM por_cantidad
		)
		SELECT t.articulo_id::text, a.codigo, a.nombre,
		       CASE WHEN cp.nombre IS NULL THEN c.nombre ELSE cp.nombre || ' › ' || c.nombre END,
		       a.modo_control, t.sede_id::text, s.nombre,
		       t.cantidad, COALESCE(n.minimo, 0), COALESCE(n.maximo, 0),
		       t.valor::text, t.costo_unit::text, t.consignadas,
		       t.valor_consignado::text
		FROM todo t
		JOIN inv_articulo a ON a.id = t.articulo_id
		JOIN inv_categoria c ON c.id = a.categoria_id
		LEFT JOIN inv_categoria cp ON cp.id = c.padre_id
		JOIN sede s ON s.id = t.sede_id
		LEFT JOIN inv_nivel n ON n.articulo_id = t.articulo_id AND n.sede_id = t.sede_id AND n.empresa_id = $1::uuid
		WHERE a.empresa_id = $1::uuid`

	args := []any{empresaID}
	if f.SedeID != "" {
		args = append(args, f.SedeID)
		q += fmt.Sprintf(` AND t.sede_id = $%d::uuid`, len(args))
	}
	if f.CategoriaID != "" {
		args = append(args, f.CategoriaID)
		q += fmt.Sprintf(` AND (a.categoria_id = $%d::uuid OR c.padre_id = $%d::uuid)`, len(args), len(args))
	}
	if f.ModoControl != "" {
		args = append(args, f.ModoControl)
		q += fmt.Sprintf(` AND a.modo_control = $%d`, len(args))
	}
	if t := strings.TrimSpace(f.Q); t != "" {
		args = append(args, "%"+t+"%")
		q += fmt.Sprintf(` AND (a.nombre ILIKE $%d OR a.codigo ILIKE $%d)`, len(args), len(args))
	}
	// El filtro es a nivel de FILA y no de unidad a propósito: la pregunta que contesta es «en qué
	// artículos y sedes tengo mercadería del proveedor», no «cuáles unidades». Para lo segundo está
	// el filtro de ListarUnidades.
	if f.SoloConsignadas {
		q += ` AND t.consignadas > 0`
	}
	q += ` ORDER BY 4, a.nombre, s.nombre`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: existencias: %w", err)
	}
	defer rows.Close()
	out := []ExistenciaSede{}
	for rows.Next() {
		var e ExistenciaSede
		if err := rows.Scan(&e.ArticuloID, &e.Codigo, &e.Articulo, &e.Categoria, &e.ModoControl,
			&e.SedeID, &e.Sede, &e.Cantidad, &e.Minimo, &e.Maximo,
			&e.ValorCRC, &e.CostoUnitarioCRC, &e.Consignadas,
			&e.ValorConsignadoCRC); err != nil {
			return nil, fmt.Errorf("inventario: scan existencia: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ExistenciaDe es cuánto hay de un artículo en una sede. Se usa antes de sacar, para no dejar el
// stock en negativo.
func (r *pgRepository) ExistenciaDe(ctx context.Context, empresaID, articuloID, sedeID string) (int, error) {
	return existenciaDe(ctx, r.pool, empresaID, articuloID, sedeID)
}

// consultor es lo mínimo que necesitan los helpers para poder correr tanto sobre el pool como
// dentro de una transacción. Sin esto habría dos copias de la misma consulta y podrían discrepar.
type consultor interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func existenciaDe(ctx context.Context, q consultor, empresaID, articuloID, sedeID string) (int, error) {
	const sql = `
		SELECT CASE
		  WHEN a.modo_control = 'UNIDAD' THEN
		    (SELECT count(*)::int FROM inv_unidad u
		     WHERE u.empresa_id = $1::uuid AND u.articulo_id = $2::uuid
		       AND u.sede_id = $3::uuid AND u.estado IN ` + estadosQueCuentan + `)
		  ELSE
		    COALESCE((SELECT SUM(` + sqlSignoMovimiento + `)::int FROM inv_movimiento m
		              WHERE m.empresa_id = $1::uuid AND m.articulo_id = $2::uuid
		                AND m.sede_id = $3::uuid), 0)
		END
		FROM inv_articulo a
		WHERE a.id = $2::uuid AND a.empresa_id = $1::uuid`
	var n int
	err := q.QueryRow(ctx, sql, empresaID, articuloID, sedeID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrArticuloNoEncontrado
	}
	if err != nil {
		return 0, fmt.Errorf("inventario: existencia de artículo: %w", err)
	}
	return n, nil
}

// ── Unidades ────────────────────────────────────────────────────────────────

func (r *pgRepository) ListarUnidades(ctx context.Context, empresaID string, f FiltroUnidades) ([]Unidad, error) {
	q := `
		SELECT u.id::text, u.numero, a.id::text, a.nombre,
		       CASE WHEN cp.nombre IS NULL THEN c.nombre ELSE cp.nombre || ' › ' || c.nombre END,
		       COALESCE(u.sede_id::text, ''), COALESCE(s.nombre, ''),
		       COALESCE(u.sede_destino_id::text, ''), COALESCE(sd.nombre, ''),
		       u.estado, u.costo_crc::text, u.es_consignada,
		       COALESCE(u.proveedor_id::text, ''), COALESCE(p.nombre, ''),
		       to_char(u.ingresada_en, 'YYYY-MM-DD'),
		       COALESCE((SELECT sv.numero FROM inv_movimiento m
		                 JOIN inv_servicio sv ON sv.id = m.servicio_id
		                 WHERE m.unidad_id = u.id AND m.tipo = 'SALIDA'
		                 ORDER BY m.fecha DESC LIMIT 1), ''),
		       -- Días desde su último movimiento: es lo que delata el capital detenido.
		       COALESCE((SELECT (CURRENT_DATE - max(m.fecha))::int FROM inv_movimiento m
		                 WHERE m.unidad_id = u.id), 0)
		FROM inv_unidad u
		JOIN inv_articulo a ON a.id = u.articulo_id
		JOIN inv_categoria c ON c.id = a.categoria_id
		LEFT JOIN inv_categoria cp ON cp.id = c.padre_id
		LEFT JOIN sede s ON s.id = u.sede_id
		LEFT JOIN sede sd ON sd.id = u.sede_destino_id
		LEFT JOIN proveedor p ON p.id = u.proveedor_id
		WHERE u.empresa_id = $1::uuid`
	args := []any{empresaID}
	if f.ArticuloID != "" {
		args = append(args, f.ArticuloID)
		q += fmt.Sprintf(` AND u.articulo_id = $%d::uuid`, len(args))
	}
	if f.SedeID != "" {
		args = append(args, f.SedeID)
		q += fmt.Sprintf(` AND u.sede_id = $%d::uuid`, len(args))
	}
	if f.Estado != "" {
		args = append(args, f.Estado)
		q += fmt.Sprintf(` AND u.estado = $%d`, len(args))
	}
	if t := strings.TrimSpace(f.Q); t != "" {
		args = append(args, "%"+t+"%")
		q += fmt.Sprintf(` AND (u.numero ILIKE $%d OR a.nombre ILIKE $%d)`, len(args), len(args))
	}
	if f.QuietasDesdeDias > 0 {
		args = append(args, f.QuietasDesdeDias)
		q += fmt.Sprintf(` AND u.estado IN `+estadosQueCuentan+`
		  AND COALESCE((SELECT (CURRENT_DATE - max(m.fecha))::int FROM inv_movimiento m
		                WHERE m.unidad_id = u.id), 0) >= $%d`, len(args))
	}
	if f.Consignada != nil {
		args = append(args, *f.Consignada)
		q += fmt.Sprintf(` AND u.es_consignada = $%d`, len(args))
	}
	args = append(args, f.Limite)
	q += fmt.Sprintf(` ORDER BY a.nombre, u.numero LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: listar unidades: %w", err)
	}
	defer rows.Close()
	out := []Unidad{}
	for rows.Next() {
		var u Unidad
		if err := rows.Scan(&u.ID, &u.Numero, &u.ArticuloID, &u.Articulo, &u.Categoria,
			&u.SedeID, &u.Sede, &u.SedeDestinoID, &u.SedeDestino, &u.Estado, &u.CostoCRC,
			&u.EsConsignada, &u.ProveedorID, &u.Proveedor, &u.IngresadaEn,
			&u.ServicioNumero, &u.DiasQuieta); err != nil {
			return nil, fmt.Errorf("inventario: scan unidad: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *pgRepository) UnidadPorNumero(ctx context.Context, empresaID, numero string) (Unidad, error) {
	us, err := r.ListarUnidades(ctx, empresaID, FiltroUnidades{Q: numero, Limite: 5})
	if err != nil {
		return Unidad{}, err
	}
	for _, u := range us {
		if strings.EqualFold(u.Numero, numero) {
			return u, nil
		}
	}
	return Unidad{}, ErrUnidadNoEncontrada
}

// ── Libro de movimientos ────────────────────────────────────────────────────

func (r *pgRepository) ListarMovimientos(ctx context.Context, empresaID string, f FiltroMovimientos) ([]Movimiento, error) {
	q := `
		SELECT m.id::text, to_char(m.fecha, 'YYYY-MM-DD'), m.tipo,
		       a.id::text, a.nombre, COALESCE(m.unidad_id::text, ''), COALESCE(u.numero, ''),
		       m.cantidad, COALESCE(s.nombre, ''), COALESCE(sc.nombre, ''),
		       m.costo_unitario_crc::text, COALESCE(sv.numero, ''), COALESCE(p.nombre, ''),
		       COALESCE(m.motivo, ''), COALESCE(us.nombre, '')
		FROM inv_movimiento m
		JOIN inv_articulo a ON a.id = m.articulo_id
		LEFT JOIN inv_unidad u ON u.id = m.unidad_id
		LEFT JOIN sede s ON s.id = m.sede_id
		LEFT JOIN sede sc ON sc.id = m.sede_contra_id
		LEFT JOIN inv_servicio sv ON sv.id = m.servicio_id
		LEFT JOIN proveedor p ON p.id = m.proveedor_id
		LEFT JOIN usuario us ON us.id = m.creado_por
		WHERE m.empresa_id = $1::uuid`
	args := []any{empresaID}
	if f.ArticuloID != "" {
		args = append(args, f.ArticuloID)
		q += fmt.Sprintf(` AND m.articulo_id = $%d::uuid`, len(args))
	}
	if f.UnidadID != "" {
		args = append(args, f.UnidadID)
		q += fmt.Sprintf(` AND m.unidad_id = $%d::uuid`, len(args))
	}
	if f.SedeID != "" {
		args = append(args, f.SedeID)
		q += fmt.Sprintf(` AND (m.sede_id = $%d::uuid OR m.sede_contra_id = $%d::uuid)`, len(args), len(args))
	}
	if f.Tipo != "" {
		args = append(args, f.Tipo)
		q += fmt.Sprintf(` AND m.tipo = $%d`, len(args))
	}
	if f.Desde != "" {
		args = append(args, f.Desde)
		q += fmt.Sprintf(` AND m.fecha >= $%d::date`, len(args))
	}
	if f.Hasta != "" {
		args = append(args, f.Hasta)
		q += fmt.Sprintf(` AND m.fecha <= $%d::date`, len(args))
	}
	args = append(args, f.Limite)
	q += fmt.Sprintf(` ORDER BY m.fecha DESC, m.creado_en DESC LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: listar movimientos: %w", err)
	}
	defer rows.Close()
	out := []Movimiento{}
	for rows.Next() {
		var m Movimiento
		if err := rows.Scan(&m.ID, &m.Fecha, &m.Tipo, &m.ArticuloID, &m.Articulo,
			&m.UnidadID, &m.UnidadNum, &m.Cantidad, &m.Sede, &m.SedeContra,
			&m.CostoUnitarioCRC, &m.Servicio, &m.Proveedor, &m.Motivo, &m.Usuario); err != nil {
			return nil, fmt.Errorf("inventario: scan movimiento: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ── Contexto para los avisos ────────────────────────────────────────────────

func (r *pgRepository) ContarSedes(ctx context.Context, empresaID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*)::int FROM sede WHERE empresa_id = $1::uuid AND activo`, empresaID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("inventario: contar sedes: %w", err)
	}
	return n, nil
}

func (r *pgRepository) ContarServicios(ctx context.Context, empresaID, desde, hasta string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*)::int FROM inv_servicio
		 WHERE empresa_id = $1::uuid AND fecha BETWEEN $2::date AND $3::date`,
		empresaID, desde, hasta).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("inventario: contar servicios: %w", err)
	}
	return n, nil
}
