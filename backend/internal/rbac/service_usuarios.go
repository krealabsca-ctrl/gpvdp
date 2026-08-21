package rbac

import (
	"context"
	"strings"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/shared"
)

// Usuarios lista los usuarios con acceso a la empresa activa.
func (s *Service) Usuarios(ctx context.Context, empresaID string) ([]UsuarioAdmin, error) {
	return s.repo.ListarUsuarios(ctx, empresaID)
}

// CrearUsuario da de alta un usuario (o vincula uno existente) a la empresa con un rol.
// Nuevo: contraseña temporal fijada por el admin (el usuario la cambia al ingresar).
// Existente (mismo correo): se le da acceso a esta empresa; su contraseña NO se toca.
func (s *Service) CrearUsuario(ctx context.Context, empresaID, nombre, email, password, rolCodigo, actorID string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	id, existe, err := s.repo.UsuarioIDPorEmail(ctx, email)
	if err != nil {
		return false, err
	}
	nuevo := !existe
	if nuevo {
		hash, err := auth.HashPassword(password)
		if err != nil {
			return false, err
		}
		id, err = s.repo.CrearUsuario(ctx, strings.TrimSpace(nombre), email, hash)
		if err != nil {
			return false, err
		}
	}
	if err := s.repo.AsignarRolEmpresa(ctx, empresaID, id, rolCodigo); err != nil {
		return false, err
	}
	s.invalidarEmpresa(empresaID)
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "usuario", EntidadID: &id, Accion: "CREAR_USUARIO", UsuarioID: &actorID,
		ValorNuevo: map[string]any{"email": email, "rol": rolCodigo, "nuevo": nuevo},
	})
	return nuevo, nil
}

// ActualizarUsuario cambia nombre/estado y (opcional) el rol en la empresa activa.
func (s *Service) ActualizarUsuario(ctx context.Context, empresaID, usuarioID, nombre string, activo bool, rolCodigo, actorID string) error {
	if err := s.repo.ActualizarUsuario(ctx, empresaID, usuarioID, strings.TrimSpace(nombre), activo); err != nil {
		return err
	}
	if rolCodigo != "" {
		if err := s.repo.AsignarRolEmpresa(ctx, empresaID, usuarioID, rolCodigo); err != nil {
			return err
		}
	}
	s.invalidarEmpresa(empresaID)
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "usuario", EntidadID: &usuarioID, Accion: "ACTUALIZAR_USUARIO", UsuarioID: &actorID,
		ValorNuevo: map[string]any{"activo": activo, "rol": rolCodigo},
	})
	return nil
}

// ResetPassword fija una nueva contraseña temporal (el usuario la cambia al ingresar).
func (s *Service) ResetPassword(ctx context.Context, empresaID, usuarioID, password, actorID string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.repo.SetPasswordTemporal(ctx, empresaID, usuarioID, hash); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "usuario", EntidadID: &usuarioID, Accion: "RESET_PASSWORD", UsuarioID: &actorID,
	})
	return nil
}

// QuitarAcceso desvincula al usuario de la empresa activa.
func (s *Service) QuitarAcceso(ctx context.Context, empresaID, usuarioID, actorID string) error {
	if err := s.repo.QuitarAcceso(ctx, empresaID, usuarioID); err != nil {
		return err
	}
	s.invalidarEmpresa(empresaID)
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "usuario", EntidadID: &usuarioID, Accion: "QUITAR_ACCESO", UsuarioID: &actorID,
	})
	return nil
}

// AplicarPermisosFaltantes otorga a los roles base los permisos por defecto que falten.
func (s *Service) AplicarPermisosFaltantes(ctx context.Context, empresaID, actorID string) (int, error) {
	n, err := s.repo.AplicarPermisosFaltantes(ctx, empresaID)
	if err != nil {
		return 0, err
	}
	s.invalidarEmpresa(empresaID)
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "rol_permiso", Accion: "APLICAR_PERMISOS_FALTANTES", UsuarioID: &actorID,
		ValorNuevo: map[string]any{"agregados": n},
	})
	return n, nil
}
