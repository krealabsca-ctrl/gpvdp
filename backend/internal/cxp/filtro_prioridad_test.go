package cxp

import (
	"strings"
	"testing"
)

// EL FILTRO POR PRIORIDAD (11 de setiembre de 2026; reescrito el 11-nov-2026, mig 0121).
//
// La prioridad existía como dato —se veía en la fila, ordenaba la lista y se contaba en el
// tablero— pero no se podía FILTRAR por ella en ningún lado. Con 4.471 facturas abiertas, armar el
// corte «solo las AA» obligaba a buscarlas a ojo.
//
// LO QUE CAMBIÓ CON LAS PRIORIDADES CONFIGURABLES. Antes esto era un mapa cerrado con los tres
// códigos que existían, y cualquier otro valor no filtraba nada. Eso dejó de servir en cuanto los
// códigos los crea el usuario: una prioridad nueva no estaría en el mapa y el filtro la ignoraría
// en silencio, que es exactamente el defecto que el mapa venía a evitar.
//
// Ahora un código cualquiera SÍ filtra, pero como PARÁMETRO. Esa es la protección que de verdad
// importaba y la que sigue probada acá: lo que llega del cliente no se concatena nunca a la
// consulta.

// La condición de un código viaja con marcador de parámetro, NUNCA con el valor adentro.
func TestElCodigoDePrioridadViajaComoParametro(t *testing.T) {
	for _, codigo := range []string{"AA", "A", "URG", "AA'; DROP TABLE documento_cxp; --"} {
		plantilla, params := condicionPrioridad(codigo)
		if len(params) != 1 || params[0] != codigo {
			t.Errorf("condicionPrioridad(%q): params = %v, se esperaba el código como único parámetro",
				codigo, params)
		}
		// La plantilla lleva el hueco del parámetro y NO el valor: si el código apareciera acá,
		// sería interpolación, y el último caso de la lista de arriba sería una inyección.
		if !strings.Contains(plantilla, "$%d") {
			t.Errorf("condicionPrioridad(%q): plantilla = %q, no lleva marcador de parámetro",
				codigo, plantilla)
		}
		if strings.Contains(plantilla, codigo) {
			t.Errorf("condicionPrioridad(%q): el código quedó DENTRO de la consulta: %q",
				codigo, plantilla)
		}
	}
}

// Los tres valores especiales arman su condición sin parámetros.
func TestLosValoresEspecialesDelFiltroDePrioridad(t *testing.T) {
	casos := []struct {
		valor  string
		quiero string
		porque string
	}{
		{FiltroPrioridadSin, "COALESCE(d.prioridad, '') = ''",
			"las que nadie priorizó; COALESCE porque la columna admite vacío"},
		{FiltroPrioridadCon, "COALESCE(d.prioridad, '') <> ''",
			"cualquiera con prioridad puesta: reemplaza al viejo AA_A, que daba por hecho que solo había dos"},
	}
	for _, c := range casos {
		plantilla, params := condicionPrioridad(c.valor)
		if plantilla != c.quiero {
			t.Errorf("condicionPrioridad(%q) = %q, quería %q — %s", c.valor, plantilla, c.quiero, c.porque)
		}
		if len(params) != 0 {
			t.Errorf("condicionPrioridad(%q) no debería llevar parámetros, lleva %v", c.valor, params)
		}
	}

	// «Urgentes» se resuelve contra el CATÁLOGO DE LA MISMA EMPRESA del documento. Las dos cosas
	// importan: que mire el catálogo (y no un código escrito) y que lo acote por empresa.
	plantilla, params := condicionPrioridad(FiltroPrioridadUrgentes)
	if len(params) != 0 {
		t.Errorf("«urgentes» no debería llevar parámetros, lleva %v", params)
	}
	for _, trozo := range []string{"prioridad_cxp", "pr.urgente", "pr.empresa_id = d.empresa_id", "pr.activa"} {
		if !strings.Contains(plantilla, trozo) {
			t.Errorf("la condición de «urgentes» no contiene %q: %s", trozo, plantilla)
		}
	}
	// Y NO menciona ningún código: si dijera 'AA', una prioridad urgente creada después no
	// contaría, y el corte de «lo que se paga sí o sí» se quedaría corto sin avisar.
	if strings.Contains(plantilla, "'AA'") {
		t.Errorf("la condición de «urgentes» tiene un código escrito a mano: %s", plantilla)
	}
}

// Sin valor no se filtra: es lo que deja ver TODAS las facturas cuando nadie eligió un filtro.
func TestSinValorNoHayCondicionDePrioridad(t *testing.T) {
	if plantilla, params := condicionPrioridad(""); plantilla != "" || params != nil {
		t.Errorf("un filtro vacío produjo condición: %q %v", plantilla, params)
	}
}
