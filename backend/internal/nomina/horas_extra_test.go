package nomina

// Tests de las horas extra (CT art. 139). Es dinero de la gente y base de la CCSS: lo que se
// prueba es la aritmética exacta y que el factor no se pueda bajar del mínimo legal.

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestValorHoraOrdinaria(t *testing.T) {
	// ₡650 000 / 240 h = ₡2 708,333… por hora.
	got, err := ValorHoraOrdinaria(dec("650000"), dec("240"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Round(4).String() != "2708.3333" {
		t.Errorf("valor hora = %s, quiere 2708.3333", got.Round(4))
	}
	if _, err := ValorHoraOrdinaria(dec("650000"), decimal.Zero); !errors.Is(err, ErrJornadaInvalida) {
		t.Errorf("divisor cero: err = %v", err)
	}
}

func TestMontoHorasExtra(t *testing.T) {
	casos := []struct {
		nombre                       string
		salario, horas, jornada, fac string
		quiere                       string
	}{
		{
			// 650 000/240 = 2 708,3333 × 1,5 = 4 062,50 × 6 h = 24 375,00
			"seis horas al mínimo legal", "650000", "6", "240", "1.5", "24375.00",
		},
		{
			"media hora", "650000", "0.5", "240", "1.5", "2031.25",
		},
		{
			// Un factor mayor sí se respeta (acuerdos que pagan doble).
			"factor 2 (acuerdo más favorable)", "650000", "6", "240", "2", "32500.00",
		},
		{
			// 480 000/240 = 2 000 × 1,5 = 3 000 × 10 = 30 000
			"cifras redondas", "480000", "10", "240", "1.5", "30000.00",
		},
	}
	for _, c := range casos {
		got, err := MontoHorasExtra(dec(c.salario), dec(c.horas), dec(c.jornada), dec(c.fac))
		if err != nil {
			t.Fatalf("%s: err = %v", c.nombre, err)
		}
		if got.StringFixed(2) != c.quiere {
			t.Errorf("%s: %s, quiere %s", c.nombre, got.StringFixed(2), c.quiere)
		}
	}
}

func TestMontoHorasExtraNoBajaDelMinimoLegal(t *testing.T) {
	// Aunque los parámetros dijeran 1,0 (o 0), se paga tiempo y medio: art. 139 es piso.
	alMinimo, _ := MontoHorasExtra(dec("650000"), dec("6"), dec("240"), dec("1.5"))
	for _, factorIlegal := range []string{"1", "0.5", "0", "-2"} {
		got, err := MontoHorasExtra(dec("650000"), dec("6"), dec("240"), dec(factorIlegal))
		if err != nil {
			t.Fatalf("factor %s: err = %v", factorIlegal, err)
		}
		if !got.Equal(alMinimo) {
			t.Errorf("factor %s pagó %s; debería elevarse al mínimo legal %s",
				factorIlegal, got.StringFixed(2), alMinimo.StringFixed(2))
		}
	}
}

func TestMontoHorasExtraRechazaHorasInvalidas(t *testing.T) {
	for _, horas := range []string{"0", "-3"} {
		if _, err := MontoHorasExtra(dec("650000"), dec(horas), dec("240"), dec("1.5")); !errors.Is(err, ErrHorasInvalidas) {
			t.Errorf("horas %s: err = %v, quiere ErrHorasInvalidas", horas, err)
		}
	}
}

func TestParametrosHorasExtraConDefaults(t *testing.T) {
	t.Run("sin parámetros guardados usa los legales de referencia", func(t *testing.T) {
		horas, factor := horasJornadaOFactorDefault(Parametros{})
		if !horas.Equal(HorasJornadaMesDefault) || !factor.Equal(FactorHoraExtraLegal) {
			t.Errorf("horas = %s, factor = %s", horas, factor)
		}
	})

	t.Run("respeta lo guardado por la empresa", func(t *testing.T) {
		horas, factor := horasJornadaOFactorDefault(Parametros{HorasJornadaMes: "208", FactorHoraExtra: "2"})
		if horas.String() != "208" || factor.String() != "2" {
			t.Errorf("horas = %s, factor = %s", horas, factor)
		}
	})

	t.Run("ignora un factor por debajo de la ley y un divisor inservible", func(t *testing.T) {
		horas, factor := horasJornadaOFactorDefault(Parametros{HorasJornadaMes: "0", FactorHoraExtra: "1.2"})
		if !horas.Equal(HorasJornadaMesDefault) {
			t.Errorf("divisor 0 debería caer al default, dio %s", horas)
		}
		if !factor.Equal(FactorHoraExtraLegal) {
			t.Errorf("factor 1.2 debería elevarse a 1.5, dio %s", factor)
		}
	})
}

func TestResolverHorasExtraEnLaCorrida(t *testing.T) {
	empleados := []EmpleadoCorrida{
		{Empleado: Empleado{ID: "e-1", SalarioBase: "650000.00"}, FraccionMes: "1"},
		{Empleado: Empleado{ID: "e-2", SalarioBase: "480000.00"}, FraccionMes: "1"},
	}
	novedades := map[string][]IngresoCalc{
		"e-1": {
			{Nombre: "Horas extra", Horas: dec("6"), AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true},
			{Nombre: "Comisiones", Monto: dec("50000"), AfectaCCSS: true},
		},
		"e-2": {{Nombre: "Horas extra", Horas: dec("10"), AfectaCCSS: true}},
		// Novedad de alguien que NO entró a esta corrida (salió del mes).
		"e-9": {{Nombre: "Horas extra", Horas: dec("4")}},
	}
	p := Parametros{HorasJornadaMes: "240", FactorHoraExtra: "1.5"}
	if err := resolverHorasExtra(novedades, empleados, p); err != nil {
		t.Fatalf("resolver: %v", err)
	}

	t.Run("deriva el monto del salario de cada uno", func(t *testing.T) {
		if got := novedades["e-1"][0].Monto.StringFixed(2); got != "24375.00" {
			t.Errorf("e-1 = %s, quiere 24375.00", got)
		}
		if got := novedades["e-2"][0].Monto.StringFixed(2); got != "30000.00" {
			t.Errorf("e-2 = %s, quiere 30000.00", got)
		}
	})

	t.Run("no toca las novedades de monto directo", func(t *testing.T) {
		if got := novedades["e-1"][1].Monto.StringFixed(2); got != "50000.00" {
			t.Errorf("la comisión cambió: %s", got)
		}
		if novedades["e-1"][1].Nombre != "Comisiones" {
			t.Errorf("el nombre de la comisión cambió: %q", novedades["e-1"][1].Nombre)
		}
	})

	t.Run("el desglose queda en la colilla", func(t *testing.T) {
		// 650000/240 × 1,5 = 4 062,50 la hora extra.
		if got := novedades["e-1"][0].Nombre; got != "Horas extra — 6 h × ₡4062.50" {
			t.Errorf("nombre = %q", got)
		}
	})

	t.Run("las horas extra siguen siendo salario", func(t *testing.T) {
		// El guardarraíl: no se puede usar esto para sacar remuneración de la base CCSS.
		if !novedades["e-1"][0].AfectaCCSS || !novedades["e-1"][0].AfectaRenta || !novedades["e-1"][0].AfectaAguinaldo {
			t.Error("las banderas de afectación NO deben cambiar al derivar el monto")
		}
	})

	t.Run("quien no está en la corrida no se paga", func(t *testing.T) {
		if !novedades["e-9"][0].Monto.IsZero() {
			t.Errorf("e-9 no entró a la corrida y quedó con monto %s", novedades["e-9"][0].Monto)
		}
	})
}
