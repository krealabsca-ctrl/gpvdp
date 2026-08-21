package cxc

import (
	"os"
	"testing"

	"github.com/shopspring/decimal"
)

// Los archivos de testdata son los DATASETS REALES que entregó el usuario (muestras del
// mismo reporte que expondrá la API). Probar contra ellos es la única forma de saber que
// el importador entiende la operación y no un ejemplo que yo mismo inventé.
func gridDeTestdata(t *testing.T, nombre string) Grid {
	t.Helper()
	b, err := os.ReadFile("testdata/" + nombre)
	if err != nil {
		t.Fatalf("leer %s: %v", nombre, err)
	}
	g, err := CargarGrid(b)
	if err != nil {
		t.Fatalf("CargarGrid(%s): %v", nombre, err)
	}
	return g
}

func TestLeerContratosDelArchivoReal(t *testing.T) {
	t.Parallel()
	g := gridDeTestdata(t, "contratos_muestra.csv")
	// 1 encabezado + 9 contratos.
	if len(g) != 10 {
		t.Fatalf("filas del grid = %d, se esperaban 10", len(g))
	}

	filas, err := LeerContratos(g, ReglasImportacion{CuotaMaxima: decimal.NewFromInt(500000)})
	if err != nil {
		t.Fatalf("LeerContratos: %v", err)
	}
	if len(filas) != 9 {
		t.Fatalf("contratos = %d, se esperaban 9", len(filas))
	}

	porNumero := map[string]FilaContrato{}
	for _, f := range filas {
		porNumero[f.Numero] = f
	}

	t.Run("los dos formatos de número se conservan tal cual", func(t *testing.T) {
		for _, n := range []string{"CD-0000000561", "CO198456"} {
			if _, ok := porNumero[n]; !ok {
				t.Errorf("no se leyó el contrato %s", n)
			}
		}
	})

	t.Run("contrato mensual con débito automático", func(t *testing.T) {
		f := porNumero["CD-0000000561"]
		if f.Cliente != "RONALD GERARDO ESQUIVEL ALVARADO" {
			t.Errorf("cliente = %q", f.Cliente)
		}
		if f.Documento != "602310132" {
			t.Errorf("documento = %q", f.Documento)
		}
		if !f.Cuota.Equal(decimal.RequireFromString("5600")) {
			t.Errorf("cuota = %s, se esperaba 5600", f.Cuota)
		}
		if f.Modalidad != "Mensual" || f.FormaPago != "Débito Automático" {
			t.Errorf("modalidad/forma = %q / %q", f.Modalidad, f.FormaPago)
		}
		if f.DiaPago != 3 {
			t.Errorf("día de pago = %d, se esperaba 3", f.DiaPago)
		}
		// 3/8/2026 es 3 de AGOSTO: día primero. Si se leyera al revés (3 de marzo),
		// el vencimiento se movería cinco meses.
		if f.PrimerCobro != "2026-08-03" {
			t.Errorf("primer cobro = %q, se esperaba 2026-08-03", f.PrimerCobro)
		}
		// «oct-28» = vence a fin de octubre de 2028.
		if f.TarjetaVence != "2028-10-31" {
			t.Errorf("tarjeta vence = %q, se esperaba 2028-10-31", f.TarjetaVence)
		}
		// Los dos teléfonos del origen se juntan sin perder ninguno.
		if f.Telefonos != "2212-2000 / 88558008" {
			t.Errorf("teléfonos = %q", f.Telefonos)
		}
		if f.EnCuarentena() {
			t.Errorf("no debería estar en cuarentena: %v", f.Motivos)
		}
	})

	t.Run("el campo Sede se parte en razón social y plaza", func(t *testing.T) {
		f := porNumero["CD-0000000561"]
		if f.RazonSocial != "VALLE DE PAZ DE COSTA RICA SA" || f.Plaza != "SAN JOSÉ" {
			t.Errorf("razón social / plaza = %q / %q", f.RazonSocial, f.Plaza)
		}
		// La otra persona jurídica del mismo archivo, con el guion pegado al nombre.
		o := porNumero["CO198456"]
		if o.RazonSocial == "" {
			t.Errorf("no se detectó la razón social en %q", o.SedeCruda)
		}
		if o.RazonSocial == f.RazonSocial {
			t.Errorf("las dos personas jurídicas quedaron iguales: %q", o.RazonSocial)
		}
	})

	t.Run("el contrato anual con mora vieja trae sus datos del origen", func(t *testing.T) {
		f := porNumero["CO198456"]
		if f.Modalidad != "Anual" {
			t.Errorf("modalidad = %q, se esperaba Anual", f.Modalidad)
		}
		if !f.Cuota.Equal(decimal.RequireFromString("2916.66")) {
			t.Errorf("cuota = %s", f.Cuota)
		}
		if f.SaldoOrigen == nil || !f.SaldoOrigen.Equal(decimal.RequireFromString("9254")) {
			t.Errorf("saldo del origen = %v, se esperaba 9254", f.SaldoOrigen)
		}
		if f.ScoreOrigen == nil || *f.ScoreOrigen != -139 {
			t.Errorf("score = %v, se esperaba -139 (negativo, como viene)", f.ScoreOrigen)
		}
		if f.DiasVencidosOrigen == nil || *f.DiasVencidosOrigen != 215 {
			t.Errorf("días vencidos = %v, se esperaban 215", f.DiasVencidosOrigen)
		}
		if f.MorosidadOrigen != "Crítico" || f.EstadoOrigen != "Alto riesgo" {
			t.Errorf("morosidad/estado = %q / %q", f.MorosidadOrigen, f.EstadoOrigen)
		}
	})

	t.Run("los días vencidos negativos se leen como negativos", func(t *testing.T) {
		f := porNumero["CD-0000000546"]
		if f.DiasVencidosOrigen == nil || *f.DiasVencidosOrigen != -26 {
			t.Fatalf("días vencidos = %v, se esperaban -26 (adelantado)", f.DiasVencidosOrigen)
		}
	})

	t.Run("la asociación solidarista se lee cuando viene", func(t *testing.T) {
		f := porNumero["CO198453"]
		if f.Asociacion != "ASEPAN" {
			t.Errorf("asociación = %q, se esperaba ASEPAN", f.Asociacion)
		}
		if f.FormaPago != "Descuento por Asociación Solidarista" {
			t.Errorf("forma de pago = %q", f.FormaPago)
		}
	})

	t.Run("ninguna fila del archivo real queda en cuarentena", func(t *testing.T) {
		for _, f := range filas {
			if f.EnCuarentena() {
				t.Errorf("%s en cuarentena por %v", f.Numero, f.Motivos)
			}
		}
	})
}

func TestLeerContratosCuarentena(t *testing.T) {
	t.Parallel()
	// El caso real que motivó la cuarentena en el portal: un SALDO pegado en el campo
	// de cuota. Debe entrar MARCADO, no rechazado ni digerido como si fuera una cuota.
	g := Grid{
		{"Contrato", "Cliente", "Cuota Servicio", "Modalidad de Cobro", "Contrato Fecha Primer Cobro", "Sede", "Dias de Pagos"},
		{"CO1", "CON SALDO PEGADO", "1250000000.00", "Mensual", "1/8/2026", "SAN JOSÉ - VALLE DE PAZ SA", "1"},
		{"CO2", "SIN CUOTA", "0.00", "Mensual", "1/8/2026", "SAN JOSÉ - VALLE DE PAZ SA", "1"},
		{"CO3", "SIN FECHA", "5600.00", "Mensual", "", "SAN JOSÉ - VALLE DE PAZ SA", "1"},
		{"CO4", "SIN MODALIDAD", "5600.00", "", "1/8/2026", "SAN JOSÉ - VALLE DE PAZ SA", "1"},
		{"CO5", "DIA IMPOSIBLE", "5600.00", "Mensual", "1/8/2026", "SAN JOSÉ - VALLE DE PAZ SA", "45"},
		{"CO6", "CUOTA ILEGIBLE", "no es un monto", "Mensual", "1/8/2026", "SAN JOSÉ - VALLE DE PAZ SA", "1"},
		{"CO7", "BUENO", "5600.00", "Mensual", "1/8/2026", "SAN JOSÉ - VALLE DE PAZ SA", "1"},
	}
	filas, err := LeerContratos(g, ReglasImportacion{CuotaMaxima: decimal.NewFromInt(500000)})
	if err != nil {
		t.Fatalf("LeerContratos: %v", err)
	}
	if len(filas) != 7 {
		t.Fatalf("filas = %d, se esperaban 7 (ninguna se descarta en silencio)", len(filas))
	}
	quiero := map[string]bool{"CO1": true, "CO2": true, "CO3": true, "CO4": true, "CO5": true, "CO6": true, "CO7": false}
	for _, f := range filas {
		if f.EnCuarentena() != quiero[f.Numero] {
			t.Errorf("%s cuarentena=%v (motivos %v), se esperaba %v", f.Numero, f.EnCuarentena(), f.Motivos, quiero[f.Numero])
		}
	}
}

func TestLeerContratosResuelvePorEncabezadoNoPorPosicion(t *testing.T) {
	t.Parallel()
	// Las columnas en otro orden, con alias distintos y una columna extra en medio:
	// la cicatriz del portal fue leer por posición. Debe dar el mismo resultado.
	g := Grid{
		{"Reporte de cobranza", "", "", ""},
		{"Modalidad", "Basura", "No. Contrato", "Cédula", "Monto Cuota", "Fecha Primer Cobro", "Día de Pago"},
		{"Mensual", "x", "CO9", "111", "7500.00", "15/9/2026", "15"},
	}
	filas, err := LeerContratos(g, ReglasImportacion{CuotaMaxima: decimal.NewFromInt(500000)})
	if err != nil {
		t.Fatalf("LeerContratos: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %d", len(filas))
	}
	f := filas[0]
	if f.Numero != "CO9" || f.Documento != "111" || f.DiaPago != 15 ||
		!f.Cuota.Equal(decimal.RequireFromString("7500")) || f.PrimerCobro != "2026-09-15" {
		t.Errorf("fila mal interpretada: %+v", f)
	}
}

func TestCargarGridDetectaSeparadorYQuitaBOM(t *testing.T) {
	t.Parallel()
	casos := map[string][]byte{
		"punto y coma con BOM": []byte(bom + "Contrato;Cuota Servicio\r\nCO1;5600.00\r\n"),
		"coma":                 []byte("Contrato,Cuota Servicio\nCO1,5600.00\n"),
		"tabulador":            []byte("Contrato\tCuota Servicio\nCO1\t5600.00\n"),
	}
	for nombre, b := range casos {
		g, err := CargarGrid(b)
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		if len(g) != 2 || g[0][0] != "Contrato" || g[1][0] != "CO1" {
			t.Errorf("%s: grid mal leído %+v", nombre, g)
		}
	}
	if _, err := CargarGrid(nil); err == nil {
		t.Error("un archivo vacío debería fallar explícitamente")
	}
}

func TestMontoDe(t *testing.T) {
	t.Parallel()
	casos := []struct {
		in     string
		quiero string
		falla  bool
	}{
		{"5600.00", "5600", false},
		{"2916.66", "2916.66", false},
		{"14928.52", "14928.52", false},
		{"₡1,084,200.00", "1084200", false},
		{"1 084 200,00", "1084200", false},
		{"1.084.200,50", "1084200.5", false},
		{"", "0", false},
		{"no es un monto", "", true},
	}
	for _, c := range casos {
		got, err := montoDe(c.in)
		if c.falla {
			if err == nil {
				t.Errorf("montoDe(%q) debería fallar", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("montoDe(%q): %v", c.in, err)
			continue
		}
		if !got.Equal(decimal.RequireFromString(c.quiero)) {
			t.Errorf("montoDe(%q) = %s, se esperaba %s", c.in, got, c.quiero)
		}
	}
}

func TestFechaDeYFinDeMes(t *testing.T) {
	t.Parallel()
	// d/m/aaaa: el día va primero en los archivos reales.
	if got := fechaDe("3/8/2026"); got != "2026-08-03" {
		t.Errorf("fechaDe(3/8/2026) = %q, se esperaba 2026-08-03", got)
	}
	if got := fechaDe("31/7/2026"); got != "2026-07-31" {
		t.Errorf("fechaDe(31/7/2026) = %q", got)
	}
	// Dos fechas separadas por «|» (planilla que llegó en dos transferencias).
	if got := fechaDe("08/07/2026|11/07/2026"); got != "2026-07-08" {
		t.Errorf("fecha doble = %q, se esperaba la primera (2026-07-08)", got)
	}
	if got := fechaDe("no es fecha"); got != "" {
		t.Errorf("basura debería dar vacío, dio %q", got)
	}
	// Vencimiento de tarjeta: mes-año → último día del mes.
	for in, quiero := range map[string]string{
		"oct-28":  "2028-10-31",
		"nov-29":  "2029-11-30",
		"sept-26": "2026-09-30",
		"feb-28":  "2028-02-29", // 2028 es bisiesto
		"":        "",
		"xx-99":   "",
	} {
		if got := finDeMesDe(in); got != quiero {
			t.Errorf("finDeMesDe(%q) = %q, se esperaba %q", in, got, quiero)
		}
	}
}
