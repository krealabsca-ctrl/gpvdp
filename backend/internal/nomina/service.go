package nomina

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// Service orquesta RRHH / Nómina (Etapa 1: empleados, parámetros, conceptos y deducciones).
type Service struct {
	repo  Repository
	audit *shared.Audit
	log   *zap.Logger
	// Notificaciones (boleta y vacaciones): opcionales. Sin ellas, los endpoints de envío
	// responden que no está configurado en vez de fallar de forma rara.
	plantillas Plantillero
	correo     Correo
}

// NewService construye el servicio de nómina.
func NewService(repo Repository, audit *shared.Audit, log *zap.Logger) *Service {
	return &Service{repo: repo, audit: audit, log: log}
}

// EnsureConceptos siembra el catálogo base de conceptos en todas las empresas (arranque).
func (s *Service) EnsureConceptos(ctx context.Context) error { return s.repo.EnsureConceptos(ctx) }

// ---- Empleados ----

// ListarEmpleados devuelve los empleados de la empresa (filtrables por texto y estado).
func (s *Service) ListarEmpleados(ctx context.Context, empresaID string, f FiltrosEmpleado) ([]Empleado, error) {
	return s.repo.ListarEmpleados(ctx, empresaID, f)
}

// EmpleadoPorID devuelve la ficha de un empleado.
func (s *Service) EmpleadoPorID(ctx context.Context, empresaID, id string) (Empleado, error) {
	return s.repo.EmpleadoPorID(ctx, empresaID, id)
}

// CrearEmpleado registra un empleado. El salario base debe ser positivo: sin él no hay
// nómina calculable ni base contributiva CCSS.
func (s *Service) CrearEmpleado(ctx context.Context, empresaID string, in EmpleadoInput, usuarioID string) (Empleado, error) {
	if !in.SalarioBase.IsPositive() {
		return Empleado{}, ErrSalarioInvalido
	}
	e, err := s.repo.CrearEmpleado(ctx, empresaID, in)
	if err != nil {
		return Empleado{}, err
	}
	s.auditar(ctx, empresaID, "empleado", e.ID, "CREAR_EMPLEADO", usuarioID, nil)
	return e, nil
}

// ActualizarEmpleado modifica la ficha. El cambio de salario queda en auditoría.
func (s *Service) ActualizarEmpleado(ctx context.Context, empresaID, id string, in EmpleadoInput, usuarioID string) (Empleado, error) {
	if !in.SalarioBase.IsPositive() {
		return Empleado{}, ErrSalarioInvalido
	}
	antes, err := s.repo.EmpleadoPorID(ctx, empresaID, id)
	if err != nil {
		return Empleado{}, err
	}
	e, err := s.repo.ActualizarEmpleado(ctx, empresaID, id, in)
	if err != nil {
		return Empleado{}, err
	}
	if antes.SalarioBase != e.SalarioBase {
		s.auditar(ctx, empresaID, "empleado", id, "CAMBIAR_SALARIO", usuarioID,
			map[string]string{"anterior": antes.SalarioBase, "nuevo": e.SalarioBase})
	} else {
		s.auditar(ctx, empresaID, "empleado", id, "ACTUALIZAR_EMPLEADO", usuarioID, nil)
	}
	return e, nil
}

// DesactivarEmpleado da de baja lógica (fija fecha de salida; nunca borra).
func (s *Service) DesactivarEmpleado(ctx context.Context, empresaID, id, fechaSalida, usuarioID string) error {
	if err := s.repo.DesactivarEmpleado(ctx, empresaID, id, fechaSalida); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "empleado", id, "DESACTIVAR_EMPLEADO", usuarioID, nil)
	return nil
}

// ---- Parámetros ----

// Parametros devuelve los parámetros del año: los guardados por la empresa, o los legales
// de referencia CR 2026 (Origen DEFAULT) si aún no se han guardado.
func (s *Service) Parametros(ctx context.Context, empresaID string, anio int) (Parametros, error) {
	p, err := s.repo.ParametrosPorAnio(ctx, empresaID, anio)
	if errors.Is(err, ErrParametrosNoEncontrados) {
		return ParametrosDefault2026(anio), nil
	}
	return p, err
}

// GuardarParametros valida y guarda (upsert) los parámetros del año.
func (s *Service) GuardarParametros(ctx context.Context, empresaID string, anio int, in ParametrosInput, usuarioID string) (Parametros, error) {
	if err := validarParametros(in); err != nil {
		return Parametros{}, err
	}
	p, err := s.repo.GuardarParametros(ctx, empresaID, anio, in)
	if err != nil {
		return Parametros{}, err
	}
	s.auditar(ctx, empresaID, "nomina_parametros", p.ID, "GUARDAR_PARAMETROS", usuarioID,
		map[string]any{"anio": anio, "cargas": in.Cargas, "renta": in.Renta})
	return p, nil
}

var cien = decimal.NewFromInt(100)

func validarParametros(in ParametrosInput) error {
	var obreras, patronales int
	for _, c := range in.Cargas {
		pct, err := decimal.NewFromString(c.Pct)
		if err != nil || c.Codigo == "" || c.Nombre == "" || pct.IsNegative() || pct.GreaterThan(cien) {
			return ErrCargaInvalida
		}
		switch c.Tipo {
		case CargaObrero:
			obreras++
		case CargaPatronal:
			patronales++
		default:
			return ErrCargaInvalida
		}
	}
	if obreras == 0 || patronales == 0 {
		return ErrCargasIncompletas
	}
	if len(in.Renta.Tramos) == 0 {
		return ErrTramosInvalidos
	}
	anterior := decimal.Zero
	for i, t := range in.Renta.Tramos {
		pct, err := decimal.NewFromString(t.Pct)
		if err != nil || pct.IsNegative() || pct.GreaterThan(cien) {
			return ErrTramosInvalidos
		}
		if t.Hasta == nil {
			// Solo el último tramo puede (y debe) ser abierto.
			if i != len(in.Renta.Tramos)-1 {
				return ErrTramosInvalidos
			}
			continue
		}
		hasta, err := decimal.NewFromString(*t.Hasta)
		if err != nil || !hasta.GreaterThan(anterior) {
			return ErrTramosInvalidos
		}
		anterior = hasta
	}
	if in.Renta.Tramos[len(in.Renta.Tramos)-1].Hasta != nil {
		return ErrTramosInvalidos
	}
	// Los créditos familiares se validan al GUARDAR (no solo al calcular): un texto sin
	// parsear aquí bloquearía la corrida semanas después con un error confuso.
	for _, credito := range []string{in.Renta.CreditoHijo, in.Renta.CreditoConyuge} {
		v, err := decimal.NewFromString(credito)
		if err != nil || v.IsNegative() {
			return ErrTramosInvalidos
		}
	}
	if in.AdelantoPct.IsNegative() || in.AdelantoPct.GreaterThan(cien) {
		return ErrCargaInvalida
	}
	for _, pct := range []decimal.Decimal{in.AguinaldoPct, in.VacacionesPct, in.CesantiaPct} {
		if pct.IsNegative() || pct.GreaterThan(cien) {
			return ErrCargaInvalida
		}
	}
	return nil
}

// ---- Conceptos (guardarraíl CCSS) ----

// ListarConceptos devuelve el catálogo de conceptos de la empresa.
func (s *Service) ListarConceptos(ctx context.Context, empresaID string) ([]ConceptoNomina, error) {
	return s.repo.ListarConceptos(ctx, empresaID)
}

// CrearConcepto agrega un concepto de empresa (nunca de sistema). GUARDARRAÍL: un INGRESO
// no afecto a CCSS exige base legal — lo salarial no se puede disfrazar de no salarial.
func (s *Service) CrearConcepto(ctx context.Context, empresaID string, in ConceptoInput, usuarioID string) (ConceptoNomina, error) {
	if err := validarConcepto(in); err != nil {
		return ConceptoNomina{}, err
	}
	c, err := s.repo.CrearConcepto(ctx, empresaID, in)
	if err != nil {
		return ConceptoNomina{}, err
	}
	s.auditar(ctx, empresaID, "concepto_nomina", c.ID, "CREAR_CONCEPTO", usuarioID, in)
	return c, nil
}

// ActualizarConcepto modifica un concepto de empresa. GUARDARRAÍL: los de sistema
// (salario, extras, comisiones, bonos habituales…) están bloqueados por ley.
func (s *Service) ActualizarConcepto(ctx context.Context, empresaID, id string, in ConceptoInput, usuarioID string) (ConceptoNomina, error) {
	actual, err := s.repo.ConceptoPorID(ctx, empresaID, id)
	if err != nil {
		return ConceptoNomina{}, err
	}
	if actual.DeSistema {
		return ConceptoNomina{}, ErrConceptoDeSistema
	}
	if err := validarConcepto(in); err != nil {
		return ConceptoNomina{}, err
	}
	c, err := s.repo.ActualizarConcepto(ctx, empresaID, id, in)
	if err != nil {
		return ConceptoNomina{}, err
	}
	s.auditar(ctx, empresaID, "concepto_nomina", id, "ACTUALIZAR_CONCEPTO", usuarioID, in)
	return c, nil
}

// DesactivarConcepto da de baja lógica un concepto de empresa (los de sistema, jamás).
func (s *Service) DesactivarConcepto(ctx context.Context, empresaID, id, usuarioID string) error {
	actual, err := s.repo.ConceptoPorID(ctx, empresaID, id)
	if err != nil {
		return err
	}
	if actual.DeSistema {
		return ErrConceptoDeSistema
	}
	if err := s.repo.DesactivarConcepto(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "concepto_nomina", id, "DESACTIVAR_CONCEPTO", usuarioID, nil)
	return nil
}

func validarConcepto(in ConceptoInput) error {
	if in.Tipo == ConceptoIngreso && !in.AfectaCCSS && in.BaseLegal == "" {
		return ErrBaseLegalRequerida
	}
	return nil
}

// ---- Deducciones recurrentes ----

// ListarDeducciones devuelve las deducciones recurrentes del empleado.
func (s *Service) ListarDeducciones(ctx context.Context, empresaID, empleadoID string) ([]DeduccionEmpleado, error) {
	if _, err := s.repo.EmpleadoPorID(ctx, empresaID, empleadoID); err != nil {
		return nil, err
	}
	return s.repo.ListarDeducciones(ctx, empresaID, empleadoID)
}

// CrearDeduccion registra una deducción recurrente (cuelga de un concepto DEDUCCION activo).
func (s *Service) CrearDeduccion(ctx context.Context, empresaID, empleadoID string, in DeduccionInput, usuarioID string) (DeduccionEmpleado, error) {
	if err := s.validarDeduccion(ctx, empresaID, in); err != nil {
		return DeduccionEmpleado{}, err
	}
	d, err := s.repo.CrearDeduccion(ctx, empresaID, empleadoID, in)
	if err != nil {
		return DeduccionEmpleado{}, err
	}
	s.auditar(ctx, empresaID, "deduccion_empleado", d.ID, "CREAR_DEDUCCION", usuarioID,
		map[string]string{"empleado_id": empleadoID, "etiqueta": in.Etiqueta, "cuota": in.Cuota.String()})
	return d, nil
}

// ActualizarDeduccion modifica etiqueta, cuota, prioridad y saldo total.
func (s *Service) ActualizarDeduccion(ctx context.Context, empresaID, empleadoID, id string, in DeduccionInput, usuarioID string) (DeduccionEmpleado, error) {
	if in.Etiqueta == "" || !in.Cuota.IsPositive() || (in.SaldoTotal != nil && !in.SaldoTotal.IsPositive()) {
		return DeduccionEmpleado{}, ErrDeduccionInvalida
	}
	d, err := s.repo.ActualizarDeduccion(ctx, empresaID, empleadoID, id, in)
	if err != nil {
		return DeduccionEmpleado{}, err
	}
	s.auditar(ctx, empresaID, "deduccion_empleado", id, "ACTUALIZAR_DEDUCCION", usuarioID, nil)
	return d, nil
}

// DesactivarDeduccion da de baja lógica una deducción recurrente.
func (s *Service) DesactivarDeduccion(ctx context.Context, empresaID, empleadoID, id, usuarioID string) error {
	if err := s.repo.DesactivarDeduccion(ctx, empresaID, empleadoID, id); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "deduccion_empleado", id, "DESACTIVAR_DEDUCCION", usuarioID, nil)
	return nil
}

func (s *Service) validarDeduccion(ctx context.Context, empresaID string, in DeduccionInput) error {
	if in.Etiqueta == "" || !in.Cuota.IsPositive() || (in.SaldoTotal != nil && !in.SaldoTotal.IsPositive()) {
		return ErrDeduccionInvalida
	}
	c, err := s.repo.ConceptoPorID(ctx, empresaID, in.ConceptoID)
	if err != nil {
		return err
	}
	if c.Tipo != ConceptoDeduccion || !c.Activo {
		return ErrConceptoNoEsDeduccion
	}
	return nil
}

func (s *Service) auditar(ctx context.Context, empresaID, entidad, id, accion, usuarioID string, valor any) {
	if s.audit == nil {
		return
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: entidad, EntidadID: &id, Accion: accion,
		UsuarioID: &usuarioID, ValorNuevo: valor,
	})
}
