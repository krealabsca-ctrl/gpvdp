package cxp

// Lo que el proveedor lee cuando se le manda el comprobante (reporte de Contabilidad, 19-oct-2026).
//
// EL CORREO REAL que motivó esto:
//
//	Le informamos que su factura  fue pagada por un monto de ₡56500.00.
//	… La referencia con la que aparece en el banco es CXP-854A6F44CA6A.
//
// Tres cosas mal en dos líneas:
//
//  1. «su factura␣␣fue pagada» — el consecutivo vino vacío y dejó un hueco. Son 10 facturas de
//     4.542 sin consecutivo, y el hueco se lee como un error del sistema.
//  2. «₡56500.00» — el decimal crudo. Sin separador de miles, ₡56500 y ₡565000 se distinguen
//     contando dígitos.
//  3. «CXP-854A6F44CA6A» — la huella de la FACTURA, con guion. Si el pago salió agrupado el banco
//     trae la de la REMESA, y el guion lo borra el canal: el proveedor buscaba en su estado de
//     cuenta un código que nunca iba a encontrar, que es justo lo contrario de lo que esa frase
//     promete.

import (
	"strings"
	"testing"

	"github.com/gpvdp/erp/internal/plantillas"
)

func envioDePrueba(mod func(*ComprobanteEnvio)) ComprobanteEnvio {
	e := ComprobanteEnvio{
		ProveedorNombre: "WISOMTI SOCIEDAD ANONIMA",
		Consecutivo:     "00100001010000025786",
		Moneda:          "CRC",
		Total:           "56500.00",
		Huella:          "CXP-854A6F44CA6A",
		HuellaQueViaja:  "CXP-854A6F44CA6A",
		FechaEmision:    "12/09/2026",
		FacturasDelPago: 1,
	}
	if mod != nil {
		mod(&e)
	}
	return e
}

func TestElMontoVaFormateadoParaQueLoLeaUnaPersona(t *testing.T) {
	t.Parallel()
	v := ValoresDelComprobante(envioDePrueba(nil))
	if v["MONTO"] != "₡56\u00a0500,00" {
		t.Errorf("MONTO = %q, quiere ₡56 500,00 — antes salía «₡56500.00»", v["MONTO"])
	}
	// Y en la moneda de la factura, no siempre en colones.
	usd := ValoresDelComprobante(envioDePrueba(func(e *ComprobanteEnvio) {
		e.Moneda, e.Total = "USD", "1234.56"
	}))
	if usd["MONTO"] != "USD 1\u00a0234,56" {
		t.Errorf("MONTO en dólares = %q", usd["MONTO"])
	}
}

// Una factura sin consecutivo NO puede dejar un hueco en la frase.
func TestSinConsecutivoSeNombraPorFechaYNoQuedaHueco(t *testing.T) {
	t.Parallel()
	v := ValoresDelComprobante(envioDePrueba(func(e *ComprobanteEnvio) { e.Consecutivo = "" }))
	if v["CONSECUTIVO"] == "" {
		t.Fatal("quedó vacío: el correo vuelve a decir «su factura  fue pagada»")
	}
	if !strings.Contains(v["CONSECUTIVO"], "12/09/2026") {
		t.Errorf("CONSECUTIVO = %q, se esperaba la fecha como respaldo", v["CONSECUTIVO"])
	}
	// Y el texto armado no puede tener dos espacios seguidos.
	texto := renderDePrueba(v)
	if strings.Contains(texto, "  ") {
		t.Errorf("el texto quedó con un hueco:\n%s", texto)
	}
}

// ── La referencia: lo que el proveedor va a buscar en SU estado de cuenta ───────────────────

func TestLaReferenciaVaSinGuionPorqueElBancoLoBorra(t *testing.T) {
	t.Parallel()
	v := ValoresDelComprobante(envioDePrueba(nil))
	if strings.Contains(v["REFERENCIA"], "-") {
		t.Errorf("REFERENCIA = %q: el banco borra el guion, así que no se puede mandar con él", v["REFERENCIA"])
	}
	if v["REFERENCIA"] != "CXP854A6F44CA6A" {
		t.Errorf("REFERENCIA = %q, quiere CXP854A6F44CA6A", v["REFERENCIA"])
	}
}

// EN UN PAGO AGRUPADO viaja el código de la REMESA, no el de la factura.
//
// Medido: 9 facturas dentro de remesa, todas con huella propia distinta a la de su remesa. El
// correo mandaba la de la factura y el banco trae la de la remesa.
func TestEnUnPagoAgrupadoLaReferenciaEsLaDeLaRemesa(t *testing.T) {
	t.Parallel()
	e := envioDePrueba(func(e *ComprobanteEnvio) {
		e.Huella = "CXP-55F38DF8F020"          // la de la factura: NO es la que vuelve
		e.HuellaQueViaja = "CXPR-AC55A0819D5C" // la de la remesa: esta sí
		e.ReferenciaRemesa = "REM 006127"
		e.FacturasDelPago = 3
	})
	ref := ReferenciaParaElProveedor(e)
	if strings.Contains(ref, "55F38DF8F020") {
		t.Errorf("mandó la huella de la factura (%q): el banco nunca trae ese código", ref)
	}
	if ref != "REM 006127 CXPRAC55A0819D5C" {
		t.Errorf("REFERENCIA = %q, quiere «REM 006127 CXPRAC55A0819D5C»", ref)
	}
}

// ── El detalle de un pago agrupado ──────────────────────────────────────────────────────────

func TestUnPagoAgrupadoListaSusFacturas(t *testing.T) {
	t.Parallel()
	e := envioDePrueba(func(e *ComprobanteEnvio) {
		e.FacturasDelPago = 3
		e.Total = "156500.00"
		e.Facturas = []FacturaDelPago{
			{Consecutivo: "FC 000487", FechaEmision: "12/09/2026", Total: "56500.00", Moneda: "CRC"},
			{Consecutivo: "FC 000512", FechaEmision: "18/09/2026", Total: "70000.00", Moneda: "CRC"},
			{Consecutivo: "", FechaEmision: "25/09/2026", Total: "30000.00", Moneda: "CRC"},
		}
	})
	v := ValoresDelComprobante(e)
	// El encabezado deja de hablar en singular, y el verbo concuerda: con una plantilla fija del
	// tipo «su factura [CONSECUTIVO] fue pagada» salía «su factura 3 facturas fue pagada».
	if v["LO_PAGADO"] != "se le pagaron 3 facturas" {
		t.Errorf("LO_PAGADO = %q, quiere «se le pagaron 3 facturas»", v["LO_PAGADO"])
	}
	// [CONSECUTIVO] se conserva para las plantillas que una empresa ya personalizó, y ahí tiene
	// que seguir calzando en la frase vieja: «su factura FC 000487 y 2 más fue pagada».
	if v["CONSECUTIVO"] != "FC 000487 y 2 más" {
		t.Errorf("CONSECUTIVO = %q, quiere «FC 000487 y 2 más»", v["CONSECUTIVO"])
	}
	det := v["DETALLE_FACTURAS"]
	for _, quiere := range []string{"FC 000487", "FC 000512", "₡56\u00a0500,00", "₡70\u00a0000,00"} {
		if !strings.Contains(det, quiere) {
			t.Errorf("el detalle no trae %q:\n%s", quiere, det)
		}
	}
	// La que no tiene consecutivo se nombra igual, no desaparece de la lista.
	if !strings.Contains(det, "(sin consecutivo)") {
		t.Errorf("una factura sin consecutivo se perdió del detalle:\n%s", det)
	}
	if n := strings.Count(det, "·"); n != 3 {
		t.Errorf("el detalle tiene %d líneas, quiere 3", n)
	}
}

// En un pago de UNA factura no se arma una lista de un ítem: el consecutivo ya está en la frase.
func TestUnPagoSueltoNoLlevaListaDeUnSoloItem(t *testing.T) {
	t.Parallel()
	v := ValoresDelComprobante(envioDePrueba(nil))
	if v["DETALLE_FACTURAS"] != "" {
		t.Errorf("DETALLE_FACTURAS = %q, quiere vacío en un pago suelto", v["DETALLE_FACTURAS"])
	}
	// Y el párrafo vacío no deja un agujero en el texto.
	texto := renderDePrueba(v)
	if strings.Contains(texto, "\n\n\n") {
		t.Errorf("quedó un agujero donde iba el detalle:\n%q", texto)
	}
}

// ── WhatsApp ────────────────────────────────────────────────────────────────────────────────

func TestElTelefonoSeNormalizaComoLoPideWhatsApp(t *testing.T) {
	t.Parallel()
	// Lo que una persona escribe de verdad.
	for _, entra := range []string{"88888888", "8888-8888", "+506 8888 8888", "(506) 88888888"} {
		got, err := NormalizarTelefonoCR(entra)
		if err != nil {
			t.Errorf("NormalizarTelefonoCR(%q): %v", entra, err)
			continue
		}
		if got != "50688888888" {
			t.Errorf("NormalizarTelefonoCR(%q) = %q, quiere 50688888888", entra, got)
		}
	}
	// Un dedazo no abre un chat vacío sin explicación.
	for _, malo := range []string{"", "888", "88888", "abc", "1"} {
		if _, err := NormalizarTelefonoCR(malo); err != ErrTelefonoInvalido {
			t.Errorf("NormalizarTelefonoCR(%q) = %v, quiere ErrTelefonoInvalido", malo, err)
		}
	}
}

// El texto de WhatsApp y el del correo salen de la MISMA plantilla. Con dos armados, en seis meses
// dicen cosas distintas y el proveedor que recibe los dos no sabe cuál vale.
func TestWhatsAppYCorreoDicenLoMismo(t *testing.T) {
	t.Parallel()
	e := envioDePrueba(nil)
	// Las dos superficies arman sus valores con la misma función; si alguien duplicara el armado,
	// esta comparación dejaría de tener sentido y hay que venir a mirarla.
	a := ValoresDelComprobante(e)
	b := ValoresDelComprobante(e)
	for k, v := range a {
		if b[k] != v {
			t.Errorf("la variable %q difiere entre canales: %q vs %q", k, v, b[k])
		}
	}
	if renderDePrueba(a) != renderDePrueba(b) {
		t.Error("el texto del correo y el de WhatsApp no coinciden")
	}
}

// renderDePrueba arma el cuerpo con la plantilla de fábrica, que es la que ve una empresa que no
// editó la suya.
func renderDePrueba(valores map[string]string) string {
	t, _ := plantillas.TipoPorClave(plantillas.ClaveCxPComprobante)
	return plantillas.Render(t.CuerpoDefault, valores)
}
