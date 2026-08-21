package nomina

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Servicio de incapacidades y vacaciones. Las ausencias alimentan la corrida del período:
// por eso no se registran ni se anulan si la corrida de ese mes ya está congelada.

// diasVacacionesPorMes devuelve el parámetro del año (default 1 día por mes trabajado).
func (s *Service) diasVacacionesPorMes(ctx context.Context, empresaID string, anio int) (decimal.Decimal, error) {
	p, err := s.Parametros(ctx, empresaID, anio)
	if err != nil {
		return decimal.Zero, err
	}
	d, err := decimal.NewFromString(p.VacacionesDiasMes)
	if err != nil || !d.IsPositive() {
		return decimal.NewFromInt(1), nil
	}
	return d, nil
}

// ListarIncapacidades devuelve las incapacidades que tocan el mes, con el detalle de
// quién paga qué días (calculado, no almacenado).
func (s *Service) ListarIncapacidades(ctx context.Context, empresaID string, anio, mes int) ([]Incapacidad, error) {
	items, err := s.repo.ListarIncapacidades(ctx, empresaID, anio, mes)
	if err != nil {
		return nil, err
	}
	for i := range items {
		explicarIncapacidad(&items[i], anio, mes)
	}
	return items, nil
}

// explicarIncapacidad completa los campos derivados (quién paga qué y cuántos días
// equivalentes cubre la empresa) para el mes indicado.
func explicarIncapacidad(inc *Incapacidad, anio, mes int) {
	inicio, err := time.Parse("2006-01-02", inc.FechaInicio)
	if err != nil {
		return
	}
	ef := CalcularEfectoIncapacidad(IncapacidadCalc{
		Entidad: inc.Entidad, FechaInicio: inicio, Dias: inc.Dias,
	}, anio, mes)
	inc.Subsidio = DescribirSubsidio(inc.Entidad, ef.DiasEnMes, ef.DiasCubreEntidad)
	inc.DiasEmpresa = ef.DiasPagaEmpresa.StringFixed(2)
}

// RegistrarIncapacidad valida y guarda la incapacidad. No se puede registrar si la corrida
// del mes en que inicia ya está aprobada o pagada (cambiaría números congelados).
func (s *Service) RegistrarIncapacidad(ctx context.Context, empresaID string, in IncapacidadInput, usuarioID string) (Incapacidad, error) {
	if in.Entidad != EntidadCCSS && in.Entidad != EntidadINS {
		return Incapacidad{}, ErrEntidadInvalida
	}
	if in.Dias <= 0 || in.Dias > 365 {
		return Incapacidad{}, ErrDiasInvalidos
	}
	inicio, err := time.Parse("2006-01-02", in.FechaInicio)
	if err != nil {
		return Incapacidad{}, ErrFechaInvalida
	}
	fin := inicio.AddDate(0, 0, in.Dias-1)
	if err := s.exigirPeriodosAbiertos(ctx, empresaID, in.EmpleadoID, inicio, fin); err != nil {
		return Incapacidad{}, err
	}
	inc, err := s.repo.CrearIncapacidad(ctx, empresaID, in, usuarioID)
	if err != nil {
		return Incapacidad{}, err
	}
	// La respuesta explica el subsidio del mes en que inicia (lo que la UI muestra al
	// confirmar): quién paga qué días y cuántos equivalentes cubre la empresa.
	explicarIncapacidad(&inc, inicio.Year(), int(inicio.Month()))
	s.auditar(ctx, empresaID, "incapacidad", inc.ID, "REGISTRAR_INCAPACIDAD", usuarioID,
		map[string]any{"empleado": inc.EmpleadoNombre, "entidad": in.Entidad, "dias": in.Dias,
			"desde": in.FechaInicio, "subsidio": inc.Subsidio})
	return inc, nil
}

// AnularIncapacidad la da de baja lógica (nunca se borra) si el período sigue abierto.
func (s *Service) AnularIncapacidad(ctx context.Context, empresaID, id, usuarioID string) error {
	inc, err := s.repo.IncapacidadPorID(ctx, empresaID, id)
	if err != nil {
		return err
	}
	inicio, err := time.Parse("2006-01-02", inc.FechaInicio)
	if err != nil {
		return ErrFechaInvalida
	}
	// Anular también mueve dinero: se valida TODO el período que la incapacidad abarcó,
	// no solo el mes en que empezó (podría haber descontado días en un mes ya pagado).
	fin := inicio.AddDate(0, 0, inc.Dias-1)
	if err := s.exigirPeriodosAbiertos(ctx, empresaID, inc.EmpleadoID, inicio, fin); err != nil {
		return err
	}
	if err := s.repo.AnularIncapacidad(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "incapacidad", id, "ANULAR_INCAPACIDAD", usuarioID, nil)
	return nil
}

// exigirPeriodosAbiertos valida que ninguna corrida ya congelada use los días que la
// ausencia toca — mes por mes, incluidos los intermedios de una incapacidad larga.
//
// Solo bloquea LA corrida que realmente usaría esos días, no cualquiera del mes:
//   - jornada quincenal: los días 1-15 los calcula la corrida del día 15 y los 16-31 la
//     del día 30, así que cada quincena se valida por separado;
//   - jornada mensual: el adelanto del día 15 no aplica incapacidad (se ajusta al
//     liquidar), de modo que solo la liquidación del mes puede estar cerrada.
//
// Sin esta precisión, pagar el adelanto el día 15 dejaría el mes entero bloqueado y la
// boleta que llega el 16 no podría registrarse: la empresa terminaría pagando días que
// la CCSS o el INS subsidian, y declarándolos como salario.
func (s *Service) exigirPeriodosAbiertos(ctx context.Context, empresaID, empleadoID string, desde, hasta time.Time) error {
	emp, err := s.repo.EmpleadoPorID(ctx, empresaID, empleadoID)
	if err != nil {
		return err
	}
	esQuincenal := emp.Jornada == JornadaQuincenal

	// Recorre cada mes tocado por el rango (el primero, los intermedios y el último).
	mes := time.Date(desde.Year(), desde.Month(), 1, 0, 0, 0, 0, time.UTC)
	ultimo := time.Date(hasta.Year(), hasta.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !mes.After(ultimo) {
		anio, m := mes.Year(), int(mes.Month())
		// Qué mitades del mes toca el rango.
		tocaPrimera, tocaSegunda := false, false
		finMes := mes.AddDate(0, 1, -1)
		ini, fin := desde, hasta
		if ini.Before(mes) {
			ini = mes
		}
		if fin.After(finMes) {
			fin = finMes
		}
		if ini.Day() <= 15 {
			tocaPrimera = true
		}
		if fin.Day() >= 16 {
			tocaSegunda = true
		}

		tipos := make([]string, 0, 2)
		if esQuincenal && tocaPrimera {
			tipos = append(tipos, CorridaAdelanto)
		}
		if !esQuincenal || tocaSegunda || !tocaPrimera {
			// La liquidación del mes siempre asienta el mes completo en jornada mensual,
			// y la 2ª quincena en jornada quincenal.
			tipos = append(tipos, CorridaLiquidacion)
		}
		for _, tipo := range tipos {
			cerrada, err := s.repo.CorridaCerradaDelMes(ctx, empresaID, anio, m, tipo)
			if err != nil {
				return err
			}
			if cerrada {
				return ErrAusenciaCorridaCerrada
			}
		}
		mes = mes.AddDate(0, 1, 0)
	}
	return nil
}

// SaldosVacaciones devuelve el saldo derivado de todos los empleados vigentes.
func (s *Service) SaldosVacaciones(ctx context.Context, empresaID string, anio int) ([]SaldoVacaciones, error) {
	dias, err := s.diasVacacionesPorMes(ctx, empresaID, anio)
	if err != nil {
		return nil, err
	}
	return s.repo.SaldosVacaciones(ctx, empresaID, dias)
}

// ListarVacaciones devuelve los disfrutes registrados (todos, o de un empleado).
func (s *Service) ListarVacaciones(ctx context.Context, empresaID, empleadoID string) ([]Vacacion, error) {
	return s.repo.ListarVacaciones(ctx, empresaID, empleadoID)
}

// RegistrarVacacion anota un disfrute de vacaciones y descuenta el saldo (derivado).
// No permite registrar más días de los acumulados.
func (s *Service) RegistrarVacacion(ctx context.Context, empresaID string, in VacacionInput, usuarioID string) (Vacacion, error) {
	dias, err := decimal.NewFromString(in.Dias)
	if err != nil || !dias.IsPositive() || dias.GreaterThan(decimal.NewFromInt(365)) {
		return Vacacion{}, ErrDiasInvalidos
	}
	inicio, err := time.Parse("2006-01-02", in.FechaInicio)
	if err != nil {
		return Vacacion{}, ErrFechaInvalida
	}
	diasPorMes, err := s.diasVacacionesPorMes(ctx, empresaID, inicio.Year())
	if err != nil {
		return Vacacion{}, err
	}
	saldo, err := s.repo.SaldoVacacionesEmpleado(ctx, empresaID, in.EmpleadoID, diasPorMes)
	if err != nil {
		return Vacacion{}, err
	}
	pendiente, err := decimal.NewFromString(saldo.Pendiente)
	if err != nil {
		return Vacacion{}, err
	}
	if dias.GreaterThan(pendiente) {
		return Vacacion{}, ErrSinSaldoVacaciones
	}
	// El INSERT re-verifica el saldo en el propio WHERE (anti-carrera).
	v, err := s.repo.CrearVacacion(ctx, empresaID, in, diasPorMes, usuarioID)
	if err != nil {
		return Vacacion{}, err
	}
	s.auditar(ctx, empresaID, "vacacion", v.ID, "REGISTRAR_VACACION", usuarioID,
		map[string]any{"empleado": v.EmpleadoNombre, "dias": in.Dias, "desde": in.FechaInicio})
	return v, nil
}

// AnularVacacion devuelve los días al saldo (baja lógica).
func (s *Service) AnularVacacion(ctx context.Context, empresaID, id, usuarioID string) error {
	if err := s.repo.AnularVacacion(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "vacacion", id, "ANULAR_VACACION", usuarioID, nil)
	return nil
}
