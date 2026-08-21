package cxc

// Suspensión del servicio por mora. La regla: **18 meses de mora, o su equivalencia** en cuotas
// según la modalidad del contrato.
//
// Tres decisiones que importan:
//
//  1. La medida son MESES, no cuotas. Un quincenal con 18 cuotas vencidas lleva 9 meses de
//     atraso: la mitad de lo que manda la regla. Un anual con 18 cuotas lleva 18 AÑOS. La
//     equivalencia sale del ciclo de la modalidad, que ya estaba en el catálogo.
//  2. El sistema NO suspende solo. Calcula cuándo se puede y lo muestra («este contrato llegó
//     al tope»), pero cortarle el servicio a una familia es una decisión de una persona con
//     permiso, con su motivo. Automatizarlo sería el tipo de cosa que después nadie puede
//     explicar.
//  3. El tope es un PARÁMETRO (MESES_PARA_SUSPENDER = 18), no una constante: la regla la puso
//     el negocio y el negocio la puede mover.

import (
	"context"
	"strconv"
	"strings"
)

// mesesParaSuspenderDefault es la regla que dio el usuario: 18 meses de mora acumulada.
const mesesParaSuspenderDefault = 18

func (s *Service) topeMeses(ctx context.Context, empresaID string) int {
	p, err := s.repo.Parametros(ctx, empresaID)
	if err != nil {
		return mesesParaSuspenderDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(p["MESES_PARA_SUSPENDER"])); err == nil && v > 0 && v <= 600 {
		return v
	}
	return mesesParaSuspenderDefault
}

// EstadoDeSuspension dice cuántos meses de mora lleva el contrato y si llegó al tope.
func (s *Service) EstadoDeSuspension(ctx context.Context, empresaID, numero string) (EstadoSuspension, error) {
	return s.repo.EstadoDeSuspension(ctx, empresaID, numero, s.topeMeses(ctx, empresaID))
}

// Suspender corta el servicio.
//
// Se puede suspender ANTES del tope a propósito: el tope es la regla general, pero un caso
// puntual (un fraude, un contrato que el cliente pidió cerrar) no tiene por qué esperar 18
// meses. Lo que no se permite es hacerlo sin motivo.
func (s *Service) Suspender(ctx context.Context, empresaID, numero, motivo, usuarioID string) (EstadoSuspension, error) {
	motivo = strings.TrimSpace(motivo)
	if !motivoUtil(motivo) {
		return EstadoSuspension{}, ErrMotivoRequerido
	}
	est, err := s.repo.Suspender(ctx, empresaID, numero, motivo, usuarioID, s.topeMeses(ctx, empresaID))
	if err != nil {
		return EstadoSuspension{}, err
	}
	s.auditar(ctx, empresaID, "SUSPENDER_CONTRATO_CXC", usuarioID, map[string]any{
		"contrato": numero, "cuotas_vencidas": est.CuotasAlSuspender,
		"meses_mora": est.MesesAlSuspender, "saldo": est.Saldo,
		"tope_meses": est.Tope, "motivo": motivo,
	})
	return est, nil
}

// Reactivar devuelve el contrato al servicio.
func (s *Service) Reactivar(ctx context.Context, empresaID, numero, motivo, usuarioID string) (EstadoSuspension, error) {
	motivo = strings.TrimSpace(motivo)
	if !motivoUtil(motivo) {
		return EstadoSuspension{}, ErrMotivoRequerido
	}
	est, err := s.repo.Reactivar(ctx, empresaID, numero, motivo, usuarioID, s.topeMeses(ctx, empresaID))
	if err != nil {
		return EstadoSuspension{}, err
	}
	s.auditar(ctx, empresaID, "REACTIVAR_CONTRATO_CXC", usuarioID, map[string]any{
		"contrato": numero, "cuotas_vencidas": est.CuotasVencidas,
		"meses_mora": est.MesesMora, "motivo": motivo,
	})
	return est, nil
}

// replaceHoy inyecta la expresión del día de Costa Rica en un fragmento de SQL. El día lo
// pone SIEMPRE la base con `AT TIME ZONE`, nunca Go: a las 7 p. m. de un 4 en Costa Rica, en
// UTC ya es el 5, y las cuotas vencidas se contarían mal por un día.
func replaceHoy(sql, hoy string) string { return strings.ReplaceAll(sql, "$HOY", hoy) }
