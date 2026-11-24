package ratios

// LOS RATIOS DEL CICLO DE EFECTIVO: DSO, DPO y el ciclo de conversión.
//
// ── QUÉ CONTESTAN ───────────────────────────────────────────────────────────────────────────
//
//	DSO — cuántos días tardo en cobrar lo que facturo
//	DPO — cuántos días tardo en pagar lo que compro
//	CCC — DSO + DIO − DPO: cuántos días de plata propia financio la operación
//
// Es lo primero que pregunta un banco, un inversionista o un due diligence, y hasta ahora el
// sistema no tenía ni uno solo —teniendo CxC, CxP e Inventario con los datos adentro—.
//
// ── POR QUÉ EL CÁLCULO PRINCIPAL ES POR AGOTAMIENTO Y NO LA FÓRMULA DE LIBRO ─────────────────
//
// La fórmula simple es (cartera ÷ ventas del período) × días. Supone que las ventas se repartieron
// parejo, y acá no: ya medimos meses que van de ₡73 a ₡187 millones. Con esa dispersión, dividir
// entre un promedio da un número que no corresponde a ninguna realidad —si el último mes fue
// grande, el DSO sale corto; si fue chico, sale largo—.
//
// El AGOTAMIENTO (countback) no promedia: va descontando la cartera contra lo facturado del mes
// más reciente, después del anterior, y así hasta agotarla. Lo que queda es «mi cartera equivale
// a tantos días de facturación real». Es el método que usa cualquier tesorería con estacionalidad,
// y acá la hay.
//
// Se publican LOS DOS. La diferencia entre ellos no es ruido: cuando el simple y el countback se
// separan mucho, está diciendo que los últimos meses no se parecen al promedio, y eso es
// información sobre el negocio, no sobre el método.

import (
	"github.com/shopspring/decimal"
)

// MesDeBase es un mes de la ventana con lo que se facturó o compró en él.
//
// El orden importa: el countback los recorre del más reciente al más viejo, porque la cartera que
// hoy está pendiente es, naturalmente, la de lo más nuevo.
type MesDeBase struct {
	Periodo string          // YYYY-MM
	Monto   decimal.Decimal // facturado (DSO) o comprado (DPO) en ese mes
	Dias    int             // días del mes: febrero no son 31
}

// Ratio es un indicador con su cálculo a la vista.
//
// NO ES SOLO UN NÚMERO. Un DSO de 290 días puede significar «cobramos tardísimo» o «los cobros
// todavía no se registran en el sistema», y las dos cosas se ven igual mirando la cifra sola. Por
// eso viaja con su numerador, su denominador y su aviso.
type Ratio struct {
	// Dias es el valor principal: el calculado por agotamiento.
	Dias string `json:"dias"`
	// DiasSimple es la fórmula de libro, para contrastar.
	DiasSimple string `json:"dias_simple"`
	// Cartera es el numerador: lo que está pendiente hoy.
	Cartera string `json:"cartera"`
	// BaseMensual es el promedio de la ventana, que es el denominador del método simple.
	BaseMensual string `json:"base_mensual"`
	// MesesUsados: cuántos meses tuvo que agotar el countback. Si se comió toda la ventana sin
	// terminar, el valor es un PISO y no una medición — y eso se dice en el aviso.
	MesesUsados int `json:"meses_usados"`
	// Calculable: hubo base suficiente. Sin ella, el ratio no se muestra en vez de mostrar cero.
	Calculable bool   `json:"calculable"`
	Aviso      string `json:"aviso,omitempty"`
}

// CicloDeEfectivo son los tres ratios juntos.
type CicloDeEfectivo struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`
	Meses int    `json:"meses"`

	DSO Ratio `json:"dso"`
	DPO Ratio `json:"dpo"`
	DIO Ratio `json:"dio"`

	// Ciclo = DSO + DIO − DPO. Positivo: financiás la operación con plata propia tantos días.
	// Negativo: la financian tus proveedores, que es la posición cómoda.
	Ciclo string `json:"ciclo"`
	// CicloCalculable: solo si DSO y DPO lo son. El DIO entra como cero cuando no hay inventario
	// cargado, y eso se avisa en vez de inventarlo.
	CicloCalculable bool     `json:"ciclo_calculable"`
	Avisos          []string `json:"avisos"`
}

// diasPorAgotamiento calcula a cuántos días de base equivale la cartera.
//
// Recorre los meses del más reciente al más viejo restando. Cuando la cartera se agota a mitad de
// un mes, se prorratea por los días de ESE mes y no por 30: un febrero no son 30 días, y en un
// ratio que se compara contra el trimestre anterior esa diferencia se nota.
//
// Devuelve además cuántos meses hizo falta agotar, porque eso dice si el resultado es una medición
// o un piso.
func diasPorAgotamiento(cartera decimal.Decimal, meses []MesDeBase) (decimal.Decimal, int, bool) {
	if !cartera.IsPositive() {
		// Sin cartera pendiente no se tarda nada en cobrar: cero días es la respuesta correcta,
		// no «no se puede calcular».
		return decimal.Zero, 0, true
	}
	resto := cartera
	dias := decimal.Zero
	usados := 0
	for _, m := range meses {
		if !m.Monto.IsPositive() {
			// Un mes sin facturación no aporta días ni consume cartera. Saltarlo —en vez de
			// contarlo como mes completo— evita inflar el ratio con meses muertos.
			continue
		}
		usados++
		if resto.LessThanOrEqual(m.Monto) {
			// Acá se agota: la fracción de este mes que hace falta, en días de este mes.
			dias = dias.Add(resto.Div(m.Monto).Mul(decimal.NewFromInt(int64(m.Dias))))
			return dias.Round(1), usados, true
		}
		resto = resto.Sub(m.Monto)
		dias = dias.Add(decimal.NewFromInt(int64(m.Dias)))
	}
	// Se acabó la ventana y todavía queda cartera: el número es un PISO —«al menos tantos días»—
	// y el llamador lo dice. Devolver el acumulado como si fuera la medición sería mentir por lo
	// bajo justo en el caso más grave.
	return dias.Round(1), usados, false
}

// diasSimple es la fórmula de libro: cartera ÷ promedio mensual × días promedio del mes.
func diasSimple(cartera decimal.Decimal, meses []MesDeBase) (decimal.Decimal, decimal.Decimal, bool) {
	total := decimal.Zero
	dias := 0
	n := 0
	for _, m := range meses {
		if !m.Monto.IsPositive() {
			continue
		}
		total = total.Add(m.Monto)
		dias += m.Dias
		n++
	}
	if n == 0 || !total.IsPositive() {
		return decimal.Zero, decimal.Zero, false
	}
	promedioMensual := total.Div(decimal.NewFromInt(int64(n)))
	diasPromedio := decimal.NewFromInt(int64(dias)).Div(decimal.NewFromInt(int64(n)))
	return cartera.Div(promedioMensual).Mul(diasPromedio).Round(1), promedioMensual.Round(2), true
}

// CalcularRatio arma un ratio completo desde su cartera y su base.
func CalcularRatio(cartera decimal.Decimal, meses []MesDeBase, queEs string) Ratio {
	simple, base, hayBase := diasSimple(cartera, meses)
	if !hayBase {
		return Ratio{
			Calculable: false,
			Cartera:    cartera.StringFixed(2),
			Aviso:      "No hay " + queEs + " en la ventana, así que no hay contra qué medir la cartera.",
		}
	}
	dias, usados, completo := diasPorAgotamiento(cartera, meses)
	r := Ratio{
		Dias:        dias.StringFixed(1),
		DiasSimple:  simple.StringFixed(1),
		Cartera:     cartera.StringFixed(2),
		BaseMensual: base.StringFixed(2),
		MesesUsados: usados,
		Calculable:  true,
	}
	if !completo {
		r.Aviso = "La cartera es mayor que todo lo registrado en la ventana: el número es un MÍNIMO, no una medición. Ampliá la ventana para medirlo de verdad."
	}
	return r
}

// CalcularCiclo arma los tres y el ciclo.
func CalcularCiclo(dso, dpo, dio Ratio) (string, bool) {
	if !dso.Calculable || !dpo.Calculable {
		return "", false
	}
	suma := decimal.Zero
	for _, par := range []struct {
		valor string
		resta bool
	}{{dso.Dias, false}, {dio.Dias, false}, {dpo.Dias, true}} {
		if par.valor == "" {
			continue
		}
		d, err := decimal.NewFromString(par.valor)
		if err != nil {
			continue
		}
		if par.resta {
			suma = suma.Sub(d)
		} else {
			suma = suma.Add(d)
		}
	}
	return suma.StringFixed(1), true
}
