package cxc

// Rehacer los cargos generados, sin tocar un solo peso cobrado.
//
// POR QUÉ EXISTE. Los cargos se calculan a partir de la cartera. Cuando la cartera se corrige
// —el 13-oct-2026, al empezar a guardar la fecha de próximo pago— los cargos viejos quedan mal:
// 18.450 contratos tenían saldo de más por ₡1.042.683.725. Rehacerlos a mano sobre una base en
// producción es la clase de operación que sale mal una vez y no se puede deshacer.
//
// EL GUARDARRAÍL, QUE ES TODA LA FUNCIÓN. Solo se borra un cargo que NO tiene nada colgando:
//
//   - `origen = 'GENERADO'`: un cargo cargado a mano o venido de otro lado no lo creó esta
//     función y no le toca borrarlo.
//   - sin una sola fila en `cobro_aplicacion`: ahí está la prueba de que alguien pagó contra él.
//   - sin nota de crédito aplicada ni emitida contra él.
//   - sin clave de Hacienda: si se facturó, el documento existe afuera.
//
// LO DE `cobro_aplicacion` NO ES REDUNDANTE CON `monto_aplicado`. La FK de esa tabla está en
// CASCADE: borrar el cargo borraría las aplicaciones del cobro EN SILENCIO, y el pago quedaría
// registrado contra nada. `monto_aplicado` es un acumulado que podría desincronizarse; la
// existencia de la fila es la verdad. Se miran los dos.

import (
	"context"
	"fmt"
)

// seguroDeBorrar son las condiciones que debe cumplir un cargo para poder rehacerse. Vive en una
// constante y no repetida en dos consultas porque el conteo que se le muestra al usuario y el
// DELETE que se ejecuta TIENEN que mirar exactamente lo mismo: si se separan, el plan promete un
// número y la operación hace otro.
const seguroDeBorrar = `
	c.empresa_id = $1::uuid
	AND c.origen = 'GENERADO'
	AND c.monto_aplicado = 0
	AND c.clave_hacienda IS NULL
	AND NOT EXISTS (SELECT 1 FROM cobro_aplicacion ca WHERE ca.cargo_id = c.id)
	AND NOT EXISTS (SELECT 1 FROM nota_credito_cxc nc WHERE nc.cargo_id = c.id)
	AND NOT EXISTS (SELECT 1 FROM nota_credito_aplicacion na WHERE na.cargo_id = c.id)`

// PlanDeRegeneracion cuenta qué se puede rehacer y qué no, ANTES de tocar nada.
func (r *pgRepository) PlanDeRegeneracion(ctx context.Context, empresaID string) (RegeneracionPlan, error) {
	const q = `
		SELECT
			count(*) FILTER (WHERE ` + seguroDeBorrar + `) AS borrables,
			count(*) AS total,
			COALESCE(sum(c.monto) FILTER (WHERE ` + seguroDeBorrar + `), 0)::text AS monto_borrable
		FROM cargo_cxc c
		WHERE c.empresa_id = $1::uuid`
	var p RegeneracionPlan
	if err := r.pool.QueryRow(ctx, q, empresaID).Scan(&p.Borrables, &p.Total, &p.MontoBorrable); err != nil {
		return RegeneracionPlan{}, fmt.Errorf("cxc: plan de regeneración: %w", err)
	}
	p.SeConservan = p.Total - p.Borrables
	return p, nil
}

// BorrarCargosRegenerables borra SOLO los seguros y devuelve cuántos salieron.
//
// No es `DELETE ... WHERE empresa_id` a secas por nada del mundo: el CASCADE de
// `cobro_aplicacion` convertiría un borrado de más en la pérdida silenciosa de pagos aplicados.
func (r *pgRepository) BorrarCargosRegenerables(ctx context.Context, empresaID string) (int, error) {
	const q = `DELETE FROM cargo_cxc c WHERE ` + seguroDeBorrar
	tag, err := r.pool.Exec(ctx, q, empresaID)
	if err != nil {
		return 0, fmt.Errorf("cxc: borrar cargos regenerables: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
