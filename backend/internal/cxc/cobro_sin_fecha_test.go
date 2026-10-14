package cxc

// Un cobro sin fecha de pago no se manda a estrellar contra la base (13-oct-2026).
//
// EL DEFECTO. `cobro_cxc.fecha_pago` es NOT NULL y el INSERT la pasa como `$4::date`. El lector ya
// marcaba la fila con el motivo «sin fecha de pago legible», pero el confirmador la intentaba
// registrar igual, y Postgres contestaba:
//
//	cxc: insertar cobro: ERROR: invalid input syntax for type date: "" (SQLSTATE 22007)
//
// Eso es lo que le llegaba al usuario, una línea por fila. Si la columna de fechas del archivo
// viene con un nombre que el lector no reconoce —ya pasó dos veces en este proyecto, con el
// snake_case y con las fechas de Excel— caen TODAS las filas, y leer 3.695 veces «invalid input
// syntax for type date» no dice qué columna arreglar.
//
// Lo que se prueba acá es la decisión, no el SQL: una fila sin fecha se descarta ANTES, con un
// motivo en palabras, y las demás del mismo archivo entran igual.

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// Las filas sin fecha se reconocen por `FechaPago == ""`, que es lo que el confirmador mira. Esta
// prueba fija esa correspondencia: si el lector dejara de vaciar la fecha cuando no la entiende,
// el guardarraíl del confirmador quedaría sin efecto y el error crudo volvería.
func TestElLectorDejaVaciaLaFechaQueNoEntiende(t *testing.T) {
	g := Grid{
		{"contrato", "consecutivo", "fecha_pago", "valor"},
		{"100001", "R-001", "", "25000"},
		{"100002", "R-002", "no es una fecha", "30000"},
		{"100003", "R-003", "02/01/2026", "35000"},
	}
	filas, err := LeerCobros(HojaDe(g), ReglasCobros{})
	if err != nil {
		t.Fatalf("leer cobros: %v", err)
	}
	if len(filas) != 3 {
		t.Fatalf("filas = %d, quería 3", len(filas))
	}

	for _, i := range []int{0, 1} {
		if filas[i].FechaPago != "" {
			t.Errorf("fila %d: fecha = %q, quería vacía", i+1, filas[i].FechaPago)
		}
		if !tieneMotivoDeFecha(filas[i].Motivos) {
			t.Errorf("fila %d: motivos = %v, quería uno que nombre la fecha", i+1, filas[i].Motivos)
		}
	}
	// Y la que sí se entiende queda con su fecha: el guardarraíl no puede comerse las buenas.
	if filas[2].FechaPago == "" {
		t.Errorf("la fila con fecha válida quedó sin fecha: %v", filas[2].Motivos)
	}
}

// El monto y el contrato pueden faltar sin que la base se queje —el cobro entra a «sin
// identificar»— pero la FECHA no: es la única columna que la tabla exige.
func TestSoloLaFechaEsImprescindibleParaGuardar(t *testing.T) {
	g := Grid{
		{"contrato", "consecutivo", "fecha_pago", "valor"},
		{"", "R-010", "02/01/2026", "25000"},
	}
	filas, err := LeerCobros(HojaDe(g), ReglasCobros{})
	if err != nil {
		t.Fatalf("leer cobros: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %d, quería 1", len(filas))
	}
	if filas[0].FechaPago == "" {
		t.Fatal("un cobro sin contrato pero CON fecha tiene que conservarla: entra a «sin identificar»")
	}
	if filas[0].Monto.Cmp(decimal.RequireFromString("25000")) != 0 {
		t.Errorf("monto = %s, quería 25000", filas[0].Monto)
	}
}

func tieneMotivoDeFecha(motivos []string) bool {
	for _, m := range motivos {
		if strings.Contains(strings.ToLower(m), "fecha") {
			return true
		}
	}
	return false
}
