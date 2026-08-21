package bancos

// Exportación a .xlsx REAL (Fase D, §30) con excelize. Los montos se escriben como
// número para que el equipo los sume/ordene en Excel; el dato autoritativo sigue
// siendo el decimal en la base (aquí solo es presentación). El Consecutivo Largo
// se deriva por fila para Davivienda.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/gpvdp/erp/internal/shared"
)

// montoNum convierte un decimal-string a float64 SOLO para la celda de Excel
// (display). No se usa en cálculos de negocio (regla: dinero = decimal en el core).
func montoNum(s string) float64 {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return 0
	}
	f, _ := d.Float64()
	return f
}

// OpcionesReporte son las decisiones de PRESENTACIÓN del reporte. Van aparte de los filtros a
// propósito: los filtros deciden QUÉ movimientos entran, esto decide CÓMO se ven. Mezclarlos haría
// que cambiar la presentación pareciera cambiar el conjunto de datos.
type OpcionesReporte struct {
	// AgruparPorPartida elige entre las dos formas del detalle, y las dos tienen que funcionar:
	//
	//   true  — bandas «Concepto › Clasificación» con el subtotal de cada partida. Es la vista de
	//           análisis: contesta «cuánto se movió en cada rubro».
	//   false — LISTADO CORRIDO: una fila por movimiento en orden de fecha, sin bandas ni
	//           subtotales, con la partida en columnas propias y autofiltro. Es la vista de
	//           trabajo: se pega en otra hoja, se ordena por lo que sea y se hace tabla dinámica.
	//
	// El gran total y las hojas de resumen van en las dos: cambiar la presentación no puede
	// cambiar los números.
	AgruparPorPartida bool
}

// ExportarMovimientosXLSX arma el reporte de movimientos del período.
//
// Presentación de reporte financiero (ver export_libro.go): sin cuadrícula, con encabezado de
// documento que identifica la empresa y quién lo emitió, fechas dd/mm/aaaa reales y montos en
// formato contable. El detalle sale agrupado por partida con subtotales o como listado corrido,
// según `op`. Trae además una hoja de resumen por partida y otra por cuenta bancaria: son las dos
// preguntas que se hacen al abrir el archivo, y no dependen de la presentación elegida.
func (s *Service) ExportarMovimientosXLSX(ctx context.Context, empresaID string, f FiltrosMovimientos, usuarioID string, op OpcionesReporte) ([]byte, int, error) {
	movs, err := s.repo.MovimientosParaExport(ctx, empresaID, f)
	if err != nil {
		return nil, 0, err
	}
	if len(movs) == 0 {
		return nil, 0, ErrExportacionVacia
	}
	empresa, detalleEmpresa, usuario, err := s.repo.EncabezadoReporte(ctx, empresaID, usuarioID)
	if err != nil {
		return nil, 0, err
	}

	titulo := "Detalle de movimientos bancarios"
	if f.Tipo == "CREDITO" {
		titulo = "Detalle de ingresos (créditos)"
	} else if f.Tipo == "DEBITO" {
		titulo = "Detalle de egresos (débitos)"
	}
	// El encabezado DECLARA con qué se filtró. Sin eso el reporte no es reproducible: dos archivos
	// con el mismo nombre pueden traer conjuntos distintos y nadie lo nota.
	filtros := s.describirFiltros(ctx, empresaID, f, movs)

	// Nota al pie sobre los USD sin convertir: el total en colones se quedaría corto y hay que
	// decirlo en el reporte, no solo en la pantalla.
	sinTC, montoSinTC := 0, 0.0
	for _, m := range movs {
		if m.Moneda != "CRC" && montoNum(m.MontoCRC) == 0 {
			sinTC++
			montoSinTC += montoNum(m.Credito) + montoNum(m.Debito)
		}
	}
	avisos := []string{
		"Los totales están en colones y salen de la columna «Equivalente CRC». " +
			"Débito y Crédito van en la moneda de cada cuenta.",
	}
	if sinTC > 0 {
		avisos = append(avisos, fmt.Sprintf(
			"%d movimiento(s) por USD %s no tienen tipo de cambio del mes y NO suman al total en colones",
			sinTC, formatoMiles(montoSinTC)))
	}

	// Si el reporte quedó en una sola cuenta, se identifica en el encabezado como en un estado
	// de cuenta. Con varias no se pone: sería mentir por omisión.
	cuenta := ""
	if cs := valoresUnicos(movs, func(m MovimientoExport) string { return m.Banco + " · " + m.Cuenta }); len(cs) == 1 {
		cuenta = "Cuenta: " + cs[0]
	}

	meta := MetaReporte{
		Empresa: empresa, EmpresaDetalle: detalleEmpresa, Cuenta: cuenta,
		Titulo: titulo, Periodo: etiquetaPeriodos(f), Filtros: filtros,
		GeneradoPor: usuario, GeneradoEn: AhoraCR(), Avisos: avisos,
	}

	hojas := []HojaReporte{
		hojaDetalleMovimientos(meta, movs, op.AgruparPorPartida),
		hojaResumenPorPartida(meta, movs),
		hojaResumenPorCuenta(meta, movs),
	}
	buf, err := ConstruirLibro(hojas)
	if err != nil {
		return nil, 0, err
	}
	s.auditarExport(ctx, empresaID, usuarioID, "movimientos", etiquetaPeriodos(f), len(movs))
	return buf, len(movs), nil
}

// hojaDetalleMovimientos arma el detalle en la forma pedida. Las DOS tienen que servir:
//
//	· agrupado  — bandas «Concepto › Clasificación» con subtotal de cada partida. La partida se
//	  identifica en la banda, así que no hace falta repetirla en cada fila.
//	· corrido   — una fila por movimiento en orden de fecha, sin bandas ni subtotales. Acá la
//	  partida SÍ va en columnas propias (Concepto y Clasificación): sin banda que la nombre,
//	  quitar los subtotales no puede costar saber de qué partida es cada movimiento. Lleva
//	  autofiltro para ordenar y filtrar en Excel, que es para lo que se pide corrido.
//
// El gran total, el encabezado y las hojas de resumen son idénticos en las dos.
func hojaDetalleMovimientos(meta MetaReporte, movs []MovimientoExport, agrupar bool) HojaReporte {
	if agrupar {
		return hojaDetalleAgrupado(meta, movs)
	}
	return hojaDetalleCorrido(meta, movs)
}

// conPresentacion agrega al encabezado la línea que dice cómo está presentado el detalle.
//
// Va SOLO en la hoja de detalle: las hojas de resumen no tienen presentación que elegir, y
// estamparles «Listado corrido» era afirmar algo que no aplica a esa hoja.
func conPresentacion(meta MetaReporte, texto string) MetaReporte {
	m := meta
	m.Filtros = append(append([][2]string{}, meta.Filtros...), [2]string{"Presentación", texto})
	return m
}

func hojaDetalleAgrupado(meta MetaReporte, movs []MovimientoExport) HojaReporte {
	meta = conPresentacion(meta, "Agrupado por partida con subtotales")
	cols := []ColumnaReporte{
		{Titulo: "Fecha", Ancho: 11, Tipo: "fecha"},
		{Titulo: "Banco", Ancho: 15, Tipo: "texto"},
		{Titulo: "Cuenta", Ancho: 22, Tipo: "texto"},
		{Titulo: "Documento", Ancho: 18, Tipo: "texto"},
		{Titulo: "Descripción", Ancho: 62, Tipo: "texto"},
		{Titulo: "Mon.", Ancho: 6, Tipo: "texto"},
		{Titulo: "Débito", Ancho: 15, Tipo: "montoDebito"},
		{Titulo: "Crédito", Ancho: 15, Tipo: "monto"},
		{Titulo: "Equivalente CRC", Ancho: 17, Tipo: "monto"},
		{Titulo: "Consecutivo largo", Ancho: 27, Tipo: "texto"},
	}
	// Ordenadas por partida y luego por fecha: así la agrupación sale contigua.
	ordenadas := make([]MovimientoExport, len(movs))
	copy(ordenadas, movs)
	sort.SliceStable(ordenadas, func(i, j int) bool {
		pi, pj := partidaDe(ordenadas[i]), partidaDe(ordenadas[j])
		if pi != pj {
			return pi < pj
		}
		return ordenadas[i].Fecha < ordenadas[j].Fecha
	})

	filas := make([]FilaReporte, 0, len(ordenadas))
	for _, m := range ordenadas {
		filas = append(filas, FilaReporte{
			Grupo: partidaDe(m),
			Valores: []any{
				m.Fecha, m.Banco, m.Cuenta, m.Documento, m.Descripcion, m.Moneda,
				montoNum(m.Debito), montoNum(m.Credito), montoNum(m.MontoCRC),
				ConsecutivoLargo(m.Banco, m.Descripcion),
			},
		})
	}
	return HojaReporte{
		Nombre: "Movimientos", Meta: meta, Cols: cols, Filas: filas, AgruparConSubtotales: true,
	}
}

func hojaDetalleCorrido(meta MetaReporte, movs []MovimientoExport) HojaReporte {
	meta = conPresentacion(meta, "Listado corrido por fecha")
	cols := []ColumnaReporte{
		{Titulo: "Fecha", Ancho: 11, Tipo: "fecha"},
		{Titulo: "Banco", Ancho: 15, Tipo: "texto"},
		{Titulo: "Cuenta", Ancho: 22, Tipo: "texto"},
		{Titulo: "Documento", Ancho: 18, Tipo: "texto"},
		{Titulo: "Descripción", Ancho: 52, Tipo: "texto"},
		{Titulo: "Concepto", Ancho: 22, Tipo: "texto"},
		{Titulo: "Clasificación", Ancho: 28, Tipo: "texto"},
		{Titulo: "Mon.", Ancho: 6, Tipo: "texto"},
		{Titulo: "Débito", Ancho: 15, Tipo: "montoDebito"},
		{Titulo: "Crédito", Ancho: 15, Tipo: "monto"},
		{Titulo: "Equivalente CRC", Ancho: 17, Tipo: "monto"},
		{Titulo: "Consecutivo largo", Ancho: 27, Tipo: "texto"},
	}
	// `MovimientosParaExport` ya viene ORDER BY fecha, id: el listado corrido es cronológico, que
	// es como se lee un estado de cuenta. No se reordena.
	filas := make([]FilaReporte, 0, len(movs))
	for _, m := range movs {
		concepto, clasificacion := m.Concepto, m.Clasificacion
		if concepto == "" {
			// Lo no clasificado se nombra, no se deja en blanco: una celda vacía se lee como
			// «se me olvidó» y no como «está pendiente de clasificar».
			concepto = "Sin clasificar"
		}
		filas = append(filas, FilaReporte{
			Valores: []any{
				m.Fecha, m.Banco, m.Cuenta, m.Documento, m.Descripcion,
				concepto, clasificacion, m.Moneda,
				montoNum(m.Debito), montoNum(m.Credito), montoNum(m.MontoCRC),
				ConsecutivoLargo(m.Banco, m.Descripcion),
			},
		})
	}
	return HojaReporte{
		Nombre: "Movimientos", Meta: meta, Cols: cols, Filas: filas, AgruparConSubtotales: false,
	}
}

// hojaResumenPorPartida es el cuadre: cuánto entró y salió por cada Concepto › Clasificación.
func hojaResumenPorPartida(meta MetaReporte, movs []MovimientoExport) HojaReporte {
	type acum struct {
		debito, credito, crc float64
		movs                 int
	}
	porPartida := map[string]*acum{}
	orden := []string{}
	for _, m := range movs {
		p := partidaDe(m)
		a, ok := porPartida[p]
		if !ok {
			a = &acum{}
			porPartida[p] = a
			orden = append(orden, p)
		}
		a.debito += montoNum(m.Debito)
		a.credito += montoNum(m.Credito)
		a.crc += montoNum(m.MontoCRC)
		a.movs++
	}
	sort.Strings(orden)

	cols := []ColumnaReporte{
		{Titulo: "Partida (Concepto › Clasificación)", Ancho: 52, Tipo: "texto"},
		{Titulo: "Movimientos", Ancho: 13, Tipo: "entero"},
		{Titulo: "Débitos", Ancho: 17, Tipo: "montoDebito"},
		{Titulo: "Créditos", Ancho: 17, Tipo: "monto"},
		{Titulo: "Equivalente CRC", Ancho: 17, Tipo: "monto"},
	}
	filas := make([]FilaReporte, 0, len(orden))
	for _, p := range orden {
		a := porPartida[p]
		filas = append(filas, FilaReporte{Valores: []any{p, a.movs, a.debito, a.credito, a.crc}})
	}
	m2 := meta
	m2.Titulo = "Resumen por partida"
	return HojaReporte{Nombre: "Resumen por partida", Meta: m2, Cols: cols, Filas: filas}
}

// hojaResumenPorCuenta responde «cuánto se movió en cada cuenta», que es la vista de tesorería.
func hojaResumenPorCuenta(meta MetaReporte, movs []MovimientoExport) HojaReporte {
	type acum struct {
		debito, credito, crc float64
		movs                 int
		moneda               string
	}
	porCuenta := map[string]*acum{}
	orden := []string{}
	for _, m := range movs {
		k := m.Banco + " · " + m.Cuenta
		a, ok := porCuenta[k]
		if !ok {
			a = &acum{moneda: m.Moneda}
			porCuenta[k] = a
			orden = append(orden, k)
		}
		a.debito += montoNum(m.Debito)
		a.credito += montoNum(m.Credito)
		a.crc += montoNum(m.MontoCRC)
		a.movs++
	}
	sort.Strings(orden)

	cols := []ColumnaReporte{
		{Titulo: "Banco · Cuenta", Ancho: 40, Tipo: "texto"},
		{Titulo: "Mon.", Ancho: 7, Tipo: "texto"},
		{Titulo: "Movimientos", Ancho: 13, Tipo: "entero"},
		{Titulo: "Débitos", Ancho: 17, Tipo: "montoDebito"},
		{Titulo: "Créditos", Ancho: 17, Tipo: "monto"},
		{Titulo: "Equivalente CRC", Ancho: 17, Tipo: "monto"},
	}
	filas := make([]FilaReporte, 0, len(orden))
	for _, k := range orden {
		a := porCuenta[k]
		filas = append(filas, FilaReporte{Valores: []any{k, a.moneda, a.movs, a.debito, a.credito, a.crc}})
	}
	m2 := meta
	m2.Titulo = "Resumen por cuenta bancaria"
	return HojaReporte{Nombre: "Resumen por cuenta", Meta: m2, Cols: cols, Filas: filas}
}

// partidaDe arma la etiqueta de la partida: «Concepto › Clasificación». Lo no clasificado se
// nombra explícitamente en vez de quedar en blanco.
func partidaDe(m MovimientoExport) string {
	switch {
	case m.Concepto != "" && m.Clasificacion != "":
		return m.Concepto + " › " + m.Clasificacion
	case m.Concepto != "":
		return m.Concepto
	default:
		return "Sin clasificar"
	}
}

// etiquetaPeriodo pasa "2026-08" a "Agosto 2026" para el encabezado.
func etiquetaPeriodo(periodo string) string {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return periodo
	}
	meses := []string{"", "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
		"Julio", "Agosto", "Setiembre", "Octubre", "Noviembre", "Diciembre"}
	return fmt.Sprintf("%s %d", meses[int(t.Month())], t.Year())
}

// etiquetaPeriodos redacta el período del encabezado según lo que se pidió: un mes, varios meses
// (contiguos se dicen como rango: «Junio a Agosto 2026»), un rango de fechas, o el histórico.
func etiquetaPeriodos(f FiltrosMovimientos) string {
	ps := append([]string{}, f.Periodos...)
	if f.Periodo != "" {
		ps = append(ps, f.Periodo)
	}
	if len(ps) == 0 {
		if f.Desde != "" || f.Hasta != "" {
			desde, hasta := f.Desde, f.Hasta
			if desde == "" {
				desde = "el inicio"
			} else {
				desde = fechaLegible(desde)
			}
			if hasta == "" {
				hasta = "hoy"
			} else {
				hasta = fechaLegible(hasta)
			}
			return desde + " al " + hasta
		}
		return "Histórico completo"
	}
	sort.Strings(ps)
	if len(ps) == 1 {
		return etiquetaPeriodo(ps[0])
	}
	if periodosContiguos(ps) {
		return etiquetaPeriodo(ps[0]) + " a " + etiquetaPeriodo(ps[len(ps)-1])
	}
	partes := make([]string, 0, len(ps))
	for _, p := range ps {
		partes = append(partes, etiquetaPeriodo(p))
	}
	return strings.Join(partes, " · ")
}

// periodosContiguos dice si la lista de YYYY-MM son meses consecutivos sin huecos.
func periodosContiguos(ps []string) bool {
	if len(ps) < 2 {
		return true
	}
	t, err := time.Parse("2006-01", ps[0])
	if err != nil {
		return false
	}
	for _, p := range ps[1:] {
		t = t.AddDate(0, 1, 0)
		if p != t.Format("2006-01") {
			return false
		}
	}
	return true
}

func fechaLegible(iso string) string {
	if t, err := time.Parse("2006-01-02", iso); err == nil {
		return t.Format("02/01/2006")
	}
	return iso
}

// describirFiltros arma el bloque «con qué se filtró» del encabezado. Los nombres de concepto y
// cuenta se resuelven contra el catálogo; si el filtro no restringe nada, no se escribe la línea
// (un encabezado lleno de «Todos» es ruido).
func (s *Service) describirFiltros(ctx context.Context, empresaID string, f FiltrosMovimientos, movs []MovimientoExport) [][2]string {
	out := [][2]string{}

	switch f.Tipo {
	case "DEBITO":
		out = append(out, [2]string{"Tipo", "Solo débitos"})
	case "CREDITO":
		out = append(out, [2]string{"Tipo", "Solo créditos"})
	default:
		out = append(out, [2]string{"Tipo", "Débitos y créditos"})
	}

	ids := append([]string{}, f.ConceptoIDs...)
	if f.ConceptoID != "" {
		ids = append(ids, f.ConceptoID)
	}
	if len(ids) > 0 {
		nombres := s.nombresDeConceptos(ctx, empresaID, ids)
		out = append(out, [2]string{"Conceptos", strings.Join(nombres, " · ")})
	} else {
		out = append(out, [2]string{"Conceptos", "Todos"})
	}

	// Clasificaciones: el nombre se lee de lo EXPORTADO en vez de consultarlo por ID, porque una
	// clasificación no se identifica sola (dos conceptos pueden tener «Comisiones») y en los
	// movimientos ya viene con su concepto.
	if len(f.ClasificacionIDs) > 0 || f.ClasificacionID != "" {
		out = append(out, [2]string{"Clasificaciones",
			nombreEnMovs(movs, func(m MovimientoExport) string { return m.Clasificacion })})
	}
	// Banco y cuenta se leen de lo exportado: si el filtro dejó una sola cuenta, se nombra.
	if f.CuentaID != "" || f.BancoID != "" {
		bancos := valoresUnicos(movs, func(m MovimientoExport) string { return m.Banco })
		cuentas := valoresUnicos(movs, func(m MovimientoExport) string { return m.Cuenta })
		if len(bancos) > 0 {
			out = append(out, [2]string{"Banco", strings.Join(bancos, " · ")})
		}
		if f.CuentaID != "" && len(cuentas) > 0 {
			out = append(out, [2]string{"Cuenta", strings.Join(cuentas, " · ")})
		}
	}
	if f.Estado == "NO_IDENTIFICADO" {
		out = append(out, [2]string{"Estado", "Solo lo que está sin clasificar"})
	}
	if f.Q != "" {
		out = append(out, [2]string{"Búsqueda", "«" + f.Q + "»"})
	}
	return out
}

func (s *Service) nombresDeConceptos(ctx context.Context, empresaID string, ids []string) []string {
	cats, err := s.repo.ListarConceptos(ctx, empresaID, false)
	if err != nil {
		return ids
	}
	porID := map[string]string{}
	for _, c := range cats {
		porID[c.ID] = c.Nombre
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := porID[id]; ok {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return ids
	}
	return out
}

func valoresUnicos(movs []MovimientoExport, de func(MovimientoExport) string) []string {
	visto := map[string]bool{}
	out := []string{}
	for _, m := range movs {
		v := de(m)
		if v != "" && !visto[v] {
			visto[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func nombreEnMovs(movs []MovimientoExport, de func(MovimientoExport) string) string {
	if v := valoresUnicos(movs, de); len(v) > 0 {
		return strings.Join(v, " · ")
	}
	return "—"
}

// ExportarCuadreXLSX arma el .xlsx del cuadre por concepto del período.
func (s *Service) ExportarCuadreXLSX(ctx context.Context, empresaID, periodo, usuarioID string) ([]byte, int, error) {
	cuadre, err := s.repo.Cuadre(ctx, empresaID, periodo)
	if err != nil {
		return nil, 0, err
	}
	if len(cuadre) == 0 {
		return nil, 0, ErrExportacionVacia
	}
	empresa, detalleEmpresa, usuario, err := s.repo.EncabezadoReporte(ctx, empresaID, usuarioID)
	if err != nil {
		return nil, 0, err
	}

	// Anchos generosos y formato contable en las tres columnas de monto. Es lo que faltaba: con
	// la columna angosta y sin formato, Excel mostraba «2,5E+07» y el número no se podía leer.
	cols := []ColumnaReporte{
		{Titulo: "Concepto", Ancho: 38, Tipo: "texto"},
		{Titulo: "Créditos", Ancho: 20, Tipo: "monto"},
		{Titulo: "Débitos", Ancho: 20, Tipo: "montoDebito"},
		{Titulo: "Neto", Ancho: 20, Tipo: "monto"},
	}
	filas := make([]FilaReporte, 0, len(cuadre))
	for _, c := range cuadre {
		// El neto se CALCULA en decimal (regla del proyecto); float64 solo para la celda.
		cred, _ := decimal.NewFromString(c.TotalCreditos)
		deb, _ := decimal.NewFromString(c.TotalDebitos)
		filas = append(filas, FilaReporte{Valores: []any{
			c.Concepto, montoNum(c.TotalCreditos), montoNum(c.TotalDebitos), montoNum(cred.Sub(deb).String()),
		}})
	}
	meta := MetaReporte{
		Empresa: empresa, EmpresaDetalle: detalleEmpresa,
		Titulo:  "Cuadre por concepto",
		Periodo: etiquetaPeriodo(periodo),
		Avisos: []string{
			"Neto = créditos − débitos. Un neto negativo significa que en el concepto salió más de lo que entró.",
		},
		GeneradoPor: usuario, GeneradoEn: AhoraCR(),
	}
	buf, err := ConstruirLibro([]HojaReporte{
		{Nombre: "Cuadre", Meta: meta, Cols: cols, Filas: filas},
	})
	if err != nil {
		return nil, 0, err
	}
	s.auditarExport(ctx, empresaID, usuarioID, "cuadre", periodo, len(cuadre))
	return buf, len(cuadre), nil
}

func (s *Service) auditarExport(ctx context.Context, empresaID, usuarioID, tipo, periodo string, filas int) {
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "exportacion", Accion: "EXPORTAR_XLSX", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"tipo": tipo, "periodo": periodo, "filas": filas},
	})
}

// construirXLSX genera un .xlsx con encabezado en negrita, fila superior congelada
// y autofiltro. Devuelve los bytes del archivo.
func construirXLSX(hoja string, headers []any, rows [][]any) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", hoja); err != nil {
		return nil, fmt.Errorf("bancos: nombrar hoja: %w", err)
	}
	if err := f.SetSheetRow(hoja, "A1", &headers); err != nil {
		return nil, fmt.Errorf("bancos: encabezado xlsx: %w", err)
	}
	for i, r := range rows {
		cell := fmt.Sprintf("A%d", i+2)
		row := r // copia local para tomar dirección
		if err := f.SetSheetRow(hoja, cell, &row); err != nil {
			return nil, fmt.Errorf("bancos: fila xlsx: %w", err)
		}
	}
	if style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err == nil {
		ultimaCol, _ := excelize.ColumnNumberToName(len(headers))
		_ = f.SetCellStyle(hoja, "A1", ultimaCol+"1", style)
		_ = f.SetPanes(hoja, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
		_ = f.AutoFilter(hoja, fmt.Sprintf("A1:%s1", ultimaCol), []excelize.AutoFilterOptions{})
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("bancos: serializar xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// formatoMiles escribe un monto con separador de miles para los AVISOS del reporte (texto, no
// celda numérica). Sin esto un aviso diría «28792.27» donde el resto del documento dice
// «28.792,27» y se lee como otro sistema.
func formatoMiles(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	partes := strings.SplitN(s, ".", 2)
	entero := partes[0]
	signo := ""
	if strings.HasPrefix(entero, "-") {
		signo, entero = "-", entero[1:]
	}
	var b strings.Builder
	for i, d := range entero {
		if i > 0 && (len(entero)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	out := signo + b.String()
	if len(partes) == 2 {
		out += "," + partes[1]
	}
	return out
}
