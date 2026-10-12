package cxc

// Las fechas que SÍ estaban y el importador leía como vacías (7-oct-2026).
//
// El usuario mandó dos capturas lado a lado: en la pantalla, cinco contratos con «SIN FECHA DE
// PRIMER COBRO NI DE PRÓXIMO PAGO»; en su Excel, esas mismas filas con «Contrato fecha primer
// cobro» = 1/2/2022 y «Fecha próximo pago» = 4/8/2023. «No me está cuadrando esto y sí tiene
// datos.»
//
// Excel no guarda «1/2/2022»: guarda el número 44593 y aparte el formato con que mostrarlo. Al
// leer solo lo formateado llegaba «02-01-22» —el render del formato del libro, con guiones y el
// mes primero— y el parser, que espera d/m/aaaa con barras, devolvía "". La fila entera caía en
// cuarentena por un dato que estaba puesto.
//
// Leer el texto y adivinar el orden NO es la salida: en este proyecto ya entraron fechas con mes
// y día invertidos en silencio por un DateStyle, y acá serían miles de vencimientos movidos de
// mes. El número de serie es absoluto y no depende de ningún formato; de ahí salen las fechas.

import (
	"bytes"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// encabezadoReal son las columnas tal como vienen en el archivo del usuario.
var encabezadoReal = []string{
	"Contrato", "Cliente", "Documento", "Teléfonos", "Otro teléfono", "Correos", "Sede",
	"Cuota servicio", "Modalidad de cobro", "Contrato fecha primer cobro", "Fecha próximo pago",
	"Días de pagos", "Forma de pago",
}

// excelConFechasReales arma un .xlsx como el que exporta Excel: las fechas son valores de
// fecha (número de serie + formato), no texto.
//
// `numFmt` decide con qué formato se muestran. Que la prueba lo reciba es el punto: el
// importador tiene que dar la MISMA fecha con cualquiera de ellos, porque el formato es una
// decisión de presentación de quien armó el archivo.
func excelConFechasReales(t *testing.T, numFmt int) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	h := f.GetSheetName(0)

	for i, e := range encabezadoReal {
		c, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellValue(h, c, e); err != nil {
			t.Fatal(err)
		}
	}
	estilo, err := f.NewStyle(&excelize.Style{NumFmt: numFmt})
	if err != nil {
		t.Fatal(err)
	}

	// Las cinco filas de la captura, con sus fechas de verdad.
	filas := []struct {
		contrato, cliente, doc string
		cuota                  float64
		primerCobro            time.Time
		proximoPago            time.Time
		dia                    int
	}{
		{"CO127269", "RAUL", "113940337", 4950, elDia(2022, 2, 1), elDia(2023, 8, 4), 4},
		{"CO149027", "STHEPHANIE ALEJANDRA", "206260965", 7800, elDia(2023, 8, 16), elDia(2023, 10, 16), 16},
		{"CO123065", "DOUGLAS", "701760435", 5000, elDia(2021, 10, 1), elDia(2023, 10, 27), 27},
		{"CO150419", "PEDRO JOSUE", "305550243", 5085, elDia(2023, 10, 30), elDia(2023, 10, 30), 15},
		{"CO136548", "RODRIGO ALBERTO", "700830368", 4600, elDia(2022, 10, 30), elDia(2023, 11, 30), 30},
	}
	for i, fl := range filas {
		r := i + 2
		poner := func(col int, v any) {
			t.Helper()
			c, err := excelize.CoordinatesToCellName(col, r)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.SetCellValue(h, c, v); err != nil {
				t.Fatal(err)
			}
		}
		ponerFecha := func(col int, v time.Time) {
			t.Helper()
			poner(col, v)
			c, err := excelize.CoordinatesToCellName(col, r)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.SetCellStyle(h, c, c, estilo); err != nil {
				t.Fatal(err)
			}
		}
		poner(1, fl.contrato)
		poner(2, fl.cliente)
		poner(3, fl.doc)
		poner(7, "Federico")
		poner(8, fl.cuota)
		poner(9, "Mensual")
		ponerFecha(10, fl.primerCobro)
		ponerFecha(11, fl.proximoPago)
		poner(12, fl.dia)
		poner(13, "Débito Automático")
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func elDia(a int, m time.Month, d int) time.Time {
	return time.Date(a, m, d, 0, 0, 0, 0, time.UTC)
}

// El formato de la celda no puede cambiar la fecha que se importa. Se prueban cuatro de los
// que Excel usa de fábrica, incluido el 14 —que es el que renderiza «02-01-22» y destapó el
// defecto— y uno con el día primero, que es el orden contrario.
func TestLaFechaNoDependeDelFormatoDeLaCelda(t *testing.T) {
	formatos := map[string]int{
		"14 (m-d-aa, el que rompía)": 14,
		"15 (d-mmm-aa)":              15,
		"22 (m/d/aa h:mm)":           22,
		"34 (aaaa-mm-dd ISO)":        34,
	}
	for nombre, numFmt := range formatos {
		t.Run(nombre, func(t *testing.T) {
			h, err := CargarHoja(excelConFechasReales(t, numFmt))
			if err != nil {
				t.Fatalf("CargarHoja: %v", err)
			}
			filas, err := LeerContratos(h, ReglasImportacion{CuotaMaxima: decimal.NewFromInt(1000000)})
			if err != nil {
				t.Fatalf("LeerContratos: %v", err)
			}
			if len(filas) != 5 {
				t.Fatalf("se leyeron %d contratos, se esperaban 5", len(filas))
			}
			quiere := []string{"2022-02-01", "2023-08-16", "2021-10-01", "2023-10-30", "2022-10-30"}
			for i, f := range filas {
				if f.PrimerCobro != quiere[i] {
					t.Errorf("contrato %s: primer cobro = %q, se esperaba %q (motivos: %v)",
						f.Numero, f.PrimerCobro, quiere[i], f.Motivos)
				}
				for _, m := range f.Motivos {
					if m == "sin fecha de primer cobro ni de próximo pago" {
						t.Errorf("contrato %s quedó marcado «sin fecha» y el archivo la trae",
							f.Numero)
					}
				}
			}
		})
	}
}

// El 1 de febrero no puede entrar como 2 de enero. Es la razón por la que la fecha sale del
// número de serie y no del texto: «02-01-22» no dice cuál de los dos números es el día.
func TestElDiaYElMesNoSeInvierten(t *testing.T) {
	h, err := CargarHoja(excelConFechasReales(t, 14))
	if err != nil {
		t.Fatalf("CargarHoja: %v", err)
	}
	// Lo que el texto formateado llegaría a decir, para dejar constancia de la trampa.
	t.Logf("texto formateado de la primera fecha: %q", h.Valores[1][9])

	filas, err := LeerContratos(h, ReglasImportacion{CuotaMaxima: decimal.NewFromInt(1000000)})
	if err != nil {
		t.Fatalf("LeerContratos: %v", err)
	}
	if filas[0].PrimerCobro == "2022-01-02" {
		t.Fatal("el 1 de febrero entró como 2 de enero: el día y el mes se invirtieron, " +
			"que es exactamente lo que mueve de mes miles de vencimientos")
	}
	if filas[0].PrimerCobro != "2022-02-01" {
		t.Fatalf("primer cobro = %q, se esperaba 2022-02-01", filas[0].PrimerCobro)
	}
	// Y una que no admite confusión: el día 16 no puede ser un mes.
	if filas[1].PrimerCobro != "2023-08-16" {
		t.Fatalf("primer cobro de la segunda = %q, se esperaba 2023-08-16", filas[1].PrimerCobro)
	}
}

// Un CSV con las fechas como TEXTO sigue leyéndose d/m/aaaa. El arreglo no podía cambiar el
// camino que ya funcionaba: los dos formatos de archivo conviven.
func TestElTextoSigueSiendoDiaMesAnio(t *testing.T) {
	csv := "Contrato;Cliente;Sede;Cuota servicio;Modalidad de cobro;Contrato fecha primer cobro;Días de pagos\n" +
		"CO127269;RAUL;Federico;4950;Mensual;16/8/2023;16\n"
	h, err := CargarHoja([]byte(csv))
	if err != nil {
		t.Fatalf("CargarHoja: %v", err)
	}
	filas, err := LeerContratos(h, ReglasImportacion{CuotaMaxima: decimal.NewFromInt(1000000)})
	if err != nil {
		t.Fatalf("LeerContratos: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("se leyeron %d contratos, se esperaba 1", len(filas))
	}
	if filas[0].PrimerCobro != "2023-08-16" {
		t.Errorf("primer cobro = %q, se esperaba 2023-08-16 (16 de AGOSTO: el día va primero)",
			filas[0].PrimerCobro)
	}
}

// Un número que no es una serie de fecha plausible NO se convierte en fecha. Sin este corte,
// un «4» —una columna corrida, un día de pago leído donde va la fecha— entraría como el 4 de
// enero de 1900: un dato inventado que nadie vuelve a mirar, en vez de una fila marcada.
func TestUnNumeroChicoNoEsUnaFecha(t *testing.T) {
	casos := map[string]string{
		"día de pago suelto":       "4",
		"cero":                     "0",
		"número de contrato corto": "127269",
		"anterior a 1990":          "1",
	}
	for nombre, crudo := range casos {
		t.Run(nombre, func(t *testing.T) {
			if got := fechaDeCelda("", crudo); got != "" {
				t.Errorf("%q se leyó como la fecha %q", crudo, got)
			}
		})
	}
	// Y el control opuesto: una serie de verdad sí entra.
	if got := fechaDeCelda("02-01-22", "44593"); got != "2022-02-01" {
		t.Errorf("la serie 44593 dio %q, se esperaba 2022-02-01", got)
	}
}

// Las dos lecturas de la hoja tienen que hablar de la misma fila. Si se desalinearan, una
// factura tomaría la fecha de otra: el peor resultado posible, porque se ve correcto.
func TestLasDosLecturasEstanAlineadas(t *testing.T) {
	h, err := CargarHoja(excelConFechasReales(t, 14))
	if err != nil {
		t.Fatalf("CargarHoja: %v", err)
	}
	if len(h.Valores) != len(h.Crudos) {
		t.Fatalf("la hoja tiene %d filas con formato y %d sin formato", len(h.Valores), len(h.Crudos))
	}
	for i := range h.Valores {
		if len(h.Valores[i]) == 0 || len(h.Crudos[i]) == 0 {
			continue
		}
		// La primera columna es el número de contrato: texto, igual en las dos lecturas.
		if h.Valores[i][0] != h.Crudos[i][0] {
			t.Errorf("fila %d: con formato empieza con %q y sin formato con %q",
				i, h.Valores[i][0], h.Crudos[i][0])
		}
	}
}
