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
func (s *Service) ExportarMovimientosXLSX(ctx context.Context, empresaID string, f FiltrosMovimientos, usuarioID string, op OpcionesReporte) ([]byte, int, string, error) {
	movs, err := s.repo.MovimientosParaExport(ctx, empresaID, f)
	if err != nil {
		return nil, 0, "", err
	}
	if len(movs) == 0 {
		return nil, 0, "", ErrExportacionVacia
	}
	empresa, detalleEmpresa, usuario, err := s.repo.EncabezadoReporte(ctx, empresaID, usuarioID)
	if err != nil {
		return nil, 0, "", err
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
		// La nota nombra la columna tal como sale en la hoja. Cuando la columna se renombró a
		// «Equivalencia», esta línea quedó citando un nombre que ya no existía en ninguna parte del
		// archivo, y una nota que manda a buscar una columna inexistente es peor que no tenerla.
		"Los totales están en colones y salen de la columna «Equivalencia». " +
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
		return nil, 0, "", err
	}
	s.auditarExport(ctx, empresaID, usuarioID, "movimientos", etiquetaPeriodos(f), len(movs))
	// El nombre lo arma quien armó el contenido: describe lo que el archivo TRAE (las
	// clasificaciones que de verdad tuvieron movimientos), y eso solo se sabe acá.
	return buf, len(movs), nombreArchivoMovimientos(empresa, f, movs), nil
}

// nombreArchivoMovimientos arma el nombre de descarga del detalle: «VDP Asoc + Dep Agosto 03092026».
//
// Las clasificaciones se leen de lo EXPORTADO y no de los IDs del filtro, por la misma razón que el
// encabezado (ver describirFiltros): una clasificación no se identifica sola —dos conceptos pueden
// tener «Comisiones»— y en los movimientos ya viene resuelta. Además así el nombre describe lo que
// el archivo TRAE: si el filtro pedía cinco clasificaciones y solo tres tuvieron movimientos, el
// nombre nombra esas tres.
func nombreArchivoMovimientos(empresa string, f FiltrosMovimientos, movs []MovimientoExport) string {
	clasifs := []string{}
	// Solo se nombran cuando se FILTRÓ por clasificación. Sin filtro el archivo es «Completo», y
	// listar las 40 partidas que aparecieron no sería un nombre de archivo.
	if len(f.ClasificacionIDs) > 0 || f.ClasificacionID != "" {
		clasifs = clasificacionesEnOrdenDelFiltro(f, movs)
	}
	return NombreArchivoReporte(empresa, EtiquetaClasificaciones(clasifs), MesDelReporte(f, AhoraCR()), AhoraCR(), "")
}

// clasificacionesEnOrdenDelFiltro devuelve los nombres de las clasificaciones EXPORTADAS, en el
// orden en que el usuario las eligió en el filtro (decisión del usuario, 2026-09-03).
//
// Ese orden es el que él escribe a mano —«Serv + Dep», no «Dep + Serv»—, así que el archivo sale
// como lo archiva. La alternativa era el orden alfabético, más estable pero ajeno: obligaba a
// renombrar.
//
// Se ordena por el ID y no por el nombre porque dos conceptos distintos pueden tener una
// clasificación con el mismo nombre, y ahí el nombre no alcanza para saber cuál eligió.
func clasificacionesEnOrdenDelFiltro(f FiltrosMovimientos, movs []MovimientoExport) []string {
	// Posición de cada clasificación en el filtro. La singular va primero: es como pide la hoja de
	// trabajo, que elige de a una.
	posicion := map[string]int{}
	pedidos := append([]string{}, f.ClasificacionID)
	pedidos = append(pedidos, f.ClasificacionIDs...)
	for _, id := range pedidos {
		if id == "" {
			continue
		}
		if _, ya := posicion[id]; !ya {
			posicion[id] = len(posicion)
		}
	}

	// Nombre de cada clasificación que DE VERDAD salió, con su posición.
	type entrada struct {
		nombre string
		pos    int
	}
	vistas := map[string]bool{}
	out := []entrada{}
	for _, m := range movs {
		if m.Clasificacion == "" || vistas[m.ClasificacionID] {
			continue
		}
		vistas[m.ClasificacionID] = true
		pos, enElFiltro := posicion[m.ClasificacionID]
		if !enElFiltro {
			// No debería pasar (el filtro las acotó), pero si pasa van al final en vez de
			// desaparecer: un nombre incompleto es peor que uno con una partida de más.
			pos = len(posicion) + len(out)
		}
		out = append(out, entrada{nombre: m.Clasificacion, pos: pos})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].pos < out[j].pos })

	nombres := make([]string, 0, len(out))
	for _, e := range out {
		nombres = append(nombres, e.nombre)
	}
	return nombres
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

// columnasDetalle es el layout ÚNICO de las dos hojas de detalle, en el orden que fijó el negocio
// (2026-09-03):
//
//	Fecha · Consecutivo · Débito · Crédito · Equivalencia · Descripción · Banco Cuenta ·
//	Concepto · Clasificación · Consecutivo largo
//
// Los tres montos van juntos y ANTES de la descripción: es el orden en que se lee un estado de
// cuenta, y deja las tres columnas que se suman una al lado de la otra.
//
// Nombres: son los que usa el negocio, no los de la base. «Consecutivo» es `documento` (la
// referencia que da el banco) y «Equivalencia» es el monto en colones. «Banco Cuenta» es UNA columna
// con las dos cosas —«Davivienda · Colones»—, y trae de vuelta la cuenta: el débito y el crédito
// están en la moneda ORIGINAL, así que saber en qué cuenta cayó el movimiento es lo que dice si esos
// montos son colones o dólares.
//
// Está en una sola función a propósito: cuando cada hoja armaba sus columnas por su lado, terminaron
// con nombres distintos para el mismo dato.
func columnasDetalle() []ColumnaReporte {
	return []ColumnaReporte{
		{Titulo: "Fecha", Ancho: 11, Tipo: "fecha"},
		{Titulo: "Consecutivo", Ancho: 18, Tipo: "texto"},
		{Titulo: "Débito", Ancho: 15, Tipo: "montoDebito"},
		{Titulo: "Crédito", Ancho: 15, Tipo: "monto"},
		{Titulo: "Equivalencia", Ancho: 17, Tipo: "monto"},
		{Titulo: "Descripción", Ancho: 52, Tipo: "texto"},
		{Titulo: "Banco Cuenta", Ancho: 30, Tipo: "texto"},
		{Titulo: "Concepto", Ancho: 22, Tipo: "texto"},
		{Titulo: "Clasificación", Ancho: 28, Tipo: "texto"},
		{Titulo: "Consecutivo largo", Ancho: 27, Tipo: "texto"},
	}
}

// filaDetalle arma la fila en el MISMO orden que columnasDetalle. Las dos van juntas: separarlas es
// lo que produce un .xlsx con los montos corridos una columna.
func filaDetalle(m MovimientoExport, concepto, clasificacion string) []any {
	return []any{
		m.Fecha, m.Documento,
		montoNum(m.Debito), montoNum(m.Credito), montoNum(m.MontoCRC),
		m.Descripcion, bancoCuenta(m),
		concepto, clasificacion,
		ConsecutivoLargo(m.Banco, m.Descripcion),
	}
}

// bancoCuenta identifica en UNA columna en qué cuenta cayó el movimiento.
//
// No concatena a ciegas: en esta empresa el alias de la cuenta YA nombra el banco en 13 de las 15
// cuentas («Davivienda Colones», «BN Jardines Dólares»), así que pegar los dos daba
// «Davivienda · Davivienda Colones». Un dato repetido en cada fila se lee como un error del sistema.
//
// Así que el banco se antepone solo cuando el alias no lo menciona —el caso de «BP Negocios», que
// nombra al Banco Popular por su abreviatura—, y ahí sí hace falta para no adivinar.
func bancoCuenta(m MovimientoExport) string {
	switch {
	case m.Cuenta == "":
		return m.Banco
	case m.Banco == "":
		return m.Cuenta
	case strings.Contains(norm(m.Cuenta), norm(m.Banco)):
		return m.Cuenta
	default:
		return m.Banco + " · " + m.Cuenta
	}
}

// nombreOSinClasificar nombra lo que no tiene partida en vez de dejar la celda vacía: una celda en
// blanco se lee como «se me olvidó» y no como «está pendiente de clasificar».
func nombreOSinClasificar(s string) string {
	if s == "" {
		return "Sin clasificar"
	}
	return s
}

func hojaDetalleAgrupado(meta MetaReporte, movs []MovimientoExport) HojaReporte {
	meta = conPresentacion(meta, "Agrupado por partida con subtotales")
	cols := columnasDetalle()
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
		// El concepto y la clasificación también van en la FILA, no solo en el encabezado del
		// grupo: así cada fila se sostiene sola cuando alguien filtra o hace una tabla dinámica
		// sobre la hoja, que es lo que la gente hace con un .xlsx.
		filas = append(filas, FilaReporte{
			Grupo:   partidaDe(m),
			Valores: filaDetalle(m, nombreOSinClasificar(m.Concepto), nombreOSinClasificar(m.Clasificacion)),
		})
	}
	return HojaReporte{
		Nombre: "Movimientos", Meta: meta, Cols: cols, Filas: filas, AgruparConSubtotales: true,
	}
}

func hojaDetalleCorrido(meta MetaReporte, movs []MovimientoExport) HojaReporte {
	meta = conPresentacion(meta, "Listado corrido por fecha")
	cols := columnasDetalle()
	// `MovimientosParaExport` ya viene ORDER BY fecha, id: el listado corrido es cronológico, que
	// es como se lee un estado de cuenta. No se reordena.
	filas := make([]FilaReporte, 0, len(movs))
	for _, m := range movs {
		filas = append(filas, FilaReporte{
			Valores: filaDetalle(m, nombreOSinClasificar(m.Concepto), m.Clasificacion),
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
		{Titulo: "Equivalencia", Ancho: 17, Tipo: "monto"},
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
		{Titulo: "Equivalencia", Ancho: 17, Tipo: "monto"},
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
