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

// ---- Excluir un movimiento suelto (mig 0093) ----
//
// Estos cinco están acá SOLO para que el fake siga cumpliendo la interfaz. La función se prueba
// contra PostgreSQL de verdad (excluir_pg_test.go): lo que hay que proteger es el SQL —que la
// exclusión a mano no se pise con la reversa, y que el detector no proponga comisiones legítimas
// como duplicados—, y nada de eso lo puede demostrar un doble.

func (f *fakeRepo) ExcluirMovimiento(_ context.Context, _, _, _, _ string) error { return nil }
func (f *fakeRepo) IncluirMovimiento(_ context.Context, _, _ string) error       { return nil }
func (f *fakeRepo) ExcluidosAMano(_ context.Context, _ string) ([]MovimientoExcluido, error) {
	return nil, nil
}
func (f *fakeRepo) MovimientoParaExcluir(_ context.Context, _, _ string) (MovimientoExcluido, error) {
	return MovimientoExcluido{}, nil
}
func (f *fakeRepo) DuplicadosSospechosos(_ context.Context, _, _, _ string) ([]ParDuplicado, error) {
	return nil, nil
}

// ---- Dividir un movimiento (mig 0094) ----
//
// Igual que los de excluir: el fake solo cumple la interfaz. La invariante —padre + hijos = lo que
// trajo el banco— se prueba contra PostgreSQL de verdad (dividir_pg_test.go), porque es una
// propiedad de lo que queda ESCRITO y un doble no la puede violar ni sostener.

func (f *fakeRepo) DividirMovimiento(_ context.Context, _, _ string, _ DivisionInput, _ string) (string, error) {
	return "", nil
}
func (f *fakeRepo) DeshacerDivision(_ context.Context, _, _ string) error { return nil }
func (f *fakeRepo) PartesDeMovimiento(_ context.Context, _, _ string) (Division, error) {
	return Division{}, nil
}
func (f *fakeRepo) MovimientoParaDividir(_ context.Context, _, _ string) (MovimientoDivisible, error) {
	return MovimientoDivisible{}, nil
}
