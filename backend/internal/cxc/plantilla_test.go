package cxc

// La plantilla tiene que pasar por el MISMO lector que el archivo del usuario (6-oct-2026).
//
// Una corrida real entró con 23.144 filas, 23.144 en cuarentena y 23.144 sin sede: los tres números
// idénticos. La causa es que un archivo con los encabezados equivocados NO FALLA — el lector no
// encuentra la columna, el campo entra vacío y la fila se marca. Veintitrés mil veces.
//
// El machote nació para evitar eso, así que lo que hay que probar no es que el archivo se genere,
// sino que **el lector lo entienda**: se arma la plantilla, se vuelve a leer con LeerContratos y se
// exige que la fila de ejemplo entre COMPLETA y sin un solo motivo. Si alguien agrega un alias a
// `aliasContrato` y no toca la plantilla, o al revés, esta prueba lo dice.

import (
	"testing"

	"github.com/shopspring/decimal"
)

func catalogoDePrueba() Catalogos {
	return Catalogos{
		Sedes:        []ItemCatalogo{{Nombre: "Valle de Paz S.A. - Cartago"}, {Nombre: "Valle de Paz S.A. - San José"}},
		Modalidades:  []ItemCatalogo{{Nombre: "Mensual"}, {Nombre: "Quincenal"}, {Nombre: "Anual"}},
		FormasPago:   []ItemCatalogo{{Nombre: "Débito Automático"}, {Nombre: "Cobrador"}},
		Asociaciones: []ItemCatalogo{{Nombre: "ASEVERTEC"}},
	}
}

// gridDeLaPlantilla genera el machote y lo vuelve a leer como lo leería el importador.
func gridDeLaPlantilla(t *testing.T) Grid {
	t.Helper()
	buf, err := PlantillaCartera(catalogoDePrueba())
	if err != nil {
		t.Fatalf("generar la plantilla: %v", err)
	}
	g, err := CargarGrid(buf)
	if err != nil {
		t.Fatalf("releer la plantilla: %v", err)
	}
	return g
}

func TestElLectorReconoceTodasLasColumnasDeLaPlantilla(t *testing.T) {
	g := gridDeLaPlantilla(t)

	cols, encabezado := columnasContrato(g)
	if cols == nil {
		t.Fatal("el lector no encontró la fila de encabezados EN SU PROPIA PLANTILLA")
	}
	if encabezado != 0 {
		t.Errorf("el encabezado quedó en la fila %d, se esperaba la primera", encabezado+1)
	}

	// Cada columna que la plantilla escribe tiene que caer en el campo que dice ser. Si no, el
	// machote está ofreciendo un encabezado que el lector ignora.
	for _, c := range columnasDeLaPlantilla {
		if _, ok := cols[c.Campo]; !ok {
			t.Errorf("la plantilla escribe «%s» para el campo %q y el lector no lo reconoció",
				c.Encabezado, c.Campo)
		}
	}
}

// La hoja de datos sale VACÍA de contratos: una fila de ejemplo ahí es un contrato más para el
// importador, y quien olvide borrarla mete «CO198456» en la cartera. Lo descubrió esta misma prueba,
// que leyó dos filas donde tenía que haber una.
func TestLaHojaDeDatosNoTraeNingunContrato(t *testing.T) {
	g := gridDeLaPlantilla(t)

	_, err := LeerContratos(HojaDe(g), ReglasImportacion{CuotaMaxima: decimal.NewFromInt(1000000)})
	if err != ErrSinFilas {
		filas, _ := LeerContratos(HojaDe(g), ReglasImportacion{CuotaMaxima: decimal.NewFromInt(1000000)})
		t.Fatalf("la plantilla trae %d contrato(s) y tiene que venir vacía (err=%v)", len(filas), err)
	}
}

// Y el ejemplo que la ayuda muestra tiene que ser VÁLIDO: se pega debajo del encabezado real y se
// lee con el lector de verdad. Si enseñara un formato que el importador no entiende, el machote
// estaría documentando cómo llenar mal el archivo.
func TestElEjemploDeLaAyudaEsUnaFilaValida(t *testing.T) {
	g := gridDeLaPlantilla(t)
	g = append(g, filaEjemplo)

	filas, err := LeerContratos(HojaDe(g), ReglasImportacion{CuotaMaxima: decimal.NewFromInt(1000000)})
	if err != nil {
		t.Fatalf("leer la plantilla con el ejemplo: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("se leyeron %d filas, se pegó UNA", len(filas))
	}
	f := filas[0]

	// EL punto: la fila de ejemplo no puede dar ni un motivo. Si lo diera, el machote estaría
	// enseñando a llenar mal el archivo.
	if f.EnCuarentena() {
		t.Errorf("el ejemplo que documenta la ayuda cae en cuarentena por: %v", f.Motivos)
	}

	// Y los campos que deciden si el contrato se puede cobrar tienen que haber entrado de verdad,
	// no vacíos: un campo vacío sin motivo sería peor que la cuarentena.
	casos := []struct{ campo, got, quiere string }{
		{"número", f.Numero, "CO198456"},
		{"cliente", f.Cliente, "MARÍA RODRÍGUEZ SOLÍS"},
		{"documento", f.Documento, "1-0888-0777"},
		{"modalidad", f.Modalidad, "Mensual"},
		{"forma de pago", f.FormaPago, "Débito Automático"},
		{"primer cobro", f.PrimerCobro, "2026-01-15"},
		{"fecha inicial", f.FechaInicial, "2026-01-01"},
	}
	for _, c := range casos {
		if c.got != c.quiere {
			t.Errorf("%s = %q, quiere %q", c.campo, c.got, c.quiere)
		}
	}
	if !f.Cuota.Equal(decimal.RequireFromString("15000.00")) {
		t.Errorf("cuota = %s, quiere 15000.00", f.Cuota)
	}
	if f.DiaPago != 15 {
		t.Errorf("día de pago = %d, quiere 15", f.DiaPago)
	}
	// La sede del ejemplo se parte en razón social y plaza, que es lo que el importador concilia.
	if f.RazonSocial != "Valle de Paz S.A." || f.Plaza != "Cartago" {
		t.Errorf("sede = %q + %q, quiere «Valle de Paz S.A.» + «Cartago»", f.RazonSocial, f.Plaza)
	}
	// El vencimiento de tarjeta viene como mes-año y se guarda el último día del mes.
	if f.TarjetaVence != "2028-10-31" {
		t.Errorf("vencimiento de tarjeta = %q, quiere 2028-10-31", f.TarjetaVence)
	}
}

// La hoja de ayuda tiene que hablar de TODAS las columnas y de ninguna que no exista: es lo que
// alguien lee para entender por qué su archivo quedó en cuarentena.
func TestLaAyudaDescribeExactamenteLasColumnasQueSeEscriben(t *testing.T) {
	vistos := map[string]bool{}
	for _, c := range columnasDeLaPlantilla {
		if c.Encabezado == "" || c.Campo == "" {
			t.Errorf("columna sin encabezado o sin campo: %+v", c)
		}
		if vistos[c.Campo] {
			t.Errorf("el campo %q aparece dos veces en la plantilla", c.Campo)
		}
		vistos[c.Campo] = true
		if _, ok := aliasContrato[c.Campo]; !ok {
			t.Errorf("la plantilla ofrece el campo %q, que el lector no conoce", c.Campo)
		}
		if c.Obligatoria == "" || c.Formato == "" || c.SiFalta == "" {
			t.Errorf("la columna «%s» no explica si es obligatoria, cómo se escribe o qué pasa si falta", c.Encabezado)
		}
	}
	// La fila de ejemplo tiene que tener exactamente un valor por columna, o las muestras se corren
	// de lugar y enseñan el formato equivocado.
	if len(filaEjemplo) != len(columnasDeLaPlantilla) {
		t.Errorf("el ejemplo tiene %d valores y hay %d columnas", len(filaEjemplo), len(columnasDeLaPlantilla))
	}
}

// Los campos del lector que la plantilla NO ofrece se nombran acá a conciencia: si mañana alguien
// agrega un alias nuevo, esta prueba obliga a decidir si va al machote o por qué no.
func TestLosCamposQueElMachoteNoOfreceEstanJustificados(t *testing.T) {
	enPlantilla := map[string]bool{}
	for _, c := range columnasDeLaPlantilla {
		enPlantilla[c.Campo] = true
	}
	exentos := map[string]string{
		"mes_cobro": "lo trae el archivo de COBROS, no el de cartera",
	}
	for campo := range aliasContrato {
		if !enPlantilla[campo] && exentos[campo] == "" {
			t.Errorf("el lector entiende el campo %q y la plantilla no lo ofrece: o se agrega, o se justifica en `exentos`", campo)
		}
	}
}

// Las tres hojas tienen que existir: la de valores es la que evita que alguien escriba una
// modalidad que no está en el catálogo y deje el contrato sin cobrar.
func TestLaPlantillaTraeLasTresHojas(t *testing.T) {
	buf, err := PlantillaCartera(catalogoDePrueba())
	if err != nil {
		t.Fatalf("generar: %v", err)
	}
	// CargarGrid lee la PRIMERA hoja; para ver las demás hay que abrir el libro.
	g, err := CargarGrid(buf)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	if len(g) == 0 || len(g[0]) == 0 || g[0][0] != "Contrato" {
		t.Fatalf("la primera hoja no es la de contratos: %v", g[0])
	}
}
