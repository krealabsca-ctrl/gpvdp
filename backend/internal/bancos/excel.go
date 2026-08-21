package bancos

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

// gridDeArchivo lee la hoja `preferida` de un Excel en memoria y, si no existe, la primera.
// La usa el diccionario del catálogo: el archivo lo edita gente que puede renombrar la hoja.
func gridDeArchivo(archivo []byte, preferida string) (Grid, error) {
	g, _, _, err := gridDeArchivoConHojas(archivo, preferida)
	return g, err
}

// gridDeArchivoConHojas hace lo mismo y además dice QUÉ hoja se leyó y cuáles había.
//
// Existe porque leer una sola hoja en silencio es una forma de perder trabajo sin avisar: alguien
// sube un libro con una hoja por cuenta, el sistema lee la primera, y los contadores del resumen
// parecen un éxito. Quien muestra el resultado necesita poder decir «leí la hoja X de estas tres».
func gridDeArchivoConHojas(archivo []byte, preferida string) (Grid, string, []string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return nil, "", nil, fmt.Errorf("bancos: abrir excel: %w", err)
	}
	defer func() { _ = f.Close() }()

	hojas := f.GetSheetList()
	if len(hojas) == 0 {
		return nil, "", nil, fmt.Errorf("bancos: el archivo no tiene hojas")
	}
	hoja := hojas[0]
	for _, h := range hojas {
		if strings.EqualFold(strings.TrimSpace(h), preferida) {
			hoja = h
			break
		}
	}
	rows, err := f.GetRows(hoja)
	if err != nil {
		return nil, "", nil, fmt.Errorf("bancos: leer filas: %w", err)
	}
	return Grid(rows), hoja, hojas, nil
}

// CargarGrid lee la primera hoja de un Excel y la devuelve como Grid de texto.
func CargarGrid(r io.Reader) (Grid, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("bancos: abrir excel: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("bancos: el archivo no tiene hojas")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("bancos: leer filas: %w", err)
	}
	return Grid(rows), nil
}
