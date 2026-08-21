package bancos

import "context"

// Stubs de administración de bancos/cuentas para el fakeRepo compartido (pipeline_test.go).
func (f *fakeRepo) ListarBancos(context.Context, string, bool) ([]BancoItem, error) { return nil, nil }
func (f *fakeRepo) CrearBanco(context.Context, string, string) (BancoItem, error) {
	return BancoItem{}, nil
}
func (f *fakeRepo) RenombrarBanco(context.Context, string, string, string) error { return nil }
func (f *fakeRepo) CrearCuenta(context.Context, string, string, string, string, string) (CuentaListItem, error) {
	return CuentaListItem{}, nil
}
func (f *fakeRepo) RenombrarCuenta(context.Context, string, string, string) error { return nil }

// Correcciones del catálogo (eliminar/desactivar/fusionar): el fake no las ejerce, pero el
// fakeRepo tiene que seguir cumpliendo la interfaz Repository.
func (f *fakeRepo) ActualizarCuenta(context.Context, string, string, CambioDeCuenta) error {
	return nil
}
func (f *fakeRepo) UsoDeCuenta(context.Context, string, string) (UsoDeCuenta, error) {
	return UsoDeCuenta{}, nil
}
func (f *fakeRepo) EliminarCuenta(context.Context, string, string) error            { return nil }
func (f *fakeRepo) CambiarActivoCuenta(context.Context, string, string, bool) error { return nil }
func (f *fakeRepo) EliminarBanco(context.Context, string, string) error             { return nil }
func (f *fakeRepo) CambiarActivoBanco(context.Context, string, string, bool) error  { return nil }
func (f *fakeRepo) FusionarConceptos(context.Context, string, string, string) (ResumenFusion, error) {
	return ResumenFusion{}, nil
}
func (f *fakeRepo) FusionarClasificaciones(context.Context, string, string, string, bool) (ResumenFusion, error) {
	return ResumenFusion{}, nil
}
