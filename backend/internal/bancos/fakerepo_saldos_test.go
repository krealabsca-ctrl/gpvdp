package bancos

import "context"

// Stubs de fakeRepo para tesorería (saldos diarios y checklist de carga). Los tests que
// necesitan datos los inyectan por estos campos.

func (f *fakeRepo) SaldosDelDia(_ context.Context, _, _ string) ([]SaldoDelDia, string, error) {
	return f.saldosDia, f.hoyCR, nil
}

func (f *fakeRepo) SerieSaldos(_ context.Context, _, _ string, _ int) ([]PuntoSaldo, error) {
	return f.serieSaldos, nil
}

func (f *fakeRepo) GuardarSaldos(_ context.Context, _, fecha string, saldos []SaldoInput, _ string) (int, error) {
	f.saldosGuardados = append(f.saldosGuardados, saldos...)
	f.fechaGuardada = fecha
	return len(saldos), nil
}

func (f *fakeRepo) CargaDelPeriodo(_ context.Context, _, _ string) ([]CargaCuenta, error) {
	return f.carga, nil
}
