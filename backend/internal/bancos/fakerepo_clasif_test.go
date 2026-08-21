package bancos

import "context"

// Stubs de fakeRepo para los métodos de clasificación/movimientos/catálogo.
// Los tests de dedup no ejercitan estos caminos; devuelven valores neutros.

func (f *fakeRepo) ListarReglas(context.Context, string) ([]Regla, error) { return f.reglasCat, nil }
func (f *fakeRepo) ListarMovimientos(context.Context, string, FiltrosMovimientos) (ListaMovimientos, error) {
	return ListaMovimientos{}, nil
}
func (f *fakeRepo) ResumenFiltro(context.Context, string, FiltrosMovimientos, string) ([]ResumenFiltroRow, error) {
	return f.resumenFiltro, nil
}
func (f *fakeRepo) MovimientosDeImportacion(context.Context, string, string) ([]MovParaClasificar, error) {
	return nil, nil
}
func (f *fakeRepo) MovimientosNoIdentificados(context.Context, string) ([]MovParaClasificar, error) {
	return nil, nil
}
func (f *fakeRepo) AplicarClasificaciones(context.Context, string, []MovClasifUpdate) (int, error) {
	return 0, nil
}
func (f *fakeRepo) ReclasificarMovimiento(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeRepo) CrearRegla(_ context.Context, _ string, r NuevaRegla) (string, error) {
	f.reglasCreadas++
	f.ultimaRegla = r
	return "regla-1", nil
}
func (f *fakeRepo) ListarConceptos(context.Context, string, bool) ([]Concepto, error) {
	return f.conceptosCat, nil
}
func (f *fakeRepo) ListarClasificaciones(context.Context, string, bool) ([]ClasificacionItem, error) {
	return f.clasifsCat, nil
}
func (f *fakeRepo) CrearConcepto(_ context.Context, _, nombre string, visibleCxP bool) (Concepto, error) {
	f.conceptosCreados++
	c := Concepto{ID: "con-nuevo", Nombre: nombre, VisibleCxP: visibleCxP}
	f.conceptosCat = append(f.conceptosCat, c)
	return c, nil
}
func (f *fakeRepo) CrearClasificacion(_ context.Context, _, conceptoID, nombre, _ string) (ClasificacionItem, error) {
	f.clasifsCreadas++
	cl := ClasificacionItem{ID: "clas-nueva", ConceptoID: conceptoID, Nombre: nombre}
	f.clasifsCat = append(f.clasifsCat, cl)
	return cl, nil
}
