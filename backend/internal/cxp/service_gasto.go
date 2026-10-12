package cxp

import "context"

// AnalisisGasto arma la pantalla completa: la cobertura primero, los ítems después.
//
// LA COBERTURA SE PIDE SIEMPRE, aunque no haya un solo ítem. Es justamente cuando la lista viene
// vacía que hace falta explicar por qué —no hay XML cargado todavía— en vez de dejar una pantalla
// en blanco que se lee como "no gastamos nada".
func (s *Service) AnalisisGasto(ctx context.Context, empresaID string, f FiltrosGasto) (AnalisisGasto, error) {
	cob, err := s.repo.CoberturaDeGasto(ctx, empresaID, f)
	if err != nil {
		return AnalisisGasto{}, err
	}
	items, total, cuantos, err := s.repo.ItemsDeGasto(ctx, empresaID, f)
	if err != nil {
		return AnalisisGasto{}, err
	}
	return AnalisisGasto{Cobertura: cob, Items: items, Total: total, ItemsTotal: cuantos}, nil
}

// ComprasDeItem lista las compras de un CABYS bajo el mismo filtro de la lista.
func (s *Service) ComprasDeItem(ctx context.Context, empresaID, cabys string, f FiltrosGasto) ([]CompraDeItem, error) {
	return s.repo.ComprasDeItem(ctx, empresaID, cabys, f)
}
