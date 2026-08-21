package nomina

// Dashboard de RRHH: arma el resumen del mes desde datos reales (nada estimado). Lee de
// corridas VIVAS —borrador incluido— porque su función es decidir ANTES de aprobar.

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// mesesTendencia son los puntos de la gráfica de tendencia (la maqueta muestra 6).
const mesesTendencia = 6

// mesCorto son las etiquetas de la tendencia (índice 1-12).
var mesCorto = [13]string{"", "Ene", "Feb", "Mar", "Abr", "May", "Jun",
	"Jul", "Ago", "Set", "Oct", "Nov", "Dic"}

// Dashboard devuelve el resumen del mes: costo real, tendencia, composición, estado del
// ciclo, headcount y alertas.
func (s *Service) Dashboard(ctx context.Context, empresaID string, anio, mes int) (Dashboard, error) {
	if anio < 2024 || anio > 2100 || mes < 1 || mes > 12 {
		return Dashboard{}, ErrMesInvalido
	}
	d := Dashboard{Anio: anio, Mes: mes}

	resumen, err := s.repo.ResumenNominaMes(ctx, empresaID, anio, mes)
	if err != nil {
		return Dashboard{}, err
	}
	bruto, err := decimal.NewFromString(resumen.Bruto)
	if err != nil {
		return Dashboard{}, fmt.Errorf("nomina: bruto del mes corrupto: %w", err)
	}
	patronal, err := decimal.NewFromString(resumen.Patronal)
	if err != nil {
		return Dashboard{}, fmt.Errorf("nomina: patronal del mes corrupto: %w", err)
	}
	baseCCSS, err := decimal.NewFromString(resumen.BaseCCSS)
	if err != nil {
		return Dashboard{}, fmt.Errorf("nomina: base CCSS del mes corrupta: %w", err)
	}
	provisiones := decimal.Zero
	for _, prov := range []struct{ nombre, valor string }{
		{"aguinaldo", resumen.ProvAguinaldo},
		{"vacaciones", resumen.ProvVacaciones},
		{"cesantía", resumen.ProvCesantia},
	} {
		p, err := decimal.NewFromString(prov.valor)
		if err != nil {
			return Dashboard{}, fmt.Errorf("nomina: provisión de %s corrupta: %w", prov.nombre, err)
		}
		provisiones = provisiones.Add(p)
	}
	costoReal := bruto.Add(patronal).Add(provisiones)

	d.Bruto = bruto.StringFixed(2)
	d.Patronal = patronal.StringFixed(2)
	d.BaseCCSS = baseCCSS.StringFixed(2)
	d.Provisiones = provisiones.StringFixed(2)
	d.ProvAguinaldo, d.ProvVacaciones, d.ProvCesantia = resumen.ProvAguinaldo, resumen.ProvVacaciones, resumen.ProvCesantia
	d.Neto, d.NetoLiquidacion = resumen.Neto, resumen.NetoLiquidacion
	d.CostoReal = costoReal.StringFixed(2)
	d.EmpleadosPagados = resumen.Empleados
	d.PatronalPct = "0.00"
	if baseCCSS.IsPositive() {
		d.PatronalPct = patronal.Mul(cien).Div(baseCCSS).StringFixed(2)
	}
	d.CostoPor100 = "0"
	if bruto.IsPositive() {
		d.CostoPor100 = costoReal.Mul(cien).Div(bruto).Round(0).String()
	}

	if d.Tendencia, err = s.tendencia(ctx, empresaID, anio, mes); err != nil {
		return Dashboard{}, err
	}
	if d.Ciclo, err = s.ciclo(ctx, empresaID, anio, mes); err != nil {
		return Dashboard{}, err
	}
	if d.Finiquitos, err = s.finiquitosDelMes(ctx, empresaID, anio, mes); err != nil {
		return Dashboard{}, err
	}
	if d.Headcount, err = s.repo.HeadcountPorDepartamento(ctx, empresaID); err != nil {
		return Dashboard{}, err
	}
	for _, h := range d.Headcount {
		d.Empleados += h.Empleados
	}
	if d.Alertas, err = s.alertas(ctx, empresaID, anio, mes, d.Finiquitos); err != nil {
		return Dashboard{}, err
	}
	return d, nil
}

// tendencia arma los últimos `mesesTendencia` períodos hasta el mes pedido; los meses sin
// corrida quedan en cero (así se ve el hueco en vez de desaparecer del eje).
func (s *Service) tendencia(ctx context.Context, empresaID string, anio, mes int) ([]DashboardMes, error) {
	inicioAnio, inicioMes := anio, mes-(mesesTendencia-1)
	for inicioMes < 1 {
		inicioMes += 12
		inicioAnio--
	}
	desde := fmt.Sprintf("%04d-%02d-01", inicioAnio, inicioMes)
	hasta := fmt.Sprintf("%04d-%02d-01", anio, mes)
	costos, err := s.repo.TendenciaCostoNomina(ctx, empresaID, desde, hasta)
	if err != nil {
		return nil, err
	}
	porMes := make(map[int]string, len(costos))
	for _, c := range costos {
		porMes[c.Anio*100+c.Mes] = c.Costo
	}
	out := make([]DashboardMes, 0, mesesTendencia)
	a, m := inicioAnio, inicioMes
	for i := 0; i < mesesTendencia; i++ {
		costo := porMes[a*100+m]
		if costo == "" {
			costo = "0.00"
		}
		monto, err := decimal.NewFromString(costo)
		if err != nil {
			return nil, fmt.Errorf("nomina: costo de %04d-%02d corrupto: %w", a, m, err)
		}
		out = append(out, DashboardMes{Anio: a, Mes: m, Etiqueta: mesCorto[m],
			Costo: monto.StringFixed(2), EnCurso: a == anio && m == mes})
		if m++; m > 12 {
			m, a = 1, a+1
		}
	}
	return out, nil
}

// ciclo traduce las corridas vivas del mes a los tres pasos de la maqueta.
func (s *Service) ciclo(ctx context.Context, empresaID string, anio, mes int) (DashboardCiclo, error) {
	corridas, err := s.repo.CorridasVivasDelMes(ctx, empresaID, anio, mes)
	if err != nil {
		return DashboardCiclo{}, err
	}
	c := DashboardCiclo{
		Adelanto:    DashboardPaso{Estado: PasoSinCrear, Etiqueta: "Sin crear"},
		Liquidacion: DashboardPaso{Estado: PasoSinCrear, Etiqueta: "Sin crear"},
		Planilla:    DashboardPaso{Estado: PasoPendiente, Etiqueta: "Pendiente"},
	}
	for _, x := range corridas {
		paso := DashboardPaso{Estado: x.Estado, CorridaID: x.ID, Etiqueta: etiquetaEstadoCorrida(x.Estado)}
		switch x.Tipo {
		case CorridaAdelanto:
			c.Adelanto = paso
		case CorridaLiquidacion:
			c.Liquidacion = paso
			// La planilla CCSS del mes se genera de la liquidación congelada.
			if x.Estado == EstAprobada || x.Estado == EstPagada {
				c.Planilla = DashboardPaso{Estado: PasoLista, CorridaID: x.ID, Etiqueta: "Lista para generar"}
			}
		}
	}
	return c, nil
}

// etiquetaEstadoCorrida es el texto del chip del ciclo.
func etiquetaEstadoCorrida(estado string) string {
	switch estado {
	case EstBorrador:
		return "Calculada (en borrador)"
	case EstAprobada:
		return "Aprobada"
	case EstPagada:
		return "Pagada"
	default:
		return estado
	}
}

// finiquitosDelMes resume los ceses congelados con salida en el mes: se pagan por el mismo
// archivo SINPE y se reportan en la planilla, pero NO se suman al costo de la corrida (su
// cesantía y aguinaldo se venían provisionando mes a mes).
func (s *Service) finiquitosDelMes(ctx context.Context, empresaID string, anio, mes int) (DashboardFiniquito, error) {
	fins, err := s.repo.FiniquitosDelMes(ctx, empresaID, anio, mes)
	if err != nil {
		return DashboardFiniquito{}, err
	}
	out := DashboardFiniquito{Cantidad: len(fins)}
	total, patronal := decimal.Zero, decimal.Zero
	for _, f := range fins {
		t, err := decimal.NewFromString(f.Total)
		if err != nil {
			return DashboardFiniquito{}, fmt.Errorf("nomina: total de finiquito corrupto: %w", err)
		}
		p, err := decimal.NewFromString(f.Patronal)
		if err != nil {
			return DashboardFiniquito{}, fmt.Errorf("nomina: patronal de finiquito corrupto: %w", err)
		}
		total, patronal = total.Add(t), patronal.Add(p)
		if f.Estado == FinAprobado {
			out.PendientesPago++
		}
	}
	out.Total, out.Patronal = total.StringFixed(2), patronal.StringFixed(2)
	return out, nil
}

// alertas redacta los avisos accionables. Todos salen de hechos de la base: empleados sin
// IBAN, deducciones con saldo, incapacidades del mes y el control del guardarraíl (ningún
// concepto excluido de CCSS sin base legal).
func (s *Service) alertas(ctx context.Context, empresaID string, anio, mes int, fins DashboardFiniquito) ([]DashboardAlerta, error) {
	a, err := s.repo.AvisosNominaMes(ctx, empresaID, anio, mes)
	if err != nil {
		return nil, err
	}
	alertas := make([]DashboardAlerta, 0, 6)
	if fins.PendientesPago > 0 {
		plural := "finiquito aprobado"
		if fins.PendientesPago > 1 {
			plural = "finiquitos aprobados"
		}
		alertas = append(alertas, DashboardAlerta{Tono: "WARN", Icono: "🧾",
			Texto: fmt.Sprintf("%d %s del mes sin marcar como pagado: entran en el archivo SINPE de la liquidación y en la planilla CCSS.",
				fins.PendientesPago, plural)})
	}
	if a.SinIBAN > 0 {
		nombres := a.NombresSinIBAN
		sufijo := ""
		if len(nombres) > 3 {
			nombres, sufijo = nombres[:3], fmt.Sprintf(" y %d más", len(a.NombresSinIBAN)-3)
		}
		plural := "empleado"
		if a.SinIBAN > 1 {
			plural = "empleados"
		}
		alertas = append(alertas, DashboardAlerta{Tono: "WARN", Icono: "⚠️",
			Texto: fmt.Sprintf("%d %s sin IBAN (%s%s) — quedan fuera del archivo SINPE hasta que se registre la cuenta.",
				a.SinIBAN, plural, strings.Join(nombres, ", "), sufijo)})
	}
	if a.DeduccionesActivas > 0 {
		saldo, err := decimal.NewFromString(a.SaldoDeducciones)
		if err != nil {
			return nil, fmt.Errorf("nomina: saldo de deducciones corrupto: %w", err)
		}
		plural := "deducción con saldo vivo"
		if a.DeduccionesActivas > 1 {
			plural = "deducciones con saldo vivo"
		}
		alertas = append(alertas, DashboardAlerta{Tono: "INFO", Icono: "💳",
			Texto: fmt.Sprintf("%d %s por ₡%s: se aplican en la corrida por prelación, hasta donde alcance el neto.",
				a.DeduccionesActivas, plural, saldo.StringFixed(2))})
	}
	if a.IncapacidadesMes > 0 {
		plural := "incapacidad toca"
		if a.IncapacidadesMes > 1 {
			plural = "incapacidades tocan"
		}
		alertas = append(alertas, DashboardAlerta{Tono: "INFO", Icono: "🏥",
			Texto: fmt.Sprintf("%d %s este mes: la corrida ya descuenta los días que subsidian la CCSS o el INS.",
				a.IncapacidadesMes, plural)})
	}
	switch {
	case a.SinBaseLegal > 0:
		alertas = append(alertas, DashboardAlerta{Tono: "WARN", Icono: "⚖️",
			Texto: fmt.Sprintf("%d concepto(s) de ingreso están excluidos de CCSS SIN base legal registrada: corregilo antes de la corrida.",
				a.SinBaseLegal)})
	case a.ConceptosNoAfectos > 0:
		alertas = append(alertas, DashboardAlerta{Tono: "LEGAL", Icono: "⚖️",
			Texto: fmt.Sprintf("%d concepto(s) no afectos a CCSS, todos con base legal registrada; el resto cotiza íntegro.",
				a.ConceptosNoAfectos)})
	default:
		alertas = append(alertas, DashboardAlerta{Tono: "LEGAL", Icono: "⚖️",
			Texto: "Todos los conceptos de ingreso están afectos a CCSS; sin exclusiones sin base legal."})
	}
	return alertas, nil
}
