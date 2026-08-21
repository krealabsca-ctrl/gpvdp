package cxc

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Grid es una hoja de cálculo ya en texto: filas × columnas. El importador trabaja
// contra esto y no contra un formato, así que el CSV y el Excel recorren exactamente
// el mismo camino de validación. Si el archivo y la API divergieran, sería porque
// alguien agregó una validación en un solo lado.
type Grid [][]string

// bom es la marca de orden de bytes que Excel pone al exportar UTF-8. Los dos archivos
// reales del usuario la traen y, sin quitarla, el primer encabezado no calza con nada.
// Se escribe como escape porque un BOM literal en el fuente no compila.
const bom = "\uFEFF"

// CargarGrid lee un archivo de cartera o de cobros, sea CSV o Excel, y lo devuelve como
// Grid. Detecta el formato por el contenido, no por la extensión: los exportadores
// mandan «.xls» que en realidad son CSV más seguido de lo que uno querría.
func CargarGrid(archivo []byte) (Grid, error) {
	if len(archivo) == 0 {
		return nil, ErrArchivoVacio
	}
	// Los XLSX son ZIP: empiezan con "PK".
	if len(archivo) > 2 && archivo[0] == 'P' && archivo[1] == 'K' {
		return gridDeExcel(archivo)
	}
	return gridDeCSV(archivo)
}

func gridDeExcel(archivo []byte) (Grid, error) {
	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return nil, fmt.Errorf("cxc: abrir excel: %w", err)
	}
	defer func() { _ = f.Close() }()
	hojas := f.GetSheetList()
	if len(hojas) == 0 {
		return nil, ErrArchivoVacio
	}
	rows, err := f.GetRows(hojas[0])
	if err != nil {
		return nil, fmt.Errorf("cxc: leer filas: %w", err)
	}
	return Grid(rows), nil
}

// gridDeCSV lee el CSV detectando el separador. El sistema de origen exporta con `;`
// (convención de Excel en español), pero una API o un exportador distinto puede mandar
// `,` o tabulador: se decide contando en el encabezado, que es la línea que sabemos
// que trae todas las columnas.
func gridDeCSV(archivo []byte) (Grid, error) {
	texto := strings.TrimPrefix(string(archivo), bom)
	sep := separadorDe(texto)

	r := csv.NewReader(strings.NewReader(texto))
	r.Comma = sep
	// Las filas del origen no siempre traen el mismo número de campos (columnas finales
	// vacías que el exportador recorta). Se leen igual y el importador valida por
	// encabezado; abortar por eso dejaría 70 000 filas afuera por una coma.
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("cxc: leer csv: %w", err)
	}
	out := make(Grid, 0, len(rows))
	for _, fila := range rows {
		if algoNoVacio(fila) {
			out = append(out, fila)
		}
	}
	if len(out) == 0 {
		return nil, ErrArchivoVacio
	}
	return out, nil
}

func separadorDe(texto string) rune {
	linea := texto
	if i := strings.IndexAny(texto, "\r\n"); i > 0 {
		linea = texto[:i]
	}
	mejor, max := ';', strings.Count(linea, ";")
	if n := strings.Count(linea, "\t"); n > max {
		mejor, max = '\t', n
	}
	if n := strings.Count(linea, ","); n > max {
		mejor = ','
	}
	return mejor
}

func algoNoVacio(fila []string) bool {
	for _, c := range fila {
		if strings.TrimSpace(c) != "" {
			return true
		}
	}
	return false
}

// celda devuelve la columna `i` de la fila, o "" si la fila viene corta.
func celda(fila []string, i int) string {
	if i < 0 || i >= len(fila) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(fila[i], bom))
}

var _ = io.EOF // el paquete io queda disponible para lectores futuros
