package cxp

// La aprobación del corte, del lado del negocio (mig 0123). El porqué está en lote_aprobacion.go.

import (
	"context"
	"strings"

	"github.com/gpvdp/erp/internal/shared"
)

// auditarConNota registra un evento sobre cualquier entidad, guardando el motivo.
//
// `auditarEntidad` no lleva nota y `auditarLote` fija la entidad en `lote_pago`; acá hace falta
// la combinación: el hecho es de la FACTURA y el motivo es la mitad útil del registro.
func (s *Service) auditarConNota(ctx context.Context, empresaID, entidad, id, accion, usuarioID, nota string) {
	if s.audit == nil {
		return
	}
	ev := shared.Evento{
		EmpresaID: &empresaID, Entidad: entidad, EntidadID: &id, Accion: accion, UsuarioID: &usuarioID,
	}
	if strings.TrimSpace(nota) != "" {
		ev.ValorNuevo = map[string]string{"nota": strings.TrimSpace(nota)}
	}
	s.audit.Registrar(ctx, ev)
}

// MandarLoteAAprobar manda el corte a la gerencia financiera.
//
// Lo que la gerencia va a mirar es el CRONOGRAMA, que ya existe (mig 0122): la lista por
// proveedor, de qué banco sale cada pago y la cascada con el número que se comunica. No se
// construye un documento nuevo para esto.
func (s *Service) MandarLoteAAprobar(ctx context.Context, empresaID, loteID, usuarioID string) (LotePago, error) {
	n, err := s.repo.MandarAAprobar(ctx, empresaID, loteID, usuarioID)
	if err != nil {
		return LotePago{}, err
	}
	if n == 0 {
		return LotePago{}, s.porQueNoSeMovio(ctx, empresaID, loteID, LoteAbierto, ErrLoteNoEstaAbierto)
	}
	s.auditarLote(ctx, empresaID, loteID, "MANDAR_LOTE_A_APROBAR", usuarioID, "")
	return s.lotePorID(ctx, empresaID, loteID)
}

// AprobarLote es la firma de la gerencia: a partir de acá se puede bajar el archivo del banco.
func (s *Service) AprobarLote(ctx context.Context, empresaID, loteID, usuarioID string) (LotePago, error) {
	n, err := s.repo.AprobarLote(ctx, empresaID, loteID, usuarioID)
	if err != nil {
		return LotePago{}, err
	}
	if n == 0 {
		return LotePago{}, s.porQueNoSeMovio(ctx, empresaID, loteID, LoteEnAprobacion, ErrLoteNoEstaEnAprobacion)
	}
	s.auditarLote(ctx, empresaID, loteID, "APROBAR_LOTE", usuarioID, "")
	return s.lotePorID(ctx, empresaID, loteID)
}

// DevolverLote rechaza el corte ENTERO y lo manda de vuelta a tesorería.
//
// Para sacar UNA factura está `ExcluirFacturaDelLote`: devolver es para cuando lo que está mal es
// el corte —la fecha, el banco de origen, la mitad de las líneas— y arreglarlo factura por
// factura sería más lento que rearmarlo.
func (s *Service) DevolverLote(ctx context.Context, empresaID, loteID, usuarioID, motivo string) (LotePago, error) {
	motivo = strings.TrimSpace(motivo)
	if !MotivoDevolucionValido(motivo) {
		return LotePago{}, ErrMotivoDevolucionRequerido
	}
	n, err := s.repo.DevolverLote(ctx, empresaID, loteID, usuarioID, motivo)
	if err != nil {
		return LotePago{}, err
	}
	if n == 0 {
		return LotePago{}, s.porQueNoSeMovio(ctx, empresaID, loteID, LoteEnAprobacion, ErrLoteNoEstaEnAprobacion)
	}
	s.auditarLote(ctx, empresaID, loteID, "DEVOLVER_LOTE", usuarioID, motivo)
	return s.lotePorID(ctx, empresaID, loteID)
}

// ExcluirFacturaDelLote saca una factura del corte; vuelve a la bandeja lista para el siguiente.
//
// Se puede mientras el corte esté ABIERTO o EN_APROBACION. Aprobado ya no: el contenido aprobado
// es lo que la gerencia firmó, y cambiarlo después haría que la firma describa otro conjunto.
func (s *Service) ExcluirFacturaDelLote(ctx context.Context, empresaID, loteID, documentoID, usuarioID, motivo string) (FacturaExcluida, error) {
	motivo = strings.TrimSpace(motivo)
	if !MotivoExclusionValido(motivo) {
		return FacturaExcluida{}, ErrMotivoExclusionRequerido
	}
	estado, existe, err := s.repo.EstadoDelLote(ctx, empresaID, loteID)
	if err != nil {
		return FacturaExcluida{}, err
	}
	if !existe {
		return FacturaExcluida{}, ErrLoteNoEncontrado
	}
	// El orden importa: primero el corte cerrado (la guarda vieja de la mig 0104), después el
	// aprobado. Un corte cerrado también está «no abierto», y decirle a alguien que lo devuelva
	// cuando en realidad ya se pagó lo mandaría a deshacer el pago.
	if estado == LoteCerrado {
		return FacturaExcluida{}, ErrLoteCerradoNoSeToca
	}
	if estado == LoteAprobado {
		return FacturaExcluida{}, ErrLoteNoSeTocaAprobado
	}

	n, err := s.repo.ExcluirDelLote(ctx, empresaID, loteID, documentoID, usuarioID, motivo)
	if err != nil {
		return FacturaExcluida{}, err
	}
	if n == 0 {
		return FacturaExcluida{}, ErrFacturaNoEstaEnElLote
	}
	// La auditoría cuelga de la FACTURA y no del lote: es un hecho de esa factura —salió del
	// corte— y es en su expediente donde alguien va a preguntar por qué no se pagó.
	s.auditarConNota(ctx, empresaID, "documento_cxp", documentoID, "EXCLUIR_DEL_LOTE", usuarioID, motivo)

	excluidas, err := s.repo.ExcluidasDelLote(ctx, empresaID, loteID)
	if err != nil {
		return FacturaExcluida{}, err
	}
	for _, e := range excluidas {
		if e.DocumentoID == documentoID {
			return e, nil
		}
	}
	return FacturaExcluida{}, nil
}

// ExcluidasDelLote lista lo que la gerencia sacó de un corte.
func (s *Service) ExcluidasDelLote(ctx context.Context, empresaID, loteID string) ([]FacturaExcluida, error) {
	return s.repo.ExcluidasDelLote(ctx, empresaID, loteID)
}

// porQueNoSeMovio traduce un UPDATE que no afectó filas en el error que explica por qué.
//
// Sin esto, «no existe» y «estaba en otro estado» darían el mismo mensaje y quien lo recibe no
// sabría si el corte se le perdió o si alguien ya lo aprobó mientras él miraba la pantalla.
func (s *Service) porQueNoSeMovio(ctx context.Context, empresaID, loteID, esperado string, siNoEstaba error) error {
	estado, existe, err := s.repo.EstadoDelLote(ctx, empresaID, loteID)
	if err != nil {
		return err
	}
	if !existe {
		return ErrLoteNoEncontrado
	}
	if estado != esperado {
		return siNoEstaba
	}
	return ErrLoteNoEncontrado
}
