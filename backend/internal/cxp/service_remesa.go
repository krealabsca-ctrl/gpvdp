package cxp

// Reporte de «lo que se va a pagar» y aviso de pago por proveedor.

import (
	"context"

	"github.com/shopspring/decimal"
)

// ReporteDeLote arma el detalle del corte: una entrada por transferencia, con sus facturas.
//
// Se compone de los MISMOS avisos que se le mandan a cada proveedor, así que el total del reporte
// y lo que recibe el proveedor no pueden discrepar: son las mismas filas leídas una vez.
func (s *Service) ReporteDeLote(ctx context.Context, empresaID, loteID string) (ReporteLote, error) {
	lote, err := s.repo.LotePorID(ctx, empresaID, loteID)
	if err != nil {
		return ReporteLote{}, err
	}
	rep := ReporteLote{
		LoteID: lote.ID, LoteNumero: lote.Numero, FechaCorte: lote.FechaCorte,
		ModoPago: lote.ModoPago, Pagos: []AvisoDePago{},
	}

	ids, err := s.idsDeTransferencias(ctx, empresaID, lote)
	if err != nil {
		return ReporteLote{}, err
	}
	total := decimal.Zero
	for _, id := range ids {
		var av AvisoDePago
		if lote.ModoPago == ModoProveedor {
			av, err = s.repo.AvisoDeRemesa(ctx, empresaID, id)
		} else {
			av, err = s.repo.AvisoDeFactura(ctx, empresaID, id)
		}
		if err != nil {
			return ReporteLote{}, err
		}
		rep.Pagos = append(rep.Pagos, av)
		rep.Facturas += len(av.Lineas)
		if av.IBAN == "" {
			rep.SinIBAN++
		}
		total = total.Add(decOrCero(av.TotalCRC))
	}
	rep.Transferencias = len(rep.Pagos)
	rep.TotalCRC = total.StringFixed(2)
	return rep, nil
}

// idsDeTransferencias devuelve qué hay que recorrer según el modo: las remesas del lote, o las
// facturas una por una. Es la única parte del reporte que depende del modo.
func (s *Service) idsDeTransferencias(ctx context.Context, empresaID string, lote LotePago) ([]string, error) {
	if lote.ModoPago == ModoProveedor {
		remesas, err := s.repo.RemesasDeLote(ctx, empresaID, lote.ID)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(remesas))
		for _, r := range remesas {
			ids = append(ids, r.ID)
		}
		return ids, nil
	}
	// Modo por factura: las mismas filas que arma la macro, para que el reporte no pueda listar
	// una factura que el archivo del banco no va a pagar (una bloqueada, por ejemplo).
	filas, err := s.repo.DocumentosParaPagoPorLote(ctx, empresaID, lote.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(filas))
	for _, f := range filas {
		ids = append(ids, f.DocumentoID)
	}
	return ids, nil
}

// AvisoDeRemesa devuelve el detalle de UNA transferencia agrupada, para mandárselo al proveedor.
func (s *Service) AvisoDeRemesa(ctx context.Context, empresaID, remesaID string) (AvisoDePago, error) {
	return s.repo.AvisoDeRemesa(ctx, empresaID, remesaID)
}

// RemesasDeLote lista los pagos agrupados de un lote.
func (s *Service) RemesasDeLote(ctx context.Context, empresaID, loteID string) ([]Remesa, error) {
	return s.repo.RemesasDeLote(ctx, empresaID, loteID)
}
