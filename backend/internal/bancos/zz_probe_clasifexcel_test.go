package bancos

// PROBE TEMPORAL — borrar. Solo mide comportamiento actual.

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestProbeFechaDeCelda(t *testing.T) {
	casos := []string{
		"03/07/2026", "07/03/2026", "13/07/2026", "07/13/2026",
		"3/7/2026", "1/2/2026", "8/18/2026",
		"02/01/06", "03-07-25", "03-07-2026", "07-03-2026",
		"2026-03-07", "2026-3-7",
		"18/08/2026 00:00", "01 AUG 2026 03:37", "01 JUN 2026",
		"46252", "46252.5", "45000", "20001", "19999", "80001",
		"1", "12345", "2026", "45,678",
		"18/08/2026 ", " 18/08/2026",
		"03/07/26", "03/07/1926",
		"29/02/2026", "31/02/2026", "32/01/2026",
	}
	for _, c := range casos {
		t, ok := fechaDeCelda(c)
		if ok {
			fmt.Printf("FECHA %-22q -> %s\n", c, t.Format("2006-01-02"))
		} else {
			fmt.Printf("FECHA %-22q -> INVALIDA\n", c)
		}
	}
}

func TestProbeParseMonto(t *testing.T) {
	casos := []string{
		"1.234", "1,2", "-1.234,56", "(1.234,56)", "1 234.567,89",
		"1234.5678", "1.234,567", "12.345", "1,234", "1,234.56",
		"2,5E+07", "2.5E+07", "1E+15", "25000000",
		"1.234,56 (ver nota 2)", "1.234,56-", "12 mil", "n/a", "-", "",
		"₡1.234,56", "$1,234.56", "CRC 1.234,56", "1'234.567,89",
		"0,00", "0", "1.234.567", "1,234,567.5", "1.5", "1.50",
		"−1.234,56", "1.234,56 CR", "45.000 y 2", "1..5", "1,,5", "..", "1.2.3",
	}
	for _, c := range casos {
		d, err := parseMontoTolerante(c)
		if err != nil {
			fmt.Printf("MONTO %-26q -> ERROR %v\n", c, err)
		} else {
			fmt.Printf("MONTO %-26q -> %s\n", c, d.String())
		}
	}
}

// Qué texto entrega excelize para celdas de fecha reales y números grandes.
func TestProbeExcelizeFormatos(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sh := "Sheet1"
	// numFmt 14 = builtin "mm-dd-yy"; 22 = "m/d/yy h:mm"
	for i, nf := range []int{14, 22, 15, 16, 17} {
		st, err := f.NewStyle(&excelize.Style{NumFmt: nf})
		if err != nil {
			t.Fatal(err)
		}
		cel, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetCellValue(sh, cel, 46252.0); err != nil { // 2026-08-18
			t.Fatal(err)
		}
		_ = f.SetCellStyle(sh, cel, cel, st)
	}
	// numFmt custom dd/mm/yyyy
	if st, err := f.NewStyle(&excelize.Style{CustomNumFmt: strptr("dd/mm/yyyy")}); err == nil {
		_ = f.SetCellValue(sh, "A6", 46252.0)
		_ = f.SetCellStyle(sh, "A6", "A6", st)
	}
	if st, err := f.NewStyle(&excelize.Style{CustomNumFmt: strptr("m/d/yyyy")}); err == nil {
		_ = f.SetCellValue(sh, "A7", 46252.0)
		_ = f.SetCellStyle(sh, "A7", "A7", st)
	}
	// Fecha sin estilo (General) y números grandes sin estilo
	_ = f.SetCellValue(sh, "B1", 46252.0)
	_ = f.SetCellValue(sh, "B2", 25000000.0)
	_ = f.SetCellValue(sh, "B3", 1234567890123456.0)
	_ = f.SetCellValue(sh, "B4", 0.000001)
	_ = f.SetCellValue(sh, "B5", 1234.56)
	// Número grande en columna angosta con formato de miles
	if st, err := f.NewStyle(&excelize.Style{NumFmt: 3}); err == nil { // #,##0
		_ = f.SetCellValue(sh, "B6", 25000000.0)
		_ = f.SetCellStyle(sh, "B6", "B6", st)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	g, err := gridDeArchivo(buf.Bytes(), hojaMovimientos)
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range g {
		for j, c := range row {
			cel, _ := excelize.CoordinatesToCellName(j+1, i+1)
			fmt.Printf("EXCELIZE %-4s -> %q\n", cel, c)
		}
	}
	_ = bytes.MinRead
}

func strptr(s string) *string { return &s }

// Qué hoja se lee cuando hay varias y ninguna se llama Movimientos.
func TestProbeHojas(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	// Sheet1 (primera) = resumen con Fecha + Concepto pero sin Débito/Crédito
	_ = f.SetSheetRow("Sheet1", "A1", &[]string{"Fecha", "Concepto", "Total"})
	_ = f.SetSheetRow("Sheet1", "A2", &[]string{"01/07/2026", "Gastos", "1.000,00"})
	if _, err := f.NewSheet("Bancos 2026"); err != nil {
		t.Fatal(err)
	}
	_ = f.SetSheetRow("Bancos 2026", "A1", &[]string{"Fecha", "Documento", "Débito", "Crédito", "Concepto", "Clasificación"})
	_ = f.SetSheetRow("Bancos 2026", "A2", &[]string{"01/07/2026", "123", "1.000,00", "", "Gastos", "Alquiler"})
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	g, err := gridDeArchivo(buf.Bytes(), hojaMovimientos)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("HOJAS grid leida = %v\n", g)
	filas, err := LeerClasifExcel(g)
	fmt.Printf("HOJAS filas=%v err=%v\n", filas, err)
}

// Encabezado duplicado y encabezado falso.
func TestProbeEncabezados(t *testing.T) {
	g := Grid{
		{"Fecha", "Concepto", "Total"},                                           // resumen (fila 1)
		{"01/07/2026", "Gastos", "1.000,00"},                                     //
		{"Fecha", "Documento", "Débito", "Crédito", "Concepto", "Clasificación"}, // real (fila 3)
		{"01/07/2026", "123", "1.000,00", "", "Gastos", "Alquiler"},              //
	}
	col, fila := columnasClasifExcel(g)
	fmt.Printf("ENC elegido fila=%d col=%v\n", fila, col)
	filas, err := LeerClasifExcel(g)
	for _, f := range filas {
		fmt.Printf("ENC fila %d: fecha=%q deb=%q cre=%q concepto=%q clasif=%q estado=%s detalle=%s\n",
			f.Linea, f.Fecha, f.Debito, f.Credito, f.Concepto, f.Clasificacion, f.Estado, f.Detalle)
	}
	fmt.Printf("ENC err=%v\n", err)

	// Dos columnas con el mismo papel: Débito CRC y Débito (segunda) USD
	g2 := Grid{
		{"Fecha", "Débito", "Crédito", "Débito", "Crédito", "Clasificación"},
		{"01/07/2026", "", "", "100,00", "", "Alquiler"},
	}
	col2, _ := columnasClasifExcel(g2)
	fmt.Printf("ENC2 col=%v\n", col2)
	filas2, err2 := LeerClasifExcel(g2)
	for _, f := range filas2 {
		fmt.Printf("ENC2 fila %d: deb=%q cre=%q estado=%s detalle=%s\n", f.Linea, f.Debito, f.Credito, f.Estado, f.Detalle)
	}
	fmt.Printf("ENC2 err=%v\n", err2)

	// Sin columnas de monto
	g3 := Grid{
		{"Fecha", "Descripción", "Concepto", "Clasificación"},
		{"01/07/2026", "algo", "Gastos", "Alquiler"},
	}
	filas3, err3 := LeerClasifExcel(g3)
	for _, f := range filas3 {
		fmt.Printf("ENC3 fila %d: estado=%s detalle=%s\n", f.Linea, f.Estado, f.Detalle)
	}
	fmt.Printf("ENC3 err=%v\n", err3)

	// Fila de totales con texto en la columna Concepto
	g4 := Grid{
		{"Fecha", "Débito", "Crédito", "Concepto", "Clasificación"},
		{"01/07/2026", "1.000,00", "", "Gastos", "Alquiler"},
		{"", "1.000,00", "", "TOTAL", ""},
	}
	filas4, _ := LeerClasifExcel(g4)
	for _, f := range filas4 {
		fmt.Printf("ENC4 fila %d: fecha=%q estado=%s detalle=%s\n", f.Linea, f.Fecha, f.Estado, f.Detalle)
	}
}
