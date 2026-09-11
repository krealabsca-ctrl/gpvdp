package cxp

import (
	"bytes"
	"strings"
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

	filas, traza, err := parsearFacturas(buf.Bytes())
	if err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if traza.FilasEnHoja != 1 || traza.SinClave != 0 || traza.Hoja == "" {
		t.Errorf("traza de lectura = %+v: tiene que decir de qué hoja salieron y qué se descartó", traza)
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

// EL BUG DE PRODUCCIÓN (9 de setiembre de 2026). El script de facturación formatea la emisión como
// `dd/mm/yyyy`, y esa cadena viajaba CRUDA al `::date` de Postgres, que está en `DateStyle = MDY`:
//
//	· día 13 al 31 → «date/time field value out of range» y la fila se rechazaba;
//	· día 1 al 12  → entraba con el mes y el día INVERTIDOS, sin un solo error a la vista.
//
// El caso «05/08/2026» es el que hay que mirar: tiene que dar el 5 de AGOSTO. Si algún día vuelve
// a dar el 8 de mayo, es que alguien volvió a interpretar mes-primero.
func TestFechaISONormalizaLoQueTraeElExcel(t *testing.T) {
	casos := []struct{ entrada, quiero, porque string }{
		{"2026-08-13", "2026-08-13", "ISO, el formato sin ambigüedad"},
		{"13/08/2026", "2026-08-13", "día-primero con día > 12: antes reventaba"},
		{"05/08/2026", "2026-08-05", "día-primero con día ≤ 12: antes entraba como 8 de MAYO"},
		{"5/8/2026", "2026-08-05", "sin ceros a la izquierda"},
		{"13-08-2026", "2026-08-13", "con guiones"},
		{"2026/08/13", "2026-08-13", "ISO con barras"},
		{"2026-08-13 14:30:00", "2026-08-13", "con hora: la emisión es un día"},
		{"46247", "2026-08-13", "número de serie de Excel (celda sin formato de fecha)"},
		{"", "", "vacío"},
		{"lo que sea", "", "ilegible: no se adivina"},
		{"31/02/2026", "", "31 de febrero no existe"},
	}
	for _, c := range casos {
		if got := fechaISO(c.entrada); got != c.quiero {
			t.Errorf("fechaISO(%q) = %q, quería %q — %s", c.entrada, got, c.quiero, c.porque)
		}
	}
}

// La clave numérica de Hacienda lleva la fecha de emisión adentro, en un formato que no admite dos
// lecturas. Es el blindaje contra el bug de arriba: aunque la columna del Excel venga mal, la clave
// no puede venir ambigua.
func TestFechaDeClave(t *testing.T) {
	casos := []struct{ clave, quiero, porque string }{
		{
			"50613082600310109024745300001010000000401177634001", "2026-08-13",
			"posiciones 4-9 = ddmmaa: día 13, mes 08, año 26",
		},
		{
			"50605082600310109024745300001010000000402177634002", "2026-08-05",
			"el caso ambiguo del Excel: en la clave el 05 es el DÍA y punto",
		},
		{"", "", "sin clave no hay fecha"},
		{"506130826", "", "clave corta: no es de 50 dígitos"},
		{"5061308260031010902474530000101000000040117763400X", "", "con una letra no es una clave"},
		{"50631022600310109024745300001010000000401177634001", "", "31 de febrero: el calendario manda"},
	}
	for _, c := range casos {
		if got := fechaDeClave(c.clave); got != c.quiero {
			t.Errorf("fechaDeClave(%q) = %q, quería %q — %s", c.clave, got, c.quiero, c.porque)
		}
	}
}

// Cuál de las dos fuentes gana, y si el desacuerdo se ve.
func TestFechaDeEmisionPrefiereLaClave(t *testing.T) {
	const claveAgosto13 = "50613082600310109024745300001010000000401177634001"
	casos := []struct {
		nombre          string
		clave, celda    string
		quiero          string
		quieroCorregida bool
	}{
		{
			nombre: "las dos coinciden", clave: claveAgosto13, celda: "13/08/2026",
			quiero: "2026-08-13", quieroCorregida: false,
		},
		{
			// Desacuerdo de verdad: la columna trae una fecha legible pero OTRA —el caso típico es
			// que el archivo ponga la fecha de recepción del correo en vez de la de emisión—. Manda
			// la clave y el desacuerdo queda contado para que se vea.
			nombre: "el Excel trae otra fecha", clave: claveAgosto13, celda: "14/08/2026",
			quiero: "2026-08-13", quieroCorregida: true,
		},
		{
			// Mes-primero NO es un desacuerdo: es ilegible, porque a propósito no se intenta esa
			// lectura. La clave la salva igual, y no se cuenta como corrección del archivo.
			nombre: "el Excel viene en mes-primero (ilegible)", clave: claveAgosto13, celda: "08/13/2026",
			quiero: "2026-08-13", quieroCorregida: false,
		},
		{
			nombre: "el Excel no trae fecha", clave: claveAgosto13, celda: "",
			quiero: "2026-08-13", quieroCorregida: false,
		},
		{
			nombre: "sin clave usable, manda el Excel", clave: "NOTA-INTERNA-1", celda: "13/08/2026",
			quiero: "2026-08-13", quieroCorregida: false,
		},
		{
			nombre: "sin clave y sin fecha", clave: "", celda: "",
			quiero: "", quieroCorregida: false,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, corregida := fechaDeEmision(c.clave, c.celda)
			if got != c.quiero || corregida != c.quieroCorregida {
				t.Errorf("fechaDeEmision(%q,%q) = %q/%v, quería %q/%v",
					c.clave, c.celda, got, corregida, c.quiero, c.quieroCorregida)
			}
		})
	}
}

// La fila entera: una factura con fecha dd/mm/yyyy y en dólares tiene que entrar bien.
func TestFilaAInputConFechaLatinaYDolares(t *testing.T) {
	fila := FilaImportada{
		Clave:        "506",
		FechaEmision: fechaISO("13/08/2026"),
		Vencimiento:  fechaISO("12/09/2026"),
		Moneda:       "USD",
		TC:           "512.35",
		Subtotal:     "100.00",
		IVA:          "13.00",
		Total:        "113.00",
	}
	in, err := filaAInput(fila, "prov-1")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if in.FechaEmision != "2026-08-13" {
		t.Errorf("fecha_emision = %q, quería 2026-08-13", in.FechaEmision)
	}
	if in.Moneda != "USD" || in.TC.String() != "512.35" {
		t.Errorf("moneda/TC = %s/%s, quería USD/512.35 (el TC de la factura manda)", in.Moneda, in.TC)
	}
}

func TestFilaAInputRechazaLoQueNoPuedeSalvar(t *testing.T) {
	casos := []struct {
		nombre   string
		fila     FilaImportada
		contiene string
	}{
		{
			// Sin fecha no hay vencimiento ni aging: la factura no sirve, y es mejor rechazarla
			// que guardarla con una fecha inventada.
			nombre:   "sin fecha de emisión",
			fila:     FilaImportada{Clave: "1", Moneda: "CRC", Total: "100"},
			contiene: "fecha de emisión",
		},
		{
			// El caso que antes se rechazaba SIEMPRE; ahora solo si de verdad falta el TC.
			nombre:   "USD sin tipo de cambio",
			fila:     FilaImportada{Clave: "1", FechaEmision: "2026-08-13", Moneda: "USD", Total: "100"},
			contiene: "tipo de cambio",
		},
		{
			nombre:   "moneda que la base no acepta",
			fila:     FilaImportada{Clave: "1", FechaEmision: "2026-08-13", Moneda: "EUR", Total: "100"},
			contiene: "no soportada",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := filaAInput(c.fila, "prov-1")
			if err == nil {
				t.Fatal("esperaba un rechazo con motivo")
			}
			if !strings.Contains(err.Error(), c.contiene) {
				t.Errorf("el motivo dice %q y tiene que mencionar %q", err.Error(), c.contiene)
			}
		})
	}
}
