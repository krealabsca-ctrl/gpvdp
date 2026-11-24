package cxp

// Deshacer una conciliación: la factura vuelve a estar esperando su débito (20-oct-2026).
//
// ── POR QUÉ EXISTE ──────────────────────────────────────────────────────────────────────────
//
// Al construir el enlace manual quedó un camino de ida sin vuelta: una persona elige un débito y
// la factura queda CONCILIADA. Si eligió el par equivocado —dos facturas del mismo proveedor por
// el mismo monto en la misma semana es un caso corriente—, la factura queda cerrada contra plata
// ajena y **no hay forma de deshacerlo desde la aplicación**.
//
// Es, exactamente, el defecto que este módulo ya tuvo tres veces: `incluido` con veinte lectores y
// cero escritores hasta la 0093; `bloqueado_para_pago` con doce lectores y una sola puerta de
// entrada hasta la 0097; el lote que solo sabía abrirse hasta la 0104. Una acción que no se puede
// revertir no es un control, es una trampa — y acá sería peor, porque la herramienta que existe
// para destapar errores de conciliación podría crear uno nuevo e invisible.
//
// ── QUÉ DESHACE Y QUÉ NO ────────────────────────────────────────────────────────────────────
//
// Deshace el ENLACE y el estado: la factura vuelve a PAGADO, que es donde estaba —«lo declaramos
// pagado, el banco no lo ha confirmado»—. NO vuelve a PROGRAMADO: que el pago se declaró es un
// hecho que sigue en pie; lo que se retira es la confirmación bancaria.
//
// NO deshace la CLASIFICACIÓN del movimiento. El enlace clasifica solo cuando el movimiento venía
// sin identificar, y esa partida puede seguir siendo la correcta aunque la factura no lo fuera.
// Revertirla a ciegas devolvería a «no identificado» un movimiento que alguien ya entendió. Si
// además está mal, se corrige por donde se corrigen todas.

import (
	"context"
	"errors"
	"strings"
)

// «NO HAY NADA QUE DESHACER» VIAJA COMO CERO, NO COMO ERROR.
//
// Esto cruza el puerto hacia Bancos, que no puede importar este paquete: un error de acá llegaría
// opaco y saldría por el `default` como 500 «error interno». Devolver cuántas facturas volvieron
// —cero incluido— deja que Bancos lo traduzca con un mensaje que se lee. Es la misma decisión que
// tomaron los veredictos de la conciliación.
var (
	// ErrMotivoDesenlaceRequerido: sin motivo no se deshace.
	//
	// Mismo criterio que enlazar: deshacer es afirmar «este débito NO era el pago de esta
	// factura», y la factura vuelve a aparecer como pendiente en el arqueo. Sin el motivo, dentro
	// de seis meses nadie distingue la corrección legítima del error que alguien tapó.
	ErrMotivoDesenlaceRequerido = errors.New("cxp: decí por qué este débito no era el pago de esa factura")
)

// Desconciliar devuelve a PAGADO las facturas que cerraba un movimiento.
//
// Recibe el destino del enlace tal como lo tenía el movimiento: una factura, o una remesa (que
// cubre varias). Devuelve cuántas volvieron, para que Bancos pueda decirlo.
func (s *Service) Desconciliar(ctx context.Context, empresaID, documentoID, remesaID, usuarioID, motivo string) (int, error) {
	if len([]rune(strings.TrimSpace(motivo))) < motivoLiberacionMinimo {
		return 0, ErrMotivoDesenlaceRequerido
	}
	// Un pago agrupado cerró VARIAS facturas de un golpe, así que deshacerlo las abre todas. Va por
	// una consulta y no por un bucle, igual que `CerrarRemesa`: abrir la mitad porque una falló a
	// medio camino dejaría la remesa en dos estados a la vez.
	if remesaID != "" {
		n, err := s.repo.AbrirRemesa(ctx, empresaID, remesaID)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, nil
		}
		s.auditarDocNota(ctx, empresaID, remesaID, "DESCONCILIAR_REMESA", usuarioID, motivo)
		return n, nil
	}

	n, err := s.repo.CambiarEstado(ctx, empresaID, documentoID, EstConciliado, EstPagado)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	s.auditarDocNota(ctx, empresaID, documentoID, "DESCONCILIAR_DOCUMENTO", usuarioID, motivo)
	return 1, nil
}
