package cxp

// Consultas del medio de pago alterno (mig 0102). Filtran por empresa_id.

import (
	"context"
	"fmt"
)

// GuardarMedioPago escribe con qué se pagó la factura y la saca del lote si estaba en uno.
//
// SACARLA DEL LOTE no es un extra: una factura que se cierra como pagada por caja chica no puede
// seguir figurando en un corte que va al banco. Si se quedara, el reporte del lote la seguiría
// contando y la macro —que se puede volver a bajar— la llevaría al banco otra vez. Ese es
// exactamente el doble pago que el motivo obligatorio intenta prevenir.
//
// Devuelve si la factura salió de un lote, para poder decírselo a quien lo hizo: es un efecto
// sobre OTRO objeto (el corte de otra persona) y callarlo sería la clase de cambio silencioso que
// después nadie puede explicar.
func (r *pgRepository) GuardarMedioPago(ctx context.Context, empresaID, id string, in MedioPagoInput, usuarioID string) (bool, error) {
	// El CTE `previo` fija si estaba en un lote ANTES del UPDATE (y bloquea la fila). Sin eso, un
	// RETURNING leería el valor nuevo —siempre NULL, porque el propio UPDATE lo borra— y la
	// respuesta diría que no estaba en ningún corte justo cuando sí estaba.
	const q = `
		WITH previo AS (
			SELECT id, (lote_id IS NOT NULL) AS estaba_en_lote
			FROM documento_cxp
			WHERE empresa_id = $1::uuid AND id = $2::uuid
			FOR UPDATE
		),
		upd AS (
			UPDATE documento_cxp d
			   SET medio_pago = $3,
			       medio_pago_motivo = $4,
			       medio_pago_vale_id = NULLIF($5, '')::uuid,
			       medio_pago_en = now(),
			       medio_pago_por = NULLIF($6, '')::uuid,
			       lote_id = NULL,
			       actualizado_en = now()
			  FROM previo p
			 WHERE d.id = p.id
			 RETURNING p.estaba_en_lote
		)
		SELECT estaba_en_lote FROM upd`

	var estabaEnLote bool
	err := r.pool.QueryRow(ctx, q, empresaID, id, in.Medio, in.Motivo, in.ValeID, usuarioID).
		Scan(&estabaEnLote)
	if err != nil {
		return false, fmt.Errorf("cxp: guardar medio de pago: %w", err)
	}
	return estabaEnLote, nil
}

// ValeDeCajaChicaExiste dice si el vale existe en esta empresa y no está anulado.
//
// Un vale anulado es un error de digitación corregido: ligarlo diría que hay respaldo donde ya no
// lo hay.
func (r *pgRepository) ValeDeCajaChicaExiste(ctx context.Context, empresaID, valeID string) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM caja_chica_vale
			WHERE empresa_id = $1::uuid AND id = $2::uuid AND NOT anulado
		)`
	var existe bool
	if err := r.pool.QueryRow(ctx, q, empresaID, valeID).Scan(&existe); err != nil {
		return false, fmt.Errorf("cxp: verificar vale de caja chica: %w", err)
	}
	return existe, nil
}
