package cxp

import (
	"context"

	"github.com/shopspring/decimal"
)

// HistorialDocumento devuelve la línea de tiempo (trazabilidad) del documento.
func (s *Service) HistorialDocumento(ctx context.Context, empresaID, docID string) ([]EventoHistorial, error) {
	return s.repo.HistorialDocumento(ctx, empresaID, docID)
}

// Dashboard devuelve el tablero del módulo CxP: cartera a hoy + movimiento del período.
// Aplica el MISMO alcance por área que la Bandeja (un validador ve solo su departamento en
// las dos pantallas; antes el tablero le mostraba la empresa completa).
func (s *Service) Dashboard(ctx context.Context, empresaID, periodo, rol, usuarioID string) (DashboardCxP, error) {
	if periodo == "" {
		periodo = PeriodoActualCR()
	}
	if !periodoValido(periodo) {
		return DashboardCxP{}, ErrPeriodoInvalido
	}
	deptIDs, err := s.departamentosVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return DashboardCxP{}, err
	}
	d, err := s.repo.DashboardCxP(ctx, empresaID, periodo, deptIDs)
	if err != nil {
		return DashboardCxP{}, err
	}
	// La cola sale del MISMO resumen que alimenta las pestañas de la Bandeja: así el
	// tablero y la Bandeja nunca pueden contradecirse (antes cada uno tenía su propio
	// mapeo de estados y «por aprobar» ni existía en el tablero).
	cola, err := s.repo.ResumenBandeja(ctx, empresaID, deptIDs)
	if err != nil {
		return DashboardCxP{}, err
	}
	if cola == nil {
		cola = []FaseBandeja{}
	}
	d.Cola = cola
	return d, nil
}

// Bandeja devuelve el resumen (conteo + monto) por fase de la Bandeja CxP, con scoping por
// área: el validador solo cuenta las facturas de su(s) departamento(s).
func (s *Service) Bandeja(ctx context.Context, empresaID, rol, usuarioID string) ([]FaseBandeja, error) {
	deptIDs, err := s.departamentosVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return nil, err
	}
	return s.repo.ResumenBandeja(ctx, empresaID, deptIDs)
}

// decOrZeroSilent parsea un monto decimal tolerando vacío o basura (0). Nunca usa float64.
func decOrZeroSilent(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}
