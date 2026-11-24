package conta

// El detector de naturaleza cruzada y la invariante «o cuenta, o motivo», sin base de datos.
// Las que necesitan Postgres están en repository_mapeo_test.go.

import (
	"errors"
	"strings"
	"testing"
)

// EL CASO QUE ORIGINÓ EL DETECTOR, medido el 23-oct-2026 sobre el catálogo real de Valle de Paz:
// 23 clasificaciones «Caja Chica» cuelgan del concepto GASTOS, y en el plan contable la caja chica
// es ACTIVO. Reponer una caja chica no es gasto: es mover plata de Banco a Caja Chica.
func TestLaCajaChicaDelERPEsGastoYEnElPlanEsActivo(t *testing.T) {
	cruzada, aviso := NaturalezaCruzada(NatGasto, true, ClaseActivo)
	if !cruzada {
		t.Fatal("no avisó del cruce: 23 clasificaciones de caja chica se mapearían a activo sin que nadie lo mire")
	}
	// El aviso tiene que decir QUÉ revisar. «Naturaleza inconsistente» no le sirve a nadie.
	if !strings.Contains(aviso, "caja chica") {
		t.Errorf("el aviso no nombra el caso concreto: %q", aviso)
	}
}

func TestNaturalezaCruzada(t *testing.T) {
	casos := []struct {
		nombre     string
		naturaleza string
		declarada  bool
		clase      string
		quiero     bool
	}{
		{"gasto contra gasto no cruza", NatGasto, true, ClaseGasto, false},
		{"gasto contra costo no cruza", NatGasto, true, ClaseCosto, false},
		{"gasto contra activo cruza (caja chica)", NatGasto, true, ClaseActivo, true},
		{"gasto contra pasivo cruza (pagar una deuda)", NatGasto, true, ClasePasivo, true},
		{"gasto contra ingreso cruza", NatGasto, true, ClaseIngreso, true},
		{"ingreso contra ingreso no cruza", NatIngreso, true, ClaseIngreso, false},
		{"ingreso contra pasivo cruza (cobro adelantado)", NatIngreso, true, ClasePasivo, true},
		{"ingreso contra activo cruza (cobrar una cuenta)", NatIngreso, true, ClaseActivo, true},
		{"neutro contra gasto cruza", NatNeutro, true, ClaseGasto, true},
		{"neutro contra ingreso cruza", NatNeutro, true, ClaseIngreso, true},
		{"neutro contra costo cruza", NatNeutro, true, ClaseCosto, true},
		{"neutro contra activo no cruza (es un traslado)", NatNeutro, true, ClaseActivo, false},
		{"neutro contra pasivo no cruza", NatNeutro, true, ClasePasivo, false},

		// SIN NATURALEZA DECLARADA NO HAY OPINIÓN. Es un caso frecuente en el ERP y callar es lo
		// correcto: inventar un cruce sobre un dato que nadie declaró llenaría la pantalla de
		// avisos falsos y enseñaría a ignorarlos.
		{"sin declarar no opina", NatGasto, false, ClaseActivo, false},
		{"naturaleza vacía no opina", "", true, ClaseActivo, false},
		{"sin cuenta todavía no opina", NatGasto, true, "", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cruzada, aviso := NaturalezaCruzada(c.naturaleza, c.declarada, c.clase)
			if cruzada != c.quiero {
				t.Fatalf("cruzada=%v, se esperaba %v (aviso %q)", cruzada, c.quiero, aviso)
			}
			if cruzada && aviso == "" {
				t.Error("avisó del cruce pero no dijo por qué")
			}
			if !cruzada && aviso != "" {
				t.Errorf("no hay cruce pero devolvió aviso: %q", aviso)
			}
		})
	}
}

// LA INVARIANTE, en el borde: o cuenta, o motivo. Nunca las dos, nunca ninguna.
func TestDecisionDeMapeoValidar(t *testing.T) {
	casos := []struct {
		nombre string
		d      DecisionDeMapeo
		quiero error
	}{
		{"mapeada con cuenta", DecisionDeMapeo{Estado: EstadoMapeada, CuentaID: "una-cuenta"}, nil},
		{"mapeada sin cuenta", DecisionDeMapeo{Estado: EstadoMapeada}, ErrCuentaRequerida},
		{"mapeada con cuenta en blanco", DecisionDeMapeo{Estado: EstadoMapeada, CuentaID: "   "}, ErrCuentaRequerida},
		{"no contabilizable con motivo",
			DecisionDeMapeo{Estado: EstadoNoContabilizable, Motivo: "es un traslado entre cuentas propias"}, nil},
		{"no contabilizable sin motivo", DecisionDeMapeo{Estado: EstadoNoContabilizable}, ErrMotivoRequerido},
		// El «ok» reflejo no es una explicación. Mismo criterio que la liberación de facturas.
		{"no contabilizable con motivo de trámite",
			DecisionDeMapeo{Estado: EstadoNoContabilizable, Motivo: "ok"}, ErrMotivoRequerido},
		{"estado inventado", DecisionDeMapeo{Estado: "QUIZAS"}, ErrEstadoInvalido},
		{"sin estado", DecisionDeMapeo{}, ErrEstadoInvalido},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			d := c.d
			err := d.Validar()
			if !errors.Is(err, c.quiero) {
				t.Fatalf("err=%v, se esperaba %v", err, c.quiero)
			}
		})
	}

	// Declarar que algo NO se contabiliza y mandar una cuenta es contradictorio: gana la decisión
	// y la cuenta se descarta, para que la base no reciba una fila que su CHECK rechaza.
	t.Run("no contabilizable descarta la cuenta que venga", func(t *testing.T) {
		d := DecisionDeMapeo{Estado: EstadoNoContabilizable, Motivo: "partida de control interno",
			CuentaID: "6-01-00-000-000"}
		if err := d.Validar(); err != nil {
			t.Fatalf("validar: %v", err)
		}
		if d.CuentaID != "" {
			t.Errorf("la cuenta sobrevivió a una decisión de no contabilizar: %q", d.CuentaID)
		}
	})
}

// El porcentaje mide lo que BLOQUEA. Contar las clasificaciones que nadie usó haría que nunca
// llegue al 100 % y que el semáforo deje de servir.
func TestPorcentajeResueltoMideLoQueBloquea(t *testing.T) {
	c := Cobertura{ConMovimientos: 4, Mapeadas: 2, NoContabilizables: 1, SinDecidir: 1, Total: 50}
	if got := c.PorcentajeResuelto(); got != 75 {
		t.Errorf("porcentaje %d, se esperaba 75 (3 resueltas de 4 con movimiento)", got)
	}
	// Sin nada que mapear, está todo resuelto: lo contrario sería dividir entre cero.
	if got := (Cobertura{Total: 10}).PorcentajeResuelto(); got != 100 {
		t.Errorf("sin clasificaciones con movimiento el porcentaje dio %d", got)
	}
}
