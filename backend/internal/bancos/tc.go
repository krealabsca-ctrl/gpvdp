package bancos

import "github.com/shopspring/decimal"

// promedioDecimales devuelve el promedio de los valores (0 si vacío).
func promedioDecimales(vals ...decimal.Decimal) decimal.Decimal {
	if len(vals) == 0 {
		return decimal.Zero
	}
	sum := decimal.Zero
	for _, v := range vals {
		sum = sum.Add(v)
	}
	return sum.Div(decimal.NewFromInt(int64(len(vals))))
}

// TCCongelado = promedio(día1, día15, último) del mes — RN-12. Se redondea a 4 decimales.
func TCCongelado(d1, d15, dUlt decimal.Decimal) decimal.Decimal {
	return promedioDecimales(d1, d15, dUlt).Round(4)
}

// TCProvisionalDia devuelve el TC provisional aplicable a un movimiento según su día — RN-11:
// días 1–14 usan el valor del día 1; del 15 en adelante, promedio(día1, día15).
// Si aún no hay cotización del día 15, se usa el día 1. Redondeado a 4 decimales.
func TCProvisionalDia(dia int, d1, d15 decimal.Decimal, tieneD15 bool) decimal.Decimal {
	if dia < 15 || !tieneD15 {
		return d1.Round(4)
	}
	return promedioDecimales(d1, d15).Round(4)
}
