package bancos

// El análisis de partidas, exportado a Excel.
//
// ── POR QUÉ EXISTE ──────────────────────────────────────────────────────────────────────────
//
// La pantalla de «Análisis y tendencias» no tenía ningún botón de exportar. Para llevar el
// análisis a una reunión había que capturar la pantalla, y una captura no se puede ordenar, ni
// filtrar, ni sumar, ni pegar en una presentación sin que se vea como una captura.
//
// ── QUÉ TRAE Y POR QUÉ EN TRES HOJAS ────────────────────────────────────────────────────────
//
// 1. «Tendencia»     — la tabla completa: una fila por partida, una columna por mes. Es la que se
//                      lleva a una tabla dinámica.
// 2. «Concentración» — las partidas ordenadas por monto, con el acumulado. Contesta «¿dónde está
//                      el dinero?», que no es la misma pregunta que «¿qué se movió?». Medido en
//                      Memorial Pets: 10 partidas de 116 explican el 73 % del gasto.
// 3. «Salud»         — qué mes entró al promedio y cuál no. Sin esta hoja, el archivo sale del
//                      sistema sin la advertencia que la pantalla sí muestra, y un mes a medio
//                      clasificar se lee como una caída del gasto.

import (
	"context"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// ExportarAnalisis arma el .xlsx del análisis de partidas.
//
// `grupo` filtra por naturaleza igual que la pantalla ("GASTO", "INGRESO", "NEUTRO", "SIN_DECLARAR"
// o vacío = todas), para que el archivo traiga lo mismo que se está mirando y no otra cosa.
func (s *Service) ExportarAnalisis(ctx context.Context, empresaID, desde, hasta, grupo string, comparar bool, usuarioID string) ([]byte, string, error) {
	var (
		res AnalisisPartidas
		err error
	)
	if comparar {
		res, err = s.AnalisisPartidasComparado(ctx, empresaID, desde, hasta)
	} else {
		res, err = s.AnalisisPartidas(ctx, empresaID, desde, hasta)
	}
	if err != nil {
		return nil, "", err
	}

	partidas := filtrarPorGrupo(res.Partidas, grupo)
	if len(partidas) == 0 {
		return nil, "", ErrExportacionVacia
	}

	empresa, detalleEmpresa, usuario, err := s.repo.EncabezadoReporte(ctx, empresaID, usuarioID)
	if err != nil {
		return nil, "", err
	}

	periodo := etiquetaPeriodo(desde)
	if desde != hasta {
		periodo = etiquetaPeriodo(desde) + " a " + etiquetaPeriodo(hasta)
	}
	// Los avisos de la pantalla viajan con el archivo. Un .xlsx se reenvía y se abre semanas
	// después, sin la pantalla al lado: si la advertencia se queda atrás, el archivo miente solo.
	avisos := []string{
		"El promedio y el desvío se calculan solo sobre los meses clasificados al 90 % o más, y excluyen el último mes del rango, que es el que se está juzgando.",
	}
	if res.Aviso != "" {
		avisos = append(avisos, res.Aviso)
	}
	if res.AvisoAnual != "" {
		avisos = append(avisos, res.AvisoAnual)
	}

	meta := MetaReporte{
		Empresa: empresa, EmpresaDetalle: detalleEmpresa,
		Titulo: "Análisis de partidas", Periodo: periodo,
		Filtros:     [][2]string{{"Naturaleza", etiquetaGrupo(grupo)}},
		GeneradoPor: usuario, GeneradoEn: AhoraCR(), Avisos: avisos,
	}

	hojas := []HojaReporte{
		hojaTendencia(meta, res, partidas),
		hojaConcentracion(meta, partidas),
		hojaSalud(meta, res),
	}
	buf, err := ConstruirLibro(hojas)
	if err != nil {
		return nil, "", err
	}
	s.auditarExport(ctx, empresaID, usuarioID, "analisis", periodo, len(partidas))
	return buf, fmt.Sprintf("Analisis %s %s a %s.xlsx", empresa, desde, hasta), nil
}

// filtrarPorGrupo deja las partidas de una naturaleza. Es el mismo corte que hace la pantalla.
func filtrarPorGrupo(ps []TendenciaPartida, grupo string) []TendenciaPartida {
	if grupo == "" || grupo == "TODAS" {
		return ps
	}
	out := make([]TendenciaPartida, 0, len(ps))
	for _, p := range ps {
		switch grupo {
		case "SIN_DECLARAR":
			if !p.NaturalezaDeclarada {
				out = append(out, p)
			}
		default:
			// Una naturaleza sin declarar NO cuenta como su valor por defecto: «NEUTRO porque
			// nadie lo decidió» y «NEUTRO porque se decidió» son cosas distintas (ver mig 0064).
			if p.NaturalezaDeclarada && p.Naturaleza == grupo {
				out = append(out, p)
			}
		}
	}
	return out
}

// etiquetaRecurrencia pone en palabras lo que significa cada grado. En un .xlsx que alguien abre
// sin la pantalla al lado, «ESPORADICO» a secas no dice qué hacer con ese número.
func etiquetaRecurrencia(r string) string {
	switch r {
	case RecurrenciaFija:
		return "Fijo · casi todos los meses"
	case RecurrenciaIntermitente:
		return "Intermitente"
	default:
		return "Esporádico · el promedio no aplica"
	}
}

func etiquetaGrupo(g string) string {
	switch g {
	case "GASTO":
		return "Gastos"
	case "INGRESO":
		return "Ingresos"
	case "NEUTRO":
		return "Otros rubros (no afectan el resultado)"
	case "SIN_DECLARAR":
		return "Falta decidir si es ingreso o gasto"
	default:
		return "Todas las partidas"
	}
}

// hojaTendencia: una fila por partida, una columna por mes, y al final los indicadores.
func hojaTendencia(meta MetaReporte, res AnalisisPartidas, partidas []TendenciaPartida) HojaReporte {
	meta = conPresentacion(meta, "Una fila por partida, una columna por mes")

	cols := []ColumnaReporte{
		{Titulo: "Concepto", Ancho: 26, Tipo: "texto"},
		{Titulo: "Partida", Ancho: 30, Tipo: "texto"},
	}
	for _, m := range res.Meses {
		// El mes que no entró al promedio se marca EN SU ENCABEZADO. Es el único lugar donde la
		// advertencia viaja pegada a la columna que afecta.
		t := etiquetaPeriodo(m.Periodo)
		if !m.Comparable {
			t += " (parcial)"
		}
		cols = append(cols, ColumnaReporte{Titulo: t, Ancho: 15, Tipo: "monto"})
	}
	cols = append(cols,
		ColumnaReporte{Titulo: "Total del rango", Ancho: 16, Tipo: "monto"},
		ColumnaReporte{Titulo: "Promedio mensual", Ancho: 16, Tipo: "monto"},
		// La mediana va AL LADO del promedio y no en otra hoja: la comparación entre las dos es
		// el dato —cuando se separan mucho, el promedio está arrastrando un pago extraordinario—.
		ColumnaReporte{Titulo: "Mediana mensual", Ancho: 16, Tipo: "monto"},
		ColumnaReporte{Titulo: "Último mes", Ancho: 15, Tipo: "monto"},
		ColumnaReporte{Titulo: "Desvío % (promedio)", Ancho: 13, Tipo: "texto"},
		ColumnaReporte{Titulo: "Desvío % (mediana)", Ancho: 13, Tipo: "texto"},
		ColumnaReporte{Titulo: "Meses comparados", Ancho: 12, Tipo: "entero"},
		ColumnaReporte{Titulo: "Meses con gasto", Ancho: 12, Tipo: "entero"},
		ColumnaReporte{Titulo: "Recurrencia", Ancho: 14, Tipo: "texto"},
	)
	if res.ComparaAnual {
		cols = append(cols,
			ColumnaReporte{Titulo: "Mismo rango año anterior", Ancho: 18, Tipo: "monto"},
			ColumnaReporte{Titulo: "Variación anual %", Ancho: 14, Tipo: "texto"},
		)
	}

	filas := make([]FilaReporte, 0, len(partidas))
	for _, p := range partidas {
		v := []any{p.Concepto, p.Clasificacion}
		// La serie se recorre por los MESES del encabezado y se busca el punto, en vez de volcar
		// p.Serie en orden: si una partida no tuviera algún mes, todo lo de la derecha quedaría
		// corrido una columna y los montos se leerían bajo el mes equivocado.
		porPeriodo := map[string]string{}
		for _, pt := range p.Serie {
			porPeriodo[pt.Periodo] = pt.Monto
		}
		for _, m := range res.Meses {
			v = append(v, montoFloat(porPeriodo[m.Periodo]))
		}
		v = append(v, montoFloat(p.Total), montoFloat(p.Promedio), montoFloat(p.Mediana),
			montoFloat(p.Ultimo))
		// Los desvíos van como TEXTO y dicen «sin historia» cuando no son confiables: un 0,0 %
		// en una columna numérica se lee como «no se movió», que es lo contrario de «no se sabe».
		if p.Confiable {
			v = append(v, p.DesvioPct+" %", p.DesvioMedianaPct+" %")
		} else {
			v = append(v, "sin historia", "sin historia")
		}
		v = append(v, p.MesesConDato, p.MesesConGasto, etiquetaRecurrencia(p.Recurrencia))
		if res.ComparaAnual {
			v = append(v, montoFloat(p.TotalAnterior))
			if p.SinBaseAnterior {
				v = append(v, "no existía")
			} else {
				v = append(v, p.VariacionAnualPct+" %")
			}
		}
		filas = append(filas, FilaReporte{Valores: v})
	}
	return HojaReporte{Nombre: "Tendencia", Meta: meta, Cols: cols, Filas: filas}
}

// hojaConcentracion: dónde está el dinero, que no es lo mismo que qué se movió.
func hojaConcentracion(meta MetaReporte, partidas []TendenciaPartida) HojaReporte {
	meta = conPresentacion(meta, "Partidas ordenadas por monto, con el acumulado")

	orden := make([]TendenciaPartida, len(partidas))
	copy(orden, partidas)
	sort.SliceStable(orden, func(i, j int) bool {
		return montoFloat(orden[i].Total) > montoFloat(orden[j].Total)
	})

	total := decimal.Zero
	for _, p := range orden {
		m, _ := decimal.NewFromString(p.Total)
		total = total.Add(m)
	}

	cols := []ColumnaReporte{
		{Titulo: "#", Ancho: 6, Tipo: "entero"},
		{Titulo: "Concepto", Ancho: 26, Tipo: "texto"},
		{Titulo: "Partida", Ancho: 30, Tipo: "texto"},
		{Titulo: "Total del rango", Ancho: 16, Tipo: "monto"},
		{Titulo: "% del total", Ancho: 12, Tipo: "texto"},
		{Titulo: "% acumulado", Ancho: 12, Tipo: "texto"},
	}
	filas := make([]FilaReporte, 0, len(orden))
	acum := decimal.Zero
	for i, p := range orden {
		m, _ := decimal.NewFromString(p.Total)
		acum = acum.Add(m)
		pct, pctAcum := "—", "—"
		if total.IsPositive() {
			pct = m.Div(total).Mul(decimal.NewFromInt(100)).StringFixed(1) + " %"
			pctAcum = acum.Div(total).Mul(decimal.NewFromInt(100)).StringFixed(1) + " %"
		}
		filas = append(filas, FilaReporte{
			Valores: []any{i + 1, p.Concepto, p.Clasificacion, montoFloat(p.Total), pct, pctAcum},
		})
	}
	return HojaReporte{Nombre: "Concentración", Meta: meta, Cols: cols, Filas: filas}
}

// hojaSalud: qué mes entró al promedio y cuál no.
func hojaSalud(meta MetaReporte, res AnalisisPartidas) HojaReporte {
	meta = conPresentacion(meta, "Qué meses entraron al promedio y cuáles no")
	cols := []ColumnaReporte{
		{Titulo: "Mes", Ancho: 18, Tipo: "texto"},
		{Titulo: "Movimientos", Ancho: 13, Tipo: "entero"},
		{Titulo: "% clasificado", Ancho: 13, Tipo: "texto"},
		{Titulo: "¿Entró al promedio?", Ancho: 20, Tipo: "texto"},
	}
	filas := make([]FilaReporte, 0, len(res.Meses))
	for _, m := range res.Meses {
		estado := "Sí"
		if !m.Comparable {
			estado = "No · muestra menos gasto del que tuvo"
		}
		filas = append(filas, FilaReporte{
			Valores: []any{etiquetaPeriodo(m.Periodo), m.Movs, m.PctClasificado + " %", estado},
		})
	}
	return HojaReporte{Nombre: "Salud", Meta: meta, Cols: cols, Filas: filas, SinTotales: true}
}

// montoFloat convierte el monto-texto del dominio al float que espera excelize.
//
// Es la ÚNICA frontera donde el dinero deja de ser decimal, y es inevitable: la celda de un
// .xlsx es un float64. Pasa acá, al escribir, y no antes: ningún cálculo usa este valor.
func montoFloat(s string) float64 {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return 0
	}
	f, _ := d.Float64()
	return f
}
