package bancos

// La comparación contra el mismo rango del año anterior.
//
// ── POR QUÉ ESTA COMPARACIÓN HACE FALTA ─────────────────────────────────────────────────────
//
// El promedio móvil que ya existía no distingue crecimiento de estacionalidad de anomalía.
// Medido sobre Memorial Pets el 27-oct-2026, con 17 meses comparables: el gasto mensual va de
// ₡7,8 M a ₡23,4 M —tres veces—, así que contra la media simple un tercio de los meses supera el
// umbral de 25 % sin que ocurra nada raro. Y el mismo mes contra el año anterior da la lectura
// OPUESTA: febrero +96 %, marzo +72 %, julio −28 %.
//
// Las dos lecturas son legítimas y contestan preguntas distintas. Por eso conviven.

import (
	"context"
	"strings"
	"testing"
)

func servicioConRangos(salud map[string][]SaludMes, series map[string][]TendenciaPartida) *Service {
	repo := &fakeRepo{saludPorRango: salud, seriePorRango: series}
	return &Service{repo: repo}
}

func mesSano(p string) SaludMes  { return SaludMes{Periodo: p, Movs: 100, PctClasificado: "95.0"} }
func mesFlojo(p string) SaludMes { return SaludMes{Periodo: p, Movs: 100, PctClasificado: "40.0"} }

func partida(id, nombre string, montos map[string]string) TendenciaPartida {
	p := TendenciaPartida{
		ClasificacionID: id, Clasificacion: nombre, Concepto: "Gastos",
		Naturaleza: "GASTO", NaturalezaDeclarada: true,
	}
	// El orden importa: el service toma el ÚLTIMO punto como «el mes que se juzga».
	for _, per := range ordenar(montos) {
		p.Serie = append(p.Serie, PuntoPartida{Periodo: per, Monto: montos[per], Movs: 1})
	}
	return p
}

func ordenar(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// EL CASO REAL, con los números de Memorial: febrero subió de 7,8 M a 15,3 M (+96 %) mientras el
// promedio móvil lo leería como «dentro de lo normal».
func TestComparacionAnualUsaElMismoRangoDelAnioAnterior(t *testing.T) {
	svc := servicioConRangos(
		map[string][]SaludMes{
			"2026-01..2026-03": {mesSano("2026-01"), mesSano("2026-02"), mesSano("2026-03")},
			"2025-01..2025-03": {mesSano("2025-01"), mesSano("2025-02"), mesSano("2025-03")},
		},
		map[string][]TendenciaPartida{
			"2026-01..2026-03": {partida("c1", "Planilla", map[string]string{
				"2026-01": "13505644", "2026-02": "15256645", "2026-03": "17050974"})},
			"2025-01..2025-03": {partida("c1", "Planilla", map[string]string{
				"2025-01": "10786868", "2025-02": "7772106", "2025-03": "9900564"})},
		})

	res, err := svc.AnalisisPartidasComparado(context.Background(), "e1", "2026-01", "2026-03")
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}
	if !res.ComparaAnual {
		t.Fatal("no quedó marcado como comparación anual")
	}
	if res.DesdeAnt != "2025-01" || res.HastaAnt != "2025-03" {
		t.Errorf("el rango anterior es %s..%s; se esperaba 2025-01..2025-03", res.DesdeAnt, res.HastaAnt)
	}
	if len(res.Partidas) != 1 {
		t.Fatalf("vinieron %d partidas", len(res.Partidas))
	}
	p := res.Partidas[0]
	// 2025: 10.786.868 + 7.772.106 + 9.900.564 = 28.459.538
	if p.TotalAnterior != "28459538.00" {
		t.Errorf("total anterior %q, se esperaba 28459538.00", p.TotalAnterior)
	}
	// 2026: 45.813.263 → +60,9 %
	if p.VariacionAnualPct != "61.0" {
		t.Errorf("variación anual %q, se esperaba 61.0", p.VariacionAnualPct)
	}
	if p.SinBaseAnterior {
		t.Error("marcó sin base cuando el año anterior sí tenía datos")
	}
}

// UNA PARTIDA NUEVA NO «CRECIÓ INFINITO»: no existía. Decirlo evita que un +∞ o un 0 se lean como
// un dato, que son las dos formas de equivocarse acá.
func TestPartidaQueNoExistiaElAnioAnterior(t *testing.T) {
	svc := servicioConRangos(
		map[string][]SaludMes{
			"2026-01..2026-02": {mesSano("2026-01"), mesSano("2026-02")},
			"2025-01..2025-02": {mesSano("2025-01"), mesSano("2025-02")},
		},
		map[string][]TendenciaPartida{
			"2026-01..2026-02": {partida("nueva", "Mercadeo digital", map[string]string{
				"2026-01": "500000", "2026-02": "700000"})},
			"2025-01..2025-02": {}, // el año pasado no existía
		})

	res, err := svc.AnalisisPartidasComparado(context.Background(), "e1", "2026-01", "2026-02")
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}
	p := res.Partidas[0]
	if !p.SinBaseAnterior {
		t.Fatal("no avisó que la partida no tenía base el año anterior")
	}
	if p.VariacionAnualPct != "" {
		t.Errorf("inventó una variación de %q sobre una base que no existe", p.VariacionAnualPct)
	}
}

// EL AÑO ANTERIOR A MEDIO CLASIFICAR MIENTE POR ABAJO: parece más barato solo porque falta
// cargarlo, y entonces este año parece un crecimiento que no ocurrió. Hay que decirlo.
func TestAvisaCuandoElAnioAnteriorEstaAMedioClasificar(t *testing.T) {
	svc := servicioConRangos(
		map[string][]SaludMes{
			"2026-01..2026-03": {mesSano("2026-01"), mesSano("2026-02"), mesSano("2026-03")},
			"2025-01..2025-03": {mesSano("2025-01"), mesFlojo("2025-02"), mesFlojo("2025-03")},
		},
		map[string][]TendenciaPartida{
			"2026-01..2026-03": {partida("c1", "Planilla", map[string]string{
				"2026-01": "100", "2026-02": "100", "2026-03": "100"})},
			"2025-01..2025-03": {partida("c1", "Planilla", map[string]string{
				"2025-01": "100", "2025-02": "10", "2025-03": "10"})},
		})

	res, err := svc.AnalisisPartidasComparado(context.Background(), "e1", "2026-01", "2026-03")
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}
	if res.MesesComparablesAnt != 1 {
		t.Errorf("meses comparables del año anterior = %d, se esperaba 1", res.MesesComparablesAnt)
	}
	if res.AvisoAnual == "" {
		t.Fatal("no avisó que el año anterior está a medio clasificar: el crecimiento se leería como real")
	}
	if !strings.Contains(res.AvisoAnual, "más caro") {
		t.Errorf("el aviso no explica el efecto: %q", res.AvisoAnual)
	}
}

// Sin nada cargado el año anterior, la comparación no sirve y hay que decirlo en vez de mostrar
// una columna de ceros con aire de dato. Es el caso de Valle de Paz y Coopeprofa, que solo tienen
// tres meses cargados (medido el 27-oct-2026).
func TestSinAnioAnteriorLoDiceEnVezDeMostrarCeros(t *testing.T) {
	svc := servicioConRangos(
		map[string][]SaludMes{
			"2026-07..2026-09": {mesSano("2026-07"), mesSano("2026-08"), mesSano("2026-09")},
			"2025-07..2025-09": {},
		},
		map[string][]TendenciaPartida{
			"2026-07..2026-09": {partida("c1", "Planilla", map[string]string{"2026-07": "100", "2026-08": "100"})},
			"2025-07..2025-09": {},
		})

	res, err := svc.AnalisisPartidasComparado(context.Background(), "e1", "2026-07", "2026-09")
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}
	if res.AvisoAnual == "" {
		t.Fatal("no avisó que no hay año anterior con qué comparar")
	}
	if !res.Partidas[0].SinBaseAnterior {
		t.Error("la partida no quedó marcada como sin base")
	}
}

// El cruce es por CLASIFICACIÓN y no por nombre: renombrar una partida no puede romper la
// comparación, y dos partidas homónimas bajo conceptos distintos no se pueden confundir.
func TestElCruceAnualVaPorClasificacionNoPorNombre(t *testing.T) {
	svc := servicioConRangos(
		map[string][]SaludMes{
			"2026-01..2026-02": {mesSano("2026-01"), mesSano("2026-02")},
			"2025-01..2025-02": {mesSano("2025-01"), mesSano("2025-02")},
		},
		map[string][]TendenciaPartida{
			"2026-01..2026-02": {partida("c1", "Gas (renombrada)", map[string]string{
				"2026-01": "600", "2026-02": "600"})},
			"2025-01..2025-02": {partida("c1", "Gas", map[string]string{
				"2025-01": "400", "2025-02": "400"})},
		})

	res, err := svc.AnalisisPartidasComparado(context.Background(), "e1", "2026-01", "2026-02")
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}
	p := res.Partidas[0]
	if p.SinBaseAnterior {
		t.Fatal("renombrar la partida rompió la comparación")
	}
	if p.VariacionAnualPct != "50.0" {
		t.Errorf("variación %q, se esperaba 50.0", p.VariacionAnualPct)
	}
}

// LOS DOS AGUJEROS DE LA COMPARACIÓN, encontrados probando contra los datos reales.
//
// La lista de partidas sale del rango ACTUAL, así que:
//
//	· una partida nueva infla el crecimiento, porque el mismo gasto antes se clasificaba con
//	  otro nombre;
//	· una partida que dejó de usarse NO TIENE FILA, así que sumar la columna del año anterior da
//	  menos que el año anterior de verdad.
//
// Medido en Memorial Pets, Q1-2026 contra Q1-2025: cinco partidas nuevas por ₡16,7 M de ₡45,8 M
// (el 36 % del trimestre), y ₡9,9 M del año anterior que no tienen dónde aparecer.
func TestAvisaDeLasPartidasSinEquivalente(t *testing.T) {
	svc := servicioConRangos(
		map[string][]SaludMes{
			"2026-01..2026-02": {mesSano("2026-01"), mesSano("2026-02")},
			"2025-01..2025-02": {mesSano("2025-01"), mesSano("2025-02")},
		},
		map[string][]TendenciaPartida{
			"2026-01..2026-02": {
				partida("sigue", "Planilla", map[string]string{"2026-01": "600", "2026-02": "600"}),
				partida("nueva", "Gas", map[string]string{"2026-01": "300", "2026-02": "300"}),
			},
			"2025-01..2025-02": {
				partida("sigue", "Planilla", map[string]string{"2025-01": "400", "2025-02": "400"}),
				partida("murio", "Combustible", map[string]string{"2025-01": "250", "2025-02": "250"}),
			},
		})

	res, err := svc.AnalisisPartidasComparado(context.Background(), "e1", "2026-01", "2026-02")
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}

	if res.PartidasSinBase != 1 || res.MontoSinBase != "600.00" {
		t.Errorf("sin base: %d partidas por %q; se esperaba 1 por 600.00",
			res.PartidasSinBase, res.MontoSinBase)
	}
	// La que desapareció NO está entre las partidas —no hay fila donde ponerla— y por eso se
	// cuenta aparte.
	if res.PartidasSoloAnt != 1 || res.MontoSoloAnt != "500.00" {
		t.Errorf("solo del año anterior: %d partidas por %q; se esperaba 1 por 500.00",
			res.PartidasSoloAnt, res.MontoSoloAnt)
	}
	for _, p := range res.Partidas {
		if p.Clasificacion == "Combustible" {
			t.Fatal("la partida que dejó de usarse apareció como fila: no tiene monto actual")
		}
	}

	// Los dos reparos se dicen, no uno solo: las dos cosas desvían la lectura en la misma
	// dirección y quedarse con una deja al lector creyendo que ya conoce el problema.
	if !strings.Contains(res.AvisoAnual, "no existían el año anterior") {
		t.Errorf("no avisó de las partidas nuevas: %q", res.AvisoAnual)
	}
	if !strings.Contains(res.AvisoAnual, "ya no aparecen") {
		t.Errorf("no avisó de las partidas que dejaron de usarse: %q", res.AvisoAnual)
	}
	if !strings.Contains(res.AvisoAnual, "da de menos") {
		t.Errorf("el aviso no explica que la columna no suma: %q", res.AvisoAnual)
	}
}

// Cuando las partidas coinciden, no hay nada que avisar: un reparo que sale siempre se ignora.
func TestSinAgujerosNoAvisaNada(t *testing.T) {
	svc := servicioConRangos(
		map[string][]SaludMes{
			"2026-01..2026-02": {mesSano("2026-01"), mesSano("2026-02")},
			"2025-01..2025-02": {mesSano("2025-01"), mesSano("2025-02")},
		},
		map[string][]TendenciaPartida{
			"2026-01..2026-02": {partida("c1", "Planilla", map[string]string{"2026-01": "600", "2026-02": "600"})},
			"2025-01..2025-02": {partida("c1", "Planilla", map[string]string{"2025-01": "400", "2025-02": "400"})},
		})

	res, err := svc.AnalisisPartidasComparado(context.Background(), "e1", "2026-01", "2026-02")
	if err != nil {
		t.Fatalf("comparar: %v", err)
	}
	if res.AvisoAnual != "" {
		t.Errorf("avisó sin motivo: %q", res.AvisoAnual)
	}
	if res.PartidasSinBase != 0 || res.PartidasSoloAnt != 0 {
		t.Errorf("contó agujeros que no hay: %d y %d", res.PartidasSinBase, res.PartidasSoloAnt)
	}
}

func TestUnAnioAntes(t *testing.T) {
	casos := map[string]string{
		"2026-01": "2025-01",
		"2026-12": "2025-12",
		"2026-02": "2025-02", // el bisiesto no mueve el mes
	}
	for in, quiero := range casos {
		if got := unAnioAntes(in); got != quiero {
			t.Errorf("unAnioAntes(%q) = %q, se esperaba %q", in, got, quiero)
		}
	}
}
