package precios

// Orquestación de las comisiones.

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// FuenteDeCargasSociales es el puerto hacia Nómina.
//
// `precios` declara la interfaz y `main.go` la satisface con el paquete `nomina`, igual que
// Inventario declara su facturador y se satisface con CxP. Si la conexión no estuviera, el
// módulo sigue funcionando: muestra la comisión bruta y dice que no pudo calcular el costo
// patronal, en vez de inventar un factor o fingir que la comisión no tiene cargas.
type FuenteDeCargasSociales interface {
	// PorcentajePatronal devuelve el porcentaje de cargas patronales del año (26.83 para CR
	// 2026), ya sumadas: CCSS, INS, Banco Popular, Asignaciones Familiares, IMAS e INA.
	PorcentajePatronal(ctx context.Context, empresaID string, anio int) (decimal.Decimal, error)
}

// RepositorioComisiones es lo que el servicio necesita.
type RepositorioComisiones interface {
	Politicas(ctx context.Context, empresaID string) ([]PoliticaComision, error)
	CrearPolitica(ctx context.Context, empresaID string, in PoliticaInput, usuarioID string) (string, error)
	ActualizarPolitica(ctx context.Context, empresaID, id string, in PoliticaInput) error
	EliminarPolitica(ctx context.Context, empresaID, id string) error
	Vendedores(ctx context.Context, empresaID string) ([]Vendedor, error)
}

// SetCargasSociales conecta el servicio con nómina. Sin esta llamada el módulo funciona igual,
// solo que no puede completar el costo patronal de una comisión.
func (s *Service) SetCargasSociales(f FuenteDeCargasSociales) { s.cargas = f }

// Politicas lista las políticas de comisión.
func (s *Service) Politicas(ctx context.Context, empresaID string) ([]PoliticaComision, error) {
	return s.repo.Politicas(ctx, empresaID)
}

// Vendedores lista los empleados que pueden tener política propia.
func (s *Service) Vendedores(ctx context.Context, empresaID string) ([]Vendedor, error) {
	return s.repo.Vendedores(ctx, empresaID)
}

// CrearPolitica valida y crea.
func (s *Service) CrearPolitica(ctx context.Context, empresaID string, in PoliticaInput, usuarioID string) (string, error) {
	if err := in.Normalizar(); err != nil {
		return "", err
	}
	return s.repo.CrearPolitica(ctx, empresaID, in, usuarioID)
}

// ActualizarPolitica valida y actualiza.
func (s *Service) ActualizarPolitica(ctx context.Context, empresaID, id string, in PoliticaInput) error {
	if err := in.Normalizar(); err != nil {
		return err
	}
	return s.repo.ActualizarPolitica(ctx, empresaID, id, in)
}

// EliminarPolitica borra una política.
func (s *Service) EliminarPolitica(ctx context.Context, empresaID, id string) error {
	return s.repo.EliminarPolitica(ctx, empresaID, id)
}

// factorPatronal pide a nómina el porcentaje del año. nil cuando no hay conexión o falla: el
// llamador muestra el bruto y dice que falta, en vez de suponer un factor.
func (s *Service) factorPatronal(ctx context.Context, empresaID string, fecha time.Time) *decimal.Decimal {
	if s.cargas == nil {
		return nil
	}
	pct, err := s.cargas.PorcentajePatronal(ctx, empresaID, fecha.Year())
	if err != nil || pct.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	return &pct
}

// ComisionDelPlan resuelve qué política aplica y cuánto cuesta la comisión de ese plan.
//
// `vendedorID` puede ir vacío: entonces resuelve con las políticas que no son de nadie en
// particular, que es lo que hay que mirar para costear un plan antes de saber quién lo venderá.
func (s *Service) ComisionDelPlan(ctx context.Context, empresaID, planID, vendedorID, fecha string) (*CostoDeComision, error) {
	f := time.Now()
	if fecha != "" {
		p, err := time.Parse("2006-01-02", fecha)
		if err != nil {
			return nil, ErrPeriodoInvalido
		}
		f = p
	}

	politicas, err := s.repo.Politicas(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	pol := Resolver(politicas, planID, vendedorID, f)
	if pol == nil {
		// Sin política NO se devuelve cero: eso diría que la venta no paga comisión, y el margen
		// saldría más alto que el real. Se devuelve nil y el llamador lo informa como faltante.
		return nil, nil
	}

	base, err := s.baseDeComision(ctx, empresaID, planID, pol.Base, f)
	if err != nil {
		return nil, err
	}
	c, err := CalcularComision(*pol, base, s.factorPatronal(ctx, empresaID, f))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// baseDeComision calcula sobre qué monto se aplica la política.
func (s *Service) baseDeComision(ctx context.Context, empresaID, planID, base string, f time.Time) (decimal.Decimal, error) {
	plan, err := s.repo.PlanPorID(ctx, empresaID, planID)
	if err != nil {
		return decimal.Zero, err
	}

	switch base {
	case BasePrimeraCuota:
		// En suscripción, comisionar sobre el total del contrato paga por adelantado 36 meses
		// que quizá no lleguen. La primera cuota es lo que de verdad entró.
		if plan.CuotaMensual != nil {
			return decimal.NewFromString(*plan.CuotaMensual)
		}
		if plan.PrecioVenta != nil {
			return decimal.NewFromString(*plan.PrecioVenta)
		}
		return decimal.Zero, nil

	case BaseMargen:
		// Sobre la contribución marginal: alinea al vendedor con la rentabilidad y no con el
		// volumen. Si el costeo está incompleto no hay margen confiable, y comisionar sobre un
		// margen inflado pagaría de más.
		costeo, err := s.Costear(ctx, empresaID, planID, f.Format("2006-01-02"))
		if err != nil {
			return decimal.Zero, err
		}
		if costeo.ContribucionMarginal == nil {
			return decimal.Zero, nil
		}
		return decimal.NewFromString(*costeo.ContribucionMarginal)

	default: // BasePrecio
		if ref := precioDeReferencia(plan); ref != nil {
			return *ref, nil
		}
		return decimal.Zero, nil
	}
}
