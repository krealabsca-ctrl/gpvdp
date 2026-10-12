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

// Hoja es el archivo leído en sus DOS formas, y existe por una sola razón: las fechas.
//
// Excel no guarda «1/2/2022», guarda el número 44593 y aparte el formato con que
// mostrarlo. Al leer solo lo formateado, lo que llega es el render de ESE formato —medido
// el 7-oct-2026: una celda con el 1 de febrero de 2022 llegaba como «02-01-22»— y ahí ya
// no hay forma de saber si el 02 es el mes o el día. El importador leía esa columna como
// vacía y la fila entera caía en cuarentena con «sin fecha de primer cobro ni de próximo
// pago», mientras el usuario miraba su archivo y veía las dos fechas puestas.
//
// Adivinar el orden del texto no es una opción: con DateStyle invertido ya se metieron
// fechas con mes y día cambiados en silencio, y acá serían miles de vencimientos movidos
// de mes. El número de serie, en cambio, es absoluto y no depende de ningún formato.
//
// Por eso se conservan las dos: `Valores` es lo que una persona ve y lo que sirve para
// todo lo demás, y `Crudos` es de donde salen las fechas. En un CSV son idénticas: no hay
// formato que aplicar.
type Hoja struct {
	Valores Grid
	Crudos  Grid
}

// bom es la marca de orden de bytes que Excel pone al exportar UTF-8. Los dos archivos
// reales del usuario la traen y, sin quitarla, el primer encabezado no calza con nada.
// Se escribe como escape porque un BOM literal en el fuente no compila.
const bom = "\uFEFF"

// CargarGrid lee un archivo de cartera o de cobros, sea CSV o Excel, y devuelve solo los
// valores formateados. Sirve para mirar el encabezado o contar filas; para LEER FECHAS hay
// que usar CargarHoja, que además trae el valor crudo (ver Hoja).
func CargarGrid(archivo []byte) (Grid, error) {
	h, err := CargarHoja(archivo)
	if err != nil {
		return nil, err
	}
	return h.Valores, nil
}

// CargarHoja lee un archivo de cartera o de cobros, sea CSV o Excel. Detecta el formato por
// el contenido, no por la extensión: los exportadores mandan «.xls» que en realidad son CSV
// más seguido de lo que uno querría.
func CargarHoja(archivo []byte) (Hoja, error) {
	if len(archivo) == 0 {
		return Hoja{}, ErrArchivoVacio
	}
	// Los XLSX son ZIP: empiezan con "PK".
	if len(archivo) > 2 && archivo[0] == 'P' && archivo[1] == 'K' {
		return hojaDeExcel(archivo)
	}
	g, err := gridDeCSV(archivo)
	if err != nil {
		return Hoja{}, err
	}
	return HojaDe(g), nil
}

// HojaDe arma una Hoja a partir de un grid de texto suelto, donde lo escrito ES el valor:
// un CSV, o una tabla construida a mano. Las fechas se leen entonces del texto, que es el
// único camino disponible y el correcto para esas fuentes.
func HojaDe(g Grid) Hoja { return Hoja{Valores: g, Crudos: g} }

// hojaDeExcel recorre la hoja DOS veces, con y sin aplicar el formato del libro.
//
// Dos pasadas y no una porque las dos lecturas sirven para cosas distintas y ninguna
// reemplaza a la otra: el texto formateado es lo que el usuario ve y lo que hay que
// mostrarle cuando algo no se entiende, y el crudo es lo único que permite leer una fecha
// sin adivinar si el primer número es el día o el mes.
func hojaDeExcel(archivo []byte) (Hoja, error) {
	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return Hoja{}, fmt.Errorf("cxc: abrir excel: %w", err)
	}
	defer func() { _ = f.Close() }()
	hojas := f.GetSheetList()
	if len(hojas) == 0 {
		return Hoja{}, ErrArchivoVacio
	}
	rows, err := f.GetRows(hojas[0])
	if err != nil {
		return Hoja{}, fmt.Errorf("cxc: leer filas: %w", err)
	}
	crudas, err := f.GetRows(hojas[0], excelize.Options{RawCellValue: true})
	if err != nil {
		return Hoja{}, fmt.Errorf("cxc: leer filas sin formato: %w", err)
	}
	return Hoja{Valores: Grid(rows), Crudos: Grid(crudas)}, nil
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
