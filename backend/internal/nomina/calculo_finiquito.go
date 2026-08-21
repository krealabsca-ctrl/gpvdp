package nomina

// Motor PURO del finiquito, conforme al Código de Trabajo (la maqueta aprobada dice
// "conforme al CT"; aquí van las escalas EXACTAS de los arts. 28 y 29, no la
// simplificación del demo). Todo sobre el salario promedio real — nunca base reducida.

import (
	"time"

	"github.com/shopspring/decimal"
)

// diasCesantiaPorAnio devuelve los días de cesantía POR AÑO según los años totales de
// servicio (CT art. 29, escala vigente). Para menos de 1 año devuelve el TOTAL de días.
func diasCesantiaPorAnio(anios int, meses int) decimal.Decimal {
	if anios < 1 {
		switch {
		case meses >= 6:
			return decimal.NewFromInt(14) // 6 meses a 1 año: 14 días (total)
		case meses >= 3:
			return decimal.NewFromInt(7) // 3 a 6 meses: 7 días (total)
		default:
			return decimal.Zero
		}
	}
	escala := map[int]string{
		1: "19.5", 2: "20", 3: "20.5", 4: "21", 5: "21.24",
		6: "21.5", 7: "22", 8: "22", 9: "22", 10: "21.5",
		11: "21", 12: "20.5",
	}
	if s, ok := escala[anios]; ok {
		return decimal.RequireFromString(s)
	}
	return decimal.NewFromInt(20) // 13 años o más
}

// diasPreaviso devuelve los días de preaviso según antigüedad (CT art. 28).
func diasPreaviso(anios, meses int) int {
	switch {
	case anios >= 1:
		return 30 // un mes
	case meses >= 6:
		return 15 // quincena
	case meses >= 3:
		return 7 // una semana
	default:
		return 0
	}
}

// EntradaFiniquito es la entrada del cálculo (sin I/O).
type EntradaFiniquito struct {
	Motivo          string
	FechaIngreso    time.Time
	FechaSalida     time.Time
	SalarioPromedio decimal.Decimal // base CCSS promedio de las últimas liquidaciones pagadas
	DiasVacaciones  decimal.Decimal
	// Créditos familiares para la renta de la porción afecta (vacaciones).
	Hijos   int
	Conyuge bool
	// Descuentos: adelanto del mes de salida aún no descontado + saldos de deducciones.
	AdelantoPendiente decimal.Decimal
	SaldosDeducciones []DeduccionCalc // se aplican por prelación hasta agotar el total
}

// ResultadoFiniquito es la liquidación calculada.
type ResultadoFiniquito struct {
	SalarioDiario decimal.Decimal
	AniosServicio int
	Preaviso      decimal.Decimal
	Cesantia      decimal.Decimal
	Vacaciones    decimal.Decimal
	Aguinaldo     decimal.Decimal
	// BaseCCSS es la porción AFECTA del finiquito (las vacaciones pendientes son salario;
	// preaviso, cesantía y aguinaldo son exentos) y sus retenciones.
	BaseCCSS   decimal.Decimal
	CCSSObrero decimal.Decimal
	Renta      decimal.Decimal
	// Patronal es la carga patronal sobre esa base afecta: costo de la empresa que va a la
	// planilla CCSS del mes del cese. NO se resta del finiquito ni sale en el documento.
	Patronal   decimal.Decimal
	Descuentos decimal.Decimal
	Total      decimal.Decimal
	Detalle    []DetalleLinea
}

// CalcularFiniquito computa la liquidación de cese.
//
//   - DESPIDO CON RESPONSABILIDAD PATRONAL: preaviso + auxilio de cesantía (tope 8 años
//     de cómputo, exentos de CCSS y renta) + vacaciones + aguinaldo proporcional.
//   - RENUNCIA / FIN DE CONTRATO / MUTUO ACUERDO: solo vacaciones + aguinaldo (la
//     cesantía no procede; cualquier monto negociado extra se registra aparte).
//
// Las vacaciones pendientes SÍ son salario (afectan CCSS/renta — se reportan en planilla);
// el aguinaldo proporcional es exento. El descuento aplica el adelanto de quincena aún no
// descontado y los saldos de préstamos, por prelación, topados al total (piso 0).
func CalcularFiniquito(e EntradaFiniquito, p ParametrosCalc) ResultadoFiniquito {
	var r ResultadoFiniquito
	detalle := make([]DetalleLinea, 0, 8)

	anios, meses := antiguedad(e.FechaIngreso, e.FechaSalida)
	r.AniosServicio = anios
	r.SalarioDiario = redondear(e.SalarioPromedio.Div(decimal.NewFromInt(30)), p.Redondeo)
	treinta := decimal.NewFromInt(30)

	if e.Motivo == MotivoDespido {
		// Preaviso (CT art. 28): si no se otorgó trabajado, se paga.
		dias := decimal.NewFromInt(int64(diasPreaviso(anios, meses)))
		r.Preaviso = redondear(e.SalarioPromedio.Mul(dias).Div(treinta), p.Redondeo)
		if r.Preaviso.IsPositive() {
			detalle = append(detalle, DetalleLinea{Tipo: "INGRESO",
				Nombre: "Preaviso (" + dias.String() + " días, CT art. 28)", Monto: r.Preaviso.StringFixed(2)})
		}
		// Auxilio de cesantía (CT art. 29): días/año según antigüedad, tope 8 años de
		// cómputo; exento de CCSS y renta.
		diasAnio := diasCesantiaPorAnio(anios, meses)
		if anios < 1 {
			r.Cesantia = redondear(r.SalarioDiario.Mul(diasAnio), p.Redondeo)
		} else {
			computo := decimal.NewFromInt(int64(min(anios, 8)))
			r.Cesantia = redondear(r.SalarioDiario.Mul(diasAnio).Mul(computo), p.Redondeo)
		}
		if r.Cesantia.IsPositive() {
			detalle = append(detalle, DetalleLinea{Tipo: "INGRESO",
				Nombre: "Auxilio de cesantía (" + diasAnio.String() + " días/año, tope 8 años — exento CCSS y renta)",
				Monto:  r.Cesantia.StringFixed(2)})
		}
	}

	// Vacaciones no disfrutadas (SÍ son salario: se reportan afectas en planilla).
	if e.DiasVacaciones.IsPositive() {
		r.Vacaciones = redondear(r.SalarioDiario.Mul(e.DiasVacaciones), p.Redondeo)
		detalle = append(detalle, DetalleLinea{Tipo: "INGRESO",
			Nombre: "Vacaciones pendientes (" + e.DiasVacaciones.String() + " días — afectas a CCSS y renta)",
			Monto:  r.Vacaciones.StringFixed(2)})
	}

	// Aguinaldo proporcional (exento): salarios desde el 1 de diciembre anterior (o el
	// ingreso, si fue después) hasta la salida, entre 12. Meses de 30 días.
	inicioAgui := time.Date(e.FechaSalida.Year()-1, time.December, 1, 0, 0, 0, 0, time.UTC)
	if e.FechaSalida.Month() == time.December {
		inicioAgui = time.Date(e.FechaSalida.Year(), time.December, 1, 0, 0, 0, 0, time.UTC)
	}
	if e.FechaIngreso.After(inicioAgui) {
		inicioAgui = e.FechaIngreso
	}
	mesesAgui := mesesTrabajados(inicioAgui, e.FechaSalida)
	r.Aguinaldo = redondear(e.SalarioPromedio.Mul(mesesAgui).Div(decimal.NewFromInt(12)), p.Redondeo)
	if r.Aguinaldo.IsPositive() {
		detalle = append(detalle, DetalleLinea{Tipo: "INGRESO",
			Nombre: "Aguinaldo proporcional (" + mesesAgui.StringFixed(2) + " meses — exento)",
			Monto:  r.Aguinaldo.StringFixed(2)})
	}

	// Retenciones sobre la porción AFECTA: las vacaciones pendientes SON salario y por
	// tanto cotizan y tributan (guardarraíl); preaviso, cesantía y aguinaldo son exentos.
	r.BaseCCSS = r.Vacaciones
	if r.BaseCCSS.IsPositive() {
		for _, c := range p.Cargas {
			if c.Tipo != CargaObrero {
				continue
			}
			monto := redondear(r.BaseCCSS.Mul(c.Pct).Div(cien), p.Redondeo)
			r.CCSSObrero = r.CCSSObrero.Add(monto)
			detalle = append(detalle, DetalleLinea{Tipo: "CCSS",
				Nombre: c.Nombre + " (" + c.Pct.String() + "% sobre vacaciones)", Monto: monto.StringFixed(2)})
		}
		r.Renta = calcularRenta(r.BaseCCSS, e.Hijos, e.Conyuge, p)
		if r.Renta.IsPositive() {
			detalle = append(detalle, DetalleLinea{Tipo: "RENTA",
				Nombre: "Impuesto al salario sobre vacaciones", Monto: r.Renta.StringFixed(2)})
		}
		// Carga patronal de esa misma base (va a la planilla CCSS del mes). El detalle se
		// descarta a propósito: el documento que firma la persona lista solo lo que se le
		// paga y lo que se le retiene, no el costo del patrono.
		r.Patronal, _ = CargasPatronales(r.BaseCCSS, p)
	}

	bruto := r.Preaviso.Add(r.Cesantia).Add(r.Vacaciones).Add(r.Aguinaldo)
	disponible := bruto.Sub(r.CCSSObrero).Sub(r.Renta)

	// Descuentos: primero el adelanto de quincena aún no descontado (dinero ya recibido),
	// luego los saldos de préstamos por prelación — topados al disponible (piso 0).
	if e.AdelantoPendiente.IsPositive() {
		monto := e.AdelantoPendiente
		if disponible.LessThan(monto) {
			monto = disponible
		}
		if monto.IsPositive() {
			r.Descuentos = r.Descuentos.Add(monto)
			disponible = disponible.Sub(monto)
			detalle = append(detalle, DetalleLinea{Tipo: "ADELANTO",
				Nombre: "Adelanto de quincena pagado y no descontado", Monto: monto.StringFixed(2)})
		}
	}
	deds := ordenarPorPrelacion(e.SaldosDeducciones)
	for _, d := range deds {
		if d.SaldoRestante == nil || !d.SaldoRestante.IsPositive() {
			continue // sin tope no hay deuda exigible en el cese
		}
		monto := *d.SaldoRestante
		if disponible.LessThan(monto) {
			monto = disponible
		}
		if !monto.IsPositive() {
			continue
		}
		r.Descuentos = r.Descuentos.Add(monto)
		disponible = disponible.Sub(monto)
		detalle = append(detalle, DetalleLinea{Tipo: "DEDUCCION",
			Nombre: d.Etiqueta + " (saldo al cese)", Monto: monto.StringFixed(2), DeduccionID: d.ID})
	}

	r.Total = bruto.Sub(r.CCSSObrero).Sub(r.Renta).Sub(r.Descuentos)
	r.Detalle = detalle
	return r
}

// antiguedad devuelve años completos y meses completos de servicio.
func antiguedad(ingreso, salida time.Time) (anios, meses int) {
	if salida.Before(ingreso) {
		return 0, 0
	}
	anios = salida.Year() - ingreso.Year()
	meses = int(salida.Month()) - int(ingreso.Month())
	if salida.Day() < ingreso.Day() {
		meses--
	}
	if meses < 0 {
		anios--
		meses += 12
	}
	return anios, meses
}

// mesesTrabajados cuenta meses (con fracción, base 30 días) entre dos fechas, tope 12.
func mesesTrabajados(desde, hasta time.Time) decimal.Decimal {
	if hasta.Before(desde) {
		return decimal.Zero
	}
	aniosD := hasta.Year() - desde.Year()
	mesesD := int(hasta.Month()) - int(desde.Month()) + aniosD*12
	dias := hasta.Day() - desde.Day() + 1 // ambos días inclusive
	meses := decimal.NewFromInt(int64(mesesD))
	fraccion := decimal.NewFromInt(int64(dias)).Div(decimal.NewFromInt(30))
	if fraccion.GreaterThan(decimal.NewFromInt(1)) {
		fraccion = decimal.NewFromInt(1)
	}
	if fraccion.IsNegative() {
		meses = meses.Sub(decimal.NewFromInt(1))
		fraccion = fraccion.Add(decimal.NewFromInt(1))
	}
	total := meses.Add(fraccion)
	doce := decimal.NewFromInt(12)
	if total.GreaterThan(doce) {
		return doce
	}
	if total.IsNegative() {
		return decimal.Zero
	}
	return total
}

// ordenarPorPrelacion devuelve las deducciones ordenadas por prioridad (copia).
func ordenarPorPrelacion(deds []DeduccionCalc) []DeduccionCalc {
	out := append([]DeduccionCalc(nil), deds...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Prioridad < out[j-1].Prioridad ||
			(out[j].Prioridad == out[j-1].Prioridad && out[j].Etiqueta < out[j-1].Etiqueta)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
