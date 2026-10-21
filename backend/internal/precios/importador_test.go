package precios

// Pruebas del importador del catálogo.
//
// El escenario se construye en memoria con excelize en vez de usar un archivo de muestra, por dos
// razones: el archivo real trae la estructura de costos de la empresa y no debe viajar en el
// repositorio, y un caso construido deja ver exactamente qué se está probando.

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

// SEDES son las plazas del archivo real. Cada una tiene que aparecer en VARIOS ítems para que
// el importador la reconozca como sede y no como parte de un nombre: ese es justo el criterio
// que evita partir «Cofre Standard -COFRE PALOMA».
var SEDES = []string{"Limon", "Heredia", "Cartago", "Liberia", "Sabana"}

// porSede repite cada ítem en las cinco sedes, que es la forma del archivo real.
func porSede(items map[string][]string) [][]string {
	filas := [][]string{{"id", "Descripcion", "Costo"}}
	for nombre, costos := range items {
		for i, s := range SEDES {
			c := costos[0]
			if len(costos) > i {
				c = costos[i]
			}
			filas = append(filas, []string{"", nombre + "-" + s, c})
		}
	}
	return filas
}

// libro arma un .xlsx en memoria con el encabezado y las filas dadas.
func libro(t *testing.T, hoja string, filas [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	idx, err := f.NewSheet(hoja)
	if err != nil {
		t.Fatalf("crear hoja: %v", err)
	}
	f.SetActiveSheet(idx)
	for i, fila := range filas {
		for j, celda := range fila {
			col, _ := excelize.ColumnNumberToName(j + 1)
			if err := f.SetCellStr(hoja, col+itoa(i+1), celda); err != nil {
				t.Fatalf("escribir celda: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("escribir libro: %v", err)
	}
	return buf.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// El sufijo de sede se detecta por REPETICIÓN, no por una lista de sedes conocidas: el archivo
// puede traer plazas que no están en el catálogo del sistema.
func TestDetectaLasSedesPorRepeticionYNoPartiNombresLegitimos(t *testing.T) {
	t.Parallel()
	filas := [][]string{{"id", "Descripcion", "Costo"},
		// Un nombre con guion que NO se repite: no se debe partir. Va primero para que entre
		// en la muestra que devuelve el análisis.
		{"", "Cofre Standard -COFRE PALOMA", "79000"},
	}
	// Seis líneas con sufijo "Limon" y seis con "Heredia": son sedes.
	for i := 0; i < 6; i++ {
		filas = append(filas,
			[]string{"", "Cremación de Adulto-Limon", "100000"},
			[]string{"", "Arreglo Floral-Heredia", "5000"},
		)
	}

	a, err := Analizar(libro(t, "Costos", filas), "Costos", "", "")
	if err != nil {
		t.Fatalf("Analizar: %v", err)
	}

	if len(a.SedesDetectadas) != 2 {
		t.Errorf("sedes detectadas = %v, se esperaban Limon y Heredia", a.SedesDetectadas)
	}
	// 2 ítems con sede + el cofre entero = 3 insumos al unificar.
	if a.InsumosSiSeUnifica != 3 {
		t.Errorf("insumos al unificar = %d, se esperaban 3", a.InsumosSiSeUnifica)
	}
	hayCofreEntero := false
	for _, f := range a.Muestra {
		if f.NombreCrudo == "Cofre Standard -COFRE PALOMA" {
			hayCofreEntero = f.NombreBase == "Cofre Standard -COFRE PALOMA" && f.Sede == ""
		}
	}
	if !hayCofreEntero {
		t.Error("partió un nombre legítimo que solo tiene un guion decorativo")
	}
}

// La decisión de unificar o no es del usuario, así que el análisis tiene que MEDIR las dos
// lecturas y decir en cuántos ítems las sedes difieren en costo.
func TestElAnalisisMideLasDosLecturasYNombraLosConflictos(t *testing.T) {
	t.Parallel()
	filas := porSede(map[string][]string{
		// Mismo costo en las 5 sedes → unificar es seguro.
		"Cremación de Adulto": {"100000"},
		// El costo CAMBIA por sede → unificar perdería información.
		"Arreglo Floral": {"5000", "5500", "6000", "6500", "7000"},
	})

	a, err := Analizar(libro(t, "Costos", filas), "Costos", "", "")
	if err != nil {
		t.Fatalf("Analizar: %v", err)
	}

	if a.InsumosSiSeUnifica != 2 || a.InsumosSiNoSeUnifica != 10 {
		t.Errorf("lecturas = %d unificado / %d sin unificar, se esperaban 2 / 10",
			a.InsumosSiSeUnifica, a.InsumosSiNoSeUnifica)
	}
	if a.GruposConCostoIgual != 1 {
		t.Errorf("grupos con costo igual = %d, se esperaba 1 (la cremación)", a.GruposConCostoIgual)
	}
	if a.GruposConCostoDistinto != 1 {
		t.Errorf("grupos con costo distinto = %d, se esperaba 1 (las flores)", a.GruposConCostoDistinto)
	}
	if len(a.Conflictos) != 1 || a.Conflictos[0].NombreBase != "Arreglo Floral" {
		t.Fatalf("no nombró el conflicto real: %+v", a.Conflictos)
	}
	if a.Conflictos[0].CostosDistintos != 5 {
		t.Errorf("costos distintos = %d, se esperaban 5", a.Conflictos[0].CostosDistintos)
	}
}

// Un costo ausente NO es cero: el insumo entra sin costo y queda contado aparte, porque cero
// diría que es gratis y lo metería en un costeo como si estuviera resuelto.
func TestUnCostoAusenteNoSeVuelveCero(t *testing.T) {
	t.Parallel()
	b := libro(t, "Costos", [][]string{
		{"id", "Descripcion", "Costo"},
		{"", "Con costo", "1000"},
		{"", "Sin costo", ""},
		{"", "Con guion", "-"},
		{"", "Con cero", "0"},
	})
	a, err := Analizar(b, "Costos", "", "")
	if err != nil {
		t.Fatalf("Analizar: %v", err)
	}
	if a.LineasConCosto != 1 {
		t.Errorf("líneas con costo = %d, se esperaba 1", a.LineasConCosto)
	}
	if a.LineasSinCosto != 3 {
		t.Errorf("líneas sin costo = %d, se esperaban 3 (vacío, guion y cero)", a.LineasSinCosto)
	}
}

// Los montos del archivo vienen con separador de miles y símbolo de moneda.
func TestLeeLosMontosConFormatoDelArchivo(t *testing.T) {
	t.Parallel()
	b := libro(t, "Costos", [][]string{
		{"id", "Descripcion", "Costo"},
		{"", "Con comas", "1,350,000.50"},
		{"", "Con colones", "₡79,000"},
	})
	a, err := Analizar(b, "Costos", "", "")
	if err != nil {
		t.Fatalf("Analizar: %v", err)
	}
	if a.LineasConCosto != 2 {
		t.Fatalf("líneas con costo = %d, se esperaban 2", a.LineasConCosto)
	}
	if a.Muestra[0].Costo == nil || *a.Muestra[0].Costo != "1350000.50" {
		t.Errorf("monto con comas = %v, se esperaba 1350000.50", a.Muestra[0].Costo)
	}
	if a.Muestra[1].Costo == nil || *a.Muestra[1].Costo != "79000.00" {
		t.Errorf("monto con ₡ = %v, se esperaba 79000.00", a.Muestra[1].Costo)
	}
}

// El código se deriva del nombre y tiene que ser ESTABLE: es lo que hace que correr la
// importación dos veces actualice en vez de duplicar.
func TestElCodigoEsEstableYSinAcentos(t *testing.T) {
	t.Parallel()
	casos := []struct{ nombre, sede, quiere string }{
		{"Cremación de un Adulto", "", "CREMACION-DE-UN-ADULTO"},
		{"Cremación de un Adulto", "Limon", "CREMACION-DE-UN-ADULTO-LIMON"},
		{"Cofre #24 / Paloma", "", "COFRE-24-PALOMA"},
		{"  Espacios   de más  ", "", "ESPACIOS-DE-MAS"},
	}
	for _, c := range casos {
		if got := codigoDesde(c.nombre, c.sede); got != c.quiere {
			t.Errorf("codigoDesde(%q,%q) = %q, se esperaba %q", c.nombre, c.sede, got, c.quiere)
		}
	}
	// Dos corridas del mismo nombre dan el mismo código: sin esto la importación duplicaría.
	if codigoDesde("Arreglo Floral", "") != codigoDesde("Arreglo Floral", "") {
		t.Error("el código no es estable entre llamadas")
	}
}

// Al unificar, el costo del grupo es el MÍNIMO y no el promedio: un promedio no le corresponde a
// ninguna sede, y conviene equivocarse hacia el lado que no infla el margen.
func TestAlUnificarSeQuedaConElCostoMinimo(t *testing.T) {
	t.Parallel()
	filas := porSede(map[string][]string{
		"Arreglo Floral": {"7000", "6500", "6000", "5500", "5000"},
		// Un segundo producto en las mismas plazas: es lo que las identifica como sedes y no
		// como variantes del nombre.
		"Cremación de Adulto": {"100000"},
	})
	plan, err := Planificar(libro(t, "Costos", filas), "Costos", "", "", true, nil)
	if err != nil {
		t.Fatalf("Planificar: %v", err)
	}
	// Dos productos, cada uno unificado a UN insumo pese a estar en cinco sedes.
	if len(plan.Insumos) != 2 {
		t.Fatalf("insumos = %d, se esperaban 2 al unificar", len(plan.Insumos))
	}
	var flor *InsumoAImportar
	for i := range plan.Insumos {
		if plan.Insumos[i].Nombre == "Arreglo Floral" {
			flor = &plan.Insumos[i]
		}
	}
	if flor == nil {
		t.Fatalf("no se unificó el arreglo floral: %+v", plan.Insumos)
	}
	// 7000, 6500, 6000, 5500, 5000 → el mínimo es 5000.
	if flor.Costo == nil || *flor.Costo != "5000.00" {
		t.Errorf("costo unificado = %v, se esperaba 5000.00 (el mínimo, no el promedio 6000)", flor.Costo)
	}
	if flor.Sede != "" {
		t.Errorf("al unificar, el insumo no debe quedar atado a una sede: %q", flor.Sede)
	}
}

func TestSinUnificarCadaSedeEsSuPropioInsumo(t *testing.T) {
	t.Parallel()
	filas := porSede(map[string][]string{
		"Arreglo Floral":      {"7000", "6500", "6000", "5500", "5000"},
		"Cremación de Adulto": {"100000"},
	})
	plan, err := Planificar(libro(t, "Costos", filas), "Costos", "", "", false, nil)
	if err != nil {
		t.Fatalf("Planificar: %v", err)
	}
	// Dos productos × cinco sedes.
	if len(plan.Insumos) != 10 {
		t.Fatalf("insumos = %d, se esperaban 10 (dos productos por cinco sedes)", len(plan.Insumos))
	}
	codigos := map[string]bool{}
	for _, i := range plan.Insumos {
		if codigos[i.Codigo] {
			t.Errorf("código repetido: %q — se pisarían entre sí al importar", i.Codigo)
		}
		codigos[i.Codigo] = true
		if i.Sede == "" {
			t.Errorf("%q perdió su sede", i.Codigo)
		}
	}
}

func TestElImportadorRechazaLoQueNoPuedeLeer(t *testing.T) {
	t.Parallel()
	t.Run("archivo vacío", func(t *testing.T) {
		if _, err := Analizar(nil, "", "", ""); err != ErrArchivoVacio {
			t.Errorf("error = %v, se esperaba ErrArchivoVacio", err)
		}
	})
	t.Run("no es un xlsx", func(t *testing.T) {
		if _, err := Analizar([]byte("esto es un csv;de verdad"), "", "", ""); err != ErrArchivoIlegible {
			t.Errorf("error = %v, se esperaba ErrArchivoIlegible", err)
		}
	})
	t.Run("hoja que no existe", func(t *testing.T) {
		b := libro(t, "Costos", [][]string{{"id", "Descripcion"}, {"", "x"}})
		if _, err := Analizar(b, "Otra hoja", "", ""); err != ErrHojaNoEncontrada {
			t.Errorf("error = %v, se esperaba ErrHojaNoEncontrada", err)
		}
	})
	t.Run("sin columna de nombre reconocible", func(t *testing.T) {
		b := libro(t, "Costos", [][]string{{"aaa", "bbb"}, {"1", "2"}})
		if _, err := Analizar(b, "Costos", "", ""); err != ErrColumnaNombreNoHallada {
			t.Errorf("error = %v, se esperaba ErrColumnaNombreNoHallada", err)
		}
	})
}

// La lista de sedes que confirma el usuario MANDA sobre la detección automática.
//
// Esta prueba nace de un caso real: contra el archivo del usuario, la detección marcó como sedes
// «URNA DE ALUMINIO», «URNA DE ONIX» y «Sabana 12 horas» junto a las ocho plazas verdaderas. Los
// dos primeros son modelos y el tercero una duración de sala. Ningún umbral los separa —aparecen
// con varios productos, igual que una plaza—, así que la decisión tiene que poder corregirse.
func TestLasSedesConfirmadasPorElUsuarioMandanSobreLaDeteccion(t *testing.T) {
	t.Parallel()
	filas := porSede(map[string][]string{
		"Arreglo Floral":      {"5000"},
		"Cremación de Adulto": {"100000"},
	})
	// Un falso positivo como los del archivo real: aparece con dos productos distintos, así que
	// la detección automática lo tomaría por sede.
	filas = append(filas,
		[]string{"", "Baul cenizas-URNA DE ALUMINIO", "17000"},
		[]string{"", "Urna adulto-URNA DE ALUMINIO", "18000"},
	)
	b := libro(t, "Costos", filas)

	t.Run("sin confirmar, la detección lo toma por sede", func(t *testing.T) {
		a, err := Analizar(b, "Costos", "", "")
		if err != nil {
			t.Fatalf("Analizar: %v", err)
		}
		hay := false
		for _, s := range a.SedesDetectadas {
			if s == "URNA DE ALUMINIO" {
				hay = true
			}
		}
		if !hay {
			t.Skip("la detección ya no marca este caso; la prueba perdió su objeto")
		}
	})

	t.Run("al confirmar solo las plazas reales, el nombre queda entero", func(t *testing.T) {
		plan, err := Planificar(b, "Costos", "", "", true, SEDES)
		if err != nil {
			t.Fatalf("Planificar: %v", err)
		}
		entero := 0
		for _, i := range plan.Insumos {
			if i.Nombre == "Baul cenizas-URNA DE ALUMINIO" || i.Nombre == "Urna adulto-URNA DE ALUMINIO" {
				entero++
			}
			if i.Nombre == "Baul cenizas" || i.Nombre == "Urna adulto" {
				t.Errorf("partió %q usando un sufijo que el usuario NO confirmó como sede", i.Nombre)
			}
		}
		if entero != 2 {
			t.Errorf("los dos nombres con sufijo de modelo debían quedar enteros; quedaron %d", entero)
		}
	})
}
