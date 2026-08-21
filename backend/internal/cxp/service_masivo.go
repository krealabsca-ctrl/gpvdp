package cxp

import (
	"context"

	"go.uber.org/zap"
)

// TransicionMasiva aplica una acción del flujo (revisar/aprobar/programar/pagar/conciliar)
// a varios documentos reutilizando la lógica por documento: misma matriz de aprobación,
// guardas de estado y auditoría. La autorización por rol se verifica UNA vez (la acción es
// la misma para todo el lote). Es best-effort: el error de un documento no frena a los demás,
// cada uno reporta su resultado.
func (s *Service) TransicionMasiva(ctx context.Context, empresaID, usuarioID, rol, accion string, ids []string, fechaPago, nota string) (ResultadoMasivo, error) {
	if !accionValida(accion) {
		return ResultadoMasivo{}, ErrAccionInvalida
	}
	if len(ids) == 0 {
		return ResultadoMasivo{}, ErrSinDocumentos
	}
	if !rolPuedeAccion(rol, accion) {
		return ResultadoMasivo{}, ErrRolNoAutorizado
	}
	if accion == AccProgramar && fechaPago == "" {
		return ResultadoMasivo{}, ErrFechaPagoRequerida
	}

	conNota := nota != "" && (accion == AccDenegar || accion == AccAnular || accion == AccLiquidar || accion == AccRebotar)
	res := ResultadoMasivo{Resultados: make([]ResultadoTransicion, 0, len(ids))}
	for _, id := range ids {
		doc, err := s.aplicarAccion(ctx, empresaID, usuarioID, rol, accion, id, fechaPago)
		rt := ResultadoTransicion{ID: id}
		if err != nil {
			rt.OK = false
			rt.Error = err.Error()
			res.Fallidos++
		} else {
			rt.OK = true
			rt.Estado = doc.Estado
			res.Exitosos++
			// Motivo/detalle del archivo (contrapartida): se guarda con la transición.
			if conNota {
				if e := s.repo.GuardarNotaRevision(ctx, empresaID, id, nota); e != nil {
					s.log.Warn("cxp: no se pudo guardar la nota de revisión", zap.Error(e))
				}
			}
		}
		res.Resultados = append(res.Resultados, rt)
	}
	return res, nil
}

// aplicarAccion despacha la acción a la transición por documento correspondiente.
func (s *Service) aplicarAccion(ctx context.Context, empresaID, usuarioID, rol, accion, id, fechaPago string) (Documento, error) {
	switch accion {
	case AccRevisar:
		return s.Revisar(ctx, empresaID, id, usuarioID)
	case AccAprobar:
		return s.Aprobar(ctx, empresaID, id, usuarioID, rol)
	case AccProgramar:
		return s.Programar(ctx, empresaID, id, fechaPago, usuarioID)
	case AccPagar:
		return s.MarcarPagado(ctx, empresaID, id, usuarioID)
	case AccConciliar:
		return s.MarcarConciliado(ctx, empresaID, id, usuarioID)
	case AccDenegar:
		return s.Denegar(ctx, empresaID, id, usuarioID)
	case AccAnular:
		return s.Anular(ctx, empresaID, id, usuarioID)
	case AccLiquidar:
		return s.Liquidar(ctx, empresaID, id, usuarioID)
	case AccRebotar:
		return s.Rebotar(ctx, empresaID, id, usuarioID)
	case AccReintentar:
		return s.Reintentar(ctx, empresaID, id, usuarioID)
	default:
		return Documento{}, ErrAccionInvalida
	}
}
