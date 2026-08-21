// Package cxc implementa Cuentas por Cobrar sobre PARTIDAS ABIERTAS: cada período de
// cada contrato es un cargo con su vencimiento y su saldo, y los cobros se aplican
// contra cargos.
//
// Este archivo es el corazón del módulo y es lógica PURA (sin base de datos): la
// aritmética de períodos y el plan de cargos de un contrato. Si esto está bien, la
// meta del mes, la antigüedad y las modalidades no mensuales salen solas.
package cxc

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Formato de `periodo`, el identificador del cargo dentro del contrato:
//
//	"2026-07"      un ciclo que ocupa el mes (mensual, trimestral, semestral, anual)
//	"2026-07-1Q"   primera quincena
//	"2026-07-2Q"   segunda quincena
//
// Se eligió texto y no una fecha porque es EL MISMO lenguaje que ya usa el sistema de
// origen en el campo `Concepto` de sus pagos («M/JULIO», «1Q/JULIO», «2Q/SEPTIEMBRE»).
// Eso permite casar los cobros históricos con los cargos durante la migración.
const (
	SufijoPrimeraQuincena = "1Q"
	SufijoSegundaQuincena = "2Q"
)

// CargoPlan es un cargo que el generador propone crear. No toca la base: se
// previsualiza, se cuenta y después se confirma.
type CargoPlan struct {
	Periodo string          `json:"periodo"`
	VenceEn string          `json:"vence_en"` // YYYY-MM-DD
	Monto   decimal.Decimal `json:"monto"`
}

// ModalidadCiclo es lo que el generador necesita saber de la modalidad de cobro.
type ModalidadCiclo struct {
	Nombre string
	// MesesCiclo: 1 mensual · 3 trimestral · 6 semestral · 12 anual.
	MesesCiclo int
	// Quincenal: dos cargos por mes en vez de uno.
	Quincenal bool
}

// ContratoParaGenerar son los datos del contrato que el generador consume.
type ContratoParaGenerar struct {
	Numero string
	// FechaPrimerCobro ancla el ciclo. Sin ella no se puede generar nada: no se
	// inventa una fecha de arranque, la fila queda en cuarentena.
	FechaPrimerCobro time.Time
	// DiaPago: día del mes en que vence la cuota (columna «Dias de Pagos»).
	// 0 = no informado; se usa el día de FechaPrimerCobro.
	DiaPago int
	Cuota   decimal.Decimal
}

// Errores del generador.
var (
	ErrSinFechaPrimerCobro = fmt.Errorf("cxc: el contrato no tiene fecha de primer cobro")
	ErrCuotaInvalida       = fmt.Errorf("cxc: la cuota debe ser mayor que cero")
	ErrModalidadInvalida   = fmt.Errorf("cxc: modalidad sin ciclo válido")
	ErrRangoInvalido       = fmt.Errorf("cxc: el rango de generación está al revés")
)

// PlanDeCargos calcula los cargos de UN contrato entre `desde` y `hasta`, inclusive.
//
// Propiedades que importan:
//   - Es PURA y determinista: mismas entradas, mismo resultado. Se puede correr mil
//     veces y no cambia nada (la unicidad (contrato, periodo) hace el resto en la base).
//   - El ciclo se ancla en FechaPrimerCobro, no en enero: un contrato trimestral que
//     arrancó en febrero cobra en febrero, mayo, agosto y noviembre.
//   - `desde` recorta el arranque para no fabricar años de historia que nadie pidió;
//     nunca adelanta el ciclo.
//
// El MONTO de todos los cargos es la cuota vigente, porque es el único dato de monto
// que trae el sistema de origen. Para los cargos históricos eso es una aproximación:
// si un contrato cambió de cuota, los períodos viejos quedan con la de hoy. Es la razón
// por la que el importador deja el saldo del origen como informativo y la verdad de lo
// ya pagado llega en la fase 2, al aplicar los cobros.
func PlanDeCargos(c ContratoParaGenerar, m ModalidadCiclo, desde, hasta time.Time) ([]CargoPlan, error) {
	if c.FechaPrimerCobro.IsZero() {
		return nil, ErrSinFechaPrimerCobro
	}
	if c.Cuota.Sign() <= 0 {
		return nil, ErrCuotaInvalida
	}
	if m.MesesCiclo < 1 || m.MesesCiclo > 12 {
		return nil, ErrModalidadInvalida
	}
	if hasta.Before(desde) {
		return nil, ErrRangoInvalido
	}

	// El día de vencimiento sale de DiaPago; si no viene, del día del primer cobro.
	dia := c.DiaPago
	if dia < 1 || dia > 31 {
		dia = c.FechaPrimerCobro.Day()
	}

	out := make([]CargoPlan, 0, 16)
	// Se recorre el ciclo desde el primer cobro. Los períodos anteriores a `desde` se
	// saltan sin romper el paso del ciclo, para no desalinear un trimestral.
	ancla := primerDiaDelMes(c.FechaPrimerCobro)
	finMes := primerDiaDelMes(hasta)
	for mes := ancla; !mes.After(finMes); mes = mes.AddDate(0, m.MesesCiclo, 0) {
		if m.Quincenal {
			for _, q := range quincenasDelMes(mes, dia, c.Cuota) {
				if dentro(q.VenceEn, desde, hasta) {
					out = append(out, q)
				}
			}
			continue
		}
		vence := conDiaValido(mes, dia)
		if dentro(fecha(vence), desde, hasta) {
			out = append(out, CargoPlan{
				Periodo: mes.Format("2006-01"),
				VenceEn: fecha(vence),
				Monto:   c.Cuota,
			})
		}
	}
	return out, nil
}

// quincenasDelMes arma los dos cargos de un mes quincenal.
//
// PENDIENTE DE CONFIRMAR con la jefatura de cobros: se derivan del `dia_pago` del
// contrato (1Q vence ese día, 2Q quince días después, topado al fin de mes) porque es
// el único dato de vencimiento que trae el origen. Si la operación usa días fijos
// (p. ej. 15 y último), se cambia acá y en un solo lugar.
func quincenasDelMes(mes time.Time, dia int, cuota decimal.Decimal) []CargoPlan {
	p := mes.Format("2006-01")
	v1 := conDiaValido(mes, dia)
	v2 := conDiaValido(mes, dia+15)
	return []CargoPlan{
		{Periodo: p + "-" + SufijoPrimeraQuincena, VenceEn: fecha(v1), Monto: cuota},
		{Periodo: p + "-" + SufijoSegundaQuincena, VenceEn: fecha(v2), Monto: cuota},
	}
}

// conDiaValido devuelve el día pedido del mes, topado al último día real.
// Un contrato que cobra el 31 vence el 28 de febrero, no el 3 de marzo: correr la
// fecha al mes siguiente movería el cargo de período.
func conDiaValido(mes time.Time, dia int) time.Time {
	ultimo := ultimoDiaDelMes(mes)
	if dia > ultimo {
		dia = ultimo
	}
	if dia < 1 {
		dia = 1
	}
	return time.Date(mes.Year(), mes.Month(), dia, 0, 0, 0, 0, time.UTC)
}

func ultimoDiaDelMes(mes time.Time) int {
	return primerDiaDelMes(mes).AddDate(0, 1, -1).Day()
}

func primerDiaDelMes(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func fecha(t time.Time) string { return t.Format("2006-01-02") }

func dentro(f string, desde, hasta time.Time) bool {
	return f >= fecha(desde) && f <= fecha(hasta)
}

// ---- Lectura del período que trae el sistema de origen ----

// mesesES traduce el nombre del mes como lo escribe el sistema de origen. Incluye
// «SEPT» además de «SET»/«SEP» porque los datos reales traen las tres formas
// («ago-26», «ene-26», «sept-26»): una abreviatura de cuatro letras que rompería un
// parser ingenuo.
var mesesES = map[string]time.Month{
	"ENERO": time.January, "ENE": time.January,
	"FEBRERO": time.February, "FEB": time.February,
	"MARZO": time.March, "MAR": time.March,
	"ABRIL": time.April, "ABR": time.April,
	"MAYO": time.May, "MAY": time.May,
	"JUNIO": time.June, "JUN": time.June,
	"JULIO": time.July, "JUL": time.July,
	"AGOSTO": time.August, "AGO": time.August,
	"SEPTIEMBRE": time.September, "SEPT": time.September, "SET": time.September, "SEP": time.September,
	"OCTUBRE": time.October, "OCT": time.October,
	"NOVIEMBRE": time.November, "NOV": time.November,
	"DICIEMBRE": time.December, "DIC": time.December,
}

// PeriodoDesdeConcepto lee el período que el sistema de origen escribe pegado en el
// campo `Concepto` de sus pagos: «M/JULIO - Adepsa Zafiro5000.00», «1Q/JULIO - …»,
// «2Q/SEPTIEMBRE - …».
//
// El año NO viene en ese texto: se recibe aparte (el del pago) y se corrige cuando el
// mes quedaría en el futuro lejano — una planilla de enero puede estar pagando
// diciembre del año anterior.
//
// Es la pieza que permite reconstruir la historia de cargos en la migración en vez de
// declarar un saldo inicial a ciegas.
func PeriodoDesdeConcepto(concepto string, anioRef int) (string, bool) {
	t := strings.ToUpper(strings.TrimSpace(concepto))
	if t == "" {
		return "", false
	}
	// El prefijo va hasta la primera barra: «M», «1Q», «2Q».
	barra := strings.Index(t, "/")
	if barra <= 0 {
		return "", false
	}
	prefijo := strings.TrimSpace(t[:barra])
	// El mes va desde la barra hasta el primer separador (« - » o coma) o fin.
	resto := t[barra+1:]
	if i := strings.IndexAny(resto, " -,"); i >= 0 {
		resto = resto[:i]
	}
	mes, ok := mesesES[strings.TrimSpace(resto)]
	if !ok {
		return "", false
	}
	base := fmt.Sprintf("%04d-%02d", anioRef, int(mes))
	switch prefijo {
	case "M":
		return base, true
	case SufijoPrimeraQuincena, SufijoSegundaQuincena:
		return base + "-" + prefijo, true
	default:
		return "", false
	}
}

// DiasMora son los días vencidos de un cargo a una fecha dada. Negativo = todavía no
// vence (el tramo ADELANTADO existe porque los datos reales traen días negativos).
func DiasMora(venceEn, hoy time.Time) int {
	return int(hoy.Sub(venceEn).Hours() / 24)
}
