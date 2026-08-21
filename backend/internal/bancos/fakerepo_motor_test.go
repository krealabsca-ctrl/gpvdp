package bancos

import "context"

// Stubs de fakeRepo para el motor que aprende (Fase A). Los tests de dedup no
// ejercitan estos caminos; devuelven valores neutros.

func (f *fakeRepo) ActualizarRegla(context.Context, string, string, CambiosRegla) error { return nil }
func (f *fakeRepo) EliminarRegla(context.Context, string, string) error                 { return nil }
func (f *fakeRepo) MovimientoClasif(context.Context, string, string) (MovClasifActual, error) {
	return MovClasifActual{}, nil
}
func (f *fakeRepo) ContarNoIdentificadosConPalabra(context.Context, string, string, string) (int, error) {
	return 0, nil
}
func (f *fakeRepo) ExisteReglaConPalabra(context.Context, string, string) (bool, error) {
	return false, nil
}
func (f *fakeRepo) ClasificarMasivo(context.Context, string, []string, string, string) (int, error) {
	return 0, nil
}
func (f *fakeRepo) ResumenClasificacion(context.Context, string, string) (ResumenClasif, error) {
	return ResumenClasif{}, nil
}
