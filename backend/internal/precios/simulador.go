package precios

// Simulador: mover una variable y ver el efecto antes de decidir.
//
// ── QUÉ CONTESTA, Y POR QUÉ HOY NADIE PUEDE ─────────────────────────────────────────────────
//
//   · Si el cofre sube 12 %, ¿qué pasa con el margen de Oro?
//   · ¿A qué cuota empata Gardenia?
//   · Si Jade sube a ₡550.000 y se pierde volumen, ¿se gana o se pierde?
//   · ¿Cuántos servicios al año aguanta un plan de suscripción antes de perder plata?
//
// ── LA PIEZA QUE SOLO EXISTE EN SUSCRIPCIÓN: LA FRECUENCIA ──────────────────────────────────
//
// Un plan directo se vende y se presta: el costo del servicio entra entero. Uno de suscripción
// cobra una cuota todos los meses y presta el servicio UNA VEZ, cuando toca — o nunca, si el
// contrato se cancela antes. Entonces el costo esperado no es el costo del servicio: es el costo
// del servicio POR LA PROBABILIDAD de usarlo.
//
//	costo esperado = costo del servicio × frecuencia
//
// De ahí sale la cifra que más decide: la FRECUENCIA DE EQUILIBRIO, o sea cuánta siniestralidad
// aguanta el plan antes de perder plata. Medido con la propuesta: Gardenia 2,53 %, Olivo 2,52 %,
// Tulipán 4,22 %, Cedro 5,63 %.
//
// ── LO QUE EL SIMULADOR NO HACE ─────────────────────────────────────────────────────────────
//
// No escribe nada y no decide la frecuencia. La recibe como supuesto y la devuelve declarada, al
// lado del resultado, porque el número de donde todo cuelga —si el 0,47 % histórico es por
// contrato o por beneficiario— es una definición que está pendiente. Un simulador que la fijara
// solo escondería la pregunta.

import (
	"github.com/shopspring/decimal"
)

// Escenario son los supuestos con que se simula. Todo opcional: lo que no venga sale del plan.
type Escenario struct {
	// Overrides del plan.
	PrecioVenta      *string `json:"precio_venta"`
	CuotaMensual     *string `json:"cuota_mensual"`
	PermanenciaMeses *int    `json:"permanencia_meses"`

	// AjusteCostosPct mueve TODO el costo de la receta: «si los insumos suben 12 %».
	AjusteCostosPct *string `json:"ajuste_costos_pct"`

	// FrecuenciaAnualPct es la probabilidad anual de que el contrato use el servicio. Solo
	// aplica a suscripción. Sin ella no se puede calcular el costo esperado.
	FrecuenciaAnualPct *string `json:"frecuencia_anual_pct"`

	// Qué se incluye en el resultado. Por defecto, todo lo que haya.
	SinComision  bool `json:"sin_comision"`
	SinIndirecto bool `json:"sin_indirecto"`
}

// Resultado es lo que da la simulación, abierto en sus partes para que cada cifra se pueda
// rastrear hasta de dónde salió.
type Resultado struct {
	Plan      string `json:"plan"`
	Modalidad string `json:"modalidad"`

	// ── Lo que entra ──
	Ingreso string `json:"ingreso"`
	// IngresoDetalle explica el ingreso: «₡4.950 × 36 meses» o «precio de venta».
	IngresoDetalle string `json:"ingreso_detalle"`

	// ── Lo que sale ──
	CostoServicio string `json:"costo_servicio"`
	// CostoEsperado es el del servicio ajustado por frecuencia. En directo es igual al del
	// servicio; en suscripción es menor, porque no todos los contratos lo usan.
	CostoEsperado string `json:"costo_esperado"`
	Indirecto     string `json:"indirecto"`
	Comision      string `json:"comision"`
	CostoTotal    string `json:"costo_total"`

	// ── El resultado ──
	Margen    string `json:"margen"`
	MargenPct string `json:"margen_pct"`

	// FrecuenciaEquilibrio es, en suscripción, cuánta siniestralidad aguanta antes de perder
	// plata. nil en planes directos, donde no hay probabilidad: el servicio se presta siempre.
	FrecuenciaEquilibrio *string `json:"frecuencia_equilibrio"`
	FrecuenciaUsada      *string `json:"frecuencia_usada"`

	// ── Lo que no se pudo calcular ──
	//
	// Confiable es falso cuando algo faltó: un insumo sin costo, un driver sin base, una comisión
	// sin política. El número igual se muestra, pero marcado — un resultado parcial leído como
	// completo es el defecto que el módulo entero existe para evitar.
	Confiable bool     `json:"confiable"`
	Faltantes []string `json:"faltantes"`
}

// Insumos del cálculo, ya resueltos por el servicio.
type InsumosDeSimulacion struct {
	Plan          Plan
	CostoVariable decimal.Decimal
	CostoFijo     decimal.Decimal
	// CosteoCompleto es falso si a la receta le falta algún costo.
	CosteoCompleto bool
	Faltantes      []string

	// Indirecto absorbido por período (de ABC). nil si no se pudo calcular.
	IndirectoPorPeriodo *decimal.Decimal
	IndirectoCompleto   bool

	// Comisión: costo total con cargas. nil si no hay política.
	Comision *decimal.Decimal
}

var docena = decimal.NewFromInt(12)

// Simular aplica el escenario y devuelve el resultado.
func Simular(in InsumosDeSimulacion, e Escenario) (Resultado, error) {
	r := Resultado{
		Plan: in.Plan.Nombre, Modalidad: in.Plan.Modalidad,
		Confiable: true, Faltantes: []string{},
	}

	// ── 1 · El costo del servicio, con el ajuste del escenario ──
	costo := in.CostoVariable.Add(in.CostoFijo)
	if e.AjusteCostosPct != nil {
		aj, err := decimal.NewFromString(*e.AjusteCostosPct)
		if err != nil {
			return Resultado{}, ErrAjusteInvalido
		}
		costo = costo.Mul(cien.Add(aj)).Div(cien).Round(2)
	}
	r.CostoServicio = costo.StringFixed(2)

	if !in.CosteoCompleto {
		r.Confiable = false
		for _, f := range in.Faltantes {
			r.Faltantes = append(r.Faltantes, "sin costo: "+f)
		}
	}

	// ── 2 · El ingreso ──
	plan := in.Plan
	if e.PrecioVenta != nil {
		plan.PrecioVenta = e.PrecioVenta
	}
	if e.CuotaMensual != nil {
		plan.CuotaMensual = e.CuotaMensual
	}
	if e.PermanenciaMeses != nil {
		plan.PermanenciaMeses = e.PermanenciaMeses
	}

	ingreso := precioDeReferencia(plan)
	if ingreso == nil {
		return Resultado{}, ErrSinPrecioParaSimular
	}
	r.Ingreso = ingreso.StringFixed(2)
	if plan.Modalidad == ModalidadSuscripcion && plan.PermanenciaMeses != nil {
		r.IngresoDetalle = *plan.CuotaMensual + " × " + itoaSimple(*plan.PermanenciaMeses) + " meses"
	} else {
		r.IngresoDetalle = "precio de venta"
	}

	// ── 3 · El indirecto ──
	indirecto := decimal.Zero
	if !e.SinIndirecto {
		if in.IndirectoPorPeriodo == nil {
			r.Confiable = false
			r.Faltantes = append(r.Faltantes, "el indirecto no se pudo calcular: falta el reparto o la base de algún driver")
		} else {
			indirecto = *in.IndirectoPorPeriodo
			if !in.IndirectoCompleto {
				r.Confiable = false
				r.Faltantes = append(r.Faltantes, "el indirecto está incompleto: algún driver no tiene costo unitario")
			}
		}
	}
	r.Indirecto = indirecto.StringFixed(2)

	// ── 4 · La comisión ──
	comision := decimal.Zero
	if !e.SinComision {
		if in.Comision == nil {
			r.Confiable = false
			r.Faltantes = append(r.Faltantes, "no hay política de comisión para este plan")
		} else {
			comision = *in.Comision
		}
	}
	r.Comision = comision.StringFixed(2)

	// ── 5 · El costo esperado y la frecuencia ──
	//
	// Acá está la diferencia entre las dos modalidades. En directo el servicio se presta siempre;
	// en suscripción se presta con una probabilidad, y multiplicarlo por ella es lo único que
	// permite comparar una cuota mensual contra el costo de un funeral.
	esperado := costo
	if plan.Modalidad == ModalidadSuscripcion {
		if e.FrecuenciaAnualPct == nil {
			r.Confiable = false
			r.Faltantes = append(r.Faltantes, "falta la frecuencia de uso: sin ella no hay costo esperado en un plan de suscripción")
			esperado = costo // peor caso: todos usan el servicio
		} else {
			f, err := decimal.NewFromString(*e.FrecuenciaAnualPct)
			if err != nil || f.IsNegative() {
				return Resultado{}, ErrFrecuenciaInvalida
			}
			s := f.StringFixed(2)
			r.FrecuenciaUsada = &s
			// La frecuencia es ANUAL y la permanencia viene en meses.
			meses := decimal.NewFromInt(int64(*plan.PermanenciaMeses))
			anios := meses.Div(docena)
			esperado = costo.Mul(f).Div(cien).Mul(anios).Round(2)
		}
	}
	r.CostoEsperado = esperado.StringFixed(2)

	total := esperado.Add(indirecto).Add(comision)
	r.CostoTotal = total.StringFixed(2)

	margen := ingreso.Sub(total).Round(2)
	r.Margen = margen.StringFixed(2)
	if ingreso.GreaterThan(decimal.Zero) {
		r.MargenPct = margen.Div(*ingreso).Mul(cien).Round(1).StringFixed(1)
	} else {
		r.MargenPct = "0.0"
	}

	// ── 6 · La frecuencia de equilibrio ──
	//
	// Cuánta siniestralidad aguanta el plan antes de perder plata:
	//
	//	(ingreso − indirecto − comisión) ÷ costo del servicio ÷ años, en porcentaje
	//
	// Es la cifra que decide si un plan de suscripción se sostiene, y no existe en el archivo de
	// costeo actual.
	if plan.Modalidad == ModalidadSuscripcion && costo.GreaterThan(decimal.Zero) && plan.PermanenciaMeses != nil {
		disponible := ingreso.Sub(indirecto).Sub(comision)
		anios := decimal.NewFromInt(int64(*plan.PermanenciaMeses)).Div(docena)
		if anios.GreaterThan(decimal.Zero) {
			eq := disponible.Div(costo).Div(anios).Mul(cien).Round(2)
			if eq.IsNegative() {
				eq = decimal.Zero
			}
			s := eq.StringFixed(2)
			r.FrecuenciaEquilibrio = &s
		}
	}
	return r, nil
}

// itoaSimple evita importar strconv solo para esto.
func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
