package cxc

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// cargos arma la cartera de un contrato como la devuelve la base: con su saldo pendiente.
func cargos(specs ...[3]string) []CargoParaAplicar {
	out := make([]CargoParaAplicar, 0, len(specs))
	for i, s := range specs {
		out = append(out, CargoParaAplicar{
			ID:       "cg" + string(rune('A'+i)),
			Periodo:  s[0],
			VenceEn:  s[1],
			Monto:    d(s[2]),
			Aplicado: decimal.Zero,
		})
	}
	return out
}

func TestAplicarFIFOPagaElMasViejoPrimero(t *testing.T) {
	t.Parallel()
	// El caso que planteó el usuario: debe 3 meses de ₡5 600 y deposita mes y medio.
	c := cargos(
		[3]string{"2026-05", "2026-05-20", "5600"},
		[3]string{"2026-06", "2026-06-20", "5600"},
		[3]string{"2026-07", "2026-07-20", "5600"},
	)
	res, err := AplicarFIFO(d("8400"), c)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(res.Aplicaciones) != 2 {
		t.Fatalf("aplicaciones = %d, se esperaban 2: %+v", len(res.Aplicaciones), res.Aplicaciones)
	}
	// Mayo completo…
	if res.Aplicaciones[0].Periodo != "2026-05" || !res.Aplicaciones[0].Monto.Equal(d("5600")) ||
		res.Aplicaciones[0].Parcial || res.Aplicaciones[0].EstadoFinal != CargoSaldado {
		t.Errorf("primera aplicación = %+v", res.Aplicaciones[0])
	}
	// …y ₡2 800 a junio, que queda PARCIAL (no «sin pagar»).
	if res.Aplicaciones[1].Periodo != "2026-06" || !res.Aplicaciones[1].Monto.Equal(d("2800")) ||
		!res.Aplicaciones[1].Parcial || res.Aplicaciones[1].EstadoFinal != CargoParcial {
		t.Errorf("segunda aplicación = %+v", res.Aplicaciones[1])
	}
	if !res.Aplicado.Equal(d("8400")) || !res.SaldoAFavor.Equal(decimal.Zero) {
		t.Errorf("aplicado = %s · saldo a favor = %s", res.Aplicado, res.SaldoAFavor)
	}
}

func TestAplicarFIFOSobrepagoVaASaldoAFavor(t *testing.T) {
	t.Parallel()
	c := cargos([3]string{"2026-07", "2026-07-20", "5600"})
	res, err := AplicarFIFO(d("20000"), c)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !res.Aplicado.Equal(d("5600")) {
		t.Errorf("aplicado = %s, se esperaba 5600 (nunca más de lo que vale el cargo)", res.Aplicado)
	}
	// Lo que sobra NO se descarta ni se fuerza contra un cargo que todavía no existe.
	if !res.SaldoAFavor.Equal(d("14400")) {
		t.Errorf("saldo a favor = %s, se esperaba 14400", res.SaldoAFavor)
	}
}

func TestAplicarFIFONuncaSobreAplicaUnCargoYaParcial(t *testing.T) {
	t.Parallel()
	// Un cargo que ya tenía ₡2 800 aplicados: solo le faltan ₡2 800.
	c := []CargoParaAplicar{
		{ID: "cgA", Periodo: "2026-06", VenceEn: "2026-06-20", Monto: d("5600"), Aplicado: d("2800")},
		{ID: "cgB", Periodo: "2026-07", VenceEn: "2026-07-20", Monto: d("5600"), Aplicado: decimal.Zero},
	}
	res, err := AplicarFIFO(d("5600"), c)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !res.Aplicaciones[0].Monto.Equal(d("2800")) || res.Aplicaciones[0].EstadoFinal != CargoSaldado {
		t.Errorf("al cargo parcial se le aplicó %s (se esperaba 2800 y quedar SALDADO)", res.Aplicaciones[0].Monto)
	}
	if !res.Aplicaciones[1].Monto.Equal(d("2800")) || !res.Aplicaciones[1].Parcial {
		t.Errorf("segunda aplicación = %+v", res.Aplicaciones[1])
	}
	// Sumado, nunca más que el cobro.
	if !res.Aplicado.Equal(d("5600")) {
		t.Errorf("aplicado = %s", res.Aplicado)
	}
}

func TestAplicarFIFOOrdenaAunqueLleguenDesordenados(t *testing.T) {
	t.Parallel()
	// La consulta podría cambiar su ORDER BY: si la función confiara en el orden de
	// entrada, la plata se aplicaría distinto sin que nadie se diera cuenta.
	c := []CargoParaAplicar{
		{ID: "z", Periodo: "2026-07", VenceEn: "2026-07-20", Monto: d("5600")},
		{ID: "a", Periodo: "2026-05", VenceEn: "2026-05-20", Monto: d("5600")},
		{ID: "m", Periodo: "2026-06", VenceEn: "2026-06-20", Monto: d("5600")},
	}
	res, err := AplicarFIFO(d("5600"), c)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if res.Aplicaciones[0].Periodo != "2026-05" {
		t.Errorf("se aplicó primero a %s, se esperaba 2026-05", res.Aplicaciones[0].Periodo)
	}
}

func TestAplicarFIFOQuincenasEnOrden(t *testing.T) {
	t.Parallel()
	// 1Q antes que 2Q del mismo mes aunque vencieran el mismo día (dato posible en los
	// contratos de asociación, que es el canal dominante).
	c := []CargoParaAplicar{
		{ID: "b", Periodo: "2026-07-2Q", VenceEn: "2026-07-16", Monto: d("2500")},
		{ID: "a", Periodo: "2026-07-1Q", VenceEn: "2026-07-16", Monto: d("2500")},
	}
	res, err := AplicarFIFO(d("2750"), c)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// Es el caso REAL del archivo de pagos: ₡2 500 a 1Q y ₡250 parciales a 2Q.
	if res.Aplicaciones[0].Periodo != "2026-07-1Q" || !res.Aplicaciones[0].Monto.Equal(d("2500")) {
		t.Errorf("primera = %+v", res.Aplicaciones[0])
	}
	if res.Aplicaciones[1].Periodo != "2026-07-2Q" || !res.Aplicaciones[1].Monto.Equal(d("250")) ||
		!res.Aplicaciones[1].Parcial {
		t.Errorf("segunda = %+v", res.Aplicaciones[1])
	}
}

func TestAplicarFIFOSinCargosTodoQuedaAFavor(t *testing.T) {
	t.Parallel()
	res, err := AplicarFIFO(d("5600"), nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(res.Aplicaciones) != 0 || !res.SaldoAFavor.Equal(d("5600")) {
		t.Errorf("sin cargos: aplicaciones=%d saldo a favor=%s", len(res.Aplicaciones), res.SaldoAFavor)
	}
}

func TestAplicarFIFOIgnoraLosSaldados(t *testing.T) {
	t.Parallel()
	c := []CargoParaAplicar{
		{ID: "saldado", Periodo: "2026-04", VenceEn: "2026-04-20", Monto: d("5600"), Aplicado: d("5600")},
		{ID: "abierto", Periodo: "2026-05", VenceEn: "2026-05-20", Monto: d("5600")},
	}
	res, err := AplicarFIFO(d("1000"), c)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(res.Aplicaciones) != 1 || res.Aplicaciones[0].CargoID != "abierto" {
		t.Errorf("aplicaciones = %+v", res.Aplicaciones)
	}
}

func TestAplicarFIFORechazaMontosImposibles(t *testing.T) {
	t.Parallel()
	c := cargos([3]string{"2026-07", "2026-07-20", "5600"})
	for _, m := range []string{"0", "-100"} {
		if _, err := AplicarFIFO(d(m), c); !errors.Is(err, ErrMontoInvalido) {
			t.Errorf("monto %s: err = %v, se esperaba ErrMontoInvalido", m, err)
		}
	}
}

func TestAplicarADestinoRespetaLaEleccionDelOperador(t *testing.T) {
	t.Parallel()
	c := cargos(
		[3]string{"2026-05", "2026-05-20", "5600"},
		[3]string{"2026-06", "2026-06-20", "5600"},
		[3]string{"2026-07", "2026-07-20", "5600"},
	)
	// El cliente dice: «esto es para julio». Se respeta, aunque no sea el más viejo.
	res, err := AplicarADestino(d("5600"), c, []string{c[2].ID})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(res.Aplicaciones) != 1 || res.Aplicaciones[0].Periodo != "2026-07" {
		t.Errorf("aplicaciones = %+v", res.Aplicaciones)
	}
}

func TestAplicarADestinoAvisaEnVezDeIgnorar(t *testing.T) {
	t.Parallel()
	c := []CargoParaAplicar{
		{ID: "saldado", Periodo: "2026-04", VenceEn: "2026-04-20", Monto: d("5600"), Aplicado: d("5600")},
	}
	// Si el operador eligió un cargo que ya está pagado, hay que decírselo: ignorarlo en
	// silencio le haría creer que su elección se respetó.
	if _, err := AplicarADestino(d("1000"), c, []string{"saldado"}); !errors.Is(err, ErrCargoSinSaldo) {
		t.Errorf("err = %v, se esperaba ErrCargoSinSaldo", err)
	}
	if _, err := AplicarADestino(d("1000"), c, []string{"no-existe"}); !errors.Is(err, ErrCargoAjeno) {
		t.Errorf("err = %v, se esperaba ErrCargoAjeno", err)
	}
}

func TestEstadoDeCargoEsLaMismaVerdadEnLasDosDirecciones(t *testing.T) {
	t.Parallel()
	casos := []struct {
		monto, aplicado, quiero string
	}{
		{"5600", "0", CargoAbierto},
		{"5600", "2800", CargoParcial},
		{"5600", "5600", CargoSaldado},
		// Defensa: si por un redondeo quedara aplicado > monto, sigue siendo SALDADO y no
		// un estado inventado.
		{"5600", "5600.01", CargoSaldado},
	}
	for _, c := range casos {
		if got := EstadoDeCargo(d(c.monto), d(c.aplicado)); got != c.quiero {
			t.Errorf("EstadoDeCargo(%s, %s) = %s, se esperaba %s", c.monto, c.aplicado, got, c.quiero)
		}
	}
}
