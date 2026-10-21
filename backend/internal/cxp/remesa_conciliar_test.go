package cxp

// Conciliar un pago AGRUPADO: un débito que cierra varias facturas.
//
// Es el momento en que el circuito se cierra, y también donde más caro sale equivocarse: si el
// monto no se verifica contra la remesa sino contra una factura, nada va a cuadrar nunca; y si se
// concilia «casi», queda un grupo de cinco facturas cerrado por una cifra que nadie mandó pagar.

import (
	"context"
	"testing"
)

func TestConciliarUnPagoAgrupado(t *testing.T) {
	ctx := context.Background()
	laRemesa := Remesa{
		ID: "rem-1", LoteID: "lote-1", LoteNumero: 29, ProveedorID: "prov-1",
		Proveedor: "ALFA SOCIEDAD ANONIMA",
		Numero:    7, Huella: "CXPR-7K2M9P4XQ1BT", MontoCRC: "400000.00", Facturas: 3,
	}
	// Tal como vuelve del banco: sin el guion.
	const delBanco = "DEBITO MASIVO SINPE ( - 1060746795 REM 0043-07 CXPR7K2M9P4XQ1BT Lote 888607 Linea 3)"

	t.Run("monto exacto: cierra las tres facturas de una", func(t *testing.T) {
		repo := &fakeRepo{
			remesaPorHuella: laRemesa, cerradas: 3,
			conceptoComun: "con-9", clasifComun: "clas-9",
		}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", delBanco, "400000.00", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaConciliado {
			t.Fatalf("veredicto = %s, quiere CONCILIADO", res.Veredicto)
		}
		if res.Facturas != 3 {
			t.Errorf("cerró %d facturas, el pago cubría 3", res.Facturas)
		}
		if res.RemesaID != "rem-1" {
			t.Errorf("remesa = %q, quiere rem-1", res.RemesaID)
		}
		// La referencia lleva el número REAL del lote. Con 0 saldría «REM 000007», que no calza
		// con nada de lo que trae el estado de cuenta y vuelve el reporte imposible de cruzar.
		if res.Consecutivo != "REM 002907" {
			t.Errorf("referencia = %q, quiere REM 002907", res.Consecutivo)
		}
		// DocumentoID vacío a propósito: enlazar el débito a una de las tres diría que esa factura
		// costó ₡400.000.
		if res.DocumentoID != "" {
			t.Errorf("DocumentoID = %q: un pago agrupado no se enlaza a UNA factura", res.DocumentoID)
		}
		if repo.capRemesaCerrada != "rem-1" {
			t.Errorf("no se intentó cerrar la remesa (capturado %q)", repo.capRemesaCerrada)
		}
		// El gasto común viaja para poder clasificar el movimiento sin inventar.
		if res.ConceptoID != "con-9" || res.ClasificacionID != "clas-9" {
			t.Errorf("falta el gasto común para clasificar el movimiento: %+v", res)
		}
	})

	t.Run("monto distinto NO cierra nada", func(t *testing.T) {
		repo := &fakeRepo{remesaPorHuella: laRemesa, cerradas: 3}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", delBanco, "399999.00", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaMontoDiferente {
			t.Fatalf("veredicto = %s, quiere MONTO_DIFERENTE", res.Veredicto)
		}
		if repo.capRemesaCerrada != "" {
			t.Error("cerró la remesa con un monto que no coincide: tres facturas quedarían dadas por pagadas")
		}
		if res.MontoEsperado != "400000.00" || res.MontoBanco != "399999.00" {
			t.Errorf("tiene que reportar las dos cifras: %+v", res)
		}
	})

	t.Run("gastos mezclados: concilia igual pero no clasifica", func(t *testing.T) {
		// El repositorio devuelve el gasto común vacío cuando las facturas no comparten uno. El
		// movimiento queda sin clasificar, que es lo honesto: no hay UN gasto que lo describa.
		repo := &fakeRepo{remesaPorHuella: laRemesa, cerradas: 3}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", delBanco, "400000.00", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaConciliado {
			t.Fatalf("veredicto = %s, quiere CONCILIADO", res.Veredicto)
		}
		if res.ClasificacionID != "" {
			t.Errorf("clasificó con %q un pago que mezcla gastos", res.ClasificacionID)
		}
	})

	t.Run("si no se movió ninguna factura no se declara conciliado", func(t *testing.T) {
		// Pasa cuando las facturas ya estaban cerradas o se anularon entre el corte y hoy.
		repo := &fakeRepo{remesaPorHuella: laRemesa, cerradas: 0}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", delBanco, "400000.00", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaSinDocumento {
			t.Errorf("veredicto = %s, quiere SIN_DOCUMENTO: no se movió nada", res.Veredicto)
		}
	})

	t.Run("un código de remesa que no existe no es un error", func(t *testing.T) {
		repo := &fakeRepo{errRemesa: ErrRemesaNoEncontrada}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", delBanco, "400000.00", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaSinDocumento || res.Huella != "CXPR-7K2M9P4XQ1BT" {
			t.Errorf("veredicto = %s, huella = %q", res.Veredicto, res.Huella)
		}
	})
}

// El camino del pago suelto no puede haberse roto al agregar el agrupado: la mayoría de los pagos
// siguen siendo de una factura.
func TestElPagoDeUnaFacturaSigueCerrandoUnaSola(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		docPorHuella: Documento{
			ID: "doc-1", Consecutivo: "00100001010000025786", Proveedor: "Gas Tomza",
			Estado: EstProgramado, ConceptoID: "con-1", ClasificacionID: "clas-1",
		},
		netoAPagar: "118650.00", filasCambio: 1,
	}
	res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1",
		"FC 025786 CXP-A1B2C3D4E5F6", "118650.00", MonedaCRC, "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Veredicto != HuellaConciliado || res.Facturas != 1 {
		t.Errorf("veredicto = %s, facturas = %d; quiere CONCILIADO y 1", res.Veredicto, res.Facturas)
	}
	if res.RemesaID != "" {
		t.Errorf("un pago suelto no tiene remesa (trae %q)", res.RemesaID)
	}
	if repo.capRemesaCerrada != "" {
		t.Error("intentó cerrar una remesa para un pago de una sola factura")
	}
}
