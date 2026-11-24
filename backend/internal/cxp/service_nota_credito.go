package cxp

// Aplicar y reversar la nota de crédito del proveedor (mig 0106). El porqué está en nota_credito.go.

import (
	"context"
	"fmt"
)

// AplicarNotaCredito rebaja una factura con una nota de crédito del mismo proveedor.
//
// Las validaciones van en este orden a propósito: primero lo que identifica mal el documento —no
// es una nota, es de otro proveedor—, después lo que depende del estado, y al final los montos.
// Así el primer mensaje que ve quien se equivocó nombra la causa real y no un síntoma.
func (s *Service) AplicarNotaCredito(ctx context.Context, empresaID, facturaID, notaID, montoStr, usuarioID string) (Documento, error) {
	monto, err := montoNotaValido(montoStr)
	if err != nil {
		return Documento{}, err
	}
	nota, err := s.repo.DocumentoPorID(ctx, empresaID, notaID)
	if err != nil {
		return Documento{}, err
	}
	if nota.Tipo != TipoNotaCredito {
		return Documento{}, ErrNoEsNotaCredito
	}
	factura, err := s.repo.DocumentoPorID(ctx, empresaID, facturaID)
	if err != nil {
		return Documento{}, err
	}
	if factura.Tipo == TipoNotaCredito {
		return Documento{}, ErrNotaSobreNota
	}
	if factura.ProveedorID != nota.ProveedorID {
		return Documento{}, ErrNotaProveedorDistinto
	}
	if factura.Moneda != nota.Moneda {
		return Documento{}, ErrNotaMonedaDistinta
	}
	// Solo antes de aprobar: lo que se aprueba es el NETO, y es la cifra con la que se arma el
	// archivo de pago.
	if !esNeteable(factura.Estado) {
		return Documento{}, ErrNotaFacturaNoNeteable
	}

	if _, err := s.repo.AplicarNotaCredito(ctx, empresaID, notaID, facturaID, monto, usuarioID); err != nil {
		return Documento{}, err
	}
	cons := nota.Consecutivo
	if cons == "" {
		cons = "nota de crédito"
	}
	s.auditarDocNota(ctx, empresaID, facturaID, "APLICAR_NOTA_CREDITO", usuarioID,
		fmt.Sprintf("%s aplicada: %s %s", cons, monto.StringFixed(2), factura.Moneda))
	return s.repo.DocumentoPorID(ctx, empresaID, facturaID)
}

// NotasDeCreditoDisponibles lista las notas con saldo de un proveedor.
func (s *Service) NotasDeCreditoDisponibles(ctx context.Context, empresaID, proveedorID string) ([]NotaCreditoSaldo, error) {
	return s.repo.NotasDeCreditoDisponibles(ctx, empresaID, proveedorID)
}

// NotasAplicadasAFactura lista las rebajas vivas sobre una factura.
func (s *Service) NotasAplicadasAFactura(ctx context.Context, empresaID, facturaID string) ([]AplicacionNotaCredito, error) {
	return s.repo.NotasAplicadasAFactura(ctx, empresaID, facturaID)
}

// ReversarNotaCredito deshace una aplicación: la factura vuelve a deber lo que la nota rebajaba.
//
// Sin esto, aplicar la nota a la factura equivocada sería un camino de ida, y es un error
// fácil de cometer: un proveedor con varias facturas abiertas por montos parecidos. Es el mismo
// criterio que el desenlace del pago (hallazgo 3.1).
func (s *Service) ReversarNotaCredito(ctx context.Context, empresaID, aplicacionID, usuarioID string) error {
	n, err := s.repo.ReversarNotaCredito(ctx, empresaID, aplicacionID, usuarioID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrAplicacionNoEncontrada
	}
	return nil
}
