package bancos

// Tests del barrido de huellas: qué se concilia, qué se reporta y qué NO se toca.

import (
	"context"
	"strings"
	"testing"
)

// conciliadorFake responde por huella según lo que se le programe.
type conciliadorFake struct {
	porHuella map[string]ResultadoHuella
	llamados  int
	// monedasVistas guarda la moneda con que se llamó por cada movimiento. CxP no puede comparar
	// monedas si Bancos no se la manda, y ese olvido no rompe nada visible: la conciliación
	// seguiría andando y solo dejaría de mirar la unidad.
	monedasVistas map[string]string
}

// Sin el guion, igual que el servicio real: el banco lo borra y un pre-filtro con guion descarta
// justo los movimientos a examinar. Un fake que mienta acá deja pasar la regresión.
func (c *conciliadorFake) PrefijoHuella() string { return "CXP" }

func (c *conciliadorFake) ConciliarHuella(_ context.Context, _, descripcion, montoBanco, monedaBanco, _ string) (ResultadoHuella, error) {
	c.llamados++
	for huella, res := range c.porHuella {
		if strings.Contains(descripcion, huella) {
			if c.monedasVistas == nil {
				c.monedasVistas = map[string]string{}
			}
			c.monedasVistas[huella] = monedaBanco
			res.MontoBanco = montoBanco
			return res, nil
		}
	}
	return ResultadoHuella{Veredicto: HuellaSinHuella}, nil
}

func TestConciliarCxPSinPuertoNoHaceNada(t *testing.T) {
	// Sin CxP conectado el barrido calla: no es un error, simplemente no aplica.
	rep, err := servicioSaldos(&fakeRepo{}).ConciliarCxP(context.Background(), "e1", "", "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rep.Disponible || rep.Examinados != 0 {
		t.Errorf("reporte = %+v, quiere no disponible y 0 examinados", rep)
	}
}

func TestConciliarCxPBarrido(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{movsConHuella: []MovimientoConHuella{
		{ID: "m1", Fecha: "2026-07-31", Cuenta: "BN Colones", Debito: "118650.00",
			Descripcion: "DEBITO MASIVO SINPE (PAGO FACTURA 025786 CXP-AAAAAAAAAAAA)"},
		{ID: "m2", Fecha: "2026-07-31", Cuenta: "BN Colones", Debito: "50000.00",
			Descripcion: "PAGO FACTURA 000123 CXP-BBBBBBBBBBBB"},
		{ID: "m3", Fecha: "2026-07-31", Cuenta: "BAC", Debito: "9000.00",
			Descripcion: "PAGO FACTURA 000999 CXP-CCCCCCCCCCCC"},
	}}
	conc := &conciliadorFake{porHuella: map[string]ResultadoHuella{
		"CXP-AAAAAAAAAAAA": {Veredicto: HuellaConciliado, Huella: "CXP-AAAAAAAAAAAA",
			DocumentoID: "doc-1", Consecutivo: "025786", Proveedor: "Gas Tomza",
			ConceptoID: "con-1", ClasificacionID: "clas-1", MontoEsperado: "118650.00"},
		"CXP-BBBBBBBBBBBB": {Veredicto: HuellaMontoDiferente, Huella: "CXP-BBBBBBBBBBBB",
			DocumentoID: "doc-2", Consecutivo: "000123", MontoEsperado: "47500.00"},
		"CXP-CCCCCCCCCCCC": {Veredicto: HuellaSinDocumento, Huella: "CXP-CCCCCCCCCCCC"},
	}}
	svc := servicioSaldos(repo)
	svc.SetConciliadorCxP(conc)

	rep, err := svc.ConciliarCxP(ctx, "e1", "", "u1")
	if err != nil {
		t.Fatalf("barrido: %v", err)
	}
	if !rep.Disponible {
		t.Error("con CxP conectado debería declararse disponible")
	}
	if rep.Examinados != 3 || rep.Conciliados != 1 || rep.MontoDiferente != 1 || rep.SinDocumento != 1 {
		t.Errorf("reporte: %d examinados, %d conciliados, %d monto distinto, %d sin documento",
			rep.Examinados, rep.Conciliados, rep.MontoDiferente, rep.SinDocumento)
	}
	if len(rep.Detalle) != 3 {
		t.Fatalf("detalle = %d líneas, quiere 3", len(rep.Detalle))
	}

	t.Run("solo el conciliado deja enlace", func(t *testing.T) {
		if repo.yaEnlazados["m1"] != "doc-1" {
			t.Errorf("m1 debería quedar enlazado a doc-1, quedó %q", repo.yaEnlazados["m1"])
		}
		if _, hay := repo.yaEnlazados["m2"]; hay {
			t.Error("el de monto distinto NO debería enlazarse: es un hallazgo, no un pago confirmado")
		}
		if _, hay := repo.yaEnlazados["m3"]; hay {
			t.Error("el sin documento no debería enlazarse")
		}
	})

	t.Run("el conciliado queda clasificado con el concepto del documento", func(t *testing.T) {
		if !rep.Detalle[0].Clasificado {
			t.Error("m1 traía concepto del documento: debería reportarse clasificado")
		}
	})

	t.Run("el detalle del monto distinto trae las dos cifras", func(t *testing.T) {
		l := rep.Detalle[1]
		if l.Veredicto != HuellaMontoDiferente || l.MontoBanco != "50000.00" || l.MontoEsperado != "47500.00" {
			t.Errorf("línea = %+v", l)
		}
	})

	t.Run("un segundo barrido no vuelve a contar lo ya enlazado", func(t *testing.T) {
		rep2, err := svc.ConciliarCxP(ctx, "e1", "", "u1")
		if err != nil {
			t.Fatalf("segundo barrido: %v", err)
		}
		if rep2.Conciliados != 0 {
			t.Errorf("conciliados en el 2.º barrido = %d, quiere 0 (idempotente)", rep2.Conciliados)
		}
	})
}

// ── La moneda del movimiento (16-oct-2026) ──────────────────────────────────────────────────

// Bancos tiene que mandarle a CxP la moneda de la CUENTA junto con el débito.
//
// CxP es quien compara —conoce la moneda de la factura— pero no puede comparar lo que no recibe.
// Si Bancos dejara de mandarla, nada fallaría de forma visible: la conciliación seguiría andando
// y solo dejaría de mirar la unidad, que es el modo de falla silencioso de siempre. Hay cinco
// cuentas bancarias en dólares.
func TestElBarridoLeMandaACxPLaMonedaDeLaCuenta(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{movsConHuella: []MovimientoConHuella{
		{ID: "m1", Fecha: "2026-07-31", Cuenta: "BN Colones", Debito: "118650.00", Moneda: "CRC",
			Descripcion: "PAGO FACTURA 025786 CXP-AAAAAAAAAAAA"},
		{ID: "m2", Fecha: "2026-07-31", Cuenta: "Davivienda Dólares", Debito: "1500.00", Moneda: "USD",
			Descripcion: "PAGO FACTURA 000123 CXP-BBBBBBBBBBBB"},
	}}
	conc := &conciliadorFake{porHuella: map[string]ResultadoHuella{
		"CXP-AAAAAAAAAAAA": {Veredicto: HuellaConciliado, Huella: "CXP-AAAAAAAAAAAA",
			DocumentoID: "doc-1", Consecutivo: "025786", MontoEsperado: "118650.00",
			MonedaEsperada: "CRC"},
		// La factura está en colones y el débito salió de la cuenta en dólares: números parecidos,
		// hechos incomparables. CxP lo rechaza y Bancos tiene que contarlo aparte.
		"CXP-BBBBBBBBBBBB": {Veredicto: HuellaMonedaDiferente, Huella: "CXP-BBBBBBBBBBBB",
			DocumentoID: "doc-2", Consecutivo: "000123", MontoEsperado: "1500.00",
			MonedaEsperada: "CRC"},
	}}
	svc := servicioSaldos(repo)
	svc.SetConciliadorCxP(conc)

	rep, err := svc.ConciliarCxP(ctx, "e1", "", "u1")
	if err != nil {
		t.Fatalf("ConciliarCxP: %v", err)
	}

	if conc.monedasVistas["CXP-AAAAAAAAAAAA"] != "CRC" {
		t.Errorf("a CxP le llegó la moneda %q para el movimiento en colones, se esperaba CRC",
			conc.monedasVistas["CXP-AAAAAAAAAAAA"])
	}
	if conc.monedasVistas["CXP-BBBBBBBBBBBB"] != "USD" {
		t.Errorf("a CxP le llegó la moneda %q para el movimiento en dólares, se esperaba USD",
			conc.monedasVistas["CXP-BBBBBBBBBBBB"])
	}

	// El veredicto nuevo se cuenta aparte: mezclarlo con MONTO_DIFERENTE diría que la cifra no
	// calza, cuando el problema es que no son la misma unidad.
	if rep.MonedaDiferente != 1 {
		t.Errorf("moneda_diferente = %d, se esperaba 1", rep.MonedaDiferente)
	}
	if rep.MontoDiferente != 0 {
		t.Errorf("monto_diferente = %d: la moneda distinta no es un monto distinto", rep.MontoDiferente)
	}
	if rep.Conciliados != 1 {
		t.Errorf("conciliados = %d, se esperaba 1 (solo el de colones)", rep.Conciliados)
	}

	// El detalle tiene que traer las DOS monedas, o el hallazgo dice que algo no calza sin decir
	// contra qué.
	var visto bool
	for _, d := range rep.Detalle {
		if d.Veredicto != HuellaMonedaDiferente {
			continue
		}
		visto = true
		if d.MonedaBanco != "USD" || d.MonedaEsperada != "CRC" {
			t.Errorf("el detalle no nombra las dos monedas: banco %q, factura %q",
				d.MonedaBanco, d.MonedaEsperada)
		}
	}
	if !visto {
		t.Error("el hallazgo de moneda no llegó al detalle del reporte")
	}
}
