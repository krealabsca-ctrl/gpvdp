package bancos

// Cargar histórico se prueba contra Postgres de verdad (historico_test.go): lo que decide si la
// plata entra bien es el SQL —el anti-duplicado, el índice parcial de la mig 0086 y la transacción
// que crea una importación por cuenta— y un doble probaría el doble. Estos stubs existen solo para
// que `fakeRepo` siga satisfaciendo `Repository`.

import "context"

func (f *fakeRepo) CuentasHistorico(_ context.Context, _ string) ([]CuentaHistorica, error) {
	return nil, nil
}

func (f *fakeRepo) CrearCargaHistorica(_ context.Context, _, _, _ string, _ []byte, _ string) (string, error) {
	return "", nil
}

func (f *fakeRepo) CargaHistorica(_ context.Context, _, _ string) (CargaHistoricaRow, error) {
	return CargaHistoricaRow{}, ErrCargaHistoricaNoEncontrada
}

func (f *fakeRepo) BloqueosDeCargaHistorica(_ context.Context, _ string, _ []string, _, _ []int) ([]string, []string, error) {
	return nil, nil, nil
}

func (f *fakeRepo) ConfirmarCargaHistorica(_ context.Context, _, _, _, _, _ string, _ []LoteHistoricoCuenta) ([]ResultadoLoteHistorico, error) {
	return nil, ErrCargaHistoricaNoEncontrada
}
