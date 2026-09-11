package bancos

import (
	"context"
	"testing"
)

// SIN PARTIDAS ELEGIDAS NO SE PIDE NADA.
//
// El reflejo natural —«sin filtro, devolver todo»— acá traería el día a día de las 168 partidas de
// la empresa más grande por todo el rango: mucha plata en pantalla y ninguna respuesta. Y sobre
// todo, ni siquiera se toca la base.
func TestSerieDiariaSinPartidasNoConsultaNada(t *testing.T) {
	repo := &fakeRepo{serieDiaria: []SerieDiariaPartida{{ClasificacionID: "no-deberia-venir"}}}
	svc := NewService(repo, nil, nil, false)

	for _, ids := range [][]string{nil, {}, {""}, {"  ", ""}} {
		res, err := svc.SerieDiariaDePartidas(context.Background(), "emp-1", "2026-08", "2026-09", ids)
		if err != nil {
			t.Fatalf("ids=%v: %v", ids, err)
		}
		if len(res.Partidas) != 0 {
			t.Errorf("ids=%v: devolvió %d partida(s); sin selección no se devuelve nada", ids, len(res.Partidas))
		}
		if repo.diarioPedido != nil {
			t.Errorf("ids=%v: se llamó al repositorio con %v; no debería consultarse", ids, repo.diarioPedido)
		}
	}
}

// Las partidas viajan sin repetidos, conservando el orden, y acotadas.
func TestSerieDiariaLimpiaYAcotaLoQuePide(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil, nil, false)

	_, err := svc.SerieDiariaDePartidas(context.Background(), "emp-1", "2026-08", "2026-09",
		[]string{"b", "a", "b", "", "  ", "a", "c"})
	if err != nil {
		t.Fatal(err)
	}
	quiero := []string{"b", "a", "c"}
	if len(repo.diarioPedido) != len(quiero) {
		t.Fatalf("pidió %v, quería %v (sin vacíos ni repetidos, en el orden de la pantalla)", repo.diarioPedido, quiero)
	}
	for i := range quiero {
		if repo.diarioPedido[i] != quiero[i] {
			t.Errorf("posición %d: pidió %q, quería %q", i, repo.diarioPedido[i], quiero[i])
		}
	}

	// El tope existe porque una gráfica con veinte curvas no se lee.
	muchas := make([]string, MaxPartidasDiario+5)
	for i := range muchas {
		muchas[i] = string(rune('a' + i))
	}
	if _, err := svc.SerieDiariaDePartidas(context.Background(), "emp-1", "2026-08", "2026-09", muchas); err != nil {
		t.Fatal(err)
	}
	if len(repo.diarioPedido) != MaxPartidasDiario {
		t.Errorf("pidió %d partidas, el tope es %d", len(repo.diarioPedido), MaxPartidasDiario)
	}
}

// «Días con movimiento» cuenta días DISTINTOS del conjunto, no la suma por partida: dos partidas
// que se movieron el mismo día son UN día con movimiento. Sumarlas inflaría el número y haría
// parecer constante un gasto que fue de golpe, que es justo lo contrario de lo que se quiere ver.
func TestSerieDiariaCuentaDiasDistintos(t *testing.T) {
	repo := &fakeRepo{serieDiaria: []SerieDiariaPartida{
		{
			ClasificacionID: "a", Clasificacion: "Internet",
			Dias: []DiaDePartida{
				{Fecha: "2026-08-05", Monto: "1000", Movs: 1},
				{Fecha: "2026-08-20", Monto: "1000", Movs: 1},
			},
		},
		{
			ClasificacionID: "b", Clasificacion: "Licencias TI",
			Dias: []DiaDePartida{
				{Fecha: "2026-08-05", Monto: "500", Movs: 1}, // el MISMO día que Internet
				{Fecha: "2026-08-28", Monto: "500", Movs: 1},
			},
		},
	}}
	svc := NewService(repo, nil, nil, false)

	res, err := svc.SerieDiariaDePartidas(context.Background(), "emp-1", "2026-08", "2026-08", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	// 4 puntos en total, pero solo 3 días distintos: el 5, el 20 y el 28.
	if res.DiasConMovimiento != 3 {
		t.Errorf("dias_con_movimiento = %d, quería 3 (el 5 se comparte entre las dos partidas)", res.DiasConMovimiento)
	}
	if len(res.Partidas) != 2 {
		t.Errorf("partidas = %d, quería 2", len(res.Partidas))
	}
}

// El rango viaja tal cual a la respuesta: la pantalla lo usa para rotular el eje, y si dijera otra
// cosa que lo consultado el gráfico mentiría sobre qué período está mostrando.
func TestSerieDiariaDevuelveElRangoConsultado(t *testing.T) {
	svc := NewService(&fakeRepo{}, nil, nil, false)
	res, err := svc.SerieDiariaDePartidas(context.Background(), "emp-1", "2026-04", "2026-09", []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Desde != "2026-04" || res.Hasta != "2026-09" {
		t.Errorf("rango = %s..%s, quería 2026-04..2026-09", res.Desde, res.Hasta)
	}
	if res.Partidas == nil {
		t.Error("Partidas es nil: sale como null en JSON y el frontend revienta al mapearlo")
	}
}
