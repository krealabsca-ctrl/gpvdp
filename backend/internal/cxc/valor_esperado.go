package cxc

import "github.com/shopspring/decimal"

// ValorEsperado es cuánto se puede recuperar de un contrato si se lo trabaja hoy:
//
//	monto cobrable × probabilidad del tramo × factor de la forma de pago
//
// El monto cobrable es lo VENCIDO, no el saldo total: la cuota que todavía no vence no se
// cobra hoy y sumarla inflaría la expectativa del día.
//
// Es el criterio de orden de la cola de cobro y la ventaja real del prototipo de Apps
// Script: ordenar por antigüedad pone primero los casos MENOS recuperables (los de +180
// días recuperan un 5 %), mientras que ordenar por valor esperado pone primero los casos
// donde una llamada cambia el mes.
//
// Ejemplo de por qué importa: un contrato con ₡9 254 y 215 días vale ₡463 esperados; otro
// con ₡14 929 y 7 días vale ₡13 436. Por antigüedad se llama primero al primero y se
// pierde el día; por valor esperado se llama al segundo.
//
// El resultado se REDONDEA a dos decimales una sola vez, al final: redondear en cada
// factor desordenaría la cola por céntimos.
func ValorEsperado(saldo, probTramo, factorForma decimal.Decimal) decimal.Decimal {
	if saldo.Sign() <= 0 {
		return decimal.Zero
	}
	prob := acotar(probTramo, decimal.Zero, decimal.NewFromInt(1))
	// El factor por forma de pago tiene el mismo rango que el CHECK de la tabla: si viniera
	// fuera de rango se acota en vez de multiplicar por un número absurdo.
	factor := acotar(factorForma, decimal.NewFromFloat(0.10), decimal.NewFromInt(2))
	v := saldo.Mul(prob).Mul(factor)
	// Nunca más que el saldo: el valor esperado es una expectativa de cobro, no un
	// recargo. Con factor 1,15 y probabilidad 1,00 el producto se pasaría del saldo y la
	// cola mostraría «recuperable ₡5 796» de una deuda de ₡5 600, que no tiene sentido.
	if v.GreaterThan(saldo) {
		v = saldo
	}
	return v.Round(2)
}

func acotar(v, min, max decimal.Decimal) decimal.Decimal {
	switch {
	case v.LessThan(min):
		return min
	case v.GreaterThan(max):
		return max
	default:
		return v
	}
}
