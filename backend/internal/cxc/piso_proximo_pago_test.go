package cxc

// Nadie debe nada anterior a su fecha de próximo pago (13-oct-2026).
//
// EL CASO REAL, en palabras del Director Financiero: «me sale con un saldo grande; lo cierto es
// que hay muchos clientes que deben menos. En el reporte iba la fecha del próximo pago, lo que se
// resume que hay varios que no deben anterior a esa fecha».
//
// Medido contra el saldo que traía el propio archivo de cartera:
//
//	18.450 contratos con saldo de MÁS, por ₡1.042.683.725
//
// El contrato CO118040 debía UNA cuota de ₡500.000 y el sistema le calculó NUEVE (₡4.500.000).
// La causa: el rango de la corrida —enero a octubre— se le aplicaba completo a todos, sin mirar
// hasta cuándo había pagado cada uno.
//
// Esto se prueba sin base: `planDe` es una función pura sobre el contrato y el rango, y es ahí
// donde vive la decisión.

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func dia(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

// mensual arma un contrato mensual de cuota fija, que es la forma del 90 % de la cartera.
func contratoMensual(primerCobro, proximoPago string) ContratoGenerable {
	return ContratoGenerable{
		ID: "x", Numero: "CO118040",
		PrimerCobro: primerCobro, ProximoPago: proximoPago,
		DiaPago: 15, Cuota: decimal.RequireFromString("500000"),
		MesesCiclo: 1,
	}
}

// EL CASO EXACTO DE CO118040: contrato viejo, al día hasta octubre, una sola cuota pendiente.
func TestNoSeCobraNadaAnteriorAlProximoPago(t *testing.T) {
	c := contratoMensual("2021-04-26", "2026-10-15")

	cargos, motivo := planDe(c, dia("2026-01-01"), dia("2026-10-31"))
	if motivo != "" {
		t.Fatalf("motivo inesperado: %s", motivo)
	}
	if len(cargos) != 1 {
		t.Fatalf("cargos = %d, quería 1: está al día hasta octubre, así que debe UNA cuota, "+
			"no las diez del rango", len(cargos))
	}
}

// SIN FECHA DE PRÓXIMO PAGO, todo sigue como antes.
//
// Es la mitad que protege: inventarle un piso a un contrato que no lo trae escondería deuda real.
// El que no la tiene genera el rango completo, igual que siempre.
func TestSinProximoPagoMandaElRango(t *testing.T) {
	c := contratoMensual("2026-01-10", "")

	cargos, motivo := planDe(c, dia("2026-01-01"), dia("2026-10-31"))
	if motivo != "" {
		t.Fatalf("motivo inesperado: %s", motivo)
	}
	if len(cargos) < 9 {
		t.Errorf("cargos = %d: sin fecha de próximo pago tiene que generar el rango entero", len(cargos))
	}
}

// UN PRÓXIMO PAGO VIEJO NO BAJA EL PISO: el rango manda.
//
// Un cliente cuya próxima cuota venció en 2023 debe desde entonces, pero la corrida que se pidió
// arranca en enero de 2026. El piso es el MÁS TARDÍO de los dos, no el más viejo: si no, una
// corrida acotada a este año terminaría generando tres años de cargos sin que nadie los pidiera.
func TestUnProximoPagoViejoNoAmpliaElRango(t *testing.T) {
	c := contratoMensual("2021-04-26", "2023-08-04")

	cargos, motivo := planDe(c, dia("2026-01-01"), dia("2026-10-31"))
	if motivo != "" {
		t.Fatalf("motivo inesperado: %s", motivo)
	}
	if len(cargos) < 9 || len(cargos) > 10 {
		t.Errorf("cargos = %d, quería los del rango (9 o 10): el piso viejo no puede ampliarlo", len(cargos))
	}
}

// AL DÍA MÁS ALLÁ DEL RANGO: no debe nada, y eso no es un error.
func TestAlDiaMasAllaDelRangoNoGeneraNada(t *testing.T) {
	c := contratoMensual("2021-04-26", "2027-03-15")

	cargos, motivo := planDe(c, dia("2026-01-01"), dia("2026-10-31"))
	if motivo != "" {
		t.Errorf("motivo = %q: estar al día no es un dato incompleto, no lleva motivo", motivo)
	}
	if len(cargos) != 0 {
		t.Errorf("cargos = %d, quería 0: está al día más allá del rango", len(cargos))
	}
}
