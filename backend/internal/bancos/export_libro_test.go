package bancos

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func libroDePrueba(t *testing.T, meta MetaReporte) *excelize.File {
	t.Helper()
	cols := []ColumnaReporte{
		{Titulo: "Fecha", Ancho: 11, Tipo: "fecha"},
		{Titulo: "Descripción", Ancho: 40, Tipo: "texto"},
		{Titulo: "Débito", Ancho: 16, Tipo: "montoDebito"},
		{Titulo: "Crédito", Ancho: 16, Tipo: "monto"},
	}
	filas := []FilaReporte{
		{Grupo: "Ingresos › Datafonos", Valores: []any{"2026-07-01", "Compra POS", 0.0, 125000.5}},
		{Grupo: "Ingresos › Datafonos", Valores: []any{"2026-07-02", "Compra POS", 0.0, 98000.0}},
		{Grupo: "Gastos › Combustible", Valores: []any{"2026-07-03", "Servicentro", 45000.0, 0.0}},
	}
	buf, err := ConstruirLibro([]HojaReporte{
		{Nombre: "Movimientos", Meta: meta, Cols: cols, Filas: filas, AgruparConSubtotales: true},
	})
	if err != nil {
		t.Fatalf("ConstruirLibro: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("abrir el libro generado: %v", err)
	}
	return f
}

func metaCompleta() MetaReporte {
	return MetaReporte{
		Empresa:        "Valle de Paz Servicios Funerarios S.A.",
		EmpresaDetalle: "Sociedad Anónima",
		Cuenta:         "Cuenta: BAC · BAC Religiosa",
		Titulo:         "Detalle de movimientos bancarios",
		Periodo:        "Julio 2026",
		Filtros:        [][2]string{{"Tipo", "Débitos y créditos"}, {"Conceptos", "Ingresos · Gastos"}},
		GeneradoPor:    "Usuario de prueba",
		GeneradoEn:     time.Date(2026, 8, 10, 9, 30, 0, 0, time.UTC),
		Avisos:         []string{"Los totales están en colones."},
	}
}

// El defecto que reportó el negocio: el recuadro del encabezado se dibujaba SOLO alrededor de la
// columna A, así que se veían líneas sueltas colgando a la izquierda. La causa es que excelize
// combina las celdas pero cada una guarda su estilo: hay que estilar el RANGO, no la primera celda.
func TestEncabezadoEstilaTodoElRangoCombinado(t *testing.T) {
	t.Parallel()
	f := libroDePrueba(t, metaCompleta())
	defer func() { _ = f.Close() }()

	const hoja = "Movimientos"
	merges, err := f.GetMergeCells(hoja)
	if err != nil {
		t.Fatalf("GetMergeCells: %v", err)
	}
	if len(merges) == 0 {
		t.Fatal("el encabezado no combinó ninguna celda")
	}
	for _, mc := range merges {
		inicio := mc.GetStartAxis()
		colIni, filaIni, err := excelize.CellNameToCoordinates(inicio)
		if err != nil {
			t.Fatalf("coordenadas de %s: %v", inicio, err)
		}
		colFin, _, err := excelize.CellNameToCoordinates(mc.GetEndAxis())
		if err != nil {
			t.Fatalf("coordenadas de %s: %v", mc.GetEndAxis(), err)
		}
		esperado, err := f.GetCellStyle(hoja, inicio)
		if err != nil {
			t.Fatalf("estilo de %s: %v", inicio, err)
		}
		if esperado == 0 {
			continue // sin estilo propio: nada que propagar
		}
		for c := colIni + 1; c <= colFin; c++ {
			celda, _ := excelize.CoordinatesToCellName(c, filaIni)
			got, err := f.GetCellStyle(hoja, celda)
			if err != nil {
				t.Fatalf("estilo de %s: %v", celda, err)
			}
			if got != esperado {
				t.Errorf("rango %s:%s — %s tiene estilo %d y %s tiene %d: el borde se dibuja solo en la primera celda",
					inicio, mc.GetEndAxis(), inicio, esperado, celda, got)
			}
		}
	}
}

// El encabezado tiene que ser CORTO: dos filas, y la tabla arranca en la tercera.
//
// Antes ocupaba nueve filas y el usuario borraba «de la 2 a la 9» cada vez que trabajaba el
// archivo. Este test fija el tamaño para que no vuelva a crecer sin que alguien lo decida.
func TestEncabezadoOcupaDosFilas(t *testing.T) {
	t.Parallel()

	f := libroDePrueba(t, metaCompleta())
	defer func() { _ = f.Close() }()

	const hoja = "Movimientos"
	// Fila 1: empresa + qué es + período, todo junto.
	a1, _ := f.GetCellValue(hoja, "A1")
	for _, esperado := range []string{"VALLE DE PAZ", "Detalle de movimientos", "Julio 2026"} {
		if !strings.Contains(a1, esperado) {
			t.Errorf("A1 = %q, tiene que incluir %q", a1, esperado)
		}
	}
	// Fila 2: el contexto —avisos primero, después los filtros y quién lo emitió.
	a2, _ := f.GetCellValue(hoja, "A2")
	for _, esperado := range []string{"colones", "Tipo: Débitos y créditos", "Emitido por"} {
		if !strings.Contains(a2, esperado) {
			t.Errorf("A2 = %q, tiene que incluir %q", a2, esperado)
		}
	}
	// Fila 3: ya es la tabla. Si aparece un título o una fila de aire acá, el encabezado creció.
	a3, _ := f.GetCellValue(hoja, "A3")
	if a3 != "Fecha" {
		t.Errorf("A3 = %q, quiere \"Fecha\": la tabla tiene que empezar en la fila 3", a3)
	}
}

// Sin tipo legal, sin cuenta, sin filtros y sin avisos no queda NADA que poner en la fila de
// contexto: esa fila tiene que quedar limpia —sin texto y sin estilo— y la tabla subir a la 2.
// Una celda en blanco que arrastra borde o relleno es exactamente la línea suelta que se veía.
func TestEncabezadoNoDejaCeldasVaciasConFormato(t *testing.T) {
	t.Parallel()

	m := metaCompleta()
	m.EmpresaDetalle = ""
	m.Cuenta = ""
	m.Filtros = nil
	m.Avisos = nil
	m.GeneradoPor = ""
	f := libroDePrueba(t, m)
	defer func() { _ = f.Close() }()

	const hoja = "Movimientos"
	// La razón social va siempre en la primera fila.
	if v, _ := f.GetCellValue(hoja, "A1"); v == "" {
		t.Error("A1 quedó vacía: la razón social tiene que ir siempre en la primera fila")
	}
	// Sin contexto, la fila 2 es ya la tabla: no puede haber una fila en blanco con formato.
	v2, _ := f.GetCellValue(hoja, "A2")
	if v2 != "Fecha" {
		t.Errorf("A2 = %q, quiere \"Fecha\": sin contexto la tabla sube a la fila 2", v2)
	}
}

// movsDePrueba: dos partidas, dos cuentas, fechas intercaladas y un movimiento sin clasificar.
// Las fechas NO están ordenadas por partida a propósito: así se ve si el corrido queda cronológico
// y el agrupado queda contiguo por partida.
func movsDePrueba() []MovimientoExport {
	return []MovimientoExport{
		{Fecha: "2026-07-01", Documento: "1001", Descripcion: "POS Datafono", Banco: "BAC", Cuenta: "BAC Religiosa",
			Debito: "0", Credito: "125000.50", Moneda: "CRC", MontoCRC: "125000.50",
			Concepto: "Ingresos", Clasificacion: "Datafonos", Estado: "REVISADO"},
		{Fecha: "2026-07-02", Documento: "1002", Descripcion: "Servicentro", Banco: "BN", Cuenta: "BN Colones",
			Debito: "45000", Credito: "0", Moneda: "CRC", MontoCRC: "45000",
			Concepto: "Gastos", Clasificacion: "Combustible", Estado: "REVISADO"},
		{Fecha: "2026-07-03", Documento: "1003", Descripcion: "POS Datafono", Banco: "BAC", Cuenta: "BAC Religiosa",
			Debito: "0", Credito: "98000", Moneda: "CRC", MontoCRC: "98000",
			Concepto: "Ingresos", Clasificacion: "Datafonos", Estado: "AUTO"},
		{Fecha: "2026-07-04", Documento: "1004", Descripcion: "Depósito sin identificar", Banco: "BN", Cuenta: "BN Colones",
			Debito: "0", Credito: "7000", Moneda: "CRC", MontoCRC: "7000",
			Estado: "NO_IDENTIFICADO"},
	}
}

func hojaEscrita(t *testing.T, h HojaReporte) *excelize.File {
	t.Helper()
	buf, err := ConstruirLibro([]HojaReporte{h})
	if err != nil {
		t.Fatalf("ConstruirLibro: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("abrir el libro: %v", err)
	}
	return f
}

// El listado corrido no lleva bandas ni subtotales, va en orden de fecha y nombra la partida en
// columnas: sin banda que la diga, quitar los subtotales no puede costar saber de qué partida es
// cada movimiento.
func TestListadoCorridoNoLlevaBandasNiSubtotales(t *testing.T) {
	t.Parallel()
	movs := movsDePrueba()
	h := hojaDetalleMovimientos(metaCompleta(), movs, false)

	if h.AgruparConSubtotales {
		t.Error("el listado corrido no debe pedir agrupación")
	}
	for _, fila := range h.Filas {
		if fila.Grupo != "" {
			t.Errorf("una fila del corrido trae Grupo %q: eso abre una banda", fila.Grupo)
		}
	}
	titulos := make([]string, len(h.Cols))
	for i, c := range h.Cols {
		titulos[i] = c.Titulo
	}
	for _, quiero := range []string{"Concepto", "Clasificación"} {
		if !contieneStr(titulos, quiero) {
			t.Errorf("falta la columna %q en el corrido: %v", quiero, titulos)
		}
	}

	f := hojaEscrita(t, h)
	defer func() { _ = f.Close() }()
	const hoja = "Movimientos"

	merges, _ := f.GetMergeCells(hoja)
	for _, mc := range merges {
		if strings.HasPrefix(mc.GetCellValue(), "Subtotal ") {
			t.Errorf("apareció un subtotal en el listado corrido: %q", mc.GetCellValue())
		}
	}
	filas, _ := f.GetRows(hoja)
	subtotales, totales := 0, 0
	for _, fila := range filas {
		if len(fila) == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(fila[0], "Subtotal "):
			subtotales++
		case strings.HasPrefix(fila[0], "TOTAL ·"):
			totales++
		}
	}
	if subtotales != 0 {
		t.Errorf("%d filas de subtotal en el corrido; debería haber 0", subtotales)
	}
	if totales != 1 {
		t.Errorf("%d filas de TOTAL; el gran total va en las dos presentaciones, una sola vez", totales)
	}
	// Orden cronológico: el corrido respeta el orden en que vienen los movimientos.
	if got := valorFila(filas, "1001", 0); got != "01/07/2026" {
		t.Errorf("el primer movimiento del corrido es %q, se esperaba 01/07/2026", got)
	}
	// Lo no clasificado se nombra en la columna de concepto.
	if got := valorFila(filas, "1004", 5); got != "Sin clasificar" {
		t.Errorf("el movimiento sin clasificar dice %q en Concepto, se esperaba «Sin clasificar»", got)
	}
}

// Las dos presentaciones tienen que dar los MISMOS números: la misma cantidad de movimientos y el
// mismo gran total. Si cambiar la vista cambia el total, el reporte no sirve para nada.
func TestAgrupadoYCorridoCoincidenEnMovimientosYTotal(t *testing.T) {
	t.Parallel()
	movs := movsDePrueba()

	agr := hojaDetalleMovimientos(metaCompleta(), movs, true)
	cor := hojaDetalleMovimientos(metaCompleta(), movs, false)

	if len(agr.Filas) != len(movs) || len(cor.Filas) != len(movs) {
		t.Fatalf("filas: agrupado %d, corrido %d, movimientos %d", len(agr.Filas), len(cor.Filas), len(movs))
	}

	// El equivalente en colones es la última columna de monto en las dos, pero en posiciones
	// distintas: se busca por título para no depender del orden.
	sumaCRC := func(h HojaReporte) float64 {
		idx := -1
		for i, c := range h.Cols {
			if c.Titulo == "Equivalente CRC" {
				idx = i
			}
		}
		if idx < 0 {
			t.Fatal("no hay columna «Equivalente CRC»")
		}
		total := 0.0
		for _, fila := range h.Filas {
			if idx < len(fila.Valores) {
				if v, ok := fila.Valores[idx].(float64); ok {
					total += v
				}
			}
		}
		return total
	}
	if a, c := sumaCRC(agr), sumaCRC(cor); a != c {
		t.Errorf("el total en colones difiere: agrupado %.2f vs corrido %.2f", a, c)
	}

	// Y el conjunto de documentos es el mismo (el agrupado reordena, no descarta ni duplica).
	docs := func(h HojaReporte) map[string]int {
		idx := -1
		for i, c := range h.Cols {
			if c.Titulo == "Documento" {
				idx = i
			}
		}
		out := map[string]int{}
		for _, fila := range h.Filas {
			if idx >= 0 && idx < len(fila.Valores) {
				if s, ok := fila.Valores[idx].(string); ok {
					out[s]++
				}
			}
		}
		return out
	}
	da, dc := docs(agr), docs(cor)
	if len(da) != len(dc) {
		t.Fatalf("documentos distintos: agrupado %d, corrido %d", len(da), len(dc))
	}
	for doc, n := range da {
		if dc[doc] != n {
			t.Errorf("el documento %q aparece %d veces agrupado y %d corrido", doc, n, dc[doc])
		}
	}
}

// La línea «Presentación» del encabezado va SOLO en la hoja de detalle. Estampársela a las hojas
// de resumen las hacía afirmar algo que no aplica: un resumen no tiene presentación que elegir, y
// las dos versiones del libro mostraban resúmenes que decían cosas distintas siendo idénticos.
func TestLaPresentacionSoloSeDeclaraEnElDetalle(t *testing.T) {
	t.Parallel()
	movs := movsDePrueba()
	base := metaCompleta()

	declara := func(m MetaReporte) string {
		for _, kv := range m.Filtros {
			if kv[0] == "Presentación" {
				return kv[1]
			}
		}
		return ""
	}

	agr := hojaDetalleMovimientos(base, movs, true)
	cor := hojaDetalleMovimientos(base, movs, false)
	if got := declara(agr.Meta); got != "Agrupado por partida con subtotales" {
		t.Errorf("el detalle agrupado declara %q", got)
	}
	if got := declara(cor.Meta); got != "Listado corrido por fecha" {
		t.Errorf("el detalle corrido declara %q", got)
	}
	// La meta que se le pasó no se ensucia: los resúmenes la reciben limpia.
	if got := declara(base); got != "" {
		t.Errorf("la meta original quedó con «Presentación: %s»: las hojas de resumen la heredarían", got)
	}
	for _, h := range []HojaReporte{hojaResumenPorPartida(base, movs), hojaResumenPorCuenta(base, movs)} {
		if got := declara(h.Meta); got != "" {
			t.Errorf("«%s» declara «Presentación: %s» y no debería", h.Nombre, got)
		}
	}
}

// El agrupado sí lleva una banda y un subtotal por partida, y las filas de una partida quedan
// contiguas (si no, se abrirían bandas repetidas de la misma partida).
func TestAgrupadoAbreUnaBandaPorPartida(t *testing.T) {
	t.Parallel()
	h := hojaDetalleMovimientos(metaCompleta(), movsDePrueba(), true)
	if !h.AgruparConSubtotales {
		t.Fatal("el agrupado debe pedir subtotales")
	}
	vistos := map[string]bool{}
	anterior := ""
	for _, fila := range h.Filas {
		if fila.Grupo == "" {
			t.Fatal("una fila del agrupado no dice a qué partida pertenece")
		}
		if fila.Grupo != anterior {
			if vistos[fila.Grupo] {
				t.Errorf("la partida %q vuelve a abrirse: las filas no quedaron contiguas", fila.Grupo)
			}
			vistos[fila.Grupo] = true
			anterior = fila.Grupo
		}
	}
	// Tres partidas en los datos de prueba: Ingresos › Datafonos, Gastos › Combustible y
	// «Sin clasificar».
	if len(vistos) != 3 {
		t.Errorf("se abrieron %d partidas, se esperaban 3: %v", len(vistos), vistos)
	}

	f := hojaEscrita(t, h)
	defer func() { _ = f.Close() }()
	filas, _ := f.GetRows("Movimientos")
	subtotales := 0
	for _, fila := range filas {
		if len(fila) > 0 && strings.HasPrefix(fila[0], "Subtotal ") {
			subtotales++
		}
	}
	if subtotales != 3 {
		t.Errorf("%d filas de subtotal, se esperaban 3 (una por partida)", subtotales)
	}
}

func contieneStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// valorFila busca la fila cuyo documento (columna 3) es el dado y devuelve la celda `col`.
func valorFila(filas [][]string, documento string, col int) string {
	for _, fila := range filas {
		if len(fila) > 3 && fila[3] == documento && col < len(fila) {
			return fila[col]
		}
	}
	return ""
}

// Los montos se escriben como NÚMERO con formato contable, no como texto: es lo que permitió que
// el cuadre mostrara «2,5E+07» cuando la columna era angosta y no tenía formato.
func TestMontosSonNumeroConFormatoYColumnaAncha(t *testing.T) {
	t.Parallel()
	f := libroDePrueba(t, metaCompleta())
	defer func() { _ = f.Close() }()

	const hoja = "Movimientos"
	for _, col := range []string{"C", "D"} {
		ancho, err := f.GetColWidth(hoja, col)
		if err != nil {
			t.Fatalf("ancho de %s: %v", col, err)
		}
		if ancho < 16 {
			t.Errorf("columna %s con ancho %.1f: menos de 16 hace que Excel muestre notación científica", col, ancho)
		}
	}
	// El crédito 125 000,50 tiene que estar guardado como NÚMERO crudo (no como el texto ya
	// formateado): si va como texto, Excel no lo suma ni lo ordena.
	filas, err := f.GetRows(hoja)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	crudo := ""
	for i := range filas {
		celda := fmt.Sprintf("D%d", i+1)
		v, err := f.GetCellValue(hoja, celda, excelize.Options{RawCellValue: true})
		if err == nil && v == "125000.5" {
			crudo = v
			// Y con formato contable puesto (número de formato distinto del general).
			estilo, err := f.GetCellStyle(hoja, celda)
			if err != nil {
				t.Fatalf("estilo de %s: %v", celda, err)
			}
			if estilo == 0 {
				t.Errorf("%s no tiene estilo: sin formato contable Excel muestra notación científica", celda)
			}
			break
		}
	}
	if crudo == "" {
		t.Error("el crédito 125000.5 no quedó como celda numérica en la columna D")
	}
}
