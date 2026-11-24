package cxp

import (
	"context"
	"fmt"
	"strings"

	"github.com/gpvdp/erp/internal/shared"
)

// FacturasDeContado lista lo que quedó trancado por el candado, para UN proveedor.
//
// Se consulta al abrir su ficha: es lo que permite decirle a quien marca «manda el convenio» que
// ese cambio no alcanza para las facturas que ya entraron, y cuántas son.
func (s *Service) FacturasDeContado(ctx context.Context, empresaID, proveedorID string) ([]FacturaDeContado, error) {
	if _, err := s.repo.PorID(ctx, empresaID, proveedorID); err != nil {
		return nil, err
	}
	return s.repo.FacturasDeContado(ctx, empresaID, proveedorID)
}

// LiberarParaPago levanta el candado del contado de varias facturas de UN proveedor, con motivo
// y huella.
//
// Cada factura liberada deja su propio evento de auditoría, no uno por el lote: dentro de seis
// meses la pregunta se hace sobre UNA factura —«¿por qué ésta se pagó por banco si era de
// contado?»— y la respuesta tiene que estar en su expediente, no en un evento colectivo que haya
// que ir a buscar por fecha.
func (s *Service) LiberarParaPago(ctx context.Context, empresaID, proveedorID string, in LiberacionInput, usuarioID string) (ResultadoLiberacion, error) {
	if err := in.Validar(); err != nil {
		return ResultadoLiberacion{}, err
	}
	// El proveedor tiene que existir en ESTA empresa antes de tocar nada: así un id de otra
	// empresa devuelve «no encontrado» en vez de una liberación silenciosa de cero facturas, que
	// se leería como éxito.
	if _, err := s.repo.PorID(ctx, empresaID, proveedorID); err != nil {
		return ResultadoLiberacion{}, err
	}

	// El estado ANTES de tocar nada, para poder explicar cada descarte por su nombre. Se lee
	// aparte y no se deduce del resultado del UPDATE porque «no volvió» tiene tres causas
	// distintas y al operador le sirven distinto: no existe, es de otro proveedor, o ya estaba
	// libre.
	antes, err := s.repo.EstadoDeBloqueo(ctx, empresaID, in.DocumentoIDs)
	if err != nil {
		return ResultadoLiberacion{}, err
	}

	liberadas, err := s.repo.LiberarParaPago(ctx, empresaID, proveedorID, in.DocumentoIDs, in.Motivo, usuarioID)
	if err != nil {
		return ResultadoLiberacion{}, err
	}

	seLibero := map[string]bool{}
	for _, f := range liberadas {
		seLibero[f.DocumentoID] = true
		s.auditarDocNota(ctx, empresaID, f.DocumentoID, "LIBERAR_PAGO", usuarioID,
			fmt.Sprintf("Liberada para pago por banco (el comprobante la declaraba de contado). Motivo: %s", in.Motivo))
	}

	res := ResultadoLiberacion{Liberadas: liberadas, NoSePudo: map[string]string{}}
	for _, id := range in.DocumentoIDs {
		if seLibero[id] {
			continue
		}
		bloqueada, existe := antes[id]
		switch {
		case !existe:
			res.NoSePudo[id] = "No existe o es de otra empresa."
		case !bloqueada:
			res.NoSePudo[id] = "Ya estaba libre: no hacía falta liberarla."
		default:
			// Estaba bloqueada al mirarla y no se liberó: o es de otro proveedor —y la ruta
			// cuelga de éste—, o alguien la tocó en el medio.
			res.NoSePudo[id] = "No es de este proveedor, o cambió mientras se confirmaba."
		}
	}

	// ── QUE LAS PRÓXIMAS NO NAZCAN BLOQUEADAS ───────────────────────────────────────────────
	//
	// Va DESPUÉS de liberar y solo si algo se liberó: configurar la ficha cuando no se pudo
	// liberar nada dejaría al proveedor cambiado por una operación que falló, y nadie ataría una
	// cosa con la otra después.
	//
	// El cambio se audita aparte del de cada factura: es de otro alcance —gobierna TODAS las
	// facturas futuras— y mezclarlo en la nota de un documento lo haría invisible.
	if in.ConfigurarProveedor && len(liberadas) > 0 {
		cambio, err := s.repo.ConfigurarProveedorSinCandado(ctx, empresaID, proveedorID)
		if err != nil {
			return ResultadoLiberacion{}, err
		}
		res.ProveedorConfigurado = true
		if cambio {
			res.QueSeConfiguro = "La ficha quedó en CRÉDITO y con «manda el convenio»: las próximas facturas de este proveedor no van a nacer bloqueadas."
			s.auditarProveedor(ctx, empresaID, proveedorID, "CONFIGURAR_SIN_CANDADO", usuarioID,
				fmt.Sprintf("Al liberar %d factura(s) se dejó la ficha en CRÉDITO con «manda el convenio» para que las próximas no nazcan bloqueadas. Motivo de la liberación: %s",
					len(liberadas), in.Motivo))
		} else {
			// Ya estaba configurada: no se anuncia un cambio que no ocurrió.
			res.QueSeConfiguro = "La ficha ya estaba configurada para que las próximas no nazcan bloqueadas."
		}
	}
	return res, nil
}

// auditarProveedor deja la huella de un cambio en la FICHA, distinto del de un documento.
//
// Entidad propia y no `documento_cxp`: el cambio gobierna todas las facturas futuras del
// proveedor, y colgarlo de la nota de un documento lo volvería invisible justo cuando alguien
// pregunte por qué dejaron de nacer bloqueadas.
func (s *Service) auditarProveedor(ctx context.Context, empresaID, proveedorID, accion, usuarioID, nota string) {
	if s.audit == nil {
		return
	}
	ev := shared.Evento{
		EmpresaID: &empresaID, Entidad: "proveedor", EntidadID: &proveedorID,
		Accion: accion, UsuarioID: &usuarioID,
	}
	if strings.TrimSpace(nota) != "" {
		ev.ValorNuevo = map[string]string{"nota": strings.TrimSpace(nota)}
	}
	s.audit.Registrar(ctx, ev)
}
