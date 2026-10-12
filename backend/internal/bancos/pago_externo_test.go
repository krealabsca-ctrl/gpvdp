package bancos

// El lector del reporte de pagos aplicados del sistema anterior (6-oct-2026).
//
// El reporte sale de OTRO ERP y nadie controla cómo nombra sus columnas ni cómo escribe las fechas,
// así que el lector tiene que ser tolerante en la forma y estricto en el fondo: acepta muchos
// encabezados y formatos, pero NUNCA inventa el dato que no está. Una fila que no se puede cruzar
// se dice; no se adivina.
//
// Lo que estas pruebas protegen es el modo de falla que ya nos costó 23.144 filas en cuarentena:
// un archivo que el lector no entiende y que no falla, sino que entra vacío.

import (
	"errors"
	"strings"
	"testing"
)

func TestElLectorReconoceLosEncabezadosQueUsanLosOtrosSistemas(t *testing.T) {
	// Cada caso es un encabezado distinto para lo MISMO. Si alguno dejara de reconocerse, el campo
	// entraría vacío y la fila no se podría cruzar.
	casos := []struct {
		nombre string
		enc    []string
	}{
		{"nombres llanos", []string{"Referencia", "Fecha", "Monto"}},
		{"con tildes y palabras de más", []string{"Referencia de pago", "Fecha de aplicación", "Monto pagado"}},
		{"en mayúsculas y con dos puntos", []string{"REFERENCIA:", "FECHA DE PAGO:", "IMPORTE:"}},
		{"el formato de cobros de CxC", []string{"Pago referencia", "Fecha de pago bancario", "Valor"}},
		{"vocabulario contable", []string{"No. Comprobante", "Fecha movimiento", "Débito"}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			g := Grid{c.enc, {"REF-001", "15/01/2026", "125000.00"}}
			filas, err := LeerReportePagos(g)
			if err != nil {
				t.Fatalf("leer: %v", err)
			}
			if len(filas) != 1 {
				t.Fatalf("se leyeron %d filas", len(filas))
			}
			f := filas[0]
			if f.Referencia != "REF-001" {
				t.Errorf("referencia = %q", f.Referencia)
			}
			if f.Fecha != "2026-01-15" {
				t.Errorf("fecha = %q, quiere 2026-01-15", f.Fecha)
			}
			if f.MontoTexto != "125000.00" {
				t.Errorf("monto = %q, quiere 125000.00", f.MontoTexto)
			}
			if !f.SePuedeCruzar() {
				t.Errorf("la fila trae los tres datos y dice que no se puede cruzar: %q", f.Motivo)
			}
		})
	}
}

func TestLaFechaSeEntiendeComoLaEscribenLosExportadores(t *testing.T) {
	casos := []struct{ texto, quiere string }{
		{"15/01/2026", "2026-01-15"},
		{"15/1/2026", "2026-01-15"},
		{"2026-01-15", "2026-01-15"},
		{"15-01-2026", "2026-01-15"},
		{"20260115", "2026-01-15"},
		{"2026-01-15 14:30:00", "2026-01-15"}, // con hora: se corta
		{"46037", "2026-01-15"},               // número de serie de Excel
	}
	for _, c := range casos {
		t.Run(c.texto, func(t *testing.T) {
			got, ok := fechaDelReporte(c.texto)
			if !ok {
				t.Fatalf("no se entendió %q", c.texto)
			}
			if got != c.quiere {
				t.Errorf("%q dio %q, quiere %q", c.texto, got, c.quiere)
			}
		})
	}

	// El día 3 de febrero en formato de Costa Rica es 3/2, NO el 2 de marzo. Para los días del 1 al
	// 12 las dos lecturas son válidas y elegir mal corre la fecha sin que nadie lo note: es
	// exactamente lo que ya pasó con las fechas de la clave de Hacienda.
	got, ok := fechaDelReporte("3/2/2026")
	if !ok || got != "2026-02-03" {
		t.Errorf("3/2/2026 dio %q (ok=%v), quiere 2026-02-03: se está leyendo como mes/día", got, ok)
	}
}

func TestUnaFilaQueNoSePuedeCruzarLoDiceEnVezDeAdivinar(t *testing.T) {
	// Solo monto: calzaría con decenas de débitos. Emparejar con un dato solo es inventar.
	g := Grid{
		{"Referencia", "Fecha", "Monto"},
		{"", "", "125000.00"},
		{"REF-002", "15/01/2026", "80000.00"},
	}
	filas, err := LeerReportePagos(g)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("se leyeron %d filas, se esperaban 2", len(filas))
	}
	if filas[0].SePuedeCruzar() {
		t.Error("la fila con SOLO monto se dio por cruzable")
	}
	if filas[0].Motivo == "" {
		t.Error("la fila no cruzable no dice por qué")
	}
	// Y la de al lado no se contagia: una fila mala no arruina el resto del archivo.
	if !filas[1].SePuedeCruzar() {
		t.Errorf("la fila completa quedó marcada: %q", filas[1].Motivo)
	}
}

func TestElSignoDelMontoNoImporta(t *testing.T) {
	// El reporte puede traer los pagos en negativo (salida de caja) y el banco los guarda como
	// débito positivo. Lo que se compara es cuánta plata se movió.
	g := Grid{
		{"Referencia", "Fecha", "Monto"},
		{"REF-001", "15/01/2026", "-125000.00"},
	}
	filas, _ := LeerReportePagos(g)
	if len(filas) != 1 || filas[0].MontoTexto != "125000.00" {
		t.Fatalf("monto = %q, quiere 125000.00 sin signo", filas[0].MontoTexto)
	}
}

func TestElEncabezadoSeBuscaAunqueNoEsteEnLaPrimeraFila(t *testing.T) {
	// Muchos reportes traen título, empresa y rango de fechas antes del encabezado.
	g := Grid{
		{"VALLE DE PAZ S.A."},
		{"Reporte de pagos aplicados"},
		{"Del 01/01/2026 al 31/12/2026"},
		{},
		{"Referencia", "Fecha", "Monto", "Proveedor"},
		{"REF-001", "15/01/2026", "125000.00", "FUNERALES ROCA ETERNA"},
	}
	filas, err := LeerReportePagos(g)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("se leyeron %d filas", len(filas))
	}
	if filas[0].Tercero != "FUNERALES ROCA ETERNA" {
		t.Errorf("tercero = %q", filas[0].Tercero)
	}
}

func TestUnArchivoQueNoSeEntiendeFallaEnVezDeEntrarVacio(t *testing.T) {
	// LA lección de la cartera: 23.144 filas entraron con los campos vacíos porque el lector no
	// reconoció el encabezado y nadie se enteró. Acá tiene que fallar fuerte y temprano.
	casos := []struct {
		nombre string
		g      Grid
		quiere error
	}{
		{"sin nada que se parezca a un encabezado",
			Grid{{"col1", "col2", "col3"}, {"a", "b", "c"}}, ErrReporteSinEncabezado},
		{"encabezado sin ninguna llave para cruzar",
			Grid{{"Proveedor", "Descripción"}, {"X", "Y"}}, ErrReporteSinEncabezado},
		{"archivo vacío", Grid{}, ErrReporteSinFilas},
		{"encabezado y nada debajo",
			Grid{{"Referencia", "Fecha", "Monto"}}, ErrReporteSinFilas},
		{"solo filas en blanco debajo",
			Grid{{"Referencia", "Fecha", "Monto"}, {"", "", ""}, {"", "", ""}}, ErrReporteSinFilas},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := LeerReportePagos(c.g)
			if err != c.quiere {
				t.Errorf("err = %v, quiere %v", err, c.quiere)
			}
		})
	}
}

func TestLasFilasDeTotalNoEntranComoPagos(t *testing.T) {
	g := Grid{
		{"Referencia", "Fecha", "Monto", "Descripción"},
		{"REF-001", "15/01/2026", "125000.00", "pago a proveedor"},
		{"", "", "", "TOTAL GENERAL"},
	}
	filas, err := LeerReportePagos(g)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	// La fila de total no trae referencia, ni fecha, ni monto: no es un pago.
	if len(filas) != 1 {
		t.Errorf("se leyeron %d filas, el total no es un pago: %+v", len(filas), filas)
	}
}

func TestElMontoNoSeMultiplicaPorMil(t *testing.T) {
	// La trampa de siempre: «1,234» son mil doscientos treinta y cuatro y «1,23» es uno con
	// veintitrés. Leerlos igual multiplica un monto por mil, en un archivo que nadie va a recontar.
	casos := []struct{ texto, quiere string }{
		{"1,234", "1234.00"},
		{"1,23", "1.23"},
		{"1.234,56", "1234.56"},
		{"1,234.56", "1234.56"},
		{"125 000,00", "125000.00"},
		{"125000", "125000.00"},
	}
	for _, c := range casos {
		t.Run(c.texto, func(t *testing.T) {
			g := Grid{{"Referencia", "Fecha", "Monto"}, {"R1", "15/01/2026", c.texto}}
			filas, err := LeerReportePagos(g)
			if err != nil {
				t.Fatalf("leer %q: %v", c.texto, err)
			}
			if filas[0].MontoTexto != c.quiere {
				t.Errorf("%q se leyó como %q, quiere %q", c.texto, filas[0].MontoTexto, c.quiere)
			}
		})
	}
}

// El archivo entra venga como venga. Nadie controla cómo exporta el otro ERP, y un 500 por subir el
// CSV que el sistema viejo genera por defecto sería culpar a la persona de una decisión nuestra.
// Esto se descubrió probando contra la API: el primer intento reventaba con «not a valid zip file».
func TestElReporteSeAbreSeaExcelOTextoSeparado(t *testing.T) {
	casos := []struct{ nombre, contenido string }{
		{"csv con comas", "Referencia,Fecha,Monto\nREF-1,15/01/2026,125000.00\n"},
		// Excel en español exporta con punto y coma: un lector que solo entienda la coma leería cada
		// fila como UNA columna —sin fallar— y el archivo entero entraría vacío.
		{"csv con punto y coma", "Referencia;Fecha;Monto\nREF-1;15/01/2026;125000.00\n"},
		{"separado por tabulaciones", "Referencia\tFecha\tMonto\nREF-1\t15/01/2026\t125000.00\n"},
		{"con el BOM que mete Excel", "\xEF\xBB\xBFReferencia,Fecha,Monto\nREF-1,15/01/2026,125000.00\n"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			g, err := GridDelReporte([]byte(c.contenido))
			if err != nil {
				t.Fatalf("abrir: %v", err)
			}
			filas, err := LeerReportePagos(g)
			if err != nil {
				t.Fatalf("leer: %v", err)
			}
			if len(filas) != 1 {
				t.Fatalf("se leyeron %d filas, se esperaba 1 (¿se detectó mal el separador?)", len(filas))
			}
			if filas[0].Referencia != "REF-1" || filas[0].MontoTexto != "125000.00" {
				t.Errorf("fila = %+v", filas[0])
			}
		})
	}
}

// Un archivo que no es un reporte tiene que terminar en un error REDACTADO, no en un 500.
//
// Se prueba el camino completo (abrir + leer) y no solo el abridor: un lector de texto separado
// acepta casi cualquier cosa —no es su trabajo juzgar el contenido— y el rechazo correcto llega al
// interpretarlo. Lo que no puede pasar es que salga «zip: not a valid zip file», que no le dice
// nada a quien subió el archivo.
func TestUnArchivoQueNoEsUnReporteTerminaEnUnErrorRedactado(t *testing.T) {
	casos := []struct {
		nombre string
		datos  []byte
	}{
		{"binario cualquiera", []byte{0x00, 0x01, 0x02, 0x03}},
		{"un texto que no es un reporte", []byte("hola\nque tal\n")},
		{"vacío", []byte("")},
	}
	conocidos := []error{ErrReporteFormato, ErrReporteSinEncabezado, ErrReporteSinFilas, ErrReporteSinLlave}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			g, err := GridDelReporte(c.datos)
			if err == nil {
				_, err = LeerReportePagos(g)
			}
			if err == nil {
				t.Fatal("se aceptó como reporte")
			}
			for _, k := range conocidos {
				if errors.Is(err, k) {
					return
				}
			}
			t.Errorf("err = %v; no es ninguno de los centinelas que el handler sabe traducir, así que saldría como 500", err)
		})
	}
}

// Los alias tienen que estar normalizados igual que los encabezados que se comparan contra ellos:
// un alias escrito con tilde o con mayúscula no calzaría NUNCA y el campo quedaría mudo.
func TestLosAliasEstanEscritosComoSeComparan(t *testing.T) {
	for campo, lista := range aliasReportePago {
		for _, a := range lista {
			if n := normalizarEncabezadoReporte(a); n != a {
				t.Errorf("el alias %q del campo %q no está normalizado (se compararía contra %q y nunca calzaría)", a, campo, n)
			}
			if strings.TrimSpace(a) == "" {
				t.Errorf("alias vacío en el campo %q: calzaría con cualquier celda en blanco", campo)
			}
		}
	}
}
