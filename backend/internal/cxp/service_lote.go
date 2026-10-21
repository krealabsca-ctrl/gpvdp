package cxp

import "context"

// CrearLote arma un lote de pago (corte) con las facturas seleccionadas. Las APROBADAS del corte
// se programan automáticamente (fecha de pago = fecha de corte, con su huella) para que "aprobar
// y cortar" sea un solo paso en la Bandeja; las ya PROGRAMADAS sin lote entran directo.
func (s *Service) CrearLote(ctx context.Context, empresaID, fechaCorte, modo string, ids []string, usuarioID string) (LotePago, error) {
	// Vacío = el modo de siempre. Es el único default que se acepta: un modo escrito y desconocido
	// se rechaza, porque decide cuántas transferencias salen y por cuánto.
	if modo == "" {
		modo = ModoFactura
	}
	if _, err := s.repo.ProgramarAprobados(ctx, empresaID, ids, fechaCorte); err != nil {
		return LotePago{}, err
	}
	lote, err := s.repo.CrearLote(ctx, empresaID, fechaCorte, modo, ids, usuarioID)
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
// MacroLote devuelve las líneas del corte y el NÚMERO del lote.
//
// El número viaja porque es lo que nombra el archivo: `NombreMacro` lo pone adelante
// («macro-lote-0043-CRC.txt») para que dos cortes del mismo día no bajen con el mismo nombre. Sin
// él, el navegador los guarda como «macro-pagos-CRC.txt» y «macro-pagos-CRC (1).txt», y cuál es
// cuál se vuelve una adivinanza — la misma trampa que ya existía entre colones y dólares.
func (s *Service) MacroLote(ctx context.Context, empresaID, loteID string) ([]PagoRow, int64, error) {
	rows, err := s.repo.DocumentosParaPagoPorLote(ctx, empresaID, loteID)
	if err != nil {
		return nil, 0, err
	}
	// El número es para NOMBRAR el archivo: si no se pudiera leer, la macro igual se baja (con el
	// nombre viejo) en vez de fallar. Bajar el archivo es lo que no puede perderse.
	lote, err := s.repo.LotePorID(ctx, empresaID, loteID)
	if err != nil {
		return rows, 0, nil
	}
	return rows, lote.Numero, nil
}
