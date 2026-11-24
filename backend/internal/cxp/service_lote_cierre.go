package cxp

// Cerrar y reabrir el corte de pago (mig 0104). El porqué está en lote_cierre.go.

import (
	"context"
	"strings"

	"github.com/gpvdp/erp/internal/shared"
)

// CerrarLote da el corte: congela el contenido del lote.
//
// Devuelve además el respaldo bancario del lote —qué facturas suyas siguen sin débito—, porque el
// cierre es el momento natural para preguntarlo: es la misma consulta del hallazgo 3.1 y es la
// única vez que alguien está mirando ese lote con la intención de darlo por terminado. Cerrar NO
// se bloquea por eso: el banco puede ir atrasado y el corte sigue siendo correcto. Se informa.
func (s *Service) CerrarLote(ctx context.Context, empresaID, loteID, usuarioID string) (LotePago, RespaldoDelLote, error) {
	n, err := s.repo.CerrarLote(ctx, empresaID, loteID, usuarioID)
	if err != nil {
		return LotePago{}, RespaldoDelLote{}, err
	}
	if n == 0 {
		existe, cerrado, err := s.repo.LoteEstaCerrado(ctx, empresaID, loteID)
		if err != nil {
			return LotePago{}, RespaldoDelLote{}, err
		}
		if !existe {
			return LotePago{}, RespaldoDelLote{}, ErrLoteNoEncontrado
		}
		if cerrado {
			return LotePago{}, RespaldoDelLote{}, ErrLoteYaCerrado
		}
		return LotePago{}, RespaldoDelLote{}, ErrLoteNoEncontrado
	}
	s.auditarLote(ctx, empresaID, loteID, "CERRAR_LOTE", usuarioID, "")

	respaldo, err := s.RespaldoBancarioDelLote(ctx, empresaID, loteID)
	if err != nil {
		return LotePago{}, RespaldoDelLote{}, err
	}
	lote, err := s.lotePorID(ctx, empresaID, loteID)
	return lote, respaldo, err
}

// ReabrirLote vuelve a abrir un corte cerrado, dejando el motivo.
func (s *Service) ReabrirLote(ctx context.Context, empresaID, loteID, usuarioID, motivo string) (LotePago, error) {
	motivo = strings.TrimSpace(motivo)
	if !MotivoReaperturaValido(motivo) {
		return LotePago{}, ErrMotivoReaperturaRequerido
	}
	n, err := s.repo.ReabrirLote(ctx, empresaID, loteID, usuarioID, motivo)
	if err != nil {
		return LotePago{}, err
	}
	if n == 0 {
		existe, cerrado, err := s.repo.LoteEstaCerrado(ctx, empresaID, loteID)
		if err != nil {
			return LotePago{}, err
		}
		if !existe {
			return LotePago{}, ErrLoteNoEncontrado
		}
		if !cerrado {
			return LotePago{}, ErrLoteNoEstaCerrado
		}
		return LotePago{}, ErrLoteNoEncontrado
	}
	s.auditarLote(ctx, empresaID, loteID, "REABRIR_LOTE", usuarioID, motivo)
	return s.lotePorID(ctx, empresaID, loteID)
}

// lotePorID devuelve un lote del listado por su id.
//
// Se reusa `ListarLotes` en vez de escribir otra consulta: el lote que devuelve cerrar y reabrir
// tiene que traer exactamente los mismos campos calculados que ve la pantalla —cantidad, total,
// pagadas, rebotadas— o la fila se actualizaría con menos datos de los que tenía.
func (s *Service) lotePorID(ctx context.Context, empresaID, loteID string) (LotePago, error) {
	lotes, err := s.repo.ListarLotes(ctx, empresaID)
	if err != nil {
		return LotePago{}, err
	}
	for _, l := range lotes {
		if l.ID == loteID {
			return l, nil
		}
	}
	return LotePago{}, ErrLoteNoEncontrado
}

// auditarLote registra el evento sobre la entidad `lote_pago`, con el motivo cuando lo hay.
//
// Entidad propia y no `documento_cxp`: cerrar un corte no es un hecho de ninguna factura en
// particular, y colgarlo de una las dejaría a las otras sin rastro.
func (s *Service) auditarLote(ctx context.Context, empresaID, loteID, accion, usuarioID, motivo string) {
	if s.audit == nil {
		return
	}
	ev := shared.Evento{
		EmpresaID: &empresaID, Entidad: "lote_pago", EntidadID: &loteID, Accion: accion, UsuarioID: &usuarioID,
	}
	if strings.TrimSpace(motivo) != "" {
		ev.ValorNuevo = map[string]string{"nota": strings.TrimSpace(motivo)}
	}
	s.audit.Registrar(ctx, ev)
}
