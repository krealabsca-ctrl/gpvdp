package cxp

// Registrar un pago parcial y mandar a cobrar el resto (mig 0105). El porqué está en pago_parcial.go.

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"
)

// RegistrarPagoParcial deja constancia de lo que el banco debitó cuando no alcanzó el neto.
//
// SI CON ESTO LA FACTURA QUEDA SALDADA, se concilia. Ese caso no es raro: una factura que entró en
// dos transferencias termina de pagarse con el segundo débito, y sería absurdo exigir que alguien
// vaya aparte a declararla conciliada cuando el propio registro ya demuestra que la plata entró
// completa.
func (s *Service) RegistrarPagoParcial(ctx context.Context, empresaID, documentoID, movimientoID, montoStr, motivo, usuarioID string) (SaldoDeFactura, error) {
	motivo = strings.TrimSpace(motivo)
	if len([]rune(motivo)) < motivoLiberacionMinimo {
		return SaldoDeFactura{}, ErrParcialMotivoRequerido
	}
	monto, err := montoParcialValido(montoStr)
	if err != nil {
		return SaldoDeFactura{}, err
	}

	doc, err := s.repo.DocumentoPorID(ctx, empresaID, documentoID)
	if err != nil {
		return SaldoDeFactura{}, err
	}
	// Solo tiene sentido sobre lo que ya salió o está por salir. Registrar un débito contra una
	// factura que nadie mandó pagar saltearía el flujo entero —aprobación incluida— por una
	// pantalla de cuadre.
	if doc.Estado != EstPagado && doc.Estado != EstProgramado {
		return SaldoDeFactura{}, ErrParcialFacturaNoPagable
	}

	// El monto va en la moneda de la FACTURA, que es la del neto contra el que se compara.
	if _, err := s.repo.RegistrarPagoParcial(ctx, empresaID, documentoID, movimientoID, monto, doc.Moneda, motivo, usuarioID); err != nil {
		return SaldoDeFactura{}, err
	}
	s.auditarDocNota(ctx, empresaID, documentoID, "REGISTRAR_PAGO_PARCIAL", usuarioID,
		monto.StringFixed(2)+" "+doc.Moneda+": "+motivo)

	saldo, err := s.repo.SaldoDeFactura(ctx, empresaID, documentoID)
	if err != nil {
		return SaldoDeFactura{}, err
	}

	// ¿Quedó saldada? El saldo lo calcula el neto, que ya resta los parciales.
	if pendiente, errD := decimal.NewFromString(saldo.Saldo); errD == nil && pendiente.IsZero() {
		if doc.Estado == EstProgramado {
			if _, err := s.repo.CambiarEstado(ctx, empresaID, documentoID, EstProgramado, EstPagado); err != nil {
				return SaldoDeFactura{}, err
			}
		}
		n, err := s.repo.CambiarEstado(ctx, empresaID, documentoID, EstPagado, EstConciliado)
		if err != nil {
			return SaldoDeFactura{}, err
		}
		if n > 0 {
			s.auditarDocNota(ctx, empresaID, documentoID, "CONCILIAR_POR_PARCIALES", usuarioID,
				"los pagos parciales cubren el neto completo")
		}
	}
	return saldo, nil
}

// SaldoDeFactura es cuánto lleva pagada una factura y cuánto le falta, con su detalle.
func (s *Service) SaldoDeFactura(ctx context.Context, empresaID, documentoID string) (SaldoDeFactura, error) {
	return s.repo.SaldoDeFactura(ctx, empresaID, documentoID)
}

// MandarACobrarElSaldo devuelve al circuito de pago lo que una factura quedó debiendo.
//
// NO es automático al registrar el parcial, y esa es la decisión que lo gobierna: la diferencia
// puede ser una nota de crédito o una retención no aplicada, y en esos casos volver a cobrarla le
// paga de más al proveedor. Lo decide una persona y queda el motivo.
func (s *Service) MandarACobrarElSaldo(ctx context.Context, empresaID, documentoID, motivo, usuarioID string) (Documento, error) {
	motivo = strings.TrimSpace(motivo)
	if len([]rune(motivo)) < motivoLiberacionMinimo {
		return Documento{}, ErrMotivoCobroRequerido
	}
	n, err := s.repo.MandarACobrarElSaldo(ctx, empresaID, documentoID)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		// Sin saldo, sin parciales, o en un estado que no corresponde. Las tres llevan al mismo
		// lugar: no hay nada que volver a cobrar.
		return Documento{}, ErrSinSaldoQueCobrar
	}
	s.auditarDocNota(ctx, empresaID, documentoID, "COBRAR_SALDO", usuarioID, motivo)
	return s.repo.DocumentoPorID(ctx, empresaID, documentoID)
}
