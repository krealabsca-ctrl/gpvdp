package bancos

// Lo que se prueba acá es el guardarraíl, no la aritmética: que un mes a medio clasificar NO entre
// al promedio. Con los datos reales del usuario, julio 2026 estaba al 30 % y aparentaba seis veces
// menos gasto que agosto; si el promedio lo incluyera, toda partida saldría marcada como anomalía.

import (
	"context"
	"testing"
)

func servicioAnalisis(t *testing.T, salud []SaludMes, series []TendenciaPartida) *Service {
	t.Helper()
	f := &fakeRepo{saludMeses: salud, seriePartidas: series}
	return servicioTC(f)
}

func TestAnalisisPartidasExcluyeMesesAMedioClasificar(t *testing.T) {
	t.Parallel()
	salud := []SaludMes{
		{Periodo: "2026-05", Movs: 400, PctClasificado: "100.0"},
		{Periodo: "2026-06", Movs: 420, PctClasificado: "100.0"},
		{Periodo: "2026-07", Movs: 5710, PctClasificado: "30.1"}, // el mes real que engañaba
		{Periodo: "2026-08", Movs: 1811, PctClasificado: "100.0"},
	}
	series := []TendenciaPartida{{
		ConceptoID: "c1", Concepto: "Gastos", ClasificacionID: "k1", Clasificacion: "Combustible",
		Naturaleza: "GASTO",
		Serie: []PuntoPartida{
			{Periodo: "2026-05", Monto: "1000.00", Movs: 4},
			{Periodo: "2026-06", Monto: "1000.00", Movs: 4},
			{Periodo: "2026-07", Monto: "300.00", Movs: 1},
			{Periodo: "2026-08", Monto: "2000.00", Movs: 8},
		},
	}}

	got, err := servicioAnalisis(t, salud, series).AnalisisPartidas(context.Background(), "e1", "2026-05", "2026-08")
	if err != nil {
		t.Fatalf("AnalisisPartidas: %v", err)
	}
	if got.MesesComparables != 3 {
		t.Fatalf("meses comparables = %d, se esperaban 3 (julio queda afuera)", got.MesesComparables)
	}
	if got.Meses[2].Comparable {
		t.Errorf("julio (30,1 %% clasificado) quedó marcado como comparable")
	}
	if got.Aviso == "" {
		t.Error("con un mes a medio clasificar el análisis debe avisarlo, no callarse")
	}
	if len(got.Partidas) != 1 {
		t.Fatalf("partidas = %d, se esperaba 1", len(got.Partidas))
	}
	p := got.Partidas[0]
	// El promedio se calcula sobre mayo y junio: julio queda fuera por incompleto y agosto por ser
	// el mes que se está juzgando.
	if p.Promedio != "1000.00" {
		t.Errorf("promedio = %s, se esperaba 1000.00 (si entrara julio daría 766.67 y el desvío sería falso)", p.Promedio)
	}
	if p.MesesConDato != 2 {
		t.Errorf("meses con dato = %d, se esperaban 2", p.MesesConDato)
	}
	if !p.Confiable {
		t.Error("con dos meses previos comparables el desvío sí es confiable")
	}
	if p.Ultimo != "2000.00" || p.DesvioPct != "100.0" {
		t.Errorf("último = %s, desvío = %s; se esperaba 2000.00 y 100.0", p.Ultimo, p.DesvioPct)
	}
	// El total sí incluye todo lo registrado en el rango: es lo que efectivamente salió del banco.
	if p.Total != "4300.00" {
		t.Errorf("total = %s, se esperaba 4300.00", p.Total)
	}
}

func TestAnalisisPartidasSinHistoriaNoInventaDesvio(t *testing.T) {
	t.Parallel()
	// El estado real de los datos al construir esto: un mes bueno y uno a medio clasificar.
	salud := []SaludMes{
		{Periodo: "2026-07", Movs: 5710, PctClasificado: "30.1"},
		{Periodo: "2026-08", Movs: 1811, PctClasificado: "100.0"},
	}
	series := []TendenciaPartida{{
		ConceptoID: "c1", Concepto: "Gastos", ClasificacionID: "k1", Clasificacion: "Combustible",
		Serie: []PuntoPartida{
			{Periodo: "2026-07", Monto: "300.00", Movs: 1},
			{Periodo: "2026-08", Monto: "2000.00", Movs: 8},
		},
	}}

	got, err := servicioAnalisis(t, salud, series).AnalisisPartidas(context.Background(), "e1", "2026-07", "2026-08")
	if err != nil {
		t.Fatalf("AnalisisPartidas: %v", err)
	}
	p := got.Partidas[0]
	if p.Confiable {
		t.Error("sin meses previos comparables el desvío no puede declararse confiable")
	}
	if p.DesvioPct != "0.0" || p.Promedio != "0.00" {
		t.Errorf("sin historia se esperaba promedio 0.00 y desvío 0.0; se obtuvo %s y %s", p.Promedio, p.DesvioPct)
	}
	if got.Aviso == "" {
		t.Error("con un solo mes comparable el análisis debe decir que no alcanza para comparar")
	}
}

func TestAnalisisPartidasNingunMesComparable(t *testing.T) {
	t.Parallel()
	salud := []SaludMes{
		{Periodo: "2026-07", Movs: 5710, PctClasificado: "30.1"},
		{Periodo: "2026-08", Movs: 0, PctClasificado: "0"},
	}
	got, err := servicioAnalisis(t, salud, nil).AnalisisPartidas(context.Background(), "e1", "2026-07", "2026-08")
	if err != nil {
		t.Fatalf("AnalisisPartidas: %v", err)
	}
	if got.MesesComparables != 0 {
		t.Fatalf("meses comparables = %d, se esperaba 0", got.MesesComparables)
	}
	if got.Aviso == "" {
		t.Error("sin ningún mes comparable el análisis no puede presentarse como válido")
	}
}
