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

// Las guardas de alcance de la puerta de Contabilidad al catálogo. `visibleCxP` deja que un test
// pruebe los dos lados: con el rubro dentro del alcance de CxP y fuera.
func (f *fakeRepo) ConceptoEsVisibleCxP(context.Context, string, string) (bool, error) {
	return f.visibleCxP, nil
}

func (f *fakeRepo) ClasificacionEsVisibleCxP(context.Context, string, string) (bool, error) {
	return f.visibleCxP, nil
}
func (f *fakeRepo) EliminarConcepto(context.Context, string, string) error { return nil }
func (f *fakeRepo) RenombrarClasificacion(context.Context, string, string, string) error {
	return nil
}
func (f *fakeRepo) ReasignarConceptoClasificacion(context.Context, string, string, string) error {
	return nil
}
func (f *fakeRepo) EliminarClasificacion(context.Context, string, string) error { return nil }

// Visibilidad por clasificación (mig 0080): `visibleClasifGuardada` deja comprobar que el servicio
// mandó el valor que le pidieron, incluido el `false` que oculta el rubro.
func (f *fakeRepo) CambiarVisibilidadCxPClasificacion(_ context.Context, _, clasificacionID string, visible bool) error {
	f.visibleClasifGuardada = [2]string{clasificacionID, map[bool]string{true: "true", false: "false"}[visible]}
	return nil
}
