package bancos

// El LECTOR de «Cargar histórico», probado contra .xlsx construidos de verdad con excelize.
//
// No alcanza con probarlo sobre una grilla de strings: el defecto que rompió el intento del
// Director Financiero está EN EL ARCHIVO —una celda que Excel guarda como fecha se lee formateada
// como «07-03-26»— y una grilla escrita a mano nunca lo reproduce. Por eso estas pruebas arman el
// libro con los formatos numéricos más comunes (NumFmt 14, NumFmt 22 y un dd/mm/yyyy personalizado)
// y lo vuelven a abrir por el mismo camino que usa el servicio.

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// libroPrueba arma un .xlsx en memoria. Un valor `time.Time` se escribe como FECHA DE VERDAD (la
// celda queda numérica con formato de fecha), que es justo el caso difícil.
type libroPrueba struct {
	hoja string
	// otrasHojas se crean vacías, para probar el aviso de «el libro tiene N hojas».
	otrasHojas []string
	filas      [][]any
	// numFmt: formato numérico incorporado para las celdas de fecha (14 = dd/mm/yy corto,
	// 22 = dd/mm/yy hh:mm). Cero = el que excelize ponga solo.
	numFmt int
	// numFmtPersonal: formato personalizado, p. ej. «dd/mm/yyyy». Gana sobre numFmt.
	numFmtPersonal string
	// date1904 guarda el libro con el sistema de fechas de 1904 (el viejo default de Excel para
	// Mac, y una casilla de Opciones). Los seriales quedan 1.462 días corridos.
	date1904 bool
}

func (l libroPrueba) bytes(t *testing.T) []byte {
	t.Helper()
	hoja := l.hoja
	if hoja == "" {
		hoja = hojaMovimientos
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	idx, err := f.NewSheet(hoja)
	if err != nil {
		t.Fatalf("crear hoja: %v", err)
	}
	f.SetActiveSheet(idx)
	if hoja != "Sheet1" {
		f.DeleteSheet("Sheet1")
	}
	for _, otra := range l.otrasHojas {
		if _, err := f.NewSheet(otra); err != nil {
			t.Fatalf("crear hoja %s: %v", otra, err)
		}
	}
	if l.date1904 {
		// Se pone ANTES de escribir las celdas: excelize convierte cada `time.Time` a serial con la
		// época del libro, igual que Excel.
		si := true
		if err := f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &si}); err != nil {
			t.Fatalf("poner el sistema de fechas 1904: %v", err)
		}
	}

	estiloFecha := 0
	if l.numFmtPersonal != "" {
		estiloFecha, err = f.NewStyle(&excelize.Style{CustomNumFmt: &l.numFmtPersonal})
		if err != nil {
			t.Fatalf("estilo de fecha personalizado: %v", err)
		}
	} else if l.numFmt != 0 {
		estiloFecha, err = f.NewStyle(&excelize.Style{NumFmt: l.numFmt})
		if err != nil {
			t.Fatalf("estilo de fecha: %v", err)
		}
	}

	for i, fila := range l.filas {
		for j, v := range fila {
			if v == nil {
				continue
			}
			celda, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatalf("coordenada: %v", err)
			}
			if err := f.SetCellValue(hoja, celda, v); err != nil {
				t.Fatalf("escribir %s: %v", celda, err)
			}
			if _, esFecha := v.(time.Time); esFecha && estiloFecha != 0 {
				if err := f.SetCellStyle(hoja, celda, celda, estiloFecha); err != nil {
					t.Fatalf("aplicar formato de fecha: %v", err)
				}
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("serializar libro: %v", err)
	}
	return buf.Bytes()
}

// leerLibro recorre el MISMO camino que el servicio: abrir, sacar las dos grillas y leer.
func leerLibro(t *testing.T, archivo []byte) LecturaHistorico {
	t.Helper()
	formateada, cruda, hoja, hojas, date1904, err := gridDeArchivoHistorico(archivo, hojaMovimientos)
	if err != nil {
		t.Fatalf("abrir el libro: %v", err)
	}
	lec, err := LeerHistorico(formateada, cruda, date1904)
	if err != nil {
		t.Fatalf("leer el libro: %v", err)
	}
	lec.Hoja, lec.Hojas = hoja, hojas
	return lec
}

// encabezadoExport son las columnas EXACTAS que escribe el reporte del sistema
// (service_export.go, columnasDetalle). Es el archivo que el Director usa de plantilla.
var encabezadoExport = []any{
	"Fecha", "Consecutivo", "Débito", "Crédito", "Equivalencia", "Descripción",
	"Banco Cuenta", "Concepto", "Clasificación", "Consecutivo largo",
}

// filaExport arma una fila de movimiento con la forma del reporte.
func filaExport(fecha any, doc string, debito, credito any, descripcion, cuenta, concepto, clasif string) []any {
	return []any{fecha, doc, debito, credito, nil, descripcion, cuenta, concepto, clasif, ""}
}

// EL caso que rompió el intento del Director Financiero.
//
// Una celda que Excel guarda como FECHA de verdad se lee FORMATEADA como «07-03-26», y el lector
// viejo (`fechaDeCelda`) no la entiende: su layout «02/01/2006» exige dos dígitos y ni siquiera
// sabe si el 07 es el día o el mes. Peor: cuando la fecha fallaba, la fila se saltaba ANTES de leer
// los montos, así que además salían en blanco — de ahí el «no reconoció las partidas NI los montos».
func TestUnaFechaDeExcelDeVerdadSeLeeBien(t *testing.T) {
	fecha := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre   string
		libro    libroPrueba
		esperada string
	}{
		{
			nombre: "formato corto incorporado (NumFmt 14)",
			libro: libroPrueba{numFmt: 14, filas: [][]any{
				{"Movimientos de Valle de Paz"}, {"del 01/01/2025 al 31/12/2026"},
				encabezadoExport,
				filaExport(fecha, "0001", nil, 125000.5, "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
			}},
			esperada: "2025-03-07",
		},
		{
			nombre: "fecha con hora (NumFmt 22)",
			libro: libroPrueba{numFmt: 22, filas: [][]any{
				encabezadoExport,
				filaExport(fecha, "0001", nil, 125000.5, "DEPOSITO", "Promerica VDP", "", ""),
			}},
			esperada: "2025-03-07",
		},
		{
			nombre: "formato personalizado dd/mm/yyyy",
			libro: libroPrueba{numFmtPersonal: "dd/mm/yyyy", filas: [][]any{
				encabezadoExport,
				filaExport(fecha, "0001", nil, 125000.5, "DEPOSITO", "Promerica VDP", "", ""),
			}},
			esperada: "2025-03-07",
		},
		{
			nombre: "sin formato: excelize le pone el suyo",
			libro: libroPrueba{filas: [][]any{
				encabezadoExport,
				filaExport(fecha, "0001", nil, 125000.5, "DEPOSITO", "Promerica VDP", "", ""),
			}},
			esperada: "2025-03-07",
		},
		{
			nombre: "escrita a mano con día de un dígito",
			libro: libroPrueba{filas: [][]any{
				encabezadoExport,
				filaExport("7/3/2025", "0001", nil, "125.000,50", "DEPOSITO", "Promerica VDP", "", ""),
			}},
			esperada: "2025-03-07",
		},
		{
			nombre: "escrita con el mes en letras",
			libro: libroPrueba{filas: [][]any{
				encabezadoExport,
				filaExport("7-mar-2025", "0001", nil, "125000.50", "DEPOSITO", "Promerica VDP", "", ""),
			}},
			esperada: "2025-03-07",
		},
		{
			nombre: "ISO",
			libro: libroPrueba{filas: [][]any{
				encabezadoExport,
				filaExport("2025-03-07", "0001", nil, "125000.50", "DEPOSITO", "Promerica VDP", "", ""),
			}},
			esperada: "2025-03-07",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			lec := leerLibro(t, c.libro.bytes(t))
			if len(lec.Errores) > 0 {
				t.Fatalf("la fila salió con error: %+v", lec.Errores)
			}
			if len(lec.Filas) != 1 {
				t.Fatalf("filas leídas = %d, se esperaba 1", len(lec.Filas))
			}
			got := lec.Filas[0]
			if got.Fecha.Format("2006-01-02") != c.esperada {
				t.Errorf("fecha = %s, se esperaba %s", got.Fecha.Format("2006-01-02"), c.esperada)
			}
			// Y lo otro que se perdía: cuando la fecha fallaba, el monto salía en blanco.
			if !igualDecimal(t, got.Credito.String(), "125000.50") {
				t.Errorf("crédito = %s, se esperaba 125000.50", got.Credito)
			}
			if got.Cuenta != "Promerica VDP" {
				t.Errorf("cuenta = %q, se esperaba «Promerica VDP»", got.Cuenta)
			}
		})
	}
}

// El negativo contable de Excel. `parseMontoTolerante` borra los paréntesis, así que «(1.500,00)»
// le sale +1500 EN SILENCIO. En un importador que CREA movimientos eso mete plata con el signo
// cambiado, y no se nota hasta que no cuadra el mes.
func TestElParentesisEsNegativoYNoEntraComoPositivo(t *testing.T) {
	libro := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("07/03/2025", "0001", "(1.500,00)", nil, "AJUSTE", "Promerica VDP", "", ""),
	}}
	lec := leerLibro(t, libro.bytes(t))

	if len(lec.Filas) != 0 {
		t.Fatalf("la fila entró como movimiento (%+v): «(1.500,00)» es NEGATIVO, no +1500", lec.Filas)
	}
	if len(lec.Errores) != 1 {
		t.Fatalf("errores = %d, se esperaba 1", len(lec.Errores))
	}
	e := lec.Errores[0]
	if !strings.Contains(e.Motivo, "negativo") {
		t.Errorf("el motivo debería decir que el monto es negativo; dice %q", e.Motivo)
	}
	// El texto original viaja: sin él hay que abrir el Excel y contar filas para saber qué pasó.
	if !strings.Contains(e.Texto, "1.500,00") {
		t.Errorf("el error no muestra lo que decía la celda: %q", e.Texto)
	}

	// Y el control: el mismo monto en positivo SÍ entra, con su valor.
	ok := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("07/03/2025", "0001", "1.500,00", nil, "AJUSTE", "Promerica VDP", "", ""),
	}}
	lec2 := leerLibro(t, ok.bytes(t))
	if len(lec2.Filas) != 1 || !igualDecimal(t, lec2.Filas[0].Debito.String(), "1500.00") {
		t.Fatalf("el positivo no entró bien: %+v %+v", lec2.Filas, lec2.Errores)
	}
}

// Las filas de adorno del reporte NO son errores. Si se reportaran como tales, el usuario vería
// media docena de errores falsos en CADA archivo que el propio sistema le exportó.
func TestLaDecoracionDelReporteNoCuentaComoError(t *testing.T) {
	libro := libroPrueba{filas: [][]any{
		{"Movimientos"},
		{"Valle de Paz · del 01/01/2025 al 30/04/2025"},
		encabezadoExport,
		// Banda de partida (el reporte la escribe combinada de A a J).
		{"Ingresos › Depósito de Clientes"},
		filaExport("07/03/2025", "0001", nil, "125000.50", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
		filaExport("08/03/2025", "0002", nil, "75000.00", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
		{"Subtotal Ingresos › Depósito de Clientes", nil, 0.0, 200000.5, 200000.5},
		{},
		// La banda de lo que no tiene partida, y sus filas con «Sin clasificar» en las dos columnas.
		{"Sin clasificar"},
		filaExport("09/03/2025", "0003", "50000.00", nil, "PAGO", "Promerica VDP", "Sin clasificar", "Sin clasificar"),
		{"Subtotal Sin clasificar", nil, 50000.0, 0.0, 50000.0},
		{},
		{"TOTAL · 3 movimiento(s)", nil, 50000.0, 200000.5, 250000.5},
		{},
		{"Generado por GPVDP ERP · Valle de Paz · 24/09/2026 10:12"},
	}}
	lec := leerLibro(t, libro.bytes(t))

	if len(lec.Errores) != 0 {
		t.Fatalf("la decoración salió como error: %+v", lec.Errores)
	}
	if len(lec.Filas) != 3 {
		t.Fatalf("movimientos = %d, se esperaban 3 — %+v", len(lec.Filas), lec.Filas)
	}
	// Dos bandas, dos subtotales, el total y el pie.
	if lec.LineasDeFormato != 6 {
		t.Errorf("líneas de formato = %d, se esperaban 6 (2 bandas, 2 subtotales, el total y el pie)",
			lec.LineasDeFormato)
	}
	// «Sin clasificar» es la forma en que el reporte dice SIN PARTIDA. Leerlo como el nombre de una
	// partida haría que el archivo del propio sistema reportara «no existe esa clasificación».
	if lec.Filas[2].Concepto != "" || lec.Filas[2].Clasificacion != "" {
		t.Errorf("«Sin clasificar» se leyó como una partida: concepto=%q clasificación=%q",
			lec.Filas[2].Concepto, lec.Filas[2].Clasificacion)
	}
}

// El encabezado del reporte NO está en la fila 1 (van dos filas de título antes), y las columnas se
// llaman como las llama el negocio: «Consecutivo» es el documento y «Banco Cuenta» es la cuenta.
func TestElEncabezadoDelExportSeEntiendeAunqueNoEsteEnLaFila1(t *testing.T) {
	libro := libroPrueba{filas: [][]any{
		{"Movimientos"},
		{"Valle de Paz"},
		encabezadoExport,
		filaExport("07/03/2025", "REF-77", nil, "125000.50", "DEPOSITO",
			"Banco Popular · BP Negocios", "Ingresos", "Depósito de Clientes"),
	}}
	lec := leerLibro(t, libro.bytes(t))
	if len(lec.Filas) != 1 {
		t.Fatalf("filas = %d (%+v), errores %+v", len(lec.Filas), lec.Filas, lec.Errores)
	}
	f := lec.Filas[0]
	if f.Documento != "REF-77" {
		t.Errorf("«Consecutivo» no se leyó como documento: %q", f.Documento)
	}
	if f.Cuenta != "Banco Popular · BP Negocios" {
		t.Errorf("«Banco Cuenta» no se leyó como cuenta: %q", f.Cuenta)
	}
	if f.Descripcion != "DEPOSITO" {
		t.Errorf("descripción = %q", f.Descripcion)
	}
	if f.Linea != 4 {
		t.Errorf("línea = %d, se esperaba 4 (la del archivo, para poder buscarla en Excel)", f.Linea)
	}
}

// Un archivo sin encabezado reconocible se RECHAZA diciendo qué se esperaba, en vez de devolver
// cero filas y un resumen que parece exitoso.
func TestUnArchivoSinEncabezadoSeRechaza(t *testing.T) {
	libro := libroPrueba{filas: [][]any{
		{"cualquier cosa", "otra"},
		{"1", "2"},
	}}
	formateada, cruda, _, _, date1904, err := gridDeArchivoHistorico(libro.bytes(t), hojaMovimientos)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	if _, err := LeerHistorico(formateada, cruda, date1904); err != ErrHistoricoSinEncabezado {
		t.Fatalf("error = %v, se esperaba ErrHistoricoSinEncabezado", err)
	}
}

// LA ÉPOCA DEL LIBRO. Un .xlsx guardado con el sistema de fechas 1904 —el viejo default de Excel
// para Mac, y una casilla que cualquiera puede marcar en Opciones— cuenta los días desde
// 1904-01-01. La celda SE VE bien en Excel («07/03/2025») pero el serial que guarda es 44261, y
// leído con la época de 1900 da 2021-03-06: el movimiento entra CUATRO AÑOS antes, en otro
// ejercicio, sin un solo aviso.
//
// Es el mismo daño que la mig «la fecha va en la clave» ya nos costó una vez con DateStyle=MDY: la
// fecha equivocada no rompe nada, solo pone la plata en el año que no es.
func TestUnLibroConElSistemaDeFechas1904NoEntraCuatroAniosCorrido(t *testing.T) {
	fecha := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)
	libro := libroPrueba{date1904: true, numFmtPersonal: "dd/mm/yyyy", filas: [][]any{
		encabezadoExport,
		filaExport(fecha, "0001", nil, 999.99, "DEPOSITO", "Promerica VDP", "", ""),
	}}

	lec := leerLibro(t, libro.bytes(t))
	if len(lec.Errores) > 0 {
		t.Fatalf("la fila salió con error: %+v", lec.Errores)
	}
	if len(lec.Filas) != 1 {
		t.Fatalf("filas leídas = %d, se esperaba 1", len(lec.Filas))
	}
	if got := lec.Filas[0].Fecha.Format("2006-01-02"); got != "2025-03-07" {
		t.Fatalf("fecha = %s, se esperaba 2025-03-07 (antes de leer la época del libro salía 2021-03-06)", got)
	}

	// Y el MISMO libro en la época normal tiene que seguir dando lo mismo: el arreglo lee la
	// propiedad del archivo, no cambia el default.
	normal := libroPrueba{numFmtPersonal: "dd/mm/yyyy", filas: libro.filas}
	lecNormal := leerLibro(t, normal.bytes(t))
	if len(lecNormal.Filas) != 1 || lecNormal.Filas[0].Fecha.Format("2006-01-02") != "2025-03-07" {
		t.Fatalf("libro 1900: %+v", lecNormal.Filas)
	}
}

// LO QUE SE MUESTRA TIENE QUE SER LO QUE SE ESCRIBE. Los libros guardan numeric(16,2): si el
// lector no redondea, Postgres redondea al insertar y el resumen que el usuario aprobó deja de
// cuadrar con la base. Medido contra la API real: tres filas de 1,005 daban «total 3,02» en la
// previsualización y 3,03 en la tabla, y dos filas de 0,004 entraban como movimientos de ¢0,00.
func TestElMontoSeRedondeaALaPrecisionDeLosLibros(t *testing.T) {
	libro := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("10/01/2025", "DEC-001", 1.005, nil, "TRES DECIMALES", "BAC Religiosa", "", ""),
		filaExport("10/01/2025", "DEC-002", 1.005, nil, "TRES DECIMALES", "BAC Religiosa", "", ""),
		filaExport("10/01/2025", "DEC-003", 1.005, nil, "TRES DECIMALES", "BAC Religiosa", "", ""),
		filaExport("10/01/2025", "DEC-004", 0.004, nil, "MILESIMA", "BAC Religiosa", "", ""),
	}}
	lec := leerLibro(t, libro.bytes(t))

	// Las tres de 1,005 entran redondeadas, como va a quedar la fila en la base.
	if len(lec.Filas) != 3 {
		t.Fatalf("filas = %d, se esperaban 3 (la de 0,004 no es un movimiento): %+v", len(lec.Filas), lec.Filas)
	}
	suma := decimal.Zero
	for _, f := range lec.Filas {
		if !igualDecimal(t, f.Debito.String(), "1.01") {
			t.Errorf("débito = %s, se esperaba 1.01 (la precisión de los libros)", f.Debito)
		}
		suma = suma.Add(f.Debito)
	}
	if suma.StringFixed(2) != "3.03" {
		t.Errorf("suma = %s, se esperaba 3.03 (que es lo que queda en la base)", suma.StringFixed(2))
	}

	// La que se redondea a cero NO entra callada como un movimiento de ¢0: sale como lo que es.
	if len(lec.Errores) != 1 {
		t.Fatalf("errores = %+v, se esperaba 1 (la fila de 0,004)", lec.Errores)
	}
	if !strings.Contains(lec.Errores[0].Motivo, "no trae monto") {
		t.Errorf("motivo = %q, se esperaba que dijera que no trae monto", lec.Errores[0].Motivo)
	}
}

// UN MONTO CON CEROS DE MÁS NO PUEDE TIRAR EL ARCHIVO ENTERO. `numeric(16,2)` aguanta catorce
// enteros; más que eso hacía fallar el INSERT, la transacción se iba completa y el usuario recibía
// «error interno» sin una sola línea que mirar. Medido contra la API real: `confirmar` devolvía 500
// y no quedaba cargado nada de las ocho mil filas buenas.
func TestUnMontoQueNoEntraEnLosLibrosSeRechazaPorFilaYNoTiraElArchivo(t *testing.T) {
	libro := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("11/01/2025", "BIG-001", "99999999999999999.99", nil, "CEROS DE MAS", "BAC Religiosa", "", ""),
		filaExport("11/01/2025", "OK-001", 250, nil, "NORMAL", "BAC Religiosa", "", ""),
	}}
	lec := leerLibro(t, libro.bytes(t))

	if len(lec.Filas) != 1 || lec.Filas[0].Documento != "OK-001" {
		t.Fatalf("filas = %+v, se esperaba solo OK-001 (la buena sigue entrando)", lec.Filas)
	}
	if len(lec.Errores) != 1 {
		t.Fatalf("errores = %+v, se esperaba 1", lec.Errores)
	}
	if !strings.Contains(lec.Errores[0].Motivo, "no entra en los libros") {
		t.Errorf("motivo = %q, tiene que decir que el monto no entra", lec.Errores[0].Motivo)
	}
	if !strings.Contains(lec.Errores[0].Texto, "99999999999999999.99") {
		t.Errorf("texto = %q, tiene que mostrar lo que decía la celda", lec.Errores[0].Texto)
	}
	// El tope se nombra en el mensaje: sin el número, «no entra» no dice qué corregir.
	if !strings.Contains(lec.Errores[0].Motivo, "99999999999999.99") {
		t.Errorf("motivo = %q, tiene que nombrar el máximo", lec.Errores[0].Motivo)
	}
}
