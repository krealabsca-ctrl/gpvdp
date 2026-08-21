package cxp

import "context"

// Departamentos lista los departamentos de la empresa (todos o solo activos).
func (s *Service) Departamentos(ctx context.Context, empresaID string, soloActivos bool) ([]Departamento, error) {
	return s.repo.ListarDepartamentos(ctx, empresaID, soloActivos)
}

// CrearDepartamento da de alta un departamento.
func (s *Service) CrearDepartamento(ctx context.Context, empresaID string, in DepartamentoInput, usuarioID string) (Departamento, error) {
	d, err := s.repo.CrearDepartamento(ctx, empresaID, in)
	if err != nil {
		return Departamento{}, err
	}
	s.auditar(ctx, empresaID, d.ID, "CREAR_DEPARTAMENTO", usuarioID)
	return d, nil
}

// ActualizarDepartamento modifica un departamento.
func (s *Service) ActualizarDepartamento(ctx context.Context, empresaID, id string, in DepartamentoInput, usuarioID string) (Departamento, error) {
	d, err := s.repo.ActualizarDepartamento(ctx, empresaID, id, in)
	if err != nil {
		return Departamento{}, err
	}
	s.auditar(ctx, empresaID, id, "ACTUALIZAR_DEPARTAMENTO", usuarioID)
	return d, nil
}

// DesactivarDepartamento da de baja lógica a un departamento.
func (s *Service) DesactivarDepartamento(ctx context.Context, empresaID, id, usuarioID string) error {
	if err := s.repo.DesactivarDepartamento(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, id, "DESACTIVAR_DEPARTAMENTO", usuarioID)
	return nil
}

// EnsureDepartamentos siembra el set base de departamentos por empresa (idempotente).
func (s *Service) EnsureDepartamentos(ctx context.Context) error {
	return s.repo.EnsureDepartamentos(ctx)
}
