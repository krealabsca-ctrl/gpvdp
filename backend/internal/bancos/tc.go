package bancos

import "github.com/shopspring/decimal"

// DecimalesTC es la precisión del tipo de cambio en todo el sistema.
//
// PASÓ DE 4 A 6 (10-nov-2026, mig 0119). El TC del mes es el PROMEDIO de tres cotizaciones y un
// promedio de tres casi nunca es exacto: (460,73 + 453,25 + 456,04) / 3 = 456,673333…, que con
// cuatro decimales se guardaba como 456,6733. Esa diferencia multiplica cada monto en dólares del
// mes y es la que impedía que la conversión cuadrara contra el estado de cuenta.
//
// Las cuatro columnas que guardan un TC usan la misma escala (mig 0119). Si alguna vez hay que
// cambiarla, se cambia acá Y en la base: redondear a seis para guardar en una columna de cuatro no
// da error —PostgreSQL redondea solo— y el defecto volvería sin que nada se queje.
const DecimalesTC = 6

// promedioDecimales devuelve el promedio de los valores (0 si vacío).
//
// NO redondea: el redondeo es decisión de quien lo llama, porque el promedio intermedio de un
// cálculo encadenado no debe perder precisión antes de tiempo.
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

// TCCongelado = promedio(día1, día15, último) del mes — RN-12, con DecimalesTC decimales.
func TCCongelado(d1, d15, dUlt decimal.Decimal) decimal.Decimal {
	return promedioDecimales(d1, d15, dUlt).Round(DecimalesTC)
}

// TCProvisionalDia devuelve el TC provisional aplicable a un movimiento según su día — RN-11:
// días 1–14 usan el valor del día 1; del 15 en adelante, promedio(día1, día15).
// Si aún no hay cotización del día 15, se usa el día 1.
func TCProvisionalDia(dia int, d1, d15 decimal.Decimal, tieneD15 bool) decimal.Decimal {
	if dia < 15 || !tieneD15 {
		return d1.Round(DecimalesTC)
	}
	return promedioDecimales(d1, d15).Round(DecimalesTC)
}
