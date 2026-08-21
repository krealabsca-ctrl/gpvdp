package bancos

import (
	"context"

	"github.com/shopspring/decimal"
)

// Stubs de fakeRepo para los métodos de tipo de cambio.
//
// Leen de campos del fakeRepo cuando están puestos; en cero se comportan igual que antes, así
// que los tests que ya existían no cambian.

// tcAplicado registra cada llamada a AplicarProvisional, para poder afirmar CON QUÉ valor se
// convirtió cada mes (que es justamente la regla que se quiere probar).
type tcAplicacion struct {
	anio, mes        int
	antes15, desde15 string
}

func (f *fakeRepo) UpsertCotizacion(context.Context, string, string, decimal.Decimal, string) error {
	return nil
}
func (f *fakeRepo) EncabezadoReporte(context.Context, string, string) (string, string, string, error) {
	return "Valle de Paz Servicios Funerarios S.A.", "Sociedad Anónima", "Usuario de prueba", nil
}
func (f *fakeRepo) CotizacionesMes(context.Context, string, int, int) ([]Cotizacion, error) {
	return f.cotizaciones, nil
}
func (f *fakeRepo) EstadoTCMes(context.Context, string, int, int) (string, *string, error) {
	return f.tcEstado, f.tcValorCongelado, nil
}
func (f *fakeRepo) AplicarProvisional(_ context.Context, _ string, anio, mes int, antes15, desde15 decimal.Decimal) (int, error) {
	f.tcAplicado = append(f.tcAplicado, tcAplicacion{anio, mes, antes15.String(), desde15.String()})
	return 0, nil
}
func (f *fakeRepo) CongelarTC(context.Context, string, int, int, decimal.Decimal) (int, error) {
	return 0, nil
}
