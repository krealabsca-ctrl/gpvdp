package cxp

import "testing"

// EL FILTRO POR PRIORIDAD (11 de setiembre de 2026).
//
// La prioridad existía como dato —se veía en la fila, ordenaba la lista y se contaba en el
// tablero— pero no se podía FILTRAR por ella en ningún lado. Con 4.471 facturas abiertas, armar el
// corte «solo las AA» obligaba a buscarlas a ojo.
//
// Lo que se prueba acá es la tabla de condiciones, que es donde vive el riesgo: una condición mal
// escrita no falla, devuelve el subconjunto equivocado, y el resultado es un archivo de pago con
// facturas que nadie quiso pagar.
func TestPrioridadesFiltro(t *testing.T) {
	casos := []struct {
		valor  string
		quiero string
		porque string
	}{
		{"AA", "d.prioridad = 'AA'", "el corte de lo que se paga sí o sí"},
		{"A", "d.prioridad = 'A'", "solo las que pueden esperar"},
		{"AA_A", "d.prioridad IN ('AA', 'A')", "las dos priorizadas juntas"},
		{"sin", "COALESCE(d.prioridad, '') = ''", "las que nadie priorizó; COALESCE porque la columna admite vacío"},
	}
	for _, c := range casos {
		if got := prioridadesFiltro[c.valor]; got != c.quiero {
			t.Errorf("prioridadesFiltro[%q] = %q, quería %q — %s", c.valor, got, c.quiero, c.porque)
		}
	}
}

// UN VALOR DESCONOCIDO NO FILTRA NADA.
//
// La alternativa peligrosa sería interpolar lo que llegue («d.prioridad = '<lo que sea>'»): un
// valor inventado devolvería CERO facturas y la pantalla mostraría una lista vacía como si no
// hubiera nada que pagar. Acá un valor que no está en la tabla devuelve cadena vacía, y el
// repositorio no agrega ninguna condición: se ven todas, que es lo honesto.
func TestPrioridadDesconocidaNoRecortaEnSilencio(t *testing.T) {
	for _, v := range []string{"", "aa", "XX", "AA'; DROP TABLE documento_cxp; --", "NULL", "  AA  "} {
		if cond := prioridadesFiltro[v]; cond != "" {
			t.Errorf("prioridadesFiltro[%q] = %q; un valor que no está en la tabla no puede producir condición", v, cond)
		}
	}
}

// La tabla no tiene más entradas que las cuatro previstas: si alguien agrega una, que sea a
// propósito y con su caso de prueba, porque cada entrada acá decide qué sale al banco.
func TestPrioridadesFiltroNoTieneEntradasDeMas(t *testing.T) {
	if len(prioridadesFiltro) != 4 {
		t.Errorf("prioridadesFiltro tiene %d entradas: %v", len(prioridadesFiltro), prioridadesFiltro)
	}
}
