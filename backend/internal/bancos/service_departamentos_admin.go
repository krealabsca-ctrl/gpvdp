package bancos

// Administrar el catálogo de departamentos y sedes desde la pantalla donde se usan.
//
// ── La regla de borrado, que es la única decisión de peso acá ─────────────────
//
// Un departamento se borra FÍSICAMENTE solo si no cuelga nada de él. Si cuelga algo —una factura, un
// empleado, un movimiento ya atribuido— se DESACTIVA: deja de ofrecerse para elegir, pero la historia
// sigue diciendo a quién se le cobró ese gasto. Es la misma regla que ya rige el catálogo de
// conceptos y clasificaciones.
//
// El error de "en uso" nombra QUÉ cuelga y cuánto. Un «no se puede eliminar» sin decir por qué obliga
// a adivinar, y lo que se adivina normalmente es que el sistema está roto.

import (
	"context"
	"strings"

	"github.com/gpvdp/erp/internal/shared"
)

// CrearDepartamento agrega un departamento al catálogo de la empresa.
func (s *Service) CrearDepartamento(ctx context.Context, empresaID, nombre, codigo, usuarioID string) (Departamento, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return Departamento{}, ErrNombreRequerido
	}
	d, err := s.repo.CrearDepartamento(ctx, empresaID, nombre, strings.TrimSpace(codigo))
	if err != nil {
		return Departamento{}, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "departamento", EntidadID: &d.ID,
		Accion: "CREAR_DEPARTAMENTO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"nombre": nombre, "codigo": codigo},
	})
	return d, nil
}

// ActualizarDepartamento renombra un departamento.
func (s *Service) ActualizarDepartamento(ctx context.Context, empresaID, deptoID, nombre, codigo, usuarioID string) error {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return ErrNombreRequerido
	}
	if err := s.repo.ActualizarDepartamento(ctx, empresaID, deptoID, nombre, strings.TrimSpace(codigo)); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "departamento", EntidadID: &deptoID,
		Accion: "ACTUALIZAR_DEPARTAMENTO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"nombre": nombre, "codigo": codigo},
	})
	return nil
}

// CambiarActivoDepartamento da de baja o revive un departamento sin perder su historia.
func (s *Service) CambiarActivoDepartamento(ctx context.Context, empresaID, deptoID string, activo bool, usuarioID string) error {
	if err := s.repo.CambiarActivoDepartamento(ctx, empresaID, deptoID, activo); err != nil {
		return err
	}
	accion := "DESACTIVAR_DEPARTAMENTO"
	if activo {
		accion = "ACTIVAR_DEPARTAMENTO"
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "departamento", EntidadID: &deptoID,
		Accion: accion, UsuarioID: &usuarioID,
	})
	return nil
}

// UsoDeDepartamento dice de qué cuelga un departamento, para poder mostrarlo ANTES de intentar
// borrarlo: es la diferencia entre «esto se puede eliminar» y «esto solo se puede desactivar».
func (s *Service) UsoDeDepartamento(ctx context.Context, empresaID, deptoID string) (UsoDepartamento, error) {
	return s.repo.UsoDeDepartamento(ctx, empresaID, deptoID)
}

// EliminarDepartamento borra un departamento del que no cuelga nada. Si cuelga algo devuelve el
// mismo CatalogoEnUsoError que ya usan conceptos y clasificaciones —con el detalle de qué cuelga—,
// y el camino correcto es desactivarlo.
func (s *Service) EliminarDepartamento(ctx context.Context, empresaID, deptoID, usuarioID string) error {
	uso, err := s.repo.UsoDeDepartamento(ctx, empresaID, deptoID)
	if err != nil {
		return err
	}
	if uso.Total() > 0 {
		return &CatalogoEnUsoError{Detalle: uso.Detalle() + " — desactivalo en vez de eliminarlo"}
	}
	if err := s.repo.EliminarDepartamento(ctx, empresaID, deptoID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "departamento", EntidadID: &deptoID,
		Accion: "ELIMINAR_DEPARTAMENTO", UsuarioID: &usuarioID,
	})
	return nil
}

// EliminarSede es lo mismo para una sede.
func (s *Service) EliminarSede(ctx context.Context, empresaID, sedeID, usuarioID string) error {
	uso, err := s.repo.UsoDeSede(ctx, empresaID, sedeID)
	if err != nil {
		return err
	}
	if uso.Total() > 0 {
		return &CatalogoEnUsoError{Detalle: uso.Detalle() + " — desactivala en vez de eliminarla"}
	}
	if err := s.repo.EliminarSede(ctx, empresaID, sedeID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "sede", EntidadID: &sedeID,
		Accion: "ELIMINAR_SEDE", UsuarioID: &usuarioID,
	})
	return nil
}

// UsoDeSede dice de qué cuelga una sede.
func (s *Service) UsoDeSede(ctx context.Context, empresaID, sedeID string) (UsoDepartamento, error) {
	return s.repo.UsoDeSede(ctx, empresaID, sedeID)
}
