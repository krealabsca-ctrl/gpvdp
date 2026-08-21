package bancos

import (
	"context"

	"github.com/gpvdp/erp/internal/shared"
)

// Bancos lista los bancos registrados por la empresa (para el catálogo/selector).
func (s *Service) Bancos(ctx context.Context, empresaID string, incluirInactivos bool) ([]BancoItem, error) {
	return s.repo.ListarBancos(ctx, empresaID, incluirInactivos)
}

// CrearBanco registra un banco de la empresa.
func (s *Service) CrearBanco(ctx context.Context, empresaID, nombre, usuarioID string) (BancoItem, error) {
	b, err := s.repo.CrearBanco(ctx, empresaID, nombre)
	if err != nil {
		return BancoItem{}, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "banco", EntidadID: &b.ID, Accion: "CREAR_BANCO", UsuarioID: &usuarioID,
	})
	return b, nil
}

// RenombrarBanco cambia el nombre de un banco de la empresa.
func (s *Service) RenombrarBanco(ctx context.Context, empresaID, bancoID, nombre, usuarioID string) error {
	if err := s.repo.RenombrarBanco(ctx, empresaID, bancoID, nombre); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "banco", EntidadID: &bancoID, Accion: "RENOMBRAR_BANCO", UsuarioID: &usuarioID,
	})
	return nil
}

// CrearCuenta registra una cuenta bajo un banco de la empresa.
func (s *Service) CrearCuenta(ctx context.Context, empresaID, bancoID, alias, iban, moneda, usuarioID string) (CuentaListItem, error) {
	c, err := s.repo.CrearCuenta(ctx, empresaID, bancoID, alias, iban, moneda)
	if err != nil {
		return CuentaListItem{}, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "cuenta_bancaria", EntidadID: &c.ID, Accion: "CREAR_CUENTA", UsuarioID: &usuarioID,
	})
	return c, nil
}

// RenombrarCuenta cambia el alias de una cuenta de la empresa.
func (s *Service) RenombrarCuenta(ctx context.Context, empresaID, cuentaID, alias, usuarioID string) error {
	if err := s.repo.RenombrarCuenta(ctx, empresaID, cuentaID, alias); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "cuenta_bancaria", EntidadID: &cuentaID, Accion: "RENOMBRAR_CUENTA", UsuarioID: &usuarioID,
	})
	return nil
}
