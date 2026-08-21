package bancos

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// ToleranciaTrasladoDefault es el valor por defecto (1%) de la diferencia máxima
// (proporción) entre las dos patas de un traslado/overnight — RN-20, decisión del DF.
// La tolerancia efectiva es CONFIGURABLE por empresa (empresa.tolerancia_traslado);
// este default solo se usa como respaldo si la empresa no tiene un valor válido (>0).
var ToleranciaTrasladoDefault = decimal.NewFromFloat(0.01)

// toleranciaEfectiva devuelve la tolerancia de la empresa. 0 es un valor VÁLIDO
// (emparejamiento exacto, elegido explícitamente por el DF); el default 1% solo
// aplica como respaldo defensivo ante un valor negativo (imposible vía API).
func toleranciaEfectiva(pct decimal.Decimal) decimal.Decimal {
	if pct.IsNegative() {
		return ToleranciaTrasladoDefault
	}
	return pct
}

// PropuestaTraslado es un par candidato (débito en una cuenta ↔ crédito en otra).
// Las descripciones ayudan a desambiguar cuando varios montos pequeños coinciden.
type PropuestaTraslado struct {
	DebitoID           string `json:"debito_id"`
	CreditoID          string `json:"credito_id"`
	FechaDebito        string `json:"fecha_debito"`
	FechaCredito       string `json:"fecha_credito"`
	CuentaDebito       string `json:"cuenta_debito"`
	CuentaCredito      string `json:"cuenta_credito"`
	MontoDebito        string `json:"monto_debito"`
	MontoCredito       string `json:"monto_credito"`
	DescripcionDebito  string `json:"descripcion_debito"`
	DescripcionCredito string `json:"descripcion_credito"`
	// Puntaje, veredicto y las razones que lo explican (ver PuntuarTraslado).
	Puntaje   int      `json:"puntaje"`
	Veredicto string   `json:"veredicto"`
	Razones   []string `json:"razones"`
}

// Veredictos de una propuesta de traslado.
const (
	// TrasladoProbable: coincide el monto, la fecha y la descripción lo respalda, y es el
	// único candidato de ese movimiento. Se puede emparejar directo.
	TrasladoProbable = "PROBABLE"
	// TrasladoRevisar: coincide lo numérico pero falta respaldo en la descripción. Exige que
	// una persona lo confirme mirando el detalle.
	TrasladoRevisar = "REVISAR"
	// TrasladoAmbiguo: el mismo movimiento tiene varios candidatos posibles, así que ninguno
	// se puede dar por bueno de un clic. Un traslado real es único.
	TrasladoAmbiguo = "AMBIGUO"
	// TrasladoDescartado: la evidencia dice que NO es un traslado (cobro a cliente, monto
	// recurrente del negocio). No se propone.
	TrasladoDescartado = "DESCARTADO"
)

// SenalesTraslado son los hechos que el repositorio observa de un par candidato. El juicio
// vive aparte (PuntuarTraslado) para poder probarlo sin base de datos.
type SenalesTraslado struct {
	// DiceTraslado: alguna de las dos descripciones trae la palabra traslado/traspaso.
	// Es la señal más fuerte, y viene del negocio: «la misma descripción a veces lo indica».
	DiceTraslado bool
	// DiceCobro: alguna descripción tiene marcas de un cobro a cliente (SINPE móvil, SMO-,
	// «PAGO DE …»). Los cobros de planes NO son traslados, aunque el monto coincida.
	DiceCobro bool
	// MontoExacto: las dos patas son exactamente iguales (no solo dentro de la tolerancia).
	MontoExacto bool
	// DiasDiferencia: días entre las dos patas (0 = mismo día).
	DiasDiferencia int
	// MontoRedondo: múltiplo de 10 000 — la redondez típica de un traslado entre cuentas
	// propias frente a un cobro (₡300 000 sí, ₡4 600 no).
	MontoRedondo bool
	// MontoAlto: supera ₡1 000 000. Es una TENDENCIA que indicó el negocio, no un requisito:
	// pesa a favor, pero no descarta un traslado chico (hay traslados reales de ₡300 000).
	MontoAlto bool
	// VecesElMonto: cuántas veces aparece ese monto en el período. Un monto que se repite
	// decenas de veces es un cobro recurrente del negocio, no un traslado.
	VecesElMonto int
	// CandidatosDelMovimiento: cuántas parejas posibles tiene ese mismo movimiento.
	CandidatosDelMovimiento int
}

// vecesParaRecurrente: a partir de esta cantidad de repeticiones en el período, el monto se
// considera recurrente del negocio (cuotas de planes, comisiones fijas) y no un traslado.
// En Valle de Paz hay montos que aparecen más de mil veces, todos cobros de clientes.
const vecesParaRecurrente = 10

// Umbrales de veredicto (ver PuntuarTraslado).
const (
	puntajeProbable = 60
	puntajeRevisar  = 25
)

// PuntuarTraslado juzga un par candidato y explica por qué. Los pesos son criterio de
// ingeniería sobre las señales que definió el negocio; el veredicto y las razones viajan a
// la pantalla para que nadie empareje a ciegas.
func PuntuarTraslado(s SenalesTraslado) (int, string, []string) {
	puntaje := 0
	razones := make([]string, 0, 6)

	if s.DiceTraslado {
		puntaje += 50
		razones = append(razones, "la descripción dice traslado")
	}
	if s.MontoExacto {
		puntaje += 25
		razones = append(razones, "monto idéntico")
	}
	if s.DiasDiferencia == 0 {
		puntaje += 10
		razones = append(razones, "mismo día")
	}
	if s.MontoRedondo {
		puntaje += 15
		razones = append(razones, "monto redondo")
	}
	if s.MontoAlto {
		puntaje += 10
		razones = append(razones, "monto alto")
	}
	if s.DiceCobro {
		puntaje -= 60
		razones = append(razones, "la descripción es de un cobro a cliente")
	}
	if s.VecesElMonto >= vecesParaRecurrente {
		puntaje -= 40
		razones = append(razones, fmt.Sprintf("ese monto aparece %d veces en el período", s.VecesElMonto))
	}

	switch {
	case s.DiceCobro || s.VecesElMonto >= vecesParaRecurrente || puntaje < puntajeRevisar:
		return puntaje, TrasladoDescartado, razones
	case s.CandidatosDelMovimiento > 1:
		razones = append(razones, fmt.Sprintf("hay %d parejas posibles: elegí cuál", s.CandidatosDelMovimiento))
		return puntaje, TrasladoAmbiguo, razones
	// PROBABLE exige respaldo en la DESCRIPCIÓN, no solo que los números calcen: dos montos
	// grandes y redondos del mismo día pueden ser un pago a proveedor y un depósito de
	// cliente. Sin la palabra, lo confirma una persona.
	case s.DiceTraslado && puntaje >= puntajeProbable:
		return puntaje, TrasladoProbable, razones
	default:
		return puntaje, TrasladoRevisar, razones
	}
}

// MovTraslado es la vista mínima de un movimiento para validar un emparejamiento.
type MovTraslado struct {
	ID         string
	CuentaID   string
	Debito     decimal.Decimal
	Credito    decimal.Decimal
	EsTraslado bool
	Incluido   bool
}

// dentroDeTolerancia indica si |a-b| <= pct * max(a,b).
func dentroDeTolerancia(a, b, pct decimal.Decimal) bool {
	mayor := a
	if b.GreaterThan(a) {
		mayor = b
	}
	return a.Sub(b).Abs().LessThanOrEqual(mayor.Mul(pct))
}
