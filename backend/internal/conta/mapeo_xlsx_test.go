package conta

// El .xlsx del mapeo, verificado ABRIENDO el archivo generado y volviéndolo a leer.
//
// LO QUE SE PROTEGE ES LA IDA Y VUELTA. El archivo sale del sistema, lo edita una persona en Excel
// —que reordena columnas, las esconde, pega valores, deja espacios— y vuelve. Si la lectura
// dependiera de la POSICIÓN de las columnas, una hoja reordenada aplicaría cada cuenta a la
// partida equivocada sin un solo error: el mapeo quedaría mal y nadie lo notaría hasta ver un
// asiento absurdo meses después.
//
// Y que la columna que hay que llenar salga VACÍA. Medido contra el plan real, una sugerencia por
// nombre acierta 6 de 131 y propone hasta 9 candidatos de clases contables distintas; si el
// exportador la escribiera, alguien la subiría tal cual.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func filasDeMuestra() []FilaDelMapeo {
	return []FilaDelMapeo{
		{
			ClasificacionID: "c1", Concepto: "Control", Clasificacion: "Traslado de Fondos",
			NaturalezaConcepto: "NEUTRO", NaturalezaDeclarada: true,
			Movimientos: 239, MontoCRC: "1115438033.00",
		},
		{
			ClasificacionID: "c2", Concepto: "Gastos", Clasificacion: "Combustible",
			NaturalezaConcepto: "GASTO", NaturalezaDeclarada: true,
			Movimientos: 374, MontoCRC: "9040298.00",
			Estado: EstadoMapeada, CuentaCodigo: "6-01-01-030-000",
		},
		{
			// Sin naturaleza declarada: el default de la columna es NEUTRO, así que escribir
			// «neutro» haría creer que alguien lo eligió.
			ClasificacionID: "c3", Concepto: "Otros", Clasificacion: "Panaderia",
			NaturalezaConcepto: "NEUTRO", NaturalezaDeclarada: false,
			Movimientos: 2, MontoCRC: "957000.00",
		},
	}
}

func cuentasDeMuestra() []Cuenta {
	return []Cuenta{
		{ID: "a1", Codigo: "6-01-01-030-000", Nombre: "Combustible", Clase: "GASTO",
			AceptaMovimiento: true, Activo: true},
		{ID: "a2", Codigo: "6-01-01-000-000", Nombre: "Gastos de Operación", Clase: "GASTO",
			AceptaMovimiento: false, Activo: true}, // agrupación: NO debe ofrecerse
		{ID: "a3", Codigo: "4-01-03-000-000", Nombre: "Alquiler de Capillas", Clase: "INGRESO",
			AceptaMovimiento: true, Activo: true},
	}
}

func abrirMapeo(t *testing.T) (*excelize.File, []byte) {
	t.Helper()
	datos, err := ConstruirMapeoXLSX(filasDeMuestra(), cuentasDeMuestra())
	if err != nil {
		t.Fatalf("construir el mapeo: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(datos))
	if err != nil {
		t.Fatalf("abrir el mapeo: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f, datos
}

// LA COLUMNA QUE SE LLENA SALE VACÍA en lo que todavía no se decidió.
func TestElArchivoNoSugiereCuentas(t *testing.T) {
	f, _ := abrirMapeo(t)
	filas, err := f.GetRows(hojaMapeo)
	if err != nil {
		t.Fatalf("leer filas: %v", err)
	}
	col := map[string]int{}
	for i, c := range filas[0] {
		col[normalizarTitulo(c)] = i
	}
	iCuenta := col["codigo de cuenta"]

	for _, fila := range filas[1:] {
		if len(fila) == 0 {
			continue
		}
		partida := fila[1]
		valor := celdaLimpia(fila, iCuenta)
		// «Combustible» ya está mapeada, así que SÍ trae su código: el archivo también sirve para
		// revisar lo hecho. Las otras dos tienen que venir en blanco.
		if partida == "Combustible" {
			if valor != "6-01-01-030-000" {
				t.Errorf("la partida ya mapeada salió con %q; se esperaba su cuenta", valor)
			}
			continue
		}
		if valor != "" {
			t.Errorf("la partida %q salió con la cuenta %q propuesta; el archivo no debe sugerir", partida, valor)
		}
	}
}

// El plan viaja al lado, y SIN las cuentas de agrupación.
func TestElArchivoTraeElPlanSinLasDeAgrupacion(t *testing.T) {
	f, _ := abrirMapeo(t)
	filas, err := f.GetRows(hojaPlan)
	if err != nil {
		t.Fatalf("leer el plan: %v", err)
	}
	var codigos []string
	for _, fila := range filas[1:] {
		if len(fila) > 0 && fila[0] != "" {
			codigos = append(codigos, fila[0])
		}
	}
	if len(codigos) != 2 {
		t.Fatalf("el plan trae %d cuentas (%v); se esperaban las 2 que aceptan movimiento", len(codigos), codigos)
	}
	for _, c := range codigos {
		if c == "6-01-01-000-000" {
			t.Error("se ofreció una cuenta de agrupación: la base la va a rechazar, " +
				"así que ofrecerla solo produce intentos que fallan")
		}
	}
}

// «Sin declarar» no es «neutro»: son dos cosas distintas y el archivo tiene que distinguirlas.
func TestElArchivoDistingueSinDeclararDeNeutro(t *testing.T) {
	f, _ := abrirMapeo(t)
	filas, err := f.GetRows(hojaMapeo)
	if err != nil {
		t.Fatalf("leer filas: %v", err)
	}
	natural := map[string]string{}
	for _, fila := range filas[1:] {
		if len(fila) >= 5 {
			natural[fila[1]] = fila[4]
		}
	}
	if natural["Traslado de Fondos"] != "neutro" {
		t.Errorf("la declarada NEUTRO salió %q", natural["Traslado de Fondos"])
	}
	if natural["Panaderia"] != "sin declarar" {
		t.Errorf("la que nadie declaró salió %q; se esperaba «sin declarar»", natural["Panaderia"])
	}
}

// LA IDA Y VUELTA: lo exportado se vuelve a leer, con las columnas REORDENADAS.
func TestLaLecturaNoDependeDelOrdenDeLasColumnas(t *testing.T) {
	// Se arma a mano una hoja con las columnas al revés, que es lo que hace quien trabaja en Excel.
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName("Sheet1", hojaMapeo)
	filas := [][]string{
		{"Motivo si no se contabiliza", "Código de cuenta", "Partida", "Colones", "Concepto"},
		{"", "6-01-01-030-000", "Combustible", "9040298", "Gastos"},
		{"es un traslado entre cuentas propias", "", "Traslado de Fondos", "1115438033", "Control"},
	}
	for i, fila := range filas {
		for j, v := range fila {
			celda, _ := excelize.CoordinatesToCellName(j+1, i+1)
			_ = f.SetCellValue(hojaMapeo, celda, v)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}

	leidas, err := LeerMapeoDesdeExcel(buf.Bytes())
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(leidas) != 2 {
		t.Fatalf("filas leídas = %d, se esperaban 2", len(leidas))
	}
	if leidas[0].Clasificacion != "Combustible" || leidas[0].CuentaCodigo != "6-01-01-030-000" {
		t.Errorf("primera fila mal leída: %+v. Las columnas reordenadas se leyeron por posición: "+
			"cada cuenta habría quedado en la partida equivocada", leidas[0])
	}
	if leidas[0].Concepto != "Gastos" {
		t.Errorf("concepto = %q, se esperaba «Gastos»", leidas[0].Concepto)
	}
	if leidas[1].Clasificacion != "Traslado de Fondos" || leidas[1].Motivo == "" {
		t.Errorf("segunda fila mal leída: %+v", leidas[1])
	}
}

// Una fila SIN cuenta y SIN motivo no es una decisión: se salta, no se aplica como vacía.
func TestLaFilaSinDecisionNoBorraLoYaHecho(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName("Sheet1", hojaMapeo)
	filas := [][]string{
		{"Concepto", "Partida", "Código de cuenta", "Motivo si no se contabiliza"},
		{"Gastos", "Combustible", "6-01-01-030-000", ""},
		{"Gastos", "Flores", "", ""},             // nadie la tocó
		{"", "", "", ""},                         // fila en blanco de relleno
		{"Control", "Traslado", "", "es propio"}, // decidida por motivo
	}
	for i, fila := range filas {
		for j, v := range fila {
			celda, _ := excelize.CoordinatesToCellName(j+1, i+1)
			_ = f.SetCellValue(hojaMapeo, celda, v)
		}
	}
	buf, _ := f.WriteToBuffer()

	leidas, err := LeerMapeoDesdeExcel(buf.Bytes())
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(leidas) != 2 {
		t.Fatalf("filas = %d; se esperaban 2 (la vacía y la de relleno se saltan). Leídas: %+v",
			len(leidas), leidas)
	}
	for _, l := range leidas {
		if l.Clasificacion == "Flores" {
			t.Error("se leyó la partida que nadie tocó: aplicarla borraría un mapeo ya hecho")
		}
	}
	// El número de fila es el del EXCEL, que es lo que la persona ve para corregir.
	if leidas[0].Fila != 2 {
		t.Errorf("la primera dice fila %d; en el Excel es la 2", leidas[0].Fila)
	}
	if leidas[1].Fila != 5 {
		t.Errorf("la última dice fila %d; en el Excel es la 5", leidas[1].Fila)
	}
}

// Un archivo sin las columnas que identifican la fila se rechaza nombrando el problema.
func TestElArchivoSinEncabezadoSeRechaza(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName("Sheet1", hojaMapeo)
	_ = f.SetCellValue(hojaMapeo, "A1", "Cuenta")
	_ = f.SetCellValue(hojaMapeo, "A2", "6-01")
	buf, _ := f.WriteToBuffer()

	if _, err := LeerMapeoDesdeExcel(buf.Bytes()); err != ErrMapeoSinEncabezado {
		t.Errorf("err = %v, se esperaba ErrMapeoSinEncabezado", err)
	}
	if _, err := LeerMapeoDesdeExcel([]byte("esto no es un xlsx")); err != ErrMapeoArchivoIlegible {
		t.Errorf("con basura err = %v, se esperaba ErrMapeoArchivoIlegible", err)
	}
}

// El encabezado con tildes, mayúsculas o espacios de más tiene que calzar igual.
func TestElEncabezadoSeLeeAunqueVengaDistinto(t *testing.T) {
	for _, titulo := range []string{"Código de cuenta", "CODIGO DE CUENTA", "  codigo  de cuenta  "} {
		if normalizarTitulo(titulo) != "codigo de cuenta" {
			t.Errorf("%q se normalizó a %q", titulo, normalizarTitulo(titulo))
		}
	}
	if !strings.Contains(normalizarTitulo("Partida"), "partida") {
		t.Error("«Partida» no se normaliza")
	}
}
