package cxp

// Una macro por moneda, y el neto que no mezcla (9-oct-2026).
//
// «Las facturas que entran en dólares se están convirtiendo en colones, y eso está mal: en la
// operativa diaria las facturas de dólares se pagan en dólares. Hay que hacer macros para colones
// y macros para dólares sin perder la integridad del mismo.»

import (
	"os"
	"strings"
	"testing"
)

func filasDeDosMonedas() []PagoRow {
	return []PagoRow{
		{Nombre: "CNFL", Moneda: "CRC", MontoNeto: "1400000.00", Consecutivo: "001", IBAN: "CR01"},
		{Nombre: "AWS", Moneda: "USD", MontoNeto: "1200.50", Consecutivo: "002", IBAN: "CR02"},
		{Nombre: "ICE", Moneda: "CRC", MontoNeto: "250000.00", Consecutivo: "003", IBAN: "CR03"},
		{Nombre: "Microsoft", Moneda: "USD", MontoNeto: "800.00", Consecutivo: "004", IBAN: "CR04"},
	}
}

// Cada moneda va a su propio archivo, y el total de cada uno está EN SU MONEDA.
func TestLaMacroSeSeparaPorMoneda(t *testing.T) {
	grupos := SepararPorMoneda(filasDeDosMonedas(), 43)
	if len(grupos) != 2 {
		t.Fatalf("salieron %d archivos, se esperaban 2 (uno por moneda)", len(grupos))
	}
	// Colones primero: es la moneda del 100 % de la operación hoy.
	if grupos[0].Moneda != "CRC" {
		t.Errorf("el primer archivo es de %s; se esperaba que colones fuera primero", grupos[0].Moneda)
	}
	if len(grupos[0].Lineas) != 2 || grupos[0].Total != "1650000.00" {
		t.Errorf("colones: %d líneas por %s, se esperaban 2 por 1650000.00",
			len(grupos[0].Lineas), grupos[0].Total)
	}
	if len(grupos[1].Lineas) != 2 || grupos[1].Total != "2000.50" {
		t.Errorf("dólares: %d líneas por %s, se esperaban 2 por 2000.50",
			len(grupos[1].Lineas), grupos[1].Total)
	}
	// NO hay un total general: sumar colones con dólares da un número que no significa nada, y
	// mostrarlo invitaría a cuadrarlo contra algo.
}

// El nombre del archivo lleva la moneda: los dos caen en la misma carpeta de descargas, y dos
// «macro-pagos.txt» son una trampa — se sube el de colones creyendo que es el de dólares.
func TestCadaArchivoSeLlamaDistinto(t *testing.T) {
	grupos := SepararPorMoneda(filasDeDosMonedas(), 43)
	if grupos[0].Archivo == grupos[1].Archivo {
		t.Fatalf("los dos archivos se llaman igual (%q)", grupos[0].Archivo)
	}
	if grupos[0].Archivo != "macro-lote-0043-CRC.txt" {
		t.Errorf("el archivo de colones se llama %q", grupos[0].Archivo)
	}
	if grupos[1].Archivo != "macro-lote-0043-USD.txt" {
		t.Errorf("el archivo de dólares se llama %q", grupos[1].Archivo)
	}
}

// Una línea sin moneda declarada es de colones: así nacieron las 4.542 facturas que existen, y
// tratarla como desconocida la dejaría fuera de los dos archivos.
func TestSinMonedaSeTratCaComoColones(t *testing.T) {
	grupos := SepararPorMoneda([]PagoRow{
		{Nombre: "Vieja", Moneda: "", MontoNeto: "5000.00"},
		{Nombre: "Nueva", Moneda: "CRC", MontoNeto: "3000.00"},
	}, 1)
	if len(grupos) != 1 {
		t.Fatalf("salieron %d archivos, se esperaba 1", len(grupos))
	}
	if len(grupos[0].Lineas) != 2 || grupos[0].Total != "8000.00" {
		t.Errorf("la línea sin moneda quedó fuera: %d líneas por %s", len(grupos[0].Lineas), grupos[0].Total)
	}
}

// Un lote de una sola moneda da un solo archivo: no se inventa uno vacío para la otra.
func TestUnLoteDeUnaSolaMonedaDaUnArchivo(t *testing.T) {
	grupos := SepararPorMoneda([]PagoRow{
		{Moneda: "CRC", MontoNeto: "100.00"},
		{Moneda: "CRC", MontoNeto: "200.00"},
	}, 7)
	if len(grupos) != 1 {
		t.Fatalf("salieron %d archivos, se esperaba 1", len(grupos))
	}
}

// Sin líneas no hay archivos. «No hay nada que pagar» y «hay un archivo sin líneas» son cosas
// distintas, y la segunda es la que se sube al banco sin que nadie lo note.
func TestSinLineasNoHayArchivo(t *testing.T) {
	if grupos := SepararPorMoneda(nil, 1); len(grupos) != 0 {
		t.Fatalf("sin líneas salieron %d archivos", len(grupos))
	}
}

// LineasDeMoneda es lo que baja el endpoint: devuelve solo las de esa moneda, y nil cuando no hay
// —para poder contestar «este lote no tiene nada en dólares» en vez de entregar un .txt vacío.
func TestLineasDeMonedaFiltraYAvisaCuandoNoHay(t *testing.T) {
	rows := filasDeDosMonedas()
	if n := len(LineasDeMoneda(rows, "USD")); n != 2 {
		t.Errorf("USD trajo %d líneas, se esperaban 2", n)
	}
	if n := len(LineasDeMoneda(rows, "crc")); n != 2 {
		t.Errorf("la moneda en minúscula trajo %d líneas", n)
	}
	if l := LineasDeMoneda(rows, "EUR"); l != nil {
		t.Errorf("una moneda que el lote no tiene devolvió %d líneas en vez de nil", len(l))
	}
}

// El neto a pagar NO puede restarle colones a una factura en dólares.
//
// EL RIESGO NO CAMBIÓ: `d.total` está en la moneda de la factura y `aa.monto_crc` guarda un
// importe; a una factura de USD 1.000 con un anticipo de ₡100.000 le quedaría «1000 − 100000» y el
// GREATEST la dejaría en CERO. El proveedor recibiría una transferencia por nada.
//
// LO QUE CAMBIÓ ES DÓNDE SE IMPIDE (20-oct-2026, hallazgo 3.8). Antes la guarda estaba en el SQL
// —`AND d.moneda = 'CRC'`— y evitaba el error peor al precio de otro: una factura en dólares con
// anticipo aplicado **se pagaba completa**, ignorando lo ya entregado. Ahora la guarda está en
// `AplicarAnticipo`, que exige que el anticipo y la factura estén en la MISMA moneda, así que todo
// lo que llega a `anticipo_aplicacion` es comparable con el total de su factura por construcción
// y el SQL puede restar sin condición.
//
// Por eso esta prueba mira el servicio y ya no el SQL: si alguien quita esa comparación, el neto
// vuelve a poder mezclar monedas y nada más lo atraparía.
func TestElNetoNoMezclaMonedas(t *testing.T) {
	src, err := os.ReadFile("service_anticipo.go")
	if err != nil {
		t.Fatalf("leer service_anticipo.go: %v", err)
	}
	fuente := string(src)

	// Las DOS puertas: la aplicación individual y la del lote. Que una sola compare no alcanza —el
	// lote netea varios anticipos de una vez y es el camino que más se usa.
	if !strings.Contains(fuente, "factura.Moneda != anticipo.Moneda") {
		t.Error("AplicarAnticipo dejó de comparar la moneda de la factura con la del anticipo: " +
			"el neto puede volver a restar colones a una factura en dólares y dejarla en cero")
	}
	if !strings.Contains(fuente, "anticipo.Moneda != factura.Moneda") {
		t.Error("AplicarAnticiposLote dejó de comparar monedas: el camino del lote netea varios " +
			"anticipos de una vez y es el que más se usa")
	}
	// Y el SQL NO debe volver a traer la condición vieja: con ella, una factura en dólares se paga
	// completa aunque tenga anticipo aplicado, que es el defecto que esto vino a arreglar.
	if strings.Contains(netoAPagarSQL, "d.moneda = 'CRC'") {
		t.Error("volvió el `AND d.moneda = 'CRC'` al neto: una factura en dólares con anticipo " +
			"aplicado se pagaría completa, pagando dos veces lo ya entregado")
	}
}
