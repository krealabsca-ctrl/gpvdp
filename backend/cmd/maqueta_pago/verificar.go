package main

// Verificación del libro generado: se abre el .xlsx y se comparan los números del resumen contra
// los del cuadro que hoy se arma a mano. Mirar la pantalla no sirve: el valor que importa es el
// que quedó escrito en la celda.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// numero lee una celda tal como la devuelve GetRows: YA FORMATEADA. «15,509,442.67» no es un
// número para ParseFloat, y sin quitarle los separadores toda comparación falla por «ilegible»
// en vez de por el valor, que es un modo de fallo que se lee como un problema distinto.
func numero(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
}

func verificar(ruta string) int {
	f, err := excelize.OpenFile(ruta)
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	esperado := map[string]float64{
		"Total colones":                     15509442.67,
		"Total dólares colonizado":          2380176.00,
		"TOTAL COLONIZADO":                  17889618.67,
		"(−) Reserva BP Negocios":           912475.00,
		"= Total flujo normal Valle":        16977143.67,
		"(−) Solicitudes por fuera":         2874709.29,
		"= CRONOGRAMA TOTAL FLUJO NORMAL":   14102434.38,
		"= Cronograma colones flujo normal": 11722258.38,
	}

	filas, err := f.GetRows("Aprobación GF")
	if err != nil {
		panic(err)
	}
	fallos := 0
	vistos := 0
	for _, fila := range filas {
		if len(fila) < 5 || fila[0] == "" {
			continue
		}
		rot := fila[0]
		// El rótulo del dólar lleva el detalle del tipo de cambio pegado.
		if strings.HasPrefix(rot, "Total dólares colonizado") {
			rot = "Total dólares colonizado"
		}
		quiere, ok := esperado[rot]
		if !ok {
			continue
		}
		vistos++
		tiene, err := numero(fila[4])
		if err != nil {
			fmt.Printf("  FALLA  %-38s celda ilegible (%q)\n", rot, fila[4])
			fallos++
			continue
		}
		if dif := tiene - quiere; dif > 0.005 || dif < -0.005 {
			fmt.Printf("  FALLA  %-38s = %.2f, el cuadro dice %.2f\n", rot, tiene, quiere)
			fallos++
			continue
		}
		fmt.Printf("  ok     %-38s = %15.2f\n", rot, tiene)
	}
	if vistos != len(esperado) {
		fmt.Printf("  FALLA  se encontraron %d de %d líneas del resumen\n", vistos, len(esperado))
		fallos++
	}

	// El aviso de pago: su total tiene que ser la suma de sus cuatro transferencias.
	avisos, err := f.GetRows("Pago Colones")
	if err != nil {
		panic(err)
	}
	var suma, total float64
	for _, fila := range avisos {
		if len(fila) < 5 {
			continue
		}
		if fila[0] == "TOTAL A TRANSFERIR EN COLONES" {
			total, _ = numero(fila[4])
			continue
		}
		if fila[1] == "Colones" {
			v, err := numero(fila[4])
			if err == nil {
				suma += v
			}
		}
	}
	if dif := total - suma; dif > 0.005 || dif < -0.005 {
		fmt.Printf("  FALLA  el total del aviso (%.2f) no es la suma de sus filas (%.2f)\n", total, suma)
		fallos++
	} else {
		fmt.Printf("  ok     total del aviso de pago             = %15.2f\n", total)
	}
	return fallos
}
