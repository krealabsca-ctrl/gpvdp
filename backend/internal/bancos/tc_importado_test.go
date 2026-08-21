package bancos

// Regresión: los movimientos en dólares se quedaban en ₡0,00 (agosto 2026).
//
// La conversión a colones se disparaba SOLO al tocar el tipo de cambio (registrar cotización o
// congelar el mes). Si el estado de cuenta entraba DESPUÉS de eso, nadie convertía esos
// movimientos y quedaban con monto_crc = 0 para siempre. Con los datos reales: la cotización del
// 1 de agosto se registró a las 16:54:56 y el archivo se importó a las 16:59:17 — cinco minutos
// después— y los 16 movimientos en dólares de agosto quedaron en cero, mientras los 111 de julio
// (importados antes de registrar las cotizaciones de julio) sí se habían convertido.
//
// Ahora la importación aplica el TC del mes con el MISMO criterio de siempre: RN-11 provisional
// escalonado (día 1 para los días 1–14; promedio(día 1, día 15) del 15 en adelante) y RN-12 el
// congelado retroactivo a todo el mes.

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func servicioTC(f *fakeRepo) *Service {
	return NewService(f, nil, zap.NewNop(), true)
}

func movEn(fechas ...string) []MovimientoParaInsertar {
	out := make([]MovimientoParaInsertar, 0, len(fechas))
	for _, s := range fechas {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			panic(err)
		}
		out = append(out, MovimientoParaInsertar{Fecha: t})
	}
	return out
}

func TestAplicarTCImportadoNoTocaCuentasEnColones(t *testing.T) {
	f := &fakeRepo{cotizaciones: []Cotizacion{{Fecha: "2026-08-01", Valor: "453.59"}}}
	servicioTC(f).AplicarTCImportado(context.Background(), "emp", "CRC", movEn("2026-08-03"))
	if len(f.tcAplicado) != 0 {
		t.Fatalf("una cuenta en colones no necesita conversión; se llamó %d veces", len(f.tcAplicado))
	}
}

// El caso real: solo está la cotización del día 1 y los movimientos son de los días 1–14.
func TestAplicarTCImportadoUsaElProvisionalDelDia1(t *testing.T) {
	f := &fakeRepo{cotizaciones: []Cotizacion{{Fecha: "2026-08-01", Valor: "453.59", Fuente: "MANUAL"}}}
	servicioTC(f).AplicarTCImportado(context.Background(), "emp", "USD", movEn("2026-08-01", "2026-08-06", "2026-08-07"))

	if len(f.tcAplicado) != 1 {
		t.Fatalf("se esperaba una aplicación (un solo mes tocado); hubo %d", len(f.tcAplicado))
	}
	a := f.tcAplicado[0]
	if a.anio != 2026 || a.mes != 8 {
		t.Errorf("mes aplicado = %d-%d, quiere 2026-8", a.anio, a.mes)
	}
	if a.antes15 != "453.59" || a.desde15 != "453.59" {
		t.Errorf("sin cotización del día 15, las dos mitades usan el día 1: antes=%s desde=%s", a.antes15, a.desde15)
	}
}

// Con día 1 y día 15, del 15 en adelante manda el promedio de los dos (RN-11).
func TestAplicarTCImportadoEscalonaDesdeElDia15(t *testing.T) {
	f := &fakeRepo{cotizaciones: []Cotizacion{
		{Fecha: "2026-07-01", Valor: "456.94"},
		{Fecha: "2026-07-15", Valor: "453.07"},
	}}
	servicioTC(f).AplicarTCImportado(context.Background(), "emp", "USD", movEn("2026-07-20"))

	if len(f.tcAplicado) != 1 {
		t.Fatalf("aplicaciones = %d, quiere 1", len(f.tcAplicado))
	}
	a := f.tcAplicado[0]
	if a.antes15 != "456.94" {
		t.Errorf("días 1–14 usan el día 1: %s", a.antes15)
	}
	if a.desde15 != "455.005" {
		t.Errorf("del 15 en adelante, promedio(456.94, 453.07) = 455.005; dio %s", a.desde15)
	}
}

// Si el mes ya está CONGELADO, ese valor es inmutable y va a TODO el mes (RN-12), incluso a un
// movimiento que se importe después de haberlo congelado.
func TestAplicarTCImportadoRespetaElMesCongelado(t *testing.T) {
	congelado := "462.1767"
	f := &fakeRepo{
		tcEstado:         "CONGELADO",
		tcValorCongelado: &congelado,
		// Las cotizaciones no deben influir: manda el congelado.
		cotizaciones: []Cotizacion{{Fecha: "2026-04-01", Valor: "999.99"}},
	}
	servicioTC(f).AplicarTCImportado(context.Background(), "emp", "USD", movEn("2026-04-03", "2026-04-28"))

	if len(f.tcAplicado) != 1 {
		t.Fatalf("aplicaciones = %d, quiere 1", len(f.tcAplicado))
	}
	a := f.tcAplicado[0]
	if a.antes15 != congelado || a.desde15 != congelado {
		t.Errorf("el congelado va a todo el mes: antes=%s desde=%s, quiere %s", a.antes15, a.desde15, congelado)
	}
}

// Sin cotización del día 1 no hay base provisional: no se inventa un tipo de cambio.
func TestAplicarTCImportadoNoInventaTCSinDia1(t *testing.T) {
	f := &fakeRepo{cotizaciones: []Cotizacion{{Fecha: "2026-08-15", Valor: "453.07"}}}
	servicioTC(f).AplicarTCImportado(context.Background(), "emp", "USD", movEn("2026-08-20"))
	if len(f.tcAplicado) != 0 {
		t.Fatalf("sin día 1 no se convierte nada; se aplicó %v", f.tcAplicado)
	}
}

// Un archivo que cruza dos meses convierte cada mes con su propio TC.
func TestAplicarTCImportadoResuelveCadaMesPorSeparado(t *testing.T) {
	f := &fakeRepo{cotizaciones: []Cotizacion{
		{Fecha: "2026-07-01", Valor: "456.94"},
		{Fecha: "2026-08-01", Valor: "453.59"},
	}}
	servicioTC(f).AplicarTCImportado(context.Background(), "emp", "USD", movEn("2026-07-30", "2026-08-02"))

	if len(f.tcAplicado) != 2 {
		t.Fatalf("dos meses tocados = dos aplicaciones; hubo %d (%v)", len(f.tcAplicado), f.tcAplicado)
	}
	meses := map[int]bool{}
	for _, a := range f.tcAplicado {
		meses[a.mes] = true
	}
	if !meses[7] || !meses[8] {
		t.Errorf("se esperaban julio y agosto; hubo %v", f.tcAplicado)
	}
}

func TestAplicarTCImportadoConCeroMovimientosNoHaceNada(t *testing.T) {
	f := &fakeRepo{cotizaciones: []Cotizacion{{Fecha: "2026-08-01", Valor: "453.59"}}}
	servicioTC(f).AplicarTCImportado(context.Background(), "emp", "USD", nil)
	if len(f.tcAplicado) != 0 {
		t.Fatalf("sin movimientos no hay nada que convertir")
	}
}
