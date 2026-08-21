package nomina

// Horas extra — Código de Trabajo, art. 139.
//
// La jornada extraordinaria se paga con un 50% más que la ordinaria («tiempo y medio»). Antes de
// esto el monto se digitaba a mano, que es donde se cometen los errores: hay que sacar el valor
// de la hora, multiplicarlo por el factor y por las horas. Ahora se capturan las HORAS.
//
// Las horas extra SON salario: cotizan a la CCSS, pagan renta y entran al aguinaldo. Eso ya viene
// marcado en el concepto «Horas extra» (de_sistema, banderas bloqueadas), así que este cálculo no
// puede usarse para sacar remuneración de la base contributiva.

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	// ErrHorasInvalidas exige horas > 0 cuando la novedad se registra por cantidad.
	ErrHorasInvalidas = errors.New("nomina: las horas extra deben ser mayores a cero")
	// ErrJornadaInvalida indica un divisor de horas inservible (parámetros mal guardados).
	ErrJornadaInvalida = errors.New("nomina: las horas de jornada del mes deben ser mayores a cero")
)

// FactorHoraExtraLegal es el mínimo del art. 139: tiempo y medio. Los parámetros pueden subirlo
// (hay acuerdos que pagan más) pero nunca bajarlo.
var FactorHoraExtraLegal = decimal.RequireFromString("1.5")

// HorasJornadaMesDefault son las horas ordinarias de un mes: 30 días × 8 horas, el uso corriente
// en Costa Rica para salario mensual.
var HorasJornadaMesDefault = decimal.RequireFromString("240")

// ValorHoraOrdinaria es el salario de una hora ordinaria: salario del mes ÷ horas del mes.
// Sin redondear: quien redondea es el cálculo final de la línea.
func ValorHoraOrdinaria(salarioMes, horasJornadaMes decimal.Decimal) (decimal.Decimal, error) {
	if horasJornadaMes.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrJornadaInvalida
	}
	return salarioMes.Div(horasJornadaMes), nil
}

// MontoHorasExtra calcula el pago de la jornada extraordinaria:
//
//	horas × (salario del mes ÷ horas del mes) × factor
//
// El factor se eleva al mínimo legal si viene por debajo: los parámetros de una empresa no pueden
// pagar la hora extra a menos de tiempo y medio.
func MontoHorasExtra(salarioMes, horas, horasJornadaMes, factor decimal.Decimal) (decimal.Decimal, error) {
	if horas.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrHorasInvalidas
	}
	valorHora, err := ValorHoraOrdinaria(salarioMes, horasJornadaMes)
	if err != nil {
		return decimal.Zero, err
	}
	if factor.LessThan(FactorHoraExtraLegal) {
		factor = FactorHoraExtraLegal
	}
	// Se redondea a céntimos al final, una sola vez.
	return valorHora.Mul(factor).Mul(horas).Round(2), nil
}

// resolverHorasExtra convierte las novedades capturadas por HORAS en su monto, usando el salario
// vigente de cada empleado. Deja el desglose en el nombre («Horas extra — 6 h × ₡4 062,50»), que
// es lo que el colaborador necesita leer en su colilla para poder verificar el pago.
//
// Muta el mapa en el lugar: el resto del motor sigue viendo solo montos, sin enterarse de que
// algunos vinieron de horas.
func resolverHorasExtra(novedades map[string][]IngresoCalc, empleados []EmpleadoCorrida, p Parametros) error {
	horasJornada, factor := horasJornadaOFactorDefault(p)
	salarios := make(map[string]decimal.Decimal, len(empleados))
	for _, ec := range empleados {
		if s, err := decimal.NewFromString(ec.Empleado.SalarioBase); err == nil {
			salarios[ec.Empleado.ID] = s
		}
	}
	for empleadoID, lista := range novedades {
		for i := range lista {
			if !lista[i].Horas.IsPositive() {
				continue // novedad de monto directo (comisión, bono, viático)
			}
			salario, ok := salarios[empleadoID]
			if !ok {
				// El empleado no entró a esta corrida: su novedad no se paga acá.
				lista[i].Monto = decimal.Zero
				continue
			}
			monto, err := MontoHorasExtra(salario, lista[i].Horas, horasJornada, factor)
			if err != nil {
				return err
			}
			valorHora, _ := ValorHoraOrdinaria(salario, horasJornada)
			lista[i].Monto = monto
			lista[i].Nombre += " — " + lista[i].Horas.String() + " h × ₡" +
				valorHora.Mul(factor).Round(2).StringFixed(2)
		}
	}
	return nil
}

// horasJornadaOFactorDefault interpreta los parámetros tolerando valores vacíos o inservibles
// (parámetros de un año que nunca se guardaron): siempre devuelve algo con el que se pueda pagar
// conforme a la ley.
func horasJornadaOFactorDefault(p Parametros) (horas, factor decimal.Decimal) {
	horas = HorasJornadaMesDefault
	if h, err := decimal.NewFromString(p.HorasJornadaMes); err == nil && h.IsPositive() {
		horas = h
	}
	factor = FactorHoraExtraLegal
	if f, err := decimal.NewFromString(p.FactorHoraExtra); err == nil && f.GreaterThan(FactorHoraExtraLegal) {
		factor = f
	}
	return horas, factor
}
