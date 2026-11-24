package bancos

// El análisis exportado a Excel.
//
// Lo que se prueba acá no es que excelize sepa escribir un archivo, sino que el archivo que sale
// del sistema diga lo mismo que la pantalla: las mismas partidas, el mismo rango y —sobre todo—
// las mismas advertencias. Un .xlsx se reenvía y se abre semanas después sin la pantalla al lado.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func abrirLibro(t *testing.T, buf []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("abrir el libro: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// textoDeHoja junta todas las celdas de una hoja, para buscar sin depender de coordenadas.
func textoDeHoja(t *testing.T, f *excelize.File, hoja string) string {
	t.Helper()
	filas, err := f.GetRows(hoja)
	if err != nil {
		t.Fatalf("leer la hoja %q: %v", hoja, err)
	}
	var b strings.Builder
	for _, fila := range filas {
		b.WriteString(strings.Join(fila, "|"))
		b.WriteString("\n")
	}
	return b.String()
}

func servicioParaExport() *Service {
	repo := &fakeRepo{
		saludPorRango: map[string][]SaludMes{
			"2026-01..2026-03": {mesSano("2026-01"), mesFlojo("2026-02"), mesSano("2026-03")},
		},
		seriePorRango: map[string][]TendenciaPartida{
			"2026-01..2026-03": {
				partida("c1", "Planilla", map[string]string{
					"2026-01": "3000000", "2026-02": "3000000", "2026-03": "4000000"}),
				partida("c2", "Gas", map[string]string{
					"2026-01": "500000", "2026-02": "500000", "2026-03": "500000"}),
			},
		},
	}
	return &Service{repo: repo}
}

func TestElAnalisisExportadoTraeLasTresHojas(t *testing.T) {
	svc := servicioParaExport()
	buf, nombre, err := svc.ExportarAnalisis(t.Context(), "e1", "2026-01", "2026-03", "GASTO", false, "u1")
	if err != nil {
		t.Fatalf("exportar: %v", err)
	}
	if !strings.HasSuffix(nombre, ".xlsx") {
		t.Errorf("el nombre del archivo no es un .xlsx: %q", nombre)
	}
	f := abrirLibro(t, buf)

	for _, hoja := range []string{"Tendencia", "Concentración", "Salud"} {
		if _, err := f.GetRows(hoja); err != nil {
			t.Errorf("falta la hoja %q: %v", hoja, err)
		}
	}

	// La tabla trae una columna por mes, con el nombre del mes.
	tend := textoDeHoja(t, f, "Tendencia")
	for _, mes := range []string{"Enero 2026", "Febrero 2026", "Marzo 2026"} {
		if !strings.Contains(tend, mes) {
			t.Errorf("la hoja Tendencia no tiene la columna de %s", mes)
		}
	}
	if !strings.Contains(tend, "Planilla") || !strings.Contains(tend, "Gas") {
		t.Error("la hoja Tendencia no trae las partidas")
	}
}

// EL MES A MEDIO CLASIFICAR SE MARCA EN SU PROPIA COLUMNA. Es el único lugar donde la advertencia
// viaja pegada al número que afecta: en una tabla de 24 columnas, una nota al pie no se asocia.
func TestElMesIncompletoSeMarcaEnSuColumna(t *testing.T) {
	svc := servicioParaExport()
	buf, _, err := svc.ExportarAnalisis(t.Context(), "e1", "2026-01", "2026-03", "GASTO", false, "u1")
	if err != nil {
		t.Fatalf("exportar: %v", err)
	}
	tend := textoDeHoja(t, abrirLibro(t, buf), "Tendencia")
	if !strings.Contains(tend, "Febrero 2026 (parcial)") {
		t.Error("febrero está al 40 % y su columna no quedó marcada como parcial")
	}
	if strings.Contains(tend, "Enero 2026 (parcial)") {
		t.Error("enero está al 95 % y quedó marcado como parcial")
	}

	// Y la hoja Salud explica por qué quedó fuera.
	salud := textoDeHoja(t, abrirLibro(t, buf), "Salud")
	if !strings.Contains(salud, "menos gasto del que tuvo") {
		t.Error("la hoja Salud no explica el efecto de un mes a medio clasificar")
	}
}

// LA CONCENTRACIÓN ES OTRA PREGUNTA: «¿dónde está el dinero?» no es «¿qué se movió?». Medido en
// Memorial Pets, 10 partidas de 116 explican el 73 % del gasto.
func TestLaHojaDeConcentracionOrdenaPorMontoYAcumula(t *testing.T) {
	svc := servicioParaExport()
	buf, _, err := svc.ExportarAnalisis(t.Context(), "e1", "2026-01", "2026-03", "GASTO", false, "u1")
	if err != nil {
		t.Fatalf("exportar: %v", err)
	}
	f := abrirLibro(t, buf)
	filas, err := f.GetRows("Concentración")
	if err != nil {
		t.Fatalf("leer Concentración: %v", err)
	}
	// Planilla (10 M) tiene que ir antes que Gas (1,5 M), aunque en la lista venga después.
	//
	// Se compara CELDA POR CELDA y no con Contains sobre la fila entera: el concepto de las dos
	// partidas es «Gastos», que contiene «Gas», así que buscar la subcadena encuentra la fila de
	// Planilla primero y la prueba falla por una razón falsa. Es el mismo tropiezo que ya costó
	// una vez en el buscador de clasificaciones.
	filaDe := func(partida string) int {
		for i, fila := range filas {
			for _, celda := range fila {
				if celda == partida {
					return i
				}
			}
		}
		return -1
	}
	iPlanilla, iGas := filaDe("Planilla"), filaDe("Gas")
	if iPlanilla < 0 || iGas < 0 {
		t.Fatalf("faltan partidas en Concentración: planilla=%d gas=%d", iPlanilla, iGas)
	}
	if iPlanilla > iGas {
		t.Error("la hoja no está ordenada por monto: Gas quedó antes que Planilla")
	}
	// Y el acumulado cierra en 100 %.
	texto := textoDeHoja(t, f, "Concentración")
	if !strings.Contains(texto, "100.0 %") {
		t.Error("el acumulado no llega a 100 %")
	}
}

// El filtro del archivo tiene que ser el mismo de la pantalla: un .xlsx que trae otra cosa que lo
// que se está mirando es peor que no tener archivo.
func TestElExportRespetaElFiltroDeNaturaleza(t *testing.T) {
	repo := &fakeRepo{
		saludPorRango: map[string][]SaludMes{"2026-01..2026-01": {mesSano("2026-01")}},
		seriePorRango: map[string][]TendenciaPartida{"2026-01..2026-01": {
			partida("c1", "Planilla", map[string]string{"2026-01": "100"}),
			func() TendenciaPartida {
				p := partida("c2", "Ventas", map[string]string{"2026-01": "900"})
				p.Naturaleza, p.Concepto = "INGRESO", "Ingresos"
				return p
			}(),
			func() TendenciaPartida {
				p := partida("c3", "Sin decidir", map[string]string{"2026-01": "50"})
				p.NaturalezaDeclarada = false
				return p
			}(),
		}},
	}
	svc := &Service{repo: repo}

	buf, _, err := svc.ExportarAnalisis(t.Context(), "e1", "2026-01", "2026-01", "GASTO", false, "u1")
	if err != nil {
		t.Fatalf("exportar gastos: %v", err)
	}
	tend := textoDeHoja(t, abrirLibro(t, buf), "Tendencia")
	if !strings.Contains(tend, "Planilla") {
		t.Error("el export de gastos no trae la partida de gasto")
	}
	if strings.Contains(tend, "Ventas") {
		t.Error("el export de gastos trajo una partida de INGRESO")
	}
	// Una naturaleza sin declarar NO cuenta como su valor por defecto: «sin decidir» no es gasto.
	if strings.Contains(tend, "Sin decidir") {
		t.Error("el export de gastos trajo una partida con la naturaleza sin declarar")
	}

	// Y el grupo propio sí la trae.
	buf2, _, err := svc.ExportarAnalisis(t.Context(), "e1", "2026-01", "2026-01", "SIN_DECLARAR", false, "u1")
	if err != nil {
		t.Fatalf("exportar sin declarar: %v", err)
	}
	if !strings.Contains(textoDeHoja(t, abrirLibro(t, buf2), "Tendencia"), "Sin decidir") {
		t.Error("el grupo «falta decidir» no trajo su partida")
	}
}

// Exportar un filtro sin partidas devuelve el error de siempre, no un archivo con encabezados y
// nada adentro: eso se abre, se mira y se concluye que no hubo gasto.
func TestExportarSinPartidasAvisa(t *testing.T) {
	repo := &fakeRepo{
		saludPorRango: map[string][]SaludMes{"2026-01..2026-01": {mesSano("2026-01")}},
		seriePorRango: map[string][]TendenciaPartida{"2026-01..2026-01": {}},
	}
	svc := &Service{repo: repo}
	if _, _, err := svc.ExportarAnalisis(t.Context(), "e1", "2026-01", "2026-01", "GASTO", false, "u1"); err == nil {
		t.Fatal("exportó un archivo vacío en vez de avisar")
	}
}

// Con la comparación anual, el archivo trae las dos columnas de más.
func TestElExportComparadoTraeElAnioAnterior(t *testing.T) {
	repo := &fakeRepo{
		saludPorRango: map[string][]SaludMes{
			"2026-01..2026-02": {mesSano("2026-01"), mesSano("2026-02")},
			"2025-01..2025-02": {mesSano("2025-01"), mesSano("2025-02")},
		},
		seriePorRango: map[string][]TendenciaPartida{
			"2026-01..2026-02": {partida("c1", "Planilla", map[string]string{"2026-01": "600", "2026-02": "600"})},
			"2025-01..2025-02": {partida("c1", "Planilla", map[string]string{"2025-01": "400", "2025-02": "400"})},
		},
	}
	svc := &Service{repo: repo}
	buf, _, err := svc.ExportarAnalisis(t.Context(), "e1", "2026-01", "2026-02", "GASTO", true, "u1")
	if err != nil {
		t.Fatalf("exportar: %v", err)
	}
	tend := textoDeHoja(t, abrirLibro(t, buf), "Tendencia")
	if !strings.Contains(tend, "Mismo rango año anterior") {
		t.Error("falta la columna del año anterior")
	}
	if !strings.Contains(tend, "50.0 %") {
		t.Errorf("falta la variación anual (800 contra 1200 = +50 %%):\n%s", tend)
	}
}
