package bancos

// El criterio del cruce, que es la parte que decide si una marca es verdad o es casualidad.
//
// Lo que se protege acá es la regla de oro: **ante la duda, no se marca**. Un movimiento marcado
// por error queda con un sello que dice «esto ya está resuelto» y nadie lo vuelve a revisar — es
// peor que dejarlo pendiente, porque un pendiente se ve y un falso resuelto no.

import (
	"testing"
)

func filaRep(ref, fecha, monto string) FilaReportePago {
	f := FilaReportePago{Referencia: ref, Fecha: fecha}
	if monto != "" {
		f.Monto = dec(monto)
		f.MontoTexto = f.Monto.StringFixed(2)
	}
	return f
}

func TestElCruceEncuentraPorReferenciaAunqueElBancoLaEscribaDistinto(t *testing.T) {
	candidatos := []DebitoCandidato{
		{ID: "m1", Fecha: "2026-01-15", Documento: "", Descripcion: "TEF 00123456 A FAVOR DE PROVEEDOR X", Monto: dec("125000.00")},
		{ID: "m2", Fecha: "2026-01-16", Documento: "987654", Descripcion: "PAGO", Monto: dec("80000.00")},
	}
	casos := []struct{ nombre, ref, espera string }{
		{"tal cual", "00123456", "m1"},
		{"sin los ceros de relleno", "123456", "m1"},
		{"con guion y prefijo", "REF-123456", "m1"}, // el prefijo no estorba: se busca contenida
		{"en el documento, no en la descripción", "987654", "m2"},
		{"con espacios", "987 654", "m2"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			r := CruzarReporte([]FilaReportePago{filaRep(c.ref, "2026-01-15", "125000.00")}, candidatos)
			if len(r) != 1 {
				t.Fatalf("resultados = %d", len(r))
			}
			if r[0].Veredicto != CruceUnico {
				t.Fatalf("veredicto = %s (candidatos %d), se esperaba UNICO", r[0].Veredicto, r[0].Candidatos)
			}
			if r[0].MovimientoID != c.espera {
				t.Errorf("calzó con %s, se esperaba %s", r[0].MovimientoID, c.espera)
			}
		})
	}
}

func TestSinReferenciaSeCruzaPorFechaYMontoExactos(t *testing.T) {
	candidatos := []DebitoCandidato{
		{ID: "m1", Fecha: "2026-01-15", Descripcion: "PAGO", Monto: dec("125000.00")},
		{ID: "m2", Fecha: "2026-01-16", Descripcion: "PAGO", Monto: dec("125000.00")}, // otro día
		{ID: "m3", Fecha: "2026-01-15", Descripcion: "PAGO", Monto: dec("125000.01")}, // un céntimo
	}
	r := CruzarReporte([]FilaReportePago{filaRep("", "2026-01-15", "125000.00")}, candidatos)
	if r[0].Veredicto != CruceUnico || r[0].MovimientoID != "m1" {
		t.Fatalf("veredicto = %s, movimiento = %s; se esperaba UNICO con m1", r[0].Veredicto, r[0].MovimientoID)
	}
	if r[0].Por != PorFechaYMonto {
		t.Errorf("se encontró por %q, se esperaba FECHA_Y_MONTO", r[0].Por)
	}
}

// LA prueba del criterio: con dos candidatos no se elige ninguno.
func TestConVariosCandidatosNoSeMarcaNinguno(t *testing.T) {
	// Dos débitos del mismo día por el mismo monto: pasa todo el tiempo con pagos recurrentes.
	candidatos := []DebitoCandidato{
		{ID: "m1", Fecha: "2026-01-15", Descripcion: "PAGO A", Monto: dec("125000.00")},
		{ID: "m2", Fecha: "2026-01-15", Descripcion: "PAGO B", Monto: dec("125000.00")},
	}
	r := CruzarReporte([]FilaReportePago{filaRep("", "2026-01-15", "125000.00")}, candidatos)
	if r[0].Veredicto != CruceVarios {
		t.Errorf("veredicto = %s, se esperaba VARIOS", r[0].Veredicto)
	}
	if r[0].MovimientoID != "" {
		t.Errorf("se eligió %s habiendo dos candidatos: acertar por casualidad deja una marca que nadie revisa", r[0].MovimientoID)
	}
	if r[0].Candidatos != 2 {
		t.Errorf("candidatos = %d, se esperaban 2", r[0].Candidatos)
	}
}

func TestUnaReferenciaMuyCortaNoSeBuscaDentroDelTexto(t *testing.T) {
	// La referencia «1» está contenida en casi cualquier descripción. Buscarla marcaría
	// movimientos al azar.
	candidatos := []DebitoCandidato{
		{ID: "m1", Fecha: "2026-03-01", Descripcion: "TEF 1098765 PROVEEDOR", Monto: dec("500.00")},
	}
	r := CruzarReporte([]FilaReportePago{filaRep("1", "2026-09-09", "999.00")}, candidatos)
	if r[0].Veredicto == CruceUnico {
		t.Errorf("una referencia de un carácter marcó el movimiento %s", r[0].MovimientoID)
	}
	// Pero contra el DOCUMENTO sí vale, porque ahí se compara por igualdad.
	candidatos[0].Documento = "1"
	r = CruzarReporte([]FilaReportePago{filaRep("1", "2026-09-09", "999.00")}, candidatos)
	if r[0].Veredicto != CruceUnico {
		t.Errorf("la referencia que ES el documento no calzó: %s", r[0].Veredicto)
	}
}

func TestUnMovimientoNoSeMarcaDosVecesEnLaMismaCorrida(t *testing.T) {
	// Un reporte con la misma fila repetida: el segundo renglón no puede llevarse el mismo débito.
	candidatos := []DebitoCandidato{
		{ID: "m1", Fecha: "2026-01-15", Descripcion: "TEF 123456", Monto: dec("125000.00")},
	}
	filas := []FilaReportePago{
		filaRep("123456", "2026-01-15", "125000.00"),
		filaRep("123456", "2026-01-15", "125000.00"),
	}
	r := CruzarReporte(filas, candidatos)
	if r[0].Veredicto != CruceUnico {
		t.Fatalf("la primera no calzó: %s", r[0].Veredicto)
	}
	if r[1].Veredicto == CruceUnico {
		t.Errorf("la segunda se llevó el MISMO movimiento (%s): el débito quedaría marcado dos veces", r[1].MovimientoID)
	}
}

func TestLaDiferenciaDeMontoSeDiceYNoImpideElSello(t *testing.T) {
	// El reporte dice 125.000 y el banco debitó 125.500: una comisión, un redondeo, un pago parcial.
	// Es un HALLAZGO, no una razón para perder la fila.
	candidatos := []DebitoCandidato{
		{ID: "m1", Fecha: "2026-01-15", Descripcion: "TEF 123456", Monto: dec("125500.00")},
	}
	r := CruzarReporte([]FilaReportePago{filaRep("123456", "2026-01-15", "125000.00")}, candidatos)
	if r[0].Veredicto != CruceUnico {
		t.Fatalf("veredicto = %s: una diferencia de monto no puede impedir el cruce por referencia", r[0].Veredicto)
	}
	if r[0].DiferenciaMonto != "-500.00" {
		t.Errorf("diferencia = %q, quiere -500.00", r[0].DiferenciaMonto)
	}
}

func TestLoQueNoCalzaConNadaSeDice(t *testing.T) {
	candidatos := []DebitoCandidato{
		{ID: "m1", Fecha: "2026-01-15", Descripcion: "OTRA COSA", Monto: dec("999.00")},
	}
	r := CruzarReporte([]FilaReportePago{filaRep("ZZZ999", "2026-07-01", "125000.00")}, candidatos)
	if r[0].Veredicto != CruceSinMovimiento {
		t.Errorf("veredicto = %s, se esperaba SIN_MOVIMIENTO", r[0].Veredicto)
	}
}

func TestUnaFilaIlegibleNoSeIntentaCruzar(t *testing.T) {
	mala := FilaReportePago{Referencia: "X", Motivo: "monto ilegible: \"abc\""}
	candidatos := []DebitoCandidato{{ID: "m1", Fecha: "2026-01-15", Descripcion: "X", Monto: dec("1.00")}}
	r := CruzarReporte([]FilaReportePago{mala}, candidatos)
	if r[0].Veredicto != CruceIlegible {
		t.Errorf("veredicto = %s, se esperaba ILEGIBLE", r[0].Veredicto)
	}
	if r[0].MovimientoID != "" {
		t.Error("una fila que no se pudo leer marcó un movimiento")
	}
}

func TestSinCandidatosElCruceNoRevienta(t *testing.T) {
	r := CruzarReporte([]FilaReportePago{filaRep("123456", "2026-01-15", "100.00")}, nil)
	if len(r) != 1 || r[0].Veredicto != CruceSinMovimiento {
		t.Errorf("con cero candidatos se esperaba SIN_MOVIMIENTO, dio %+v", r)
	}
}
