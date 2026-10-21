package precios

// Acceso a datos del costeo.
//
// ── DE DÓNDE SALE UN COSTO ──────────────────────────────────────────────────────────────────
//
// No hay ninguna columna «costo_actual». El costo vigente a una fecha se resuelve con un LATERAL
// que toma de insumo_costo la fila de mayor vigente_desde que no supere esa fecha. Eso es lo que
// permite contestar «¿qué costaba Oro en enero?» sin archivos viejos, y lo que impide que exista
// un número mantenido a mano que se desincronice del historial.
//
// El contraste contra CxP sale por el mismo camino: la última factura del proveedor del insumo,
// en SUBTOTAL —no total—, porque el IVA soportado es crédito fiscal y no es costo.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio sobre PostgreSQL.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// baseInsumoSQL trae el catálogo con su costo vigente y su contraste contra CxP.
//
// $1 = empresa, $2 = fecha de corte para la vigencia del costo.
const baseInsumoSQL = `
SELECT i.id::text, i.codigo, i.nombre, i.unidad, i.tipo_costo, i.base_iva,
       i.proveedor_id::text, COALESCE(p.nombre, ''), i.cabys, COALESCE(i.notas, ''), i.activo,
       -- Las dos fechas van a texto en el SQL y no en Go: un date de Postgres no entra en un
       -- *string, y el scan falla recién cuando hay filas. Con el catálogo vacío el error no
       -- existe, así que el casteo tiene que estar puesto desde acá y no descubrirse en uso.
       c.costo, c.vigente_desde::text, COALESCE(c.moneda, 'CRC'),
       u.subtotal, u.fecha_emision::text
FROM insumo i
LEFT JOIN proveedor p ON p.id = i.proveedor_id AND p.empresa_id = i.empresa_id
-- El costo vigente: la captura más reciente que no supera la fecha de corte.
LEFT JOIN LATERAL (
    SELECT ic.costo, ic.vigente_desde, ic.moneda
    FROM insumo_costo ic
    WHERE ic.insumo_id = i.id AND ic.empresa_id = i.empresa_id
      AND ic.vigente_desde <= $2::date
    ORDER BY ic.vigente_desde DESC
    LIMIT 1
) c ON true
-- La última compra real al proveedor del insumo. SUBTOTAL, no total: el IVA es crédito fiscal.
LEFT JOIN LATERAL (
    SELECT d.subtotal, d.fecha_emision
    FROM documento_cxp d
    WHERE d.proveedor_id = i.proveedor_id AND d.empresa_id = i.empresa_id
    ORDER BY d.fecha_emision DESC
    LIMIT 1
) u ON i.proveedor_id IS NOT NULL
WHERE i.empresa_id = $1::uuid`

func scanInsumo(row pgx.Row) (Insumo, error) {
	var i Insumo
	var provID, cabys *string
	var costo, ultima *decimal.Decimal
	var vigente, compraEn *string
	err := row.Scan(&i.ID, &i.Codigo, &i.Nombre, &i.Unidad, &i.TipoCosto, &i.BaseIVA,
		&provID, &i.Proveedor, &cabys, &i.Notas, &i.Activo,
		&costo, &vigente, &i.Moneda, &ultima, &compraEn)
	if err != nil {
		return Insumo{}, err
	}
	i.ProveedorID, i.CABYS, i.VigenteDesde, i.UltimaCompraEn = provID, cabys, vigente, compraEn
	if costo != nil {
		s := costo.StringFixed(2)
		i.CostoVigente = &s
	}
	if ultima != nil {
		s := ultima.StringFixed(2)
		i.UltimaCompraCxP = &s
	}
	return i, nil
}

// Insumos lista el catálogo a una fecha, con filtros opcionales.
func (r *pgRepository) Insumos(ctx context.Context, empresaID, fecha, busca, tipo string, soloSinCosto bool) ([]Insumo, error) {
	q := baseInsumoSQL
	args := []any{empresaID, fecha}
	if s := strings.TrimSpace(busca); s != "" {
		args = append(args, "%"+strings.ToLower(s)+"%")
		q += fmt.Sprintf(" AND (lower(i.nombre) LIKE $%d OR lower(i.codigo) LIKE $%d)", len(args), len(args))
	}
	if t := strings.ToUpper(strings.TrimSpace(tipo)); t == TipoVariable || t == TipoFijo {
		args = append(args, t)
		q += fmt.Sprintf(" AND i.tipo_costo = $%d", len(args))
	}
	if soloSinCosto {
		q += " AND c.costo IS NULL"
	}
	q += " ORDER BY i.activo DESC, lower(i.nombre)"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("precios: listar insumos: %w", err)
	}
	defer rows.Close()

	out := []Insumo{}
	for rows.Next() {
		i, err := scanInsumo(rows)
		if err != nil {
			return nil, fmt.Errorf("precios: scan insumo: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// InsumoPorID trae un insumo con su costo vigente a la fecha.
func (r *pgRepository) InsumoPorID(ctx context.Context, empresaID, id, fecha string) (Insumo, error) {
	q := baseInsumoSQL + " AND i.id = $3::uuid"
	i, err := scanInsumo(r.pool.QueryRow(ctx, q, empresaID, fecha, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Insumo{}, ErrInsumoNoEncontrado
	}
	if err != nil {
		return Insumo{}, fmt.Errorf("precios: insumo por id: %w", err)
	}
	return i, nil
}

// CrearInsumo agrega una línea al catálogo.
func (r *pgRepository) CrearInsumo(ctx context.Context, empresaID string, in InsumoInput) (string, error) {
	const q = `INSERT INTO insumo (empresa_id, codigo, nombre, unidad, tipo_costo, base_iva,
	                               proveedor_id, cabys, notas)
	           VALUES ($1::uuid, $2, $3, $4, $5, $6, NULLIF($7,'')::uuid, $8, $9)
	           RETURNING id::text`
	var prov string
	if in.ProveedorID != nil {
		prov = *in.ProveedorID
	}
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, in.Codigo, in.Nombre, in.Unidad, in.TipoCosto,
		in.BaseIVA, prov, in.CABYS, in.Notas).Scan(&id)
	if esViolacionUnica(err) {
		return "", ErrCodigoDuplicado
	}
	if err != nil {
		return "", fmt.Errorf("precios: crear insumo: %w", err)
	}
	return id, nil
}

// ActualizarInsumo cambia los datos de una línea del catálogo.
func (r *pgRepository) ActualizarInsumo(ctx context.Context, empresaID, id string, in InsumoInput) error {
	const q = `UPDATE insumo SET codigo=$3, nombre=$4, unidad=$5, tipo_costo=$6, base_iva=$7,
	                  proveedor_id=NULLIF($8,'')::uuid, cabys=$9, notas=$10,
	                  activo=COALESCE($11, activo), actualizado_en=now()
	           WHERE empresa_id=$1::uuid AND id=$2::uuid`
	var prov string
	if in.ProveedorID != nil {
		prov = *in.ProveedorID
	}
	tag, err := r.pool.Exec(ctx, q, empresaID, id, in.Codigo, in.Nombre, in.Unidad, in.TipoCosto,
		in.BaseIVA, prov, in.CABYS, in.Notas, in.Activo)
	if esViolacionUnica(err) {
		return ErrCodigoDuplicado
	}
	if err != nil {
		return fmt.Errorf("precios: actualizar insumo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInsumoNoEncontrado
	}
	return nil
}

// BaseIVADelInsumo devuelve en qué base se capturan los costos de ese insumo. El servicio la
// necesita ANTES de guardar un costo, para convertir una sola vez.
func (r *pgRepository) BaseIVADelInsumo(ctx context.Context, empresaID, insumoID string) (string, error) {
	const q = `SELECT base_iva FROM insumo WHERE empresa_id=$1::uuid AND id=$2::uuid`
	var base string
	err := r.pool.QueryRow(ctx, q, empresaID, insumoID).Scan(&base)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInsumoNoEncontrado
	}
	if err != nil {
		return "", fmt.Errorf("precios: base de iva: %w", err)
	}
	return base, nil
}

// GuardarCosto escribe una captura de costo, YA en base sin IVA.
func (r *pgRepository) GuardarCosto(ctx context.Context, empresaID, insumoID string, costo decimal.Decimal, in CostoInput, usuarioID string) error {
	const q = `INSERT INTO insumo_costo (empresa_id, insumo_id, costo, moneda, vigente_desde,
	                                     origen, documento_cxp_id, nota, creado_por)
	           VALUES ($1::uuid, $2::uuid, $3, $4, $5::date, $6, NULLIF($7,'')::uuid, $8, NULLIF($9,'')::uuid)
	           ON CONFLICT (empresa_id, insumo_id, vigente_desde)
	           DO UPDATE SET costo = EXCLUDED.costo, moneda = EXCLUDED.moneda,
	                         origen = EXCLUDED.origen, nota = EXCLUDED.nota,
	                         creado_por = EXCLUDED.creado_por, creado_en = now()`
	var doc string
	if in.DocumentoCxP != nil {
		doc = *in.DocumentoCxP
	}
	_, err := r.pool.Exec(ctx, q, empresaID, insumoID, costo, in.Moneda, in.VigenteDesde,
		in.Origen, doc, in.Nota, usuarioID)
	if err != nil {
		return fmt.Errorf("precios: guardar costo: %w", err)
	}
	return nil
}

// HistorialDeCosto trae todas las capturas de un insumo, de la más nueva a la más vieja.
func (r *pgRepository) HistorialDeCosto(ctx context.Context, empresaID, insumoID string) ([]CapturaCosto, error) {
	const q = `SELECT ic.costo, ic.moneda, ic.vigente_desde::text, ic.origen,
	                  COALESCE(ic.nota,''), COALESCE(u.nombre,'')
	           FROM insumo_costo ic
	           LEFT JOIN usuario u ON u.id = ic.creado_por
	           WHERE ic.empresa_id=$1::uuid AND ic.insumo_id=$2::uuid
	           ORDER BY ic.vigente_desde DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, insumoID)
	if err != nil {
		return nil, fmt.Errorf("precios: historial de costo: %w", err)
	}
	defer rows.Close()

	out := []CapturaCosto{}
	for rows.Next() {
		var c CapturaCosto
		var monto decimal.Decimal
		if err := rows.Scan(&monto, &c.Moneda, &c.VigenteDesde, &c.Origen, &c.Nota, &c.CapturadoPor); err != nil {
			return nil, fmt.Errorf("precios: scan historial: %w", err)
		}
		c.Costo = monto.StringFixed(2)
		out = append(out, c)
	}
	return out, rows.Err()
}

// EliminarInsumo lo borra solo si no está en ninguna receta. Si está, se desactiva —nunca se
// borra algo que un costeo histórico necesita para explicarse.
func (r *pgRepository) EliminarInsumo(ctx context.Context, empresaID, id string) error {
	const enUso = `SELECT EXISTS (SELECT 1 FROM receta_linea WHERE empresa_id=$1::uuid AND insumo_id=$2::uuid)`
	var usado bool
	if err := r.pool.QueryRow(ctx, enUso, empresaID, id).Scan(&usado); err != nil {
		return fmt.Errorf("precios: ver uso del insumo: %w", err)
	}
	if usado {
		return ErrInsumoEnUso
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM insumo WHERE empresa_id=$1::uuid AND id=$2::uuid`, empresaID, id)
	if err != nil {
		return fmt.Errorf("precios: eliminar insumo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInsumoNoEncontrado
	}
	return nil
}
