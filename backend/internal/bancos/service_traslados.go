package bancos

import (
	"context"
	"fmt"
	"sort"

	"github.com/gpvdp/erp/internal/shared"
)

// PropuestasTraslados devuelve los pares candidatos débito↔crédito del período (RN-19) que
// vale la pena mirar, con su veredicto y las razones que lo explican.
//
// Los DESCARTADOS no se devuelven: son cobros a clientes y montos recurrentes del negocio que
// solo coinciden en monto y fecha. Antes se ofrecían todos por igual, con el mismo botón
// «Emparejar», y así un cobro de plan terminaba marcado como traslado y saliendo del EBITDA.
func (s *Service) PropuestasTraslados(ctx context.Context, empresaID, periodo string) ([]PropuestaTraslado, error) {
	pct, err := s.repo.ToleranciaTraslado(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	todas, err := s.repo.PropuestasTraslados(ctx, empresaID, periodo, toleranciaEfectiva(pct))
	if err != nil {
		return nil, err
	}
	out := make([]PropuestaTraslado, 0, len(todas))
	for _, p := range todas {
		if p.Veredicto == TrasladoDescartado {
			continue
		}
		out = append(out, p)
	}
	// Lo más confiable primero: es el orden en que conviene trabajar la cola.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Puntaje > out[j].Puntaje })
	return out, nil
}

// EmparejarTraslado confirma un par débito↔crédito entre cuentas del grupo (dentro de tolerancia).
func (s *Service) EmparejarTraslado(ctx context.Context, empresaID, debitoID, creditoID, usuarioID string) error {
	if debitoID == creditoID {
		return ErrTrasladoInvalido
	}
	d, err := s.repo.MovimientoParaTraslado(ctx, empresaID, debitoID)
	if err != nil {
		return err
	}
	c, err := s.repo.MovimientoParaTraslado(ctx, empresaID, creditoID)
	if err != nil {
		return err
	}
	pct, err := s.repo.ToleranciaTraslado(ctx, empresaID)
	if err != nil {
		return err
	}
	// Débito en una cuenta, crédito en OTRA, ambos INCLUIDOS, ninguno ya emparejado,
	// dentro de la tolerancia CONFIGURABLE de la empresa (misma que las propuestas).
	if d.EsTraslado || c.EsTraslado ||
		!d.Incluido || !c.Incluido ||
		d.CuentaID == c.CuentaID ||
		!d.Debito.IsPositive() || !c.Credito.IsPositive() ||
		!dentroDeTolerancia(d.Debito, c.Credito, toleranciaEfectiva(pct)) {
		return ErrTrasladoInvalido
	}
	if err := s.repo.EmparejarTraslado(ctx, empresaID, debitoID, creditoID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "movimiento_bancario", EntidadID: &debitoID,
		Accion: "EMPAREJAR_TRASLADO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"credito_id": creditoID},
	})
	return nil
}

// DesemparejarTraslado deshace un emparejamiento (ambas patas vuelven a NO_IDENTIFICADO).
func (s *Service) DesemparejarTraslado(ctx context.Context, empresaID, movID, usuarioID string) error {
	if err := s.repo.DesemparejarTraslado(ctx, empresaID, movID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "movimiento_bancario", EntidadID: &movID,
		Accion: "DESEMPAREJAR_TRASLADO", UsuarioID: &usuarioID,
	})
	return nil
}

// EstadoPeriodo indica si el período está cerrado.
func (s *Service) EstadoPeriodo(ctx context.Context, empresaID string, anio, mes int) (bool, error) {
	return s.repo.PeriodoCerrado(ctx, empresaID, anio, mes)
}

// CerrarPeriodo cierra el período. Si CierreBloqueante y hay No-identificados, falla (RN-22).
// Devuelve la cantidad de No-identificados (para reportarla, incluso en el error).
func (s *Service) CerrarPeriodo(ctx context.Context, empresaID string, anio, mes int, usuarioID string) (int, error) {
	cerrado, err := s.repo.PeriodoCerrado(ctx, empresaID, anio, mes)
	if err != nil {
		return 0, err
	}
	if cerrado {
		return 0, ErrPeriodoYaCerrado
	}
	periodo := fmt.Sprintf("%04d-%02d", anio, mes)
	totales, err := s.repo.TotalesPeriodo(ctx, empresaID, periodo)
	if err != nil {
		return 0, err
	}
	noID := totales.NoIdentificados
	if s.cierreBloqueante && noID > 0 {
		return noID, ErrPeriodoConNoIdentificados
	}
	// «Se debe cerrar todo e identificar todo» (decisión del 2026-07-31): además de clasificar
	// el 100%, cada cuenta activa tiene que tener su acta de conciliación firmada — y firmarla
	// exige diferencia sin explicar cero. Así el mes no cierra con un saldo que no cuadra.
	if s.cierreBloqueante {
		conc, err := s.Conciliacion(ctx, empresaID, anio, mes)
		if err != nil {
			return noID, err
		}
		if !conc.PuedeCerrar {
			pend := &ErrorConciliacion{}
			for _, a := range conc.Actas {
				if a.FirmadoEn == "" {
					pend.Pendientes++
					pend.Cuentas = append(pend.Cuentas, a.Alias)
				}
			}
			return noID, pend
		}
	}
	if err := s.repo.CerrarPeriodo(ctx, empresaID, anio, mes, noID, usuarioID); err != nil {
		return 0, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "periodo_cierre", Accion: "CERRAR_PERIODO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]int{"anio": anio, "mes": mes, "no_identificados": noID},
	})
	return noID, nil
}
