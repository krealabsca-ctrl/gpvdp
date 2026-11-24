package cxp

// El cronograma de pago, del lado del negocio. El porqué está en cronograma.go.

import (
	"context"

	"github.com/shopspring/decimal"
)

// Cronograma arma el cuadro que se manda ANTES de pagar.
func (s *Service) Cronograma(ctx context.Context, empresaID, loteID string) (Cronograma, error) {
	lote, err := s.repo.LotePorID(ctx, empresaID, loteID)
	if err != nil {
		return Cronograma{}, err
	}
	tc, err := s.repo.TCDelCronograma(ctx, empresaID, loteID)
	if err != nil {
		return Cronograma{}, err
	}
	filas, err := s.repo.LineasDelCronograma(ctx, empresaID, loteID)
	if err != nil {
		return Cronograma{}, err
	}
	c, err := ConstruirCronograma(filas, tc)
	if err != nil {
		return Cronograma{}, err
	}
	c.LoteID = lote.ID
	c.LoteNumero = lote.Numero
	c.FechaCorte = lote.FechaCorte
	return c, nil
}

// GuardarTCDelCronograma deja el tipo de cambio con el que tesorería coloniza este cuadro.
func (s *Service) GuardarTCDelCronograma(ctx context.Context, empresaID, loteID, valor, usuarioID string) error {
	tc, err := decimal.NewFromString(valor)
	if err != nil || !tc.IsPositive() {
		return ErrTCCronogramaInvalido
	}
	if err := s.repo.GuardarTCDelCronograma(ctx, empresaID, loteID, tc); err != nil {
		return err
	}
	s.auditarDoc(ctx, empresaID, loteID, "TC_CRONOGRAMA", usuarioID)
	return nil
}

// AsignarCuentaDePago dice de qué cuenta sale lo que se le paga a un proveedor en este corte.
//
// `cuentaID` vacío la quita: hay que poder deshacer una asignación equivocada, y sin cuenta la
// línea vuelve al flujo normal, que es de donde salió.
func (s *Service) AsignarCuentaDePago(ctx context.Context, empresaID, loteID, proveedorID, cuentaID, usuarioID string) error {
	n, err := s.repo.AsignarCuentaDePago(ctx, empresaID, loteID, proveedorID, cuentaID)
	if err != nil {
		return err
	}
	if n == 0 {
		// Cero filas significa que el proveedor no está en el corte, o que la cuenta no es de esta
		// empresa. Las dos cosas son lo mismo para quien lo pidió: no había a qué aplicarlo.
		return ErrProveedorNoEstaEnElCorte
	}
	s.auditarProveedor(ctx, empresaID, proveedorID, "CUENTA_DE_PAGO", usuarioID,
		"Se definió de qué cuenta sale el pago de este proveedor en el corte.")
	return nil
}

// MarcarPorFuera saca del cronograma (o lo devuelve) lo que se le paga a un proveedor.
func (s *Service) MarcarPorFuera(ctx context.Context, empresaID, loteID, proveedorID string, porFuera bool, usuarioID string) error {
	n, err := s.repo.MarcarPorFuera(ctx, empresaID, loteID, proveedorID, porFuera, usuarioID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrProveedorNoEstaEnElCorte
	}
	accion, detalle := "QUITAR_POR_FUERA", "Este pago vuelve a entrar en el cronograma que se comunica."
	if porFuera {
		accion, detalle = "MARCAR_POR_FUERA", "Este pago sale igual, pero no entra en el cronograma que se comunica."
	}
	s.auditarProveedor(ctx, empresaID, proveedorID, accion, usuarioID, detalle)
	return nil
}
