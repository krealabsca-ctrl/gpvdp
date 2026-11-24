package bancos

import "testing"

func TestTCCongelado(t *testing.T) {
	got := TCCongelado(dec("500.00"), dec("510.00"), dec("520.00"))
	if !got.Equal(dec("510")) {
		t.Errorf("TCCongelado = %s, quería 510", got)
	}
}

// El promedio que NO es exacto, que es el caso normal (10-nov-2026).
//
// Estas son las tres cotizaciones reales de octubre 2026: (460,73 + 453,25 + 456,04) / 3 da
// 456,673333…, y con cuatro decimales se guardaba como 456,6733. Esa diferencia multiplica cada
// monto en dólares del mes y es la que impedía que la conversión cuadrara.
//
// Las otras pruebas de este archivo usan valores que dan un promedio exacto (500, 510, 520 → 510),
// así que pasaban igual de bien con cuatro decimales que con seis: no protegían nada de esto.
func TestElPromedioConservaSeisDecimales(t *testing.T) {
	got := TCCongelado(dec("460.73"), dec("453.25"), dec("456.04"))
	if !got.Equal(dec("456.673333")) {
		t.Errorf("TCCongelado = %s, quería 456.673333 (se están perdiendo decimales)", got)
	}
	// Y el provisional del día 15 en adelante, que es el mismo promedio con dos valores.
	prov := TCProvisionalDia(20, dec("460.73"), dec("453.25"), true)
	if !prov.Equal(dec("456.99")) {
		t.Errorf("TCProvisionalDia = %s, quería 456.99", prov)
	}
}

func TestTCProvisionalDia(t *testing.T) {
	d1 := dec("500")
	d15 := dec("520")
	casos := []struct {
		dia      int
		tieneD15 bool
		want     string
	}{
		{1, true, "500"},   // día 1 -> d1
		{14, true, "500"},  // día 14 -> d1
		{15, true, "510"},  // día 15 -> promedio(500,520)
		{28, true, "510"},  // día 28 -> promedio
		{20, false, "500"}, // sin d15 -> d1
	}
	for _, c := range casos {
		got := TCProvisionalDia(c.dia, d1, d15, c.tieneD15)
		if !got.Equal(dec(c.want)) {
			t.Errorf("TCProvisionalDia(dia=%d, tieneD15=%v) = %s, quería %s", c.dia, c.tieneD15, got, c.want)
		}
	}
}
