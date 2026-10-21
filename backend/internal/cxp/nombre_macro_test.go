package cxp

// El nombre del archivo de la macro (16-oct-2026).
//
// EL PEDIDO: «sería bueno agregar una huella a las macros si es de colones o dólares al nombre de
// la macro». La MONEDA ya estaba —el archivo bajaba como «macro-pagos-CRC.txt»—, pero el NÚMERO
// DEL LOTE no: `NombreMacro` existía desde la mig 0095, lo contemplaba, y nadie la llamaba.
//
// Sin el número, dos cortes del mismo día bajan con el mismo nombre; el navegador los deja como
// «macro-pagos-CRC.txt» y «macro-pagos-CRC (1).txt», y cuál es cuál pasa a ser una adivinanza
// frente a un archivo que mueve plata. Es la misma trampa que ya existía entre colones y dólares,
// un escalón más arriba.

import "testing"

func TestElNombreDeLaMacroLlevaElLoteYLaMoneda(t *testing.T) {
	t.Parallel()
	casos := []struct {
		lote   int64
		moneda string
		quiere string
	}{
		{43, "CRC", "macro-lote-0043-CRC.txt"},
		{43, "USD", "macro-lote-0043-USD.txt"},
		// El lote va con cuatro dígitos para que ordenen bien en la carpeta de descargas.
		{7, "CRC", "macro-lote-0007-CRC.txt"},
		{1234, "USD", "macro-lote-1234-USD.txt"},
		// Más de cuatro dígitos no se recorta: el nombre crece antes que mentir sobre el lote.
		{12345, "CRC", "macro-lote-12345-CRC.txt"},
		// La moneda se normaliza: dos archivos del mismo lote no pueden diferir solo en la caja.
		{43, "usd", "macro-lote-0043-USD.txt"},
	}
	for _, c := range casos {
		if got := NombreMacro(c.lote, c.moneda); got != c.quiere {
			t.Errorf("NombreMacro(%d, %q) = %q, se esperaba %q", c.lote, c.moneda, got, c.quiere)
		}
	}
}

// Dos cortes distintos NO pueden bajar con el mismo nombre: es todo el punto.
func TestDosLotesDistintosNoCompartenNombreDeArchivo(t *testing.T) {
	t.Parallel()
	if NombreMacro(43, "CRC") == NombreMacro(44, "CRC") {
		t.Error("dos lotes distintos bajan con el mismo nombre: el segundo se guarda como «(1)»")
	}
	// Y las dos monedas del MISMO lote tampoco, que es el caso que ya estaba resuelto.
	if NombreMacro(43, "CRC") == NombreMacro(43, "USD") {
		t.Error("las dos monedas del mismo lote comparten nombre")
	}
}

// Cada grupo de moneda trae el nombre del archivo que de verdad se va a bajar. La pantalla lo
// anuncia antes de bajarlo: si dijera otro, el aviso sería peor que no darlo.
func TestCadaGrupoDeMonedaTraeElNombreRealDeSuArchivo(t *testing.T) {
	t.Parallel()
	grupos := SepararPorMoneda([]PagoRow{
		{Moneda: "CRC", MontoNeto: "100000.00"},
		{Moneda: "USD", MontoNeto: "384.20"},
	}, 64)
	if len(grupos) != 2 {
		t.Fatalf("grupos = %d, se esperaban 2", len(grupos))
	}
	if grupos[0].Archivo != "macro-lote-0064-CRC.txt" {
		t.Errorf("archivo de colones = %q", grupos[0].Archivo)
	}
	if grupos[1].Archivo != "macro-lote-0064-USD.txt" {
		t.Errorf("archivo de dólares = %q", grupos[1].Archivo)
	}
	// Y el total de cada grupo va en SU moneda, no convertido: es lo que el banco va a debitar.
	if grupos[1].Total != "384.20" {
		t.Errorf("total en dólares = %q, se esperaba 384.20", grupos[1].Total)
	}
}
