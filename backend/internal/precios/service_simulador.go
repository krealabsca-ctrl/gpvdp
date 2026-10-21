package precios

// Orquestación del simulador: junta costeo, indirecto y comisión, y aplica el escenario.

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// OpcionesSimulacion es lo que llega del cliente.
type OpcionesSimulacion struct {
	Fecha     string
	Desde     string
	Hasta     string
	Vendedor  string
	Escenario Escenario
}

// Simular arma los insumos del cálculo y corre el escenario.
//
// Las tres fuentes —receta, indirecto y comisión— se consultan por separado y cada una puede
// faltar sin tumbar la simulación: lo que falta se nombra en `Faltantes` y el resultado se marca
// como no confiable. Abortar ante el primer faltante dejaría al usuario sin la mitad útil del
// número justo cuando más lo necesita.
func (s *Service) Simular(ctx context.Context, empresaID, planID string, op OpcionesSimulacion) (Resultado, error) {
	fecha := op.Fecha
	if fecha == "" {
		fecha = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", fecha); err != nil {
		return Resultado{}, ErrPeriodoInvalido
	}

	costeo, err := s.Costear(ctx, empresaID, planID, fecha)
	if err != nil {
		return Resultado{}, err
	}

	in := InsumosDeSimulacion{
		Plan:           costeo.Plan,
		CosteoCompleto: costeo.EstaCompleto(),
		Faltantes:      costeo.InsumosSinCosto,
	}
	if v, err := decimal.NewFromString(costeo.CostoVariable); err == nil {
		in.CostoVariable = v
	}
	if f, err := decimal.NewFromString(costeo.CostoFijo); err == nil {
		in.CostoFijo = f
	}

	// El indirecto solo se consulta si el escenario lo incluye y hay período.
	if !op.Escenario.SinIndirecto && op.Desde != "" && op.Hasta != "" {
		abs, err := s.AbsorcionDelPlan(ctx, empresaID, planID, op.Desde, op.Hasta)
		if err == nil {
			if t, err := decimal.NewFromString(abs.Total); err == nil {
				in.IndirectoPorPeriodo = &t
				in.IndirectoCompleto = abs.Completo
			}
		}
	}

	if !op.Escenario.SinComision {
		com, err := s.ComisionDelPlan(ctx, empresaID, planID, op.Vendedor, fecha)
		if err == nil && com != nil && com.CostoTotal != nil {
			if t, err := decimal.NewFromString(*com.CostoTotal); err == nil {
				in.Comision = &t
			}
		}
	}

	return Simular(in, op.Escenario)
}
