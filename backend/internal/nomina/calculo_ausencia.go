package nomina

// Motor PURO del efecto de las incapacidades en la corrida.
//
// Política confirmada por el Director Financiero (2026-07-29) — la empresa paga lo que la
// ley le obliga y la entidad cubre el resto:
//
//	CCSS (enfermedad): días 1, 2 y 3 → la empresa paga el 50% del salario de esos días.
//	                   Día 4 en adelante → subsidio de la CCSS directo al trabajador;
//	                   la empresa no paga esos días (Reglamento del Seguro de Salud).
//	INS (riesgo del trabajo): día del accidente → la empresa paga el 100%.
//	                   Día siguiente en adelante → subsidio del INS; la empresa no paga
//	                   esos días (Código de Trabajo, art. 236).
//
// El conteo de "primeros días" corre desde el INICIO de la incapacidad, aunque el plazo
// cruce de un mes al otro: la incapacidad que empieza el 30 de junio tiene su día 1 en
// junio, no en julio.
//
// Los días que la empresa no paga NO son salario, así que no cotizan: la base CCSS del mes
// baja legítimamente y esos días se reportan como incapacidad. Esto no es subdeclaración —
// el salario simplemente no se devengó — y el detalle de la colilla lo deja por escrito.

import (
	"time"

	"github.com/shopspring/decimal"
)

// IncapacidadCalc es una incapacidad lista para calcular su efecto en un mes.
type IncapacidadCalc struct {
	ID          string
	Entidad     string
	FechaInicio time.Time
	Dias        int
}

// EfectoIncapacidad es el resultado del cálculo para un mes concreto.
type EfectoIncapacidad struct {
	// DiasEnMes son los días de incapacidad que caen dentro del mes de la corrida.
	DiasEnMes int
	// DiasPagaEmpresa son los días EQUIVALENTES que paga la empresa dentro del mes
	// (tres días al 50% cuentan como 1,5).
	DiasPagaEmpresa decimal.Decimal
	// DiasCubreEntidad son los días que la empresa no paga (los cubre la CCSS o el INS).
	DiasCubreEntidad int
}

// factorPatronoDia devuelve la parte del salario diario que paga la EMPRESA en el día
// n-ésimo de una incapacidad (n empieza en 1), según la entidad.
func factorPatronoDia(entidad string, n int) decimal.Decimal {
	if entidad == EntidadINS {
		if n == 1 {
			return decimal.NewFromInt(1) // el día del accidente lo paga la empresa completo
		}
		return decimal.Zero
	}
	// CCSS: los tres primeros días al 50%.
	if n <= 3 {
		return decimal.NewFromFloat(0.5)
	}
	return decimal.Zero
}

// CalcularEfectoIncapacidad reparte una incapacidad sobre el mes completo (anio, mes).
func CalcularEfectoIncapacidad(inc IncapacidadCalc, anio, mes int) EfectoIncapacidad {
	return CalcularEfectoIncapacidadRango(inc, anio, mes, 1, 31)
}

// CalcularEfectoIncapacidadRango limita el cálculo a los días [diaDesde, diaHasta] del
// mes, para repartir la incapacidad entre la 1ª y la 2ª quincena de un pago quincenal.
// El conteo de "primeros días" (los que paga la empresa) sigue corriendo desde el inicio
// de la incapacidad, no desde el inicio del rango.
//
// El salario diario de la nómina es mensual/30, así que los días contados se topan a 30:
// en un mes de 31 días una incapacidad completa descuenta el mes, nunca más que el mes.
func CalcularEfectoIncapacidadRango(inc IncapacidadCalc, anio, mes, diaDesde, diaHasta int) EfectoIncapacidad {
	var e EfectoIncapacidad
	e.DiasPagaEmpresa = decimal.Zero
	for n := 1; n <= inc.Dias; n++ {
		dia := inc.FechaInicio.AddDate(0, 0, n-1)
		if dia.Year() != anio || int(dia.Month()) != mes {
			continue
		}
		if dia.Day() < diaDesde || dia.Day() > diaHasta {
			continue
		}
		if dia.Day() > 30 {
			continue // base 30: el día 31 no descuenta salario adicional
		}
		e.DiasEnMes++
		factor := factorPatronoDia(inc.Entidad, n)
		e.DiasPagaEmpresa = e.DiasPagaEmpresa.Add(factor)
		if factor.IsZero() {
			e.DiasCubreEntidad++
		}
	}
	return e
}

// AjusteIncapacidades calcula, para un mes, cuánto se descuenta del salario del empleado
// por los días que la empresa no paga, y devuelve los renglones para la colilla.
//
// descuento = salario_diario × (días de incapacidad en el mes − días equivalentes que
// paga la empresa). Con CCSS de 5 días: 5 − 1,5 = 3,5 días de salario descontados.
func AjusteIncapacidades(incs []IncapacidadCalc, salarioMensual decimal.Decimal, anio, mes int, p ParametrosCalc) (decimal.Decimal, []DetalleLinea) {
	return AjusteIncapacidadesRango(incs, salarioMensual, anio, mes, 1, 31, p)
}

// AjusteIncapacidadesRango hace lo mismo limitado a los días [diaDesde, diaHasta] del mes
// (la 1ª quincena descuenta lo suyo y la 2ª lo suyo, sin pisarse).
func AjusteIncapacidadesRango(incs []IncapacidadCalc, salarioMensual decimal.Decimal, anio, mes, diaDesde, diaHasta int, p ParametrosCalc) (decimal.Decimal, []DetalleLinea) {
	if len(incs) == 0 {
		return decimal.Zero, nil
	}
	salarioDiario := salarioMensual.Div(decimal.NewFromInt(30))
	total := decimal.Zero
	renglones := make([]DetalleLinea, 0, len(incs))
	for _, inc := range incs {
		ef := CalcularEfectoIncapacidadRango(inc, anio, mes, diaDesde, diaHasta)
		if ef.DiasEnMes == 0 {
			continue
		}
		noPagados := decimal.NewFromInt(int64(ef.DiasEnMes)).Sub(ef.DiasPagaEmpresa)
		monto := redondear(salarioDiario.Mul(noPagados), p.Redondeo)
		if !monto.IsPositive() {
			continue
		}
		total = total.Add(monto)
		renglones = append(renglones, DetalleLinea{
			Tipo:   "INCAPACIDAD",
			Nombre: DescribirSubsidio(inc.Entidad, ef.DiasEnMes, ef.DiasCubreEntidad),
			Monto:  monto.StringFixed(2),
		})
	}
	return total, renglones
}

// DescribirSubsidio explica en una línea quién paga qué, para la colilla y la pantalla.
func DescribirSubsidio(entidad string, diasEnMes, diasEntidad int) string {
	plural := func(n int) string {
		if n == 1 {
			return " día"
		}
		return " días"
	}
	if entidad == EntidadINS {
		if diasEntidad == 0 {
			return "Incapacidad INS · el día del accidente lo paga la empresa"
		}
		return "Incapacidad INS · riesgo del trabajo: el día del accidente lo paga la empresa y " +
			itoa(diasEntidad) + plural(diasEntidad) + " los subsidia el INS"
	}
	pagaEmpresa := diasEnMes - diasEntidad
	if diasEntidad == 0 {
		return "Incapacidad CCSS · " + itoa(pagaEmpresa) + plural(pagaEmpresa) + " al 50% por cuenta de la empresa"
	}
	if pagaEmpresa == 0 {
		return "Incapacidad CCSS · " + itoa(diasEntidad) + plural(diasEntidad) + " con subsidio de la CCSS"
	}
	return "Incapacidad CCSS · " + itoa(pagaEmpresa) + plural(pagaEmpresa) + " al 50% por cuenta de la empresa y " +
		itoa(diasEntidad) + plural(diasEntidad) + " con subsidio de la CCSS"
}

// itoa evita importar strconv solo para esto (los valores son días, siempre pequeños).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
