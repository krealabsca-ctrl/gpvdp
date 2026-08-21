package bancos

import (
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// Grillas sintéticas que reproducen el layout real de cada banco (ver docs/GPVDP_Formatos_Bancos_v1.0.md).

var gridPromerica = Grid{
	{"TRANSACCIONES POR FECHA"},
	{},
	{"Banco Promerica Costa Rica"},
	{"Número de cuenta: 30000002738132-CUENTA CORRIENTE CORPORATIVA COLONES"},
	{"Fecha", "Documento", "Descripción", "Débitos", "Créditos", "Saldo"},
	{"06-16-26", "17130407", "VALLE DE PAZ SERV: TRASLADO", "", "361,600.00", "497,292.69"},
	{"06-16-26", "50850563", "CANCELACION FACTURAS CCSS", "361,600.00", "", "135,692.69"},
	{"Total de Débitos", "361,600.00"},
}

var gridBN = Grid{
	{"oficina", "fechaMovimiento", "numeroDocumento", "debito", "credito", "descripcion"},
	{"228", "06-30-26", "29762797", "15,889.69", "", "PAGO M S C/JARDINES TROPICALES"},
	{"228", "06-01-26", "27505236", "", "2,000,000.00", "TRASLADO 1989 A 1990/VALLE DE PAZ"},
	{"", "", "TOTAL", "10,494,499.23", "10,500,000.00"},
}

var gridBAC = Grid{
	{"", "", "", "", "DETALLE DE MOVIMIENTOS DEL PERÍODO"},
	{"Producto", "CR26010200009038253541", "", "", "", "CRC"},
	{"Fecha", "Referencia", "", "Código", "Descripción", "", "", "Débitos", "Créditos", "Balance*"},
	{"", "", "", "", "Saldo Inicial", "", "", "", "", "7119622.00"},
	{"01/06/2026", "37641", "", "TF", "COBRO BAC TOKEN", "", "", "467.00", "0.00", "7119155.00"},
	{"01/06/2026", "37641", "", "TF", "COBRO BAC TOKEN", "", "", "467.00", "0.00", "7118688.00"},
	{"Cuadro de Resumen"},
}

var gridBCR = Grid{
	{},
	{"", "", "Cliente", "FUNERARIA LA RELIGIOSA"},
	{"Cuenta IBAN", "Tipo de Movimiento", "Fecha Desde", "Fecha Hasta"},
	{"CC-CR48015201349000020206", "Todos", "06-01-26", "06-30-26"},
	{"", "", "Movimientos de Cuenta"},
	{"Fecha Contable", "Fecha de Registro", "Hora", "Número Documento", "Descripción", "Oficina", "Débitos", "Créditos"},
	{"06-01-26", "06-01-26", "05:02", "5020383", "TRANSFERENC // JUNIO", "CONTA", "-", "8,700.00"},
	{"", "", "", "", "Total Débitos", "Total Créditos"},
}

var gridBP = Grid{
	{"Movimiento de cuenta", " Banco Popular"},
	{"Titular", " VALLE DE PAZ", "Cuenta IBAN", " CR62016101008810244232"},
	{"Rango de fecha", "01/06/2026 - 30/06/2026", "Moneda", "Colon Costa Rica"},
	{"Fecha y Hora", "Descripcion", "Documento", "Debito", "Creditos", "Saldo"},
	{"", "Saldo Inicial", "", "CRC ", "CRC ", "CRC 2,169,852.12"},
	{"01 JUN 2026 03:23", "Orden Fija VALLE DE PAZ", "FT26152PKWZW\\C36", "CRC 0.00", "CRC 6,500.00", "CRC 2,176,352.12"},
	{"", "Saldo Final", "", "CRC ", "CRC ", "CRC 1,562,023.54"},
}

var gridDavivienda = Grid{
	{"Titular de la cuenta", "VALLE DE PAZ"},
	{"Número de Cuenta", "CR76010409142215626710"},
	{"Moneda", "Colones"},
	{"Fecha", "Descripción", "Ref.", "Débitos (DR)", "Créditos (CR)", "Saldo Contable", "Ref2", "Tipo Tran", "Causa", "Sucursal", "D/C", "Cuenta"},
	{"01/06/2026", "TRF DE CR91..., DESINV OVN (DESINV. OVERNIGHT)", "186132439", "0.00", "7,754,527.33", "7,754,527.33", "", "48", "67", "50 - CASA MATRIZ", "C", "CR76010409142215626710"},
	{"30/06/2026", "TRF A CR91..., INV. OVERNIGHT", "2142955433", "5,618,392.20", "0.00", "0.00", "", "50", "64", "50 - CASA MATRIZ", "D", "CR76010409142215626710"},
}

func TestDetectarYParsear(t *testing.T) {
	casos := []struct {
		nombre       string
		grid         Grid
		banco        Banco
		nMovs        int
		iban         string
		moneda       string
		primerDebito string
		primerCredit string
	}{
		{"promerica", gridPromerica, BancoPromerica, 2, "", "CRC", "0", "361600"},
		{"bn", gridBN, BancoBN, 2, "", "", "15889.69", "0"},
		{"bac", gridBAC, BancoBAC, 2, "CR26010200009038253541", "CRC", "467", "0"},
		{"bcr", gridBCR, BancoBCR, 1, "CR48015201349000020206", "", "0", "8700"},
		{"bp", gridBP, BancoBP, 1, "CR62016101008810244232", "CRC", "0", "6500"},
		{"davivienda", gridDavivienda, BancoDavivienda, 2, "CR76010409142215626710", "CRC", "0", "7754527.33"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			a, err := Detectar(c.grid)
			if err != nil {
				t.Fatalf("Detectar: %v", err)
			}
			if a.Banco() != c.banco {
				t.Fatalf("banco = %q, quería %q", a.Banco(), c.banco)
			}
			res, err := a.Parsea(c.grid)
			if err != nil {
				t.Fatalf("Parsea: %v", err)
			}
			if len(res.Movimientos) != c.nMovs {
				t.Fatalf("movimientos = %d, quería %d", len(res.Movimientos), c.nMovs)
			}
			if res.IBAN != c.iban {
				t.Errorf("IBAN = %q, quería %q", res.IBAN, c.iban)
			}
			if res.MonedaArchivo != c.moneda {
				t.Errorf("moneda = %q, quería %q", res.MonedaArchivo, c.moneda)
			}
			m0 := res.Movimientos[0]
			if !m0.Debito.Equal(dec(c.primerDebito)) {
				t.Errorf("débito[0] = %s, quería %s", m0.Debito, c.primerDebito)
			}
			if !m0.Credito.Equal(dec(c.primerCredit)) {
				t.Errorf("crédito[0] = %s, quería %s", m0.Credito, c.primerCredit)
			}
		})
	}
}

func TestBACDuplicadosIndiceOcurrencia(t *testing.T) {
	res, err := bac.Parsea(gridBAC)
	if err != nil {
		t.Fatalf("Parsea: %v", err)
	}
	if len(res.Movimientos) != 2 {
		t.Fatalf("movimientos = %d, quería 2", len(res.Movimientos))
	}
	// Dos líneas idénticas (RN-07): se conservan ambas con índice de ocurrencia 1 y 2.
	if res.Movimientos[0].IndiceOcurrencia != 1 || res.Movimientos[1].IndiceOcurrencia != 2 {
		t.Errorf("indice_ocurrencia = %d,%d; quería 1,2",
			res.Movimientos[0].IndiceOcurrencia, res.Movimientos[1].IndiceOcurrencia)
	}
}

func TestFechas(t *testing.T) {
	res, _ := promerica.Parsea(gridPromerica)
	if got := res.Movimientos[0].Fecha.Format("2006-01-02"); got != "2026-06-16" {
		t.Errorf("fecha Promerica = %s, quería 2026-06-16", got)
	}
	res, _ = bp.Parsea(gridBP)
	if got := res.Movimientos[0].Fecha.Format("2006-01-02"); got != "2026-06-01" {
		t.Errorf("fecha BP = %s, quería 2026-06-01", got)
	}
	res, _ = bac.Parsea(gridBAC)
	if got := res.Movimientos[0].Fecha.Format("2006-01-02"); got != "2026-06-01" {
		t.Errorf("fecha BAC = %s, quería 2026-06-01", got)
	}
}

func TestParseMonto(t *testing.T) {
	casos := map[string]string{
		"":             "0",
		"-":            "0",
		"CRC ":         "0",
		"1,234.56":     "1234.56",
		"CRC 6,500.00": "6500",
		"361,600.00":   "361600",
	}
	for in, want := range casos {
		got, err := parseMonto(in)
		if err != nil {
			t.Errorf("parseMonto(%q): %v", in, err)
			continue
		}
		if !got.Equal(dec(want)) {
			t.Errorf("parseMonto(%q) = %s, quería %s", in, got, want)
		}
	}
}

func TestDetectarNoReconocido(t *testing.T) {
	basura := Grid{{"hola", "mundo"}, {"a", "b", "c"}}
	if _, err := Detectar(basura); err == nil {
		t.Error("un archivo desconocido debe devolver ErrNoReconocido")
	}
}
