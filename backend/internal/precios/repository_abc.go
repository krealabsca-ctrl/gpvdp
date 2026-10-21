package precios

// Acceso a datos del costeo por actividad.
//
// El gasto NO se guarda acá: se lee de movimiento_bancario a través de la clasificación. Por eso
// el indirecto del costeo y el gasto del libro son el mismo número por construcción, y no dos
// cifras que alguien tiene que mantener iguales.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Drivers lista los generadores de costo con su uso y la base del período.
func (r *pgRepository) Drivers(ctx context.Context, empresaID, periodo string) ([]Driver, error) {
	const q = `
	SELECT d.id::text, d.codigo, d.nombre, d.unidad, COALESCE(d.descripcion,''), d.activo,
	       (SELECT count(*) FROM gasto_driver g WHERE g.driver_id = d.id),
	       (SELECT count(*) FROM plan_driver p WHERE p.driver_id = d.id),
	       b.cantidad
	FROM driver_costo d
	LEFT JOIN LATERAL (
	    SELECT db.cantidad FROM driver_base db
	    WHERE db.driver_id = d.id AND db.empresa_id = d.empresa_id
	      AND ($2 = '' OR db.periodo = $2::date)
	    ORDER BY db.periodo DESC LIMIT 1
	) b ON true
	WHERE d.empresa_id = $1::uuid
	ORDER BY d.activo DESC, lower(d.nombre)`

	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("precios: listar drivers: %w", err)
	}
	defer rows.Close()

	out := []Driver{}
	for rows.Next() {
		var d Driver
		var base *decimal.Decimal
		if err := rows.Scan(&d.ID, &d.Codigo, &d.Nombre, &d.Unidad, &d.Descripcion, &d.Activo,
			&d.Gastos, &d.Planes, &base); err != nil {
			return nil, fmt.Errorf("precios: scan driver: %w", err)
		}
		if base != nil {
			s := base.String()
			d.BaseDelPeriodo = &s
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CrearDriver agrega un generador de costo.
func (r *pgRepository) CrearDriver(ctx context.Context, empresaID string, in DriverInput) (string, error) {
	const q = `INSERT INTO driver_costo (empresa_id, codigo, nombre, unidad, descripcion)
	           VALUES ($1::uuid, $2, $3, $4, $5) RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, in.Codigo, in.Nombre, in.Unidad, in.Descripcion).Scan(&id)
	if esViolacionUnica(err) {
		return "", ErrCodigoDuplicado
	}
	if err != nil {
		return "", fmt.Errorf("precios: crear driver: %w", err)
	}
	return id, nil
}

// EliminarDriver lo borra solo si nada cuelga de él.
func (r *pgRepository) EliminarDriver(ctx context.Context, empresaID, id string) error {
	const enUso = `SELECT EXISTS (SELECT 1 FROM gasto_driver WHERE empresa_id=$1::uuid AND driver_id=$2::uuid)
	               OR EXISTS (SELECT 1 FROM plan_driver WHERE empresa_id=$1::uuid AND driver_id=$2::uuid)`
	var usado bool
	if err := r.pool.QueryRow(ctx, enUso, empresaID, id).Scan(&usado); err != nil {
		return fmt.Errorf("precios: ver uso del driver: %w", err)
	}
	if usado {
		return ErrDriverEnUso
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM driver_costo WHERE empresa_id=$1::uuid AND id=$2::uuid`, empresaID, id)
	if err != nil {
		return fmt.Errorf("precios: eliminar driver: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDriverNoEncontrado
	}
	return nil
}

// FijarBase guarda la medición de un driver para un período.
func (r *pgRepository) FijarBase(ctx context.Context, empresaID, driverID, periodo string, cantidad decimal.Decimal, nota, usuarioID string) error {
	const q = `INSERT INTO driver_base (empresa_id, driver_id, periodo, cantidad, origen, nota, creado_por)
	           VALUES ($1::uuid, $2::uuid, $3::date, $4, 'MANUAL', $5, NULLIF($6,'')::uuid)
	           ON CONFLICT (empresa_id, driver_id, periodo)
	           DO UPDATE SET cantidad = EXCLUDED.cantidad, nota = EXCLUDED.nota,
	                         creado_por = EXCLUDED.creado_por, creado_en = now()`
	_, err := r.pool.Exec(ctx, q, empresaID, driverID, periodo, cantidad, nota, usuarioID)
	if err != nil {
		return fmt.Errorf("precios: fijar base: %w", err)
	}
	return nil
}

// Repartos trae las clasificaciones de gasto con su reparto y el monto real del período.
//
// Incluye las clasificaciones SIN reparto que igual tuvieron gasto: son las que explican el
// «gasto sin asignar», y esconderlas haría que el total absorbido pareciera completo.
func (r *pgRepository) Repartos(ctx context.Context, empresaID, desde, hasta string) ([]RepartoDeGasto, error) {
	const q = `
	WITH gasto AS (
	    SELECT m.clasificacion_id, sum(m.monto_crc) AS monto
	    FROM movimiento_bancario m
	    WHERE m.empresa_id = $1::uuid AND m.incluido AND m.debito > 0
	      AND m.fecha BETWEEN $2::date AND $3::date
	      AND m.clasificacion_id IS NOT NULL
	    GROUP BY 1
	)
	SELECT cl.id::text, cl.nombre, co.nombre, COALESCE(g.monto, 0),
	       COALESCE(gd.id::text,''), COALESCE(gd.driver_id::text,''), COALESCE(d.nombre,''),
	       COALESCE(gd.porcentaje, 0)
	FROM clasificacion cl
	JOIN concepto co ON co.id = cl.concepto_id
	LEFT JOIN gasto g ON g.clasificacion_id = cl.id
	LEFT JOIN gasto_driver gd ON gd.clasificacion_id = cl.id AND gd.empresa_id = cl.empresa_id
	LEFT JOIN driver_costo d ON d.id = gd.driver_id
	WHERE cl.empresa_id = $1::uuid
	  AND co.naturaleza = 'GASTO'
	  AND (g.monto IS NOT NULL OR gd.id IS NOT NULL)
	ORDER BY COALESCE(g.monto,0) DESC, cl.nombre`

	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("precios: repartos: %w", err)
	}
	defer rows.Close()

	porClasif := map[string]*RepartoDeGasto{}
	orden := []string{}
	for rows.Next() {
		var clasifID, clasif, concepto, lineaID, driverID, driver string
		var monto, pct decimal.Decimal
		if err := rows.Scan(&clasifID, &clasif, &concepto, &monto, &lineaID, &driverID, &driver, &pct); err != nil {
			return nil, fmt.Errorf("precios: scan reparto: %w", err)
		}
		g, ok := porClasif[clasifID]
		if !ok {
			g = &RepartoDeGasto{
				ClasificacionID: clasifID, Clasificacion: clasif, Concepto: concepto,
				GastoDelPeriodo: monto.StringFixed(2), Lineas: []LineaDeReparto{},
			}
			porClasif[clasifID] = g
			orden = append(orden, clasifID)
		}
		if lineaID == "" {
			continue
		}
		g.Lineas = append(g.Lineas, LineaDeReparto{
			ID: lineaID, DriverID: driverID, Driver: driver,
			Porcentaje: pct.String(),
			Monto:      monto.Mul(pct).Div(cien).Round(2).StringFixed(2),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]RepartoDeGasto, 0, len(orden))
	for _, id := range orden {
		g := porClasif[id]
		suma, valido, motivo := ValidarReparto(g.Lineas)
		g.SumaPct, g.Valido, g.Motivo = suma.StringFixed(2), valido, motivo
		out = append(out, *g)
	}
	return out, nil
}

// FijarReparto asigna un porcentaje de una clasificación a un driver.
func (r *pgRepository) FijarReparto(ctx context.Context, empresaID, clasificacionID, driverID string, pct decimal.Decimal, vigenteDesde, usuarioID string) error {
	const q = `INSERT INTO gasto_driver (empresa_id, clasificacion_id, driver_id, porcentaje, vigente_desde, creado_por)
	           VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::date, NULLIF($6,'')::uuid)
	           ON CONFLICT (empresa_id, clasificacion_id, driver_id, vigente_desde)
	           DO UPDATE SET porcentaje = EXCLUDED.porcentaje, creado_por = EXCLUDED.creado_por`
	_, err := r.pool.Exec(ctx, q, empresaID, clasificacionID, driverID, pct, vigenteDesde, usuarioID)
	if err != nil {
		return fmt.Errorf("precios: fijar reparto: %w", err)
	}
	return nil
}

// QuitarReparto borra una línea de reparto.
func (r *pgRepository) QuitarReparto(ctx context.Context, empresaID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM gasto_driver WHERE empresa_id=$1::uuid AND id=$2::uuid`, empresaID, id)
	if err != nil {
		return fmt.Errorf("precios: quitar reparto: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDriverNoEncontrado
	}
	return nil
}

// ConsumosDelPlan trae cuánto consume un plan de cada driver.
func (r *pgRepository) ConsumosDelPlan(ctx context.Context, empresaID, planID string) ([]IndirectoDelPlan, error) {
	const q = `SELECT d.id::text, d.nombre, d.unidad, pd.consumo
	           FROM plan_driver pd
	           JOIN driver_costo d ON d.id = pd.driver_id
	           WHERE pd.empresa_id = $1::uuid AND pd.plan_id = $2::uuid
	           ORDER BY lower(d.nombre)`
	rows, err := r.pool.Query(ctx, q, empresaID, planID)
	if err != nil {
		return nil, fmt.Errorf("precios: consumos del plan: %w", err)
	}
	defer rows.Close()

	out := []IndirectoDelPlan{}
	for rows.Next() {
		var c IndirectoDelPlan
		var consumo decimal.Decimal
		if err := rows.Scan(&c.DriverID, &c.Driver, &c.Unidad, &consumo); err != nil {
			return nil, fmt.Errorf("precios: scan consumo: %w", err)
		}
		c.Consumo = consumo.String()
		out = append(out, c)
	}
	return out, rows.Err()
}

// FijarConsumo dice cuántas unidades de un driver consume un plan.
func (r *pgRepository) FijarConsumo(ctx context.Context, empresaID, planID, driverID string, consumo decimal.Decimal, nota string) error {
	const q = `INSERT INTO plan_driver (empresa_id, plan_id, driver_id, consumo, nota)
	           VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)
	           ON CONFLICT (empresa_id, plan_id, driver_id)
	           DO UPDATE SET consumo = EXCLUDED.consumo, nota = EXCLUDED.nota`
	_, err := r.pool.Exec(ctx, q, empresaID, planID, driverID, consumo, nota)
	if err != nil {
		return fmt.Errorf("precios: fijar consumo: %w", err)
	}
	return nil
}

// QuitarConsumo saca un driver de un plan.
func (r *pgRepository) QuitarConsumo(ctx context.Context, empresaID, planID, driverID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM plan_driver WHERE empresa_id=$1::uuid AND plan_id=$2::uuid AND driver_id=$3::uuid`,
		empresaID, planID, driverID)
	if err != nil {
		return fmt.Errorf("precios: quitar consumo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDriverNoEncontrado
	}
	return nil
}

// BaseDelDriver devuelve la medición de un driver para un período.
func (r *pgRepository) BaseDelDriver(ctx context.Context, empresaID, driverID, periodo string) (*decimal.Decimal, error) {
	const q = `SELECT cantidad FROM driver_base
	           WHERE empresa_id=$1::uuid AND driver_id=$2::uuid AND periodo <= $3::date
	           ORDER BY periodo DESC LIMIT 1`
	var c decimal.Decimal
	err := r.pool.QueryRow(ctx, q, empresaID, driverID, periodo).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("precios: base del driver: %w", err)
	}
	return &c, nil
}
