package cxc

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestValorEsperado(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre                      string
		saldo, prob, factor, quiero string
	}{
		// Los dos contratos reales de la muestra, que son el argumento entero de ordenar
		// por valor esperado en vez de por antigüedad.
		{"CO198456: ₡9 254 con 215 días (Legal 0,05 × transferencia 1,00)", "9254", "0.05", "1.00", "462.7"},
		{"CO198454: ₡14 928,52 con 7 días (Preventivo 0,90 × transferencia 1,00)", "14928.52", "0.90", "1.00", "13435.67"},
		// El débito automático sube el valor esperado: la plata sale sola.
		{"débito automático mejora el factor", "5600", "0.90", "1.15", "5600"},
		{"cobrador lo baja", "5600", "0.90", "0.80", "4032"},
		{"saldo en cero no vale nada", "0", "0.90", "1.15", "0"},
		{"saldo negativo tampoco", "-100", "0.90", "1.00", "0"},
		// Al día: se recupera todo, pero nunca más que el saldo.
		{"al día con débito no se pasa del saldo", "5600", "1.00", "1.15", "5600"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := ValorEsperado(d(c.saldo), d(c.prob), d(c.factor))
			if !got.Equal(d(c.quiero)) {
				t.Errorf("ValorEsperado(%s, %s, %s) = %s, se esperaba %s", c.saldo, c.prob, c.factor, got, c.quiero)
			}
		})
	}
}

func TestValorEsperadoNuncaPasaDelSaldo(t *testing.T) {
	t.Parallel()
	// Un valor esperado mayor que la deuda haría que la cola prometa cobrar más de lo que
	// se debe. Con probabilidad 1 y factor 1,15 el producto se pasaría.
	saldo := d("5600")
	got := ValorEsperado(saldo, d("1.00"), d("1.15"))
	if got.GreaterThan(saldo) {
		t.Errorf("valor esperado %s > saldo %s", got, saldo)
	}
}

func TestValorEsperadoAcotaParametrosAbsurdos(t *testing.T) {
	t.Parallel()
	saldo := d("10000")
	// Probabilidad fuera de [0,1] y factor fuera de [0.10, 2]: se acotan en vez de producir
	// un orden de cola sin sentido. Los CHECK de la base ya lo impiden; esto es el cinturón.
	if got := ValorEsperado(saldo, d("5"), d("1.00")); !got.Equal(saldo) {
		t.Errorf("prob 5 debería acotarse a 1: %s", got)
	}
	if got := ValorEsperado(saldo, d("-1"), d("1.00")); !got.Equal(decimal.Zero) {
		t.Errorf("prob negativa debería acotarse a 0: %s", got)
	}
	if got := ValorEsperado(saldo, d("1"), d("99")); !got.Equal(saldo) {
		t.Errorf("factor 99 debería acotarse a 2 y luego topar al saldo: %s", got)
	}
}

func TestValorEsperadoOrdenaLaColaAlRevesQueLaAntiguedad(t *testing.T) {
	t.Parallel()
	// La prueba que justifica la decisión de diseño: el contrato MÁS viejo NO es el que más
	// rinde. Si la cola se ordenara por días, el operador gastaría el día en el peor caso.
	viejo := ValorEsperado(d("9254"), d("0.05"), d("1.00"))        // 215 días
	reciente := ValorEsperado(d("14928.52"), d("0.90"), d("1.00")) // 7 días
	if !reciente.GreaterThan(viejo) {
		t.Fatalf("el reciente (%s) debería valer más que el viejo (%s)", reciente, viejo)
	}
	// Y la diferencia es enorme: 29 veces.
	if reciente.Div(viejo).LessThan(d("20")) {
		t.Errorf("la diferencia es de %s veces, se esperaba mucho más", reciente.Div(viejo).Round(1))
	}
}
