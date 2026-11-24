package bancos

import (
	"context"

	"github.com/shopspring/decimal"
)

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
//
// Si el test sembró POR RANGO, se responde según el rango pedido. Lo necesita la comparación
// anual, que hace dos consultas con rangos distintos: un doble que devuelve lo mismo a las dos
// haría que la prueba pasara con cualquier implementación, incluida una que compare el rango
// contra sí mismo.
func (f *fakeRepo) SaludMeses(_ context.Context, _, desde, hasta string) ([]SaludMes, error) {
	if f.saludPorRango != nil {
		return f.saludPorRango[desde+".."+hasta], nil
	}
	return f.saludMeses, nil
}

func (f *fakeRepo) SeriePorPartida(_ context.Context, _, desde, hasta string) ([]TendenciaPartida, error) {
	if f.seriePorRango != nil {
		return f.seriePorRango[desde+".."+hasta], nil
	}
	return f.seriePartidas, nil
}

// Día a día: devuelve lo sembrado y GUARDA los ids pedidos, porque parte de lo que hay que probar
// es justamente qué le pide el service al repositorio (cuántas partidas, sin repetidos).
func (f *fakeRepo) SerieDiariaPorPartida(_ context.Context, _, _, _ string, clasificaciones []string) ([]SerieDiariaPartida, error) {
	f.diarioPedido = clasificaciones
	return f.serieDiaria, nil
}

// CarteraPendiente: lo que el test siembre. Va en decimal porque es dinero.
func (f *fakeRepo) CarteraPendiente(context.Context, string) (decimal.Decimal, decimal.Decimal, error) {
	return f.cxpVencido, f.cxpPorVencer, nil
}
