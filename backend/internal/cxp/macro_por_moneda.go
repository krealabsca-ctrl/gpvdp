package cxp

// Una macro POR MONEDA, no una sola con todo adentro.
//
// El pedido del Director Financiero (9-oct-2026): «las facturas que entran en dólares se están
// convirtiendo en colones, y eso está mal: en la operativa diaria las facturas de dólares se pagan
// en dólares. Hay que saber identificar en todo el flujo lo que es de dólares y lo que es de
// colones, y hay que hacer macros para colones y macros para dólares **sin perder la integridad
// del mismo**.»
//
// Esa última frase manda el diseño: EL LOTE SIGUE SIENDO UNO. No se parte el corte ni se obliga a
// cortar dos veces; lo que se separa es el ARCHIVO que se sube al banco, porque una transferencia
// en dólares sale de una cuenta en dólares y el banco no acepta las dos cosas en la misma carga.
//
// Hasta ahora la macro mezclaba: ponía todas las líneas en un solo .txt y ni siquiera llevaba la
// moneda —el CSV sí, el .txt no—, así que el banco recibía «1000» sin saber si eran colones o
// dólares. Lo decidía la cuenta desde la que alguien subiera el archivo.

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// MacroDeMoneda es el archivo de UNA moneda, listo para subir al banco.
type MacroDeMoneda struct {
	Moneda string `json:"moneda"`
	// Lineas son las que van a ese archivo.
	Lineas []PagoRow `json:"lineas"`
	// Total es la suma de los netos, EN SU MONEDA. No hay un total general del lote a propósito:
	// sumar colones con dólares da un número que no significa nada, y mostrarlo invitaría a
	// cuadrarlo contra algo.
	Total string `json:"total"`
	// Archivo es cómo se llama el .txt. Lleva la moneda en el nombre porque los dos archivos
	// terminan en la misma carpeta de descargas y «macro-pagos.txt» dos veces es una trampa.
	Archivo string `json:"archivo"`
}

// SepararPorMoneda agrupa las líneas de pago en un archivo por moneda.
//
// Devuelve los grupos ORDENADOS con colones primero: es la moneda del 100 % de la operación hoy, y
// quien baje las macros espera encontrarla arriba. Un lote sin líneas devuelve vacío, no un grupo
// vacío: «no hay nada que pagar» y «hay un archivo sin líneas» son cosas distintas, y la segunda
// es la que se sube al banco sin que nadie lo note.
func SepararPorMoneda(rows []PagoRow, loteNumero int64) []MacroDeMoneda {
	if len(rows) == 0 {
		return nil
	}
	porMoneda := map[string][]PagoRow{}
	for _, r := range rows {
		m := strings.ToUpper(strings.TrimSpace(r.Moneda))
		if m == "" {
			// Una línea sin moneda declarada es de colones: así nacieron las 4.542 facturas que
			// existen, y tratarla como desconocida la dejaría fuera de los dos archivos.
			m = "CRC"
		}
		porMoneda[m] = append(porMoneda[m], r)
	}
	out := make([]MacroDeMoneda, 0, len(porMoneda))
	for moneda, lineas := range porMoneda {
		total := decimal.Zero
		for _, l := range lineas {
			if v, err := decimal.NewFromString(strings.TrimSpace(l.MontoNeto)); err == nil {
				total = total.Add(v)
			}
		}
		out = append(out, MacroDeMoneda{
			Moneda:  moneda,
			Lineas:  lineas,
			Total:   total.StringFixed(2),
			Archivo: NombreMacro(loteNumero, moneda),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		// Colones primero; el resto, alfabético.
		if out[i].Moneda == "CRC" {
			return true
		}
		if out[j].Moneda == "CRC" {
			return false
		}
		return out[i].Moneda < out[j].Moneda
	})
	return out
}

// NombreMacro arma el nombre del archivo: «macro-lote-0043-CRC.txt».
//
// El número del lote va adelante para que los archivos de dos cortes distintos no se pisen en la
// carpeta de descargas, y la moneda al final porque es lo que decide desde qué cuenta se sube.
func NombreMacro(loteNumero int64, moneda string) string {
	n := decimal.NewFromInt(loteNumero).String()
	for len(n) < 4 {
		n = "0" + n
	}
	return "macro-lote-" + n + "-" + strings.ToUpper(moneda) + ".txt"
}

// MonedasDe devuelve las monedas presentes, con colones primero. Sirve para que la pantalla diga
// cuántos archivos va a bajar antes de bajarlos.
func MonedasDe(rows []PagoRow) []string {
	grupos := SepararPorMoneda(rows, 0)
	out := make([]string, 0, len(grupos))
	for _, g := range grupos {
		out = append(out, g.Moneda)
	}
	return out
}

// LineasDeMoneda devuelve las líneas de UNA moneda, o nil si no hay.
//
// Es lo que usa la descarga: el endpoint recibe la moneda y baja ese archivo. Devolver nil cuando
// no hay —en vez de una lista vacía— es lo que permite que el handler conteste «este lote no tiene
// nada en dólares» en vez de entregar un .txt vacío.
func LineasDeMoneda(rows []PagoRow, moneda string) []PagoRow {
	m := strings.ToUpper(strings.TrimSpace(moneda))
	if m == "" {
		return nil
	}
	var out []PagoRow
	for _, r := range rows {
		rm := strings.ToUpper(strings.TrimSpace(r.Moneda))
		if rm == "" {
			rm = "CRC"
		}
		if rm == m {
			out = append(out, r)
		}
	}
	return out
}
