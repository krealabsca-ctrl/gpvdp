package bancos

import "testing"

func TestClasificarMatcher(t *testing.T) {
	reglas := []Regla{
		// Ordenadas por prioridad desc (como las devuelve el repo).
		{ID: "r1", AplicaA: "CREDITO", ConceptoID: "cIng", ClasificacionID: "clDatafono", Prioridad: 200, Palabras: []string{"datafono", "datáfono"}},
		{ID: "r2", AplicaA: "DEBITO", ConceptoID: "cGas", ClasificacionID: "clCCSS", Prioridad: 100, Palabras: []string{"ccss", "caja costarricense"}},
		{ID: "r3", AplicaA: "MIXTO", ConceptoID: "cTras", ClasificacionID: "clTraslado", Prioridad: 50, Palabras: []string{"traslado"}},
	}

	casos := []struct {
		nombre   string
		desc     string
		esDebito bool
		matchea  bool
		concepto string
		reglaID  string
	}{
		{"credito datafono", "PAGO POR DATAFONO BAC", false, true, "cIng", "r1"},
		{"datafono pero es debito -> no aplica CREDITO", "PAGO POR DATAFONO", true, false, "", ""},
		{"debito ccss", "CANCELACION FACTURAS CCSS-APL", true, true, "cGas", "r2"},
		{"acentos e insensible a mayus", "Pago con DATÁFONO", false, true, "cIng", "r1"},
		{"mixto traslado en credito", "TRASLADO 1989 A 132 FONDOS", false, true, "cTras", "r3"},
		{"sin coincidencia", "COMPRA SUPERMERCADO XYZ", true, false, "", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, ok := Clasificar(c.desc, c.esDebito, reglas)
			if ok != c.matchea {
				t.Fatalf("matchea = %v, quería %v", ok, c.matchea)
			}
			if !ok {
				return
			}
			if got.ConceptoID != c.concepto || got.ReglaID != c.reglaID {
				t.Errorf("concepto/regla = %s/%s, quería %s/%s", got.ConceptoID, got.ReglaID, c.concepto, c.reglaID)
			}
			if got.Confianza != 100 {
				t.Errorf("confianza = %d, quería 100", got.Confianza)
			}
		})
	}
}
