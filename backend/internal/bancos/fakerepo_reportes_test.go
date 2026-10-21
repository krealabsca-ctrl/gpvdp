package bancos

import (
	"context"
	"fmt"
)

// Stubs de fakeRepo para cuadre/dashboard.

func (f *fakeRepo) Cuadre(context.Context, string, string) ([]CuadreRow, error) { return nil, nil }

// TotalesPeriodo devuelve lo que se le ponga en `totales` (por período); en cero, ceros.
func (f *fakeRepo) TotalesPeriodo(_ context.Context, _, periodo string) (TotalesEbitda, error) {
	if t, ok := f.totales[periodo]; ok {
		return t, nil
	}
	return TotalesEbitda{}, nil
}

// TotalesPeriodoHastaDia responde por TRAMO cuando la prueba lo programó («2026-10:19»), y si no,
// por el mes entero. Dos mapas y no uno: lo que hay que poder comprobar es que el servicio pide el
// MISMO día para los tres períodos que compara, y con una sola tabla por mes eso sería invisible.
func (f *fakeRepo) TotalesPeriodoHastaDia(_ context.Context, _, periodo string, dia int) (TotalesEbitda, error) {
	f.diasPedidos = append(f.diasPedidos, fmt.Sprintf("%s:%d", periodo, dia))
	if t, ok := f.totalesHastaDia[fmt.Sprintf("%s:%d", periodo, dia)]; ok {
		return t, nil
	}
	if t, ok := f.totales[periodo]; ok {
		return t, nil
	}
	return TotalesEbitda{}, nil
}

func (f *fakeRepo) UltimoDiaCargado(_ context.Context, _, periodo string) (int, error) {
	return f.ultimoDiaCargado[periodo], nil
}

func (f *fakeRepo) ConceptosSinNaturaleza(context.Context, string) (int, error) {
	return f.conceptosSinNaturaleza, nil
}
