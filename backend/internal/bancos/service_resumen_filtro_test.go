package bancos

import (
	"context"
	"testing"
)

func TestResumenFiltro(t *testing.T) {
	t.Parallel()

	t.Run("agrega por padre y deriva el neto como crédito menos débito", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{resumenFiltro: []ResumenFiltroRow{
			{PadreID: "c1", Padre: "Ingresos", HijoID: "k1", Hijo: "Datáfonos", CreditoSum: dec("97212973.28"), DebitoSum: dec("0"), Movs: 1398},
			{PadreID: "c2", Padre: "Gastos", HijoID: "k2", Hijo: "Proveedores", CreditoSum: dec("0"), DebitoSum: dec("4272560.00"), Movs: 350},
			{PadreID: "c2", Padre: "Gastos", HijoID: "k3", Hijo: "Servicios", CreditoSum: dec("0"), DebitoSum: dec("1000000.00"), Movs: 10},
		}}
		svc := NewService(repo, nil, nil, false)

		got, err := svc.ResumenFiltro(context.Background(), "e1", FiltrosMovimientos{}, AgruparConcepto)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if got.Movs != 1758 {
			t.Errorf("movs = %d, se esperaba 1758 (1398+350+10)", got.Movs)
		}
		if got.TotalCredito != "97212973.28" {
			t.Errorf("crédito = %s", got.TotalCredito)
		}
		if got.TotalDebito != "5272560" {
			t.Errorf("débito = %s, se esperaba 5272560", got.TotalDebito)
		}
		if got.Neto != "91940413.28" {
			t.Errorf("neto = %s, se esperaba 91940413.28 (crédito − débito)", got.Neto)
		}
		if len(got.Conceptos) != 2 {
			t.Fatalf("conceptos = %d, se esperaban 2", len(got.Conceptos))
		}
		// Gastos agrupa sus dos clasificaciones en un solo padre.
		gastos := got.Conceptos[1]
		if gastos.Concepto != "Gastos" || gastos.Movs != 360 || gastos.Debito != "5272560" {
			t.Errorf("gastos = %+v", gastos)
		}
		if len(gastos.Clasificaciones) != 2 {
			t.Errorf("clasificaciones de gastos = %d, se esperaban 2", len(gastos.Clasificaciones))
		}
	})

	t.Run("el conteo NO se deriva de los lados: un movimiento sin lado igual se cuenta", func(t *testing.T) {
		t.Parallel()
		// Fila con débito y crédito en cero (una importación defectuosa). Si el conteo
		// se derivara de «débitos + créditos» este movimiento desaparecería del
		// encabezado mientras la lista sí lo muestra, y los números no cuadrarían.
		repo := &fakeRepo{resumenFiltro: []ResumenFiltroRow{
			{PadreID: "", Padre: "(sin concepto)", HijoID: "", Hijo: "(sin clasificación)", CreditoSum: dec("0"), DebitoSum: dec("0"), Movs: 3},
		}}
		svc := NewService(repo, nil, nil, false)

		got, err := svc.ResumenFiltro(context.Background(), "e1", FiltrosMovimientos{}, AgruparConcepto)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if got.Movs != 3 {
			t.Errorf("movs = %d, se esperaban 3", got.Movs)
		}
		if got.Neto != "0" {
			t.Errorf("neto = %s, se esperaba 0", got.Neto)
		}
	})

	t.Run("un agrupamiento desconocido cae en concepto, no en SQL arbitrario", func(t *testing.T) {
		t.Parallel()
		svc := NewService(&fakeRepo{}, nil, nil, false)
		got, err := svc.ResumenFiltro(context.Background(), "e1", FiltrosMovimientos{}, "m.empresa_id; DROP TABLE")
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if got.Agrupar != AgruparConcepto {
			t.Errorf("agrupar = %q, se esperaba %q", got.Agrupar, AgruparConcepto)
		}
	})

	t.Run("cuenta es un agrupamiento válido y se respeta", func(t *testing.T) {
		t.Parallel()
		svc := NewService(&fakeRepo{}, nil, nil, false)
		got, err := svc.ResumenFiltro(context.Background(), "e1", FiltrosMovimientos{}, AgruparCuenta)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if got.Agrupar != AgruparCuenta {
			t.Errorf("agrupar = %q, se esperaba %q", got.Agrupar, AgruparCuenta)
		}
		if got.Conceptos == nil {
			t.Error("conceptos no debería ser nil con repo vacío (el handler lo normaliza, pero el servicio no debe devolver nil sorpresivo)")
		}
	})
}

// El agrupamiento sale de una whitelist de literales: ningún texto del cliente llega al SQL.
func TestAgrupamientoResumenEsWhitelist(t *testing.T) {
	t.Parallel()
	porCuenta := agrupamientoResumen(AgruparCuenta)
	porConcepto := agrupamientoResumen("cualquier-cosa")
	if porCuenta.groupBy == porConcepto.groupBy {
		t.Error("cuenta y concepto deberían agrupar distinto")
	}
	if porConcepto.groupBy != "m.concepto_id, co.nombre, m.clasificacion_id, cl.nombre" {
		t.Errorf("group by de concepto = %q", porConcepto.groupBy)
	}
}
