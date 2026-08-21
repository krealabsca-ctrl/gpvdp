package nomina

// Motor de cálculo PURO de la corrida (sin I/O): espejo del motor aprobado en la maqueta.
//
// GUARDARRAÍL LEGAL: la base CCSS es la suma de TODOS los ingresos afectos (salario,
// comisiones, extras, bonos habituales — son salario por ley). Las banderas de afectación
// vienen del concepto (los de sistema están bloqueados). Nada aquí cuantifica ni optimiza
// "ahorro por no reportar": el detalle expone la base CCSS íntegra en cada colilla.

import (
	"sort"

	"github.com/shopspring/decimal"
)

// CargaCalc es una carga social lista para calcular (pct ya parseado).
type CargaCalc struct {
	Codigo string
	Nombre string
	Tipo   string // OBRERO | PATRONAL
	Pct    decimal.Decimal
}

// TramoCalc es un tramo de renta parseado. Hasta nil = tramo final abierto.
type TramoCalc struct {
	Hasta *decimal.Decimal
	Pct   decimal.Decimal
}

// ParametrosCalc son los parámetros del año parseados a decimal (desde el snapshot jsonb).
type ParametrosCalc struct {
	Cargas         []CargaCalc
	Tramos         []TramoCalc
	CreditoHijo    decimal.Decimal
	CreditoConyuge decimal.Decimal
	INSRiesgosPct  decimal.Decimal
	AplicaINA      bool
	AdelantoPct    decimal.Decimal
	Redondeo       string // COLON | CENTIMO
	AguinaldoPct   decimal.Decimal
	VacacionesPct  decimal.Decimal
	CesantiaPct    decimal.Decimal
}

// IngresoCalc es un ingreso del mes (salario base o novedad) con sus banderas.
type IngresoCalc struct {
	Nombre string
	Monto  decimal.Decimal
	// Horas > 0: la novedad se capturó por HORAS y su monto lo deriva resolverHorasExtra
	// con el salario vigente. Cero = monto directo (comisión, bono, viático).
	Horas           decimal.Decimal
	AfectaCCSS      bool
	AfectaRenta     bool
	AfectaAguinaldo bool
}

// DeduccionCalc es una deducción recurrente vigente lista para aplicar.
type DeduccionCalc struct {
	ID            string
	Etiqueta      string
	Cuota         decimal.Decimal
	SaldoRestante *decimal.Decimal // nil = sin tope
	Prioridad     int
	// Frecuencia de cobro: AMBAS | PRIMERA | SEGUNDA | MENSUAL. El servicio filtra por
	// ella antes de llamar al motor (el motor recibe solo las que tocan este período).
	Frecuencia string
}

// Frecuencias de cobro de una deducción (espejo del CHECK de la migración 0036).
const (
	FrecAmbas   = "AMBAS"   // cada quincena
	FrecPrimera = "PRIMERA" // solo la 1ª quincena
	FrecSegunda = "SEGUNDA" // solo la 2ª quincena
	FrecMensual = "MENSUAL" // una vez al mes (se cobra en la 2ª quincena)
)

// CobraEn indica si la deducción se cobra en el período dado. `primera` distingue la 1ª
// quincena de la 2ª; en una liquidación mensual (un solo pago) siempre se cobra una vez.
func CobraEn(frecuencia string, primera, esQuincenal bool) bool {
	if !esQuincenal {
		return true // jornada mensual: un único cálculo, la cuota se cobra ahí
	}
	switch frecuencia {
	case FrecAmbas:
		return true
	case FrecPrimera:
		return primera
	default: // SEGUNDA y MENSUAL se cobran al cerrar el mes
		return !primera
	}
}

// RentaPeriodo describe cómo se retiene el impuesto al salario en este período, dado que
// los tramos son MENSUALES (decisión del DF: mitad estimada el día 15, ajuste real el 30).
//
//   - BaseMensual: base afecta del MES para entrar a los tramos (cero = usar la del período,
//     que es el comportamiento de la liquidación mensual).
//   - Fraccion: parte del impuesto mensual que retiene este período (cero = 1, todo).
//   - YaRetenida: lo retenido antes en el mismo mes, que se descuenta del resultado.
type RentaPeriodo struct {
	BaseMensual decimal.Decimal
	Fraccion    decimal.Decimal
	YaRetenida  decimal.Decimal
}

// EmpleadoCalc es la entrada del cálculo de una línea.
type EmpleadoCalc struct {
	Hijos       int
	Conyuge     bool
	SalarioBase decimal.Decimal
	// FraccionMes es la proporción del mes efectivamente laborada (1 = mes completo;
	// menor si ingresó o salió a mitad de mes, base 30 días). Cero se trata como 1.
	FraccionMes    decimal.Decimal
	Ingresos       []IngresoCalc   // salario (ya prorrateado) + novedades del mes (LIQUIDACION)
	Deducciones    []DeduccionCalc // las que cobran en ESTE período, por prioridad
	AdelantoPagado decimal.Decimal // adelanto de la jornada mensual (0 en pago quincenal real)
	// Renta: cómo se retiene el impuesto en este período (ver RentaPeriodo). Cero = se
	// grava la base del propio período por los tramos completos (liquidación mensual).
	Renta RentaPeriodo
}

// fraccion devuelve la proporción del mes laborada (1 si no viene informada).
func (e EmpleadoCalc) fraccion() decimal.Decimal {
	if e.FraccionMes.IsZero() {
		return decimal.NewFromInt(1)
	}
	return e.FraccionMes
}

// ResultadoLinea es la colilla calculada de un empleado.
type ResultadoLinea struct {
	Bruto          decimal.Decimal
	BaseCCSS       decimal.Decimal
	BaseRenta      decimal.Decimal
	CCSSObrero     decimal.Decimal
	Renta          decimal.Decimal
	Deducciones    decimal.Decimal
	Adelanto       decimal.Decimal
	Neto           decimal.Decimal
	Patronal       decimal.Decimal
	ProvAguinaldo  decimal.Decimal
	ProvVacaciones decimal.Decimal
	ProvCesantia   decimal.Decimal
	Detalle        []DetalleLinea
}

// restarPiso0 resta b de a sin bajar de cero (un descuento nunca vuelve negativo un monto).
func restarPiso0(a, b decimal.Decimal) decimal.Decimal {
	r := a.Sub(b)
	if r.IsNegative() {
		return decimal.Zero
	}
	return r
}

// redondear aplica la política de redondeo (maqueta: al colón).
func redondear(v decimal.Decimal, politica string) decimal.Decimal {
	if politica == "CENTIMO" {
		return v.Round(2)
	}
	return v.Round(0)
}

// CalcularAdelanto calcula la línea de la corrida ADELANTO: % del salario base (prorrateado
// si el mes es parcial), sin deducciones — la liquidación del día 30 asienta el mes y lo descuenta.
func CalcularAdelanto(e EmpleadoCalc, p ParametrosCalc) ResultadoLinea {
	monto := redondear(e.SalarioBase.Mul(p.AdelantoPct).Div(cien).Mul(e.fraccion()), p.Redondeo)
	nombre := "Adelanto de quincena (" + p.AdelantoPct.String() + "% del salario base)"
	if e.fraccion().LessThan(decimal.NewFromInt(1)) {
		nombre += " — mes parcial"
	}
	return ResultadoLinea{
		Bruto: monto,
		Neto:  monto,
		Detalle: []DetalleLinea{
			{Tipo: "INGRESO", Nombre: nombre, Monto: monto.StringFixed(2)},
		},
	}
}

// CalcularLiquidacion calcula la colilla del mes completo de un empleado.
func CalcularLiquidacion(e EmpleadoCalc, p ParametrosCalc) ResultadoLinea {
	var r ResultadoLinea
	detalle := make([]DetalleLinea, 0, len(e.Ingresos)+len(e.Deducciones)+8)

	// 1. Ingresos y bases: comisiones/extras/bonos afectan CCSS por sus banderas (guardarraíl).
	baseAguinaldo := decimal.Zero
	for _, ing := range e.Ingresos {
		r.Bruto = r.Bruto.Add(ing.Monto)
		if ing.AfectaCCSS {
			r.BaseCCSS = r.BaseCCSS.Add(ing.Monto)
		}
		if ing.AfectaRenta {
			r.BaseRenta = r.BaseRenta.Add(ing.Monto)
		}
		if ing.AfectaAguinaldo {
			baseAguinaldo = baseAguinaldo.Add(ing.Monto)
		}
		detalle = append(detalle, DetalleLinea{Tipo: "INGRESO", Nombre: ing.Nombre, Monto: ing.Monto.StringFixed(2)})
	}

	// 2. Cargas obreras: cada carga se redondea por separado (colilla y total cuadran exacto).
	for _, c := range p.Cargas {
		if c.Tipo != CargaObrero {
			continue
		}
		monto := redondear(r.BaseCCSS.Mul(c.Pct).Div(cien), p.Redondeo)
		r.CCSSObrero = r.CCSSObrero.Add(monto)
		detalle = append(detalle, DetalleLinea{Tipo: "CCSS", Nombre: c.Nombre + " (" + c.Pct.String() + "%)", Monto: monto.StringFixed(2)})
	}

	// 3. Renta: los tramos son MENSUALES, así que el impuesto se calcula sobre la base del
	//    MES y luego se retiene la parte que toca a este período, menos lo ya retenido.
	//    Liquidación mensual: base del período, fracción 1, nada retenido antes.
	//    1ª quincena: base mensual estimada, fracción ½. 2ª quincena: base mensual real,
	//    fracción 1, menos lo retenido el día 15 → el ajuste. El total del mes queda exacto.
	baseTramos := r.BaseRenta
	if e.Renta.BaseMensual.IsPositive() {
		baseTramos = e.Renta.BaseMensual
	}
	impuestoMes := calcularRenta(baseTramos, e.Hijos, e.Conyuge, p)
	fraccionRenta := e.Renta.Fraccion
	if fraccionRenta.IsZero() {
		fraccionRenta = decimal.NewFromInt(1)
	}
	r.Renta = redondear(impuestoMes.Mul(fraccionRenta), p.Redondeo).Sub(e.Renta.YaRetenida)
	if r.Renta.IsNegative() {
		r.Renta = decimal.Zero // ya se retuvo de más antes en el mes: no se devuelve aquí
	}
	if r.Renta.IsPositive() {
		nombre := "Impuesto al salario (tras créditos familiares)"
		switch {
		case fraccionRenta.LessThan(decimal.NewFromInt(1)):
			nombre = "Impuesto al salario — mitad estimada del mes"
		case e.Renta.YaRetenida.IsPositive():
			nombre = "Impuesto al salario — ajuste del mes (ya retenido " +
				e.Renta.YaRetenida.StringFixed(2) + ")"
		}
		detalle = append(detalle, DetalleLinea{Tipo: "RENTA", Nombre: nombre, Monto: r.Renta.StringFixed(2)})
	}

	// 4. Adelanto del mes: se descuenta lo REALMENTE pagado en la corrida ADELANTO.
	r.Adelanto = e.AdelantoPagado
	if r.Adelanto.IsPositive() {
		detalle = append(detalle, DetalleLinea{Tipo: "ADELANTO", Nombre: "Adelanto de quincena ya pagado (día 15)", Monto: r.Adelanto.StringFixed(2)})
	}

	// 5. Deducciones recurrentes con prelación: menor prioridad primero (pensión, embargo…);
	//    cada una aplica hasta su cuota, topada por su saldo restante y por el neto disponible
	//    (si el neto se agota, las siguientes quedan pospuestas — parcial incluido).
	deds := append([]DeduccionCalc(nil), e.Deducciones...)
	sort.SliceStable(deds, func(i, j int) bool {
		if deds[i].Prioridad != deds[j].Prioridad {
			return deds[i].Prioridad < deds[j].Prioridad
		}
		return deds[i].Etiqueta < deds[j].Etiqueta
	})
	disponible := r.Bruto.Sub(r.CCSSObrero).Sub(r.Renta).Sub(r.Adelanto)
	for _, d := range deds {
		// La cuota se redondea por política; los topes (saldo y neto disponible) se aplican
		// DESPUÉS y en EXACTO: el redondeo nunca cobra de más, la última cuota parcial
		// extingue el saldo al céntimo, y el neto jamás baja de cero por deducciones.
		monto := redondear(d.Cuota, p.Redondeo)
		if d.SaldoRestante != nil && d.SaldoRestante.LessThan(monto) {
			monto = *d.SaldoRestante
		}
		if disponible.LessThan(monto) {
			monto = disponible
		}
		if !monto.IsPositive() {
			continue
		}
		r.Deducciones = r.Deducciones.Add(monto)
		disponible = disponible.Sub(monto)
		detalle = append(detalle, DetalleLinea{Tipo: "DEDUCCION", Nombre: d.Etiqueta, Monto: monto.StringFixed(2), DeduccionID: d.ID})
	}

	// 6. Neto a depositar.
	r.Neto = r.Bruto.Sub(r.CCSSObrero).Sub(r.Renta).Sub(r.Deducciones).Sub(r.Adelanto)

	// 7. Costo patronal (informativo): cargas patronales + INS; el INA se excluye si la
	//    empresa está en la excepción legal (<5 empleados no agrícolas).
	patronal, detPatronal := CargasPatronales(r.BaseCCSS, p)
	r.Patronal = patronal
	detalle = append(detalle, detPatronal...)

	// 8. Provisiones informativas (aguinaldo sobre su base afecta; vacaciones y cesantía
	//    sobre la base salarial — las vacaciones son salario).
	r.ProvAguinaldo = redondear(baseAguinaldo.Mul(p.AguinaldoPct).Div(cien), p.Redondeo)
	r.ProvVacaciones = redondear(r.BaseCCSS.Mul(p.VacacionesPct).Div(cien), p.Redondeo)
	r.ProvCesantia = redondear(r.BaseCCSS.Mul(p.CesantiaPct).Div(cien), p.Redondeo)

	r.Detalle = detalle
	return r
}

// CargasPatronales calcula el costo patronal sobre una base afecta (cargas PATRONAL de los
// parámetros + INS Riesgos del Trabajo), con el detalle renglón por renglón. El INA se
// excluye si la empresa está en la excepción legal (<5 empleados no agrícolas).
//
// Lo usan la colilla de la corrida y el finiquito: las vacaciones que se pagan al cese son
// salario, así que generan la misma carga patronal y entran a la planilla del mes.
func CargasPatronales(base decimal.Decimal, p ParametrosCalc) (decimal.Decimal, []DetalleLinea) {
	total := decimal.Zero
	detalle := make([]DetalleLinea, 0, len(p.Cargas)+1)
	for _, c := range p.Cargas {
		if c.Tipo != CargaPatronal {
			continue
		}
		if c.Codigo == "INA" && !p.AplicaINA {
			continue
		}
		monto := redondear(base.Mul(c.Pct).Div(cien), p.Redondeo)
		total = total.Add(monto)
		detalle = append(detalle, DetalleLinea{Tipo: "PATRONAL", Nombre: c.Nombre + " (" + c.Pct.String() + "%)", Monto: monto.StringFixed(2)})
	}
	ins := redondear(base.Mul(p.INSRiesgosPct).Div(cien), p.Redondeo)
	total = total.Add(ins)
	detalle = append(detalle, DetalleLinea{Tipo: "PATRONAL", Nombre: "INS Riesgos del Trabajo (" + p.INSRiesgosPct.String() + "%)", Monto: ins.StringFixed(2)})
	return total, detalle
}

// calcularRenta aplica los tramos mensuales sobre la base afecta y resta los créditos
// familiares (hijo/cónyuge); nunca negativo. Espejo de rentaSalario de la maqueta.
func calcularRenta(base decimal.Decimal, hijos int, conyuge bool, p ParametrosCalc) decimal.Decimal {
	impuesto := decimal.Zero
	anterior := decimal.Zero
	for _, t := range p.Tramos {
		tope := base
		if t.Hasta != nil && t.Hasta.LessThan(base) {
			tope = *t.Hasta
		}
		porcion := tope.Sub(anterior)
		if porcion.IsPositive() {
			impuesto = impuesto.Add(porcion.Mul(t.Pct).Div(cien))
		}
		if t.Hasta == nil || !t.Hasta.LessThan(base) {
			break
		}
		anterior = *t.Hasta
	}
	creditos := p.CreditoHijo.Mul(decimal.NewFromInt(int64(hijos)))
	if conyuge {
		creditos = creditos.Add(p.CreditoConyuge)
	}
	impuesto = impuesto.Sub(creditos)
	if impuesto.IsNegative() {
		return decimal.Zero
	}
	return redondear(impuesto, p.Redondeo)
}

// parametrosACalc parsea el snapshot de Parametros (strings decimales) al motor.
// Devuelve ErrCargaInvalida / ErrTramosInvalidos si el snapshot está corrupto.
func parametrosACalc(p Parametros) (ParametrosCalc, error) {
	out := ParametrosCalc{
		AplicaINA: p.AplicaINA,
		Redondeo:  p.Redondeo,
	}
	var err error
	if out.AdelantoPct, err = decimal.NewFromString(p.AdelantoPct); err != nil {
		return out, ErrCargaInvalida
	}
	if out.INSRiesgosPct, err = decimal.NewFromString(p.INSRiesgosPct); err != nil {
		return out, ErrCargaInvalida
	}
	if out.AguinaldoPct, err = decimal.NewFromString(p.AguinaldoPct); err != nil {
		return out, ErrCargaInvalida
	}
	if out.VacacionesPct, err = decimal.NewFromString(p.VacacionesPct); err != nil {
		return out, ErrCargaInvalida
	}
	if out.CesantiaPct, err = decimal.NewFromString(p.CesantiaPct); err != nil {
		return out, ErrCargaInvalida
	}
	for _, c := range p.Cargas {
		pct, err := decimal.NewFromString(c.Pct)
		if err != nil {
			return out, ErrCargaInvalida
		}
		out.Cargas = append(out.Cargas, CargaCalc{Codigo: c.Codigo, Nombre: c.Nombre, Tipo: c.Tipo, Pct: pct})
	}
	for _, t := range p.Renta.Tramos {
		pct, err := decimal.NewFromString(t.Pct)
		if err != nil {
			return out, ErrTramosInvalidos
		}
		tc := TramoCalc{Pct: pct}
		if t.Hasta != nil {
			h, err := decimal.NewFromString(*t.Hasta)
			if err != nil {
				return out, ErrTramosInvalidos
			}
			tc.Hasta = &h
		}
		out.Tramos = append(out.Tramos, tc)
	}
	if out.CreditoHijo, err = decimal.NewFromString(p.Renta.CreditoHijo); err != nil {
		return out, ErrTramosInvalidos
	}
	if out.CreditoConyuge, err = decimal.NewFromString(p.Renta.CreditoConyuge); err != nil {
		return out, ErrTramosInvalidos
	}
	return out, nil
}
