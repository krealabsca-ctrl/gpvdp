package bancos

// Administración del catálogo: renombrar y eliminar conceptos/clasificaciones
// (p. ej. deshacer una clasificación duplicada por error), con auditoría.

import (
	"context"

	"github.com/gpvdp/erp/internal/shared"
)

// RenombrarConcepto cambia el nombre de un concepto de la empresa.
func (s *Service) RenombrarConcepto(ctx context.Context, empresaID, conceptoID, nombre, usuarioID string) error {
	if err := s.repo.RenombrarConcepto(ctx, empresaID, conceptoID, nombre); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "concepto", EntidadID: &conceptoID,
		Accion: "RENOMBRAR_CONCEPTO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"nombre": nombre},
	})
	return nil
}

// CambiarVisibilidadCxP muestra u oculta un concepto (y sus clasificaciones) en CxP.
func (s *Service) CambiarVisibilidadCxP(ctx context.Context, empresaID, conceptoID string, visible bool, usuarioID string) error {
	if err := s.repo.CambiarVisibilidadCxP(ctx, empresaID, conceptoID, visible); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "concepto", EntidadID: &conceptoID,
		Accion: "CAMBIAR_VISIBILIDAD_CXP", UsuarioID: &usuarioID,
		ValorNuevo: map[string]bool{"visible_cxp": visible},
	})
	return nil
}

// CambiarNaturaleza declara si el concepto es INGRESO, GASTO o NEUTRO para el EBITDA.
//
// Es la decisión que arregla el KPI: antes el dashboard sumaba como ingreso cualquier crédito y como
// gasto cualquier débito, y así el ahorro, las reservas y los aportes entre empresas inflaban el
// número. Queda en auditoría con el valor anterior porque mueve el EBITDA de todos los períodos.
func (s *Service) CambiarNaturaleza(ctx context.Context, empresaID, conceptoID, naturaleza, usuarioID string) error {
	if !NaturalezaValida(naturaleza) {
		return ErrNaturalezaInvalida
	}
	anterior, err := s.repo.CambiarNaturaleza(ctx, empresaID, conceptoID, naturaleza)
	if err != nil {
		return err
	}
	// El ANTES va dentro de ValorNuevo porque el evento no tiene columna aparte para él; lo que
	// importa es que quede registrado: mover un concepto de NEUTRO a GASTO cambia el EBITDA de
	// todos los períodos y sin el valor viejo no se puede explicar por qué el número cambió.
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "concepto", EntidadID: &conceptoID,
		Accion: "CAMBIAR_NATURALEZA", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"naturaleza": naturaleza, "naturaleza_anterior": anterior},
	})
	return nil
}

// EliminarConcepto borra un concepto SIN referencias (en uso → CatalogoEnUsoError).
func (s *Service) EliminarConcepto(ctx context.Context, empresaID, conceptoID, usuarioID string) error {
	if err := s.repo.EliminarConcepto(ctx, empresaID, conceptoID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "concepto", EntidadID: &conceptoID,
		Accion: "ELIMINAR_CONCEPTO", UsuarioID: &usuarioID,
	})
	return nil
}

// RenombrarClasificacion cambia el nombre de una clasificación de la empresa.
func (s *Service) RenombrarClasificacion(ctx context.Context, empresaID, clasificacionID, nombre, usuarioID string) error {
	if err := s.repo.RenombrarClasificacion(ctx, empresaID, clasificacionID, nombre); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "clasificacion", EntidadID: &clasificacionID,
		Accion: "RENOMBRAR_CLASIFICACION", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"nombre": nombre},
	})
	return nil
}

// ReasignarConceptoClasificacion mueve una clasificación (sin uso) a otro concepto.
func (s *Service) ReasignarConceptoClasificacion(ctx context.Context, empresaID, clasificacionID, nuevoConceptoID, usuarioID string) error {
	if err := s.repo.ReasignarConceptoClasificacion(ctx, empresaID, clasificacionID, nuevoConceptoID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "clasificacion", EntidadID: &clasificacionID,
		Accion: "REASIGNAR_CONCEPTO_CLASIFICACION", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"concepto_id": nuevoConceptoID},
	})
	return nil
}

// EliminarClasificacion borra una clasificación SIN referencias (en uso → CatalogoEnUsoError).
func (s *Service) EliminarClasificacion(ctx context.Context, empresaID, clasificacionID, usuarioID string) error {
	if err := s.repo.EliminarClasificacion(ctx, empresaID, clasificacionID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "clasificacion", EntidadID: &clasificacionID,
		Accion: "ELIMINAR_CLASIFICACION", UsuarioID: &usuarioID,
	})
	return nil
}
