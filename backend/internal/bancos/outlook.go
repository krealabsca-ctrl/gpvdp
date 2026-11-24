package bancos

// OUTLOOK DEL AÑO: la tercera columna que faltaba.
//
// ── QUÉ CONTESTA ────────────────────────────────────────────────────────────────────────────
//
// «¿Cómo vamos a cerrar el año?» — la pregunta de toda junta directiva, y la que el sistema no
// podía contestar. Había dos columnas (Presupuesto y Real) y el estándar son TRES:
//
//	PRESUPUESTO — lo que se declaró, congelado
//	REAL        — lo que pasó hasta hoy
//	OUTLOOK     — lo real más lo que se espera del resto del año
//
// Con dos columnas solo se sabe dónde se está; con la tercera, dónde se va a terminar.
//
// ── POR QUÉ DOS ESCENARIOS Y NO UNO ─────────────────────────────────────────────────────────
//
// Porque «lo que se espera del resto del año» admite dos respuestas legítimas y opuestas:
//
//	PLAN  — los meses que faltan salen como se presupuestaron
//	RITMO — los meses que faltan salen como vienen saliendo
//
// Publicar uno solo obliga a elegir por el usuario. Publicar los dos convierte la BRECHA entre
// ellos en el dato: si el plan promete mucho más que el ritmo, el plan ya no describe el año y
// alguien tiene que decidir si se corrige la meta o la operación.
//
// ── EL MES EN CURSO SE PROYECTA A PARIDAD DE DÍA, EN LOS DOS ────────────────────────────────
//
// Es la trampa que ya mordió en el Dashboard: un mes a medio transcurrir metido en un promedio
// lo hunde, y el número mide los días que pasaron en vez del desempeño. Acá el mes en curso se
// lleva a mes completo por regla de tres sobre los días, y los escenarios solo difieren en los
// meses FUTUROS. Así la única variable entre ellos es la que se está comparando.

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// MesDelAnio es lo que pasó (o se espera) en un mes del año.
type MesDelAnio struct {
	Periodo string `json:"periodo"`

	IngresoReal string `json:"ingreso_real"`
	GastoReal   string `json:"gasto_real"`
	IngresoMeta string `json:"ingreso_meta"`
	GastoMeta   string `json:"gasto_meta"`

	// Estado: CERRADO (pasó entero), EN_CURSO (es el mes de hoy), FUTURO.
	Estado string `json:"estado"`
	// Proyectado: para el mes EN CURSO, lo que daría el mes completo a paridad de día.
	IngresoProyectado string `json:"ingreso_proyectado,omitempty"`
	GastoProyectado   string `json:"gasto_proyectado,omitempty"`
}

// Estados de un mes dentro del año.
const (
	MesCerrado = "CERRADO"
	MesEnCurso = "EN_CURSO"
	MesFuturo  = "FUTURO"
)

// Escenario es una forma de completar el año.
type Escenario struct {
	// Nombre: PLAN o RITMO.
	Nombre string `json:"nombre"`

	Ingreso   string `json:"ingreso"`
	Gasto     string `json:"gasto"`
	Resultado string `json:"resultado"`

	// Base dice de dónde salió lo proyectado, para que el número se pueda discutir.
	Base string `json:"base"`
	// Calculable: hubo con qué proyectar. Sin meta no hay PLAN; sin meses cerrados no hay RITMO.
	Calculable bool   `json:"calculable"`
	Aviso      string `json:"aviso,omitempty"`
}

// Nombres de los escenarios.
const (
	EscenarioPlan  = "PLAN"
	EscenarioRitmo = "RITMO"
)

// OutlookAnual es el año entero: lo que pasó, lo que se presupuestó y cómo se cierra.
type OutlookAnual struct {
	Anio int `json:"anio"`

	Meses []MesDelAnio `json:"meses"`

	// Lo acumulado hasta hoy (incluye el mes en curso tal como va, sin proyectar).
	IngresoYTD   string `json:"ingreso_ytd"`
	GastoYTD     string `json:"gasto_ytd"`
	ResultadoYTD string `json:"resultado_ytd"`

	// El presupuesto del año COMPLETO, como se declaró.
	IngresoPresupuesto   string `json:"ingreso_presupuesto"`
	GastoPresupuesto     string `json:"gasto_presupuesto"`
	ResultadoPresupuesto string `json:"resultado_presupuesto"`

	Plan  Escenario `json:"plan"`
	Ritmo Escenario `json:"ritmo"`

	// MesesCerrados y MesesFuturos: cuánto del año es hecho y cuánto es proyección. Un outlook
	// con once meses por delante es casi todo supuesto, y eso tiene que verse.
	MesesCerrados int `json:"meses_cerrados"`
	// MesesCerradosSinDatos: cerraron sin un solo movimiento. NO entran en el promedio del ritmo
	// —un mes sin cargar no es un mes de cero— y se avisan, porque casi siempre son un estado de
	// cuenta que falta importar.
	MesesCerradosSinDatos int    `json:"meses_cerrados_sin_datos"`
	MesesFuturos          int    `json:"meses_futuros"`
	MesEnCurso            string `json:"mes_en_curso"`
	// AvanceDelMes: qué proporción del mes en curso transcurrió, para explicar la proyección.
	AvanceDelMes string `json:"avance_del_mes"`

	Avisos []string `json:"avisos"`
}

// diasDelMes devuelve cuántos días tiene el mes de esa fecha.
func diasDelMes(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// aParidadDeMes lleva lo que va del mes a mes completo por regla de tres sobre los días.
//
// NO ES UNA PREDICCIÓN FINA: supone que el resto del mes se parece a lo que va. Es exactamente lo
// que hace falta para que el mes en curso no hunda un promedio, y es mejor que las dos
// alternativas —ignorarlo (pierde lo que ya pasó) o contarlo entero (dice que el mes fue malo
// cuando apenas empezó)—.
func aParidadDeMes(parcial decimal.Decimal, diaDeHoy, diasMes int) decimal.Decimal {
	if diaDeHoy <= 0 || diasMes <= 0 {
		return parcial
	}
	if diaDeHoy >= diasMes {
		return parcial
	}
	return parcial.Mul(decimal.NewFromInt(int64(diasMes))).
		Div(decimal.NewFromInt(int64(diaDeHoy))).Round(2)
}

// ConstruirOutlook arma el año completo. Es PURA: recibe todo medido.
//
// `reales` y `metas` van por período YYYY-MM. `hoy` fija qué meses son cerrados, cuál está en
// curso y cuáles faltan.
func ConstruirOutlook(hoy time.Time, anio int, reales, metas map[string][2]decimal.Decimal) OutlookAnual {
	o := OutlookAnual{Anio: anio}

	mesActual := 0
	if hoy.Year() == anio {
		mesActual = int(hoy.Month())
	} else if hoy.Year() > anio {
		// Un año ya pasado: todos sus meses están cerrados.
		mesActual = 13
	}

	diaDeHoy, diasMes := hoy.Day(), diasDelMes(hoy)

	var ingYTD, gasYTD, ingPresup, gasPresup decimal.Decimal
	// Lo cerrado sirve de base del RITMO; el mes en curso NO entra ahí, que es todo el punto.
	var ingCerrado, gasCerrado decimal.Decimal
	// mesesConActividad son los cerrados que de verdad tuvieron movimiento: la base del ritmo.
	mesesConActividad := 0
	var ingEnCursoProy, gasEnCursoProy decimal.Decimal
	var ingMetaFutura, gasMetaFutura decimal.Decimal

	for m := 1; m <= 12; m++ {
		periodo := periodoDe(anio, m)
		real := reales[periodo]
		meta := metas[periodo]

		fila := MesDelAnio{
			Periodo:     periodo,
			IngresoReal: real[0].StringFixed(2),
			GastoReal:   real[1].StringFixed(2),
			IngresoMeta: meta[0].StringFixed(2),
			GastoMeta:   meta[1].StringFixed(2),
		}

		ingPresup = ingPresup.Add(meta[0])
		gasPresup = gasPresup.Add(meta[1])

		switch {
		case m < mesActual:
			fila.Estado = MesCerrado
			o.MesesCerrados++
			ingCerrado = ingCerrado.Add(real[0])
			gasCerrado = gasCerrado.Add(real[1])
			ingYTD = ingYTD.Add(real[0])
			gasYTD = gasYTD.Add(real[1])
			// UN MES CERRADO SIN MOVIMIENTO NO ES UN MES DE CERO: es un mes que no se cargó.
			// Contarlo en el promedio del ritmo lo hunde en proporción directa —con uno de diez,
			// el año proyectado sale un 10 % abajo— y el número se ve perfectamente plausible.
			// Se cuentan aparte los que SÍ tienen actividad, y el aviso nombra los otros.
			if real[0].IsZero() && real[1].IsZero() {
				o.MesesCerradosSinDatos++
			} else {
				mesesConActividad++
			}
		case m == mesActual:
			fila.Estado = MesEnCurso
			o.MesEnCurso = periodo
			ingYTD = ingYTD.Add(real[0])
			gasYTD = gasYTD.Add(real[1])
			ingEnCursoProy = aParidadDeMes(real[0], diaDeHoy, diasMes)
			gasEnCursoProy = aParidadDeMes(real[1], diaDeHoy, diasMes)
			fila.IngresoProyectado = ingEnCursoProy.StringFixed(2)
			fila.GastoProyectado = gasEnCursoProy.StringFixed(2)
		default:
			fila.Estado = MesFuturo
			o.MesesFuturos++
			ingMetaFutura = ingMetaFutura.Add(meta[0])
			gasMetaFutura = gasMetaFutura.Add(meta[1])
		}
		o.Meses = append(o.Meses, fila)
	}

	o.IngresoYTD, o.GastoYTD = ingYTD.StringFixed(2), gasYTD.StringFixed(2)
	o.ResultadoYTD = ingYTD.Sub(gasYTD).StringFixed(2)
	o.IngresoPresupuesto, o.GastoPresupuesto = ingPresup.StringFixed(2), gasPresup.StringFixed(2)
	o.ResultadoPresupuesto = ingPresup.Sub(gasPresup).StringFixed(2)
	if mesActual >= 1 && mesActual <= 12 {
		o.AvanceDelMes = decimal.NewFromInt(int64(diaDeHoy)).
			Mul(decimal.NewFromInt(100)).
			Div(decimal.NewFromInt(int64(diasMes))).StringFixed(0) + " %"
	}

	// La parte HECHA del año es igual en los dos escenarios: lo cerrado más el mes en curso
	// llevado a mes completo. Lo que cambia es cómo se completan los meses que faltan.
	baseIng := ingCerrado.Add(ingEnCursoProy)
	baseGas := gasCerrado.Add(gasEnCursoProy)

	o.Plan = armarEscenario(EscenarioPlan, baseIng.Add(ingMetaFutura), baseGas.Add(gasMetaFutura),
		"los meses que faltan salen como se presupuestaron",
		ingMetaFutura.IsPositive() || gasMetaFutura.IsPositive() || o.MesesFuturos == 0,
		"No hay meta declarada para los meses que faltan, así que este escenario los toma en cero: el año cierra artificialmente bajo.")

	// El RITMO necesita meses CERRADOS CON MOVIMIENTO. Dos razones distintas:
	//
	//  · El mes en curso no entra: el promedio sería el de un mes a medio transcurrir.
	//  · Un mes cerrado SIN datos tampoco: dividir entre él hunde el ritmo en proporción directa
	//    —uno de diez lo baja un 10 %— y el año proyectado sale plausible pero bajo. Un mes sin
	//    movimiento casi nunca es un mes sin operación; es un estado de cuenta sin importar.
	ritmoIng, ritmoGas := decimal.Zero, decimal.Zero
	if mesesConActividad > 0 {
		n := decimal.NewFromInt(int64(mesesConActividad))
		f := decimal.NewFromInt(int64(o.MesesFuturos))
		ritmoIng = ingCerrado.Div(n).Mul(f)
		ritmoGas = gasCerrado.Div(n).Mul(f)
	}
	o.Ritmo = armarEscenario(EscenarioRitmo, baseIng.Add(ritmoIng), baseGas.Add(ritmoGas),
		"los meses que faltan salen como el promedio de los cerrados con movimiento",
		mesesConActividad > 0,
		"No hay ningún mes cerrado CON MOVIMIENTO en el año, así que no hay un ritmo del cual proyectar.")

	o.Avisos = avisosDelOutlook(o)
	return o
}

func armarEscenario(nombre string, ing, gas decimal.Decimal, base string, ok bool, aviso string) Escenario {
	e := Escenario{
		Nombre: nombre, Base: base, Calculable: ok,
		Ingreso:   ing.StringFixed(2),
		Gasto:     gas.StringFixed(2),
		Resultado: ing.Sub(gas).StringFixed(2),
	}
	if !ok {
		e.Aviso = aviso
	}
	return e
}

// avisosDelOutlook dice lo que las cifras no muestran.
func avisosDelOutlook(o OutlookAnual) []string {
	var av []string

	// EL AVISO QUE MÁS IMPORTA: cuánto del año es proyección. Un outlook armado en febrero es
	// casi todo supuesto, y el número se ve igual de firme que uno armado en noviembre.
	if o.MesesFuturos >= 6 {
		av = append(av, "Más de medio año está por venir: este cierre es sobre todo proyección, no medición.")
	}
	if o.MesesCerrados == 0 {
		av = append(av, "No hay ningún mes cerrado todavía: el año se proyecta desde el mes en curso, que apenas empezó.")
	}
	// Un mes cerrado sin un solo movimiento casi siempre es un estado de cuenta sin importar, no
	// un mes sin operación. Queda fuera del ritmo —si entrara, lo hundiría— pero hay que decirlo:
	// mientras falte, tanto el acumulado del año como el cierre proyectado están cortos.
	if o.MesesCerradosSinDatos > 0 {
		av = append(av, fmt.Sprintf(
			"%d mes(es) ya cerrados no tienen ningún movimiento cargado. No entran en el ritmo —si entraran lo hundirían—, "+
				"pero mientras falten, lo que va del año y el cierre proyectado están cortos.",
			o.MesesCerradosSinDatos))
	}
	if o.Plan.Aviso != "" {
		av = append(av, "Plan: "+o.Plan.Aviso)
	}
	if o.Ritmo.Aviso != "" {
		av = append(av, "Ritmo: "+o.Ritmo.Aviso)
	}
	// La brecha entre los dos escenarios es el dato, y conviene nombrarla.
	if o.Plan.Calculable && o.Ritmo.Calculable {
		plan, err1 := decimal.NewFromString(o.Plan.Resultado)
		ritmo, err2 := decimal.NewFromString(o.Ritmo.Resultado)
		if err1 == nil && err2 == nil && !plan.Equal(ritmo) {
			if ritmo.LessThan(plan) {
				av = append(av, "Al ritmo actual el año cierra POR DEBAJO del plan: o se corrige la operación, o la meta ya no describe el año.")
			} else {
				av = append(av, "Al ritmo actual el año cierra POR ENCIMA del plan: la meta quedó corta.")
			}
		}
	}
	// El mes en curso va proyectado a mes completo, y eso hay que decirlo: es el único renglón
	// del año que no es ni hecho ni presupuesto.
	if o.MesEnCurso != "" {
		av = append(av, "El mes en curso ("+o.MesEnCurso+") va llevado a mes completo con lo que lleva: transcurrió el "+
			o.AvanceDelMes+" y el resto se estima a ese mismo ritmo.")
	}
	return av
}

// periodoDe arma «YYYY-MM».
func periodoDe(anio, mes int) string {
	return time.Date(anio, time.Month(mes), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
}
