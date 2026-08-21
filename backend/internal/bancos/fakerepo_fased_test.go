package bancos

import (
	"context"

	"github.com/shopspring/decimal"
)

// Stubs de fakeRepo para la Fase D (parámetros por empresa, export, sync BCCR).

func (f *fakeRepo) ToleranciaTraslado(context.Context, string) (decimal.Decimal, error) {
	return ToleranciaTrasladoDefault, nil
}
func (f *fakeRepo) ActualizarTolerancia(context.Context, string, decimal.Decimal) error { return nil }
func (f *fakeRepo) MovimientosParaExport(context.Context, string, FiltrosMovimientos) ([]MovimientoExport, error) {
	return nil, nil
}
func (f *fakeRepo) CotizacionExistente(context.Context, string, string) (string, string, bool, error) {
	return "", "", false, nil
}
func (f *fakeRepo) UpsertCotizacionBCCR(context.Context, string, string, decimal.Decimal) (bool, error) {
	return true, nil
}
func (f *fakeRepo) RegistrarSyncBCCR(context.Context, BCCRSyncLog) error { return nil }
func (f *fakeRepo) UltimoSyncBCCR(context.Context, string) (*BCCRSyncLog, error) {
	return nil, nil
}
func (f *fakeRepo) EmpresasActivas(context.Context) ([]string, error) { return nil, nil }
