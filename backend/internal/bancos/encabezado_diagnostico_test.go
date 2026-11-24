package bancos

// El encabezado que no se reconoce tiene que DECIR QUÉ ENCONTRÓ (21-oct-2026).
//
// EL CASO DE NEGOCIO, no el técnico: el sistema se va a vender, y un cliente nuevo va a subir el
// archivo de su banco. Si ese banco escribe «F. Movimiento» donde los siete conocidos escriben
// «Fecha», la respuesta vieja era una frase fija que no distinguía ese caso de haber elegido la
// hoja equivocada, de un archivo vacío o de filas de título de más.
//
// La diferencia práctica es entre «alguien mira el mensaje y ve en dos minutos qué sinónimo
// agregar» y «hay que pedir el archivo, abrirlo y leer código».

import (
	"errors"
	"strings"
	"testing"
)

// EL BANCO NUEVO: ningún encabezado de la tabla de sinónimos.
func TestUnBancoDesconocidoDiceQueEncontro(t *testing.T) {
	g := Grid{
		{"BANCO IMAGINARIO S.A.", "", ""},
		{"Estado de cuenta – Junio 2026", "", ""},
		{"F. Movimiento", "Detalle de la transacción", "Monto DB", "Monto CR"},
		{"01/06/2026", "PAGO PROVEEDOR", "15000", ""},
	}

	_, err := LeerHistorico(g, g, false)
	if err == nil {
		t.Fatal("se leyó un archivo cuyo encabezado no se reconoce")
	}
	var enc *ErrorDeEncabezado
	if !errors.As(err, &enc) {
		t.Fatalf("el error no es un diagnóstico de encabezado: %T", err)
	}

	msg := err.Error()
	// Lo que hacía falta: ya lo decía antes.
	if !strings.Contains(msg, "fecha") {
		t.Error("el mensaje no nombra lo que hacía falta")
	}
	// Lo NUEVO y lo que importa: qué traía el archivo.
	for _, esperado := range []string{"F. Movimiento", "Monto DB", "Detalle de la transacción"} {
		if !strings.Contains(msg, esperado) {
			t.Errorf("el mensaje no muestra el encabezado real del archivo (falta %q): sin eso nadie puede saber qué sinónimo agregar", esperado)
		}
	}
	// Y el número de fila, para poder ir a mirarla.
	if !strings.Contains(msg, "fila 3") {
		t.Error("el mensaje no dice en qué fila estaba el encabezado que no entendió")
	}
}

// Una fila donde SÍ se entendió algo tiene que decirlo: «entendí la fecha, me faltó el monto»
// manda a mirar la columna del monto; «no se reconoció el encabezado» no manda a ningún lado.
func TestDiceQueColumnasSiEntendio(t *testing.T) {
	g := Grid{
		{"Fecha", "Descripción", "Importe"}, // fecha y descripción sí; el monto no
		{"01/06/2026", "PAGO", "15000"},
	}

	_, err := LeerHistorico(g, g, false)
	var enc *ErrorDeEncabezado
	if !errors.As(err, &enc) {
		t.Fatalf("esperaba un diagnóstico de encabezado, obtuve %T", err)
	}
	if len(enc.Vistas) == 0 || len(enc.Vistas[0].Reconocidas) == 0 {
		t.Fatal("no se informó ninguna columna reconocida, habiendo entendido «Fecha»")
	}
	msg := err.Error()
	if !strings.Contains(msg, "se entendió") {
		t.Error("el mensaje no dice qué columnas sí se entendieron")
	}
	if !strings.Contains(msg, "fecha") {
		t.Error("no se reporta «fecha» como entendida, y sí se entendió")
	}
}

// El archivo vacío es un caso DISTINTO y tiene que decirse distinto: con la frase fija se veía
// igual que un encabezado desconocido, y el arreglo es otro.
func TestElArchivoVacioSeDistingue(t *testing.T) {
	_, err := LeerHistorico(Grid{{"", ""}, {"", ""}}, Grid{{"", ""}, {"", ""}}, false)
	var enc *ErrorDeEncabezado
	if !errors.As(err, &enc) {
		t.Fatalf("esperaba un diagnóstico, obtuve %T", err)
	}
	if !strings.Contains(err.Error(), "ninguna fila con contenido") {
		t.Errorf("un archivo vacío no se distingue de un encabezado desconocido: %q", err.Error())
	}
}

// Las columnas vacías de la derecha no se muestran: una fila de Excel trae decenas y taparían lo
// que importa justo en el mensaje que existe para mostrarlo.
func TestNoSeMuestranLasColumnasVaciasDeLaDerecha(t *testing.T) {
	g := Grid{{"F. Mov", "Detalle", "", "", "", "", "", ""}}
	enc := DiagnosticarEncabezado(g, "", []string{"una columna de fecha"})
	if len(enc.Vistas) != 1 {
		t.Fatalf("esperaba 1 fila vista, obtuve %d", len(enc.Vistas))
	}
	if n := len(enc.Vistas[0].Celdas); n != 2 {
		t.Errorf("se muestran %d celdas en vez de 2: las vacías de la derecha tapan el encabezado real", n)
	}
}

// El lector de clasificación usa el MISMO diagnóstico, con sus propios requisitos: su trabajo es
// pintar la partida sobre movimientos que ya existen, así que lo que le falta es otra cosa.
func TestElLectorDeClasificacionTambienDiagnostica(t *testing.T) {
	g := Grid{{"Columna A", "Columna B"}, {"x", "y"}}
	_, err := LeerClasifExcel(g)
	var enc *ErrorDeEncabezado
	if !errors.As(err, &enc) {
		t.Fatalf("esperaba un diagnóstico, obtuve %T", err)
	}
	if !strings.Contains(err.Error(), "clasificación") {
		t.Errorf("no nombra la partida, que es lo que este lector necesita: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "Columna A") {
		t.Error("no muestra el encabezado real del archivo")
	}
}
