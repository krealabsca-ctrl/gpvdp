package cxc

import (
	"errors"
	"sort"

	"github.com/shopspring/decimal"
)

// Estados de un cargo según cuánto le queda por cobrar.
const (
	CargoAbierto = "ABIERTO"
	CargoParcial = "PARCIAL"
	CargoSaldado = "SALDADO"
	CargoAnulado = "ANULADO"
)

// Estados de un cobro.
const (
	CobroAplicado       = "APLICADO"
	CobroSinIdentificar = "SIN_IDENTIFICAR"
	CobroReversado      = "REVERSADO"
)

var (
	ErrMontoInvalido    = errors.New("cxc: el monto del cobro debe ser mayor que cero")
	ErrCargoAjeno       = errors.New("cxc: el cargo no pertenece al contrato del cobro")
	ErrCargoSinSaldo    = errors.New("cxc: el cargo ya está saldado")
	ErrCobroYaReversado = errors.New("cxc: el cobro ya estaba reversado")
)

// CargoParaAplicar es lo que la aplicación necesita saber de un cargo abierto.
type CargoParaAplicar struct {
	ID string
	// Periodo y VenceEn se usan para ordenar y para explicar el resultado.
	Periodo  string
	VenceEn  string
	Monto    decimal.Decimal
	Aplicado decimal.Decimal
}

// Saldo es lo que todavía se le debe a este cargo. Derivado, nunca guardado.
func (c CargoParaAplicar) Saldo() decimal.Decimal { return c.Monto.Sub(c.Aplicado) }

// Aplicacion es un tramo del cobro imputado a un cargo.
type Aplicacion struct {
	CargoID string          `json:"cargo_id"`
	Periodo string          `json:"periodo"`
	Monto   decimal.Decimal `json:"monto"`
	// Parcial: el cargo quedó con saldo después de esta aplicación.
	Parcial bool `json:"parcial"`
	// EstadoFinal del cargo tras aplicar: PARCIAL o SALDADO.
	EstadoFinal string `json:"estado_final"`
}

// ResultadoAplicacion es lo que produce aplicar un cobro.
type ResultadoAplicacion struct {
	Aplicaciones []Aplicacion    `json:"aplicaciones"`
	Aplicado     decimal.Decimal `json:"aplicado"`
	// SaldoAFavor es lo que sobró y no calzó en ningún cargo. NO se pierde: queda a
	// favor del cliente y se usará contra cargos futuros.
	SaldoAFavor decimal.Decimal `json:"saldo_a_favor"`
}

// AplicarFIFO reparte un cobro entre los cargos abiertos, del MÁS VIEJO al más nuevo
// (decisión del Director Financiero: reduce la mora y la antigüedad de la cartera).
//
// Reglas que encierra, y por qué:
//   - Nunca aplica más de lo que le falta a un cargo. Sobre-aplicar produciría cargos con
//     saldo negativo, que es la forma silenciosa de perder plata en un CxC.
//   - Un pago que no alcanza deja el cargo PARCIAL, no «sin pagar»: pagar la mitad es un
//     hecho distinto de no pagar, y la gestión de cobro cambia con eso.
//   - Lo que sobra va a SALDO A FAVOR del cliente. No se descarta ni se fuerza contra un
//     cargo futuro que todavía no existe.
//   - Es PURA: mismo cobro y mismos cargos, mismo resultado. Lo demás (escribir, cambiar
//     estados) lo hace el repositorio en una transacción.
//
// Los cargos se ordenan acá y no se confía en el orden de entrada: si la consulta
// cambiara su ORDER BY, la plata se aplicaría distinto sin que nadie lo note.
func AplicarFIFO(monto decimal.Decimal, cargos []CargoParaAplicar) (ResultadoAplicacion, error) {
	if monto.Sign() <= 0 {
		return ResultadoAplicacion{}, ErrMontoInvalido
	}
	pendientes := make([]CargoParaAplicar, 0, len(cargos))
	for _, c := range cargos {
		if c.Saldo().Sign() > 0 {
			pendientes = append(pendientes, c)
		}
	}
	// Más viejo primero: por vencimiento y, a igual vencimiento, por período (para que
	// 1Q vaya antes que 2Q del mismo mes) y por id (para que el resultado sea estable).
	sort.SliceStable(pendientes, func(i, j int) bool {
		if pendientes[i].VenceEn != pendientes[j].VenceEn {
			return pendientes[i].VenceEn < pendientes[j].VenceEn
		}
		if pendientes[i].Periodo != pendientes[j].Periodo {
			return pendientes[i].Periodo < pendientes[j].Periodo
		}
		return pendientes[i].ID < pendientes[j].ID
	})

	res := ResultadoAplicacion{Aplicaciones: []Aplicacion{}, Aplicado: decimal.Zero}
	restante := monto
	for _, c := range pendientes {
		if restante.Sign() <= 0 {
			break
		}
		saldo := c.Saldo()
		aplicar := decimal.Min(restante, saldo)
		parcial := aplicar.LessThan(saldo)
		estado := CargoSaldado
		if parcial {
			estado = CargoParcial
		}
		res.Aplicaciones = append(res.Aplicaciones, Aplicacion{
			CargoID: c.ID, Periodo: c.Periodo, Monto: aplicar, Parcial: parcial, EstadoFinal: estado,
		})
		res.Aplicado = res.Aplicado.Add(aplicar)
		restante = restante.Sub(aplicar)
	}
	res.SaldoAFavor = restante
	return res, nil
}

// AplicarADestino aplica un cobro a cargos ELEGIDOS por el operador, en el orden que
// los eligió. Es la vía de excepción: el cliente dice explícitamente qué mes paga.
//
// Valida lo mismo que FIFO —nunca sobre-aplica— y devuelve error si un cargo pedido ya
// está saldado, en vez de ignorarlo en silencio: si el operador eligió ese cargo, hay que
// decirle que no correspondía.
func AplicarADestino(monto decimal.Decimal, cargos []CargoParaAplicar, orden []string) (ResultadoAplicacion, error) {
	if monto.Sign() <= 0 {
		return ResultadoAplicacion{}, ErrMontoInvalido
	}
	porID := make(map[string]CargoParaAplicar, len(cargos))
	for _, c := range cargos {
		porID[c.ID] = c
	}
	res := ResultadoAplicacion{Aplicaciones: []Aplicacion{}, Aplicado: decimal.Zero}
	restante := monto
	for _, id := range orden {
		c, ok := porID[id]
		if !ok {
			return ResultadoAplicacion{}, ErrCargoAjeno
		}
		if c.Saldo().Sign() <= 0 {
			return ResultadoAplicacion{}, ErrCargoSinSaldo
		}
		if restante.Sign() <= 0 {
			break
		}
		aplicar := decimal.Min(restante, c.Saldo())
		parcial := aplicar.LessThan(c.Saldo())
		estado := CargoSaldado
		if parcial {
			estado = CargoParcial
		}
		res.Aplicaciones = append(res.Aplicaciones, Aplicacion{
			CargoID: c.ID, Periodo: c.Periodo, Monto: aplicar, Parcial: parcial, EstadoFinal: estado,
		})
		res.Aplicado = res.Aplicado.Add(aplicar)
		restante = restante.Sub(aplicar)
	}
	res.SaldoAFavor = restante
	return res, nil
}

// EstadoDeCargo deriva el estado de un cargo a partir de sus montos. Se usa al aplicar y
// al reversar, para que las dos direcciones lleguen exactamente a la misma conclusión.
func EstadoDeCargo(monto, aplicado decimal.Decimal) string {
	switch {
	case aplicado.Sign() <= 0:
		return CargoAbierto
	case aplicado.GreaterThanOrEqual(monto):
		return CargoSaldado
	default:
		return CargoParcial
	}
}
