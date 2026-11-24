package bancos

// Qué dice el importador cuando el banco no se reconoce (22-oct-2026).
//
// LO QUE DECÍA ANTES, medido subiendo un archivo de verdad al sistema corriendo:
//
//     422 · "formato de banco no reconocido"
//
// Eso era todo. No decía qué había leído, ni en qué fila, ni —desde la migración 0108— que un
// banco nuevo se agrega desde una pantalla. El lugar donde alguien descubre que necesita
// «Formatos de banco» es justamente este rechazo, y el rechazo no la nombraba: el mismo defecto de
// descubribilidad que ya tuvimos con horas extra y vacaciones.
//
// Lo que NO cambia: `errors.Is(err, ErrNoReconocido)` sigue siendo cierto. Lo único distinto es
// cuánto dice el mensaje.

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// libroConHojas arma un .xlsx con varias hojas. `filasPorHoja` va en el orden de las hojas.
func libroConHojas(t *testing.T, hojas []string, filasPorHoja [][][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for i, nombre := range hojas {
		if i == 0 {
			// La primera hoja ya existe («Sheet1»): se renombra para no dejarla vacía al lado.
			if err := f.SetSheetName("Sheet1", nombre); err != nil {
				t.Fatalf("renombrar hoja: %v", err)
			}
		} else if _, err := f.NewSheet(nombre); err != nil {
			t.Fatalf("crear hoja %s: %v", nombre, err)
		}
		for j, fila := range filasPorHoja[i] {
			cel, err := excelize.CoordinatesToCellName(1, j+1)
			if err != nil {
				t.Fatalf("celda: %v", err)
			}
			if err := f.SetSheetRow(nombre, cel, &fila); err != nil {
				t.Fatalf("escribir fila: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("escribir libro: %v", err)
	}
	return buf.Bytes()
}

func TestUnBancoDesconocidoDiceQueSeLeyoYQueHacer(t *testing.T) {
	archivo := libroConHojas(t,
		[]string{"Resumen", "Movimientos"},
		[][][]any{
			{
				{"BANCO LAFISE COSTA RICA"},
				{},
				{"Fecha mov.", "Concepto", "Comprobante", "Haber", "Debe"},
				{"2026-10-01", "DEPOSITO EN EFECTIVO", "991001", "150,000.00", ""},
			},
			{{"esta hoja no se lee"}},
		})

	_, err := parseArchivo(archivo, nil)
	if err == nil {
		t.Fatal("un banco que nadie reconoce tiene que fallar")
	}

	// 1 · Lo de siempre sigue siendo cierto: quien compare con el centinela lo encuentra.
	if !errors.Is(err, ErrNoReconocido) {
		t.Errorf("ya no se reconoce como ErrNoReconocido: %v", err)
	}
	// Y sigue siendo el error tipado que el handler ya sabía traducir a 422 con su texto.
	var diag *ErrorDeEncabezado
	if !errors.As(err, &diag) {
		t.Fatalf("no es un ErrorDeEncabezado, así que el handler lo mandaría como genérico: %v", err)
	}

	msg := err.Error()

	// 2 · QUÉ HOJA se leyó, y cuáles más había. Es la causa más común del rechazo y la única que
	// se descarta sin abrir el Excel.
	if !strings.Contains(msg, "«Resumen»") {
		t.Errorf("no dice qué hoja leyó: %s", msg)
	}
	if !strings.Contains(msg, "«Movimientos»") {
		t.Errorf("no dice que el archivo trae otra hoja; es LA causa más común: %s", msg)
	}

	// 3 · QUÉ ENCONTRÓ, con la fila y las celdas tal como venían: es contra eso que alguien
	// configura las columnas.
	if !strings.Contains(msg, "fila 3") || !strings.Contains(msg, "Fecha mov.") {
		t.Errorf("no muestra la fila de títulos que encontró: %s", msg)
	}
	if !strings.Contains(msg, "Haber") || !strings.Contains(msg, "Debe") {
		t.Errorf("no muestra las columnas de dinero que encontró: %s", msg)
	}

	// 4 · QUÉ HACER. Sin esto el mensaje describe el problema y deja a alguien sin saber a dónde ir.
	if !strings.Contains(msg, "Formatos de banco") {
		t.Errorf("no dice dónde se agrega un banco nuevo: %s", msg)
	}
}

// Con una sola hoja no se habla de hojas: decir «el archivo también trae» una lista vacía es ruido
// en el mensaje justo cuando hace falta leerlo entero.
func TestConUnaSolaHojaNoSeMencionanOtras(t *testing.T) {
	archivo := libroConHojas(t,
		[]string{"Movimientos"},
		[][][]any{{{"Columna rara"}, {"otra cosa"}}})

	_, err := parseArchivo(archivo, nil)
	if err == nil {
		t.Fatal("tenía que fallar")
	}
	if strings.Contains(err.Error(), "también trae") {
		t.Errorf("habla de otras hojas cuando hay una sola: %s", err.Error())
	}
}

// Y un banco QUE SÍ se reconoce no pasa por acá: el diagnóstico no puede estorbar al camino feliz.
func TestUnBancoReconocidoNoProduceDiagnostico(t *testing.T) {
	archivo := libroConHojas(t,
		[]string{"Hoja1"},
		[][][]any{{
			{"TRANSACCIONES POR FECHA"},
			{},
			{"Banco Promerica Costa Rica"},
			{"Fecha", "Documento", "Descripción", "Débitos", "Créditos", "Saldo"},
			{"10-16-26", "17130407", "TRASLADO", "", "361,600.00", "497,292.69"},
		}})

	res, err := parseArchivo(archivo, nil)
	if err != nil {
		t.Fatalf("un archivo de Promerica dejó de leerse: %v", err)
	}
	if res.Banco != BancoPromerica {
		t.Errorf("banco = %q, se esperaba Promerica", res.Banco)
	}
	if len(res.Movimientos) != 1 {
		t.Errorf("movimientos = %d, se esperaba 1", len(res.Movimientos))
	}
}

// Los otros tres lectores (histórico, clasificación, diccionario) no envuelven ningún error de
// dominio: su `Unwrap` devuelve nil y se comportan exactamente como antes. Si un día alguien les
// pusiera `Base`, esta prueba avisa de que ahora los atrapa un `errors.Is` que antes no.
func TestElDiagnosticoSinBaseNoSeConfundeConNingunCentinela(t *testing.T) {
	diag := DiagnosticarEncabezado(Grid{{"una", "fila"}}, "Hoja1", []string{"Fecha"})
	if errors.Is(diag, ErrNoReconocido) {
		t.Error("un diagnóstico sin Base no puede identificarse como ErrNoReconocido")
	}
	if errors.Unwrap(diag) != nil {
		t.Error("sin Base, Unwrap tiene que devolver nil")
	}
}
