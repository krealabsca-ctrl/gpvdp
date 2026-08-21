package bancos

// Servicio de parámetros de negocio por empresa (Fase D).

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// Parametros son los ajustes de negocio de la empresa activa.
type Parametros struct {
	// ToleranciaTraslado como proporción (0.01 = 1%) y como porcentaje para la UI.
	ToleranciaTraslado    string `json:"tolerancia_traslado"`
	ToleranciaTrasladoPct string `json:"tolerancia_traslado_pct"`
	// CierreBloqueante es GLOBAL (env) hoy; se expone de solo lectura para contexto.
	CierreBloqueante bool `json:"cierre_bloqueante"`
}

// Parametros devuelve los ajustes de la empresa activa.
func (s *Service) Parametros(ctx context.Context, empresaID string) (Parametros, error) {
	pct, err := s.repo.ToleranciaTraslado(ctx, empresaID)
	if err != nil {
		return Parametros{}, err
	}
	return Parametros{
		ToleranciaTraslado:    pct.String(),
		ToleranciaTrasladoPct: pct.Mul(decimal.NewFromInt(100)).String(),
		CierreBloqueante:      s.cierreBloqueante,
	}, nil
}

// ActualizarTolerancia fija la tolerancia de traslado (recibe proporción, p. ej. 0.015 = 1.5%).
// Rango válido 0–0.05 (0–5%; 0 = emparejamiento exacto). Se redondea a 4 decimales de
// proporción (2 de porcentaje) ANTES de validar/persistir/auditar, para que la base,
// la auditoría y la API devuelvan exactamente el mismo valor (la columna es numeric(6,4)).
func (s *Service) ActualizarTolerancia(ctx context.Context, empresaID string, pct decimal.Decimal, usuarioID string) error {
	pct = pct.Round(4)
	if pct.IsNegative() || pct.GreaterThan(decimal.NewFromFloat(0.05)) {
		return ErrToleranciaFueraDeRango
	}
	if err := s.repo.ActualizarTolerancia(ctx, empresaID, pct); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "empresa", EntidadID: &empresaID,
		Accion: "ACTUALIZAR_TOLERANCIA_TRASLADO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"tolerancia_traslado": pct.String()},
	})
	return nil
}
