package nomina

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Servicio del finiquito. Ciclo: crear (BORRADOR, calculado al instante) → editar/
// recalcular → aprobar (recalcula y congela; permiso crítico rrhh.finiquito) → pagar
// (descuenta saldos, cierra deducciones y da de baja la ficha). ANULADO permite rehacer.

// ListarFiniquitos devuelve los finiquitos de la empresa.
func (s *Service) ListarFiniquitos(ctx context.Context, empresaID string) ([]Finiquito, error) {
	return s.repo.ListarFiniquitos(ctx, empresaID)
}

// FiniquitoPorID devuelve el finiquito con el acumulado provisionado para el comparativo.
func (s *Service) FiniquitoPorID(ctx context.Context, empresaID, id string) (Finiquito, error) {
	fi, err := s.repo.FiniquitoPorID(ctx, empresaID, id)
	if err != nil {
		return Finiquito{}, err
	}
	prov, err := s.repo.ProvisionesEmpleado(ctx, empresaID, fi.EmpleadoID)
	if err != nil {
		return Finiquito{}, err
	}
	fi.Provisionado = prov.StringFixed(2)
	return fi, nil
}

// CrearFiniquito calcula y guarda el BORRADOR de la liquidación de cese.
func (s *Service) CrearFiniquito(ctx context.Context, empresaID string, in FiniquitoInput, usuarioID string) (Finiquito, error) {
	return s.guardarFiniquito(ctx, empresaID, "", in, usuarioID, "CREAR_FINIQUITO")
}

// ActualizarFiniquito recalcula un BORRADOR con nuevos datos (motivo, fecha, vacaciones).
func (s *Service) ActualizarFiniquito(ctx context.Context, empresaID, id string, in FiniquitoInput, usuarioID string) (Finiquito, error) {
	actual, err := s.repo.FiniquitoPorID(ctx, empresaID, id)
	if err != nil {
		return Finiquito{}, err
	}
	if actual.Estado != FinBorrador {
		return Finiquito{}, ErrFiniquitoNoEditable
	}
	in.EmpleadoID = actual.EmpleadoID
	return s.guardarFiniquito(ctx, empresaID, id, in, usuarioID, "ACTUALIZAR_FINIQUITO")
}

func (s *Service) guardarFiniquito(ctx context.Context, empresaID, id string, in FiniquitoInput, usuarioID, accion string) (Finiquito, error) {
	if !EsMotivoValido(in.Motivo) {
		return Finiquito{}, ErrMotivoInvalido
	}
	dias := decimal.Zero
	if in.DiasVacaciones != "" {
		var err error
		// Tope 365: sin él, un valor absurdo desborda numeric(6,2) y revienta con 500.
		if dias, err = decimal.NewFromString(in.DiasVacaciones); err != nil ||
			dias.IsNegative() || dias.GreaterThan(decimal.NewFromInt(365)) {
			return Finiquito{}, ErrDiasVacacionesInvalidos
		}
		in.DiasManual = true
	} else {
		// Sin dato explícito se usa el SALDO PENDIENTE calculado (acumulado por meses de
		// servicio menos lo disfrutado). RRHH puede sobrescribirlo si negocia otra cosa.
		saldo, err := s.saldoVacacionesPendiente(ctx, empresaID, in.EmpleadoID)
		if err != nil {
			return Finiquito{}, err
		}
		dias = saldo
		in.DiasManual = false
	}
	emp, err := s.repo.EmpleadoPorID(ctx, empresaID, in.EmpleadoID)
	if err != nil {
		return Finiquito{}, err
	}
	salida, err := time.Parse("2006-01-02", in.FechaSalida)
	if err != nil {
		return Finiquito{}, ErrFechaSalidaInvalida
	}
	ingreso, err := time.Parse("2006-01-02", emp.FechaIngreso)
	if err != nil || !salida.After(ingreso) {
		return Finiquito{}, ErrFechaSalidaInvalida
	}
	params, err := s.Parametros(ctx, empresaID, salida.Year())
	if err != nil {
		return Finiquito{}, err
	}
	calcP, err := parametrosACalc(params)
	if err != nil {
		return Finiquito{}, err
	}
	// Salario promedio real (últimas liquidaciones pagadas; sin historial: salario base).
	promedio, err := s.repo.SalarioPromedioEmpleado(ctx, empresaID, in.EmpleadoID)
	if err != nil {
		return Finiquito{}, err
	}
	// Dinero ya entregado que el cese debe recuperar: adelanto del mes sin descontar +
	// saldos de préstamos (netos de lo comprometido en liquidaciones aprobadas).
	adelanto, err := s.repo.AdelantoPendienteEmpleado(ctx, empresaID, in.EmpleadoID, salida.Year(), int(salida.Month()))
	if err != nil {
		return Finiquito{}, err
	}
	deducciones, err := s.repo.DeduccionesParaCalc(ctx, empresaID)
	if err != nil {
		return Finiquito{}, err
	}
	res := CalcularFiniquito(EntradaFiniquito{
		Motivo: in.Motivo, FechaIngreso: ingreso, FechaSalida: salida,
		SalarioPromedio: promedio, DiasVacaciones: dias,
		Hijos: emp.Hijos, Conyuge: emp.Conyuge,
		AdelantoPendiente: adelanto, SaldosDeducciones: deducciones[in.EmpleadoID],
	}, calcP)
	fi, err := s.repo.GuardarFiniquito(ctx, empresaID, id, in, res, promedio, dias, usuarioID)
	if err != nil {
		return Finiquito{}, err
	}
	s.auditar(ctx, empresaID, "finiquito", fi.ID, accion, usuarioID,
		map[string]string{"empleado": emp.Nombre, "motivo": in.Motivo, "total": fi.Total})
	return s.FiniquitoPorID(ctx, empresaID, fi.ID)
}

// AprobarFiniquito RECALCULA el borrador (datos vigentes) y lo congela.
func (s *Service) AprobarFiniquito(ctx context.Context, empresaID, id, usuarioID string) (Finiquito, error) {
	actual, err := s.repo.FiniquitoPorID(ctx, empresaID, id)
	if err != nil {
		return Finiquito{}, err
	}
	if actual.Estado != FinBorrador {
		return Finiquito{}, ErrFiniquitoNoAprobable
	}
	// Se recalcula con los datos vigentes. Los días de vacaciones solo se reenvían si los
	// digitó RRHH; si venían del saldo, se vuelven a leer (pudo disfrutar días entre medio
	// y congelar el valor viejo sería pagarle dos veces esos días).
	recalculo := FiniquitoInput{
		EmpleadoID: actual.EmpleadoID, Motivo: actual.Motivo, FechaSalida: actual.FechaSalida,
	}
	if actual.DiasManual {
		recalculo.DiasVacaciones = actual.DiasVacaciones
	}
	recalculado, err := s.guardarFiniquito(ctx, empresaID, id, recalculo, usuarioID, "RECALCULAR_FINIQUITO")
	if err != nil {
		return Finiquito{}, err
	}
	// Locking optimista contra los valores que YO acabo de dejar: si otra sesión escribe
	// entre el recálculo y la aprobación, el UPDATE no aplica y no se congela un cálculo
	// obsoleto (el recálculo propio sí puede haber cambiado los días del saldo).
	n, err := s.repo.AprobarFiniquito(ctx, empresaID, id, usuarioID,
		recalculado.Motivo, recalculado.FechaSalida, recalculado.DiasVacaciones)
	if err != nil {
		return Finiquito{}, err
	}
	if n == 0 {
		vigente, err := s.repo.FiniquitoPorID(ctx, empresaID, id)
		if err != nil {
			return Finiquito{}, err
		}
		if vigente.Estado != FinBorrador {
			return Finiquito{}, ErrFiniquitoNoAprobable
		}
		return Finiquito{}, ErrFiniquitoModificado
	}
	s.auditar(ctx, empresaID, "finiquito", id, "APROBAR_FINIQUITO", usuarioID, nil)
	return s.FiniquitoPorID(ctx, empresaID, id)
}

// PagarFiniquito marca PAGADO: descuenta saldos, cierra deducciones y da de baja la ficha.
func (s *Service) PagarFiniquito(ctx context.Context, empresaID, id, usuarioID string) (Finiquito, error) {
	actual, err := s.repo.FiniquitoPorID(ctx, empresaID, id)
	if err != nil {
		return Finiquito{}, err
	}
	if actual.Estado != FinAprobado {
		return Finiquito{}, ErrFiniquitoNoPagable
	}
	n, err := s.repo.PagarFiniquito(ctx, empresaID, id, usuarioID)
	if err != nil {
		return Finiquito{}, err
	}
	if n == 0 {
		return Finiquito{}, ErrFiniquitoNoPagable
	}
	s.auditar(ctx, empresaID, "finiquito", id, "PAGAR_FINIQUITO", usuarioID,
		map[string]string{"total": actual.Total})
	return s.FiniquitoPorID(ctx, empresaID, id)
}

// AnularFiniquito anula un BORRADOR o APROBADO sin pagar.
func (s *Service) AnularFiniquito(ctx context.Context, empresaID, id, usuarioID string) (Finiquito, error) {
	actual, err := s.repo.FiniquitoPorID(ctx, empresaID, id)
	if err != nil {
		return Finiquito{}, err
	}
	if actual.Estado == FinPagado || actual.Estado == FinAnulado {
		return Finiquito{}, ErrFiniquitoNoAnulable
	}
	n, err := s.repo.AnularFiniquito(ctx, empresaID, id)
	if err != nil {
		return Finiquito{}, err
	}
	if n == 0 {
		// La guarda SQL rechazó: una liquidación cerrada se apoyó en este finiquito.
		return Finiquito{}, ErrFiniquitoRespaldaCorrida
	}
	s.auditar(ctx, empresaID, "finiquito", id, "ANULAR_FINIQUITO", usuarioID, nil)
	return s.FiniquitoPorID(ctx, empresaID, id)
}

// ProvisionesDelAnio devuelve el reporte de provisiones acumuladas (corridas pagadas).
func (s *Service) ProvisionesDelAnio(ctx context.Context, empresaID string, anio int) ([]ProvisionEmpleadoAnio, error) {
	return s.repo.ProvisionesAnio(ctx, empresaID, anio)
}

// saldoVacacionesPendiente devuelve los días de vacaciones que el empleado no ha
// disfrutado (los que el finiquito debe pagar).
func (s *Service) saldoVacacionesPendiente(ctx context.Context, empresaID, empleadoID string) (decimal.Decimal, error) {
	diasPorMes, err := s.diasVacacionesPorMes(ctx, empresaID, time.Now().Year())
	if err != nil {
		return decimal.Zero, err
	}
	saldo, err := s.repo.SaldoVacacionesEmpleado(ctx, empresaID, empleadoID, diasPorMes)
	if err != nil {
		return decimal.Zero, err
	}
	pendiente, err := decimal.NewFromString(saldo.Pendiente)
	if err != nil {
		return decimal.Zero, nil // sin saldo legible: el finiquito arranca en cero
	}
	return pendiente, nil
}

// SaldoVacacionesDeEmpleado expone el saldo pendiente (lo usa el frontend para precargar).
func (s *Service) SaldoVacacionesDeEmpleado(ctx context.Context, empresaID, empleadoID string) (SaldoVacaciones, error) {
	diasPorMes, err := s.diasVacacionesPorMes(ctx, empresaID, time.Now().Year())
	if err != nil {
		return SaldoVacaciones{}, err
	}
	return s.repo.SaldoVacacionesEmpleado(ctx, empresaID, empleadoID, diasPorMes)
}
