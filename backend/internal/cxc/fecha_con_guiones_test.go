package cxc

// El mismo origen escribe la misma fecha con barras y con guiones, en el MISMO archivo
// (8-oct-2026).
//
// En el reporte de pagos real —`listar_pagos_261007160022.csv`, 3.695 cobros— la columna «Fecha de
// Pago Bancario de la asociación» traía «27/01/2026» y la de al lado, «Fecha de Pago», traía
// «01-10-2026». El lector solo conocía la barra, así que la primera entraba y la segunda se perdía:
// las 3.695 filas caían con «sin fecha de pago legible», que es el motivo que impide aplicar el
// cobro.
//
// Que el DÍA va primero no se supuso: se midió sobre el archivo. El segundo componente es «10» en
// las 3.695 filas —octubre— y los primeros van del 01 al 07, que son los días que cubre un reporte
// exportado el 7 de octubre. Si el día fuera el segundo, los 3.695 cobros habrían caído todos en un
// día 10.

import "testing"

func TestLaFechaSeLeeConGuionYConBarra(t *testing.T) {
	casos := map[string]string{
		// Las dos formas del archivo real, lado a lado.
		"27/01/2026": "2026-01-27",
		"01-10-2026": "2026-10-01",
		"28-01-2026": "2026-01-28",
		// Un día que no puede ser mes, en los dos separadores: fija que el día va primero.
		"27-01-2026": "2026-01-27",
		"16/8/2023":  "2023-08-16",
		"16-8-2023":  "2023-08-16",
		// Sin cero a la izquierda.
		"1-2-2022": "2022-02-01",
		// Año de dos dígitos.
		"01-10-26": "2026-10-01",
		"01/10/26": "2026-10-01",
		// ISO, que tiene que seguir leyéndose como año-mes-día.
		"2026-01-27": "2026-01-27",
		"2026/01/27": "2026-01-27",
	}
	for crudo, quiere := range casos {
		if got := fechaDe(crudo); got != quiere {
			t.Errorf("fechaDe(%q) = %q, se esperaba %q", crudo, got, quiere)
		}
	}
}

// El ISO y el d-m-aaaa no se confunden aunque compartan el separador.
//
// Nota de lo que esta prueba NO demuestra: probé a mover el ISO al final de la lista de formatos y
// no cambió ni un caso. El orden no es lo que los separa —lo es el largo del primer componente:
// «2026-01-27» leído como d-m-aaaa toma «20» de día y revienta en el separador siguiente—. Por eso
// acá no hay una prueba que fije ese orden: fijaría algo que no sostiene nada.
func TestElISONoSeConfundeConDiaMesAnio(t *testing.T) {
	if got := fechaDe("2026-01-27"); got != "2026-01-27" {
		t.Fatalf("«2026-01-27» se leyó como %q: el año quedó de día", got)
	}
	// Y al revés: una fecha d-m-aaaa no puede leerse como ISO.
	if got := fechaDe("01-10-2026"); got != "2026-10-01" {
		t.Fatalf("«01-10-2026» se leyó como %q, se esperaba 2026-10-01", got)
	}
}

// Lo que NO es una fecha sigue sin serlo: el lector no puede volverse permisivo de más y empezar a
// aceptar cualquier cosa con guiones.
func TestLoQueNoEsFechaSigueSinSerlo(t *testing.T) {
	for _, s := range []string{"", "   ", "N/A", "sin fecha", "01-2026", "2026", "32-01-2026",
		"01-13-2026", "oct-28", "CO29869"} {
		if got := fechaDe(s); got != "" {
			t.Errorf("fechaDe(%q) = %q y no es una fecha", s, got)
		}
	}
}
