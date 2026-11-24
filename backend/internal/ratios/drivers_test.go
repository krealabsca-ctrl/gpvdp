package ratios

// Los drivers de la cartera.
//
// Lo que se prueba acá es que la pantalla no pueda presentar lo CONTRATADO como si fuera lo
// facturado, y que un porcentaje sobre un total de cero no se invente. Las dos cosas producen
// números que se ven perfectamente razonables.

import (
	"strings"
	"testing"
)

// LA DISTINCIÓN QUE SOSTIENE LA PANTALLA: contratado no es facturado, y mientras difieran hay
// que decirlo. Medido en la empresa: 10.468 de 35.375 contratos generaron cargo en el mes.
func TestLaBrechaEntreContratadoYFacturadoSeAvisa(t *testing.T) {
	d := DriversDeCartera{
		Dimension:           PorTipoServicio,
		PeriodoFacturado:    "2026-09",
		ContratosActivos:    35375,
		CuotaContratada:     "227207030.00",
		ContratosFacturados: 10468,
		CuotaFacturada:      "15761395.00",
	}
	CompletarDrivers(&d)

	if d.CoberturaPct != "29.6" {
		t.Errorf("cobertura %q, se esperaba 29.6", d.CoberturaPct)
	}
	var avisado bool
	for _, a := range d.Avisos {
		if strings.Contains(a, "no es lo FACTURADO") {
			avisado = true
		}
	}
	if !avisado {
		t.Errorf("no se avisó la brecha: %v", d.Avisos)
	}
	// Y el ticket recurrente sale de dividir lo contratado entre los contratos:
	// 227.207.030 / 35.375 = 6.422,81.
	if d.CuotaPromedio != "6422.81" {
		t.Errorf("cuota promedio %q, se esperaba 6422.81", d.CuotaPromedio)
	}
}

// Cuando TODOS facturaron no hay brecha que avisar: el aviso de más gasta la atención del que
// sí importa.
func TestSinBrechaNoSeAvisaDeMas(t *testing.T) {
	d := DriversDeCartera{
		PeriodoFacturado:    "2026-09",
		ContratosActivos:    100,
		CuotaContratada:     "500000.00",
		ContratosFacturados: 100,
	}
	CompletarDrivers(&d)
	for _, a := range d.Avisos {
		if strings.Contains(a, "no es lo FACTURADO") {
			t.Errorf("avisó una brecha que no existe: %q", a)
		}
	}
	if d.CoberturaPct != "100.0" {
		t.Errorf("cobertura %q", d.CoberturaPct)
	}
}

// UN PORCENTAJE SOBRE CERO NO ES CERO POR CIENTO. Un corte sin contratos que muestre «0 % de
// aporte» afirma que no aporta nada; lo correcto es no afirmar.
func TestUnPorcentajeSobreCeroNoSeInventa(t *testing.T) {
	d := DriversDeCartera{
		PeriodoFacturado: "2026-09",
		ContratosActivos: 0,
		CuotaContratada:  "0.00",
		Filas: []FilaDriver{
			{Clave: "x", Nombre: "Sin contratos", Contratos: 0, CuotaContratada: "0.00"},
		},
	}
	CompletarDrivers(&d)

	if d.CoberturaPct != "" {
		t.Errorf("cobertura %q sobre cero contratos", d.CoberturaPct)
	}
	if d.Filas[0].AportePct != "" || d.Filas[0].CoberturaPct != "" {
		t.Errorf("la fila inventó porcentajes: aporte %q, cobertura %q",
			d.Filas[0].AportePct, d.Filas[0].CoberturaPct)
	}
	// La cuota promedio sí es cero: no hay con qué dividir y cero es la respuesta honesta.
	if d.Filas[0].CuotaPromedio != "0.00" {
		t.Errorf("cuota promedio %q", d.Filas[0].CuotaPromedio)
	}
}

// El mix: cada corte con su ticket, su peso y su cobertura propias.
func TestElMixCalculaTicketPesoYCobertura(t *testing.T) {
	d := DriversDeCartera{
		Dimension:        PorTipoServicio,
		PeriodoFacturado: "2026-09",
		ContratosActivos: 300,
		CuotaContratada:  "2000000.00",
		Filas: []FilaDriver{
			{Clave: "a", Nombre: "CORPORATIVO", Contratos: 200, CuotaContratada: "1500000.00", ContratosFacturados: 100},
			{Clave: "b", Nombre: "ESCAPADITAS", Contratos: 100, CuotaContratada: "500000.00", ContratosFacturados: 100},
		},
	}
	CompletarDrivers(&d)

	// CORPORATIVO: ticket 7.500, aporta el 75 %, cobertura 50 %.
	if d.Filas[0].CuotaPromedio != "7500.00" || d.Filas[0].AportePct != "75.0" || d.Filas[0].CoberturaPct != "50.0" {
		t.Errorf("corporativo: ticket %q aporte %q cobertura %q",
			d.Filas[0].CuotaPromedio, d.Filas[0].AportePct, d.Filas[0].CoberturaPct)
	}
	// ESCAPADITAS: ticket 5.000, aporta el 25 %, cobertura total.
	if d.Filas[1].CuotaPromedio != "5000.00" || d.Filas[1].AportePct != "25.0" || d.Filas[1].CoberturaPct != "100.0" {
		t.Errorf("escapaditas: ticket %q aporte %q cobertura %q",
			d.Filas[1].CuotaPromedio, d.Filas[1].AportePct, d.Filas[1].CoberturaPct)
	}
}

// LA LIMITACIÓN DE FONDO VA SIEMPRE, aunque todo lo demás esté perfecto: esto mide la cartera,
// no la operación. Sin decirlo, «drivers del negocio» se lee como si incluyera los servicios.
func TestSiempreSeDiceQueNoHayServiciosPrestados(t *testing.T) {
	d := DriversDeCartera{ContratosActivos: 10, ContratosFacturados: 10, CuotaContratada: "1000.00"}
	CompletarDrivers(&d)
	var hallado bool
	for _, a := range d.Avisos {
		if strings.Contains(a, "servicio PRESTADO") {
			hallado = true
		}
	}
	if !hallado {
		t.Errorf("no se declaró la limitación: %v", d.Avisos)
	}
}

// La dimensión es una lista CERRADA: su valor entra en la consulta.
func TestLaDimensionEsUnaListaCerrada(t *testing.T) {
	for _, ok := range []string{PorTipoServicio, PorModalidad, PorSede} {
		if !DimensionValida(ok) {
			t.Errorf("%q tendría que ser válida", ok)
		}
	}
	for _, malo := range []string{"", "cuota_vigente", "nombre; DROP TABLE", "SEDE"} {
		if DimensionValida(malo) {
			t.Errorf("%q se aceptó como dimensión", malo)
		}
	}
}

func TestMiles(t *testing.T) {
	casos := map[int]string{0: "0", 999: "999", 1000: "1.000", 35375: "35.375", 1234567: "1.234.567"}
	for n, quiere := range casos {
		if got := miles(n); got != quiere {
			t.Errorf("miles(%d) = %q, quiere %q", n, got, quiere)
		}
	}
}

// SIN CARTERA, UNA PANTALLA DE CEROS NO SE EXPLICA SOLA.
//
// Memorial Pets no tiene contratos de prepago —los suyos viven en Valle de Paz— y la pantalla
// mostraba todo en cero sin nada que distinguiera eso de una consulta fallida. El aviso corta:
// los demás hablarían de una cartera que no existe.
func TestSinCarteraSeDiceYNoSeAvisaDeOtraCosa(t *testing.T) {
	d := DriversDeCartera{PeriodoFacturado: "2026-10", ContratosActivos: 0, CuotaContratada: "0.00"}
	CompletarDrivers(&d)

	if len(d.Avisos) != 1 {
		t.Fatalf("se esperaba un único aviso, vinieron %d: %v", len(d.Avisos), d.Avisos)
	}
	if !strings.Contains(d.Avisos[0], "no tiene contratos de cartera activos") {
		t.Errorf("el aviso no explica el vacío: %q", d.Avisos[0])
	}
	// Y NO se habla de la brecha ni de los servicios prestados: sin cartera, las dos cosas
	// serían ruido sobre algo que no existe.
	for _, a := range d.Avisos {
		if strings.Contains(a, "no es lo FACTURADO") || strings.Contains(a, "servicio PRESTADO") {
			t.Errorf("avisó sobre una cartera inexistente: %q", a)
		}
	}
}
