package cxp

import (
	"context"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// PermisoChecker resuelve si (empresa, rol) tiene un permiso. Lo implementa rbac.Service;
// se define aquí como interfaz local para no acoplar CxP al paquete rbac.
type PermisoChecker interface {
	Tiene(ctx context.Context, empresaID, rolCodigo, permiso string) (bool, error)
}

// permisoVerTodo: quien lo tiene ve las facturas de TODOS los departamentos; sin él, el usuario
// solo ve las de su(s) área(s) (scoping del validador). Debe existir en el catálogo RBAC.
const permisoVerTodo = "cxp.ver_todo"

// permisoCarteraAbierta: la cartera abierta es la DEUDA TOTAL de la empresa (cruza fases y
// departamentos), no una fase del flujo. Se pide permiso aparte para que no la vea todo el mundo.
const permisoCarteraAbierta = "cxp.cartera_abierta"

// permisoAprobarContabilidad: aprobar las facturas marcadas «de Contabilidad», que se saltan la
// validación de área. Es un permiso APARTE de cxp.aprobar porque autoriza una excepción al flujo,
// no un monto más alto. Debe existir en el catálogo RBAC.
const permisoAprobarContabilidad = "cxp.aprobar_contabilidad"

// permisoAprobar es la aprobación financiera general. Quien la tiene también puede firmar una
// factura «de Contabilidad»: el permiso propio ABRE la puerta a quien no tiene este, no la cierra
// a quien sí lo tiene.
const permisoAprobar = "cxp.aprobar"

// Service orquesta la lógica de CxP (por ahora, maestro de proveedores).
type Service struct {
	repo   Repository
	audit  *shared.Audit
	log    *zap.Logger
	mailer *Mailer        // opcional; si es nil, el envío de comprobantes falla con error claro
	perms  PermisoChecker // opcional; si es nil, no hay scoping por área (ve todo)
	// plantillas: opcional; si es nil, las notificaciones salen con el texto de fábrica.
	plantillas Plantillero
}

// NewService construye el servicio de CxP.
func NewService(repo Repository, audit *shared.Audit, log *zap.Logger) *Service {
	return &Service{repo: repo, audit: audit, log: log}
}

// SetMailer inyecta el mailer para el envío de comprobantes (se configura en el arranque).
func (s *Service) SetMailer(m *Mailer) { s.mailer = m }

// SetPermisos inyecta el verificador RBAC para el scoping por área (se configura en el arranque).
func (s *Service) SetPermisos(p PermisoChecker) { s.perms = p }

// departamentosVisibles decide el alcance de datos del usuario en la Bandeja:
//   - nil  => ve TODAS las áreas (tiene cxp.ver_todo, o no hay checker configurado);
//   - no-nil (posiblemente vacío) => solo esos departamentos (validador de área).
func (s *Service) departamentosVisibles(ctx context.Context, empresaID, rol, usuarioID string) ([]string, error) {
	if s.perms == nil {
		return nil, nil // sin checker (p. ej. tests): sin scoping
	}
	verTodo, err := s.perms.Tiene(ctx, empresaID, rol, permisoVerTodo)
	if err != nil {
		return nil, err
	}
	if verTodo {
		return nil, nil
	}
	return s.repo.DepartamentosDeUsuario(ctx, empresaID, usuarioID)
}

// Crear registra un proveedor.
func (s *Service) Crear(ctx context.Context, empresaID string, in ProveedorInput, usuarioID string) (Proveedor, error) {
	p, err := s.repo.Crear(ctx, empresaID, in)
	if err != nil {
		return Proveedor{}, err
	}
	s.auditar(ctx, empresaID, p.ID, "CREAR_PROVEEDOR", usuarioID)
	return p, nil
}

// Listar devuelve los proveedores de la empresa (filtrable por texto y criterios de la tabla).
func (s *Service) Listar(ctx context.Context, empresaID string, f FiltrosProveedor, page, pageSize int) (ListaProveedores, error) {
	return s.repo.Listar(ctx, empresaID, f, page, pageSize)
}

// PorID devuelve un proveedor.
func (s *Service) PorID(ctx context.Context, empresaID, id string) (Proveedor, error) {
	return s.repo.PorID(ctx, empresaID, id)
}

// Actualizar modifica un proveedor.
func (s *Service) Actualizar(ctx context.Context, empresaID, id string, in ProveedorInput, usuarioID string) (Proveedor, error) {
	p, err := s.repo.Actualizar(ctx, empresaID, id, in)
	if err != nil {
		return Proveedor{}, err
	}
	s.auditar(ctx, empresaID, id, "ACTUALIZAR_PROVEEDOR", usuarioID)
	return p, nil
}

// Desactivar da de baja lógica a un proveedor.
func (s *Service) Desactivar(ctx context.Context, empresaID, id, usuarioID string) error {
	if err := s.repo.Desactivar(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, id, "DESACTIVAR_PROVEEDOR", usuarioID)
	return nil
}

func (s *Service) auditar(ctx context.Context, empresaID, id, accion, usuarioID string) {
	s.auditarEntidad(ctx, empresaID, "proveedor", id, accion, usuarioID)
}

// auditarEntidad registra el evento diciendo SOBRE QUÉ es. `auditar` da por sentado que la entidad
// es un proveedor, y con eso una marca puesta sobre un concepto o una clasificación quedaba
// archivada como si fuera de un proveedor: el histórico de esa entidad no la encontraba.
func (s *Service) auditarEntidad(ctx context.Context, empresaID, entidad, id, accion, usuarioID string) {
	if s.audit == nil {
		return
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: entidad, EntidadID: &id, Accion: accion, UsuarioID: &usuarioID,
	})
}
