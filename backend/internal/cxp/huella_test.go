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
			"FC 025786 CXP-A1B2C3D4E5F6",
		},
		{
			"consecutivo corto se rellena a 6",
			"1000", "CXP-0123456789AB",
			"FC 001000 CXP-0123456789AB",
		},
		{
			// Sin programar no hay huella: el pago sigue siendo legible para el proveedor.
			"sin huella queda legible",
			"00100001010000025786", "",
			"FC 025786",
		},
		{"sin consecutivo ni huella", "", "", "FC"},
		{"solo huella", "", "CXP-A1B2C3D4E5F6", "FC CXP-A1B2C3D4E5F6"},
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
		res, err := servicioHuella(&fakeRepo{}).ConciliarHuella(ctx, "e1", "COMISION POR TRANSACCION", "1000", "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaSinHuella {
			t.Errorf("veredicto = %s, quiere SIN_HUELLA", res.Veredicto)
		}
	})

	t.Run("huella sin documento pagable", func(t *testing.T) {
		repo := &fakeRepo{errPorHuella: ErrDocumentoNoEncontrado}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", "PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "1000", "u1")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if res.Veredicto != HuellaSinDocumento || res.Huella != "CXP-A1B2C3D4E5F6" {
			t.Errorf("veredicto = %s, huella = %q", res.Veredicto, res.Huella)
		}
	})

	t.Run("monto distinto NO concilia: es un hallazgo", func(t *testing.T) {
		repo := &fakeRepo{docPorHuella: docProgramado, netoAPagar: "118650.00", filasCambio: 1}
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", "PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "100000.00", "u1")
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
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", "PAGO FACTURA 025786 CXP-A1B2C3D4E5F6", "118650.00", "u1")
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
		res, err := servicioHuella(repo).ConciliarHuella(ctx, "e1", "CXP-A1B2C3D4E5F6", "", "u1")
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
		"FC 025786 CXP-A1B2C3D4E5F6",                         // formato actual
		"TRANSFERENCIA A PROVEEDOR CXP-A1B2C3D4E5F6 REF 998", // el banco agrega texto propio
	} {
		h, ok := extraerHuella(desc)
		if !ok || h != "CXP-A1B2C3D4E5F6" {
			t.Errorf("extraerHuella(%q) = (%q, %v), quiere CXP-A1B2C3D4E5F6", desc, h, ok)
		}
	}
}
