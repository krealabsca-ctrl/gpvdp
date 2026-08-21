package bancos

// Servicio de la clasificación en bloque desde Excel: previsualizar, aplicar y armar la plantilla.
//
// Previsualizar y aplicar recorren EXACTAMENTE el mismo plan; lo único que cambia es si se escribe.
// Es el mismo contrato del diccionario del catálogo, y existe para que lo que el usuario aprueba en
// la pantalla sea lo que ocurre.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gpvdp/erp/internal/shared"
	"github.com/xuri/excelize/v2"
)

// ImportarClasificacionExcel lee el archivo y, si aplicar es true, escribe las clasificaciones.
//
//	cuentaPorDefecto: cuenta a usar cuando el archivo no trae columna de cuenta (o la trae vacía).
//	reclasificar:     false = solo se clasifica lo que está SIN clasificar. El default es false a
//	                  propósito: pisar en silencio miles de partidas ya decididas por subir un
//	                  archivo es exactamente lo que no debe poder pasar sin pedirlo.
func (s *Service) ImportarClasificacionExcel(ctx context.Context, empresaID string, archivo []byte, cuentaPorDefecto string, reclasificar, aplicar bool, usuarioID string) (PlanClasifExcel, error) {
	g, hoja, hojas, err := gridDeArchivoConHojas(archivo, hojaMovimientos)
	if err != nil {
		return PlanClasifExcel{}, err
	}
	filas, err := LeerClasifExcel(g)
	if err != nil {
		return PlanClasifExcel{}, err
	}
	plan, err := s.planClasifExcel(ctx, empresaID, filas, cuentaPorDefecto, reclasificar, aplicar, usuarioID)
	if err != nil {
		return plan, err
	}
	plan.Hoja, plan.Hojas = hoja, hojas
	plan.Aviso = avisoClasifExcel(plan)
	return plan, nil
}

// planClasifExcel resuelve cada fila contra el catálogo y los movimientos, y arma el plan.
// Separado del parseo para poder probar el criterio sin construir un archivo Excel.
func (s *Service) planClasifExcel(ctx context.Context, empresaID string, entrada []FilaClasifExcel, cuentaPorDefecto string, reclasificar, aplicar bool, usuarioID string) (PlanClasifExcel, error) {
	// Se trabaja sobre una COPIA: el plan escribe el estado resuelto en cada fila, y mutar la entrada
	// haría que previsualizar y después aplicar con las mismas filas arrastre el estado de la primera
	// pasada (la segunda vería filas ya resueltas y las saltaría).
	filas := make([]FilaClasifExcel, len(entrada))
	copy(filas, entrada)

	cuentas, err := s.repo.ListarCuentas(ctx, empresaID, true)
	if err != nil {
		return PlanClasifExcel{}, err
	}
	// Una cuenta se puede nombrar por su alias o por su IBAN. El alias hace falta porque seis de las
	// cuentas reales no tienen IBAN: el Banco Nacional no lo trae en el estado de cuenta.
	//
	// La base garantiza UNIQUE(empresa_id, alias), pero acá se compara NORMALIZADO (sin tildes ni
	// mayúsculas), y eso puede empatar dos alias que la base considera distintos («BAC Religiosa» y
	// «bac religiosa»). Cuando pasa, la clave se marca como colisión en vez de quedarse con la última:
	// escribir la partida en la cuenta equivocada es peor que no escribirla.
	porCuenta := map[string]string{}
	const cuentaAmbigua = "?"
	agregar := func(clave, id string) {
		if clave == "" {
			return
		}
		if ya, hay := porCuenta[clave]; hay && ya != id {
			porCuenta[clave] = cuentaAmbigua
			return
		}
		porCuenta[clave] = id
	}
	for _, c := range cuentas {
		agregar(norm(c.Alias), c.ID)
		agregar(norm(strings.NewReplacer(" ", "", "-", "").Replace(c.IBAN)), c.ID)
	}

	clasifs, err := s.repo.ListarClasificaciones(ctx, empresaID, false)
	if err != nil {
		return PlanClasifExcel{}, err
	}
	// Dos índices: uno por (concepto, clasificación) —el bueno— y otro solo por el nombre de la
	// clasificación, para poder resolver cuando el archivo no trae el concepto. El segundo guarda
	// CUÁNTAS calzan, porque el catálogo permite el mismo nombre bajo dos conceptos: medido hoy hay
	// 3 casos en Coopeprofa y 1 en Memorial Pets (ninguno todavía en Valle de Paz). Resolver solo por
	// nombre elegiría una de las dos partidas sin avisar, y el 25 % del dinero de una empresa puede
	// depender de eso.
	porPartida := map[string]ClasificacionItem{}
	porNombre := map[string][]ClasificacionItem{}
	for _, cl := range clasifs {
		porPartida[claveClasif(cl.Concepto, cl.Nombre)] = cl
		n := norm(cl.Nombre)
		porNombre[n] = append(porNombre[n], cl)
	}

	// ── Paso 1: resolver cuenta y partida de cada fila ──────────────────────────
	for i := range filas {
		f := &filas[i]
		if f.Estado != "" {
			continue // ya vino inválida del parseo
		}

		// Cuenta. Se prueba el nombre tal cual normalizado y también sin espacios ni guiones (así
		// entra un IBAN pegado del banco, «CR76 0104 …»).
		clave := norm(strings.NewReplacer(" ", "", "-", "").Replace(f.Cuenta))
		hallada := porCuenta[norm(f.Cuenta)]
		if hallada == "" {
			hallada = porCuenta[clave]
		}
		switch {
		case f.Cuenta == "" && cuentaPorDefecto != "":
			f.cuentaID = cuentaPorDefecto
		case hallada == cuentaAmbigua:
			f.Estado = ClasifExcelSinCuenta
			f.Detalle = fmt.Sprintf("hay más de una cuenta que se llama %q: renombralas para distinguirlas", f.Cuenta)
			continue
		case hallada != "":
			f.cuentaID = hallada
		case cuentaPorDefecto != "":
			// El archivo nombra una cuenta que no existe: NO se cae a la de por defecto. Escribir la
			// clasificación en la cuenta equivocada es peor que no escribirla.
			f.Estado = ClasifExcelSinCuenta
			f.Detalle = fmt.Sprintf("no hay una cuenta que se llame %q ni con ese IBAN", f.Cuenta)
			continue
		default:
			f.Estado = ClasifExcelSinCuenta
			f.Detalle = "la fila no dice de qué cuenta es y no se eligió una cuenta para todo el archivo"
			continue
		}

		// Partida
		if f.Concepto != "" {
			cl, ok := porPartida[claveClasif(f.Concepto, f.Clasificacion)]
			if !ok {
				f.Estado = ClasifExcelSinPartida
				f.Detalle = fmt.Sprintf("no existe %s › %s en el catálogo", f.Concepto, f.Clasificacion)
				continue
			}
			f.clasifID, f.conceptID = cl.ID, cl.ConceptoID
			continue
		}
		candidatas := porNombre[norm(f.Clasificacion)]
		switch len(candidatas) {
		case 0:
			f.Estado = ClasifExcelSinPartida
			f.Detalle = fmt.Sprintf("no existe una clasificación %q en el catálogo", f.Clasificacion)
		case 1:
			f.clasifID, f.conceptID = candidatas[0].ID, candidatas[0].ConceptoID
			f.Concepto = candidatas[0].Concepto
		default:
			nombres := make([]string, 0, len(candidatas))
			for _, c := range candidatas {
				nombres = append(nombres, c.Concepto)
			}
			f.Estado = ClasifExcelSinPartida
			f.Detalle = fmt.Sprintf("%q existe en %d conceptos (%s): agregá la columna Concepto para decir cuál",
				f.Clasificacion, len(candidatas), strings.Join(nombres, ", "))
		}
	}

	// ── Paso 2: buscar los movimientos de las filas que sobrevivieron ───────────
	var cuentasQ, fechasQ, debitosQ, creditosQ, docsQ []string
	pedidosPorClave := map[string][]int{} // clave → índices de fila
	for i := range filas {
		f := &filas[i]
		if f.Estado != "" {
			continue
		}
		k := claveMovimiento(f.cuentaID, f.fecha, f.debito, f.credito, f.Documento)
		if _, visto := pedidosPorClave[k]; !visto {
			cuentasQ = append(cuentasQ, f.cuentaID)
			fechasQ = append(fechasQ, f.Fecha)
			debitosQ = append(debitosQ, f.Debito)
			creditosQ = append(creditosQ, f.Credito)
			docsQ = append(docsQ, strings.TrimSpace(f.Documento))
		}
		pedidosPorClave[k] = append(pedidosPorClave[k], i)
	}

	calzados := map[string][]MovimientoCalzado{}
	if len(cuentasQ) > 0 {
		encontrados, err := s.repo.BuscarMovimientosPorTupla(ctx, empresaID, cuentasQ, fechasQ, debitosQ, creditosQ, docsQ)
		if err != nil {
			return PlanClasifExcel{}, err
		}
		for _, m := range encontrados {
			calzados[m.Clave] = append(calzados[m.Clave], m)
		}
	}

	// ── Paso 3: emparejar y decidir qué pasa con cada fila ──────────────────────
	asigs := make([]AsignacionClasif, 0, len(filas))
	for clave, idxs := range pedidosPorClave {
		movs := calzados[clave]
		if len(movs) == 0 {
			for _, i := range idxs {
				filas[i].Estado = ClasifExcelSinMovim
				filas[i].Detalle = "ese movimiento no está cargado: primero importá el estado de cuenta de ese mes"
			}
			continue
		}
		// Varios movimientos idénticos (mismos fecha, monto y documento) son los duplicados legítimos
		// que el importador conserva. Solo se puede emparejar sin adivinar cuando el archivo les da
		// la MISMA partida y hay tantas filas como movimientos; si no, nadie puede saber cuál es cuál
		// y elegir al azar dejaría plata en la partida equivocada.
		if len(movs) > 1 || len(idxs) > 1 {
			mismaPartida := true
			for _, i := range idxs {
				if filas[i].clasifID != filas[idxs[0]].clasifID {
					mismaPartida = false
					break
				}
			}
			if !mismaPartida || len(idxs) != len(movs) {
				for _, i := range idxs {
					filas[i].Estado = ClasifExcelAmbiguo
					filas[i].Detalle = fmt.Sprintf(
						"hay %d movimientos idénticos y el archivo trae %d fila(s) para ellos: no se puede saber cuál es cuál",
						len(movs), len(idxs))
				}
				continue
			}
		}
		for n, i := range idxs {
			f := &filas[i]
			m := movs[n]
			f.Descripcion = m.Descripcion
			if m.Concepto != "" || m.Clasificacion != "" {
				f.PartidaActual = strings.TrimSpace(m.Concepto + " › " + m.Clasificacion)
			}
			switch {
			case m.ClasifID == f.clasifID:
				f.Estado = ClasifExcelSinCambio
				f.Detalle = "ya tiene esa partida"
			case m.ClasifID == "":
				f.Estado = ClasifExcelClasifica
				asigs = append(asigs, AsignacionClasif{MovimientoID: m.ID, ConceptoID: f.conceptID, ClasificacionID: f.clasifID})
			case reclasificar:
				f.Estado = ClasifExcelReclasifica
				f.Detalle = "tenía " + f.PartidaActual
				asigs = append(asigs, AsignacionClasif{MovimientoID: m.ID, ConceptoID: f.conceptID, ClasificacionID: f.clasifID})
			default:
				f.Estado = ClasifExcelProtegido
				f.Detalle = "ya está clasificado como " + f.PartidaActual + ": marcá «reemplazar» si querés cambiarlo"
			}
		}
	}

	// ── Paso 4: contar, aplicar y armar la respuesta ───────────────────────────
	plan := PlanClasifExcel{Filas: len(filas), Aplicado: aplicar}
	for i := range filas {
		switch filas[i].Estado {
		case ClasifExcelClasifica:
			plan.Clasifica++
		case ClasifExcelReclasifica:
			plan.Reclasifica++
		case ClasifExcelSinCambio:
			plan.SinCambio++
		case ClasifExcelSinMovim:
			plan.SinMovimiento++
		case ClasifExcelSinPartida:
			plan.SinPartida++
		case ClasifExcelSinCuenta:
			plan.SinCuenta++
		case ClasifExcelFilaInvalida:
			plan.Invalidas++
		case ClasifExcelAmbiguo:
			plan.Ambiguas++
		case ClasifExcelProtegido:
			plan.Protegidas++
		case ClasifExcelSinLlenar:
			plan.SinLlenar++
		}
	}

	if aplicar && len(asigs) > 0 {
		n, err := s.repo.AplicarClasificacionesEnBloque(ctx, empresaID, asigs)
		if err != nil {
			return PlanClasifExcel{}, err
		}
		plan.Clasificados = n
		s.audit.Registrar(ctx, shared.Evento{
			EmpresaID: &empresaID, Entidad: "movimiento_bancario", Accion: "CLASIFICAR_DESDE_EXCEL",
			UsuarioID: &usuarioID,
			ValorNuevo: map[string]int{
				"filas": plan.Filas, "clasificados": n,
				"reclasificados": plan.Reclasifica, "sin_movimiento": plan.SinMovimiento,
				"sin_partida": plan.SinPartida, "ambiguas": plan.Ambiguas,
			},
		})
	}

	plan.Detalle, plan.DetalleTruncado = detalleClasifExcel(filas)
	plan.Aviso = avisoClasifExcel(plan)
	return plan, nil
}

// detalleClasifExcel elige qué filas viajan al navegador cuando el archivo es grande.
//
// Los PROBLEMAS van primero y completos hasta el tope: son las filas sobre las que hay que hacer
// algo. Mandar las primeras 400 filas por orden de aparición dejaría un archivo de 20.000 líneas
// mostrando 400 filas correctas y esconderia justo lo que falló.
func detalleClasifExcel(filas []FilaClasifExcel) ([]FilaClasifExcel, bool) {
	if len(filas) <= maxDetalleClasifExcel {
		return filas, false
	}
	prioridad := func(estado string) int {
		switch estado {
		case ClasifExcelFilaInvalida, ClasifExcelSinPartida, ClasifExcelSinCuenta, ClasifExcelAmbiguo:
			return 0
		case ClasifExcelSinMovim, ClasifExcelProtegido:
			return 1
		case ClasifExcelReclasifica:
			return 2
		case ClasifExcelSinLlenar:
			// Lo último de todo: son las filas que el usuario dejó en blanco a propósito. Ponerlas
			// antes copaba la tabla y expulsaba los cuatro problemas que sí había que ver.
			return 4
		default:
			return 3
		}
	}
	orden := make([]FilaClasifExcel, len(filas))
	copy(orden, filas)
	sort.SliceStable(orden, func(i, j int) bool {
		pi, pj := prioridad(orden[i].Estado), prioridad(orden[j].Estado)
		if pi != pj {
			return pi < pj
		}
		return orden[i].Linea < orden[j].Linea
	})
	return orden[:maxDetalleClasifExcel], true
}

// avisoClasifExcel resume en una frase qué hay que mirar antes de aplicar.
func avisoClasifExcel(p PlanClasifExcel) string {
	var partes []string
	if p.SinMovimiento > 0 {
		partes = append(partes, fmt.Sprintf("%d fila(s) apuntan a movimientos que no están cargados (hay que importar esos meses antes)", p.SinMovimiento))
	}
	if p.SinPartida > 0 {
		partes = append(partes, fmt.Sprintf("%d fila(s) nombran una partida que no existe en el catálogo", p.SinPartida))
	}
	if p.SinCuenta > 0 {
		partes = append(partes, fmt.Sprintf("%d fila(s) no se pudieron atribuir a una cuenta", p.SinCuenta))
	}
	if p.Ambiguas > 0 {
		partes = append(partes, fmt.Sprintf("%d fila(s) calzan con movimientos idénticos y no se puede saber cuál es cuál", p.Ambiguas))
	}
	if p.Protegidas > 0 {
		partes = append(partes, fmt.Sprintf("%d movimiento(s) ya tienen otra partida y no se tocan", p.Protegidas))
	}
	if p.Invalidas > 0 {
		partes = append(partes, fmt.Sprintf("%d fila(s) no se pudieron leer", p.Invalidas))
	}
	// Se lee UNA hoja. Si el libro traía más, hay que decirlo: alguien puede haber puesto una hoja por
	// cuenta y el resumen se vería igual de exitoso habiendo leído solo la primera.
	if len(p.Hojas) > 1 {
		otras := make([]string, 0, len(p.Hojas)-1)
		for _, h := range p.Hojas {
			if h != p.Hoja {
				otras = append(otras, h)
			}
		}
		partes = append(partes, fmt.Sprintf(
			"el libro tiene %d hojas y solo se leyó «%s»: %s quedó sin leer (subilas de a una)",
			len(p.Hojas), p.Hoja, strings.Join(otras, ", ")))
	}
	if len(partes) == 0 {
		return ""
	}
	return strings.Join(partes, " · ") + "."
}

// PlantillaClasificacion arma el .xlsx que se baja, se llena en Excel y se vuelve a subir.
//
// La plantilla ES el formato que el importador vuelve a leer: mismas columnas y mismos encabezados.
// Con eso el usuario no tiene que adivinar cómo se llama nada, y la ida y vuelta no puede desalinear
// una columna. Las dos columnas de partida van vacías cuando el movimiento no está clasificado: son
// las que hay que llenar.
func (s *Service) PlantillaClasificacion(ctx context.Context, empresaID, desde, hasta string, soloSinClasificar bool) ([]byte, int, error) {
	movs, err := s.repo.MovimientosPlantillaClasif(ctx, empresaID, desde, hasta, soloSinClasificar, maxFilasClasifExcel)
	if err != nil {
		return nil, 0, err
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	idx, err := f.NewSheet(hojaMovimientos)
	if err != nil {
		return nil, 0, err
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	// El orden de las columnas es el mismo del reporte «Listado corrido» para que los dos archivos se
	// parezcan; de todos modos el importador las resuelve por encabezado, no por posición.
	encabezados := []string{"Fecha", "Banco", "Cuenta", "Documento", "Descripción", "Mon.",
		"Débito", "Crédito", "Concepto", "Clasificación"}
	for i, h := range encabezados {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStr(hojaMovimientos, celda, h)
	}
	if estilo, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err == nil {
		_ = f.SetRowStyle(hojaMovimientos, 1, 1, estilo)
	}

	for i, m := range movs {
		fila := i + 2
		// Los montos y la fecha van como TEXTO a propósito: es lo que el lector espera y evita que
		// Excel reinterprete «03/07» como una fecha del año en curso al abrir el archivo.
		vals := []string{m.Fecha, m.Banco, m.Cuenta, m.Documento, m.Descripcion, m.Moneda,
			m.Debito, m.Credito, m.Concepto, m.Clasificacion}
		for j, v := range vals {
			celda, _ := excelize.CoordinatesToCellName(j+1, fila)
			_ = f.SetCellStr(hojaMovimientos, celda, v)
		}
	}

	_ = f.SetColWidth(hojaMovimientos, "A", "A", 12)
	_ = f.SetColWidth(hojaMovimientos, "B", "C", 22)
	_ = f.SetColWidth(hojaMovimientos, "D", "D", 18)
	_ = f.SetColWidth(hojaMovimientos, "E", "E", 56)
	_ = f.SetColWidth(hojaMovimientos, "F", "F", 7)
	_ = f.SetColWidth(hojaMovimientos, "G", "H", 15)
	_ = f.SetColWidth(hojaMovimientos, "I", "J", 26)
	_ = f.SetPanes(hojaMovimientos, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), len(movs), nil
}
