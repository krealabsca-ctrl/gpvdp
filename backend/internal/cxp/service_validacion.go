package cxp

import "context"

// Validadores lista los responsables (titular/suplente) de un departamento.
func (s *Service) Validadores(ctx context.Context, empresaID, deptoID string) ([]Validador, error) {
	return s.repo.ListarValidadores(ctx, empresaID, deptoID)
}

// UsuariosEmpresa lista los usuarios que operan la empresa (para asignar validadores).
func (s *Service) UsuariosEmpresa(ctx context.Context, empresaID string) ([]UsuarioRef, error) {
	return s.repo.UsuariosEmpresa(ctx, empresaID)
}

// AsignarValidador asigna (o reasigna el rol de) un validador a un departamento.
func (s *Service) AsignarValidador(ctx context.Context, empresaID, deptoID, usuarioID, rol, actorID string) error {
	if rol != "TITULAR" && rol != "SUPLENTE" {
		rol = "TITULAR"
	}
	if err := s.repo.AsignarValidador(ctx, empresaID, deptoID, usuarioID, rol); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, deptoID, "ASIGNAR_VALIDADOR", actorID)
	return nil
}

// QuitarValidador desasigna un validador de un departamento.
func (s *Service) QuitarValidador(ctx context.Context, empresaID, deptoID, usuarioID, actorID string) error {
	if err := s.repo.QuitarValidador(ctx, empresaID, deptoID, usuarioID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, deptoID, "QUITAR_VALIDADOR", actorID)
	return nil
}
