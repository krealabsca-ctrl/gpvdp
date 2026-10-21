package precios

// Importador del catálogo desde el archivo de costeo.
//
// ── POR QUÉ EXISTE ──────────────────────────────────────────────────────────────────────────
//
// El catálogo nace vacío y el archivo vigente tiene 693 líneas. Cargarlas a mano no lo va a
// hacer nadie, así que sin importador el módulo se queda en una pantalla vacía.
//
// ── EL PROBLEMA DE LAS SEDES, QUE ES LO QUE HACE FALTA DECIDIR ──────────────────────────────
//
// El archivo repite cada ítem por sede: «Cremación de un Adulto-Limon», «-Liberia», «-Heredia».
// Medido: 63 sufijos distintos, con Heredia en 103 líneas y Sabana en 84.
//
// Eso puede significar dos cosas y el archivo no lo dice:
//
//   · Que el costo es el MISMO y la repetición es ruido del formato → 693 líneas son ~90 insumos.
//   · Que el costo CAMBIA por sede (en flores y salas es verosímil) → son 693 insumos distintos.
//
// El importador NO elige. Mide las dos lecturas, muestra en cuántos ítems los costos de las
// sedes coinciden y en cuántos difieren, y deja que una persona decida con ese número a la vista.
// Elegir por el usuario sería inventar una regla de negocio que nadie pidió.

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// Los dos umbrales con que se decide si un sufijo es una sede o parte del nombre del producto.
// «Cofre Standard -COFRE PALOMA» también tiene guion, y partirlo sería un error.
const (
	// Un sufijo que acompaña a 2+ productos distintos es una plaza, no un modelo: «Limon» va con
	// cremación, flores y cofres, mientras que «COFRE PALOMA» va solo con «Cofre Standard ».
	minProductosParaSerSede = 2
	// Respaldo por volumen, para la plaza que aparece muchas veces con un solo producto.
	minLineasParaSerSede = 5
)

// FilaImportada es una línea del archivo ya interpretada.
type FilaImportada struct {
	Fila        int    `json:"fila"`
	NombreCrudo string `json:"nombre_crudo"`
	// NombreBase es el nombre sin el sufijo de sede; igual al crudo cuando no hay sufijo.
	NombreBase string  `json:"nombre_base"`
	Sede       string  `json:"sede"`
	Costo      *string `json:"costo"`
	Motivo     string  `json:"motivo"`
}

// GrupoDeInsumo son todas las líneas del archivo que comparten nombre base.
type GrupoDeInsumo struct {
	NombreBase string   `json:"nombre_base"`
	Lineas     int      `json:"lineas"`
	Sedes      []string `json:"sedes"`
	ConCosto   int      `json:"con_costo"`
	// CostosDistintos es la cantidad de valores de costo diferentes entre las sedes. Es el número
	// que decide si unificar es seguro: 0 o 1 significa que todas las sedes cuestan igual.
	CostosDistintos int     `json:"costos_distintos"`
	CostoMinimo     *string `json:"costo_minimo"`
	CostoMaximo     *string `json:"costo_maximo"`
}

// AnalisisImportacion es lo que se le muestra al usuario ANTES de escribir nada.
type Analisis struct {
	Hoja          string   `json:"hoja"`
	HojasDelLibro []string `json:"hojas_del_libro"`
	ColumnaNombre string   `json:"columna_nombre"`
	ColumnaCosto  string   `json:"columna_costo"`

	LineasTotales  int `json:"lineas_totales"`
	LineasConCosto int `json:"lineas_con_costo"`
	LineasSinCosto int `json:"lineas_sin_costo"`

	// Las dos lecturas posibles, medidas.
	InsumosSiSeUnifica     int      `json:"insumos_si_se_unifica"`
	InsumosSiNoSeUnifica   int      `json:"insumos_si_no_se_unifica"`
	SedesDetectadas        []string `json:"sedes_detectadas"`
	GruposConCostoIgual    int      `json:"grupos_con_costo_igual"`
	GruposConCostoDistinto int      `json:"grupos_con_costo_distinto"`

	// Los grupos donde las sedes NO coinciden: son los que harían perder información al unificar.
	Conflictos []GrupoDeInsumo `json:"conflictos"`
	Muestra    []FilaImportada `json:"muestra"`
}

// LeerLibro abre el archivo y devuelve los nombres de sus hojas.
func LeerLibro(archivo []byte) ([]string, error) {
	if len(archivo) == 0 {
		return nil, ErrArchivoVacio
	}
	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return nil, ErrArchivoIlegible
	}
	defer func() { _ = f.Close() }()
	hojas := f.GetSheetList()
	if len(hojas) == 0 {
		return nil, ErrArchivoVacio
	}
	return hojas, nil
}

// normalizarMonto limpia un número tal como viene del archivo: separadores de miles, símbolo de
// moneda y espacios. Devuelve nil cuando no hay un número utilizable — que NO es lo mismo que
// cero: un costo ausente deja el insumo sin costear, y cero diría que es gratis.
func normalizarMonto(s string) *decimal.Decimal {
	t := strings.TrimSpace(s)
	t = strings.NewReplacer(",", "", "₡", "", "$", "", " ", "", " ", "").Replace(t)
	if t == "" || t == "-" {
		return nil
	}
	v, err := decimal.NewFromString(t)
	if err != nil || v.IsNegative() || v.IsZero() {
		return nil
	}
	return &v
}

// partirSede separa «Cremación de un Adulto-Limon» en base y sede, usando los sufijos que el
// análisis ya determinó que son sedes.
func partirSede(nombre string, sedes map[string]bool) (string, string) {
	p := strings.LastIndex(nombre, "-")
	if p <= 0 {
		return nombre, ""
	}
	posible := strings.TrimSpace(nombre[p+1:])
	if posible == "" || !sedes[strings.ToLower(posible)] {
		return nombre, ""
	}
	return strings.TrimSpace(nombre[:p]), posible
}

// detectarSedes decide qué sufijos son una sede y cuáles son parte del nombre del producto.
//
// ── EL CRITERIO, Y POR QUÉ ESTE ─────────────────────────────────────────────────────────────
//
// Lo que distingue una sede de un modelo es CON CUÁNTOS PRODUCTOS DISTINTOS aparece:
//
//	· «Limon» acompaña a cremación, flores, cofres y salas → es una plaza.
//	· «COFRE PALOMA» acompaña solo a «Cofre Standard » → es el modelo del cofre.
//
// Contar apariciones a secas no alcanza: un modelo que se repite mucho se tomaría por sede. Por
// eso manda el número de nombres base distintos, y el conteo de líneas queda como respaldo para
// el caso en que una plaza aparezca muchas veces con un solo producto.
//
// Queda un caso que NO se puede resolver y conviene nombrarlo: un archivo con un único producto
// repartido en varias sedes es indistinguible de un producto con variantes. Ahí el importador no
// parte nada, y las líneas entran con su nombre completo — que es el error reversible, porque
// unir después es editar, mientras que partir de más pierde la distinción entre plazas.
func detectarSedes(nombres []string) map[string]bool {
	basesPorSufijo := map[string]map[string]bool{}
	lineasPorSufijo := map[string]int{}
	for _, n := range nombres {
		p := strings.LastIndex(n, "-")
		if p <= 0 {
			continue
		}
		s := strings.TrimSpace(n[p+1:])
		if s == "" {
			continue
		}
		k := strings.ToLower(s)
		if basesPorSufijo[k] == nil {
			basesPorSufijo[k] = map[string]bool{}
		}
		basesPorSufijo[k][strings.ToLower(strings.TrimSpace(n[:p]))] = true
		lineasPorSufijo[k]++
	}
	out := map[string]bool{}
	for k, bases := range basesPorSufijo {
		if len(bases) >= minProductosParaSerSede || lineasPorSufijo[k] >= minLineasParaSerSede {
			out[k] = true
		}
	}
	return out
}

// columnaDe busca una columna por palabras clave en la fila de encabezado. Devuelve la letra.
func columnaDe(encabezado []string, claves ...string) string {
	for i, celda := range encabezado {
		c := strings.ToLower(strings.TrimSpace(celda))
		if c == "" {
			continue
		}
		for _, k := range claves {
			if strings.Contains(c, k) {
				letra, _ := excelize.ColumnNumberToName(i + 1)
				return letra
			}
		}
	}
	return ""
}

func indiceDeColumna(letra string) int {
	n, err := excelize.ColumnNameToNumber(letra)
	if err != nil {
		return -1
	}
	return n - 1
}

// Analizar lee una hoja y mide qué pasaría al importarla, sin escribir nada.
//
// `colNombre` y `colCosto` son opcionales: si vienen vacías se detectan por encabezado. Se
// permiten explícitas porque el archivo real tiene columnas con nombres poco estándar y es mejor
// que el usuario las señale a que el importador adivine mal en silencio.
func Analizar(archivo []byte, hoja, colNombre, colCosto string) (Analisis, error) {
	hojas, err := LeerLibro(archivo)
	if err != nil {
		return Analisis{}, err
	}
	if hoja == "" {
		hoja = hojas[0]
	}
	existe := false
	for _, h := range hojas {
		if h == hoja {
			existe = true
		}
	}
	if !existe {
		return Analisis{}, ErrHojaNoEncontrada
	}

	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return Analisis{}, ErrArchivoIlegible
	}
	defer func() { _ = f.Close() }()

	rows, err := f.GetRows(hoja)
	if err != nil {
		return Analisis{}, ErrArchivoIlegible
	}
	if len(rows) < 2 {
		return Analisis{}, ErrArchivoVacio
	}

	encabezado := rows[0]
	if colNombre == "" {
		colNombre = columnaDe(encabezado, "descripcion", "descripción", "nombre", "detalle", "articulo", "artículo")
	}
	if colCosto == "" {
		colCosto = columnaDe(encabezado, "costo")
	}
	if colNombre == "" {
		return Analisis{}, ErrColumnaNombreNoHallada
	}

	iNombre, iCosto := indiceDeColumna(colNombre), indiceDeColumna(colCosto)
	if iNombre < 0 {
		return Analisis{}, ErrColumnaNombreNoHallada
	}

	// Primera pasada: juntar los nombres para saber qué sufijos son sedes.
	nombres := []string{}
	for i := 1; i < len(rows); i++ {
		if iNombre < len(rows[i]) {
			if n := strings.TrimSpace(rows[i][iNombre]); n != "" {
				nombres = append(nombres, n)
			}
		}
	}
	sedes := detectarSedes(nombres)

	// Segunda pasada: armar las filas y agruparlas.
	a := Analisis{
		Hoja: hoja, HojasDelLibro: hojas,
		ColumnaNombre: colNombre, ColumnaCosto: colCosto,
		Conflictos: []GrupoDeInsumo{}, Muestra: []FilaImportada{}, SedesDetectadas: []string{},
	}
	grupos := map[string]*GrupoDeInsumo{}
	costosPorGrupo := map[string]map[string]bool{}
	sedesVistas := map[string]bool{}

	for i := 1; i < len(rows); i++ {
		if iNombre >= len(rows[i]) {
			continue
		}
		crudo := strings.TrimSpace(rows[i][iNombre])
		if crudo == "" {
			continue
		}
		base, sede := partirSede(crudo, sedes)
		var costo *decimal.Decimal
		if iCosto >= 0 && iCosto < len(rows[i]) {
			costo = normalizarMonto(rows[i][iCosto])
		}

		fila := FilaImportada{Fila: i + 1, NombreCrudo: crudo, NombreBase: base, Sede: sede}
		if costo != nil {
			s := costo.StringFixed(2)
			fila.Costo = &s
			a.LineasConCosto++
		} else {
			fila.Motivo = "sin costo en el archivo"
			a.LineasSinCosto++
		}
		a.LineasTotales++
		if sede != "" {
			sedesVistas[sede] = true
		}
		if len(a.Muestra) < 12 {
			a.Muestra = append(a.Muestra, fila)
		}

		g, ok := grupos[base]
		if !ok {
			g = &GrupoDeInsumo{NombreBase: base, Sedes: []string{}}
			grupos[base] = g
			costosPorGrupo[base] = map[string]bool{}
		}
		g.Lineas++
		if sede != "" {
			g.Sedes = append(g.Sedes, sede)
		}
		if costo != nil {
			g.ConCosto++
			costosPorGrupo[base][costo.StringFixed(2)] = true
			if g.CostoMinimo == nil || costo.LessThan(decimal.RequireFromString(*g.CostoMinimo)) {
				s := costo.StringFixed(2)
				g.CostoMinimo = &s
			}
			if g.CostoMaximo == nil || costo.GreaterThan(decimal.RequireFromString(*g.CostoMaximo)) {
				s := costo.StringFixed(2)
				g.CostoMaximo = &s
			}
		}
	}

	for base, g := range grupos {
		g.CostosDistintos = len(costosPorGrupo[base])
		if g.CostosDistintos > 1 {
			a.GruposConCostoDistinto++
			if len(a.Conflictos) < 20 {
				a.Conflictos = append(a.Conflictos, *g)
			}
		} else {
			a.GruposConCostoIgual++
		}
	}
	a.InsumosSiSeUnifica = len(grupos)
	a.InsumosSiNoSeUnifica = a.LineasTotales
	for s := range sedesVistas {
		a.SedesDetectadas = append(a.SedesDetectadas, s)
	}
	sort.Strings(a.SedesDetectadas)
	sort.Slice(a.Conflictos, func(i, j int) bool {
		return a.Conflictos[i].CostosDistintos > a.Conflictos[j].CostosDistintos
	})
	return a, nil
}

// codigoDesde arma un código estable a partir del nombre: mayúsculas, sin acentos ni símbolos.
// Estable importa porque es la llave que hace la importación idempotente — correrla dos veces
// tiene que actualizar las mismas filas, no duplicarlas.
func codigoDesde(nombre string, sede string) string {
	base := strings.ToUpper(strings.TrimSpace(nombre))
	base = strings.NewReplacer(
		"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ñ", "N", "Ü", "U",
	).Replace(base)
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '/':
			b.WriteRune('-')
		}
	}
	c := strings.Trim(b.String(), "-")
	for strings.Contains(c, "--") {
		c = strings.ReplaceAll(c, "--", "-")
	}
	if len(c) > 48 {
		c = c[:48]
	}
	if sede != "" {
		s := strings.ToUpper(strings.TrimSpace(sede))
		if len(s) > 8 {
			s = s[:8]
		}
		c = c + "-" + s
	}
	if c == "" {
		return "SIN-CODIGO"
	}
	return c
}

// PlanDeImportacion es lo que se va a escribir, ya resuelto. Se calcula ANTES de tocar la base
// para poder decir cuántos insumos y cuántos costos van a entrar, y exigir esa confirmación.
type PlanDeImportacion struct {
	Insumos  []InsumoAImportar `json:"insumos"`
	ConCosto int               `json:"con_costo"`
	SinCosto int               `json:"sin_costo"`
}

// InsumoAImportar es una línea del plan.
type InsumoAImportar struct {
	Codigo string  `json:"codigo"`
	Nombre string  `json:"nombre"`
	Sede   string  `json:"sede"`
	Costo  *string `json:"costo"`
}

// Planificar convierte el análisis en la lista exacta de insumos a escribir.
//
// Al unificar, el costo del grupo es el MÍNIMO de las sedes y no el promedio: un promedio es un
// número que no le corresponde a ninguna sede, y para costear conviene equivocarse hacia el lado
// que no infla el margen.
// `sedesElegidas` acota qué sufijos tratar como sede. Vacío significa «usá los que detectaste».
//
// Existe porque la detección automática se equivoca con datos reales, y se comprobó contra el
// archivo del usuario: además de las 8 plazas verdaderas marcó «URNA DE ALUMINIO», «URNA DE ONIX»
// y «Sabana 12 horas». Los dos primeros son modelos de urna y el tercero es una duración de sala;
// partirlos rompería nombres legítimos. Ningún umbral los separa bien —aparecen con varios
// productos, igual que una plaza—, así que la lista la confirma una persona, que es la única que
// sabe cuáles de sus plazas son plazas.
func Planificar(archivo []byte, hoja, colNombre, colCosto string, unificarSedes bool, sedesElegidas []string) (PlanDeImportacion, error) {
	a, err := Analizar(archivo, hoja, colNombre, colCosto)
	if err != nil {
		return PlanDeImportacion{}, err
	}

	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return PlanDeImportacion{}, ErrArchivoIlegible
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(a.Hoja)
	if err != nil {
		return PlanDeImportacion{}, ErrArchivoIlegible
	}

	iNombre, iCosto := indiceDeColumna(a.ColumnaNombre), indiceDeColumna(a.ColumnaCosto)
	nombres := []string{}
	for i := 1; i < len(rows); i++ {
		if iNombre < len(rows[i]) {
			if n := strings.TrimSpace(rows[i][iNombre]); n != "" {
				nombres = append(nombres, n)
			}
		}
	}
	// Si el usuario confirmó una lista, manda esa: la detección automática solo propone.
	sedes := detectarSedes(nombres)
	if len(sedesElegidas) > 0 {
		sedes = map[string]bool{}
		for _, s := range sedesElegidas {
			if s = strings.TrimSpace(s); s != "" {
				sedes[strings.ToLower(s)] = true
			}
		}
	}

	porCodigo := map[string]*InsumoAImportar{}
	orden := []string{}
	for i := 1; i < len(rows); i++ {
		if iNombre >= len(rows[i]) {
			continue
		}
		crudo := strings.TrimSpace(rows[i][iNombre])
		if crudo == "" {
			continue
		}
		base, sede := partirSede(crudo, sedes)
		nombre, sedeDelCodigo := crudo, ""
		if unificarSedes {
			nombre, sedeDelCodigo = base, ""
		} else if sede != "" {
			nombre, sedeDelCodigo = base, sede
		}
		cod := codigoDesde(nombre, sedeDelCodigo)

		var costo *decimal.Decimal
		if iCosto >= 0 && iCosto < len(rows[i]) {
			costo = normalizarMonto(rows[i][iCosto])
		}

		ex, ok := porCodigo[cod]
		if !ok {
			ins := InsumoAImportar{Codigo: cod, Nombre: nombre, Sede: sedeDelCodigo}
			if costo != nil {
				s := costo.StringFixed(2)
				ins.Costo = &s
			}
			porCodigo[cod] = &ins
			orden = append(orden, cod)
			continue
		}
		// Ya existe: se queda con el costo MÍNIMO de las líneas que caen en el mismo código.
		if costo != nil {
			if ex.Costo == nil || costo.LessThan(decimal.RequireFromString(*ex.Costo)) {
				s := costo.StringFixed(2)
				ex.Costo = &s
			}
		}
	}

	p := PlanDeImportacion{Insumos: make([]InsumoAImportar, 0, len(orden))}
	for _, c := range orden {
		ins := *porCodigo[c]
		if ins.Costo != nil {
			p.ConCosto++
		} else {
			p.SinCosto++
		}
		p.Insumos = append(p.Insumos, ins)
	}
	return p, nil
}

// Resumen imprime el plan en una línea, para el log.
func (p PlanDeImportacion) Resumen() string {
	return fmt.Sprintf("%d insumos (%d con costo, %d sin costo)", len(p.Insumos), p.ConCosto, p.SinCosto)
}
