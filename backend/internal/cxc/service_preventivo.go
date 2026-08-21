package cxc

import (
	"context"
	"strconv"
	"strings"
)

// diasPreventivos lee DIAS_CONTACTO_PREVENTIVO.
func (s *Service) diasPreventivos(ctx context.Context, empresaID string) int {
	p, err := s.repo.Parametros(ctx, empresaID)
	if err != nil {
		return diasContactoPreventivoDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(p["DIAS_CONTACTO_PREVENTIVO"])); err == nil && v >= 1 && v <= 365 {
		return v
	}
	return diasContactoPreventivoDefault
}

// ListaPreventiva es la lista de avisos antes del vencimiento. El alcance por sede lo pone el
// servicio igual que en la cola: un permiso nuevo no debilita el que ya existía.
func (s *Service) ListaPreventiva(ctx context.Context, empresaID, rol, usuarioID string, f FiltrosPreventivo) (ListaPreventiva, error) {
	sedes, err := s.sedesVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return ListaPreventiva{}, err
	}
	f.SedeIDs = sedes
	if sedes != nil && f.SedeID != "" && !contiene(sedes, f.SedeID) {
		return ListaPreventiva{}, ErrSinPermisoSedes
	}
	return s.repo.ListaPreventiva(ctx, empresaID, f,
		s.diasPreventivos(ctx, empresaID), s.parametrosCola(ctx, empresaID).DiasAlertaTarjeta)
}
