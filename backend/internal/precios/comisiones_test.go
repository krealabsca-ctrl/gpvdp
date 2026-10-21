package precios

// Pruebas de las políticas de comisión.
//
// La prueba central es la de la carga social: en nómina el concepto «Comisiones» afecta CCSS,
// renta y aguinaldo en las tres empresas, así que la comisión NO cuesta lo que recibe el
// vendedor. El costeo actual la cuenta sin cargas, y son ₡36.221 por venta que faltan en todos
// los planes directos.

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func fecha(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func pol(id string, plan, vendedor *string, tipo, base, valor, desde string) PoliticaComision {
	return PoliticaComision{
		ID: id, PlanID: plan, VendedorID: vendedor, Tipo: tipo, Base: base,
		Valor: valor, VigenteDesde: desde, Activo: true,
		Especificidad: especificidadDe(plan, vendedor, ""),
	}
}

// El caso real del plan Oro: ₡135.000 de comisión cuestan ₡171.221 con las cargas patronales.
func TestLaComisionCuestaMasDeLoQueRecibeElVendedor(t *testing.T) {
	t.Parallel()
	p := pol("1", nil, nil, ComisionMontoFijo, BasePrecio, "135000", "2026-01-01")
	factor := decimal.RequireFromString("26.83") // CR 2026: cargas patronales + INS

	c, err := CalcularComision(p, decimal.RequireFromString("1350000"), &factor)
	if err != nil {
		t.Fatalf("CalcularComision: %v", err)
	}
	if c.Bruto != "135000.00" {
		t.Errorf("bruto = %q, se esperaba 135000.00", c.Bruto)
	}
	if c.CargaPatronal == nil || *c.CargaPatronal != "36220.50" {
		t.Errorf("carga patronal = %v, se esperaba 36220.50", c.CargaPatronal)
	}
	if c.CostoTotal == nil || *c.CostoTotal != "171220.50" {
		t.Errorf("costo total = %v, se esperaba 171220.50 — el costeo actual usa 135000", c.CostoTotal)
	}
}

// Sin el factor de nómina NO se hace pasar el bruto por costo total: se muestra lo que hay y se
// dice qué falta. Fingir que la comisión no tiene cargas la abarata un 27 %.
func TestSinElFactorDeNominaElCostoQuedaIncompleto(t *testing.T) {
	t.Parallel()
	p := pol("1", nil, nil, ComisionMontoFijo, BasePrecio, "135000", "2026-01-01")

	c, err := CalcularComision(p, decimal.RequireFromString("1350000"), nil)
	if err != nil {
		t.Fatalf("CalcularComision: %v", err)
	}
	if c.Bruto != "135000.00" {
		t.Errorf("el bruto sí se puede mostrar: %q", c.Bruto)
	}
	if c.CostoTotal != nil {
		t.Errorf("costo total = %v: sin factor NO se puede publicar un total", c.CostoTotal)
	}
	if c.Motivo == "" {
		t.Error("tiene que decir POR QUÉ el costo está incompleto")
	}
}

func TestElPorcentajeSeAplicaSobreLaBase(t *testing.T) {
	t.Parallel()
	p := pol("1", nil, nil, ComisionPorcentaje, BasePrecio, "10", "2026-01-01")
	factor := decimal.RequireFromString("26.83")

	c, _ := CalcularComision(p, decimal.RequireFromString("1350000"), &factor)
	if c.Bruto != "135000.00" {
		t.Errorf("10 %% de 1.350.000 = %q, se esperaba 135000.00", c.Bruto)
	}
}

// Comisionar un porcentaje de un margen negativo daría una comisión negativa, que no significa
// nada: el piso es cero.
func TestUnaBaseNegativaNoDaComisionNegativa(t *testing.T) {
	t.Parallel()
	p := pol("1", nil, nil, ComisionPorcentaje, BaseMargen, "10", "2026-01-01")
	factor := decimal.RequireFromString("26.83")

	c, _ := CalcularComision(p, decimal.RequireFromString("-50000"), &factor)
	if c.Bruto != "0.00" {
		t.Errorf("bruto = %q, se esperaba 0.00 sobre una base negativa", c.Bruto)
	}
}

// ── Resolución ──────────────────────────────────────────────────────────────────────────────

func TestGanaLaPoliticaMasEspecifica(t *testing.T) {
	t.Parallel()
	oro, juan := "plan-oro", "vend-juan"
	politicas := []PoliticaComision{
		pol("general", nil, nil, ComisionPorcentaje, BasePrecio, "5", "2026-01-01"),
		pol("del-plan", &oro, nil, ComisionPorcentaje, BasePrecio, "8", "2026-01-01"),
		pol("del-vendedor", nil, &juan, ComisionPorcentaje, BasePrecio, "9", "2026-01-01"),
		pol("de-ambos", &oro, &juan, ComisionPorcentaje, BasePrecio, "12", "2026-01-01"),
	}

	casos := []struct{ plan, vendedor, quiere string }{
		{oro, juan, "de-ambos"},
		{oro, "otro", "del-plan"},
		{"otro-plan", juan, "del-vendedor"},
		{"otro-plan", "otro", "general"},
	}
	for _, c := range casos {
		got := Resolver(politicas, c.plan, c.vendedor, fecha("2026-06-01"))
		if got == nil {
			t.Errorf("plan %q vendedor %q: no resolvió ninguna", c.plan, c.vendedor)
			continue
		}
		if got.ID != c.quiere {
			t.Errorf("plan %q vendedor %q → %q, se esperaba %q", c.plan, c.vendedor, got.ID, c.quiere)
		}
	}
}

// Una política que cambia en marzo no puede reescribir lo costeado en febrero: la vigencia es lo
// que hace que el costeo de un período pasado siga mostrando lo que costó entonces.
func TestLaVigenciaDecideQuePoliticaAplica(t *testing.T) {
	t.Parallel()
	oro := "plan-oro"
	politicas := []PoliticaComision{
		pol("vieja", &oro, nil, ComisionPorcentaje, BasePrecio, "5", "2026-01-01"),
		pol("nueva", &oro, nil, ComisionPorcentaje, BasePrecio, "8", "2026-03-01"),
	}

	if got := Resolver(politicas, oro, "", fecha("2026-02-15")); got == nil || got.ID != "vieja" {
		t.Errorf("en febrero debe regir la vieja, resolvió %v", got)
	}
	if got := Resolver(politicas, oro, "", fecha("2026-04-15")); got == nil || got.ID != "nueva" {
		t.Errorf("en abril debe regir la nueva, resolvió %v", got)
	}
}

func TestUnaPoliticaVencidaYaNoAplica(t *testing.T) {
	t.Parallel()
	oro := "plan-oro"
	hasta := "2026-06-30"
	p := pol("temporal", &oro, nil, ComisionPorcentaje, BasePrecio, "15", "2026-01-01")
	p.VigenteHasta = &hasta

	if Resolver([]PoliticaComision{p}, oro, "", fecha("2026-06-15")) == nil {
		t.Error("dentro de la vigencia debe aplicar")
	}
	if got := Resolver([]PoliticaComision{p}, oro, "", fecha("2026-07-15")); got != nil {
		t.Errorf("después del %s no debe aplicar, resolvió %q", hasta, got.ID)
	}
}

// Sin política NO se devuelve cero: eso diría que la venta no paga comisión y el margen saldría
// más alto que el real.
func TestSinPoliticaNoHayComisionCero(t *testing.T) {
	t.Parallel()
	if got := Resolver(nil, "plan", "vend", fecha("2026-06-01")); got != nil {
		t.Errorf("sin políticas no debe resolver nada, devolvió %+v", got)
	}
	inactiva := pol("x", nil, nil, ComisionPorcentaje, BasePrecio, "5", "2026-01-01")
	inactiva.Activo = false
	if got := Resolver([]PoliticaComision{inactiva}, "plan", "vend", fecha("2026-06-01")); got != nil {
		t.Error("una política inactiva no debe aplicar")
	}
}

func TestLaValidacionDeLaPoliticaRechazaLoInvalido(t *testing.T) {
	t.Parallel()
	base := func() PoliticaInput {
		return PoliticaInput{Tipo: ComisionPorcentaje, Base: BasePrecio, Valor: "10", VigenteDesde: "2026-01-01"}
	}
	t.Run("acepta lo válido y pone los defaults", func(t *testing.T) {
		in := PoliticaInput{Valor: "10", VigenteDesde: "2026-01-01"}
		if err := in.Normalizar(); err != nil {
			t.Fatalf("Normalizar: %v", err)
		}
		if in.Tipo != ComisionPorcentaje || in.Base != BasePrecio {
			t.Errorf("defaults = %q / %q", in.Tipo, in.Base)
		}
	})
	t.Run("un porcentaje mayor a 100 pagaría más de lo que entra", func(t *testing.T) {
		in := base()
		in.Valor = "150"
		if err := in.Normalizar(); err != ErrPorcentajeMayorACien {
			t.Errorf("error = %v, se esperaba ErrPorcentajeMayorACien", err)
		}
		// Pero un MONTO FIJO alto es legítimo: puede ser una comisión de ₡275.000.
		fijo := base()
		fijo.Tipo, fijo.Valor = ComisionMontoFijo, "275000"
		if err := fijo.Normalizar(); err != nil {
			t.Errorf("un monto fijo alto es válido: %v", err)
		}
	})
	t.Run("rechaza el resto", func(t *testing.T) {
		neg := base()
		neg.Valor = "-5"
		if err := neg.Normalizar(); err != ErrValorComisionInvalido {
			t.Errorf("valor negativo: %v", err)
		}
		tipo := base()
		tipo.Tipo = "REGALO"
		if err := tipo.Normalizar(); err != ErrTipoComisionInvalido {
			t.Errorf("tipo inválido: %v", err)
		}
		b := base()
		b.Base = "LO QUE SEA"
		if err := b.Normalizar(); err != ErrBaseComisionInvalida {
			t.Errorf("base inválida: %v", err)
		}
		inv := base()
		h := "2025-12-31"
		inv.VigenteHasta = &h
		if err := inv.Normalizar(); err != ErrVigenciaInvertida {
			t.Errorf("vigencia invertida: %v", err)
		}
	})
}
