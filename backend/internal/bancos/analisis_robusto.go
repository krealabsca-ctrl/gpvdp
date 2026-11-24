package bancos

// Tanda 2 del análisis: mediana, recurrencia y estacionalidad.
//
// ── QUÉ PROBLEMA RESUELVEN, MEDIDO ──────────────────────────────────────────────────────────
//
// El promedio móvil contra el que se juzga cada partida tiene tres puntos ciegos, y los tres se
// midieron sobre Memorial Pets (17 meses comparables, 28-oct-2026):
//
//  1. UN PAGO GRANDE LO INFLA PARA SIEMPRE. En varias partidas el promedio está 90 % a 126 % por
//     encima de la mediana —Synergia +126 %, Mantenimiento de Carrozas +100 %, Leasing +93 %—.
//     Después de ese mes, todos los meses normales aparecen «muy por debajo del promedio» y el
//     desvío marca una caída que no existe.
//
//  2. MEZCLA EL GASTO FIJO CON EL QUE PASA UNA VEZ. De 96 partidas de gasto, 8 aparecen casi
//     todos los meses y son el 47 % del dinero; 72 son esporádicas y apenas el 19,6 %. Para esas
//     72, «promedio mensual» no significa nada: no hay un ritmo del que apartarse.
//
//  3. NO SABE QUE DICIEMBRE SIEMPRE ES ALTO. Julio promedia un 41 % más que un mes cualquiera.
//     Sin índice estacional, ese 41 % se lee como anomalía todos los años.
//
// ── LO QUE ESTO NO HACE ─────────────────────────────────────────────────────────────────────
//
// No reemplaza el promedio: lo acompaña. Las dos bases contestan preguntas distintas y la
// pantalla deja elegir, igual que el umbral.

import (
	"sort"

	"github.com/shopspring/decimal"
)

// Grados de recurrencia de una partida.
const (
	// RecurrenciaFija: aparece en casi todos los meses. Es el gasto del que sí hay un ritmo.
	RecurrenciaFija = "FIJO"
	// RecurrenciaIntermitente: aparece seguido pero no siempre.
	RecurrenciaIntermitente = "INTERMITENTE"
	// RecurrenciaEsporadica: aparece pocas veces. Su «promedio mensual» no significa nada.
	RecurrenciaEsporadica = "ESPORADICO"
)

// Umbrales de recurrencia, como fracción de los meses comparables del rango.
//
// 0,8 y 0,4 salen de mirar la forma real del catálogo: con 17 meses, «fijo» es aparecer en 14 o
// más —lo que deja fuera a una partida mensual que se saltó dos meses por calendario de pago— y
// «esporádico» es aparecer en menos de 7, donde ya no hay serie que promediar.
const (
	fraccionFija         = 0.8
	fraccionIntermitente = 0.4
)

// mediana devuelve el valor central de una lista de montos. Con dos valores centrales promedia
// los dos, que es la definición y evita que la mediana salte según la paridad del conteo.
func mediana(montos []decimal.Decimal) decimal.Decimal {
	if len(montos) == 0 {
		return decimal.Zero
	}
	orden := make([]decimal.Decimal, len(montos))
	copy(orden, montos)
	sort.Slice(orden, func(i, j int) bool { return orden[i].LessThan(orden[j]) })
	n := len(orden)
	if n%2 == 1 {
		return orden[n/2]
	}
	return orden[n/2-1].Add(orden[n/2]).Div(decimal.NewFromInt(2))
}

// recurrenciaDe clasifica una partida por en cuántos meses tuvo movimiento.
//
// `conGasto` cuenta los meses con monto distinto de cero, no los meses del rango: una partida que
// existe desde hace un año pero solo se paga en diciembre tiene un mes con gasto, no doce.
func recurrenciaDe(conGasto, mesesComparables int) string {
	if mesesComparables == 0 {
		return RecurrenciaEsporadica
	}
	f := float64(conGasto) / float64(mesesComparables)
	switch {
	case f >= fraccionFija:
		return RecurrenciaFija
	case f >= fraccionIntermitente:
		return RecurrenciaIntermitente
	default:
		return RecurrenciaEsporadica
	}
}

// MesEstacional es el peso de un mes del calendario frente a un mes cualquiera.
type MesEstacional struct {
	// Mes va de 1 a 12.
	Mes int `json:"mes"`
	// Anios: sobre cuántas observaciones se calculó. CON UNA SOLA NO HAY ESTACIONALIDAD: es el
	// dato de ese mes, y presentarlo como patrón sería inventar. Medido en Memorial el
	// 28-oct-2026: solo 5 de los 12 meses tienen dos años cargados.
	Anios int `json:"anios"`
	// Indice: 100 = un mes promedio. 141 = gasta un 41 % más que un mes cualquiera.
	Indice string `json:"indice"`
	// Promedio del mes, en colones.
	Promedio string `json:"promedio"`
	// Confiable es false con menos de dos años: la pantalla lo muestra en gris y no opina.
	Confiable bool `json:"confiable"`
}

// Estacionalidad arma el índice por mes del calendario a partir de la salud y las series.
//
// SE CALCULA SOBRE EL RANGO PEDIDO, no sobre todo el histórico: el usuario eligió qué mirar y el
// sistema contesta sobre eso. La consecuencia es que con un trimestre no hay estacionalidad que
// mostrar, y es correcto que no la haya —tres meses no son un patrón anual—.
func Estacionalidad(meses []SaludMes, partidas []TendenciaPartida) ([]MesEstacional, error) {
	// La comparabilidad se EVALÚA acá en vez de leer el campo: así la función vale aunque la
	// llamen con la salud tal como vino del repositorio, donde `Comparable` todavía es false.
	comparable := map[string]bool{}
	for _, m := range meses {
		if m.Comparable || esComparable(m) {
			comparable[m.Periodo] = true
		}
	}

	// Total por período, sumando todas las partidas que llegaron (ya vienen filtradas por quien
	// llama: si pidió solo gastos, la estacionalidad es la del gasto).
	porPeriodo := map[string]decimal.Decimal{}
	for _, p := range partidas {
		for _, pt := range p.Serie {
			if !comparable[pt.Periodo] {
				continue
			}
			m, err := decimal.NewFromString(pt.Monto)
			if err != nil {
				return nil, err
			}
			porPeriodo[pt.Periodo] = porPeriodo[pt.Periodo].Add(m)
		}
	}
	if len(porPeriodo) == 0 {
		return nil, nil
	}

	// Agrupar por mes del calendario.
	type acum struct {
		suma decimal.Decimal
		n    int
	}
	porMes := map[int]*acum{}
	granTotal := decimal.Zero
	nPeriodos := 0
	for periodo, monto := range porPeriodo {
		m := mesDelPeriodo(periodo)
		if m == 0 {
			continue
		}
		if porMes[m] == nil {
			porMes[m] = &acum{suma: decimal.Zero}
		}
		porMes[m].suma = porMes[m].suma.Add(monto)
		porMes[m].n++
		granTotal = granTotal.Add(monto)
		nPeriodos++
	}
	if nPeriodos == 0 {
		return nil, nil
	}
	mesTipico := granTotal.Div(decimal.NewFromInt(int64(nPeriodos)))

	out := make([]MesEstacional, 0, len(porMes))
	for m, a := range porMes {
		prom := a.suma.Div(decimal.NewFromInt(int64(a.n)))
		e := MesEstacional{
			Mes: m, Anios: a.n,
			Promedio: prom.StringFixed(2),
			// Con dos años o más el índice es un patrón; con uno es el dato de ese mes.
			Confiable: a.n >= 2,
			Indice:    "100.0",
		}
		if mesTipico.IsPositive() {
			e.Indice = prom.Div(mesTipico).Mul(decimal.NewFromInt(100)).StringFixed(1)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mes < out[j].Mes })
	return out, nil
}

// mesDelPeriodo saca el número de mes de «YYYY-MM». Devuelve 0 si no se puede leer.
func mesDelPeriodo(periodo string) int {
	if len(periodo) != 7 || periodo[4] != '-' {
		return 0
	}
	m := int(periodo[5]-'0')*10 + int(periodo[6]-'0')
	if m < 1 || m > 12 {
		return 0
	}
	return m
}
