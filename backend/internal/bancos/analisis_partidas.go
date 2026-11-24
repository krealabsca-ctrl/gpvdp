package bancos

// Análisis de partidas a lo largo del tiempo: la tendencia de cada gasto y qué se salió de su cauce.
//
// Responde una pregunta que el sistema no podía contestar (pedido del usuario, 2026-08-17):
// «¿esta partida se está descontrolando, o es normal?». Existía el desglose por partida de UN mes
// (el Cuadre) y la serie de 12 meses de TOTALES (el Dashboard); nadie había cruzado las dos.
//
// El criterio de anomalía es el que decidió el usuario: **cada partida contra su propio promedio**
// de los meses anteriores. No necesita presupuesto —que no existe en el sistema— y es el mismo
// criterio que ya usa CxP para el desvío por proveedor, así que el ERP mide la rareza de una sola
// manera.
//
// GUARDARRAÍL DE DATOS: cada mes reporta su porcentaje de clasificación. Un mes a medio clasificar
// muestra menos gasto del que tuvo, y comparar contra él inventa anomalías que no existen. Medido
// al construir esto: julio 2026 estaba al 30 % y aparentaba SEIS VECES menos gasto que agosto, que
// estaba al 100 %. Sin el aviso, la primera conclusión de la pantalla habría sido «el gasto se
// multiplicó por 6» — falso, y con toda la autoridad de un número.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// PuntoPartida es el monto de una partida en un mes.
type PuntoPartida struct {
	Periodo string `json:"periodo"`
	Monto   string `json:"monto"`
	Movs    int    `json:"movs"`
}

// SaludMes dice si un mes es comparable. Sin esto, un mes a medio clasificar arrastra todo el
// análisis y nadie se entera.
type SaludMes struct {
	Periodo string `json:"periodo"`
	Movs    int    `json:"movs"`
	// PctClasificado: 100 = todo el mes tiene su partida asignada.
	PctClasificado string `json:"pct_clasificado"`
	// Comparable es false cuando el mes está tan incompleto que compararlo engaña.
	Comparable bool `json:"comparable"`
}

// TendenciaPartida es una partida con su serie, su promedio y su desvío del último mes.
type TendenciaPartida struct {
	ConceptoID      string `json:"concepto_id"`
	Concepto        string `json:"concepto"`
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Naturaleza      string `json:"naturaleza"`
	// NaturalezaDeclarada separa «el usuario decidió que no entra al EBITDA» de «nadie lo decidió
	// todavía»: sin esto la pantalla llama «sin declarar» a Utilidades y a Proyecto Edificio, que
	// son rubros legítimos del negocio. Ver migración 0064.
	NaturalezaDeclarada bool           `json:"naturaleza_declarada"`
	Serie               []PuntoPartida `json:"serie"`
	// Total del rango y promedio mensual sobre los meses COMPARABLES.
	Total    string `json:"total"`
	Promedio string `json:"promedio"`
	// Ultimo es el monto del mes más reciente del rango; DesvioPct cuánto se aparta del promedio
	// de los meses anteriores (positivo = gastó más de lo habitual).
	Ultimo    string `json:"ultimo"`
	DesvioPct string `json:"desvio_pct"`
	// MesesConDato: sobre cuántos meses comparables se calculó el promedio. Con 0 o 1 no hay
	// historia suficiente y el desvío no significa nada: la pantalla lo dice en vez de inventarlo.
	MesesConDato int  `json:"meses_con_dato"`
	Confiable    bool `json:"confiable"`

	// ── La base robusta (tanda 2) ──────────────────────────────────────────────────────────
	//
	// La MEDIANA de los mismos meses con los que se calcula el promedio, y el desvío contra
	// ella. Existe porque un pago grande infla el promedio para siempre: medido en Memorial
	// Pets, el promedio está entre 90 % y 126 % por encima de la mediana en varias partidas, y
	// a partir de ese mes todos los meses normales marcan una caída que no ocurrió.
	//
	// No reemplaza al promedio: la pantalla deja elegir contra cuál comparar.
	Mediana          string `json:"mediana"`
	DesvioMedianaPct string `json:"desvio_mediana_pct"`

	// MesesConGasto: en cuántos meses comparables esta partida tuvo movimiento. No es lo mismo
	// que MesesConDato —una partida que solo se paga en diciembre tiene un mes con gasto aunque
	// el rango tenga doce—.
	MesesConGasto int `json:"meses_con_gasto"`
	// Recurrencia: FIJO, INTERMITENTE o ESPORADICO. Para una partida esporádica, «promedio
	// mensual» no significa nada: no hay ritmo del que apartarse. Son 72 de 96 en Memorial, y
	// apenas el 19,6 % del dinero.
	Recurrencia string `json:"recurrencia"`

	// ── Comparación con el mismo rango del año anterior (solo si se pidió) ──────────────────
	//
	// POR QUÉ HACE FALTA, Y NO ALCANZA CON EL PROMEDIO: el promedio móvil no distingue
	// crecimiento de estacionalidad de anomalía. Medido en Memorial Pets el 27-oct-2026, el
	// gasto mensual va de ₡7,8 M a ₡23,4 M —tres veces—, así que contra la media simple un
	// tercio de los meses se pasa del umbral de 25 % sin que ocurra nada raro. Y el mismo mes
	// contra el año anterior da la lectura OPUESTA: febrero +96 %, julio −28 %.
	//
	// Las dos lecturas son legítimas y contestan preguntas distintas («¿se salió de su cauce?»
	// y «¿cómo venimos contra el año pasado?»). Por eso conviven en vez de reemplazarse.
	TotalAnterior string `json:"total_anterior,omitempty"`
	// VariacionAnualPct es la variación contra el mismo rango del año anterior.
	VariacionAnualPct string `json:"variacion_anual_pct,omitempty"`
	// SinBaseAnterior: la partida no existía el año pasado, así que no hay con qué compararla.
	// Es distinto de «varió 0 %» y de «cayó 100 %»: decirlo evita leer un +∞ como un dato.
	SinBaseAnterior bool `json:"sin_base_anterior,omitempty"`
}

// AnalisisPartidas es la respuesta completa.
type AnalisisPartidas struct {
	Desde    string             `json:"desde"`
	Hasta    string             `json:"hasta"`
	Meses    []SaludMes         `json:"meses"`
	Partidas []TendenciaPartida `json:"partidas"`
	// MesesComparables: cuántos de los meses del rango sirven para comparar.
	MesesComparables int `json:"meses_comparables"`
	// Aviso explica en una frase por qué el análisis puede no ser confiable (vacío si está bien).
	Aviso string `json:"aviso"`

	// Estacionalidad: cuánto pesa cada mes del calendario frente a un mes cualquiera. Vacía
	// cuando el rango es corto, y eso es correcto: tres meses no son un patrón anual.
	Estacionalidad []MesEstacional `json:"estacionalidad,omitempty"`
	// MesesConDosAnios: cuántos meses del calendario tienen al menos dos observaciones. Con
	// pocos, el índice es el dato de un año y no un patrón. Medido en Memorial el 28-oct-2026:
	// solo 5 de 12.
	MesesConDosAnios int `json:"meses_con_dos_anios,omitempty"`

	// ── Comparación anual, cuando se pidió ─────────────────────────────────────────────────
	ComparaAnual bool   `json:"compara_anual"`
	DesdeAnt     string `json:"desde_ant,omitempty"`
	HastaAnt     string `json:"hasta_ant,omitempty"`
	// MesesComparablesAnt: cuántos meses del rango ANTERIOR sirven. Si son pocos, la comparación
	// anual miente por abajo —el año pasado parece más barato solo porque está sin clasificar—.
	MesesComparablesAnt int `json:"meses_comparables_ant,omitempty"`
	// AvisoAnual dice por qué la comparación con el año anterior puede no servir.
	AvisoAnual string `json:"aviso_anual,omitempty"`

	// ── Las dos formas en que la columna anual NO cuadra ───────────────────────────────────
	//
	// La lista de partidas sale del rango ACTUAL, así que la comparación tiene dos agujeros que
	// no se ven y que invitan a sumar la columna y sacar una conclusión falsa:
	//
	//  · PARTIDAS NUEVAS: están este año y no el pasado. Su crecimiento es, en parte,
	//    reclasificación —el gasto existía bajo otro nombre—. Medido en Memorial Pets, Q1-2026:
	//    Operaciones Bancarias, Mercadeo, Gas, Servicios Profesionales y Leasing no existían como
	//    partida en Q1-2025, y entre las cinco son ₡16,7 M de ₡45,8 M (el 36 % del trimestre).
	//
	//  · PARTIDAS QUE DEJARON DE USARSE: estaban el año pasado y ya no. NO APARECEN EN LA TABLA,
	//    porque no hay fila donde ponerlas, así que sumar la columna «año anterior» da menos que
	//    el año anterior de verdad. En el mismo Q1 son ₡9,9 M que se perderían en silencio.
	PartidasSinBase int    `json:"partidas_sin_base,omitempty"`
	MontoSinBase    string `json:"monto_sin_base,omitempty"`
	PartidasSoloAnt int    `json:"partidas_solo_ant,omitempty"`
	MontoSoloAnt    string `json:"monto_solo_ant,omitempty"`
}

// umbralClasificadoComparable es el mínimo de clasificación para que un mes entre en el promedio.
// Por debajo, el mes muestra menos gasto del que tuvo y compararlo produce anomalías falsas.
const umbralClasificadoComparable = 90.0

// esComparable decide si un mes entra a los cálculos.
//
// Está extraída porque la usan dos sitios —el análisis y la estacionalidad— y porque antes la
// segunda LEÍA el campo `Comparable` suponiendo que alguien ya lo había calculado: llamada con
// la salud cruda devolvía vacío en silencio, que es la peor forma de equivocarse.
func esComparable(m SaludMes) bool {
	if m.Movs <= 0 {
		return false
	}
	pct, err := decimal.NewFromString(m.PctClasificado)
	if err != nil {
		return false
	}
	return pct.GreaterThanOrEqual(decimal.NewFromFloat(umbralClasificadoComparable))
}

// maxMesesAnalisis acota el rango que se puede pedir: cada mes multiplica por el número de
// partidas, y un rango de años arma una tabla que ninguna pantalla puede mostrar.
const maxMesesAnalisis = 24

// MesesDelRango cuenta los meses de CALENDARIO entre dos períodos, con las dos puntas incluidas:
// de 2026-09 a 2026-09 es 1, y de 2024-10 a 2026-09 son 24.
//
// Antes se contaba dividiendo los días entre 28 (`horas/24/28 + 1`), y 28 es el mes MÁS CORTO que
// existe, así que todo rango largo salía inflado: los 24 meses que pedía el botón de la pantalla se
// contaban como 26 y el servidor los rechazaba con «el rango no puede pasar de 24 meses». El botón
// de 24 meses nunca funcionó, y el de 23 pasaba raspando porque se contaba como 24.
//
// Un mes no dura una cantidad fija de días; esto no se aproxima, se cuenta.
func MesesDelRango(desde, hasta time.Time) int {
	return (hasta.Year()-desde.Year())*12 + int(hasta.Month()-desde.Month()) + 1
}

// AnalisisPartidasComparado agrega al análisis el mismo rango del año anterior.
//
// SE RESUELVE EN EL SERVIDOR Y NO EN LA PANTALLA porque lo delicado es ALINEAR los períodos:
// 2026-02 contra 2025-02, no contra «el segundo elemento del arreglo». Un rango que cruza fin de
// año, o un mes sin movimientos que el relleno completa en cero, desalinean las dos series y
// nadie lo nota: los números se ven razonables y están corridos un mes.
func (s *Service) AnalisisPartidasComparado(ctx context.Context, empresaID, desde, hasta string) (AnalisisPartidas, error) {
	out, err := s.AnalisisPartidas(ctx, empresaID, desde, hasta)
	if err != nil {
		return AnalisisPartidas{}, err
	}
	desdeAnt, hastaAnt := unAnioAntes(desde), unAnioAntes(hasta)
	ant, err := s.AnalisisPartidas(ctx, empresaID, desdeAnt, hastaAnt)
	if err != nil {
		return AnalisisPartidas{}, err
	}

	// Se cruza por CLASIFICACIÓN, no por nombre: dos partidas pueden llamarse igual bajo
	// conceptos distintos, y renombrar una no debería romper la comparación.
	totalAnt := map[string]decimal.Decimal{}
	for _, p := range ant.Partidas {
		m, err := decimal.NewFromString(p.Total)
		if err != nil {
			return AnalisisPartidas{}, fmt.Errorf("bancos: total anterior de %s: %w", p.Clasificacion, err)
		}
		totalAnt[p.ClasificacionID] = m
	}

	vistas := map[string]bool{}
	montoSinBase := decimal.Zero
	for i := range out.Partidas {
		p := &out.Partidas[i]
		vistas[p.ClasificacionID] = true
		base, hubo := totalAnt[p.ClasificacionID]
		actual, err := decimal.NewFromString(p.Total)
		if err != nil {
			return AnalisisPartidas{}, fmt.Errorf("bancos: total de %s: %w", p.Clasificacion, err)
		}
		if !hubo || !base.IsPositive() {
			// Sin base no hay porcentaje. Una partida nueva no «creció infinito»: no existía, y
			// eso es lo que hay que decir.
			p.SinBaseAnterior = true
			p.TotalAnterior = "0.00"
			out.PartidasSinBase++
			montoSinBase = montoSinBase.Add(actual)
			continue
		}
		p.TotalAnterior = base.StringFixed(2)
		p.VariacionAnualPct = actual.Sub(base).Div(base).Mul(decimal.NewFromInt(100)).StringFixed(1)
	}
	out.MontoSinBase = montoSinBase.StringFixed(2)

	// Lo que estaba el año pasado y ya no está no tiene fila donde aparecer, así que se cuenta
	// aparte. Sin esto, sumar la columna «año anterior» da menos que el año anterior de verdad y
	// nada avisa de la diferencia.
	montoSoloAnt := decimal.Zero
	for _, p := range ant.Partidas {
		if vistas[p.ClasificacionID] {
			continue
		}
		m, err := decimal.NewFromString(p.Total)
		if err != nil {
			return AnalisisPartidas{}, fmt.Errorf("bancos: total anterior de %s: %w", p.Clasificacion, err)
		}
		if !m.IsPositive() {
			continue
		}
		out.PartidasSoloAnt++
		montoSoloAnt = montoSoloAnt.Add(m)
	}
	out.MontoSoloAnt = montoSoloAnt.StringFixed(2)

	out.ComparaAnual = true
	out.DesdeAnt, out.HastaAnt = desdeAnt, hastaAnt
	out.MesesComparablesAnt = ant.MesesComparables
	// Los avisos se ACUMULAN en vez de pisarse: las tres cosas pueden pasar a la vez y cada una
	// desvía la lectura en la misma dirección —hacer que este año parezca más caro—, así que
	// quedarse con una sola dejaría al lector creyendo que ya conoce el reparo.
	var reparos []string
	switch {
	case ant.MesesComparables == 0:
		reparos = append(reparos,
			"No hay ningún mes comparable en el mismo rango del año anterior: todavía no se puede comparar contra el año pasado.")
	case ant.MesesComparables < out.MesesComparables:
		reparos = append(reparos, fmt.Sprintf(
			"El año anterior tiene %d mes(es) comparables contra %d de este rango: lo que falta por clasificar allá hace que este año parezca más caro de lo que es.",
			ant.MesesComparables, out.MesesComparables))
	}
	if out.PartidasSinBase > 0 {
		reparos = append(reparos, fmt.Sprintf(
			"%d partida(s) de este rango no existían el año anterior, por %s. Parte de lo que se ve como crecimiento puede ser que el mismo gasto antes se clasificaba con otro nombre.",
			out.PartidasSinBase, formatoMiles(montoNum(out.MontoSinBase))))
	}
	if out.PartidasSoloAnt > 0 {
		reparos = append(reparos, fmt.Sprintf(
			"%d partida(s) que se usaban el año anterior ya no aparecen, por %s: no tienen fila en la tabla, así que sumar la columna del año anterior da de menos.",
			out.PartidasSoloAnt, formatoMiles(montoNum(out.MontoSoloAnt))))
	}
	out.AvisoAnual = strings.Join(reparos, " ")
	return out, nil
}

// unAnioAntes corre un período «YYYY-MM» doce meses hacia atrás. Devuelve el original si no se
// puede leer; el llamador ya validó la forma.
func unAnioAntes(periodo string) string {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return periodo
	}
	return t.AddDate(-1, 0, 0).Format("2006-01")
}

// AnalisisPartidas devuelve la serie mensual de cada partida en el rango, con su promedio y su
// desvío, más la salud de cada mes.
func (s *Service) AnalisisPartidas(ctx context.Context, empresaID, desde, hasta string) (AnalisisPartidas, error) {
	salud, err := s.repo.SaludMeses(ctx, empresaID, desde, hasta)
	if err != nil {
		return AnalisisPartidas{}, err
	}
	comparables := map[string]bool{}
	nComparables := 0
	for i := range salud {
		salud[i].Comparable = esComparable(salud[i])
		if salud[i].Comparable {
			comparables[salud[i].Periodo] = true
			nComparables++
		}
	}

	crudas, err := s.repo.SeriePorPartida(ctx, empresaID, desde, hasta)
	if err != nil {
		return AnalisisPartidas{}, err
	}

	out := AnalisisPartidas{Desde: desde, Hasta: hasta, Meses: salud, MesesComparables: nComparables}
	for i := range crudas {
		p := &crudas[i]
		// El promedio y el desvío se calculan SOLO sobre meses comparables, y excluyendo el último
		// (el que se está juzgando): compararlo contra un promedio que lo incluye lo diluye.
		total := decimal.Zero
		sumaAnteriores := decimal.Zero
		nAnteriores := 0
		ultimoMonto := decimal.Zero
		// Los montos anteriores se guardan además de sumarse: la mediana necesita los valores,
		// no su suma.
		anteriores := make([]decimal.Decimal, 0, len(p.Serie))
		conGasto := 0
		for j, punto := range p.Serie {
			m, err := decimal.NewFromString(punto.Monto)
			if err != nil {
				return AnalisisPartidas{}, fmt.Errorf("bancos: monto de %s en %s: %w", p.Clasificacion, punto.Periodo, err)
			}
			total = total.Add(m)
			// La recurrencia mira TODOS los meses comparables, el último incluido: es una
			// propiedad de la partida, no un juicio sobre el mes que se está mirando.
			if comparables[punto.Periodo] && !m.IsZero() {
				conGasto++
			}
			esUltimo := j == len(p.Serie)-1
			if esUltimo {
				ultimoMonto = m
				continue
			}
			if comparables[punto.Periodo] {
				sumaAnteriores = sumaAnteriores.Add(m)
				anteriores = append(anteriores, m)
				nAnteriores++
			}
		}
		p.MesesConGasto = conGasto
		p.Recurrencia = recurrenciaDe(conGasto, nComparables)
		p.Total = total.StringFixed(2)
		p.Ultimo = ultimoMonto.StringFixed(2)
		p.MesesConDato = nAnteriores
		// Con menos de dos meses previos comparables, un «promedio» es un solo dato y el desvío
		// contra él no dice nada. Se marca como no confiable en vez de mostrar un número vistoso.
		p.Confiable = nAnteriores >= 2
		if nAnteriores > 0 {
			prom := sumaAnteriores.Div(decimal.NewFromInt(int64(nAnteriores)))
			p.Promedio = prom.StringFixed(2)
			if prom.IsPositive() {
				p.DesvioPct = ultimoMonto.Sub(prom).Div(prom).Mul(decimal.NewFromInt(100)).StringFixed(1)
			} else {
				p.DesvioPct = "0.0"
			}
			// La mediana sale de LOS MISMOS meses que el promedio —comparables y sin el último—
			// para que las dos bases se puedan comparar entre sí. Calcularla sobre otro conjunto
			// haría que la diferencia entre ambas mezclara dos efectos.
			med := mediana(anteriores)
			p.Mediana = med.StringFixed(2)
			if med.IsPositive() {
				p.DesvioMedianaPct = ultimoMonto.Sub(med).Div(med).Mul(decimal.NewFromInt(100)).StringFixed(1)
			} else {
				p.DesvioMedianaPct = "0.0"
			}
		} else {
			p.Promedio = "0.00"
			p.DesvioPct = "0.0"
			p.Mediana = "0.00"
			p.DesvioMedianaPct = "0.0"
		}
		out.Partidas = append(out.Partidas, *p)
	}

	// El aviso: decirlo arriba, no al pie, y en términos de qué significa para el análisis.
	noComparables := len(salud) - nComparables
	switch {
	case nComparables == 0:
		out.Aviso = "Ningún mes del rango está clasificado al 90 % o más: los montos por partida están incompletos y todavía no se puede comparar nada."
	case nComparables < 3:
		out.Aviso = fmt.Sprintf("Solo %d mes(es) del rango sirven para comparar. Hacen falta al menos 3 para que un promedio signifique algo.", nComparables)
	case noComparables > 0:
		out.Aviso = fmt.Sprintf("%d mes(es) quedaron fuera del promedio por estar a medio clasificar: mostrarían menos gasto del que tuvieron.", noComparables)
	}

	// La estacionalidad va sobre TODAS las partidas del rango. Quien quiera la del gasto filtra
	// antes de llamar; acá no se puede saber qué naturaleza está mirando la pantalla.
	est, err := Estacionalidad(salud, out.Partidas)
	if err != nil {
		return AnalisisPartidas{}, fmt.Errorf("bancos: estacionalidad: %w", err)
	}
	out.Estacionalidad = est
	for _, e := range est {
		if e.Confiable {
			out.MesesConDosAnios++
		}
	}
	return out, nil
}
