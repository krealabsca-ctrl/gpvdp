package bancos

// PROBE TEMPORAL (revisor escéptico) — se borra al terminar.

import (
	"fmt"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Qué texto entrega excelize para números con ruido de coma flotante y con 4 decimales,
// en General y con formato de miles, y qué monto sale de parseMontoTolerante.
func TestSkepticMontoDesdeExcelize(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sh := "Sheet1"

	valores := []float64{
		1234.56,
		1234.5600000000001,
		0.1 + 0.2,
		1234.5678,
		350000.0 / 3.0,
		12345.678901234,
	}
	for i, v := range valores {
		cel, _ := excelize.CoordinatesToCellName(1, i+1) // A: General
		if err := f.SetCellValue(sh, cel, v); err != nil {
			t.Fatal(err)
		}
		cel2, _ := excelize.CoordinatesToCellName(2, i+1) // B: #,##0.00
		if err := f.SetCellValue(sh, cel2, v); err != nil {
			t.Fatal(err)
		}
		if st, err := f.NewStyle(&excelize.Style{NumFmt: 4}); err == nil { // #,##0.00
			_ = f.SetCellStyle(sh, cel2, cel2, st)
		}
		cel3, _ := excelize.CoordinatesToCellName(3, i+1) // C: texto tal cual
		_ = f.SetCellStr(sh, cel3, fmt.Sprintf("%v", v))
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
			d, err := parseMontoTolerante(c)
			fmt.Printf("SKEP %-4s celda=%-24q -> monto=%s err=%v\n", cel, c, d.StringFixed(2), err)
		}
	}
}

// El XML crudo que escribe excelize para esos valores (lo que Excel también escribiría).
func TestSkepticXMLCrudo(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetCellValue("Sheet1", "A1", 1234.5600000000001)
	_ = f.SetCellValue("Sheet1", "A2", 0.1+0.2)
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	g, _ := CargarGrid(buf)
	fmt.Printf("SKEP grid=%q\n", g)

	// Y con RawCellValue (lo que se leería si el archivo no trae estilo alguno)
	f2 := excelize.NewFile()
	defer func() { _ = f2.Close() }()
	_ = f2.SetCellValue("Sheet1", "A1", 1234.5600000000001)
	b2, _ := f2.WriteToBuffer()
	f3, err := excelize.OpenReader(b2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f3.Close() }()
	raw, err := f3.GetCellValue("Sheet1", "A1", excelize.Options{RawCellValue: true})
	fmt.Printf("SKEP raw=%q err=%v\n", raw, err)
	fmtd, err := f3.GetCellValue("Sheet1", "A1")
	fmt.Printf("SKEP formateado=%q err=%v\n", fmtd, err)
}

// Fila completa: una celda con 4 decimales, ¿qué estado y qué clave sale?
func TestSkepticFilaCon4Decimales(t *testing.T) {
	g := Grid{
		{"Fecha", "Cuenta", "Documento", "Débito", "Crédito", "Concepto", "Clasificación"},
		{"01/07/2026", "BN Colones", "123", "1234.5678", "", "Gastos", "Alquiler"},
		{"01/07/2026", "BN Colones", "124", "1234.56", "", "Gastos", "Alquiler"},
	}
	filas, err := LeerClasifExcel(g)
	fmt.Printf("SKEP err=%v\n", err)
	for _, f := range filas {
		fmt.Printf("SKEP fila %d deb=%q estado=%q detalle=%q\n", f.Linea, f.Debito, f.Estado, f.Detalle)
	}
}
