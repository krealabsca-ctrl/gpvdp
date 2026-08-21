package bancos

// Fase C — Proyecciones (CU-10): matemática PURA de los métodos de proyección.
// Semánticas aprobadas por el Director Financiero con la maqueta (2026-07-16):
//   RITMO       — media diaria de los días con actividad de ESTE mes × días
//                 activos restantes (domingos excluidos). Único método sin histórico;
//                 conserva el run-rate confirmado el 2026-07-06.
//   HISTORICO   — el MISMO MES del año anterior: el acumulado real se escala con
//                 el % de avance que ese mes llevaba al mismo día.
//   PROMEDIO    — acumulado real + promedio de lo que entró en los días restantes
//                 en los meses anteriores del año en curso.
//   COINCIDENCIA— el mes histórico cuya senda normalizada al día actual más se
//                 parece a la del mes en curso; el cierre se proyecta con su forma.
// Solo INGRESOS (créditos sin traslados), por línea de ingreso (spec §5/§17).

import (
	"time"

	"github.com/shopspring/decimal"
)

// DiaMonto es el ingreso de un día del mes (solo días con actividad).
type DiaMonto struct {
	Dia   int
	Monto decimal.Decimal
}

// SendaMes es la senda diaria de ingresos de un mes histórico.
type SendaMes struct {
	Periodo string // YYYY-MM
	Dias    []DiaMonto
}

// Métodos de proyección (los valores viajan por la API y se persisten).
const (
	MetodoRitmo        = "RITMO"
	MetodoHistorico    = "HISTORICO"
	MetodoPromedio     = "PROMEDIO"
	MetodoCoincidencia = "COINCIDENCIA"
)

// acumuladoHasta suma los montos de los días 1..hastaDia.
func acumuladoHasta(dias []DiaMonto, hastaDia int) decimal.Decimal {
	acc := decimal.Zero
	for _, d := range dias {
		if d.Dia <= hastaDia {
			acc = acc.Add(d.Monto)
		}
	}
	return acc
}

// totalMes suma la senda completa.
func totalMes(dias []DiaMonto) decimal.Decimal { return acumuladoHasta(dias, 31) }

// diasActivosRestantes cuenta los días > diaCalculo del mes que no son domingo.
func diasActivosRestantes(anio, mes, diaCalculo, diasMes int) int {
	n := 0
	for d := diaCalculo + 1; d <= diasMes; d++ {
		if time.Date(anio, time.Month(mes), d, 0, 0, 0, 0, time.UTC).Weekday() != time.Sunday {
			n++
		}
	}
	return n
}

// proyectarRitmo: media de los días CON actividad del propio mes × días activos restantes.
func proyectarRitmo(actual []DiaMonto, diaCalculo int, anio, mes, diasMes int) decimal.Decimal {
	realAcum := acumuladoHasta(actual, diaCalculo)
	activos := 0
	for _, d := range actual {
		if d.Dia <= diaCalculo && d.Monto.IsPositive() {
			activos++
		}
	}
	if activos == 0 {
		return realAcum
	}
	media := realAcum.Div(decimal.NewFromInt(int64(activos)))
	restantes := diasActivosRestantes(anio, mes, diaCalculo, diasMes)
	return realAcum.Add(media.Mul(decimal.NewFromInt(int64(restantes))))
}

// proyectarHistorico escala el acumulado real con el avance del mes histórico al mismo día.
// ok=false si el histórico no sirve (sin total o sin avance a ese día).
func proyectarHistorico(realAcum decimal.Decimal, historico []DiaMonto, diaCalculo int) (decimal.Decimal, bool) {
	total := totalMes(historico)
	acum := acumuladoHasta(historico, diaCalculo)
	if !total.IsPositive() || !acum.IsPositive() {
		return decimal.Zero, false
	}
	// cierre = realAcum / (acumHist/totalHist) = realAcum × totalHist / acumHist
	return realAcum.Mul(total).Div(acum), true
}

// proyectarPromedio suma al acumulado real el promedio de lo que entró en los días
// restantes (diaCalculo+1..fin) en los meses dados. ok=false sin meses útiles.
func proyectarPromedio(realAcum decimal.Decimal, meses []SendaMes, diaCalculo int) (decimal.Decimal, bool) {
	suma := decimal.Zero
	n := 0
	for _, m := range meses {
		total := totalMes(m.Dias)
		if !total.IsPositive() {
			continue
		}
		resto := total.Sub(acumuladoHasta(m.Dias, diaCalculo))
		suma = suma.Add(resto)
		n++
	}
	if n == 0 {
		return decimal.Zero, false
	}
	return realAcum.Add(suma.Div(decimal.NewFromInt(int64(n)))), true
}

// proyectarCoincidencia busca el mes "gemelo": el de senda acumulada normalizada
// (día a día, hasta diaCalculo) más parecida a la actual, y proyecta con su forma.
func proyectarCoincidencia(actual []DiaMonto, realAcum decimal.Decimal, meses []SendaMes, diaCalculo int) (decimal.Decimal, string, bool) {
	if !realAcum.IsPositive() {
		return decimal.Zero, "", false
	}
	var mejor *SendaMes
	var mejorDist decimal.Decimal
	for i := range meses {
		m := &meses[i]
		acumM := acumuladoHasta(m.Dias, diaCalculo)
		if !acumM.IsPositive() || !totalMes(m.Dias).IsPositive() {
			continue
		}
		dist := decimal.Zero
		for d := 1; d <= diaCalculo; d++ {
			cur := acumuladoHasta(actual, d).Div(realAcum)
			cand := acumuladoHasta(m.Dias, d).Div(acumM)
			dif := cur.Sub(cand)
			dist = dist.Add(dif.Mul(dif))
		}
		if mejor == nil || dist.LessThan(mejorDist) {
			mejor, mejorDist = m, dist
		}
	}
	if mejor == nil {
		return decimal.Zero, "", false
	}
	acumG := acumuladoHasta(mejor.Dias, diaCalculo)
	cierre := realAcum.Mul(totalMes(mejor.Dias)).Div(acumG)
	return cierre, mejor.Periodo, true
}

// metaCierre calcula la meta: cierre del mes anterior × (1 + pct/100).
// Sin base (mes anterior en cero) la meta queda en cero y el front lo señala.
func metaCierre(totalMesAnterior decimal.Decimal, pct decimal.Decimal) decimal.Decimal {
	if !totalMesAnterior.IsPositive() {
		return decimal.Zero
	}
	cien := decimal.NewFromInt(100)
	return totalMesAnterior.Mul(cien.Add(pct)).Div(cien)
}

// sendaAcumulada convierte la senda diaria en puntos acumulados (para el gráfico).
func sendaAcumulada(dias []DiaMonto, hastaDia int) []PuntoSenda {
	out := make([]PuntoSenda, 0, hastaDia)
	acc := decimal.Zero
	porDia := make(map[int]decimal.Decimal, len(dias))
	for _, d := range dias {
		porDia[d.Dia] = d.Monto
	}
	for d := 1; d <= hastaDia; d++ {
		if m, ok := porDia[d]; ok {
			acc = acc.Add(m)
		}
		out = append(out, PuntoSenda{Dia: d, Acumulado: acc.String()})
	}
	return out
}

// sendaProyectada reparte el remanente proyectado en los días activos restantes
// (solo para DIBUJAR la senda punteada; el cierre ya viene del método).
func sendaProyectada(realAcum, cierre decimal.Decimal, diaCalculo, diasMes, anio, mes int) []PuntoSenda {
	restantes := diasActivosRestantes(anio, mes, diaCalculo, diasMes)
	out := []PuntoSenda{{Dia: diaCalculo, Acumulado: realAcum.String()}}
	if restantes == 0 {
		return out
	}
	paso := cierre.Sub(realAcum).Div(decimal.NewFromInt(int64(restantes)))
	acc := realAcum
	for d := diaCalculo + 1; d <= diasMes; d++ {
		if time.Date(anio, time.Month(mes), d, 0, 0, 0, 0, time.UTC).Weekday() != time.Sunday {
			acc = acc.Add(paso)
		}
		out = append(out, PuntoSenda{Dia: d, Acumulado: acc.String()})
	}
	return out
}
