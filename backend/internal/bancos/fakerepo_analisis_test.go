package bancos

import "context"

// Stubs de fakeRepo para el análisis visual (Fase B). Los tests de dedup no
// ejercitan estos caminos; devuelven valores neutros.

func (f *fakeRepo) SerieMensual(context.Context, string, string, string) ([]SerieMensualPunto, error) {
	return nil, nil
}
func (f *fakeRepo) CalendarioDiario(context.Context, string, string) ([]DiaCalendario, error) {
	return nil, nil
}
func (f *fakeRepo) ResumenPorCuenta(context.Context, string, string) ([]CuentaResumen, error) {
	return nil, nil
}

// Análisis de partidas en el tiempo: devuelven lo que el test siembre.
func (f *fakeRepo) SaludMeses(context.Context, string, string, string) ([]SaludMes, error) {
	return f.saludMeses, nil
}

func (f *fakeRepo) SeriePorPartida(context.Context, string, string, string) ([]TendenciaPartida, error) {
	return f.seriePartidas, nil
}

// Día a día: devuelve lo sembrado y GUARDA los ids pedidos, porque parte de lo que hay que probar
// es justamente qué le pide el service al repositorio (cuántas partidas, sin repetidos).
func (f *fakeRepo) SerieDiariaPorPartida(_ context.Context, _, _, _ string, clasificaciones []string) ([]SerieDiariaPartida, error) {
	f.diarioPedido = clasificaciones
	return f.serieDiaria, nil
}
