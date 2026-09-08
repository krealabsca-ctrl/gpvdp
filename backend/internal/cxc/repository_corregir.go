package cxc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ModalidadExiste verifica que la modalidad esté en el catálogo DE ESTA empresa.
//
// Se valida en la base y no contra una lista en Go: el catálogo es editable por empresa, así que
// cualquier lista quemada en el código quedaría vieja el día que alguien agregue «Semanal» —que es
// justo la modalidad que trajo el archivo de Coopeprofa y que el catálogo no tenía—.
func (r *pgRepository) ModalidadExiste(ctx context.Context, empresaID, modalidadID string) (bool, error) {
	if strings.TrimSpace(modalidadID) == "" {
		return false, nil
	}
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM cxc_modalidad
		  WHERE id = $2::uuid AND empresa_id = $1::uuid
		)`, empresaID, modalidadID).Scan(&existe)
	if err != nil {
		// Un id con forma inválida es un «no existe», no un 500.
		if strings.Contains(err.Error(), "invalid input syntax") {
			return false, nil
		}
		return false, fmt.Errorf("cxc: verificar modalidad: %w", err)
	}
	return existe, nil
}

// CorregirContrato aplica los cambios y vuelve a derivar la marca de revisión.
//
// Son DOS sentencias en una transacción, y el orden es la razón: en un `UPDATE ... SET`, las
// columnas que aparecen del lado derecho todavía valen lo VIEJO, así que derivar la marca en el
// mismo UPDATE la calcularía sobre el dato anterior —el contrato quedaría corregido y apartado al
// mismo tiempo—. Primero se guarda el dato, después se relee la fila ya guardada y se decide.
//
// Nadie «desmarca» un contrato a mano: la marca se DERIVA de que el dato esté bueno, con la misma
// regla que exige el generador de cargos. Si se pudiera desmarcar sin arreglar la cuota, el contrato
// entraría a la cola de cobro con cuota 0 y el cobrador llamaría a alguien a pedirle nada.
func (r *pgRepository) CorregirContrato(ctx context.Context, empresaID, contratoID string, cambios map[string]any) (Contrato, error) {
	if len(cambios) == 0 {
		return Contrato{}, ErrNadaQueCorregir
	}

	sets := make([]string, 0, len(cambios)+1)
	args := []any{empresaID, contratoID}
	add := func(v any) int { args = append(args, v); return len(args) }

	// Orden fijo para que la sentencia sea estable y reproducible.
	for _, col := range []string{"cuota_vigente", "dia_pago", "modalidad_id"} {
		v, ok := cambios[col]
		if !ok {
			continue
		}
		if col == "modalidad_id" {
			sets = append(sets, fmt.Sprintf("modalidad_id = $%d::uuid", add(v)))
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", col, add(v)))
	}
	if len(sets) == 0 {
		return Contrato{}, ErrNadaQueCorregir
	}
	sets = append(sets, "actualizado_en = now()")

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Contrato{}, fmt.Errorf("cxc: begin corregir: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var numero string
	err = tx.QueryRow(ctx, `
		UPDATE contrato_cxc SET `+strings.Join(sets, ", ")+`
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		RETURNING numero`, args...).Scan(&numero)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Contrato{}, ErrContratoNoEncontrado
		}
		return Contrato{}, fmt.Errorf("cxc: corregir contrato: %w", err)
	}

	// Segunda sentencia: la fila ya tiene los valores nuevos, así que se re-deriva la marca con la
	// parte OBJETIVA de la regla del generador. Se usa `sqlMotivoDatoIncompleto` y no
	// `sqlMotivoNoGenerable` porque esa segunda empieza mirando `revision_pendiente`: derivar la
	// marca a partir de sí misma dejaría al contrato apartado para siempre.
	if _, err := tx.Exec(ctx, `
		UPDATE contrato_cxc dest
		SET revision_pendiente = (mo.motivo <> ''), revision_motivo = mo.motivo
		FROM (
		  SELECT `+sqlMotivoDatoIncompleto+` AS motivo
		  FROM contrato_cxc c
		  LEFT JOIN cxc_modalidad m ON m.id = c.modalidad_id AND m.empresa_id = c.empresa_id
		  WHERE c.empresa_id = $1::uuid AND c.id = $2::uuid
		) mo
		WHERE dest.empresa_id = $1::uuid AND dest.id = $2::uuid`, empresaID, contratoID); err != nil {
		return Contrato{}, fmt.Errorf("cxc: rederivar revisión: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Contrato{}, fmt.Errorf("cxc: commit corregir: %w", err)
	}
	// Se relee por el camino normal para devolver el contrato con todo lo derivado (saldo, mora):
	// armarlo a mano acá sería una segunda versión del mismo cálculo.
	return r.ContratoPorNumero(ctx, empresaID, numero)
}
