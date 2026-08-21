package nomina

import "context"

// Stubs de fakeRepo para las notificaciones de RRHH.

func (f *fakeRepo) CorreosEmpleados(context.Context, string) (map[string]string, error) {
	return f.correos, nil
}

func (f *fakeRepo) VacacionParaAviso(context.Context, string, string) (VacacionAviso, error) {
	return f.vacacionAviso, f.errVacacionAviso
}
