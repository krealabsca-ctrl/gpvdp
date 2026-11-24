// Maqueta de los dos papeles del pago a proveedores, para aprobar el DISEÑO antes de construirlo.
//
// Programa temporal: no forma parte del ERP. Escribe un .xlsx con dos hojas usando los datos
// exactos de los cuadros que hoy se arman a mano en Excel.
package main

import (
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

const (
	azul     = "#1F3864"
	azulSuav = "#D9E1F2"
	verde    = "#C6E0B4"
	amarillo = "#FFE699"
	gris     = "#F2F2F2"
	borde    = "#8EA9DB"
)

type est struct {
	titulo, cabecera, texto, textoAlt, monto, montoAlt int
	montoVerde, montoAmar, textoVerde, textoAmar       int
	rotulo, valor, valorFuerte, rotuloFuerte           int
	leyendaV, leyendaA, firma, firmaLinea, nota        int
}

func bordes() []excelize.Border {
	return []excelize.Border{
		{Type: "left", Color: borde, Style: 1}, {Type: "right", Color: borde, Style: 1},
		{Type: "top", Color: borde, Style: 1}, {Type: "bottom", Color: borde, Style: 1},
	}
}

func nuevoEstilo(f *excelize.File, s *excelize.Style) int {
	id, err := f.NewStyle(s)
	if err != nil {
		panic(err)
	}
	return id
}

func estilos(f *excelize.File) est {
	fill := func(c string) excelize.Fill {
		return excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{c}}
	}
	const fmtMonto = "#,##0.00"
	var e est
	e.titulo = nuevoEstilo(f, &excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 13, Color: "#000000"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    bordes(),
	})
	e.cabecera = nuevoEstilo(f, &excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "#FFFFFF"},
		Fill:      fill(azul),
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    bordes(),
	})
	e.texto = nuevoEstilo(f, &excelize.Style{
		Font:      &excelize.Font{Size: 10},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border:    bordes(),
	})
	e.textoAlt = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 10}, Fill: fill(gris),
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border:    bordes(),
	})
	e.textoVerde = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 10}, Fill: fill(verde),
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}, Border: bordes(),
	})
	e.textoAmar = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 10}, Fill: fill(amarillo),
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}, Border: bordes(),
	})
	e.monto = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 10}, CustomNumFmt: strPtr(fmtMonto),
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}, Border: bordes(),
	})
	e.montoAlt = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 10}, Fill: fill(gris), CustomNumFmt: strPtr(fmtMonto),
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}, Border: bordes(),
	})
	e.montoVerde = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 10}, Fill: fill(verde), CustomNumFmt: strPtr(fmtMonto),
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}, Border: bordes(),
	})
	e.montoAmar = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 10}, Fill: fill(amarillo), CustomNumFmt: strPtr(fmtMonto),
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}, Border: bordes(),
	})
	e.rotulo = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 10}, Fill: fill(azulSuav),
		Alignment: &excelize.Alignment{Vertical: "center"}, Border: bordes(),
	})
	e.rotuloFuerte = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 10, Color: "#FFFFFF"}, Fill: fill(azul),
		Alignment: &excelize.Alignment{Vertical: "center"}, Border: bordes(),
	})
	e.valor = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 10}, CustomNumFmt: strPtr(fmtMonto),
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"}, Border: bordes(),
	})
	e.valorFuerte = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11, Color: "#FFFFFF"}, Fill: fill(azul),
		CustomNumFmt: strPtr(fmtMonto),
		Alignment:    &excelize.Alignment{Horizontal: "right", Vertical: "center"}, Border: bordes(),
	})
	e.leyendaV = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 9}, Fill: fill(verde),
		Alignment: &excelize.Alignment{Vertical: "center"}, Border: bordes(),
	})
	e.leyendaA = nuevoEstilo(f, &excelize.Style{
		Font: &excelize.Font{Size: 9}, Fill: fill(amarillo),
		Alignment: &excelize.Alignment{Vertical: "center"}, Border: bordes(),
	})
	e.firma = nuevoEstilo(f, &excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 9, Color: azul},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	e.firmaLinea = nuevoEstilo(f, &excelize.Style{
		Border: []excelize.Border{{Type: "bottom", Color: "#000000", Style: 1}},
	})
	e.nota = nuevoEstilo(f, &excelize.Style{
		Font:      &excelize.Font{Size: 9, Italic: true, Color: "#595959"},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	return e
}

func strPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }

// mergeConEstilo combina Y estila el rango completo. Estilar solo la primera celda deja el borde
// colgando a la izquierda: excelize conserva el estilo de cada celda aunque estén combinadas.
func mergeConEstilo(f *excelize.File, hoja, a, b string, estilo int) {
	if err := f.MergeCell(hoja, a, b); err != nil {
		panic(err)
	}
	if err := f.SetCellStyle(hoja, a, b, estilo); err != nil {
		panic(err)
	}
}

func set(f *excelize.File, hoja, celda string, v any) {
	if err := f.SetCellValue(hoja, celda, v); err != nil {
		panic(err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────────────────────
// HOJA 1 · «Pago Proveedores Colones» — el aviso que se manda al pagar
// ─────────────────────────────────────────────────────────────────────────────────────────────

type filaPago struct {
	proveedor, moneda, concepto, documentos, cuenta, identificacion string
	monto                                                           float64
}

func hojaAviso(f *excelize.File, e est) {
	const h = "Pago Colones"
	if err := f.SetSheetName("Sheet1", h); err != nil {
		panic(err)
	}
	anchos := map[string]float64{"A": 26, "B": 11, "C": 20, "D": 46, "E": 15, "F": 24, "G": 15}
	for col, w := range anchos {
		_ = f.SetColWidth(h, col, col, w)
	}
	_ = f.SetSheetProps(h, &excelize.SheetPropsOptions{})

	// Fila 1: el título, tal como está hoy.
	mergeConEstilo(f, h, "A1", "G1", e.titulo)
	set(f, h, "A1", "Pago Proveedores Colones")
	_ = f.SetRowHeight(h, 1, 24)

	// Fila 2 (PROPUESTA): el contexto que hoy falta y que hace el aviso reproducible.
	mergeConEstilo(f, h, "A2", "G2", e.nota)
	set(f, h, "A2", "Valle de Paz  ·  Corte N° 0043  ·  Fecha de corte 04/11/2026  ·  "+
		"4 transferencias  ·  12 facturas  ·  Generado por Administrador GPVDP el 04/11/2026 15:40")
	_ = f.SetRowHeight(h, 2, 16)

	cols := []string{"Proveedor", "Moneda", "Concepto", "Documentos", "Monto", "Cuenta Bancaria", "Identificación"}
	for i, c := range cols {
		celda, _ := excelize.CoordinatesToCellName(i+1, 3)
		set(f, h, celda, c)
		_ = f.SetCellStyle(h, celda, celda, e.cabecera)
	}
	_ = f.SetRowHeight(h, 3, 22)

	filas := []filaPago{
		{"Daniel Esteban Chaves Arguedas", "Colones", "Urnas Personalizadas",
			"FC 07 10 13 14 URNAS ORSM 2572 2760 2822 2881 DANIEL CHAVES",
			"CR38010200009432533105", "01-1715-0521", 72320.00},
		{"Gas Tomza De Costa Rica SA", "Colones", "Suministro de Gas",
			"FC 3020250 22674 25022 25023 25024 GAS CREMATORIO TOMZA",
			"CR63015100210010133038", "3-101-349880", 216174.09},
		{"Pamela Mendez Cespedes", "Colones", "Joyería y Alhajas",
			"FC 228 229 JOYAS PERSONALISADAS TRIBUTO JOYAS CON AMOR",
			"CR54015114620010230401", "03-0440-0075", 285000.00},
		{"ZoAart Artesanías", "Colones", "Urnas Personalizadas",
			"FC 87 88 ORS 2743 2866 2852 2937 2991 2870 URNAS ZOAART",
			"CR46015107820010459999", "3-102-897077", 237300.00},
	}
	total := 0.0
	r := 4
	for i, fp := range filas {
		txt, mon := e.texto, e.monto
		if i%2 == 1 {
			txt, mon = e.textoAlt, e.montoAlt
		}
		set(f, h, fmt.Sprintf("A%d", r), fp.proveedor)
		set(f, h, fmt.Sprintf("B%d", r), fp.moneda)
		set(f, h, fmt.Sprintf("C%d", r), fp.concepto)
		set(f, h, fmt.Sprintf("D%d", r), fp.documentos)
		set(f, h, fmt.Sprintf("E%d", r), fp.monto)
		set(f, h, fmt.Sprintf("F%d", r), fp.cuenta)
		set(f, h, fmt.Sprintf("G%d", r), fp.identificacion)
		_ = f.SetCellStyle(h, fmt.Sprintf("A%d", r), fmt.Sprintf("D%d", r), txt)
		_ = f.SetCellStyle(h, fmt.Sprintf("E%d", r), fmt.Sprintf("E%d", r), mon)
		_ = f.SetCellStyle(h, fmt.Sprintf("F%d", r), fmt.Sprintf("G%d", r), txt)
		_ = f.SetRowHeight(h, r, 30)
		total += fp.monto
		r++
	}

	// PROPUESTA: el total al pie. Hoy el cuadro no lo trae y es justo el número que hay que
	// comparar contra el archivo que se sube al banco.
	mergeConEstilo(f, h, fmt.Sprintf("A%d", r), fmt.Sprintf("D%d", r), e.rotuloFuerte)
	set(f, h, fmt.Sprintf("A%d", r), "TOTAL A TRANSFERIR EN COLONES")
	set(f, h, fmt.Sprintf("E%d", r), total)
	_ = f.SetCellStyle(h, fmt.Sprintf("E%d", r), fmt.Sprintf("E%d", r), e.valorFuerte)
	mergeConEstilo(f, h, fmt.Sprintf("F%d", r), fmt.Sprintf("G%d", r), e.rotuloFuerte)
	_ = f.SetRowHeight(h, r, 20)

	r += 2
	mergeConEstilo(f, h, fmt.Sprintf("A%d", r), fmt.Sprintf("G%d", r), e.nota)
	set(f, h, fmt.Sprintf("A%d", r), "Este detalle corresponde al archivo de pago en colones del corte N° 0043. "+
		"Las facturas en dólares viajan en su propio cuadro y en su propio archivo.")

	_ = f.SetSheetView(h, 0, &excelize.ViewOptions{ShowGridLines: boolPtr(false)})
	_ = f.SetPanes(h, &excelize.Panes{Freeze: true, Split: false, XSplit: 0, YSplit: 3,
		TopLeftCell: "A4", ActivePane: "bottomLeft"})
}

// ─────────────────────────────────────────────────────────────────────────────────────────────
// HOJA 2 · Solicitud de aprobación — lo que firma la Gerencia Financiera ANTES de pagar
// ─────────────────────────────────────────────────────────────────────────────────────────────

type filaAprob struct {
	proveedor, identificacion, moneda, clasificacion string
	monto, equivalente                               float64
}

func hojaAprobacion(f *excelize.File, e est) {
	const h = "Aprobación GF"
	if _, err := f.NewSheet(h); err != nil {
		panic(err)
	}
	anchos := map[string]float64{"A": 44, "B": 16, "C": 10, "D": 16, "E": 18, "F": 24}
	for col, w := range anchos {
		_ = f.SetColWidth(h, col, col, w)
	}

	mergeConEstilo(f, h, "A1", "F1", e.titulo)
	set(f, h, "A1", "SOLICITUD DE APROBACIÓN DE PAGO PROGRAMADO")
	_ = f.SetRowHeight(h, 1, 26)

	mergeConEstilo(f, h, "A2", "F2", e.nota)
	set(f, h, "A2", "Valle de Paz  ·  Corte N° 0043  ·  Fecha de corte 04/11/2026  ·  "+
		"32 proveedores  ·  Tipo de cambio aplicado ₡450,00  ·  Preparado por Administrador GPVDP el 04/11/2026 15:40")
	_ = f.SetRowHeight(h, 2, 16)

	cols := []string{"Proveedor", "Identificación", "Moneda", "Monto", "Equivalente en ₡", "Clasificación"}
	for i, c := range cols {
		celda, _ := excelize.CoordinatesToCellName(i+1, 3)
		set(f, h, celda, c)
		_ = f.SetCellStyle(h, celda, celda, e.cabecera)
	}
	_ = f.SetRowHeight(h, 3, 22)

	const (
		normal  = "Flujo normal"
		reserva = "Reserva BP Negocios"
		fuera   = "Solicitud por fuera"
	)
	filas := []filaAprob{
		{"Asociacion Solidarista Empleados de Irex de Costa Rica y Afines", "3-002-045128", "CRC", normal, 20444.06, 20444.06},
		{"Ati Capital Solutions S.A.", "3-101-770011", "USD", normal, 5289.28, 2380176.00},
		{"Augusto Cesar Bonilla Campos", "01-0902-0456", "CRC", normal, 4695600.00, 4695600.00},
		{"Comercializadora Triple Oro de Costa Rica S.A", "3-101-556677", "CRC", normal, 423750.00, 423750.00},
		{"CORPORACION RMJ TECH SOCIEDAD ANONIMA", "3-101-778899", "CRC", normal, 8850.00, 8850.00},
		{"Daniel Esteban Hernández Sterling", "01-1402-0789", "CRC", normal, 15820.00, 15820.00},
		{"Delgado Valverde Magaly Tatiana", "02-0511-0334", "CRC", normal, 133350.00, 133350.00},
		{"Distribuidora Comercial Tres Ases S.A", "3-101-112233", "CRC", normal, 242778.75, 242778.75},
		{"Dr Office CR Sociedad Anonima", "3-101-334455", "CRC", normal, 161681.69, 161681.69},
		{"Eduardo Mauricio Angulo Zamora", "01-0788-0112", "CRC", normal, 30000.00, 30000.00},
		{"Electritel Sociedad Anonima", "3-101-667788", "CRC", reserva, 912475.00, 912475.00},
		{"Floristería Ebenezer", "01-1203-0998", "CRC", fuera, 2383000.00, 2383000.00},
		{"Floristeria y Regalos Curiosidades", "01-0955-0221", "CRC", normal, 90000.00, 90000.00},
		{"Funerales Roca Eterna SA", "3-101-889900", "CRC", normal, 197750.00, 197750.00},
		{"Funeraria El Ministerio Sociedad Anonima", "3-101-443322", "CRC", normal, 44000.01, 44000.01},
		{"Gas Tomza de Costa Rica S.A", "3-101-349880", "CRC", fuera, 249209.29, 249209.29},
		{"GOS Servicios Integrales XXI Sociedad Anónima", "3-101-220044", "CRC", normal, 1068029.67, 1068029.67},
		{"Ivette Maria Saenz Faerron", "01-0644-0877", "CRC", normal, 130000.00, 130000.00},
		{"Jorge Mario Artavia Hidalgo", "02-0399-0455", "CRC", normal, 107350.00, 107350.00},
		{"KEISY MELISSA CECILIANO MATA", "01-1688-0203", "CRC", normal, 286000.00, 286000.00},
		{"KEYLOR JAVIER RODRIGUEZ CONEJO", "01-1455-0677", "CRC", fuera, 92500.00, 92500.00},
		{"La Tulecr Fcs Sociedad Anonima", "3-101-991122", "CRC", fuera, 150000.00, 150000.00},
		{"LANCO PAINTS SA", "3-101-005566", "CRC", normal, 192680.00, 192680.00},
		{"Mainor Moya Rojas", "01-0877-0344", "CRC", normal, 2517640.00, 2517640.00},
		{"Masis Fuentes Diego Steven", "01-1799-0566", "CRC", normal, 152550.00, 152550.00},
		{"MAYCSOLUCIONESCR SOCIEDAD ANONIMA", "3-101-667799", "CRC", normal, 161319.80, 161319.80},
		{"R y L Garage", "3-101-443311", "CRC", normal, 238145.24, 238145.24},
		{"Sharom Paulette Vargas Leon", "01-1566-0788", "CRC", normal, 276850.00, 276850.00},
		{"Silvia Teresita Acuna Solano", "02-0455-0199", "CRC", normal, 242950.00, 242950.00},
		{"Superfotos Shops S.A.", "3-101-228833", "CRC", normal, 50135.00, 50135.00},
		{"Vital Soluciones S.A.", "3-101-556644", "CRC", normal, 184331.25, 184331.25},
		{"Volcano 506 S.A", "3-101-774411", "CRC", normal, 50252.91, 50252.91},
	}

	var totCRC, totUSDEnCRC, totReserva, totFuera float64
	r := 4
	for i, fa := range filas {
		txt, mon := e.texto, e.monto
		if i%2 == 1 {
			txt, mon = e.textoAlt, e.montoAlt
		}
		switch fa.clasificacion {
		case reserva:
			txt, mon = e.textoVerde, e.montoVerde
			totReserva += fa.equivalente
		case fuera:
			txt, mon = e.textoAmar, e.montoAmar
			totFuera += fa.equivalente
		}
		if fa.moneda == "USD" {
			totUSDEnCRC += fa.equivalente
		} else {
			totCRC += fa.monto
		}
		set(f, h, fmt.Sprintf("A%d", r), fa.proveedor)
		set(f, h, fmt.Sprintf("B%d", r), fa.identificacion)
		set(f, h, fmt.Sprintf("C%d", r), fa.moneda)
		set(f, h, fmt.Sprintf("D%d", r), fa.monto)
		set(f, h, fmt.Sprintf("E%d", r), fa.equivalente)
		set(f, h, fmt.Sprintf("F%d", r), fa.clasificacion)
		_ = f.SetCellStyle(h, fmt.Sprintf("A%d", r), fmt.Sprintf("C%d", r), txt)
		_ = f.SetCellStyle(h, fmt.Sprintf("D%d", r), fmt.Sprintf("E%d", r), mon)
		_ = f.SetCellStyle(h, fmt.Sprintf("F%d", r), fmt.Sprintf("F%d", r), txt)
		r++
	}

	// ── RESUMEN PAGO PROGRAMADO · la cascada, con la misma aritmética del cuadro actual ──
	r++
	mergeConEstilo(f, h, fmt.Sprintf("A%d", r), fmt.Sprintf("F%d", r), e.rotuloFuerte)
	set(f, h, fmt.Sprintf("A%d", r), "RESUMEN PAGO PROGRAMADO")
	_ = f.SetRowHeight(h, r, 20)
	r++

	totColonizado := totCRC + totUSDEnCRC
	flujoNormal := totColonizado - totReserva
	cronTotal := flujoNormal - totFuera
	cronColones := cronTotal - totUSDEnCRC

	lineas := []struct {
		rotulo string
		valor  float64
		tono   string // "", "verde", "amarillo", "fuerte"
	}{
		{"Total colones", totCRC, ""},
		{"Total dólares colonizado  (US$ 5.289,28 × ₡450,00)", totUSDEnCRC, ""},
		{"TOTAL COLONIZADO", totColonizado, "fuerte"},
		{"(−) Reserva BP Negocios", totReserva, "verde"},
		{"= Total flujo normal Valle", flujoNormal, ""},
		{"(−) Solicitudes por fuera", totFuera, "amarillo"},
		{"= CRONOGRAMA TOTAL FLUJO NORMAL", cronTotal, "fuerte"},
		{"(−) Dólares colonizados", totUSDEnCRC, ""},
		{"= Cronograma colones flujo normal", cronColones, ""},
	}
	for _, l := range lineas {
		rot, val := e.rotulo, e.valor
		switch l.tono {
		case "verde":
			rot, val = e.leyendaV, e.montoVerde
		case "amarillo":
			rot, val = e.leyendaA, e.montoAmar
		case "fuerte":
			rot, val = e.rotuloFuerte, e.valorFuerte
		}
		mergeConEstilo(f, h, fmt.Sprintf("A%d", r), fmt.Sprintf("D%d", r), rot)
		set(f, h, fmt.Sprintf("A%d", r), l.rotulo)
		set(f, h, fmt.Sprintf("E%d", r), l.valor)
		_ = f.SetCellStyle(h, fmt.Sprintf("E%d", r), fmt.Sprintf("E%d", r), val)
		_ = f.SetCellStyle(h, fmt.Sprintf("F%d", r), fmt.Sprintf("F%d", r), rot)
		r++
	}

	// ── Leyenda ──
	r++
	mergeConEstilo(f, h, fmt.Sprintf("A%d", r), fmt.Sprintf("B%d", r), e.leyendaV)
	set(f, h, fmt.Sprintf("A%d", r), "Reserva BP Negocios")
	mergeConEstilo(f, h, fmt.Sprintf("C%d", r), fmt.Sprintf("F%d", r), e.nota)
	set(f, h, fmt.Sprintf("C%d", r), "Sale de la reserva, no del flujo normal de Valle.")
	r++
	mergeConEstilo(f, h, fmt.Sprintf("A%d", r), fmt.Sprintf("B%d", r), e.leyendaA)
	set(f, h, fmt.Sprintf("A%d", r), "Solicitud por fuera")
	mergeConEstilo(f, h, fmt.Sprintf("C%d", r), fmt.Sprintf("F%d", r), e.nota)
	set(f, h, fmt.Sprintf("C%d", r), "Entró fuera del cronograma programado.")

	// ── Aprobación ──
	r += 2
	mergeConEstilo(f, h, fmt.Sprintf("A%d", r), fmt.Sprintf("F%d", r), e.rotuloFuerte)
	set(f, h, fmt.Sprintf("A%d", r), "APROBACIÓN")
	_ = f.SetRowHeight(h, r, 20)
	r += 3

	firmas := [][2]string{
		{"A", "Preparado por"}, {"C", "Revisado por"}, {"E", "Aprobado por"},
	}
	for _, fi := range firmas {
		col := fi[0]
		colFin := string(rune(col[0] + 1))
		_ = f.SetCellStyle(h, fmt.Sprintf("%s%d", col, r), fmt.Sprintf("%s%d", colFin, r), e.firmaLinea)
		_ = f.MergeCell(h, fmt.Sprintf("%s%d", col, r), fmt.Sprintf("%s%d", colFin, r))
	}
	r++
	for _, fi := range firmas {
		col := fi[0]
		colFin := string(rune(col[0] + 1))
		mergeConEstilo(f, h, fmt.Sprintf("%s%d", col, r), fmt.Sprintf("%s%d", colFin, r), e.firma)
		set(f, h, fmt.Sprintf("%s%d", col, r), fi[1])
	}
	r++
	for _, fi := range firmas {
		col := fi[0]
		colFin := string(rune(col[0] + 1))
		mergeConEstilo(f, h, fmt.Sprintf("%s%d", col, r), fmt.Sprintf("%s%d", colFin, r), e.nota)
	}
	set(f, h, fmt.Sprintf("A%d", r), "Tesorería · nombre y fecha")
	set(f, h, fmt.Sprintf("C%d", r), "Contabilidad · nombre y fecha")
	set(f, h, fmt.Sprintf("E%d", r), "Gerencia Financiera · nombre y fecha")

	r += 2
	mergeConEstilo(f, h, fmt.Sprintf("A%d", r), fmt.Sprintf("F%d", r), e.nota)
	set(f, h, fmt.Sprintf("A%d", r), "La aprobación de este corte queda registrada en el sistema con el usuario, "+
		"la fecha y el total aprobado. Si el detalle cambia después de aprobado, la aprobación se invalida y hay que volver a pedirla.")

	_ = f.SetSheetView(h, 0, &excelize.ViewOptions{ShowGridLines: boolPtr(false)})
	_ = f.SetPanes(h, &excelize.Panes{Freeze: true, Split: false, XSplit: 0, YSplit: 3,
		TopLeftCell: "A4", ActivePane: "bottomLeft"})
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("uso: maqueta_pago <ruta.xlsx>")
		os.Exit(1)
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	e := estilos(f)
	hojaAviso(f, e)
	hojaAprobacion(f, e)
	f.SetActiveSheet(0)
	if err := f.SaveAs(os.Args[1]); err != nil {
		panic(err)
	}
	fmt.Println("escrito:", os.Args[1])
	fmt.Println("verificación:")
	if n := verificar(os.Args[1]); n > 0 {
		fmt.Printf("%d comprobación(es) fallaron\n", n)
		os.Exit(1)
	}
	fmt.Println("todo cuadra")
}
