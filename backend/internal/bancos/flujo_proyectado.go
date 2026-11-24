package bancos

// Flujo de caja proyectado: cuánto se espera que entre y salga en los próximos meses.
//
// ── POR QUÉ NO SE PROYECTA POR VENCIMIENTOS ─────────────────────────────────────────────────
//
// La forma habitual de proyectar egresos es sumar lo que vence cada mes. Acá no sirve, y no es
// una opinión: medido el 28-oct-2026 sobre la cartera real, de 4.542 facturas pendientes
// **4.536 ya están vencidas (₡940 millones) y solo 2 vencen a futuro**. Una proyección por
// vencimiento pondría ₡940 M en «este mes» y casi nada después: no proyecta, repite que la
// cartera está vencida —que ya lo dice el tablero de CxP—.
//
// Se proyecta entonces por PATRÓN: lo que cada partida suele mover, corregido por el mes del
// calendario. Es la única base que estos datos sostienen.
//
// ── POR QUÉ LA MEDIANA Y NO EL PROMEDIO ─────────────────────────────────────────────────────
//
// Porque un pago extraordinario dentro del histórico inflaría todos los meses proyectados.
// Medido: el promedio está hasta un 126 % por encima de la mediana en algunas partidas.
//
// ── LO QUE ESTO NO ES ───────────────────────────────────────────────────────────────────────
//
// No es un compromiso ni un presupuesto: es lo que la historia sugiere. Y NO DA UN SALDO, porque
// no hay saldo bancario capturado del cual partir —`saldo_cuenta_diario` está vacío—: da el
// movimiento esperado. El día que se capturen saldos, encadenarlos es sumar.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// MesProyectado es lo que se espera de un mes que todavía no ocurrió.
type MesProyectado struct {
	Periodo  string `json:"periodo"`
	Ingresos string `json:"ingresos"`
	Gastos   string `json:"gastos"`
	Neto     string `json:"neto"`
	// IndiceEstacional aplicado, o vacío si el mes no tenía respaldo para corregir.
	IndiceEstacional string `json:"indice_estacional,omitempty"`
}

// FlujoProyectado es la respuesta completa.
type FlujoProyectado struct {
	// DesdeBase/HastaBase: el rango histórico del que salió el patrón.
	DesdeBase string          `json:"desde_base"`
	HastaBase string          `json:"hasta_base"`
	Meses     []MesProyectado `json:"meses"`

	// ── Lo que sostiene la proyección, para que se pueda juzgar ────────────────────────────
	//
	// PartidasProyectadas son las que tienen un ritmo del que proyectar (fijas e intermitentes).
	// LasEsporadicas no se proyectan una por una —no hay patrón— pero su gasto no desaparece:
	// entra como un agregado mensual, y se dice cuánto es.
	PartidasProyectadas int    `json:"partidas_proyectadas"`
	PartidasEsporadicas int    `json:"partidas_esporadicas"`
	AporteEsporadicas   string `json:"aporte_esporadicas"`
	MesesComparables    int    `json:"meses_comparables"`
	ConEstacionalidad   bool   `json:"con_estacionalidad"`

	// Pendiente es lo que YA se debe y no es un flujo futuro: se informa aparte para no
	// mezclarlo con la proyección. Mezclarlo haría que el mes que viene pareciera catastrófico
	// por una deuda que se arrastra desde hace años.
	CxPVencido   string `json:"cxp_vencido"`
	CxPPorVencer string `json:"cxp_por_vencer"`

	// Aviso dice en una frase qué tan firme es esto.
	Aviso string `json:"aviso"`
}

// maxMesesProyeccion acota el horizonte. Más allá de un año, un patrón de 17 meses no sostiene
// nada: sería dibujar una curva bonita sobre una suposición.
const maxMesesProyeccion = 12

// ProyectarFlujo estima el movimiento de los próximos `meses` a partir del patrón histórico.
func (s *Service) ProyectarFlujo(ctx context.Context, empresaID, desdeBase, hastaBase string, meses int) (FlujoProyectado, error) {
	if meses <= 0 || meses > maxMesesProyeccion {
		meses = 6
	}
	base, err := s.AnalisisPartidas(ctx, empresaID, desdeBase, hastaBase)
	if err != nil {
		return FlujoProyectado{}, err
	}

	out := FlujoProyectado{
		DesdeBase: desdeBase, HastaBase: hastaBase,
		MesesComparables: base.MesesComparables,
		Meses:            make([]MesProyectado, 0, meses),
	}

	// La cartera se consulta SIEMPRE, antes de decidir si se puede proyectar. No depende del
	// histórico —es lo que ya se debe hoy— y es justamente cuando NO se puede proyectar cuando
	// más importa verla: la pantalla quedaría en blanco sobre una empresa que debe ₡936,9 M.
	vencido, porVencer, err := s.repo.CarteraPendiente(ctx, empresaID)
	if err != nil {
		return FlujoProyectado{}, err
	}
	out.CxPVencido, out.CxPPorVencer = vencido.StringFixed(2), porVencer.StringFixed(2)

	if base.MesesComparables < 3 {
		out.Aviso = fmt.Sprintf(
			"Solo %d mes(es) del histórico sirven para proyectar. Con menos de tres, lo que salga es una suposición y no un patrón.",
			base.MesesComparables)
		return out, nil
	}

	// El índice estacional por mes del calendario, solo donde hay respaldo.
	indice := map[int]decimal.Decimal{}
	for _, e := range base.Estacionalidad {
		if !e.Confiable {
			continue
		}
		v, err := decimal.NewFromString(e.Indice)
		if err != nil {
			return FlujoProyectado{}, err
		}
		indice[e.Mes] = v.Div(decimal.NewFromInt(100))
		out.ConEstacionalidad = true
	}

	// El ritmo de cada partida: la MEDIANA de sus meses, que no se mueve por un pago grande.
	ingresoMes, gastoMes := decimal.Zero, decimal.Zero
	esporadicasMes := decimal.Zero
	for _, p := range base.Partidas {
		ritmo, err := ritmoMensual(p, base.MesesComparables)
		if err != nil {
			return FlujoProyectado{}, err
		}
		if !ritmo.IsPositive() {
			continue
		}
		esIngreso := p.NaturalezaDeclarada && p.Naturaleza == "INGRESO"
		esGasto := p.NaturalezaDeclarada && p.Naturaleza == "GASTO"
		if !esIngreso && !esGasto {
			// Lo que no está declarado como ingreso o gasto no entra al flujo: no se sabe de qué
			// lado sumarlo, y elegir uno sería inventar el resultado.
			continue
		}

		if p.Recurrencia == RecurrenciaEsporadica {
			out.PartidasEsporadicas++
			esporadicasMes = esporadicasMes.Add(ritmo)
			// Las esporádicas igual pesan en el total: lo que no se puede predecir una por una
			// se puede estimar en bloque, y esconderlo dejaría la proyección corta.
			if esGasto {
				gastoMes = gastoMes.Add(ritmo)
			} else {
				ingresoMes = ingresoMes.Add(ritmo)
			}
			continue
		}
		out.PartidasProyectadas++
		if esGasto {
			gastoMes = gastoMes.Add(ritmo)
		} else {
			ingresoMes = ingresoMes.Add(ritmo)
		}
	}
	out.AporteEsporadicas = esporadicasMes.StringFixed(2)

	for i := 1; i <= meses; i++ {
		periodo := sumarMeses(hastaBase, i)
		factor := decimal.NewFromInt(1)
		m := MesDelPeriodo(periodo)
		if f, hay := indice[m]; hay {
			factor = f
		}
		ing := ingresoMes.Mul(factor)
		gas := gastoMes.Mul(factor)
		mp := MesProyectado{
			Periodo:  periodo,
			Ingresos: ing.StringFixed(2),
			Gastos:   gas.StringFixed(2),
			Neto:     ing.Sub(gas).StringFixed(2),
		}
		if !factor.Equal(decimal.NewFromInt(1)) {
			mp.IndiceEstacional = factor.Mul(decimal.NewFromInt(100)).StringFixed(1)
		}
		out.Meses = append(out.Meses, mp)
	}

	out.Aviso = avisoDeProyeccion(out)
	return out, nil
}

// ritmoMensual es cuánto mueve una partida en un mes típico.
//
// Usa la MEDIANA de sus meses, repartida sobre los meses del rango: una partida que gasta
// ₡600.000 cada dos meses mueve ₡300.000 por mes, y proyectar ₡600.000 todos los meses la
// duplicaría. Para las fijas, que aparecen todos los meses, el reparto no cambia nada.
func ritmoMensual(p TendenciaPartida, mesesComparables int) (decimal.Decimal, error) {
	if mesesComparables == 0 || p.MesesConGasto == 0 {
		return decimal.Zero, nil
	}
	med, err := decimal.NewFromString(p.Mediana)
	if err != nil {
		return decimal.Zero, fmt.Errorf("bancos: mediana de %s: %w", p.Clasificacion, err)
	}
	// La mediana se calcula sobre los meses anteriores comparables, que incluyen los ceros de
	// los meses sin gasto. Para una partida que casi no aparece, esa mediana es cero y el
	// `ritmo` quedaría en nada: en ese caso se reparte el total sobre el rango, que es la mejor
	// estimación disponible.
	if !med.IsPositive() {
		total, err := decimal.NewFromString(p.Total)
		if err != nil {
			return decimal.Zero, fmt.Errorf("bancos: total de %s: %w", p.Clasificacion, err)
		}
		if !total.IsPositive() {
			return decimal.Zero, nil
		}
		return total.Div(decimal.NewFromInt(int64(mesesComparables))), nil
	}
	return med, nil
}

// avisoDeProyeccion arma la frase que dice qué tan firme es esto.
func avisoDeProyeccion(f FlujoProyectado) string {
	msg := fmt.Sprintf(
		"Estimado a partir de %d mes(es) de historia: lo que cada partida suele mover, no lo que está comprometido.",
		f.MesesComparables)
	if !f.ConEstacionalidad {
		msg += " Todavía no hay ningún mes con dos años cargados, así que no se corrige por estacionalidad: todos los meses salen iguales."
	}
	if f.PartidasEsporadicas > 0 {
		msg += fmt.Sprintf(
			" %d partida(s) son esporádicas y no tienen un ritmo propio; entran como un bloque de %s por mes.",
			f.PartidasEsporadicas, formatoMiles(montoNum(f.AporteEsporadicas)))
	}
	return msg
}

// MesDelPeriodo saca el número de mes de «YYYY-MM». Exportada porque la usan el flujo y la
// estacionalidad.
func MesDelPeriodo(periodo string) int { return mesDelPeriodo(periodo) }

// sumarMeses corre un período «YYYY-MM» hacia adelante, sin pasar por time (que arrastra zona).
func sumarMeses(periodo string, n int) string {
	anio, mes := partesDePeriodo(periodo)
	total := anio*12 + (mes - 1) + n
	return fmt.Sprintf("%04d-%02d", total/12, total%12+1)
}
