package bancos

// La reversa de una carga se prueba contra Postgres de verdad (reversa_test.go): lo que cambió es
// el SQL, y un doble probaría el doble. Estos stubs existen solo para que `fakeRepo` siga
// satisfaciendo `Repository` después de sumarle los cuatro métodos.

import "context"

func (f *fakeRepo) ListarImportaciones(_ context.Context, _ string, _ FiltrosImportaciones) (ListaImportaciones, error) {
	return ListaImportaciones{Items: []ImportacionItem{}}, nil
}

func (f *fakeRepo) EstadoDeReversa(_ context.Context, _, _ string) (EstadoDeReversa, error) {
	return EstadoDeReversa{}, ErrImportacionNoEncontrada
}

func (f *fakeRepo) RevertirImportacion(_ context.Context, _, _, _, _ string) (CambioDeReversa, error) {
	return CambioDeReversa{}, ErrImportacionNoEncontrada
}

func (f *fakeRepo) DeshacerReversaImportacion(_ context.Context, _, _ string) (CambioDeReversa, error) {
	return CambioDeReversa{}, ErrImportacionNoEncontrada
}
