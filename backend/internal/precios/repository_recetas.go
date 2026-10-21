package precios

// Escritura de planes, insumos y recetas importados.
//
// Todo en una transacción, por lo mismo que la importación del catálogo: si falla a la mitad,
// quedarían planes sin receta y recetas sin sus insumos, que es más difícil de arreglar que no
// haber importado nada.
//
// La receta de un plan se REEMPLAZA, no se acumula: reimportar la misma hoja tiene que dejar el
// plan como está en el archivo. Acumular haría que la segunda corrida duplicara cada línea y que
// el costo del plan saliera al doble sin que nada lo delate.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// ResultadoRecetas es lo que pasó.
type ResultadoRecetas struct {
	PlanesCreados      int `json:"planes_creados"`
	PlanesActualizados int `json:"planes_actualizados"`
	InsumosCreados     int `json:"insumos_creados"`
	InsumosReusados    int `json:"insumos_reusados"`
	LineasEscritas     int `json:"lineas_escritas"`
	CostosCapturados   int `json:"costos_capturados"`
}

// ImportarRecetas escribe planes, insumos y líneas de receta.
func (r *pgRepository) ImportarRecetas(ctx context.Context, empresaID string, plan PlanDeRecetas, vigenteDesde, usuarioID string) (ResultadoRecetas, error) {
	var res ResultadoRecetas

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("precios: abrir transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// ── 1 · Los insumos ──
	const upsertInsumo = `
		INSERT INTO insumo (empresa_id, codigo, nombre, unidad, tipo_costo, base_iva, notas)
		VALUES ($1::uuid, $2, $3, 'UNIDAD', 'VARIABLE', 'SIN_IVA', 'importado de la matriz de costeo')
		ON CONFLICT (empresa_id, lower(codigo))
		DO UPDATE SET actualizado_en = now()
		RETURNING id::text, (xmax = 0) AS creado`

	const upsertCosto = `
		INSERT INTO insumo_costo (empresa_id, insumo_id, costo, moneda, vigente_desde, origen, nota, creado_por)
		VALUES ($1::uuid, $2::uuid, $3, 'CRC', $4::date, 'IMPORTADO', 'matriz de costeo', NULLIF($5,'')::uuid)
		ON CONFLICT (empresa_id, insumo_id, vigente_desde)
		DO UPDATE SET costo = EXCLUDED.costo, creado_en = now()`

	idDeInsumo := map[string]string{}
	for _, ins := range plan.Insumos {
		var id string
		var creado bool
		if err := tx.QueryRow(ctx, upsertInsumo, empresaID, ins.Codigo, ins.Nombre).Scan(&id, &creado); err != nil {
			return res, fmt.Errorf("precios: insumo %q: %w", ins.Codigo, err)
		}
		idDeInsumo[ins.Codigo] = id
		if creado {
			res.InsumosCreados++
		} else {
			res.InsumosReusados++
		}
		if ins.Costo == nil {
			continue
		}
		monto, err := decimal.NewFromString(*ins.Costo)
		if err != nil {
			return res, fmt.Errorf("precios: costo ilegible en %q: %w", ins.Codigo, err)
		}
		if _, err := tx.Exec(ctx, upsertCosto, empresaID, id, monto, vigenteDesde, usuarioID); err != nil {
			return res, fmt.Errorf("precios: costo de %q: %w", ins.Codigo, err)
		}
		res.CostosCapturados++
	}

	// ── 2 · Los planes ──
	//
	// El precio entra en `precio_venta` o en `cuota_mensual` según la modalidad: son dos cosas
	// distintas y meterlas en la misma columna haría que el costeo compare una cuota mensual
	// contra el costo de un servicio completo.
	const upsertPlan = `
		INSERT INTO plan_precio (empresa_id, codigo, nombre, familia, modalidad,
		                         precio_venta, cuota_mensual, moneda, notas)
		VALUES ($1::uuid, $2, $3, $4, $5,
		        CASE WHEN $5 = 'DIRECTO' THEN NULLIF($6,'')::numeric END,
		        CASE WHEN $5 = 'SUSCRIPCION' THEN NULLIF($6,'')::numeric END,
		        'CRC', 'importado de la matriz de costeo')
		ON CONFLICT (empresa_id, lower(codigo))
		DO UPDATE SET nombre = EXCLUDED.nombre, familia = EXCLUDED.familia,
		              modalidad = EXCLUDED.modalidad,
		              precio_venta = EXCLUDED.precio_venta,
		              cuota_mensual = EXCLUDED.cuota_mensual,
		              actualizado_en = now()
		RETURNING id::text, (xmax = 0) AS creado`

	idDePlan := map[string]string{}
	for _, p := range plan.Planes {
		var precio string
		if p.Precio != nil {
			precio = *p.Precio
		}
		var id string
		var creado bool
		if err := tx.QueryRow(ctx, upsertPlan, empresaID, p.Codigo, p.Nombre, p.Familia,
			p.Modalidad, precio).Scan(&id, &creado); err != nil {
			return res, fmt.Errorf("precios: plan %q: %w", p.Codigo, err)
		}
		idDePlan[p.Codigo] = id
		if creado {
			res.PlanesCreados++
		} else {
			res.PlanesActualizados++
		}
		// La receta se reemplaza: ver la cabecera.
		if _, err := tx.Exec(ctx,
			`DELETE FROM receta_linea WHERE empresa_id=$1::uuid AND plan_id=$2::uuid`,
			empresaID, id); err != nil {
			return res, fmt.Errorf("precios: limpiar receta de %q: %w", p.Codigo, err)
		}
	}

	// ── 3 · Las líneas ──
	const insertLinea = `
		INSERT INTO receta_linea (empresa_id, plan_id, insumo_id, cantidad, ordinal, nota)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, 'matriz de costeo')
		ON CONFLICT (empresa_id, plan_id, insumo_id)
		DO UPDATE SET cantidad = EXCLUDED.cantidad`

	ordinal := map[string]int{}
	for _, l := range plan.Lineas {
		planID, ok := idDePlan[l.PlanCodigo]
		if !ok {
			continue
		}
		insumoID, ok := idDeInsumo[l.InsumoCodigo]
		if !ok {
			continue
		}
		cant, err := decimal.NewFromString(l.Cantidad)
		if err != nil || cant.LessThanOrEqual(decimal.Zero) {
			continue
		}
		if _, err := tx.Exec(ctx, insertLinea, empresaID, planID, insumoID, cant, ordinal[l.PlanCodigo]); err != nil {
			return res, fmt.Errorf("precios: línea %s/%s: %w", l.PlanCodigo, l.InsumoCodigo, err)
		}
		ordinal[l.PlanCodigo]++
		res.LineasEscritas++
	}

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("precios: confirmar importación de recetas: %w", err)
	}
	return res, nil
}
