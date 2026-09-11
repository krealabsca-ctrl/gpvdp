package bancos

// Día a día de una partida: CUÁNDO se movió la plata dentro del rango.
//
// Pedido del usuario (2026-09-10): la pantalla de Análisis solo sabía de meses, y la pregunta
// «¿este gasto fue de golpe o repartido?» no se podía contestar.
//
// ── POR QUÉ ES UNA VISTA APARTE Y NO UN PARÁMETRO DE LA MENSUAL ─────────────
//
// El análisis mensual juzga: compara cada partida contra su propio promedio y marca lo que se
// salió de cauce. Ese juicio NO se traduce al día. El gasto diario es a saltos —un día se paga el
// alquiler y diez días no pasa nada—, así que «este día se desvió 400 % del promedio diario»
// marcaría casi todos los días de pago y el aviso perdería todo su valor.
//
// Por eso acá no hay promedio, ni desvío, ni semáforo de anomalía: hay una serie. Lo que se
// contesta es CUÁNDO, no SI ESTÁ MAL. El «si está mal» sigue siendo mensual, que es donde el
// promedio significa algo.
//
// El monto usa la MISMA expresión que la serie mensual (`sqlMontoEnSuSentido`) y los mismos
// filtros. Si usara otros, las dos vistas de la misma pantalla mostrarían totales distintos del
// mismo gasto, que es peor que no tener la vista.

import (
	"context"
	"strings"
)

// DiaDePartida es lo que se movió en una partida en UN día. Solo vienen los días CON movimiento:
// rellenar con ceros los 180 días de un semestre convertiría la serie en una línea plana con
// picos, y esconde justamente lo que se quiere ver.
type DiaDePartida struct {
	Fecha string `json:"fecha"`
	Monto string `json:"monto"`
	Movs  int    `json:"movs"`
}

// SerieDiariaPartida es una partida con sus días.
type SerieDiariaPartida struct {
	ClasificacionID string         `json:"clasificacion_id"`
	Clasificacion   string         `json:"clasificacion"`
	Concepto        string         `json:"concepto"`
	Total           string         `json:"total"`
	Movs            int            `json:"movs"`
	Dias            []DiaDePartida `json:"dias"`
}

// AnalisisDiario es la respuesta completa.
type AnalisisDiario struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`
	// DiasConMovimiento: cuántos días distintos tuvieron movimiento en TODO el conjunto. Es el
	// dato que dice si la serie vale la pena mirarla o si fueron dos pagos sueltos.
	DiasConMovimiento int                  `json:"dias_con_movimiento"`
	Partidas          []SerieDiariaPartida `json:"partidas"`
}

// MaxPartidasDiario acota cuántas partidas se piden a la vez.
//
// No es una limitación técnica: es que una gráfica con veinte curvas no se lee. La pantalla pide
// las que el usuario seleccionó, y sin selección no pide nada —traer el día a día de las 168
// partidas sería mucha plata en pantalla y ninguna respuesta—.
const MaxPartidasDiario = 12

// SerieDiariaDePartidas devuelve el día a día de las partidas indicadas dentro del rango de meses.
//
// Sin clasificaciones devuelve vacío a propósito, y no «todas»: es una pregunta sobre partidas
// concretas. Un endpoint que ante la falta de filtro devuelve el universo entero es el que después
// nadie entiende por qué tarda.
func (s *Service) SerieDiariaDePartidas(ctx context.Context, empresaID, desde, hasta string, clasificaciones []string) (AnalisisDiario, error) {
	out := AnalisisDiario{Desde: desde, Hasta: hasta, Partidas: []SerieDiariaPartida{}}

	ids := limpiarIDs(clasificaciones)
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > MaxPartidasDiario {
		ids = ids[:MaxPartidasDiario]
	}

	partidas, err := s.repo.SerieDiariaPorPartida(ctx, empresaID, desde, hasta, ids)
	if err != nil {
		return AnalisisDiario{}, err
	}
	// Solo se pisa el slice vacío si vino algo: asignar un nil lo devolvería a `null` en el JSON y
	// la pantalla revienta al mapearlo. Es la misma trampa que tiró saldos diarios en producción.
	if partidas != nil {
		out.Partidas = partidas
	}

	// Días distintos con movimiento en el conjunto, no la suma por partida: dos partidas que se
	// movieron el mismo día son UN día con movimiento.
	dias := map[string]bool{}
	for i := range partidas {
		for _, d := range partidas[i].Dias {
			dias[d.Fecha] = true
		}
	}
	out.DiasConMovimiento = len(dias)
	return out, nil
}

// limpiarIDs quita vacíos y repetidos conservando el orden en que los pidió la pantalla.
func limpiarIDs(ids []string) []string {
	vistos := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || vistos[id] {
			continue
		}
		vistos[id] = true
		out = append(out, id)
	}
	return out
}
