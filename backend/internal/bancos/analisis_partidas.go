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
}

// umbralClasificadoComparable es el mínimo de clasificación para que un mes entre en el promedio.
// Por debajo, el mes muestra menos gasto del que tuvo y compararlo produce anomalías falsas.
const umbralClasificadoComparable = 90.0

// maxMesesAnalisis acota el rango que se puede pedir: cada mes multiplica por el número de
// partidas, y un rango de años arma una tabla que ninguna pantalla puede mostrar.
const maxMesesAnalisis = 24

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
		pct, _ := decimal.NewFromString(salud[i].PctClasificado)
		salud[i].Comparable = salud[i].Movs > 0 &&
			pct.GreaterThanOrEqual(decimal.NewFromFloat(umbralClasificadoComparable))
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
		for j, punto := range p.Serie {
			m, err := decimal.NewFromString(punto.Monto)
			if err != nil {
				return AnalisisPartidas{}, fmt.Errorf("bancos: monto de %s en %s: %w", p.Clasificacion, punto.Periodo, err)
			}
			total = total.Add(m)
			esUltimo := j == len(p.Serie)-1
			if esUltimo {
				ultimoMonto = m
				continue
			}
			if comparables[punto.Periodo] {
				sumaAnteriores = sumaAnteriores.Add(m)
				nAnteriores++
			}
		}
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
		} else {
			p.Promedio = "0.00"
			p.DesvioPct = "0.0"
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
	return out, nil
}
