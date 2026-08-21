package bancos

// Constructor de libros de Excel con presentación de reporte financiero.
//
// El criterio de diseño, que es el del pedido y el de cualquier estado de cuenta bancario:
//
//   · SIN cuadrícula. La retícula por defecto de Excel hace que todo se lea como una hoja de
//     cálculo en borrador. Las únicas líneas del reporte son estructurales: la que cierra el
//     encabezado de la tabla, la que abre los totales y la del cierre. Nada de bordes por celda.
//   · Encabezado de DOCUMENTO antes de la tabla: quién lo emite (razón social), qué es, de qué
//     período, con qué filtros y cuándo/quién lo generó. Sin eso una hoja de movimientos no es
//     un reporte: es un pegote de datos que nadie puede citar en una reunión.
//   · Fechas como FECHA de verdad (serial de Excel) con formato dd/mm/aaaa. Si van como texto,
//     no ordenan ni filtran por rango, que es lo primero que uno hace.
//   · Montos como número con formato contable (miles, dos decimales, cero como guion) y el
//     símbolo por columna, no pegado a cada celda. Los débitos en rojo.
//   · Preparado para imprimir: horizontal, ajustado al ancho, con la fila de encabezado repetida
//     en cada página. Estos reportes se imprimen y se firman.
//
// El dato autoritativo sigue siendo el decimal de la base; acá todo es presentación.

import (
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// Paleta del reporte. Verde VDP para los títulos y grises neutros para las reglas.
const (
	colorTinta      = "1F2937" // gris muy oscuro para texto principal
	colorSuave      = "6B7280" // gris medio para etiquetas y notas
	colorRegla      = "D1D5DB" // gris claro para las líneas estructurales
	colorMarca      = "0F5132" // verde VDP
	colorMarcaSuave = "E8F0EC" // verde muy claro para bandas de agrupación
	colorNegativo   = "B42318" // rojo para débitos
)

// Formatos numéricos. El «-» del cero es el estilo contable: un cero suelto ensucia la columna.
const (
	fmtMonto = `#,##0.00;[Red]-#,##0.00;"—"`
	fmtFecha = "dd/mm/yyyy"
)

// ColumnaReporte describe una columna de la tabla de detalle.
type ColumnaReporte struct {
	Titulo string
	Ancho  float64
	// Tipo: "texto" | "fecha" | "monto" | "montoDebito" | "entero"
	Tipo string
}

// FilaReporte es una fila de datos. Los valores se emparejan por posición con las columnas.
type FilaReporte struct {
	Valores []any
	// Grupo: si cambia respecto de la fila anterior, se abre una banda de agrupación con este
	// texto (la «partida»: Concepto › Clasificación). Vacío = tabla sin agrupar.
	Grupo string
}

// MetaReporte es el encabezado del documento, con el orden de un estado de cuenta.
type MetaReporte struct {
	// Empresa es la razón social; EmpresaDetalle su identificación secundaria (tipo legal, y
	// cuando exista, la cédula jurídica).
	Empresa        string
	EmpresaDetalle string
	// Cuenta identifica la cuenta bancaria cuando el reporte es de UNA sola («BN · Valle de Paz
	// Colones · CR12… · CRC»). Vacío cuando abarca varias.
	Cuenta string
	Titulo string
	// Periodo legible («del 01/07/2026 al 31/08/2026», «Agosto 2026»).
	Periodo string
	// Filtros aplicados, en pares etiqueta→valor. Van en una línea compacta bajo el título para
	// que el reporte sea reproducible.
	Filtros [][2]string
	// GeneradoPor y GeneradoEn cierran la trazabilidad.
	GeneradoPor string
	GeneradoEn  time.Time
	// Avisos van en un RECUADRO ARRIBA, antes de la tabla: lo que hay que saber para leer bien
	// los números (que los totales están en colones, que hay montos sin tipo de cambio). Al pie
	// nadie los lee.
	Avisos []string
}

// estilosLibro agrupa los ids de estilo para no recrearlos por celda.
type estilosLibro struct {
	tituloEmpresa, tituloReporte, subtitulo       int
	identidad, metaDerecha, metaDerechaFuerte     int
	tituloCentrado, recuadroAviso, filtrosLinea   int
	etiqueta, valor, nota                         int
	thTexto, thNumero                             int
	tdTexto, tdFecha, tdMonto, tdDebito, tdEntero int
	grupo, grupoMonto                             int
	totalTexto, totalMonto                        int
	granTotalTexto, granTotalMonto                int
}

func nuevosEstilos(f *excelize.File) (estilosLibro, error) {
	var e estilosLibro
	mk := func(s *excelize.Style) (int, error) { return f.NewStyle(s) }
	var err error

	if e.tituloEmpresa, err = mk(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 15, Color: colorMarca, Family: "Calibri"},
	}); err != nil {
		return e, err
	}
	if e.tituloReporte, err = mk(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11, Color: colorTinta},
	}); err != nil {
		return e, err
	}
	if e.subtitulo, err = mk(&excelize.Style{
		Font: &excelize.Font{Size: 9, Color: colorSuave, Italic: true},
	}); err != nil {
		return e, err
	}
	if e.etiqueta, err = mk(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: colorSuave},
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	}); err != nil {
		return e, err
	}
	if e.valor, err = mk(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Bold: true, Color: colorTinta},
		Alignment: &excelize.Alignment{Vertical: "center"},
	}); err != nil {
		return e, err
	}
	if e.nota, err = mk(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: colorSuave},
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	}); err != nil {
		return e, err
	}

	// ── Encabezado de documento ──────────────────────────────────────────────
	// Identificación secundaria bajo la razón social (tipo legal, cuenta bancaria).
	if e.identidad, err = mk(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: colorSuave},
		Alignment: &excelize.Alignment{Vertical: "center"},
	}); err != nil {
		return e, err
	}
	// Los datos del documento van a la derecha, alineados a la derecha.
	if e.metaDerecha, err = mk(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: colorSuave},
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	}); err != nil {
		return e, err
	}
	if e.metaDerechaFuerte, err = mk(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Bold: true, Color: colorTinta},
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	}); err != nil {
		return e, err
	}
	// El título va CENTRADO sobre la tabla, como en un estado de cuenta.
	if e.tituloCentrado, err = mk(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 12, Color: colorTinta},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	}); err != nil {
		return e, err
	}
	// Recuadro de aviso: la ÚNICA caja con borde completo del reporte, y por eso se ve.
	if e.recuadroAviso, err = mk(&excelize.Style{
		Font: &excelize.Font{Size: 9, Color: colorMarca},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{colorMarcaSuave}},
		Border: []excelize.Border{
			{Type: "top", Color: colorMarca, Style: 1},
			{Type: "bottom", Color: colorMarca, Style: 1},
			{Type: "left", Color: colorMarca, Style: 1},
			{Type: "right", Color: colorMarca, Style: 1},
		},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	}); err != nil {
		return e, err
	}
	if e.filtrosLinea, err = mk(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: colorSuave, Italic: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	}); err != nil {
		return e, err
	}

	// Encabezado de tabla: fondo de marca, letra blanca, y la ÚNICA línea gruesa del reporte.
	thBase := func(h string) *excelize.Style {
		return &excelize.Style{
			Font:      &excelize.Font{Bold: true, Size: 9, Color: "FFFFFF"},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{colorMarca}},
			Alignment: &excelize.Alignment{Horizontal: h, Vertical: "center", WrapText: true},
		}
	}
	if e.thTexto, err = mk(thBase("left")); err != nil {
		return e, err
	}
	if e.thNumero, err = mk(thBase("right")); err != nil {
		return e, err
	}

	if e.tdTexto, err = mk(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: colorTinta},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	}); err != nil {
		return e, err
	}
	if e.tdFecha, err = mk(&excelize.Style{
		Font:         &excelize.Font{Size: 9, Color: colorTinta},
		CustomNumFmt: strPtr(fmtFecha),
		Alignment:    &excelize.Alignment{Horizontal: "center", Vertical: "top"},
	}); err != nil {
		return e, err
	}
	if e.tdMonto, err = mk(&excelize.Style{
		Font:         &excelize.Font{Size: 9, Color: colorTinta},
		CustomNumFmt: strPtr(fmtMonto),
		Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "top"},
	}); err != nil {
		return e, err
	}
	if e.tdDebito, err = mk(&excelize.Style{
		Font:         &excelize.Font{Size: 9, Color: colorNegativo},
		CustomNumFmt: strPtr(fmtMonto),
		Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "top"},
	}); err != nil {
		return e, err
	}
	if e.tdEntero, err = mk(&excelize.Style{
		Font:         &excelize.Font{Size: 9, Color: colorTinta},
		CustomNumFmt: strPtr("#,##0"),
		Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "top"},
	}); err != nil {
		return e, err
	}

	// Banda de agrupación (la partida) y su subtotal.
	if e.grupo, err = mk(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 9, Color: colorMarca},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{colorMarcaSuave}},
		Alignment: &excelize.Alignment{Vertical: "center"},
	}); err != nil {
		return e, err
	}
	if e.grupoMonto, err = mk(&excelize.Style{
		Font:         &excelize.Font{Bold: true, Size: 9, Color: colorMarca},
		Fill:         excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{colorMarcaSuave}},
		CustomNumFmt: strPtr(fmtMonto),
		Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	}); err != nil {
		return e, err
	}

	// Subtotal de grupo: una regla fina arriba, sin relleno.
	reglaArriba := []excelize.Border{{Type: "top", Color: colorRegla, Style: 1}}
	if e.totalTexto, err = mk(&excelize.Style{
		Font:   &excelize.Font{Bold: true, Size: 9, Color: colorSuave},
		Border: reglaArriba,
	}); err != nil {
		return e, err
	}
	if e.totalMonto, err = mk(&excelize.Style{
		Font:         &excelize.Font{Bold: true, Size: 9, Color: colorTinta},
		CustomNumFmt: strPtr(fmtMonto),
		Alignment:    &excelize.Alignment{Horizontal: "right"},
		Border:       reglaArriba,
	}); err != nil {
		return e, err
	}

	// Gran total: doble regla arriba, como un cierre contable.
	cierre := []excelize.Border{
		{Type: "top", Color: colorTinta, Style: 6}, // doble
	}
	if e.granTotalTexto, err = mk(&excelize.Style{
		Font:   &excelize.Font{Bold: true, Size: 10, Color: colorTinta},
		Border: cierre,
	}); err != nil {
		return e, err
	}
	if e.granTotalMonto, err = mk(&excelize.Style{
		Font:         &excelize.Font{Bold: true, Size: 10, Color: colorTinta},
		CustomNumFmt: strPtr(fmtMonto),
		Alignment:    &excelize.Alignment{Horizontal: "right"},
		Border:       cierre,
	}); err != nil {
		return e, err
	}
	return e, nil
}

func strPtr(s string) *string { return &s }

// HojaReporte es una hoja del libro: su encabezado, sus columnas y sus filas.
type HojaReporte struct {
	Nombre string
	Meta   MetaReporte
	Cols   []ColumnaReporte
	Filas  []FilaReporte
	// Totales: etiquetas→valores del cierre. Si está vacío, se calculan los de las columnas de
	// monto. Se pasa explícito cuando el total no es una simple suma de la columna.
	SinTotales bool
	// AgruparConSubtotales: además de la banda, escribe el subtotal de cada grupo.
	AgruparConSubtotales bool
}

// ConstruirLibro arma el .xlsx con todas sus hojas.
func ConstruirLibro(hojas []HojaReporte) ([]byte, error) {
	if len(hojas) == 0 {
		return nil, fmt.Errorf("bancos: el libro no tiene hojas")
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	est, err := nuevosEstilos(f)
	if err != nil {
		return nil, fmt.Errorf("bancos: estilos del libro: %w", err)
	}

	for i, h := range hojas {
		nombre := nombreHojaValido(h.Nombre)
		if i == 0 {
			if err := f.SetSheetName("Sheet1", nombre); err != nil {
				return nil, fmt.Errorf("bancos: nombrar hoja: %w", err)
			}
		} else if _, err := f.NewSheet(nombre); err != nil {
			return nil, fmt.Errorf("bancos: crear hoja %q: %w", nombre, err)
		}
		if err := escribirHoja(f, nombre, h, est); err != nil {
			return nil, err
		}
	}
	f.SetActiveSheet(0)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("bancos: serializar xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// nombreHojaValido recorta a 31 caracteres y quita los que Excel prohíbe.
func nombreHojaValido(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']':
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > 31 {
		s = string(r[:31])
	}
	if s == "" {
		s = "Hoja"
	}
	return s
}

func escribirHoja(f *excelize.File, hoja string, h HojaReporte, est estilosLibro) error {
	// SIN cuadrícula: es lo que separa un reporte de una hoja de cálculo en borrador.
	if err := f.SetSheetView(hoja, 0, &excelize.ViewOptions{ShowGridLines: boolPtr(false)}); err != nil {
		return fmt.Errorf("bancos: quitar cuadrícula: %w", err)
	}

	ultimaColIdx := len(h.Cols)
	ultimaCol, _ := excelize.ColumnNumberToName(ultimaColIdx)

	fila := escribirEncabezado(f, hoja, h.Meta, ultimaColIdx, est)

	// Encabezado de la tabla.
	filaTH := fila
	for c, col := range h.Cols {
		nombreCol, _ := excelize.ColumnNumberToName(c + 1)
		celda := fmt.Sprintf("%s%d", nombreCol, filaTH)
		if err := f.SetCellValue(hoja, celda, col.Titulo); err != nil {
			return err
		}
		estilo := est.thTexto
		if esColumnaNumerica(col.Tipo) {
			estilo = est.thNumero
		}
		if err := f.SetCellStyle(hoja, celda, celda, estilo); err != nil {
			return err
		}
		// El ancho SIEMPRE se fija, y las columnas de monto nunca por debajo de 16: una columna
		// angosta hace que Excel muestre el número en notación científica (2,5E+07), que es la
		// forma más rápida de arruinar un reporte financiero.
		ancho := col.Ancho
		if ancho <= 0 {
			ancho = 14
		}
		if esColumnaNumerica(col.Tipo) && ancho < 16 {
			ancho = 16
		}
		_ = f.SetColWidth(hoja, nombreCol, nombreCol, ancho)
	}
	_ = f.SetRowHeight(hoja, filaTH, 26)
	fila++

	// Detalle, con bandas de agrupación y subtotales por partida.
	grupoActual := ""
	subtot := map[int]float64{}
	granTot := map[int]float64{}
	filaInicioDatos := fila

	cerrarSubtotal := func() error {
		if !h.AgruparConSubtotales || grupoActual == "" {
			return nil
		}
		return escribirFilaTotal(f, hoja, fila, h.Cols, subtot,
			"Subtotal "+grupoActual, est.totalTexto, est.totalMonto)
	}

	for _, r := range h.Filas {
		if h.AgruparConSubtotales && r.Grupo != grupoActual {
			if grupoActual != "" {
				if err := cerrarSubtotal(); err != nil {
					return err
				}
				fila++
				fila++ // una línea de aire entre partidas
			}
			grupoActual = r.Grupo
			subtot = map[int]float64{}
			if err := escribirBandaGrupo(f, hoja, fila, ultimaCol, r.Grupo, est); err != nil {
				return err
			}
			fila++
		}
		for c, v := range r.Valores {
			if c >= len(h.Cols) {
				break
			}
			nombreCol, _ := excelize.ColumnNumberToName(c + 1)
			celda := fmt.Sprintf("%s%d", nombreCol, fila)
			if err := escribirCelda(f, hoja, celda, v, h.Cols[c].Tipo, est); err != nil {
				return err
			}
			if n, ok := comoFloat(v); ok && esColumnaNumerica(h.Cols[c].Tipo) {
				subtot[c] += n
				granTot[c] += n
			}
		}
		fila++
	}
	if err := cerrarSubtotal(); err != nil {
		return err
	}
	if h.AgruparConSubtotales && grupoActual != "" {
		fila += 2
	}

	// Gran total.
	if !h.SinTotales && len(h.Filas) > 0 {
		etiqueta := fmt.Sprintf("TOTAL · %d movimiento(s)", len(h.Filas))
		if err := escribirFilaTotal(f, hoja, fila, h.Cols, granTot, etiqueta,
			est.granTotalTexto, est.granTotalMonto); err != nil {
			return err
		}
		fila += 2
	}

	// Pie: la firma del sistema. Los avisos van ARRIBA en el recuadro, no acá.
	mergeConEstilo(f, hoja, fmt.Sprintf("A%d", fila), fmt.Sprintf("%s%d", ultimaCol, fila),
		"Generado por GPVDP ERP · "+h.Meta.Empresa+" · "+h.Meta.GeneradoEn.Format("02/01/2006 15:04"), est.nota)
	fila++

	// Congelar bajo el encabezado de tabla y filtro automático sobre el detalle.
	_ = f.SetPanes(hoja, &excelize.Panes{
		Freeze: true, YSplit: filaTH, TopLeftCell: fmt.Sprintf("A%d", filaTH+1), ActivePane: "bottomLeft",
	})
	if !h.AgruparConSubtotales && len(h.Filas) > 0 {
		// Con bandas y subtotales intercalados el autofiltro estorba; sin agrupar, ayuda.
		_ = f.AutoFilter(hoja, fmt.Sprintf("A%d:%s%d", filaTH, ultimaCol, filaInicioDatos+len(h.Filas)-1),
			[]excelize.AutoFilterOptions{})
	}

	// Para imprimir: horizontal, ajustado al ancho, repitiendo el encabezado en cada página.
	_ = f.SetPageLayout(hoja, &excelize.PageLayoutOptions{
		Orientation: strPtr("landscape"), FitToWidth: intPtr(1), FitToHeight: intPtr(0),
	})
	_ = f.SetPageMargins(hoja, &excelize.PageLayoutMarginsOptions{
		Left: f64Ptr(0.4), Right: f64Ptr(0.4), Top: f64Ptr(0.5), Bottom: f64Ptr(0.5),
	})
	_ = f.SetDefinedName(&excelize.DefinedName{
		Name: "_xlnm.Print_Titles", RefersTo: fmt.Sprintf("'%s'!$%d:$%d", hoja, filaTH, filaTH), Scope: hoja,
	})
	return nil
}

// escribirEncabezado pinta el bloque de identificación y devuelve la primera fila libre.
//
// El orden es el de un estado de cuenta bancario, que es el que pidió el negocio: la identidad a
// la IZQUIERDA (de quién es esto), los datos del documento a la DERECHA (cuándo se generó y de qué
// período), un recuadro de aviso, y recién ahí el título centrado sobre la tabla. Un bloque de
// etiqueta→valor apilado —lo que había antes— tiene la misma información y se lee como un
// formulario, no como un documento.
func escribirEncabezado(f *excelize.File, hoja string, m MetaReporte, ultimaColIdx int, est estilosLibro) int {
	ultimaCol, _ := excelize.ColumnNumberToName(ultimaColIdx)
	// La derecha arranca aproximadamente en el 60 % del ancho de la tabla, para que las dos
	// columnas del encabezado respiren.
	colDerechaIdx := ultimaColIdx*3/5 + 1
	if colDerechaIdx < 2 {
		colDerechaIdx = 2
	}
	if colDerechaIdx > ultimaColIdx {
		colDerechaIdx = ultimaColIdx
	}
	colDerecha, _ := excelize.ColumnNumberToName(colDerechaIdx)
	colAntesDerecha, _ := excelize.ColumnNumberToName(max2(colDerechaIdx-1, 1))

	// Encabezado de DOS filas, y la tabla arranca en la 3.
	//
	// Antes ocupaba nueve filas (identidad en tres, aire, recuadro de aviso, título centrado,
	// línea de filtros, aire). El usuario borraba «de la línea 2 a la 9» cada vez que trabajaba o
	// compartía el archivo, que es la señal más clara de que ese espacio no servía. Ahora todo lo
	// que hace falta —quién, qué, de cuándo, con qué filtros y las advertencias— cabe en dos
	// filas, y nada obliga a borrar antes de usar el reporte.
	//
	// Fila 1: EMPRESA — Título · Período      (a la derecha) fecha y hora de emisión
	// Fila 2: contexto: avisos, filtros, cuenta y quién lo emitió, en letra chica.
	titulo := strings.ToUpper(m.Empresa)
	if m.Titulo != "" {
		titulo += " — " + m.Titulo
	}
	if m.Periodo != "" {
		titulo += " · " + m.Periodo
	}
	mergeConEstilo(f, hoja, "A1", fmt.Sprintf("%s1", colAntesDerecha), titulo, est.tituloEmpresa)
	mergeConEstilo(f, hoja, fmt.Sprintf("%s1", colDerecha), fmt.Sprintf("%s1", ultimaCol),
		m.GeneradoEn.Format("02/01/2006")+" "+m.GeneradoEn.Format("15:04"), est.metaDerecha)
	_ = f.SetRowHeight(hoja, 1, 22)

	// Segunda fila: TODO el contexto junto. Los avisos van primero porque son lo que el lector
	// necesita saber antes de mirar los números (que los totales están en colones, o que hay
	// montos en dólares sin tipo de cambio que no suman).
	contexto := make([]string, 0, len(m.Avisos)+len(m.Filtros)+3)
	contexto = append(contexto, m.Avisos...)
	if m.EmpresaDetalle != "" {
		contexto = append(contexto, m.EmpresaDetalle)
	}
	if m.Cuenta != "" {
		contexto = append(contexto, m.Cuenta)
	}
	for _, kv := range m.Filtros {
		contexto = append(contexto, kv[0]+": "+kv[1])
	}
	if m.GeneradoPor != "" {
		contexto = append(contexto, "Emitido por "+m.GeneradoPor)
	}
	fila := 2
	if len(contexto) > 0 {
		mergeConEstilo(f, hoja, "A2", fmt.Sprintf("%s2", ultimaCol),
			strings.Join(contexto, "   ·   "), est.filtrosLinea)
		fila = 3
	}
	return fila
}

// AhoraCR es el momento de emisión en hora de Costa Rica (UTC−6).
//
// El sello del reporte se hacía con time.Now(), que en el contenedor es UTC: un reporte emitido a
// las 14:23 decía «20:23» y el que lo recibe no tiene forma de saber que está corrido seis horas.
// Mismo criterio de día de operación que usan Bancos y CxC.
func AhoraCR() time.Time {
	return time.Now().UTC().Add(-6 * time.Hour)
}

// mergeConEstilo combina el rango y aplica el estilo a TODAS sus celdas.
//
// Lo segundo es lo que importa: excelize combina las celdas pero cada una conserva su propio
// estilo, así que poner el estilo solo en la primera dibujaba el borde y el relleno del recuadro
// alrededor de la columna A únicamente — se veían líneas sueltas colgando a la izquierda del
// encabezado. El formato de un rango combinado hay que aplicarlo al rango completo.
func mergeConEstilo(f *excelize.File, hoja, desde, hasta, texto string, estilo int) {
	if texto == "" {
		return
	}
	_ = f.SetCellValue(hoja, desde, texto)
	_ = f.MergeCell(hoja, desde, hasta)
	_ = f.SetCellStyle(hoja, desde, hasta, estilo)
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func escribirBandaGrupo(f *excelize.File, hoja string, fila int, ultimaCol, texto string, est estilosLibro) error {
	if texto == "" {
		texto = "Sin clasificar"
	}
	// La banda tiene relleno y borde inferior: el estilo va al rango combinado completo, o la
	// banda se pinta solo sobre la columna A.
	mergeConEstilo(f, hoja, fmt.Sprintf("A%d", fila), fmt.Sprintf("%s%d", ultimaCol, fila), texto, est.grupo)
	_ = f.SetRowHeight(hoja, fila, 18)
	return nil
}

func escribirFilaTotal(f *excelize.File, hoja string, fila int, cols []ColumnaReporte,
	totales map[int]float64, etiqueta string, estTexto, estMonto int) error {
	for c, col := range cols {
		nombreCol, _ := excelize.ColumnNumberToName(c + 1)
		celda := fmt.Sprintf("%s%d", nombreCol, fila)
		if esColumnaNumerica(col.Tipo) {
			if err := f.SetCellValue(hoja, celda, totales[c]); err != nil {
				return err
			}
			_ = f.SetCellStyle(hoja, celda, celda, estMonto)
			continue
		}
		// La etiqueta va en la primera columna; las demás de texto quedan con la regla puesta.
		if c == 0 {
			if err := f.SetCellValue(hoja, celda, etiqueta); err != nil {
				return err
			}
		}
		_ = f.SetCellStyle(hoja, celda, celda, estTexto)
	}
	return nil
}

func escribirCelda(f *excelize.File, hoja, celda string, v any, tipo string, est estilosLibro) error {
	switch tipo {
	case "fecha":
		if t, ok := comoFecha(v); ok {
			if err := f.SetCellValue(hoja, celda, t); err != nil {
				return err
			}
			return f.SetCellStyle(hoja, celda, celda, est.tdFecha)
		}
		if err := f.SetCellValue(hoja, celda, v); err != nil {
			return err
		}
		return f.SetCellStyle(hoja, celda, celda, est.tdTexto)
	case "monto", "montoDebito":
		n, _ := comoFloat(v)
		if err := f.SetCellValue(hoja, celda, n); err != nil {
			return err
		}
		estilo := est.tdMonto
		if tipo == "montoDebito" {
			estilo = est.tdDebito
		}
		return f.SetCellStyle(hoja, celda, celda, estilo)
	case "entero":
		n, _ := comoFloat(v)
		if err := f.SetCellValue(hoja, celda, n); err != nil {
			return err
		}
		return f.SetCellStyle(hoja, celda, celda, est.tdEntero)
	default:
		if err := f.SetCellValue(hoja, celda, v); err != nil {
			return err
		}
		return f.SetCellStyle(hoja, celda, celda, est.tdTexto)
	}
}

func esColumnaNumerica(tipo string) bool {
	return tipo == "monto" || tipo == "montoDebito" || tipo == "entero"
}

func comoFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		return montoNum(n), n != ""
	}
	return 0, false
}

// comoFecha acepta time.Time o el "YYYY-MM-DD" que devuelven los repositorios.
func comoFecha(v any) (time.Time, bool) {
	switch d := v.(type) {
	case time.Time:
		return d, true
	case string:
		if t, err := time.Parse("2006-01-02", d); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func boolPtr(b bool) *bool      { return &b }
func intPtr(i int) *int         { return &i }
func f64Ptr(v float64) *float64 { return &v }
