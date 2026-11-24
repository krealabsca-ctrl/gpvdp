package bancos

// PRESUPUESTO DE INGRESO (mig 0116): la mitad de la meta que faltaba.
//
// ── POR QUÉ IMPORTA ─────────────────────────────────────────────────────────────────────────
//
// El presupuesto que había era solo de GASTO. Con media meta se controla cuánto se gasta sin
// saber contra qué: no hay resultado presupuestado, ni margen, ni EBITDA presupuestado, y
// «vamos bien» no se puede afirmar mirando un solo lado.
//
// ── EL INGRESO SE MIDE AL REVÉS QUE EL GASTO, Y AHÍ SE EQUIVOCA TODO EL MUNDO ───────────────
//
// En el gasto, consumir el 95 % del presupuesto es una ALERTA: queda poco. En el ingreso, llegar
// al 95 % de la meta es casi un logro —falta poco—. El mismo porcentaje significa cosas opuestas,
// así que los estados NO se reutilizan: un semáforo copiado del gasto pintaría de rojo un mes
// bueno.
//
// Lo mismo con el signo del desvío: gastar de menos es bueno y cobrar de menos es malo.

import "github.com/shopspring/decimal"

// Estados del cumplimiento de la meta de ingreso.
//
// Son propios y no los del gasto (EN_RANGO / ALERTA / EXCEDIDO) justamente porque la dirección es
// la contraria: acá PASARSE es la buena noticia.
const (
	// IngresoSinMeta: no se declaró meta para ese período. No es un estado malo, es ausencia de
	// decisión — y mostrar 0 % de cumplimiento contra una meta inexistente sería inventarla.
	IngresoSinMeta = "SIN_META"
	// IngresoCumplido: se llegó o se pasó.
	IngresoCumplido = "CUMPLIDO"
	// IngresoEnCamino: por debajo de la meta pero cerca.
	IngresoEnCamino = "EN_CAMINO"
	// IngresoCorto: lejos de la meta.
	IngresoCorto = "CORTO"
)

// umbralIngresoEnCamino es el piso para no marcar CORTO. Debajo de 85 % de la meta, un mes dejó
// de ser «falta poco» y pasó a ser un problema que alguien tiene que explicar.
const umbralIngresoEnCamino = 85

// MetaIngreso es una línea cargada del presupuesto de ingreso.
type MetaIngreso struct {
	Periodo string `json:"periodo"`
	Monto   string `json:"monto"`
	Nota    string `json:"nota"`
	// ClasificacionID vacío = es el TOTAL del mes. Con valor = la meta de esa línea.
	//
	// Las dos clases NO se suman entre sí: sumarlas contaría el mismo colón dos veces, así que
	// quien totaliza elige una. Mismo criterio que el subpresupuesto por partida (mig 0066).
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Concepto        string `json:"concepto"`
}

// EsTotalDelMes distingue el total del desglose por línea.
func (m MetaIngreso) EsTotalDelMes() bool { return m.ClasificacionID == "" }

// FilaIngreso es una línea del control: meta contra real.
type FilaIngreso struct {
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Concepto        string `json:"concepto"`

	// Meta del rango. Vacía si esa línea no tiene meta propia.
	Meta string `json:"meta"`
	// Real: el ingreso NETO del rango, con la misma definición que el EBITDA del tablero. Una
	// devolución dentro de un concepto de ingreso lo BAJA, no aparece como gasto.
	Real string `json:"real"`
	Movs int    `json:"movs"`

	// Diferencia = real − meta. POSITIVO ES BUENO acá, al revés del gasto.
	Diferencia string `json:"diferencia"`
	// CumplidoPct sobre la meta. Vacío si no hay meta: dividir entre cero no es 0 %.
	CumplidoPct string `json:"cumplido_pct"`
	Estado      string `json:"estado"`
}

// ControlIngreso es el cruce completo de un rango.
type ControlIngreso struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`

	// MetaTotal es la suma de los totales mensuales del rango —no la suma de las líneas—.
	MetaTotal string `json:"meta_total"`
	// MesesConMeta: sobre cuántos meses del rango hay meta declarada. Con 0, el cumplimiento no
	// significa nada y la pantalla no debe pintar un semáforo.
	MesesConMeta int    `json:"meses_con_meta"`
	RealTotal    string `json:"real_total"`
	Diferencia   string `json:"diferencia"`
	CumplidoPct  string `json:"cumplido_pct"`
	Estado       string `json:"estado"`

	// Lineas: el desglose, cuando se cargó. Trae también las que tienen real SIN meta, que es
	// justamente lo que una meta por líneas deja afuera sin avisar.
	Lineas []FilaIngreso `json:"lineas"`
	// SumaDeLineas es lo repartido por línea, y SinRepartir lo que queda del total. NEGATIVO
	// significa que el desglose promete MÁS que el total declarado, que es el error a ver.
	SumaDeLineas string `json:"suma_de_lineas"`
	SinRepartir  string `json:"sin_repartir"`
	// LineasSinMeta: cuántas líneas facturaron sin tener meta propia.
	LineasSinMeta int    `json:"lineas_sin_meta"`
	Aviso         string `json:"aviso,omitempty"`
}

// estadoDeIngreso traduce el cumplimiento a un estado.
//
// SIN META NO ES CERO POR CIENTO. Un mes sin meta declarada no «incumplió»: nadie dijo qué se
// esperaba, y pintarlo en rojo haría que el semáforo mienta sobre el único período que no tiene
// nada que juzgar.
func estadoDeIngreso(meta, real decimal.Decimal) (string, string) {
	if !meta.IsPositive() {
		return IngresoSinMeta, ""
	}
	pct := real.Div(meta).Mul(decimal.NewFromInt(100)).Round(1)
	switch {
	case pct.GreaterThanOrEqual(decimal.NewFromInt(100)):
		return IngresoCumplido, pct.StringFixed(1)
	case pct.GreaterThanOrEqual(decimal.NewFromInt(umbralIngresoEnCamino)):
		return IngresoEnCamino, pct.StringFixed(1)
	default:
		return IngresoCorto, pct.StringFixed(1)
	}
}

// ── El resultado presupuestado ──────────────────────────────────────────────────────────────

// ResultadoPresupuestado cruza las DOS mitades: lo que se espera cobrar contra lo que se autoriza
// gastar, y lo mismo con lo real.
//
// Es lo que no se podía calcular con media meta, y es la pregunta de cualquier comité: «¿vamos a
// cerrar el mes como dijimos?».
type ResultadoPresupuestado struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`

	IngresoMeta string `json:"ingreso_meta"`
	IngresoReal string `json:"ingreso_real"`
	GastoMeta   string `json:"gasto_meta"`
	GastoReal   string `json:"gasto_real"`

	// ResultadoMeta = ingreso meta − gasto meta. ResultadoReal, lo mismo con lo real.
	ResultadoMeta string `json:"resultado_meta"`
	ResultadoReal string `json:"resultado_real"`
	// Desvio = real − meta. Positivo: se cerró mejor de lo planeado.
	Desvio string `json:"desvio"`

	// Completo: hay meta de los DOS lados. Sin eso el resultado presupuestado no existe —y
	// calcularlo con un lado en cero daría un número que se lee como completo—.
	Completo bool   `json:"completo"`
	Aviso    string `json:"aviso,omitempty"`
}

// CalcularResultado arma el cruce desde las cuatro cifras.
func CalcularResultado(ingresoMeta, ingresoReal, gastoMeta, gastoReal decimal.Decimal) ResultadoPresupuestado {
	r := ResultadoPresupuestado{
		IngresoMeta: ingresoMeta.StringFixed(2),
		IngresoReal: ingresoReal.StringFixed(2),
		GastoMeta:   gastoMeta.StringFixed(2),
		GastoReal:   gastoReal.StringFixed(2),
	}
	resMeta := ingresoMeta.Sub(gastoMeta)
	resReal := ingresoReal.Sub(gastoReal)
	r.ResultadoMeta = resMeta.StringFixed(2)
	r.ResultadoReal = resReal.StringFixed(2)
	r.Desvio = resReal.Sub(resMeta).StringFixed(2)
	r.Completo = ingresoMeta.IsPositive() && gastoMeta.IsPositive()

	switch {
	case !ingresoMeta.IsPositive() && !gastoMeta.IsPositive():
		r.Aviso = "No hay meta declarada de ingreso ni de gasto para este rango: lo que se ve es el resultado real, sin nada contra qué compararlo."
	case !ingresoMeta.IsPositive():
		r.Aviso = "Falta la meta de INGRESO: el resultado presupuestado toma el ingreso como cero y por eso sale peor de lo que debería."
	case !gastoMeta.IsPositive():
		r.Aviso = "Falta el presupuesto de GASTO: el resultado presupuestado toma el gasto como cero y por eso sale mejor de lo que debería."
	}
	return r
}
