package cxp

// El aviso de pago como .xlsx: el papel que hoy se arma a mano y se manda por correo.
//
// ── POR QUÉ NO PASA POR `ConstruirLibro` ────────────────────────────────────────────────────
//
// El motor de reportes de Bancos está hecho para TABLAS DE MOVIMIENTOS: sin cuadrícula, débitos
// en rojo, agrupación por partida, filas angostas y muchas. Esto es otra cosa: un documento de
// pocas filas, con una glosa larga que tiene que leerse entera, que alguien imprime y manda. El
// criterio visual es el mismo —encabezado de documento, montos y fechas de verdad, preparado para
// imprimir— pero el layout no, y forzarlo daría un resultado peor que el cuadro que ya se usa.
//
// La deuda queda anotada: si aparece un tercer documento de este tipo, el motor se saca a un
// paquete propio en vez de que CxP importe a Bancos, que hoy sería la primera dependencia entre
// dos dominios del sistema.

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// Colores del documento. El azul es el del cuadro que ya se manda: se conserva para que el papel
// nuevo se reconozca como el mismo papel.
const (
	avisoAzul      = "#1F3864"
	avisoGris      = "#F2F2F2"
	avisoBordeAzul = "#8EA9DB"
	avisoAmbar     = "#FFF2CC"
)

type estilosAviso struct {
	titulo, contexto, cabecera   int
	texto, textoAlt, textoAlerta int
	monto, montoAlt, montoAlerta int
	rotuloTotal, valorTotal, pie int
}

func bordesAviso() []excelize.Border {
	return []excelize.Border{
		{Type: "left", Color: avisoBordeAzul, Style: 1},
		{Type: "right", Color: avisoBordeAzul, Style: 1},
		{Type: "top", Color: avisoBordeAzul, Style: 1},
		{Type: "bottom", Color: avisoBordeAzul, Style: 1},
	}
}

func nuevosEstilosAviso(f *excelize.File) (estilosAviso, error) {
	var e estilosAviso
	rellenar := func(c string) *excelize.Fill {
		return &excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{c}}
	}
	// Formato con dos decimales y separador de miles. El símbolo NO va pegado a la celda: la
	// moneda tiene su propia columna, y repetirlo en cada monto ensancha la columna sin aportar.
	fmtMonto := "#,##0.00"
	defs := []struct {
		dst   *int
		style *excelize.Style
	}{
		{&e.titulo, &excelize.Style{
			Font:      &excelize.Font{Bold: true, Size: 13},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
			Border:    bordesAviso(),
		}},
		{&e.contexto, &excelize.Style{
			Font:      &excelize.Font{Size: 9, Italic: true, Color: "#595959"},
			Alignment: &excelize.Alignment{Vertical: "center"},
		}},
		{&e.cabecera, &excelize.Style{
			Font:      &excelize.Font{Bold: true, Size: 10, Color: "#FFFFFF"},
			Fill:      *rellenar(avisoAzul),
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
			Border:    bordesAviso(),
		}},
		{&e.texto, &excelize.Style{
			Font:      &excelize.Font{Size: 10},
			Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
			Border:    bordesAviso(),
		}},
		{&e.textoAlt, &excelize.Style{
			Font: &excelize.Font{Size: 10}, Fill: *rellenar(avisoGris),
			Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
			Border:    bordesAviso(),
		}},
		{&e.textoAlerta, &excelize.Style{
			Font: &excelize.Font{Size: 10}, Fill: *rellenar(avisoAmbar),
			Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
			Border:    bordesAviso(),
		}},
		{&e.monto, &excelize.Style{
			Font: &excelize.Font{Size: 10}, CustomNumFmt: &fmtMonto,
			Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
			Border:    bordesAviso(),
		}},
		{&e.montoAlt, &excelize.Style{
			Font: &excelize.Font{Size: 10}, Fill: *rellenar(avisoGris), CustomNumFmt: &fmtMonto,
			Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
			Border:    bordesAviso(),
		}},
		{&e.montoAlerta, &excelize.Style{
			Font: &excelize.Font{Size: 10}, Fill: *rellenar(avisoAmbar), CustomNumFmt: &fmtMonto,
			Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
			Border:    bordesAviso(),
		}},
		{&e.rotuloTotal, &excelize.Style{
			Font: &excelize.Font{Bold: true, Size: 10, Color: "#FFFFFF"}, Fill: *rellenar(avisoAzul),
			Alignment: &excelize.Alignment{Vertical: "center"}, Border: bordesAviso(),
		}},
		{&e.valorTotal, &excelize.Style{
			Font: &excelize.Font{Bold: true, Size: 11, Color: "#FFFFFF"}, Fill: *rellenar(avisoAzul),
			CustomNumFmt: &fmtMonto,
			Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "center"},
			Border:       bordesAviso(),
		}},
		{&e.pie, &excelize.Style{
			Font:      &excelize.Font{Size: 9, Color: "#595959"},
			Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		}},
	}
	for _, d := range defs {
		id, err := f.NewStyle(d.style)
		if err != nil {
			return estilosAviso{}, fmt.Errorf("cxp: estilo del aviso: %w", err)
		}
		*d.dst = id
	}
	return e, nil
}

// mergeConEstiloAviso combina Y estila el rango COMPLETO.
//
// Estilar solo la primera celda de un rango combinado deja el borde dibujado alrededor de la
// columna A y una línea suelta colgando: excelize conserva el estilo de cada celda aunque estén
// combinadas. Es la misma trampa que ya se documentó en el reporte de Bancos.
func mergeConEstiloAviso(f *excelize.File, hoja, a, b string, estilo int) error {
	if err := f.MergeCell(hoja, a, b); err != nil {
		return fmt.Errorf("cxp: combinar %s:%s: %w", a, b, err)
	}
	if err := f.SetCellStyle(hoja, a, b, estilo); err != nil {
		return fmt.Errorf("cxp: estilar %s:%s: %w", a, b, err)
	}
	return nil
}

// TituloDeMoneda nombra el cuadro como lo nombra quien lo manda hoy: «Pago Proveedores Colones».
func TituloDeMoneda(moneda string) string {
	switch strings.ToUpper(strings.TrimSpace(moneda)) {
	case "CRC", "":
		return "Pago Proveedores Colones"
	case "USD":
		return "Pago Proveedores Dólares"
	default:
		return "Pago Proveedores " + strings.ToUpper(moneda)
	}
}

// NombreAvisoXLSX nombra el archivo con el corte y la moneda.
//
// Los dos archivos de un mismo corte terminan en la misma carpeta de descargas: sin la moneda en
// el nombre, el segundo baja como «(1)» y cuál es cuál se vuelve una adivinanza. Es la misma
// razón por la que la macro ya lo hace.
func NombreAvisoXLSX(loteNumero int64, moneda string) string {
	return fmt.Sprintf("aviso-pago-corte-%04d-%s.xlsx", loteNumero, strings.ToUpper(moneda))
}

// ConstruirAvisoXLSX arma el libro de UNA moneda.
//
// `empresa` y `generadoPor` van en la línea de contexto: un papel que se manda por fuera del
// sistema tiene que decir de quién sale y quién lo emitió, o no se puede citar después.
func ConstruirAvisoXLSX(av Aviso, m AvisoDeMoneda, empresa, generadoPor, generadoEn string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	hoja := "Aviso"
	if err := f.SetSheetName("Sheet1", hoja); err != nil {
		return nil, fmt.Errorf("cxp: nombrar hoja: %w", err)
	}
	e, err := nuevosEstilosAviso(f)
	if err != nil {
		return nil, err
	}

	anchos := []struct {
		col string
		w   float64
	}{{"A", 30}, {"B", 10}, {"C", 22}, {"D", 48}, {"E", 16}, {"F", 26}, {"G", 16}}
	for _, a := range anchos {
		if err := f.SetColWidth(hoja, a.col, a.col, a.w); err != nil {
			return nil, fmt.Errorf("cxp: ancho de columna: %w", err)
		}
	}

	// EL BLOQUE DE FIRMAS VA ARRIBA, antes del cuadro: es el encabezado del respaldo, lo primero
	// que mira quien lo recibe para saber por cuántas manos pasó este pago.
	//
	// Las cuatro líneas salen SIEMPRE, aunque alguna esté vacía. Omitir la de «Aprobado por»
	// cuando nadie aprobó haría que el papel se leyera como si ese paso no existiera; dejarla en
	// blanco dice que existe y no se hizo, que es la información que importa.
	filaFirma := 1
	for _, fi := range av.Firmas {
		etq := fmt.Sprintf("A%d", filaFirma)
		val := fmt.Sprintf("C%d", filaFirma)
		if err := f.SetCellValue(hoja, etq, fi.Rol+":"); err != nil {
			return nil, err
		}
		if err := f.SetCellStyle(hoja, etq, etq, e.rotuloTotal); err != nil {
			return nil, err
		}
		if err := mergeConEstiloAviso(f, hoja, val, fmt.Sprintf("G%d", filaFirma), e.texto); err != nil {
			return nil, err
		}
		texto := fi.Nombre
		if texto != "" && fi.Fecha != "" {
			texto += "   (" + fi.Fecha + ")"
		}
		if texto != "" {
			if err := f.SetCellValue(hoja, val, texto); err != nil {
				return nil, err
			}
		}
		filaFirma++
	}
	// La fecha de pago cierra el bloque. Vacía mientras el corte no se haya cerrado: poner la de
	// hoy sería afirmar que ya salió el dinero.
	if err := f.SetCellValue(hoja, fmt.Sprintf("A%d", filaFirma), "Fecha de pago:"); err != nil {
		return nil, err
	}
	if err := f.SetCellStyle(hoja, fmt.Sprintf("A%d", filaFirma), fmt.Sprintf("A%d", filaFirma), e.rotuloTotal); err != nil {
		return nil, err
	}
	if err := mergeConEstiloAviso(f, hoja, fmt.Sprintf("C%d", filaFirma),
		fmt.Sprintf("G%d", filaFirma), e.texto); err != nil {
		return nil, err
	}
	if av.FechaPago != "" {
		if err := f.SetCellValue(hoja, fmt.Sprintf("C%d", filaFirma), av.FechaPago); err != nil {
			return nil, err
		}
	}

	// El cuadro empieza después del bloque de firmas, con una fila de aire.
	filaTitulo := filaFirma + 2
	filaContexto := filaTitulo + 1
	filaCabecera := filaContexto + 1

	celdaTitulo := fmt.Sprintf("A%d", filaTitulo)
	if err := mergeConEstiloAviso(f, hoja, celdaTitulo, fmt.Sprintf("G%d", filaTitulo), e.titulo); err != nil {
		return nil, err
	}
	if err := f.SetCellValue(hoja, celdaTitulo, TituloDeMoneda(m.Moneda)); err != nil {
		return nil, err
	}
	_ = f.SetRowHeight(hoja, filaTitulo, 24)

	// La línea de contexto. Lleva las transferencias porque NO siempre son tantas como filas: en
	// el corte por factura una fila puede cubrir varias, y ese es el número que se cuadra contra
	// el estado de cuenta.
	contexto := fmt.Sprintf("%s  ·  Corte N° %04d  ·  Fecha de corte %s  ·  %d proveedor(es)  ·  %d transferencia(s)",
		empresa, av.LoteNumero, av.FechaCorte, len(m.Filas), m.Transferencias)
	if generadoPor != "" {
		contexto += "  ·  Generado por " + generadoPor
	}
	if generadoEn != "" {
		contexto += " el " + generadoEn
	}
	celdaContexto := fmt.Sprintf("A%d", filaContexto)
	if err := mergeConEstiloAviso(f, hoja, celdaContexto, fmt.Sprintf("G%d", filaContexto), e.contexto); err != nil {
		return nil, err
	}
	if err := f.SetCellValue(hoja, celdaContexto, contexto); err != nil {
		return nil, err
	}

	cols := []string{"Proveedor", "Moneda", "Concepto", "Documentos", "Monto", "Cuenta Bancaria", "Identificación"}
	for i, c := range cols {
		celda, err := excelize.CoordinatesToCellName(i+1, filaCabecera)
		if err != nil {
			return nil, fmt.Errorf("cxp: celda de cabecera: %w", err)
		}
		if err := f.SetCellValue(hoja, celda, c); err != nil {
			return nil, err
		}
		if err := f.SetCellStyle(hoja, celda, celda, e.cabecera); err != nil {
			return nil, err
		}
	}
	_ = f.SetRowHeight(hoja, filaCabecera, 22)

	fila := filaCabecera + 1
	for i, fa := range m.Filas {
		txt, mon := e.texto, e.monto
		if i%2 == 1 {
			txt, mon = e.textoAlt, e.montoAlt
		}
		// El proveedor sin cuenta se pinta: el banco rechaza esa línea y el aviso no puede
		// mandarse como si estuviera todo bien.
		if fa.SinCuenta {
			txt, mon = e.textoAlerta, e.montoAlerta
		}
		valores := []any{
			fa.Proveedor, nombreDeMoneda(fa.Moneda), fa.Concepto, fa.Documentos,
			nil, cuentaOAviso(fa), fa.Identificacion,
		}
		for j, v := range valores {
			celda, err := excelize.CoordinatesToCellName(j+1, fila)
			if err != nil {
				return nil, fmt.Errorf("cxp: celda de fila: %w", err)
			}
			estilo := txt
			if j == 4 {
				// El monto va como NÚMERO, no como texto: en texto no suma ni ordena, que es lo
				// primero que hace quien recibe el cuadro.
				estilo = mon
				if d, err := decimal.NewFromString(fa.Monto); err == nil {
					v, _ = d.Float64()
				}
			}
			if v != nil {
				if err := f.SetCellValue(hoja, celda, v); err != nil {
					return nil, err
				}
			}
			if err := f.SetCellStyle(hoja, celda, celda, estilo); err != nil {
				return nil, err
			}
		}
		_ = f.SetRowHeight(hoja, fila, 30)
		fila++
	}

	// EL TOTAL. Es el número que se compara contra el archivo que se sube al banco, y el cuadro
	// que se manda hoy no lo trae: hay que sumarlo a mano justo cuando más importa no equivocarse.
	if err := mergeConEstiloAviso(f, hoja, fmt.Sprintf("A%d", fila), fmt.Sprintf("D%d", fila), e.rotuloTotal); err != nil {
		return nil, err
	}
	if err := f.SetCellValue(hoja, fmt.Sprintf("A%d", fila),
		"TOTAL A TRANSFERIR EN "+strings.ToUpper(nombreDeMoneda(m.Moneda))); err != nil {
		return nil, err
	}
	if d, err := decimal.NewFromString(m.Total); err == nil {
		v, _ := d.Float64()
		if err := f.SetCellValue(hoja, fmt.Sprintf("E%d", fila), v); err != nil {
			return nil, err
		}
	}
	if err := f.SetCellStyle(hoja, fmt.Sprintf("E%d", fila), fmt.Sprintf("E%d", fila), e.valorTotal); err != nil {
		return nil, err
	}
	if err := mergeConEstiloAviso(f, hoja, fmt.Sprintf("F%d", fila), fmt.Sprintf("G%d", fila), e.rotuloTotal); err != nil {
		return nil, err
	}
	_ = f.SetRowHeight(hoja, fila, 20)

	fila += 2
	pie := fmt.Sprintf("Este detalle corresponde al archivo de pago en %s del corte N° %04d.",
		nombreDeMoneda(m.Moneda), av.LoteNumero)
	if len(av.Monedas) > 1 {
		pie += " Las facturas en otra moneda viajan en su propio cuadro y en su propio archivo."
	}
	if m.SinCuenta > 0 {
		pie += fmt.Sprintf(" ATENCIÓN: %d proveedor(es) sin cuenta registrada; el banco rechaza esas líneas.", m.SinCuenta)
	}
	if err := mergeConEstiloAviso(f, hoja, fmt.Sprintf("A%d", fila), fmt.Sprintf("G%d", fila), e.pie); err != nil {
		return nil, err
	}
	if err := f.SetCellValue(hoja, fmt.Sprintf("A%d", fila), pie); err != nil {
		return nil, err
	}

	// Sin cuadrícula y con la cabecera congelada: el papel se lee como documento, no como hoja de
	// cálculo en borrador. Mismo criterio que el reporte de Bancos.
	if err := f.SetSheetView(hoja, 0, &excelize.ViewOptions{ShowGridLines: boolFalso()}); err != nil {
		return nil, fmt.Errorf("cxp: vista de la hoja: %w", err)
	}
	// La cabecera queda fija al desplazarse. El corte sigue a `filaCabecera` y no a un 3 escrito a
	// mano: el bloque de firmas de arriba movió la tabla, y un número fijo acá dejaría congeladas
	// las firmas en vez de los títulos de las columnas, que es lo que hay que seguir viendo al
	// recorrer cien proveedores.
	_ = f.SetPanes(hoja, &excelize.Panes{
		Freeze: true, YSplit: filaCabecera, TopLeftCell: fmt.Sprintf("A%d", filaCabecera+1),
		ActivePane: "bottomLeft",
	})
	// Se imprime y se firma: horizontal y ajustado al ancho, o las siete columnas salen partidas
	// en dos páginas y el cuadro deja de leerse.
	_ = f.SetPageLayout(hoja, &excelize.PageLayoutOptions{Orientation: orientacionHorizontal()})
	_ = f.SetSheetProps(hoja, &excelize.SheetPropsOptions{FitToPage: boolVerdadero()})

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("cxp: serializar el aviso: %w", err)
	}
	return buf.Bytes(), nil
}

func boolFalso() *bool     { b := false; return &b }
func boolVerdadero() *bool { b := true; return &b }

func orientacionHorizontal() *string { s := "landscape"; return &s }

// nombreDeMoneda escribe la moneda como la escribe el cuadro que se manda hoy: «Colones», no
// «CRC». El papel lo lee un proveedor.
func nombreDeMoneda(m string) string {
	switch strings.ToUpper(strings.TrimSpace(m)) {
	case "CRC", "":
		return "Colones"
	case "USD":
		return "Dólares"
	default:
		return strings.ToUpper(m)
	}
}

// cuentaOAviso dice por qué está vacía la celda en vez de dejarla en blanco: una celda vacía se
// lee como un descuido de quien armó el cuadro, no como el dato que falta.
func cuentaOAviso(f FilaAviso) string {
	if f.SinCuenta {
		return "SIN CUENTA REGISTRADA"
	}
	return f.Cuenta
}
