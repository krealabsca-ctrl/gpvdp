package cxp

// El .xlsx del aviso, verificado ABRIENDO el archivo generado.
//
// Mirar la pantalla no sirve: lo que importa es el valor y el estilo que quedaron escritos en la
// celda. Las dos cosas que se arruinan solas —el monto escrito como texto y el rango combinado
// estilado a medias— no se ven hasta que alguien intenta sumar la columna o imprime el papel.

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func avisoDeMuestra() (Aviso, AvisoDeMoneda) {
	av := Aviso{LoteID: "l1", LoteNumero: 43, FechaCorte: "2026-11-04"}
	m := AvisoDeMoneda{
		Moneda: "CRC",
		Filas: []FilaAviso{
			{Proveedor: "Daniel Esteban Chaves Arguedas", Identificacion: "01-1715-0521",
				Moneda: "CRC", Concepto: "Urnas Personalizadas",
				Documentos: "FC 07 10 13 14 URNAS ORSM", Monto: "72320.00",
				Cuenta: "CR38010200009432533105", Transferencias: 1},
			{Proveedor: "Gas Tomza De Costa Rica SA", Identificacion: "3-101-349880",
				Moneda: "CRC", Concepto: "Suministro de Gas",
				Documentos: "FC 3020250 22674", Monto: "216174.09",
				Cuenta: "CR63015100210010133038", Transferencias: 1},
		},
		Total: "288494.09", Transferencias: 2,
	}
	av.Monedas = []AvisoDeMoneda{m}
	return av, m
}

func abrirAviso(t *testing.T) *excelize.File {
	t.Helper()
	av, m := avisoDeMuestra()
	datos, err := ConstruirAvisoXLSX(av, m, "Valle de Paz", "Administrador GPVDP", "04/11/2026 15:40")
	if err != nil {
		t.Fatalf("construir el aviso: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(datos))
	if err != nil {
		t.Fatalf("abrir el aviso: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// filaCabeceraDelAviso localiza la fila donde empiezan los títulos de las columnas.
//
// LAS PRUEBAS DE ESTE ARCHIVO NO USAN COORDENADAS FIJAS, y la razón es concreta: estaban escritas
// contra «A1», «E4», «A8»… y el día que el papel ganó su bloque de firmas arriba (11-nov-2026)
// todas señalaron a la celda equivocada. Ninguna estaba mal: lo que cambió fue la maquetación, que
// es exactamente lo que estas pruebas NO deberían fijar. Lo que protegen es el contenido.
func filaCabeceraDelAviso(t *testing.T, f *excelize.File) int {
	t.Helper()
	filas, err := f.GetRows("Aviso")
	if err != nil {
		t.Fatalf("leer filas: %v", err)
	}
	for i, fila := range filas {
		if len(fila) > 0 && fila[0] == "Proveedor" {
			return i + 1 // GetRows es base 0; las celdas, base 1
		}
	}
	t.Fatal("no se encontró la cabecera del cuadro (la fila que empieza con «Proveedor»)")
	return 0
}

// celdaDelAviso devuelve el valor de una celda por columna y fila.
func celdaDelAviso(t *testing.T, f *excelize.File, col string, fila int, opts ...excelize.Options) string {
	t.Helper()
	v, err := f.GetCellValue("Aviso", fmt.Sprintf("%s%d", col, fila), opts...)
	if err != nil {
		t.Fatalf("leer %s%d: %v", col, fila, err)
	}
	return v
}

// EL CUADRO SE LLAMA COMO SE LLAMA HOY. El papel nuevo tiene que reconocerse como el mismo papel.
func TestElAvisoSeTitulaComoElCuadroQueSeMandaHoy(t *testing.T) {
	f := abrirAviso(t)
	// El título va dos filas por encima de la cabecera (en medio está la línea de contexto).
	got := celdaDelAviso(t, f, "A", filaCabeceraDelAviso(t, f)-2)
	if got != "Pago Proveedores Colones" {
		t.Errorf("título = %q, quería «Pago Proveedores Colones»", got)
	}
	if TituloDeMoneda("USD") != "Pago Proveedores Dólares" {
		t.Errorf("el cuadro de dólares se titula %q", TituloDeMoneda("USD"))
	}
}

// LAS SIETE COLUMNAS, EN SU ORDEN. Quien recibe el cuadro lo lee por posición; reordenarlas
// obliga a leerlo de nuevo y rompe cualquier cosa que alguien haya armado encima.
func TestElOrdenDeLasColumnasDelAviso(t *testing.T) {
	f := abrirAviso(t)
	cab := filaCabeceraDelAviso(t, f)
	quiere := []string{"Proveedor", "Moneda", "Concepto", "Documentos", "Monto", "Cuenta Bancaria", "Identificación"}
	for i, q := range quiere {
		celda, _ := excelize.CoordinatesToCellName(i+1, cab)
		got, err := f.GetCellValue("Aviso", celda)
		if err != nil {
			t.Fatal(err)
		}
		if got != q {
			t.Errorf("columna %d = %q, quería %q", i+1, got, q)
		}
	}
}

// EL MONTO ES UN NÚMERO, NO TEXTO. En texto no suma ni ordena, que es lo primero que hace quien
// recibe el cuadro; y una columna numérica sin formato muestra «2,5E+05».
func TestLosMontosDelAvisoSonNumerosConFormato(t *testing.T) {
	f := abrirAviso(t)
	// La primera fila de datos va justo debajo de la cabecera.
	primera := filaCabeceraDelAviso(t, f) + 1
	celdaMonto := fmt.Sprintf("E%d", primera)
	// Un número no lleva atributo `t` en el XML, así que excelize lo reporta como «unset»; lo que
	// delata al monto escrito como texto es que venga como cadena (suelta o compartida).
	tipo, err := f.GetCellType("Aviso", celdaMonto)
	if err != nil {
		t.Fatal(err)
	}
	if tipo == excelize.CellTypeSharedString || tipo == excelize.CellTypeInlineString {
		t.Errorf("el monto quedó como texto (%v): no se puede sumar ni ordenar la columna", tipo)
	}
	crudoE4 := celdaDelAviso(t, f, "E", primera, excelize.Options{RawCellValue: true})
	if _, err := strconv.ParseFloat(crudoE4, 64); err != nil {
		t.Errorf("el monto escrito (%q) no es un número: %v", crudoE4, err)
	}
	// Y lleva formato de miles con dos decimales, o una columna angosta muestra «2,5E+05».
	estilo, err := f.GetCellStyle("Aviso", celdaMonto)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.GetStyle(estilo)
	if err != nil {
		t.Fatal(err)
	}
	if st.CustomNumFmt == nil || *st.CustomNumFmt != "#,##0.00" {
		t.Errorf("el monto no tiene formato de miles: %+v", st.CustomNumFmt)
	}
	// Y el valor es el que corresponde, no uno redondeado por el formato.
	crudo := celdaDelAviso(t, f, "E", primera+1, excelize.Options{RawCellValue: true})
	if crudo != "216174.09" {
		t.Errorf("el monto escrito es %q, quería 216174.09", crudo)
	}
}

// EL TOTAL ESTÁ, Y ES LA SUMA. El cuadro que se manda hoy no lo trae: hay que sumarlo a mano
// justo cuando más importa no equivocarse, porque es lo que se compara contra el banco.
func TestElAvisoLlevaElTotalYEsLaSumaDeSusFilas(t *testing.T) {
	f := abrirAviso(t)
	// El total va justo después de las dos filas de datos de este caso.
	filaTotal := filaCabeceraDelAviso(t, f) + 3
	if rot := celdaDelAviso(t, f, "A", filaTotal); rot != "TOTAL A TRANSFERIR EN COLONES" {
		t.Fatalf("el rótulo del total es %q", rot)
	}
	total := celdaDelAviso(t, f, "E", filaTotal, excelize.Options{RawCellValue: true})
	if total != "288494.09" {
		t.Errorf("total = %q, quería 288494.09 (72320.00 + 216174.09)", total)
	}
}

// LA TRAMPA DEL RANGO COMBINADO: `SetCellStyle` sobre un rango combinado estila SOLO la primera
// celda si se le pasa la misma celda dos veces. El resultado es un borde colgando alrededor de la
// columna A y líneas sueltas en el papel impreso. Se comprueba que todas las celdas del rango
// comparten estilo.
func TestLosRangosCombinadosDelAvisoEstanEstiladosCompletos(t *testing.T) {
	f := abrirAviso(t)
	merges, err := f.GetMergeCells("Aviso")
	if err != nil {
		t.Fatal(err)
	}
	if len(merges) == 0 {
		t.Fatal("el aviso no tiene ningún rango combinado: el título y el total lo están")
	}
	for _, m := range merges {
		inicio := m.GetStartAxis()
		fin := m.GetEndAxis()
		c1, f1, err := excelize.CellNameToCoordinates(inicio)
		if err != nil {
			t.Fatal(err)
		}
		c2, _, err := excelize.CellNameToCoordinates(fin)
		if err != nil {
			t.Fatal(err)
		}
		quiere, err := f.GetCellStyle("Aviso", inicio)
		if err != nil {
			t.Fatal(err)
		}
		for col := c1; col <= c2; col++ {
			celda, _ := excelize.CoordinatesToCellName(col, f1)
			got, err := f.GetCellStyle("Aviso", celda)
			if err != nil {
				t.Fatal(err)
			}
			if got != quiere {
				t.Errorf("en el rango %s:%s la celda %s tiene otro estilo: el borde quedaría "+
					"dibujado solo alrededor de la primera columna", inicio, fin, celda)
			}
		}
	}
}

// EL PROVEEDOR SIN CUENTA LO DICE EN LA CELDA. Dejarla vacía se lee como un descuido de quien
// armó el cuadro, no como la línea que el banco va a rechazar.
func TestElAvisoDiceCuandoFaltaLaCuenta(t *testing.T) {
	av, m := avisoDeMuestra()
	m.Filas[0].Cuenta = ""
	m.Filas[0].SinCuenta = true
	m.SinCuenta = 1
	av.Monedas = []AvisoDeMoneda{m}
	datos, err := ConstruirAvisoXLSX(av, m, "Valle de Paz", "", "")
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	primera := filaCabeceraDelAviso(t, f) + 1
	if got := celdaDelAviso(t, f, "F", primera); got != "SIN CUENTA REGISTRADA" {
		t.Errorf("la celda de la cuenta dice %q sobre un proveedor sin IBAN", got)
	}
	// El pie se busca por CONTENIDO en toda la hoja, no en una celda: lo que importa es que el
	// aviso esté en el papel, no en qué renglón cayó.
	filas, err := f.GetRows("Aviso")
	if err != nil {
		t.Fatal(err)
	}
	hayAviso := false
	for _, fila := range filas {
		for _, celda := range fila {
			if bytes.Contains([]byte(celda), []byte("ATENCIÓN")) {
				hayAviso = true
			}
		}
	}
	if !hayAviso {
		t.Error("el papel no avisa en ninguna parte de la cuenta faltante")
	}
}

// El nombre del archivo lleva el corte Y la moneda: los dos archivos de un mismo corte caen en la
// misma carpeta de descargas, y sin la moneda el segundo baja como «(1)».
func TestElNombreDelAvisoLlevaCorteYMoneda(t *testing.T) {
	if got := NombreAvisoXLSX(43, "CRC"); got != "aviso-pago-corte-0043-CRC.xlsx" {
		t.Errorf("nombre = %q", got)
	}
	if NombreAvisoXLSX(43, "CRC") == NombreAvisoXLSX(43, "USD") {
		t.Error("los dos archivos del mismo corte se llaman igual")
	}
}

// LAS FIRMAS ESTÁN EN EL ARCHIVO, no solo en la pantalla (11-nov-2026).
//
// El .xlsx es lo que se adjunta al correo y lo que alguien imprime y archiva. Si las firmas se
// vieran solo en la pantalla, el papel que queda como respaldo no diría por cuántas manos pasó el
// pago — que es justamente para lo que existe.
func TestElArchivoDelAvisoLlevaLasFirmas(t *testing.T) {
	av, m := avisoDeMuestra()
	av.Firmas = []Firma{
		{Rol: RolSolicitado, Nombre: "Administrador GPVDP", Fecha: "2026-11-11"},
		{Rol: RolRevisado, Nombre: "Fernando Rodríguez / Jossabeth Castro", Fecha: "2026-11-12"},
		{Rol: RolAprobado},
		{Rol: RolPagado},
	}
	av.FechaPago = ""

	datos, err := ConstruirAvisoXLSX(av, m, "Valle de Paz", "Tesorería", "11/11/2026 09:00")
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	todo := ""
	filas, err := f.GetRows("Aviso")
	if err != nil {
		t.Fatal(err)
	}
	for _, fila := range filas {
		todo += strings.Join(fila, "|") + "\n"
	}

	// Las CUATRO etiquetas, aunque dos no tengan nombre: un papel sin la línea de «Aprobado por»
	// se lee como si ese paso no existiera; con la línea en blanco, como que no se hizo.
	for _, rol := range []string{RolSolicitado, RolRevisado, RolAprobado, RolPagado, "Fecha de pago"} {
		if !strings.Contains(todo, rol+":") {
			t.Errorf("el archivo no trae la línea %q", rol)
		}
	}
	if !strings.Contains(todo, "Administrador GPVDP") {
		t.Error("el archivo no dice quién solicitó el pago")
	}
	// Los dos revisores, no uno: poner solo el primero diría que el otro no participó.
	if !strings.Contains(todo, "Fernando Rodríguez / Jossabeth Castro") {
		t.Error("el archivo no trae a los dos revisores")
	}
	// Y el bloque de firmas va ANTES del cuadro: es el encabezado del respaldo.
	if strings.Index(todo, RolSolicitado) > strings.Index(todo, "Proveedor|Moneda") {
		t.Error("las firmas quedaron debajo del cuadro; van arriba, como en el papel que se firma")
	}
}
