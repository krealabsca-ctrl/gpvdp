package bancos

import "context"

// Stubs de fakeRepo para la administración del catálogo. Los tests de dedup no
// ejercitan estos caminos; devuelven valores neutros.

func (f *fakeRepo) RenombrarConcepto(context.Context, string, string, string) error { return nil }

// CambiarNaturaleza guarda lo aplicado y devuelve lo que había antes (por defecto NEUTRO, que es
// el default de la columna).
func (f *fakeRepo) CambiarNaturaleza(_ context.Context, _, _, naturaleza string) (string, error) {
	anterior := f.naturalezaActual
	if anterior == "" {
		anterior = NaturalezaNeutro
	}
	f.naturalezaActual = naturaleza
	return anterior, nil
}

func (f *fakeRepo) CambiarVisibilidadCxP(context.Context, string, string, bool) error {
	return nil
}
func (f *fakeRepo) EliminarConcepto(context.Context, string, string) error { return nil }
func (f *fakeRepo) RenombrarClasificacion(context.Context, string, string, string) error {
	return nil
}
func (f *fakeRepo) ReasignarConceptoClasificacion(context.Context, string, string, string) error {
	return nil
}
func (f *fakeRepo) EliminarClasificacion(context.Context, string, string) error { return nil }
