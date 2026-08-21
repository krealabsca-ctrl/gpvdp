package bancos

import "context"

// Stubs de fakeRepo para Proyecciones (Fase C). Los tests de dedup no ejercitan
// estos caminos; devuelven valores neutros.

func (f *fakeRepo) SendaIngresos(context.Context, string, string) ([]DiaMonto, error) {
	return nil, nil
}
func (f *fakeRepo) SendasIngresosRango(context.Context, string, []string) ([]SendaMes, error) {
	return nil, nil
}
func (f *fakeRepo) IngresosPorClasificacion(context.Context, string, string) ([]LineaIngreso, error) {
	return nil, nil
}
func (f *fakeRepo) GuardarEscenario(context.Context, string, EscenarioNuevo) (string, error) {
	return "esc-1", nil
}
func (f *fakeRepo) ListarEscenarios(context.Context, string, string) ([]EscenarioGuardado, error) {
	return nil, nil
}
