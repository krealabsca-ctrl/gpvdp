package precios

// Escritura masiva del catálogo importado.
//
// ── TODO EN UNA TRANSACCIÓN ─────────────────────────────────────────────────────────────────
//
// Son cientos de filas. Si a la mitad falla una, dejar escritas las primeras sería peor que no
// haber importado: el usuario vería un catálogo a medias sin saber dónde se cortó, y volver a
// correrlo sobre ese estado es justo donde se arman los duplicados.
//
// ── ES IDEMPOTENTE POR CÓDIGO ───────────────────────────────────────────────────────────────
//
// El código se deriva del nombre y es estable, así que correr la importación dos veces ACTUALIZA
// las mismas filas en vez de duplicarlas. Eso importa porque la primera corrida casi nunca es la
// buena: se prueba, se mira el resultado y se vuelve a correr con otra hoja o con las sedes
// unificadas de otra forma.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// ResultadoImportacion es lo que pasó, para mostrarlo.
type ResultadoImportacion struct {
	InsumosCreados      int `json:"insumos_creados"`
	InsumosActualizados int `json:"insumos_actualizados"`
	CostosCapturados    int `json:"costos_capturados"`
	SinCosto            int `json:"sin_costo"`
}

// ImportarCatalogo escribe el plan completo en una transacción.
func (r *pgRepository) ImportarCatalogo(ctx context.Context, empresaID string, plan PlanDeImportacion, vigenteDesde, usuarioID string) (ResultadoImportacion, error) {
	var res ResultadoImportacion

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("precios: abrir transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// `xmax = 0` distingue una fila recién insertada de una actualizada por el ON CONFLICT. Sin
	// eso no se puede decir «creé 90 y actualicé 3», que es la diferencia entre una primera
	// corrida y una repetida.
	const upsertInsumo = `
		INSERT INTO insumo (empresa_id, codigo, nombre, unidad, tipo_costo, base_iva, notas)
		VALUES ($1::uuid, $2, $3, 'UNIDAD', 'VARIABLE', 'SIN_IVA', $4)
		ON CONFLICT (empresa_id, lower(codigo))
		DO UPDATE SET nombre = EXCLUDED.nombre, actualizado_en = now()
		RETURNING id::text, (xmax = 0) AS creado`

	const upsertCosto = `
		INSERT INTO insumo_costo (empresa_id, insumo_id, costo, moneda, vigente_desde, origen, nota, creado_por)
		VALUES ($1::uuid, $2::uuid, $3, 'CRC', $4::date, 'IMPORTADO', $5, NULLIF($6,'')::uuid)
		ON CONFLICT (empresa_id, insumo_id, vigente_desde)
		DO UPDATE SET costo = EXCLUDED.costo, origen = EXCLUDED.origen,
		              nota = EXCLUDED.nota, creado_por = EXCLUDED.creado_por, creado_en = now()`

	for _, ins := range plan.Insumos {
		nota := "importado del archivo de costeo"
		if ins.Sede != "" {
			nota = "importado del archivo de costeo · sede " + ins.Sede
		}

		var id string
		var creado bool
		if err := tx.QueryRow(ctx, upsertInsumo, empresaID, ins.Codigo, ins.Nombre, nota).Scan(&id, &creado); err != nil {
			return res, fmt.Errorf("precios: importar insumo %q: %w", ins.Codigo, err)
		}
		if creado {
			res.InsumosCreados++
		} else {
			res.InsumosActualizados++
		}

		if ins.Costo == nil {
			res.SinCosto++
			continue
		}
		monto, err := decimal.NewFromString(*ins.Costo)
		if err != nil {
			return res, fmt.Errorf("precios: costo ilegible en %q: %w", ins.Codigo, err)
		}
		if _, err := tx.Exec(ctx, upsertCosto, empresaID, id, monto, vigenteDesde, nota, usuarioID); err != nil {
			return res, fmt.Errorf("precios: importar costo de %q: %w", ins.Codigo, err)
		}
		res.CostosCapturados++
	}

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("precios: confirmar importación: %w", err)
	}
	return res, nil
}
