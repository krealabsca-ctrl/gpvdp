package bancos

// Las guardas de alcance de la puerta de Contabilidad al catálogo.
//
// El permiso `cxp.catalogo` solo alcanza para los rubros marcados visibles para CxP. Sin esto,
// Contabilidad podría renombrar un rubro bancario que no debería ni ver: el permiso diría una cosa
// y el endpoint permitiría otra.

import (
	"context"
	"fmt"
	"strings"
)

// ConceptoEsVisibleCxP dice si el concepto existe en ESTA empresa y está marcado visible para CxP.
//
// Un id con forma inválida es un «no existe» y no un 500: llega de la URL, así que cualquiera puede
// escribir cualquier cosa ahí.
func (r *pgRepository) ConceptoEsVisibleCxP(ctx context.Context, empresaID, conceptoID string) (bool, error) {
	if strings.TrimSpace(conceptoID) == "" {
		return false, nil
	}
	var visible bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM concepto
		  WHERE id = $2::uuid AND empresa_id = $1::uuid AND activo = true AND visible_cxp
		)`, empresaID, conceptoID).Scan(&visible)
	if err != nil {
		if strings.Contains(err.Error(), "invalid input syntax") {
			return false, nil
		}
		return false, fmt.Errorf("bancos: concepto visible para cxp: %w", err)
	}
	return visible, nil
}

// ClasificacionEsVisibleCxP: la clasificación hereda la visibilidad de su concepto, así que se
// pregunta por el padre. Es la misma regla que usa ListarClasificaciones con soloCxP.
func (r *pgRepository) ClasificacionEsVisibleCxP(ctx context.Context, empresaID, clasificacionID string) (bool, error) {
	if strings.TrimSpace(clasificacionID) == "" {
		return false, nil
	}
	var visible bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM clasificacion cl
		  JOIN concepto co ON co.id = cl.concepto_id
		  WHERE cl.id = $2::uuid AND cl.empresa_id = $1::uuid AND cl.activo = true AND co.visible_cxp
		)`, empresaID, clasificacionID).Scan(&visible)
	if err != nil {
		if strings.Contains(err.Error(), "invalid input syntax") {
			return false, nil
		}
		return false, fmt.Errorf("bancos: clasificación visible para cxp: %w", err)
	}
	return visible, nil
}
