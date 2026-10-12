package bancos

// Contar los meses de un rango (6-oct-2026).
//
// El Director abrió «Análisis y tendencias», tocó el botón de **24 meses** y el servidor le
// contestó «el rango no puede pasar de 24 meses». Pedía exactamente el tope y se lo negaban.
//
// La cuenta vieja aproximaba: `horas/24/28 + 1`. Veintiocho es el mes MÁS CORTO que existe, así que
// todo rango largo salía inflado —los 24 meses se contaban como 26— y el botón de 24 meses NUNCA
// funcionó desde que existe. El de 23 pasaba raspando porque se contaba como 24, y por eso el
// defecto se veía solo en el extremo.
//
// Un mes no dura una cantidad fija de días. Acá se CUENTA el calendario, no se estima.

import (
	"testing"
	"time"
)

func mes(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse("2006-01", s)
	if err != nil {
		t.Fatalf("período %q: %v", s, err)
	}
	return v
}

func TestMesesDelRangoCuentaElCalendario(t *testing.T) {
	casos := []struct {
		nombre, desde, hasta string
		quiere               int
	}{
		// EL caso del reporte: el botón de «24 meses» de la pantalla.
		{"los 24 meses del botón son 24, no 26", "2024-10", "2026-09", 24},
		// Las dos puntas cuentan: un solo mes es 1, no 0.
		{"un mes solo", "2026-09", "2026-09", 1},
		{"dos meses seguidos", "2026-08", "2026-09", 2},
		{"el semestre", "2026-04", "2026-09", 6},
		{"doce meses", "2025-10", "2026-09", 12},
		{"veintitrés, el que pasaba raspando", "2024-11", "2026-09", 23},
		// Cruzar el año no cambia nada, y febrero tampoco —que es justo lo que rompía la estimación.
		{"de febrero a febrero", "2025-02", "2026-02", 13},
		{"un año exacto", "2025-09", "2026-09", 13},
		{"dos años exactos pasan del tope", "2024-09", "2026-09", 25},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := MesesDelRango(mes(t, c.desde), mes(t, c.hasta))
			if got != c.quiere {
				t.Errorf("de %s a %s dio %d meses, son %d", c.desde, c.hasta, got, c.quiere)
			}
		})
	}
}

// La frontera del tope, que es donde vivía el defecto: justo 24 ENTRA y 25 no.
func TestElTopeDelAnalisisDejaPasarExactamenteVeinticuatro(t *testing.T) {
	hasta := mes(t, "2026-09")

	justo := hasta.AddDate(0, -(maxMesesAnalisis - 1), 0)
	if n := MesesDelRango(justo, hasta); n != maxMesesAnalisis {
		t.Errorf("pedir el tope dio %d meses y el tope es %d: el botón de la pantalla se rechaza a sí mismo", n, maxMesesAnalisis)
	}

	unoDeMas := hasta.AddDate(0, -maxMesesAnalisis, 0)
	if n := MesesDelRango(unoDeMas, hasta); n <= maxMesesAnalisis {
		t.Errorf("un mes más que el tope dio %d: el límite dejó de limitar", n)
	}
}

// La cuenta vieja, para que quede escrito POR QUÉ no se puede aproximar con días.
func TestDividirPorVeintiochoInflaLosRangosLargos(t *testing.T) {
	desde, hasta := mes(t, "2024-10"), mes(t, "2026-09")

	viejo := int(hasta.Sub(desde).Hours()/24/28) + 1
	nuevo := MesesDelRango(desde, hasta)

	if nuevo != 24 {
		t.Fatalf("la cuenta buena dio %d, se esperaban 24", nuevo)
	}
	if viejo <= maxMesesAnalisis {
		t.Fatalf("la cuenta vieja dio %d y no habría rechazado nada: esta prueba dejó de reproducir el defecto", viejo)
	}
}
