package nomina

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Servicio de la corrida quincenal. El ciclo: crear (BORRADOR, se calcula al instante) →
// capturar novedades / recalcular las veces necesarias → aprobar (congela) → pagar
// (descuenta saldos de deducciones). ANULADA es terminal y permite rehacer el mes.

// ListarCorridas devuelve las corridas del año.
func (s *Service) ListarCorridas(ctx context.Context, empresaID string, anio int) ([]Corrida, error) {
	return s.repo.ListarCorridas(ctx, empresaID, anio)
}

// CorridaDetalle devuelve la corrida con colillas y novedades.
func (s *Service) CorridaDetalle(ctx context.Context, empresaID, id string) (CorridaDetalle, error) {
	c, err := s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return CorridaDetalle{}, err
	}
	return s.armarDetalle(ctx, empresaID, c)
}

func (s *Service) armarDetalle(ctx context.Context, empresaID string, c Corrida) (CorridaDetalle, error) {
	lineas, err := s.repo.LineasCorrida(ctx, empresaID, c.ID)
	if err != nil {
		return CorridaDetalle{}, err
	}
	novedades, err := s.repo.NovedadesCorrida(ctx, empresaID, c.ID)
	if err != nil {
		return CorridaDetalle{}, err
	}
	return CorridaDetalle{Corrida: c, Lineas: lineas, Novedades: novedades}, nil
}

// CrearCorrida abre la corrida del mes (BORRADOR) y la calcula de inmediato.
func (s *Service) CrearCorrida(ctx context.Context, empresaID string, anio, mes int, tipo, fechaPago, usuarioID string) (CorridaDetalle, error) {
	if anio < 2024 || anio > 2100 || mes < 1 || mes > 12 {
		return CorridaDetalle{}, ErrMesInvalido
	}
	if tipo != CorridaAdelanto && tipo != CorridaLiquidacion {
		return CorridaDetalle{}, ErrMesInvalido
	}
	if tipo == CorridaAdelanto {
		// Un adelanto de un mes cuya liquidación ya está cerrada jamás se descontaría.
		cerrada, err := s.repo.LiquidacionCerradaDelMes(ctx, empresaID, anio, mes)
		if err != nil {
			return CorridaDetalle{}, err
		}
		if cerrada {
			return CorridaDetalle{}, ErrLiquidacionCerrada
		}
	}
	params, err := s.Parametros(ctx, empresaID, anio)
	if err != nil {
		return CorridaDetalle{}, err
	}
	snapshot, err := json.Marshal(params)
	if err != nil {
		return CorridaDetalle{}, fmt.Errorf("nomina: snapshot parámetros: %w", err)
	}
	c, err := s.repo.CrearCorrida(ctx, empresaID, anio, mes, tipo, fechaPago, snapshot, usuarioID)
	if err != nil {
		return CorridaDetalle{}, err
	}
	s.auditar(ctx, empresaID, "corrida_nomina", c.ID, "CREAR_CORRIDA", usuarioID,
		map[string]any{"anio": anio, "mes": mes, "tipo": tipo})
	if err := s.calcular(ctx, empresaID, c); err != nil {
		return CorridaDetalle{}, err
	}
	c, err = s.repo.CorridaPorID(ctx, empresaID, c.ID)
	if err != nil {
		return CorridaDetalle{}, err
	}
	return s.armarDetalle(ctx, empresaID, c)
}

// RecalcularCorrida rehace las colillas de un BORRADOR con la ficha y parámetros vigentes.
func (s *Service) RecalcularCorrida(ctx context.Context, empresaID, id, usuarioID string) (CorridaDetalle, error) {
	c, err := s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return CorridaDetalle{}, err
	}
	if c.Estado != EstBorrador {
		return CorridaDetalle{}, ErrCorridaNoEditable
	}
	if err := s.calcular(ctx, empresaID, c); err != nil {
		return CorridaDetalle{}, err
	}
	s.auditar(ctx, empresaID, "corrida_nomina", id, "RECALCULAR_CORRIDA", usuarioID, nil)
	c, err = s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return CorridaDetalle{}, err
	}
	return s.armarDetalle(ctx, empresaID, c)
}

// GuardarNovedades reemplaza las novedades del mes (solo LIQUIDACION en BORRADOR) y recalcula.
func (s *Service) GuardarNovedades(ctx context.Context, empresaID, id string, novedades []NovedadInput, usuarioID string) (CorridaDetalle, error) {
	c, err := s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return CorridaDetalle{}, err
	}
	if c.Estado != EstBorrador {
		return CorridaDetalle{}, ErrCorridaNoEditable
	}
	if c.Tipo != CorridaLiquidacion {
		return CorridaDetalle{}, ErrNovedadSoloLiquidacion
	}
	// Parseo y dedupe (empleado+concepto): la última captura gana.
	porClave := map[string]novedadValidada{}
	orden := make([]string, 0, len(novedades))
	for _, n := range novedades {
		if n.EmpleadoID == "" || n.ConceptoID == "" {
			return CorridaDetalle{}, ErrNovedadInvalida
		}
		// Por HORAS: el monto lo calcula el sistema al recalcular la corrida (art. 139), así
		// que acá se guarda en cero y lo que venga en Monto se ignora.
		cantidad := decimal.Zero
		if c, err := decimal.NewFromString(n.Cantidad); err == nil && c.IsPositive() {
			cantidad = c
		}
		monto := decimal.Zero
		if cantidad.IsZero() {
			m, err := decimal.NewFromString(n.Monto)
			if err != nil || !m.IsPositive() {
				return CorridaDetalle{}, ErrNovedadInvalida
			}
			monto = m
		}
		clave := n.EmpleadoID + "|" + n.ConceptoID
		if _, visto := porClave[clave]; !visto {
			orden = append(orden, clave)
		}
		porClave[clave] = novedadValidada{
			EmpleadoID: n.EmpleadoID, ConceptoID: n.ConceptoID, Monto: monto, Cantidad: cantidad,
		}
	}
	validadas := make([]novedadValidada, 0, len(porClave))
	for _, clave := range orden {
		validadas = append(validadas, porClave[clave])
	}
	if err := s.repo.ReemplazarNovedades(ctx, empresaID, id, validadas); err != nil {
		return CorridaDetalle{}, err
	}
	s.auditar(ctx, empresaID, "corrida_nomina", id, "NOVEDADES_CORRIDA", usuarioID,
		map[string]any{"cantidad": len(validadas)})
	if err := s.calcular(ctx, empresaID, c); err != nil {
		return CorridaDetalle{}, err
	}
	c, err = s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return CorridaDetalle{}, err
	}
	return s.armarDetalle(ctx, empresaID, c)
}

// AprobarCorrida congela un BORRADOR (permiso crítico rrhh.corrida). Antes de aprobar se
// RECALCULA: los números congelados siempre reflejan la ficha, los parámetros y el adelanto
// vigentes (si el adelanto del mes se anuló después del último cálculo, aquí se corrige).
func (s *Service) AprobarCorrida(ctx context.Context, empresaID, id, usuarioID string) (Corrida, error) {
	c, err := s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return Corrida{}, err
	}
	if c.Estado != EstBorrador {
		return Corrida{}, ErrCorridaNoAprobable
	}
	if c.Tipo == CorridaLiquidacion {
		// Con el adelanto del mes en borrador se pagaría el mes 1.5 veces: primero se
		// aprueba (para descontarlo) o se anula.
		pendiente, err := s.repo.ExisteAdelantoBorrador(ctx, empresaID, c.Anio, c.Mes)
		if err != nil {
			return Corrida{}, err
		}
		if pendiente {
			return Corrida{}, ErrAdelantoPendiente
		}
	} else {
		// Un adelanto cuyo mes ya liquidó jamás se descontaría (mes pagado 1.5x).
		cerrada, err := s.repo.LiquidacionCerradaDelMes(ctx, empresaID, c.Anio, c.Mes)
		if err != nil {
			return Corrida{}, err
		}
		if cerrada {
			return Corrida{}, ErrLiquidacionCerrada
		}
	}
	if err := s.calcular(ctx, empresaID, c); err != nil {
		return Corrida{}, err
	}
	c, err = s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return Corrida{}, err
	}
	if c.Empleados == 0 {
		return Corrida{}, ErrCorridaSinEmpleados
	}
	// Nunca se congela un depósito negativo (adelanto muy alto / novedades faltantes).
	negativo, err := s.repo.TieneNetoNegativo(ctx, empresaID, id)
	if err != nil {
		return Corrida{}, err
	}
	if negativo {
		return Corrida{}, ErrNetoNegativo
	}
	if c.Tipo == CorridaLiquidacion {
		// Adelantos pagados a empleados que ya no están en la corrida (baja posterior):
		// aprobar así dejaría salario pagado sin cotizar CCSS ni descontarse.
		huerfanos, err := s.repo.AdelantosSinColilla(ctx, empresaID, c.Anio, c.Mes, id)
		if err != nil {
			return Corrida{}, err
		}
		if huerfanos {
			return Corrida{}, ErrAdelantoSinColilla
		}
	}
	n, err := s.repo.AprobarCorrida(ctx, empresaID, id, usuarioID)
	if err != nil {
		return Corrida{}, err
	}
	if n == 0 {
		// La guarda SQL atómica rechazó: desambiguar para dar el error correcto.
		actual, err := s.repo.CorridaPorID(ctx, empresaID, id)
		if err != nil {
			return Corrida{}, err
		}
		switch {
		case actual.Estado != EstBorrador:
			return Corrida{}, ErrCorridaNoAprobable
		case actual.Tipo == CorridaAdelanto:
			return Corrida{}, ErrLiquidacionCerrada
		default:
			return Corrida{}, ErrAdelantoPendiente
		}
	}
	s.auditar(ctx, empresaID, "corrida_nomina", id, "APROBAR_CORRIDA", usuarioID,
		map[string]string{"total_neto": c.TotalNeto})
	return s.repo.CorridaPorID(ctx, empresaID, id)
}

// PagarCorrida marca PAGADA una corrida APROBADA; si es liquidación, descuenta en la misma
// transacción los saldos de las deducciones recurrentes aplicadas (corte automático en 0).
func (s *Service) PagarCorrida(ctx context.Context, empresaID, id, usuarioID string) (Corrida, error) {
	c, err := s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return Corrida{}, err
	}
	if c.Estado != EstAprobada {
		return Corrida{}, ErrCorridaNoPagable
	}
	n, err := s.repo.PagarCorrida(ctx, empresaID, id, usuarioID)
	if err != nil {
		return Corrida{}, err
	}
	if n == 0 {
		return Corrida{}, ErrCorridaNoPagable
	}
	s.auditar(ctx, empresaID, "corrida_nomina", id, "PAGAR_CORRIDA", usuarioID,
		map[string]string{"total_neto": c.TotalNeto})
	return s.repo.CorridaPorID(ctx, empresaID, id)
}

// AnularCorrida anula un BORRADOR o una APROBADA sin pagar (terminal; el mes se puede
// rehacer). Excepción: un ADELANTO aprobado que una liquidación cerrada ya descontó no se
// anula — debe pagarse, porque al empleado ya se le retuvo ese monto.
func (s *Service) AnularCorrida(ctx context.Context, empresaID, id, usuarioID string) (Corrida, error) {
	c, err := s.repo.CorridaPorID(ctx, empresaID, id)
	if err != nil {
		return Corrida{}, err
	}
	if c.Estado == EstPagada || c.Estado == EstAnulada {
		return Corrida{}, ErrCorridaNoAnulable
	}
	n, err := s.repo.AnularCorrida(ctx, empresaID, id)
	if err != nil {
		return Corrida{}, err
	}
	if n == 0 {
		// La guarda SQL rechazó: adelanto aprobado ya descontado por la liquidación, o carrera.
		if c.Tipo == CorridaAdelanto && c.Estado == EstAprobada {
			return Corrida{}, ErrAdelantoDescontado
		}
		return Corrida{}, ErrCorridaNoAnulable
	}
	s.auditar(ctx, empresaID, "corrida_nomina", id, "ANULAR_CORRIDA", usuarioID, nil)
	return s.repo.CorridaPorID(ctx, empresaID, id)
}

// diaDelMes devuelve el día del mes de una fecha YYYY-MM-DD si cae en (anio, mes).
// Sirve para intersecar las ausencias con el período efectivamente laborado.
func diaDelMes(fecha string, anio, mes int) (int, bool) {
	if fecha == "" {
		return 0, false
	}
	d, err := time.Parse("2006-01-02", fecha)
	if err != nil || d.Year() != anio || int(d.Month()) != mes {
		return 0, false
	}
	return d.Day(), true
}

// calcular rehace las colillas de la corrida (BORRADOR) con parámetros vigentes del año.
// El snapshot se refresca en cada cálculo y queda congelado al aprobar.
func (s *Service) calcular(ctx context.Context, empresaID string, c Corrida) error {
	params, err := s.Parametros(ctx, empresaID, c.Anio)
	if err != nil {
		return err
	}
	calcP, err := parametrosACalc(params)
	if err != nil {
		return err
	}
	snapshot, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("nomina: snapshot parámetros: %w", err)
	}
	// Los elegibles según el tipo: la liquidación incluye a quien salió durante el mes
	// (su devengo debe pagarse y cotizar); el adelanto excluye a quien ya tiene finiquito.
	empleados, err := s.repo.EmpleadosParaCorrida(ctx, empresaID, c.Anio, c.Mes, c.Tipo)
	if err != nil {
		return err
	}
	if len(empleados) == 0 {
		return ErrCorridaSinEmpleados
	}

	// Insumos. Las deducciones se necesitan también en la 1ª quincena (el pago quincenal
	// real las retiene según su frecuencia); las novedades y el descuento del adelanto solo
	// al cerrar el mes, y la renta ya retenida el día 15 para calcular el ajuste.
	var novedades map[string][]IngresoCalc
	var adelantos, rentaRetenida map[string]decimal.Decimal
	deducciones, err := s.repo.DeduccionesParaCalc(ctx, empresaID)
	if err != nil {
		return err
	}
	// Las incapacidades del mes descuentan los días que la empresa no paga (los cubre la
	// CCSS o el INS). Se aplican en el período donde caen los días.
	incapacidades, err := s.repo.IncapacidadesParaCalc(ctx, empresaID, c.Anio, c.Mes)
	if err != nil {
		return err
	}
	if c.Tipo == CorridaLiquidacion {
		if novedades, err = s.repo.NovedadesParaCalc(ctx, empresaID, c.ID); err != nil {
			return err
		}
		// Las novedades registradas por HORAS (extra, art. 139) traen su monto derivado del
		// salario VIGENTE, no del que había cuando se capturaron. Se resuelve acá, una sola
		// vez, para que el resto del motor siga viendo solo montos.
		if err := resolverHorasExtra(novedades, empleados, params); err != nil {
			return err
		}
		if adelantos, err = s.repo.AdelantosPagadosDelMes(ctx, empresaID, c.Anio, c.Mes); err != nil {
			return err
		}
		if rentaRetenida, err = s.repo.RentaRetenidaPrimeraQuincena(ctx, empresaID, c.Anio, c.Mes); err != nil {
			return err
		}
	}
	esPrimeraQuincena := c.Tipo == CorridaAdelanto

	lineas := make([]LineaCorrida, 0, len(empleados))
	var tot TotalesCorrida
	uno := decimal.NewFromInt(1)
	dosMitades := decimal.NewFromFloat(0.5) // fracción del impuesto mensual en la 1ª quincena
	for _, ec := range empleados {
		e := ec.Empleado
		salario, err := decimal.NewFromString(e.SalarioBase)
		if err != nil {
			return fmt.Errorf("nomina: salario corrupto (%s): %w", e.ID, err)
		}
		fraccion, err := decimal.NewFromString(ec.FraccionMes)
		if err != nil || !fraccion.IsPositive() {
			fraccion = uno
		}
		entrada := EmpleadoCalc{
			Hijos: e.Hijos, Conyuge: e.Conyuge, SalarioBase: salario, FraccionMes: fraccion,
		}
		// La JORNADA de la ficha decide el tratamiento (decisión del DF 2026-07-29):
		// QUINCENAL cobra dos salarios reales; el resto va por adelanto + liquidación.
		esQuincenal := e.Jornada == JornadaQuincenal
		// Solo las deducciones que cobran en este período, según su frecuencia.
		delPeriodo := make([]DeduccionCalc, 0, len(deducciones[e.ID]))
		for _, d := range deducciones[e.ID] {
			if CobraEn(d.Frecuencia, esPrimeraQuincena, esQuincenal) {
				delPeriodo = append(delPeriodo, d)
			}
		}
		// Salario devengado del período, prorrateado si el mes fue parcial.
		devengadoMes := redondear(salario.Mul(fraccion), calcP.Redondeo)
		sufijoDias := ""
		if fraccion.LessThan(uno) {
			dias := fraccion.Mul(decimal.NewFromInt(30)).Round(0)
			sufijoDias = " (" + dias.String() + "/30 días laborados)"
		}
		mitad := func(v decimal.Decimal) decimal.Decimal {
			return redondear(v.Div(decimal.NewFromInt(2)), calcP.Redondeo)
		}
		// Incapacidades: los días que la empresa no paga (los cubre la CCSS o el INS) se
		// descuentan del salario del período donde caen. En un pago quincenal cada quincena
		// descuenta solo los días que le tocan; en la liquidación mensual, todo el mes.
		incapDesde, incapHasta := 1, 31
		if esQuincenal {
			if esPrimeraQuincena {
				incapHasta = 15
			} else {
				incapDesde = 16
			}
		}
		// Se intersecan con los días efectivamente laborados: si ingresó o salió a mitad
		// de mes, los días fuera de su relación laboral ya están descontados por el
		// prorrateo y no deben castigarse otra vez con la incapacidad.
		if diaIngreso, ok := diaDelMes(e.FechaIngreso, c.Anio, c.Mes); ok && diaIngreso > incapDesde {
			incapDesde = diaIngreso
		}
		if diaSalida, ok := diaDelMes(e.FechaSalida, c.Anio, c.Mes); ok && diaSalida < incapHasta {
			incapHasta = diaSalida
		}
		descIncap, renglonesIncap := AjusteIncapacidadesRango(
			incapacidades[e.ID], salario, c.Anio, c.Mes, incapDesde, incapHasta, calcP)
		// Lo descontado por incapacidad en la PRIMERA quincena: la segunda lo necesita para
		// que la base mensual de la renta no reintegre un salario que nunca se devengó
		// (los tramos son mensuales y el ajuste se calcula sobre el mes completo).
		descIncapPrimera := decimal.Zero
		if esQuincenal && !esPrimeraQuincena {
			desdePrimera := 1
			if diaIngreso, ok := diaDelMes(e.FechaIngreso, c.Anio, c.Mes); ok && diaIngreso > 1 {
				desdePrimera = diaIngreso
			}
			descIncapPrimera, _ = AjusteIncapacidadesRango(
				incapacidades[e.ID], salario, c.Anio, c.Mes, desdePrimera, 15, calcP)
		}
		// El adelanto de la jornada mensual no aplica incapacidad: es un anticipo y el mes
		// se asienta (con su descuento) en la liquidación del día 30.
		if esPrimeraQuincena && !esQuincenal {
			descIncap, renglonesIncap = decimal.Zero, nil
		}
		sufijoIncap := ""
		if descIncap.IsPositive() {
			sufijoIncap = " — menos días de incapacidad"
		}

		var r ResultadoLinea
		tratamiento := TratMensual
		switch {
		case esPrimeraQuincena && esQuincenal:
			// 1ª quincena: media base con su CCSS, sus deducciones y la MITAD del impuesto
			// mensual estimado (los tramos son mensuales; el ajuste llega el día 30).
			tratamiento = TratQuincena1
			devengadoQuincena := restarPiso0(mitad(devengadoMes), descIncap)
			entrada.Ingresos = []IngresoCalc{{
				Nombre: "Salario de quincena" + sufijoDias + sufijoIncap, Monto: devengadoQuincena,
				AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true,
			}}
			entrada.Deducciones = delPeriodo
			// La renta se estima sobre el mes: media base ya cobrada + la otra media menos
			// lo que la incapacidad ya restó de esta quincena.
			entrada.Renta = RentaPeriodo{
				BaseMensual: restarPiso0(devengadoMes, descIncap), Fraccion: dosMitades,
			}
			r = CalcularLiquidacion(entrada, calcP)
		case esPrimeraQuincena:
			// Jornada mensual: anticipo sin deducciones, se descuenta al cerrar el mes.
			tratamiento = TratAdelanto
			r = CalcularAdelanto(entrada, calcP)
		case esQuincenal:
			// 2ª quincena: media base + las novedades del mes; la renta se recalcula sobre
			// el mes real y cobra la diferencia contra lo retenido el día 15.
			tratamiento = TratQuincena2
			devengadoQuincena := restarPiso0(devengadoMes.Sub(mitad(devengadoMes)), descIncap)
			entrada.Ingresos = append([]IngresoCalc{{
				Nombre: "Salario de quincena" + sufijoDias + sufijoIncap, Monto: devengadoQuincena,
				AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true,
			}}, novedades[e.ID]...)
			entrada.Deducciones = delPeriodo
			// Base del mes para la renta: lo devengado menos la incapacidad de AMBAS
			// quincenas (lo retenido el día 15 se resta aparte, pero la base tiene que
			// reflejar el salario real del mes) + las novedades afectas.
			baseMes := restarPiso0(devengadoMes, descIncap.Add(descIncapPrimera))
			for _, n := range novedades[e.ID] {
				if n.AfectaRenta {
					baseMes = baseMes.Add(n.Monto)
				}
			}
			entrada.Renta = RentaPeriodo{BaseMensual: baseMes, YaRetenida: rentaRetenida[e.ID]}
			// Su pago del día 15 fue salario, no un anticipo: nada que descontar aquí.
			r = CalcularLiquidacion(entrada, calcP)
		default:
			// Liquidación mensual: el mes completo (menos incapacidad) y menos el adelanto.
			entrada.Ingresos = append([]IngresoCalc{{
				Nombre:     "Salario ordinario" + sufijoDias + sufijoIncap,
				Monto:      restarPiso0(devengadoMes, descIncap),
				AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true,
			}}, novedades[e.ID]...)
			entrada.Deducciones = delPeriodo
			entrada.AdelantoPagado = adelantos[e.ID] // zero value si no hubo adelanto
			r = CalcularLiquidacion(entrada, calcP)
		}
		// Los renglones de incapacidad son informativos: explican en la colilla cuánto
		// cubre la CCSS o el INS. El salario ya viene neto, así que no restan otra vez.
		if len(renglonesIncap) > 0 {
			r.Detalle = append(r.Detalle, renglonesIncap...)
		}

		lineas = append(lineas, LineaCorrida{
			Tratamiento: tratamiento,
			EmpleadoID:  e.ID, Nombre: e.Nombre, Identificacion: e.Identificacion, IBAN: e.IBAN,
			Departamento: e.DepartamentoNombre, Puesto: e.Puesto,
			SalarioBase: salario.StringFixed(2), Hijos: e.Hijos, Conyuge: e.Conyuge,
			Bruto: r.Bruto.StringFixed(2), BaseCCSS: r.BaseCCSS.StringFixed(2),
			BaseRenta: r.BaseRenta.StringFixed(2), CCSSObrero: r.CCSSObrero.StringFixed(2),
			Renta: r.Renta.StringFixed(2), Deducciones: r.Deducciones.StringFixed(2),
			Adelanto: r.Adelanto.StringFixed(2), Neto: r.Neto.StringFixed(2),
			Patronal: r.Patronal.StringFixed(2), ProvAguinaldo: r.ProvAguinaldo.StringFixed(2),
			ProvVacaciones: r.ProvVacaciones.StringFixed(2), ProvCesantia: r.ProvCesantia.StringFixed(2),
			Detalle: r.Detalle,
		})
		tot.Bruto = tot.Bruto.Add(r.Bruto)
		tot.CCSSObrero = tot.CCSSObrero.Add(r.CCSSObrero)
		tot.Renta = tot.Renta.Add(r.Renta)
		tot.Deducciones = tot.Deducciones.Add(r.Deducciones)
		tot.Adelanto = tot.Adelanto.Add(r.Adelanto)
		tot.Neto = tot.Neto.Add(r.Neto)
		tot.Patronal = tot.Patronal.Add(r.Patronal)
		tot.Provisiones = tot.Provisiones.Add(r.ProvAguinaldo).Add(r.ProvVacaciones).Add(r.ProvCesantia)
	}
	return s.repo.GuardarLineas(ctx, empresaID, c.ID, lineas, tot, snapshot)
}
