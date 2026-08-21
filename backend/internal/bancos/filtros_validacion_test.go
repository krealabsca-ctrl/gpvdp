package bancos

import "testing"

// Un identificador mal escrito en el query string tiene que salir por 400, no por 500: antes
// llegaba al cast `::uuid[]` de Postgres y el cliente recibía «error interno» por un typo.
func TestValidarFiltros(t *testing.T) {
	t.Parallel()
	const bueno = "7341057e-c233-4ae2-8a21-84f366fb46b2"

	casos := []struct {
		nombre    string
		f         FiltrosMovimientos
		valido    bool
		paramMalo string
	}{
		{"vacío es válido (sin restricción)", FiltrosMovimientos{}, true, ""},
		{"uuid bueno singular", FiltrosMovimientos{ConceptoID: bueno}, true, ""},
		{"uuid bueno en lista", FiltrosMovimientos{ClasificacionIDs: []string{bueno, bueno}}, true, ""},
		{"clasificación no-uuid", FiltrosMovimientos{ClasificacionIDs: []string{"no-es-uuid"}}, false, "clasificaciones"},
		{"una buena y una mala en la lista", FiltrosMovimientos{ConceptoIDs: []string{bueno, "xx"}}, false, "conceptos"},
		{"concepto singular mal", FiltrosMovimientos{ConceptoID: "123"}, false, "concepto_id"},
		{"cuenta singular mal", FiltrosMovimientos{CuentaID: "'; DROP TABLE movimiento_bancario; --"}, false, "cuenta_bancaria_id"},
		{"largo correcto pero con letra no hex", FiltrosMovimientos{BancoID: "7341057e-c233-4ae2-8a21-84f366fb46bZ"}, false, "banco_id"},
		{"guion fuera de lugar", FiltrosMovimientos{BancoID: "7341057ec-233-4ae2-8a21-84f366fb46b2"}, false, "banco_id"},
		{"período bueno", FiltrosMovimientos{Periodos: []string{"2026-08"}}, true, ""},
		{"período mal formado", FiltrosMovimientos{Periodos: []string{"agosto"}}, false, "periodos"},
	}

	for _, c := range casos {
		c := c
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			param, ok := validarFiltros(c.f)
			if ok != c.valido {
				t.Fatalf("validarFiltros() válido = %v, se esperaba %v (param %q)", ok, c.valido, param)
			}
			if !c.valido && param != c.paramMalo {
				t.Fatalf("señaló el parámetro %q, se esperaba %q", param, c.paramMalo)
			}
		})
	}
}
