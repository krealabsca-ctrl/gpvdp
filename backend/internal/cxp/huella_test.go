package cxp

// Tests de la huella Bancos↔CxP: lo que viaja al banco y lo que se acepta al volver.

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestMacroDescripcionLlevaLaHuella(t *testing.T) {
	casos := []struct {
		nombre, consecutivo, huella, quiere string
	}{
		// Nomenclatura del usuario (2026-08-17): «FC» + los 6 dígitos + la huella.
		{
			"consecutivo largo + huella",
			"00100001010000025786", "CXP-A1B2C3D4E5F6",
			"FC 025786 CXPA1B2C3D4E5F6",
		},
		{
			"consecutivo corto se rellena a 6",
			"1000", "CXP-0123456789AB",
			"FC 001000 CXP0123456789AB",
		},
		{
			// Sin programar no hay huella: el pago sigue siendo legible para el proveedor.
			"sin huella queda legible",
			"00100001010000025786", "",
			"FC 025786",
		},
		{"sin consecutivo ni huella", "", "", "FC"},
		{"solo huella", "", "CXP-A1B2C3D4E5F6", "FC CXPA1B2C3D4E5F6"},
	}
	for _, c := range casos {
		if got := macroDescripcion(c.consecutivo, c.huella); got != c.quiere {
			t.Errorf("%s: %q, quiere %q", c.nombre, got, c.quiere)
		}
	}
}

func TestMacroDescripcionSeReconoceASiMisma(t *testing.T) {
	// La prueba que importa: lo que el sistema emite tiene que poder volver a leerse. Si esto
	// se rompe, la conciliación automática deja de funcionar sin que nada más falle.
	desc := macroDescripcion("00100001010000025786", "CXP-A1B2C3D4E5F6")
	huella, ok := extraerHuella(desc)
	if !ok || huella != "CXP-A1B2C3D4E5F6" {
		t.Errorf("extraerHuella(%q) = (%q, %v)", desc, huella, ok)
	}
	// Y también dentro del texto que el banco suele agregar alrededor.
	conRuido := "DEBITO MASIVO SINPE (2026073110431000132373477 - " + desc + " [PROVEEDOR X])"
	if h, ok := extraerHuella(conRuido); !ok || h != "CXP-A1B2C3D4E5F6" {
		t.Errorf("con ruido del banco: (%q, %v)", h, ok)
	}
}

func servicioHuella(repo *fakeRepo) *Service {
	return NewService(repo, nil, zap.NewNop())
}

func TestConciliarHuella(t *testing.T) {
	ctx := context.Background()
	docProgramado := Documento{
		ID: "doc-1", Consecutivo: "00100001010000025786", Proveedor: "Gas Tomza",
		Estado: EstProgramado, ConceptoID: "con-1", ClasificacionID: "clas-1",
	}

	t.Run("sin huella no es un pago de CxP", func(t *testing.T) {
		res, err := servicioHuella(&fakeRepo{}).ConciliarHuella(ctx, "e1", "COMISION POR TRANSACCION", "1000", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaSinHuella {
			t.Errorf("veredicto = %s, quiere SIN_HUELLA", res.Veredicto)
		}
	})

	t.Run("huella sin documento pagable", func(t *testing.T) {
		repo := &fakeRepo{errPorHuella: ErrDocumentoNoEncontrado}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", "PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "1000", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaSinDocumento || res.Huella != "CXP-A1B2C3D4E5F6" {
			t.Errorf("veredicto = %s, huella = %q", res.Veredicto, res.Huella)
		}
	})

	t.Run("monto distinto NO concilia: es un hallazgo", func(t *testing.T) {
		repo := &fakeRepo{docPorHuella: docProgramado, netoAPagar: "118650.00", filasCambio: 1}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", "PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "100000.00", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaMontoDiferente {
			t.Fatalf("veredicto = %s, quiere MONTO_DIFERENTE", res.Veredicto)
		}
		if res.MontoEsperado != "118650.00" || res.MontoBanco != "100000.00" {
			t.Errorf("debería reportar las dos cifras: esperado %q, banco %q", res.MontoEsperado, res.MontoBanco)
		}
		if repo.capA != "" {
			t.Errorf("no debería haber cambiado el estado del documento (cambió a %q)", repo.capA)
		}
	})

	t.Run("monto exacto concilia y devuelve la clasificación del documento", func(t *testing.T) {
		repo := &fakeRepo{docPorHuella: docProgramado, netoAPagar: "118650.00", filasCambio: 1}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", "PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "118650.00", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaConciliado {
			t.Fatalf("veredicto = %s, quiere CONCILIADO", res.Veredicto)
		}
		if res.ConceptoID != "con-1" || res.ClasificacionID != "clas-1" {
			t.Errorf("debería devolver la clasificación del documento para el movimiento: %+v", res)
		}
		if res.Consecutivo != docProgramado.Consecutivo || res.Proveedor != "Gas Tomza" {
			t.Errorf("faltan datos para el reporte: %+v", res)
		}
	})

	t.Run("sin monto no verifica (emparejamiento manual)", func(t *testing.T) {
		repo := &fakeRepo{docPorHuella: docProgramado, netoAPagar: "118650.00", filasCambio: 1}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", "CXP-A1B2C3D4E5F6", "", MonedaCRC, "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaConciliado {
			t.Errorf("veredicto = %s, quiere CONCILIADO", res.Veredicto)
		}
	})
}

// Los pagos ya emitidos con el formato ANTERIOR («PAGO FACTURA 025786 CXP-…») tienen que seguir
// conciliando cuando el banco los devuelva en el estado de cuenta: cambiar la nomenclatura de la
// macro no puede dejar huérfano lo que ya salió al banco. La huella se busca por su propio patrón,
// no por el texto que la rodea, y este test lo fija para que nadie lo rompa "limpiando" el regex.
func TestConciliaDescripcionesDelFormatoViejo(t *testing.T) {
	for _, desc := range []string{
		"PAGO FACTURA 025786 CXP-A1B2C3D4E5F6",               // formato anterior
		"FC 025786 CXPA1B2C3D4E5F6",                          // formato actual
		"TRANSFERENCIA A PROVEEDOR CXP-A1B2C3D4E5F6 REF 998", // el banco agrega texto propio
	} {
		h, ok := extraerHuella(desc)
		if !ok || h != "CXP-A1B2C3D4E5F6" {
			t.Errorf("extraerHuella(%q) = (%q, %v), quiere CXP-A1B2C3D4E5F6", desc, h, ok)
		}
	}
}

// EL BANCO BORRA EL GUION. Esto no es una hipótesis: son las descripciones que devolvió el estado
// de cuenta de Valle de Paz después de las dos primeras corridas de pago reales (30-set-2026). El
// sistema emitió «CXP-54A4281AE49A» y el banco escribió «CXP54A4281AE49A».
//
// Mientras el patrón exigió el guion, el barrido examinó 0 movimientos y enlazó 0 facturas. Si
// alguien "limpia" el regex y vuelve a poner el guion obligatorio, esta prueba se pone roja antes
// de que la conciliación se muera otra vez en silencio.
func TestElBancoBorraElGuionDeLaHuella(t *testing.T) {
	reales := []struct{ nombre, descripcion, quiere string }{
		{
			"débito SINPE tal como vuelve del banco",
			"DEBITO MASIVO SINPE ( - 1060746795 Pagar Proveedores Nuevo FC 000487 CXP54A4281AE49A Lote 888607 Linea 21 [FC 000487 CXP54A4281AE49A])",
			"CXP-54A4281AE49A",
		},
		{
			"segunda línea de la misma corrida",
			"DEBITO MASIVO SINPE ( - 1060746795 Pagar Proveedores Nuevo FC 001856 CXPBF83125517BE Lote 888607 Linea 15 [FC 001856 CXPBF83125517BE])",
			"CXP-BF83125517BE",
		},
		{
			// La forma canónica tiene que seguir funcionando: hay pagos emitidos de las dos maneras.
			"con guion, como lo emite la macro",
			"FC 000487 CXP-54A4281AE49A",
			"CXP-54A4281AE49A",
		},
	}
	for _, c := range reales {
		t.Run(c.nombre, func(t *testing.T) {
			h, ok := extraerHuella(c.descripcion)
			if !ok {
				t.Fatalf("no reconoció la huella en una descripción REAL del banco:\n%s", c.descripcion)
			}
			// Siempre canónica: es la forma con la que se busca el documento en la base.
			if h != c.quiere {
				t.Errorf("huella = %q, quiere %q (canónica, con guion)", h, c.quiere)
			}
		})
	}
}

// El pre-filtro de Bancos trae de más (busca «CXP» a secas) y es el patrón el que decide. Estas
// descripciones son de movimientos REALES de la empresa que contienen «CXP» y NO son pagos
// nuestros: si alguna calzara, el barrido intentaría conciliar un pago de otro banco contra una
// factura, y eso sí mueve plata de lugar.
func TestNoConfundeUnCXPAjenoConLaHuella(t *testing.T) {
	ajenas := []string{
		"FIDE 1937 CXP 430581 JUNIO 26/BANCO IMPROSA S.A",
		"A1 FIDE 1937 CXP 432547 JUIO 2/BANCO IMPROSA S.A",
		"INTERNET TRANSF PROV // ASEMOPT _ CXP_GT_ 280826 ASEMOPT FU",
		"COMISION POR TRANSACCION",
	}
	for _, desc := range ajenas {
		if h, ok := extraerHuella(desc); ok {
			t.Errorf("reconoció %q como huella en una descripción ajena:\n%s", h, desc)
		}
	}
}

// ── La moneda se compara ANTES que el importe (16-oct-2026) ─────────────────────────────────

// Comparar importes sin mirar la moneda es comparar números, no plata.
//
// Hay cinco cuentas bancarias en dólares y el neto de la factura está en la moneda de la factura.
// Una factura de USD 1.500 y un débito de ₡1.500 son el mismo número y hechos incomparables: sin
// esta puerta, el barrido los daría por conciliados y la factura quedaría cerrada contra un pago
// que no la cubre ni de lejos.
func TestLaMonedaSeComparaAntesQueElImporte(t *testing.T) {
	ctx := context.Background()
	enDolares := Documento{
		ID: "doc-1", Consecutivo: "00100001010000025786", Proveedor: "VOLCANO 506 S.A",
		Estado: EstProgramado, Moneda: "USD", ConceptoID: "con-1", ClasificacionID: "clas-1",
	}

	t.Run("el mismo número en otra moneda NO concilia", func(t *testing.T) {
		repo := &fakeRepo{docPorHuella: enDolares, netoAPagar: "1500.00", filasCambio: 1}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1",
			"PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "1500.00", "CRC", "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaMonedaDiferente {
			t.Fatalf("veredicto = %s, quiere MONEDA_DIFERENTE: ₡1.500 no pagan USD 1.500", res.Veredicto)
		}
		// Lo más importante: NO se tocó el estado del documento.
		if repo.capA != "" {
			t.Errorf("cerró la factura contra un pago en otra moneda (cambió a %q)", repo.capA)
		}
		// Y el hallazgo tiene que decir contra qué no calza.
		if res.MonedaEsperada != "USD" {
			t.Errorf("moneda esperada = %q, quiere USD", res.MonedaEsperada)
		}
	})

	t.Run("la misma moneda sigue conciliando", func(t *testing.T) {
		repo := &fakeRepo{docPorHuella: enDolares, netoAPagar: "1500.00", filasCambio: 1}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1",
			"PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "1500.00", "usd", "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		// Minúsculas y espacios no son una moneda distinta.
		if res.Veredicto != HuellaConciliado {
			t.Errorf("veredicto = %s, quiere CONCILIADO", res.Veredicto)
		}
	})

	t.Run("la moneda se mira antes que el importe", func(t *testing.T) {
		// Monto distinto Y moneda distinta: el veredicto tiene que ser el de la moneda. Decir
		// «monto diferente» mandaría a revisar la cifra, cuando lo que no calza es la unidad.
		repo := &fakeRepo{docPorHuella: enDolares, netoAPagar: "1500.00", filasCambio: 1}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1",
			"PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "860000.00", "CRC", "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaMonedaDiferente {
			t.Errorf("veredicto = %s, quiere MONEDA_DIFERENTE", res.Veredicto)
		}
	})

	t.Run("sin moneda no se bloquea: es «no verificable», igual que sin monto", func(t *testing.T) {
		// El emparejamiento manual que ya existía pasa el monto vacío. Desde el barrido la moneda
		// nunca viene vacía: sale de `cuenta_bancaria.moneda`, NOT NULL con CHECK (CRC|USD).
		repo := &fakeRepo{docPorHuella: enDolares, netoAPagar: "1500.00", filasCambio: 1}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1",
			"PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "1500.00", "", "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaConciliado {
			t.Errorf("veredicto = %s: sin moneda se comporta como antes", res.Veredicto)
		}
	})
}

// Una remesa se declara SIEMPRE en colones (`remesa_pago.monto_crc`), cualquiera sea la moneda de
// las facturas que agrupa. Un débito en dólares no la paga — y acá el daño es mayor que en una
// factura suelta, porque cerrar la remesa cierra TODAS sus facturas de una vez.
func TestUnaRemesaEnColonesNoLaPagaUnDebitoEnDolares(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		remesaPorHuella: Remesa{
			ID: "rem-1", Proveedor: "VOLCANO 506 S.A", MontoCRC: "400000.00",
			LoteNumero: 43, Numero: 7, Facturas: 5,
		},
		cerradas: 5,
	}
	res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1",
		"DEBITO SINPE CXPR-7K2M9P4XQ1BT", "400000.00", "USD", "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Veredicto != HuellaMonedaDiferente {
		t.Fatalf("veredicto = %s, quiere MONEDA_DIFERENTE", res.Veredicto)
	}
	if res.Facturas != 0 {
		t.Errorf("cerró %d facturas con un pago en otra moneda", res.Facturas)
	}
}
