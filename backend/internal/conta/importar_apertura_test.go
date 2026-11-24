package conta

// El lector del Excel de saldos iniciales.
//
// Lo que se prueba acá es la LECTURA, que es donde un archivo de otro sistema se interpreta mal
// en silencio: un saldo que entra en cero no rompe nada, solo deja el balance sin cuadrar meses
// después, y para entonces nadie recuerda qué archivo se cargó.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// excelDe arma un .xlsx en memoria con las filas dadas.
func excelDe(t *testing.T, filas [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for i, fila := range filas {
		for j, v := range fila {
			celda, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatalf("coordenadas: %v", err)
			}
			if err := f.SetCellValue("Sheet1", celda, v); err != nil {
				t.Fatalf("escribir celda: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatalf("serializar: %v", err)
	}
	return buf.Bytes()
}

// EL ENCABEZADO SE BUSCA, no se asume en la fila 1: un balance exportado trae el nombre de la
// empresa, el título y la fecha arriba.
func TestLeeElBalanceConTitulosArriba(t *testing.T) {
	archivo := excelDe(t, [][]any{
		{"VALLE DE PAZ S.A."},
		{"Balance de comprobación"},
		{"Al 31 de julio de 2026"},
		{},
		{"Cuenta", "Descripción", "Debe", "Haber"},
		{"1-01-01-001-001", "Caja Chica", "150000", ""},
		{"1-01-02-001-001", "Banco Nacional", "2.500.000", ""},
		{"2-01-01-001-001", "Proveedores", "", "2.650.000"},
	})

	filas, err := LeerAperturaDesdeExcel(archivo)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(filas) != 3 {
		t.Fatalf("vinieron %d filas, se esperaban 3: %+v", len(filas), filas)
	}
	if filas[0].Codigo != "1-01-01-001-001" || filas[0].Debe != "150000.00" {
		t.Errorf("primera fila: %+v", filas[0])
	}
	// EL CASO QUE ROMPÍA: miles con punto y sin decimales.
	if filas[1].Debe != "2500000.00" {
		t.Errorf("«2.500.000» se leyó como %q, se esperaba 2500000.00", filas[1].Debe)
	}
	if filas[2].Haber != "2650000.00" {
		t.Errorf("el haber se leyó como %q", filas[2].Haber)
	}
}

// LOS ENCABEZADOS DE VARIAS PALABRAS, que es como los escribe un balance de comprobación de
// verdad. Antes se normalizaban dejando los espacios, así que «Saldo Deudor» no coincidía con
// ninguna clave y el archivo más común del sistema anterior se rechazaba entero.
func TestReconoceEncabezadosDeVariasPalabras(t *testing.T) {
	casos := []struct{ codigo, debe, haber string }{
		{"Código Cuenta", "Saldo Deudor", "Saldo Acreedor"},
		{"No. Cuenta", "Debe", "Haber"},
		{"Número de Cuenta", "Debe", "Haber"},
		{"Cuenta Contable", "Deudor", "Acreedor"},
	}
	for _, c := range casos {
		t.Run(c.codigo+"/"+c.debe, func(t *testing.T) {
			archivo := excelDe(t, [][]any{
				{c.codigo, c.debe, c.haber},
				{"1-01-01-001-001", "1000", ""},
				{"2-01-01-001-000", "", "1000"},
			})
			filas, err := LeerAperturaDesdeExcel(archivo)
			if err != nil {
				t.Fatalf("leer: %v", err)
			}
			if len(filas) != 2 {
				t.Fatalf("vinieron %d filas, se esperaban 2", len(filas))
			}
			if filas[0].Debe != "1000.00" || filas[1].Haber != "1000.00" {
				t.Errorf("las columnas no se asignaron: %+v", filas)
			}
		})
	}
}

// LA COLUMNA ÚNICA DE «SALDO» con el signo decidiendo el lado: es como queda el balance cuando
// alguien lo resume a mano, y sin esto habría que reformatear el archivo antes de cargarlo —que
// es donde se cuelan los errores de tipeo sobre los saldos de apertura—.
func TestLeeUnaSolaColumnaDeSaldo(t *testing.T) {
	archivo := excelDe(t, [][]any{
		{"Cuenta", "Saldo"},
		{"1-01-01-001-001", "150000"},
		{"2-01-01-001-001", "-150000"},
	})
	filas, err := LeerAperturaDesdeExcel(archivo)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("filas: %d", len(filas))
	}
	if filas[0].Debe != "150000.00" || filas[0].Haber != "0.00" {
		t.Errorf("el positivo no fue al debe: %+v", filas[0])
	}
	if filas[1].Haber != "150000.00" || filas[1].Debe != "0.00" {
		t.Errorf("el negativo no fue al haber: %+v", filas[1])
	}
}

// Un saldo en las DOS columnas se netea en vez de rechazarse: un balance exportado a veces trae
// las dos llenas por un arrastre, y lo que importa es el neto de la cuenta.
func TestUnSaldoEnLosDosLadosSeNetea(t *testing.T) {
	archivo := excelDe(t, [][]any{
		{"Cuenta", "Debe", "Haber"},
		{"1-01-01-001-001", "1000", "300"}, // neto 700 al debe
		{"2-01-01-001-001", "200", "900"},  // neto 700 al haber
	})
	filas, err := LeerAperturaDesdeExcel(archivo)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if filas[0].Debe != "700.00" || filas[0].Haber != "0.00" {
		t.Errorf("no se neteó al debe: %+v", filas[0])
	}
	if filas[1].Haber != "700.00" || filas[1].Debe != "0.00" {
		t.Errorf("no se neteó al haber: %+v", filas[1])
	}
}

// Las cuentas en cero se descartan: no aportan al asiento de apertura y ensuciarían el libro
// con líneas que no dicen nada. No es un error del archivo.
func TestLasCuentasEnCeroNoEntran(t *testing.T) {
	archivo := excelDe(t, [][]any{
		{"Cuenta", "Debe", "Haber"},
		{"1-01-01-001-001", "1000", ""},
		{"1-01-01-001-002", "0", "0"},
		{"1-01-01-001-003", "", ""},
	})
	filas, err := LeerAperturaDesdeExcel(archivo)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(filas) != 1 {
		t.Errorf("entraron %d filas, se esperaba 1: %+v", len(filas), filas)
	}
}

// Sin encabezado reconocible, el error DICE QUÉ HACE FALTA. Un «formato inválido» obliga a
// adivinar sobre un archivo de cien columnas.
func TestSinEncabezadoDiceQueFalta(t *testing.T) {
	archivo := excelDe(t, [][]any{
		{"Lo que sea", "Otra cosa"},
		{"1-01", "100"},
	})
	_, err := LeerAperturaDesdeExcel(archivo)
	if err == nil {
		t.Fatal("aceptó un archivo sin encabezado reconocible")
	}
	if !strings.Contains(err.Error(), "Saldo") || !strings.Contains(err.Error(), "Debe") {
		t.Errorf("el error no dice qué columnas hacen falta: %v", err)
	}
}

// Una fila más corta que el encabezado no entra en pánico: las filas de un .xlsx llegan
// RECORTADAS en la última celda con contenido.
func TestUnaFilaCortaNoRompe(t *testing.T) {
	archivo := excelDe(t, [][]any{
		{"Cuenta", "Debe", "Haber"},
		{"1-01-01-001-001", "1000"}, // sin la columna de haber
	})
	filas, err := LeerAperturaDesdeExcel(archivo)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(filas) != 1 || filas[0].Debe != "1000.00" {
		t.Errorf("filas: %+v", filas)
	}
}
