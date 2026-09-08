package rbac

import (
	"context"

	"github.com/gpvdp/erp/internal/shared"
)

// RolesTraibles lista los roles a medida de las otras empresas del usuario que se pueden traer acá.
func (s *Service) RolesTraibles(ctx context.Context, empresaID, actorID string) ([]RolTraible, error) {
	return s.repo.RolesTraibles(ctx, empresaID, actorID)
}

// TraerRol copia a la empresa activa un rol a medida de otra empresa, con sus permisos.
//
// Queda en auditoría con la empresa de origen y la cantidad de permisos copiados: un rol nuevo con
// permisos que nadie marcó a mano tiene que poder explicarse después.
func (s *Service) TraerRol(ctx context.Context, empresaID, empresaOrigen, codigo, usuarioID string) (RolItem, int, error) {
	it, copiados, err := s.repo.TraerRol(ctx, empresaID, empresaOrigen, codigo, usuarioID)
	if err != nil {
		return RolItem{}, 0, err
	}
	s.invalidarEmpresa(empresaID)
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "rol", EntidadID: &it.ID,
		Accion: "TRAER_ROL_DE_OTRA_EMPRESA", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"codigo": codigo, "nombre": it.Nombre,
			"empresa_origen_id": empresaOrigen, "permisos_copiados": copiados,
		},
	})
	return it, copiados, nil
}
