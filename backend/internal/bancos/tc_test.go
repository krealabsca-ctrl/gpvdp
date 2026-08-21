package bancos

import "testing"

func TestTCCongelado(t *testing.T) {
	got := TCCongelado(dec("500.00"), dec("510.00"), dec("520.00"))
	if !got.Equal(dec("510")) {
		t.Errorf("TCCongelado = %s, quería 510", got)
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
