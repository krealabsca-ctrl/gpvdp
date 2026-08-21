package cxp

import (
	"context"
	"errors"
)

// ArchivoPago devuelve las líneas del archivo de pago (documentos PROGRAMADOS).
func (s *Service) ArchivoPago(ctx context.Context, empresaID, fecha string) ([]PagoRow, error) {
	return s.repo.DocumentosParaPago(ctx, empresaID, fecha)
}

// ArchivoPagoLote devuelve las líneas del archivo de pago de un lote de documentos por id
// (solo los que estén PROGRAMADO). Es la "macro" del lote seleccionado en la bandeja.
func (s *Service) ArchivoPagoLote(ctx context.Context, empresaID string, ids []string) ([]PagoRow, error) {
	return s.repo.DocumentosParaPagoPorIDs(ctx, empresaID, ids)
}

// Conciliar busca la huella en la descripción de un movimiento bancario y, si encuentra el
// documento CxP asociado (PROGRAMADO/PAGADO), lo lleva a CONCILIADO. Devuelve (doc, encontrado).
// Si no hay huella o no hay documento, devuelve encontrado=false sin error (no es un pago CxP).
func (s *Service) Conciliar(ctx context.Context, empresaID, descripcion, usuarioID string) (Documento, bool, error) {
	huella, ok := extraerHuella(descripcion)
	if !ok {
		return Documento{}, false, nil
	}
	doc, err := s.repo.DocumentoPorHuella(ctx, empresaID, huella)
	if err != nil {
		if errors.Is(err, ErrDocumentoNoEncontrado) {
			return Documento{}, false, nil
		}
		return Documento{}, false, err
	}
	// El movimiento bancario ES el pago: se lleva a CONCILIADO respetando el flujo.
	if doc.Estado == EstProgramado {
		if _, err := s.repo.CambiarEstado(ctx, empresaID, doc.ID, EstProgramado, EstPagado); err != nil {
			return Documento{}, false, err
		}
	}
	if _, err := s.repo.CambiarEstado(ctx, empresaID, doc.ID, EstPagado, EstConciliado); err != nil {
		return Documento{}, false, err
	}
	s.auditarDoc(ctx, empresaID, doc.ID, "CONCILIAR_AUTO", usuarioID)
	final, err := s.repo.DocumentoPorID(ctx, empresaID, doc.ID)
	return final, true, err
}
