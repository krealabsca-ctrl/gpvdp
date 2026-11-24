package cxp

// El cuadro de pago que se manda por correo (4-nov-2026).
//
// Lo que se prueba acá es la construcción del cuadro a partir de las filas que van al banco: la
// agrupación, los totales por moneda y de dónde sale cada texto. Es matemática pura y no toca la
// base: el .xlsx se prueba aparte abriendo el archivo.

import (
	"testing"
)

func ptr(s string) *string { return &s }

// filasDeUnCorte: dos proveedores en colones —uno con dos facturas— y uno en dólares.
func filasDeUnCorte() []PagoRow {
	return []PagoRow{
		{ProveedorID: "p1", Nombre: "ZoAart Artesanías", Cedula: "3-102-897077",
			IBAN: "CR46015107820010459999", Moneda: "CRC", MontoNeto: "137300.00", Consecutivo: "87"},
		{ProveedorID: "p1", Nombre: "ZoAart Artesanías", Cedula: "3-102-897077",
			IBAN: "CR46015107820010459999", Moneda: "CRC", MontoNeto: "100000.00", Consecutivo: "88"},
		{ProveedorID: "p2", Nombre: "Gas Tomza De Costa Rica SA", Cedula: "3-101-349880",
			IBAN: "CR63015100210010133038", Moneda: "CRC", MontoNeto: "216174.09", Consecutivo: "22674"},
		{ProveedorID: "p3", Nombre: "Ati Capital Solutions S.A.", Cedula: "3-101-770011",
			IBAN: "CR11000000000000000001", Moneda: "USD", MontoNeto: "5289.28", Consecutivo: "A-1"},
	}
}

// UNA FILA POR PROVEEDOR, CON SUS DOCUMENTOS JUNTOS. Es la forma del cuadro que se manda hoy.
func TestElAvisoAgrupaPorProveedorYSumaSusFacturas(t *testing.T) {
	monedas := ConstruirAviso(filasDeUnCorte(), nil, nil)
	if len(monedas) != 2 {
		t.Fatalf("monedas = %d, quería 2 (colones y dólares)", len(monedas))
	}
	crc := monedas[0]
	if crc.Moneda != "CRC" {
		t.Fatalf("la primera moneda es %q: colones va primero, es casi toda la operación", crc.Moneda)
	}
	if len(crc.Filas) != 2 {
		t.Fatalf("filas en colones = %d, quería 2 proveedores", len(crc.Filas))
	}
	zoaart := crc.Filas[0]
	if zoaart.Monto != "237300.00" {
		t.Errorf("el monto de ZoAart es %q: tiene que ser la SUMA de sus dos facturas (237300.00)", zoaart.Monto)
	}
	// El número que se cuadra contra el banco no es la cantidad de filas.
	if zoaart.Transferencias != 2 {
		t.Errorf("transferencias de ZoAart = %d, quería 2", zoaart.Transferencias)
	}
	if crc.Transferencias != 3 {
		t.Errorf("transferencias en colones = %d, quería 3 sobre 2 filas: sin este número, "+
			"un aviso de 2 líneas se leería como 2 movimientos", crc.Transferencias)
	}
	if crc.Total != "453474.09" {
		t.Errorf("total en colones = %q, quería 453474.09", crc.Total)
	}
}

// LAS MONEDAS NO SE SUMAN ENTRE SÍ. Cada una es un cuadro y un archivo.
func TestElAvisoNoMezclaMonedas(t *testing.T) {
	monedas := ConstruirAviso(filasDeUnCorte(), nil, nil)
	usd := monedas[1]
	if usd.Moneda != "USD" {
		t.Fatalf("la segunda moneda es %q", usd.Moneda)
	}
	if usd.Total != "5289.28" {
		t.Errorf("total en dólares = %q, quería 5289.28 (en dólares, no colonizado)", usd.Total)
	}
	if len(usd.Filas) != 1 {
		t.Fatalf("filas en dólares = %d", len(usd.Filas))
	}
	if usd.Filas[0].Monto != "5289.28" {
		t.Errorf("el monto en dólares quedó en %q: el cuadro de dólares muestra dólares", usd.Filas[0].Monto)
	}
}

// EL MISMO PROVEEDOR EN DOS MONEDAS SON DOS LÍNEAS, en dos archivos distintos. Juntarlas daría un
// monto que no es plata y que el banco no podría transferir.
func TestUnProveedorEnDosMonedasSonDosFilas(t *testing.T) {
	rows := []PagoRow{
		{ProveedorID: "p1", Nombre: "Dual S.A.", Moneda: "CRC", MontoNeto: "1000.00", IBAN: "CR1"},
		{ProveedorID: "p1", Nombre: "Dual S.A.", Moneda: "USD", MontoNeto: "25.00", IBAN: "CR1"},
	}
	monedas := ConstruirAviso(rows, nil, nil)
	if len(monedas) != 2 {
		t.Fatalf("monedas = %d, quería 2", len(monedas))
	}
	for _, m := range monedas {
		if len(m.Filas) != 1 {
			t.Errorf("en %s quedaron %d filas, quería 1", m.Moneda, len(m.Filas))
		}
	}
}

// LA MONEDA VACÍA ES COLONES. Así nacieron las facturas que existen; tratarla como desconocida
// las sacaría del cuadro sin que nadie lo note.
func TestLaMonedaVaciaEsColones(t *testing.T) {
	rows := []PagoRow{{ProveedorID: "p1", Nombre: "Sin moneda", Moneda: "", MontoNeto: "500.00", IBAN: "CR1"}}
	monedas := ConstruirAviso(rows, nil, nil)
	if len(monedas) != 1 || monedas[0].Moneda != "CRC" {
		t.Fatalf("la moneda vacía no cayó en colones: %+v", monedas)
	}
}

// EL TEXTO: se propone, y lo escrito manda.
func TestElTextoEscritoLePasaPorEncimaALaPropuesta(t *testing.T) {
	datos := map[string]DatosDelAviso{
		"p1": {ConceptoPropuesto: "Urnas", Consecutivos: []string{"87", "88"}},
	}
	rows := []PagoRow{{ProveedorID: "p1", Nombre: "ZoAart", Moneda: "CRC", MontoNeto: "10.00", IBAN: "CR1"}}

	// Sin texto escrito: manda la propuesta.
	sinTexto := ConstruirAviso(rows, datos, nil)[0].Filas[0]
	if sinTexto.Concepto != "Urnas" {
		t.Errorf("concepto propuesto = %q, quería «Urnas»", sinTexto.Concepto)
	}
	if sinTexto.Documentos != "FC 87 88" {
		t.Errorf("documentos propuestos = %q, quería «FC 87 88»", sinTexto.Documentos)
	}
	if sinTexto.Editado {
		t.Error("se marcó como editado algo que nadie escribió")
	}

	// Con texto escrito: manda lo escrito, y se nota que alguien lo tocó.
	textos := map[string]TextoAviso{"p1": {
		Concepto:   ptr("Urnas Personalizadas"),
		Documentos: ptr("FC 87 88 ORS 2743 URNAS ZOAART"),
	}}
	conTexto := ConstruirAviso(rows, datos, textos)[0].Filas[0]
	if conTexto.Concepto != "Urnas Personalizadas" || conTexto.Documentos != "FC 87 88 ORS 2743 URNAS ZOAART" {
		t.Errorf("no ganó el texto escrito: %q / %q", conTexto.Concepto, conTexto.Documentos)
	}
	if !conTexto.Editado {
		t.Error("no se marcó como editado: la pantalla pisaría la corrección con la propuesta")
	}
}

// BORRAR A PROPÓSITO NO ES «NO TOCAR». Con cadenas pelados las dos cosas serían "" y el campo
// vaciado volvería a llenarse solo en la siguiente descarga.
func TestUnTextoVaciadoAPropositoNoVuelveALaPropuesta(t *testing.T) {
	datos := map[string]DatosDelAviso{"p1": {ConceptoPropuesto: "Urnas"}}
	rows := []PagoRow{{ProveedorID: "p1", Nombre: "ZoAart", Moneda: "CRC", MontoNeto: "10.00", IBAN: "CR1"}}
	textos := map[string]TextoAviso{"p1": {Concepto: ptr("")}}

	f := ConstruirAviso(rows, datos, textos)[0].Filas[0]
	if f.Concepto != "" {
		t.Errorf("el concepto borrado volvió como %q", f.Concepto)
	}
	if !f.Editado {
		t.Error("borrar es editar: sin la marca, la pantalla lo repondría")
	}
}

// EL PROVEEDOR SIN CUENTA SE DELATA. El banco rechaza esa línea, y el aviso no puede mandarse
// como si estuviera todo bien.
func TestElProveedorSinCuentaQuedaMarcado(t *testing.T) {
	rows := []PagoRow{
		{ProveedorID: "p1", Nombre: "Con cuenta", Moneda: "CRC", MontoNeto: "10.00", IBAN: "CR1"},
		{ProveedorID: "p2", Nombre: "Sin cuenta", Moneda: "CRC", MontoNeto: "20.00", IBAN: ""},
	}
	m := ConstruirAviso(rows, nil, nil)[0]
	if m.SinCuenta != 1 {
		t.Fatalf("sin cuenta = %d, quería 1", m.SinCuenta)
	}
	if m.Filas[0].SinCuenta {
		t.Error("se marcó al que sí tiene cuenta")
	}
	if !m.Filas[1].SinCuenta {
		t.Error("no se marcó al que no tiene cuenta: el aviso saldría como si el banco fuera a pagarle")
	}
}

// UN CORTE SIN NADA NO DEVUELVE UN CUADRO VACÍO. «No hay nada que pagar» y «hay un cuadro sin
// líneas» son cosas distintas, y la segunda es la que se manda por correo sin que nadie lo note.
func TestUnCorteSinFilasNoDevuelveCuadro(t *testing.T) {
	if m := ConstruirAviso(nil, nil, nil); m != nil {
		t.Errorf("devolvió %+v para un corte vacío", m)
	}
}

// Los consecutivos repetidos no se repiten en la glosa: en el modo agrupado la misma factura
// puede llegar por dos caminos y «FC 87 87 88» se lee como un error de quien armó el cuadro.
func TestLaGlosaNoRepiteConsecutivos(t *testing.T) {
	datos := map[string]DatosDelAviso{"p1": {Consecutivos: []string{"87", "87", "88", ""}}}
	rows := []PagoRow{{ProveedorID: "p1", Nombre: "X", Moneda: "CRC", MontoNeto: "1.00", IBAN: "CR1"}}
	if got := ConstruirAviso(rows, datos, nil)[0].Filas[0].Documentos; got != "FC 87 88" {
		t.Errorf("documentos = %q, quería «FC 87 88»", got)
	}
}
