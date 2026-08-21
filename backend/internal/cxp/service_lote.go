package cxp

import "context"

// CrearLote arma un lote de pago (corte) con las facturas seleccionadas. Las APROBADAS del corte
// se programan automáticamente (fecha de pago = fecha de corte, con su huella) para que "aprobar
// y cortar" sea un solo paso en la Bandeja; las ya PROGRAMADAS sin lote entran directo.
func (s *Service) CrearLote(ctx context.Context, empresaID, fechaCorte string, ids []string, usuarioID string) (LotePago, error) {
	if _, err := s.repo.ProgramarAprobados(ctx, empresaID, ids, fechaCorte); err != nil {
		return LotePago{}, err
	}
	lote, err := s.repo.CrearLote(ctx, empresaID, fechaCorte, ids, usuarioID)
	if err != nil {
		return LotePago{}, err
	}
	s.auditarDoc(ctx, empresaID, lote.ID, "CREAR_LOTE_PAGO", usuarioID)
	return lote, nil
}

// Lotes lista los lotes de pago de la empresa.
func (s *Service) Lotes(ctx context.Context, empresaID string) ([]LotePago, error) {
	return s.repo.ListarLotes(ctx, empresaID)
}

// MacroLote devuelve las líneas de la macro (.txt) de todas las facturas de un lote.
func (s *Service) MacroLote(ctx context.Context, empresaID, loteID string) ([]PagoRow, error) {
	return s.repo.DocumentosParaPagoPorLote(ctx, empresaID, loteID)
}
