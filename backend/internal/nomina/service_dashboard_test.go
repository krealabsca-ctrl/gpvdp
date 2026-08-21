package nomina

// Tests del dashboard de RRHH: el costo real del mes, sin contar dos veces el adelanto, y
// las alertas que salen de hechos de la base.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Jornada MENSUAL: el adelanto del día 15 es un pago A CUENTA del mismo salario que la
// liquidación devenga íntegro. El bruto del mes debe ser el salario del mes (900 000), no
// 1.5 veces; el neto desembolsado sí es la suma de los dos pagos.
func TestDashboardNoDuplicaElAdelanto(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "María Ramírez", Identificacion: "1-111",
		IBAN: "CR0101", DepartamentoNombre: "Administración", SalarioBase: "900000.00", Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	adel, err := svc.CrearCorrida(ctx, "e1", 2026, 8, CorridaAdelanto, "2026-08-15", "u1")
	if err != nil {
		t.Fatalf("crear adelanto: %v", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", adel.ID, "u1"); err != nil {
		t.Fatalf("aprobar adelanto: %v", err)
	}
	liq, err := svc.CrearCorrida(ctx, "e1", 2026, 8, CorridaLiquidacion, "2026-08-30", "u1")
	if err != nil {
		t.Fatalf("crear liquidación: %v", err)
	}

	d, err := svc.Dashboard(ctx, "e1", 2026, 8)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if d.Bruto != "900000.00" {
		t.Errorf("bruto del mes = %s, quiere 900000.00 (el adelanto no se suma dos veces)", d.Bruto)
	}
	if d.BaseCCSS != "900000.00" {
		t.Errorf("base contributiva = %s, quiere 900000.00", d.BaseCCSS)
	}
	// Costo real = bruto + patronal + provisiones, y el ratio por ₡100 de bruto.
	esperado := dec(d.Bruto).Add(dec(d.Patronal)).Add(dec(d.Provisiones))
	exigir(t, "costo real", dec(d.CostoReal), esperado.String())
	if dec(d.CostoPor100).LessThanOrEqual(dec("100")) {
		t.Errorf("costo por ₡100 = %s, debe superar 100 (cargas + provisiones)", d.CostoPor100)
	}
	// Neto desembolsado del mes = adelanto (450 000) + neto de la liquidación.
	netoLiq := liq.Lineas[0].Neto
	exigir(t, "neto del mes", dec(d.Neto), dec("450000").Add(dec(netoLiq)).String())
	exigir(t, "neto de la liquidación", dec(d.NetoLiquidacion), dec(netoLiq).String())
	if d.EmpleadosPagados != 1 || d.Empleados != 1 {
		t.Errorf("empleados = %d activos / %d en la corrida, quiere 1 y 1", d.Empleados, d.EmpleadosPagados)
	}

	// Ciclo: adelanto aprobado, liquidación en borrador, planilla pendiente (aún no congela).
	if d.Ciclo.Adelanto.Estado != EstAprobada || d.Ciclo.Adelanto.CorridaID != adel.ID {
		t.Errorf("paso adelanto = %+v, quiere APROBADA con su id", d.Ciclo.Adelanto)
	}
	if d.Ciclo.Liquidacion.Estado != EstBorrador {
		t.Errorf("paso liquidación = %s, quiere BORRADOR", d.Ciclo.Liquidacion.Estado)
	}
	if d.Ciclo.Planilla.Estado != PasoPendiente {
		t.Errorf("paso planilla = %s, quiere PENDIENTE", d.Ciclo.Planilla.Estado)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", liq.ID, "u1"); err != nil {
		t.Fatalf("aprobar liquidación: %v", err)
	}
	d2, err := svc.Dashboard(ctx, "e1", 2026, 8)
	if err != nil {
		t.Fatalf("dashboard tras aprobar: %v", err)
	}
	if d2.Ciclo.Planilla.Estado != PasoLista || d2.Ciclo.Planilla.CorridaID != liq.ID {
		t.Errorf("planilla = %+v, quiere LISTA con la liquidación", d2.Ciclo.Planilla)
	}

	// Tendencia: 6 meses hasta agosto (marzo..agosto), con el mes en curso marcado.
	if len(d2.Tendencia) != mesesTendencia {
		t.Fatalf("tendencia = %d puntos, quiere %d", len(d2.Tendencia), mesesTendencia)
	}
	primero, ultimo := d2.Tendencia[0], d2.Tendencia[len(d2.Tendencia)-1]
	if primero.Mes != 3 || primero.Costo != "0.00" || primero.EnCurso {
		t.Errorf("primer punto = %+v, quiere marzo en cero y no en curso", primero)
	}
	if ultimo.Mes != 8 || !ultimo.EnCurso || ultimo.Costo != d2.CostoReal {
		t.Errorf("último punto = %+v, quiere agosto en curso con el costo del mes (%s)", ultimo, d2.CostoReal)
	}

	// Headcount por departamento.
	if len(d2.Headcount) != 1 || d2.Headcount[0].Departamento != "Administración" {
		t.Errorf("headcount = %+v, quiere 1 en Administración", d2.Headcount)
	}
	// Sin conceptos no afectos: queda el aviso legal del guardarraíl.
	if len(d2.Alertas) != 1 || d2.Alertas[0].Tono != "LEGAL" {
		t.Errorf("alertas = %+v, quiere solo el aviso legal", d2.Alertas)
	}
}

// Las alertas reportan hechos: empleado sin IBAN (queda fuera del SINPE) y un concepto
// excluido de CCSS sin base legal (control del guardarraíl).
func TestDashboardAlertas(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Randall Sánchez", Identificacion: "1-111",
		SalarioBase: "500000.00", Activo: true} // sin IBAN
	repo.conceptos["viat"] = ConceptoNomina{ID: "viat", Nombre: "Viáticos", Tipo: ConceptoIngreso,
		AfectaCCSS: false, Activo: true} // sin base legal
	svc := NewService(repo, nil, nil)

	d, err := svc.Dashboard(context.Background(), "e1", 2026, 8)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	var sinIBAN, legal bool
	for _, a := range d.Alertas {
		if a.Tono == "WARN" && strings.Contains(a.Texto, "Randall Sánchez") && strings.Contains(a.Texto, "SINPE") {
			sinIBAN = true
		}
		if a.Tono == "WARN" && strings.Contains(a.Texto, "SIN base legal") {
			legal = true
		}
	}
	if !sinIBAN {
		t.Errorf("falta la alerta de IBAN: %+v", d.Alertas)
	}
	if !legal {
		t.Errorf("falta la alerta del guardarraíl: %+v", d.Alertas)
	}
	// Mes sin corrida: todo en cero, sin dividir por cero.
	if d.CostoReal != "0.00" || d.CostoPor100 != "0" || d.PatronalPct != "0.00" {
		t.Errorf("mes vacío: costo=%s por100=%s pct=%s, quiere ceros", d.CostoReal, d.CostoPor100, d.PatronalPct)
	}
	if _, err := svc.Dashboard(context.Background(), "e1", 2026, 13); !errors.Is(err, ErrMesInvalido) {
		t.Errorf("mes 13: err = %v, quiere ErrMesInvalido", err)
	}
}
