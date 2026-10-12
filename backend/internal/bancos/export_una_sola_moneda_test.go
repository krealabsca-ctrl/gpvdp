package bancos

// La exportación va ENTERA en colones (9-oct-2026).
//
// «Eso nos ayudará que los cuadres calcen, porque el Excel los que estaba en dólares los estaba
// tomando sin la conversión.» Hasta ahora las columnas Débito y Crédito iban en la moneda de cada
// cuenta, así que quien sumaba la columna en Excel mezclaba colones con dólares y el total salía
// corto —sin que nada lo avisara—. El reporte se veía bien y no cuadraba.
//
// Lo que se prueba acá es justamente esa suma: que sumar la columna del .xlsx dé el mismo número
// que la columna «Equivalencia». Probar la función sola no alcanza, porque el defecto no estaba en
// el cálculo sino en lo que llegaba a la hoja.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// movsDeDosMonedas: una cuenta en colones y una en dólares, más el caso sin tipo de cambio.
func movsDeDosMonedas() []MovimientoExport {
	return []MovimientoExport{
		{
			Fecha: "2026-09-01", Documento: "7861378", Moneda: "CRC",
			Credito: "26110.62", MontoCRC: "26110.62",
			Descripcion: "INTERNET TRANS BCR", Banco: "BCR", Cuenta: "BCR Colones",
		},
		{
			// 469,75 dólares a 452,93 = 212.763,87 colones.
			Fecha: "2026-09-08", Documento: "DAV-1", Moneda: "USD",
			Credito: "469.75", MontoCRC: "212763.87",
			Descripcion: "TRF DE CR89010409142415535328", Banco: "Davivienda", Cuenta: "Davivienda Dólares",
		},
		{
			// Un débito en dólares, para que el lado del débito también se convierta.
			Fecha: "2026-09-09", Documento: "DAV-2", Moneda: "USD",
			Debito: "26438.80", MontoCRC: "11974925.68",
			Descripcion: "DR/CR LINEA SINPE", Banco: "Davivienda", Cuenta: "Davivienda Dólares",
		},
		{
			// SIN tipo de cambio del mes: se queda en dólares y no suma.
			Fecha: "2026-09-10", Documento: "BAC-9", Moneda: "USD",
			Credito: "1000.00", MontoCRC: "0",
			Descripcion: "TEF DE 955642863", Banco: "BAC", Cuenta: "BAC Dolares",
		},
	}
}

// El cuadre: sumar la columna Crédito del archivo tiene que dar lo mismo que sumar Equivalencia.
//
// Es la prueba que define la función. Antes, el crédito en dólares entraba como «469,75» y el de
// colones como «26.110,62»: sumarlos daba 26.580,37 en vez de 238.874,49.
func TestSumarLaColumnaDelExcelDaElTotalEnColones(t *testing.T) {
	hoja := hojaDetalleCorrido(MetaReporte{Titulo: "Detalle", Empresa: "Valle de Paz"}, movsDeDosMonedas())
	// Se suman los DOS lados contra Equivalencia: un movimiento tiene débito o crédito, nunca los
	// dos, así que el equivalente de la hoja es la suma de ambas columnas.
	var movidos, equivalencias float64
	var creditos float64
	for _, f := range hoja.Filas {
		// 2 = Débito, 3 = Crédito, 4 = Equivalencia (ver columnasDetalle).
		// El que no tiene tipo de cambio queda EN DÓLARES a propósito: no entra en ninguna suma.
		if _, sinTC := f.Valores[3].(MontoSinConvertir); sinTC {
			continue
		}
		d, _ := comoFloat(f.Valores[2])
		c, _ := comoFloat(f.Valores[3])
		e, _ := comoFloat(f.Valores[4])
		movidos += d + c
		creditos += c
		equivalencias += e
	}
	if movidos != equivalencias {
		t.Fatalf("Débito+Crédito suma %.2f y Equivalencia %.2f: el cuadre en Excel no calza",
			movidos, equivalencias)
	}
	// Y los números concretos: créditos = 26.110,62 (colones) + 212.763,87 (469,75 dólares
	// convertidos); el débito convertido aporta los 11.974.925,68 restantes.
	if creditos != 238874.49 {
		t.Errorf("los créditos suman %.2f, se esperaban 238874.49", creditos)
	}
	if equivalencias != 12213800.17 {
		t.Errorf("las equivalencias suman %.2f, se esperaban 12213800.17", equivalencias)
	}
}

// Un monto en dólares sale CONVERTIDO y marcado; uno en colones, intacto y sin marca.
func TestSoloLoQueVinoEnOtraMonedaSeMarca(t *testing.T) {
	hoja := hojaDetalleCorrido(MetaReporte{Titulo: "Detalle"}, movsDeDosMonedas())

	// Fila 0: colones. Ni se convierte ni se marca.
	if _, marcado := hoja.Filas[0].Valores[3].(MontoConvertido); marcado {
		t.Error("un movimiento en colones salió marcado como convertido")
	}
	if c, _ := comoFloat(hoja.Filas[0].Valores[3]); c != 26110.62 {
		t.Errorf("el crédito en colones cambió a %.2f", c)
	}

	// Fila 1: dólares con tipo de cambio → el número es el de colones, y va marcado.
	conv, ok := hoja.Filas[1].Valores[3].(MontoConvertido)
	if !ok {
		t.Fatalf("el crédito en dólares no quedó marcado como convertido (%T)", hoja.Filas[1].Valores[3])
	}
	if float64(conv) != 212763.87 {
		t.Errorf("el crédito convertido es %.2f, se esperaba 212763.87 (469,75 × 452,93)", float64(conv))
	}

	// Fila 2: el DÉBITO en dólares también se convierte, y el crédito queda en cero.
	if d, _ := comoFloat(hoja.Filas[2].Valores[2]); d != 11974925.68 {
		t.Errorf("el débito convertido es %.2f, se esperaba 11974925.68", d)
	}
	if c, _ := comoFloat(hoja.Filas[2].Valores[3]); c != 0 {
		t.Errorf("un movimiento con débito trajo también crédito (%.2f)", c)
	}

	// Fila 3: sin tipo de cambio → se queda en DÓLARES. Poner cero perdería el dato y convertirlo
	// con un tipo inventado sería peor.
	sin, ok := hoja.Filas[3].Valores[3].(MontoSinConvertir)
	if !ok {
		t.Fatalf("el movimiento sin tipo de cambio no quedó marcado (%T)", hoja.Filas[3].Valores[3])
	}
	if float64(sin) != 1000 {
		t.Errorf("el monto sin convertir es %.2f, se esperaban los 1000 dólares originales", float64(sin))
	}
}

// El archivo de verdad: que el número que Excel lee en la celda sea el convertido.
//
// Entre la fila y la celda hay un paso más —`escribirCelda`— y es justo donde el valor podría
// perderse. Este proyecto ya vio un dato calcularse bien y morir en el último tramo.
func TestElNumeroLlegaConvertidoAlArchivo(t *testing.T) {
	hoja := hojaDetalleCorrido(MetaReporte{Titulo: "Detalle", Empresa: "Valle de Paz"}, movsDeDosMonedas())
	buf, err := ConstruirLibro([]HojaReporte{hoja})
	if err != nil {
		t.Fatalf("ConstruirLibro: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("abrir el libro: %v", err)
	}
	defer func() { _ = f.Close() }()

	// RawCellValue: el valor SIN el formato de la celda. Con el formato puesto, «212763.87» se lee
	// «212,763.87» y la prueba estaría midiendo el separador de miles en vez del número.
	filas, err := f.GetRows(f.GetSheetList()[0], excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	// Se busca por el documento para no atarse al número de filas del encabezado.
	buscar := func(doc string) []string {
		t.Helper()
		for _, fila := range filas {
			if len(fila) > 1 && fila[1] == doc {
				return fila
			}
		}
		t.Fatalf("no se encontró la fila del documento %q", doc)
		return nil
	}
	credito := func(doc string) string { return buscar(doc)[3] }

	if got := credito("DAV-1"); got != "212763.87" {
		t.Errorf("en el archivo, el crédito de DAV-1 es %q y se esperaba 212763.87: "+
			"el valor se convirtió en la fila y se perdió al escribir la celda", got)
	}
	if got := credito("7861378"); got != "26110.62" {
		t.Errorf("el crédito en colones llegó como %q", got)
	}
	if got := credito("BAC-9"); got != "1000" {
		t.Errorf("el que no tiene tipo de cambio llegó como %q, se esperaban los 1000 dólares", got)
	}
}

// La nota que decía «Débito y Crédito van en la moneda de cada cuenta» tiene que desaparecer:
// dejó de ser cierta, y una nota que miente sobre los números es peor que no tener nota.
func TestYaNoDiceQueLosMontosVanEnSuMoneda(t *testing.T) {
	hoja := hojaDetalleCorrido(MetaReporte{
		Titulo: "Detalle",
		Avisos: []string{"1 movimiento(s) por USD 1,000.00 no tienen tipo de cambio del mes"},
	}, movsDeDosMonedas())
	buf, err := ConstruirLibro([]HojaReporte{hoja})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	filas, err := f.GetRows(f.GetSheetList()[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, fila := range filas {
		for _, c := range fila {
			if strings.Contains(c, "moneda de cada cuenta") {
				t.Fatalf("el archivo todavía dice %q", c)
			}
		}
	}
}
