package cxp

import (
	"context"

	"github.com/shopspring/decimal"
)

// Stubs de fakeRepo para el catálogo de prioridades (mig 0121).
//
// `codigosPrioridad` es el único con sustancia: es lo que el servicio consulta para validar que
// una factura no se marque con un código inventado, y esa validación SÍ es lógica del servicio.
// El resto del catálogo (crear, editar, eliminar) es SQL y se prueba contra Postgres.

func (f *fakeRepo) Prioridades(context.Context, string, bool) ([]Prioridad, error) {
	return f.prioridades, nil
}

func (f *fakeRepo) CrearPrioridad(_ context.Context, _ string, in PrioridadInput) (Prioridad, error) {
	return Prioridad{Codigo: in.Codigo, Nombre: in.Nombre, Orden: in.Orden,
		Urgente: in.Urgente, Tono: in.Tono, Activa: in.Activa}, nil
}

func (f *fakeRepo) ActualizarPrioridad(_ context.Context, _, id string, in PrioridadInput) (Prioridad, error) {
	return Prioridad{ID: id, Nombre: in.Nombre, Orden: in.Orden,
		Urgente: in.Urgente, Tono: in.Tono, Activa: in.Activa}, nil
}

func (f *fakeRepo) EliminarPrioridad(context.Context, string, string) error { return nil }

func (f *fakeRepo) CodigosDePrioridadValidos(context.Context, string) ([]string, error) {
	return f.codigosPrioridad, nil
}

// Las firmas del corte (constancia de pago). Stub: lo que de verdad se protege —que salgan de la
// auditoría y no de nombres tecleados— es SQL, y se prueba contra Postgres.
func (f *fakeRepo) FirmasDelLote(context.Context, string, string) ([]Firma, string, error) {
	return f.firmasDelLote, f.fechaPagoDelLote, nil
}

// El cronograma de pago (mig 0122). Stubs: lo que de verdad importa —la cascada de cuatro restas—
// es matemática pura y se prueba en cronograma_test.go contra las cifras del archivo real.
func (f *fakeRepo) LineasDelCronograma(context.Context, string, string) ([]FilaCronogramaCruda, error) {
	return f.lineasCronograma, nil
}
func (f *fakeRepo) TCDelCronograma(context.Context, string, string) (decimal.Decimal, error) {
	return f.tcCronograma, nil
}
func (f *fakeRepo) GuardarTCDelCronograma(_ context.Context, _, _ string, tc decimal.Decimal) error {
	f.tcCronograma = tc
	return nil
}
func (f *fakeRepo) AsignarCuentaDePago(_ context.Context, _, _, _, cuentaID string) (int, error) {
	f.cuentaAsignada = cuentaID
	return 1, nil
}
func (f *fakeRepo) MarcarPorFuera(_ context.Context, _, _, _ string, porFuera bool, _ string) (int, error) {
	f.marcoPorFuera = porFuera
	return 1, nil
}
