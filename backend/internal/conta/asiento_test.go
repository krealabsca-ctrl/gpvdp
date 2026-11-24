package conta

// La validación del asiento y el autoadivinado por IBAN, sin base de datos.

import (
	"errors"
	"testing"
)

func TestValidarAsiento(t *testing.T) {
	linea := func(cuenta, debe, haber string) NuevaLinea {
		return NuevaLinea{CuentaID: cuenta, Debe: debe, Haber: haber}
	}
	casos := []struct {
		nombre string
		a      NuevoAsiento
		quiero error
	}{
		{"cuadrado", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra de papelería",
			Lineas: []NuevaLinea{linea("c1", "1000", ""), linea("c2", "", "1000")}}, nil},
		{"descuadrado", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra",
			Lineas: []NuevaLinea{linea("c1", "1000", ""), linea("c2", "", "900")}}, ErrAsientoDescuadrado},
		{"una sola línea", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra",
			Lineas: []NuevaLinea{linea("c1", "1000", "")}}, ErrAsientoSinLineas},
		{"sin líneas", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra"}, ErrAsientoSinLineas},
		{"sin glosa", NuevoAsiento{Fecha: "2026-10-27",
			Lineas: []NuevaLinea{linea("c1", "1000", ""), linea("c2", "", "1000")}}, ErrGlosaRequerida},
		{"sin fecha", NuevoAsiento{Glosa: "compra",
			Lineas: []NuevaLinea{linea("c1", "1000", ""), linea("c2", "", "1000")}}, ErrFechaAsientoRequerida},
		{"fecha de pantalla", NuevoAsiento{Fecha: "27/10/2026", Glosa: "compra",
			Lineas: []NuevaLinea{linea("c1", "1000", ""), linea("c2", "", "1000")}}, ErrFechaInvalida},
		{"línea con debe y haber", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra",
			Lineas: []NuevaLinea{linea("c1", "500", "500"), linea("c2", "", "0")}}, ErrLineaDebeYHaber},
		{"línea en cero", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra",
			Lineas: []NuevaLinea{linea("c1", "1000", ""), linea("c2", "", "1000"), linea("c3", "", "")}}, ErrLineaSinMonto},
		{"línea sin cuenta", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra",
			Lineas: []NuevaLinea{linea("", "1000", ""), linea("c2", "", "1000")}}, ErrCuentaRequerida},
		{"monto que no es número", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra",
			Lineas: []NuevaLinea{linea("c1", "mil", ""), linea("c2", "", "1000")}}, ErrMontoInvalido},
		// La dirección la da la columna, no el signo. Permitir «-1000 al debe» haría que el mismo
		// asiento se vea distinto según quién lo cargó.
		{"monto negativo", NuevoAsiento{Fecha: "2026-10-27", Glosa: "compra",
			Lineas: []NuevaLinea{linea("c1", "-1000", ""), linea("c2", "", "-1000")}}, ErrMontoNegativo},
		// Tres líneas contra una es un asiento normal: el pago de una factura con retención.
		{"varias contra una", NuevoAsiento{Fecha: "2026-10-27", Glosa: "pago con retención",
			Lineas: []NuevaLinea{linea("c1", "1000", ""), linea("c2", "", "980"), linea("c3", "", "20")}}, nil},
		// Los decimales tienen que cuadrar de verdad: 0.1 + 0.2 no da 0.3 en coma flotante, y por
		// eso el dinero va en decimal.
		{"decimales que en float no cuadrarían", NuevoAsiento{Fecha: "2026-10-27", Glosa: "centavos",
			Lineas: []NuevaLinea{linea("c1", "0.10", ""), linea("c2", "0.20", ""), linea("c3", "", "0.30")}}, nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			a := c.a
			_, err := a.Validar()
			if !errors.Is(err, c.quiero) {
				t.Fatalf("err=%v, se esperaba %v", err, c.quiero)
			}
		})
	}
}

// EL AUTOADIVINADO ES EL 79 % DEL TRABAJO. Medido el 26-oct-2026 en Valle de Paz: el plan trae el
// IBAN escrito dentro del nombre de la cuenta, y así 5 de 16 cuentas bancarias se enlazan solas —
// esas 5 cubren 15.873 de los 20.082 movimientos.
func TestSugerirPorIBAN(t *testing.T) {
	plan := []Cuenta{
		{ID: "bn-crc", Codigo: "1-01-02-001-001", Nombre: "Banco Nacional Colones CR15015108710010019890", AceptaMovimiento: true},
		{ID: "bn-usd", Codigo: "1-01-02-001-002", Nombre: "Banco Nacional Dólares CR41015108710026014819", AceptaMovimiento: true},
		{ID: "bac", Codigo: "1-01-02-002-001", Nombre: "BAC San José Colones CR10010200009510302389", AceptaMovimiento: true},
		// Una de agrupación con el mismo texto: no puede ganar nunca, no admite asientos.
		{ID: "grupo", Codigo: "1-01-02-001-000", Nombre: "Banco Nacional CR15015108710010019890", AceptaMovimiento: false},
	}

	t.Run("calza el IBAN exacto", func(t *testing.T) {
		c, ok := SugerirPorIBAN("CR15015108710010019890", plan)
		if !ok || c.ID != "bn-crc" {
			t.Fatalf("sugirió %q (ok=%v)", c.ID, ok)
		}
	})
	t.Run("calza aunque venga con espacios o guiones", func(t *testing.T) {
		// El mismo IBAN se escribe de varias formas según de dónde salga.
		c, ok := SugerirPorIBAN("CR10 0102 0000 9510 3023 89", plan)
		if !ok || c.ID != "bac" {
			t.Fatalf("sugirió %q (ok=%v)", c.ID, ok)
		}
	})
	t.Run("nunca sugiere una cuenta de agrupación", func(t *testing.T) {
		// La de agrupación lleva el mismo IBAN. Si contara, habría dos candidatas y además una de
		// ellas no admite asientos: la base la rechazaría después.
		c, ok := SugerirPorIBAN("CR15015108710010019890", plan)
		if !ok || c.ID == "grupo" {
			t.Fatalf("sugirió la de agrupación: %q", c.ID)
		}
	})
	t.Run("sin IBAN no sugiere", func(t *testing.T) {
		// 8 de las 16 cuentas reales no tienen IBAN cargado (BN, Promerica, Scotiabank).
		if _, ok := SugerirPorIBAN("", plan); ok {
			t.Error("sugirió algo para una cuenta sin IBAN")
		}
	})
	t.Run("un fragmento corto no sugiere", func(t *testing.T) {
		// «CR15» calzaría como subcadena con media docena de cuentas. Sugerir ahí sería mandar
		// los asientos de un banco a otro.
		if c, ok := SugerirPorIBAN("CR15", plan); ok {
			t.Errorf("sugirió %q a partir de un fragmento", c.Codigo)
		}
	})
	t.Run("si hay dos candidatas no sugiere ninguna", func(t *testing.T) {
		// Dos cuentas de movimiento con el mismo IBAN es un catálogo con un problema; elegir una
		// al azar lo escondería.
		ambiguo := append([]Cuenta(nil), plan...)
		ambiguo = append(ambiguo, Cuenta{ID: "copia", Codigo: "1-01-02-009-001",
			Nombre: "Banco Nacional Colones (copia) CR15015108710010019890", AceptaMovimiento: true})
		if c, ok := SugerirPorIBAN("CR15015108710010019890", ambiguo); ok {
			t.Errorf("eligió %q entre dos candidatas iguales", c.Codigo)
		}
	})
}

// El signo del saldo sigue a la naturaleza de la cuenta, no a la resta.
func TestSaldoSegunNaturaleza(t *testing.T) {
	// Un pasivo que recibió 1000 al haber tiene saldo acreedor de 1000, no de −1000.
	if got := saldoSegunNaturaleza(dec("0"), dec("1000"), Acreedora); got.String() != "1000" {
		t.Errorf("cuenta acreedora: %s, se esperaba 1000", got)
	}
	if got := saldoSegunNaturaleza(dec("1000"), dec("0"), Deudora); got.String() != "1000" {
		t.Errorf("cuenta deudora: %s, se esperaba 1000", got)
	}
	// Y al revés: un activo que solo recibió créditos queda en negativo, que es una señal real.
	if got := saldoSegunNaturaleza(dec("0"), dec("600"), Deudora); got.String() != "-600" {
		t.Errorf("activo con solo créditos: %s, se esperaba -600", got)
	}
}
