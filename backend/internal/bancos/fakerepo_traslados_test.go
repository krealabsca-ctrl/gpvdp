package bancos

import (
	"context"

	"github.com/shopspring/decimal"
)

// Stubs de fakeRepo para traslados/overnight y cierre de período.

func (f *fakeRepo) PropuestasTraslados(context.Context, string, string, decimal.Decimal) ([]PropuestaTraslado, error) {
	return nil, nil
}
func (f *fakeRepo) MovimientoParaTraslado(context.Context, string, string) (MovTraslado, error) {
	return MovTraslado{}, nil
}
func (f *fakeRepo) EmparejarTraslado(context.Context, string, string, string) error    { return nil }
func (f *fakeRepo) DesemparejarTraslado(context.Context, string, string) error         { return nil }
func (f *fakeRepo) CerrarPeriodo(context.Context, string, int, int, int, string) error { return nil }
func (f *fakeRepo) PeriodoCerrado(context.Context, string, int, int) (bool, error) {
	return f.cerrado, nil
}
