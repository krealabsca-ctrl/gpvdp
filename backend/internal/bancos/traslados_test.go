package bancos

import (
	"strings"
	"testing"
)

// Casos REALES reportados por el usuario con capturas de la pantalla de emparejamiento.
// Estos tests son el contrato del criterio: si alguien vuelve a emparejar por monto y fecha
// a secas, acá se cae.
func TestPuntuarTraslado(t *testing.T) {
	// El traslado real: «TRASLADO 1989 A 1990 FONDOS/…» en las dos patas, ₡300 000 exactos,
	// el mismo día, entre BN Valle de Paz y BN Jardines. Ojo: ₡300 000 NO supera ₡1 M, así
	// que el criterio no puede exigir monto alto.
	trasladoReal := SenalesTraslado{
		DiceTraslado: true, MontoExacto: true, DiasDiferencia: 0,
		MontoRedondo: true, MontoAlto: false, VecesElMonto: 2, CandidatosDelMovimiento: 1,
	}
	// El falso positivo: cobros de planes por SINPE de ₡4 600 que el sistema ofrecía como
	// traslado contra un débito de comisiones del mismo monto.
	cobroDePlan := SenalesTraslado{
		DiceTraslado: false, DiceCobro: true, MontoExacto: true, DiasDiferencia: 2,
		MontoRedondo: false, MontoAlto: false, VecesElMonto: 41, CandidatosDelMovimiento: 4,
	}

	casos := []struct {
		nombre          string
		s               SenalesTraslado
		quiereVeredicto string
		quiereRazon     string
	}{
		{"traslado real de ₡300 000 (no supera 1M y aun así es traslado)", trasladoReal, TrasladoProbable, "la descripción dice traslado"},
		{"cobro de plan por SINPE", cobroDePlan, TrasladoDescartado, "cobro a cliente"},
		{
			"monto recurrente del negocio: ₡5 600 aparece 1046 veces",
			SenalesTraslado{MontoExacto: true, DiasDiferencia: 0, VecesElMonto: 1046, CandidatosDelMovimiento: 1},
			TrasladoDescartado, "1046 veces",
		},
		{
			"varias parejas posibles: no se empareja de un clic",
			SenalesTraslado{DiceTraslado: true, MontoExacto: true, DiasDiferencia: 0, MontoRedondo: true, VecesElMonto: 3, CandidatosDelMovimiento: 3},
			TrasladoAmbiguo, "3 parejas posibles",
		},
		{
			"coincide lo numérico pero la descripción no respalda: a revisar",
			SenalesTraslado{MontoExacto: true, DiasDiferencia: 0, MontoRedondo: true, VecesElMonto: 2, CandidatosDelMovimiento: 1},
			TrasladoRevisar, "monto idéntico",
		},
		{
			// Aunque los números calcen perfecto, sin la palabra en la descripción no se
			// empareja de un clic: podrían ser un pago a proveedor y un depósito de cliente
			// del mismo monto y día.
			"grande, redondo y del mismo día, pero sin la palabra: lo confirma una persona",
			SenalesTraslado{MontoExacto: true, DiasDiferencia: 0, MontoRedondo: true, MontoAlto: true, VecesElMonto: 1, CandidatosDelMovimiento: 1},
			TrasladoRevisar, "monto alto",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			puntaje, veredicto, razones := PuntuarTraslado(c.s)
			if veredicto != c.quiereVeredicto {
				t.Errorf("veredicto = %s (puntaje %d), quiere %s · razones: %v",
					veredicto, puntaje, c.quiereVeredicto, razones)
			}
			if !strings.Contains(strings.Join(razones, " · "), c.quiereRazon) {
				t.Errorf("las razones no explican el veredicto: %v (buscaba %q)", razones, c.quiereRazon)
			}
		})
	}
}

// El traslado real siempre tiene que puntuar por encima del cobro de plan: es la comparación
// que importa, más allá de los umbrales exactos.
func TestTrasladoRealSuperaAlCobro(t *testing.T) {
	real, _, _ := PuntuarTraslado(SenalesTraslado{
		DiceTraslado: true, MontoExacto: true, DiasDiferencia: 0, MontoRedondo: true,
		VecesElMonto: 2, CandidatosDelMovimiento: 1,
	})
	cobro, _, _ := PuntuarTraslado(SenalesTraslado{
		DiceCobro: true, MontoExacto: true, DiasDiferencia: 2, VecesElMonto: 41, CandidatosDelMovimiento: 4,
	})
	if real <= cobro {
		t.Fatalf("el traslado real puntuó %d y el cobro %d", real, cobro)
	}
}

func TestDentroDeTolerancia(t *testing.T) {
	pct := ToleranciaTrasladoDefault // 1%
	casos := []struct {
		a, b string
		want bool
	}{
		{"1000", "1000", true},      // exacto
		{"1000", "1005", true},      // 0.5% -> dentro
		{"1000", "1010", true},      // 1% exacto (10 <= 1% de 1010 = 10.1)
		{"1000", "1020", false},     // 2% -> fuera
		{"100.00", "100.50", true},  // USD 0.5%
		{"100.00", "102.00", false}, // USD 2%
	}
	for _, c := range casos {
		if got := dentroDeTolerancia(dec(c.a), dec(c.b), pct); got != c.want {
			t.Errorf("dentroDeTolerancia(%s,%s) = %v, quería %v", c.a, c.b, got, c.want)
		}
	}
}
