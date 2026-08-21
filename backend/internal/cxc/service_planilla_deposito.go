package cxc

// Planillas de asociación: conciliar el detalle que manda la asociación contra el depósito
// que de verdad entró al banco.
//
// El flujo real del negocio, tal como lo describió el usuario: la asociación deduce de la
// planilla del trabajador, deposita, y manda un correo con el comprobante bancario. El monto
// NO se le pregunta a nadie: ya está en Bancos. Lo único que hace falta es decir CUÁL de los
// créditos del mes es el de esta asociación, porque la descripción del banco casi nunca lo
// dice.

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"
)

// margenDiasDeposito: cuántos días después del cierre del mes se siguen ofreciendo créditos
// como candidatos. La plata de una planilla de julio suele entrar en los primeros días de
// agosto (el dato real traía depósitos el 8 y el 11 del mes siguiente).
const margenDiasDeposito = 20

// toleranciaPlanilla lee PLANILLA_TOLERANCIA. Arranca en CERO a propósito: no se inventa una
// tolerancia. Si las asociaciones depositan neto de comisión, la diferencia aparece con su
// monto exacto en pantalla y ahí se decide cuánto tolerar.
func (s *Service) toleranciaPlanilla(ctx context.Context, empresaID string) decimal.Decimal {
	p, err := s.repo.Parametros(ctx, empresaID)
	if err != nil {
		return decimal.Zero
	}
	v, err := decimal.NewFromString(strings.TrimSpace(p["PLANILLA_TOLERANCIA"]))
	if err != nil || v.Sign() < 0 {
		return decimal.Zero
	}
	return v
}

// PlanillaDeAsociacion es la ficha de conciliación de una asociación en un período: los tres
// montos y los depósitos vinculados.
func (s *Service) PlanillaDeAsociacion(ctx context.Context, empresaID, asociacionID, periodo string) (PlanillaDetalle, error) {
	if periodo == "" {
		periodo = hoyCR().Format("2006-01")
	}
	return s.repo.PlanillaDeAsociacion(ctx, empresaID, asociacionID, periodo, s.toleranciaPlanilla(ctx, empresaID))
}

// AbrirPlanilla registra que la asociación mandó su planilla del período, con la referencia
// del comprobante que llegó por correo. Idempotente: no crea dos por período.
func (s *Service) AbrirPlanilla(ctx context.Context, empresaID, asociacionID, periodo, referencia, nota, usuarioID string) (PlanillaDetalle, error) {
	if periodo == "" {
		periodo = hoyCR().Format("2006-01")
	}
	if _, err := s.repo.AbrirPlanilla(ctx, empresaID, asociacionID, periodo, referencia, nota, usuarioID); err != nil {
		return PlanillaDetalle{}, err
	}
	s.auditar(ctx, empresaID, "ABRIR_PLANILLA_CXC", usuarioID, map[string]any{
		"asociacion": asociacionID, "periodo": periodo, "referencia": referencia,
	})
	return s.PlanillaDeAsociacion(ctx, empresaID, asociacionID, periodo)
}

// CandidatosDeposito propone los créditos de Bancos que podrían ser el depósito de esta
// planilla, con la señal de por qué (el monto calza, la descripción la nombra).
func (s *Service) CandidatosDeposito(ctx context.Context, empresaID, planillaID string) ([]CandidatoDeposito, error) {
	return s.repo.CandidatosDeposito(ctx, empresaID, planillaID, margenDiasDeposito)
}

// VincularDeposito ata un movimiento bancario a la planilla y devuelve la ficha actualizada:
// el operador necesita ver de inmediato si con eso quedó conciliada o todavía falta.
func (s *Service) VincularDeposito(ctx context.Context, empresaID, planillaID, movimientoID, usuarioID string) (PlanillaDetalle, error) {
	if err := s.repo.VincularDeposito(ctx, empresaID, planillaID, movimientoID, usuarioID); err != nil {
		return PlanillaDetalle{}, err
	}
	s.auditar(ctx, empresaID, "VINCULAR_DEPOSITO_PLANILLA_CXC", usuarioID, map[string]any{
		"planilla": planillaID, "movimiento": movimientoID,
	})
	return s.fichaDePlanilla(ctx, empresaID, planillaID)
}

// DesvincularDeposito deshace el vínculo. El movimiento no se toca: sigue en Bancos con su
// clasificación, solo deja de contar como depósito de esta planilla.
func (s *Service) DesvincularDeposito(ctx context.Context, empresaID, planillaID, movimientoID, usuarioID string) (PlanillaDetalle, error) {
	if err := s.repo.DesvincularDeposito(ctx, empresaID, planillaID, movimientoID); err != nil {
		return PlanillaDetalle{}, err
	}
	s.auditar(ctx, empresaID, "DESVINCULAR_DEPOSITO_PLANILLA_CXC", usuarioID, map[string]any{
		"planilla": planillaID, "movimiento": movimientoID,
	})
	return s.fichaDePlanilla(ctx, empresaID, planillaID)
}

// fichaDePlanilla recarga la ficha a partir del id de la planilla.
func (s *Service) fichaDePlanilla(ctx context.Context, empresaID, planillaID string) (PlanillaDetalle, error) {
	asociacionID, periodo, err := s.repo.DatosDePlanilla(ctx, empresaID, planillaID)
	if err != nil {
		return PlanillaDetalle{}, err
	}
	return s.PlanillaDeAsociacion(ctx, empresaID, asociacionID, periodo)
}
