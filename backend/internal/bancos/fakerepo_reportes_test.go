package bancos

import (
	"context"
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

func (f *fakeRepo) ConceptosSinNaturaleza(context.Context, string) (int, error) {
	return f.conceptosSinNaturaleza, nil
}
