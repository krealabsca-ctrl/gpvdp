package cxp

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParsearFacturas(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	headers := []string{
		"Fecha Registro", "Clave (50 dígitos)", "Fecha de Emisión", "Número Consecutivo",
		"Nombre del Proveedor", "Cédula", "Moneda", "Subtotal (Total Venta)", "Total Impuestos",
		"Total Comprobante", "Condición (Contado / Crédito)", "Fecha de Vencimiento",
		"Asunto del Correo", "ID del Mensaje",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	fila := []string{
		"5/18/2026 17:31:56", "50618052600310109024745300001010000000402177634929", "2026-05-18",
		"45300001010000000402", "COMAPAN S.A", "3101090247", "CRC", "14004.42", "1380.53",
		"11999.99", "Contado", "2026-05-18", "asunto", "19e3d5a9db9e9495",
	}
	for i, v := range fila {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet, cell, v)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	filas, err := parsearFacturas(buf.Bytes())
	if err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %d, want 1", len(filas))
	}
	g := filas[0]
	if g.Clave != "50618052600310109024745300001010000000402177634929" {
		t.Errorf("clave = %q", g.Clave)
	}
	if g.Consecutivo != "45300001010000000402" || g.Proveedor != "COMAPAN S.A" ||
		g.Cedula != "3101090247" || g.Moneda != "CRC" || g.FechaEmision != "2026-05-18" ||
		g.Subtotal != "14004.42" || g.IVA != "1380.53" || g.Total != "11999.99" ||
		g.Condicion != "Contado" || g.Vencimiento != "2026-05-18" {
		t.Errorf("fila mal mapeada = %+v", g)
	}
}

func TestCondicionDeFila(t *testing.T) {
	casos := []struct {
		cond, emi, ven string
		wantCond       string
		wantPlazo      int
	}{
		{"Contado", "2026-07-01", "2026-07-01", "CONTADO", 0},
		{"Crédito", "2026-07-01", "2026-07-31", "CREDITO", 30},
		{"CREDITO", "2026-05-27", "2026-06-26", "CREDITO", 30},
		{"Crédito", "2026-07-01", "", "CREDITO", 0},           // sin vencimiento => plazo 0
		{"Crédito", "2026-07-10", "2026-07-01", "CREDITO", 0}, // vencimiento anterior => 0
		{"", "2026-07-01", "2026-07-31", "CONTADO", 0},
	}
	for _, c := range casos {
		cond, plazo := condicionDeFila(FilaImportada{Condicion: c.cond, FechaEmision: c.emi, Vencimiento: c.ven})
		if cond != c.wantCond || plazo != c.wantPlazo {
			t.Errorf("condicionDeFila(%q,%q,%q) = %s/%d, want %s/%d", c.cond, c.emi, c.ven, cond, plazo, c.wantCond, c.wantPlazo)
		}
	}
}
