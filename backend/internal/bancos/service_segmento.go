package bancos

// Consulta por segmento — la lógica.
//
// Acá vive la única regla que importa de verdad: el alcance lo pone el SERVIDOR desde el rol del
// usuario, y un alcance vacío cierra. El cliente puede afinar dentro de su alcance (un mes, una
// cuenta, una búsqueda) pero no puede ensancharlo.

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// auditarSegmento deja el evento del segmento en `auditoria_evento` (append-only).
func (s *Service) auditarSegmento(ctx context.Context, empresaID, entidad, entidadID, accion, usuarioID string, detalle map[string]any) {
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: entidad, EntidadID: &entidadID,
		Accion: accion, UsuarioID: &usuarioID, ValorNuevo: detalle,
	})
}

// normalizarVista traduce la vista pedida. Vacía es la única que hay; cualquier otra cosa es un
// error y no la principal «por las dudas» (ver ErrVistaInvalida).
//
// `sin_clasificar` se rechaza EXPLÍCITAMENTE con el resto: existió un día, y un cliente que quedó
// viejo tiene que enterarse de que ya no está en vez de recibir la partida con otro nombre.
func normalizarVista(vista string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(vista)) {
	case "", VistaPartida:
		return VistaPartida, nil
	default:
		return "", ErrVistaInvalida
	}
}

// MiSegmento devuelve los créditos que el equipo ve en «Mi partida».
//
// Dos cosas se fuerzan y no se piden:
//   - `Tipo = CREDITO`: la pantalla contesta «¿entró el dinero?». Los débitos son gasto de la
//     empresa y no son de este permiso (decisión del usuario, 2026-09-04).
//   - `Alcance`: sale del rol. Si el cliente manda clasificaciones, se intersecan con el alcance
//     porque las condiciones se suman con AND; nunca lo reemplazan.
func (s *Service) MiSegmento(ctx context.Context, empresaID, usuarioID, vista string, f FiltrosMovimientos) (MiSegmento, error) {
	vista, err := normalizarVista(vista)
	if err != nil {
		return MiSegmento{}, err
	}
	alcance, err := s.repo.AlcanceDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return MiSegmento{}, err
	}
	// Corte temprano: sin alcance no se consulta la tabla de movimientos. El repositorio también
	// cierra (condición imposible), pero esto lo deja explícito y le ahorra el viaje a la base.
	if len(alcance) == 0 {
		return MiSegmento{
			Vista:          vista,
			Partidas:       []PartidaDelSegmento{},
			Cuentas:        []CuentaDelSegmento{},
			CargaPorCuenta: []CuentaCargadaHasta{},
			Movimientos:    ListaMovimientos{Items: []MovimientoRow{}, Page: 1, PageSize: 0},
		}, ErrSinAlcance
	}

	partidas, err := s.repo.PartidasDelAlcance(ctx, empresaID, alcance)
	if err != nil {
		return MiSegmento{}, err
	}
	cuentas, err := s.repo.CuentasDelAlcance(ctx, empresaID, alcance)
	if err != nil {
		return MiSegmento{}, err
	}
	// La cuenta del segmento más atrasada (ver CargadoHasta en los tipos). La misma cuenta que usa
	// BuscarFaltante, para que el encabezado y el diálogo no se contradigan.
	carga, err := s.repo.CargaDeCuentasDelSegmento(ctx, empresaID, alcance)
	if err != nil {
		return MiSegmento{}, err
	}
	cargadoHasta, masAtrasada := cuentaMasAtrasada(carga)

	f.Alcance = alcance
	f.Tipo = "CREDITO"
	// El estado de clasificación no se filtra: todo lo que sale TIENE clasificación por definición,
	// porque el alcance ES una lista de clasificaciones.
	lista, err := s.repo.ListarMovimientos(ctx, empresaID, f)
	if err != nil {
		return MiSegmento{}, err
	}

	if err := s.anotarAvisos(ctx, empresaID, usuarioID, lista.Items); err != nil {
		return MiSegmento{}, err
	}

	return MiSegmento{
		Vista:             vista,
		Partidas:          partidas,
		Cuentas:           cuentas,
		Movimientos:       lista,
		CargadoHasta:      cargadoHasta,
		CuentaMasAtrasada: masAtrasada,
		CargaPorCuenta:    carga,
	}, nil
}

// anotarAvisos le pone a cada fila lo que el equipo puede saber de sus avisos.
//
//   - El ABIERTO, sea de quien sea: la fila muestra que está en revisión en vez de ofrecer avisar
//     otra vez (el índice de un aviso abierto por movimiento respondería 409). El motivo solo si es
//     de quien pregunta y no es un faltante; si no, TextoAvisoAbiertoDeOtro, y
//     ReporteAbiertoPropio dice cuál de los dos es.
//   - El RESUELTO, solo si no hay uno abierto, y solo el de quien pregunta: la respuesta y cuándo,
//     al lado del botón.
//
// Es la MISMA anotación en la lista y en «Falta un movimiento»: las filas que devuelve la búsqueda
// son las mismas que el usuario ve en su lista, y no pueden decir otra cosa.
func (s *Service) anotarAvisos(ctx context.Context, empresaID, usuarioID string, items []MovimientoRow) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, m := range items {
		ids = append(ids, m.ID)
	}
	reportados, err := s.repo.ReportesDeMovimientos(ctx, empresaID, usuarioID, ids)
	if err != nil {
		return err
	}
	resueltos, err := s.repo.AvisosResueltosDeMovimientos(ctx, empresaID, usuarioID, ids)
	if err != nil {
		return err
	}
	for i := range items {
		if ab, abierto := reportados[items[i].ID]; abierto {
			propio := ab.Propio && ab.Motivo != ""
			items[i].ReporteAbierto = TextoAvisoAbiertoDeOtro
			if propio {
				items[i].ReporteAbierto = ab.Motivo
			}
			items[i].ReporteAbiertoPropio = &propio
			continue
		}
		if a, ok := resueltos[items[i].ID]; ok {
			items[i].AvisoResuelto = &a
		}
	}
	return nil
}

// cuentaMasAtrasada es el «cargado hasta» de la pantalla: de las fechas de carga de cada cuenta del
// segmento, la MÍNIMA, con la cuenta que la tiene (decisión del Director Financiero, 22-set-2026).
//
// No confía en el orden del repositorio: lo recalcula. Las fechas son ISO (YYYY-MM-DD), así que
// compararlas como texto es compararlas en el calendario. En un empate gana la primera de la lista
// (el repositorio las ordena por banco y cuenta). Sin cuentas devuelve "" y nil: no hay nada que
// afirmar, y la pantalla no puede inventar una fecha.
func cuentaMasAtrasada(cargas []CuentaCargadaHasta) (string, *CuentaCargadaHasta) {
	var peor *CuentaCargadaHasta
	for i := range cargas {
		if cargas[i].CargadoHasta == "" {
			continue
		}
		if peor == nil || cargas[i].CargadoHasta < peor.CargadoHasta {
			peor = &cargas[i]
		}
	}
	if peor == nil {
		return "", nil
	}
	c := *peor
	return c.CargadoHasta, &c
}

// normalizarPaginaAvisos aplica la regla del paginado de «Mis avisos»: 50 por página por defecto y
// 200 como máximo (los tamaños que ofrece el paginador de la pantalla).
//
// La página fuera de rango se trata igual que el tamaño fuera de rango: vuelve al defecto (ver
// paginaMaxima). Sin ese tope, `page=200000000000000000` desbordaba `(page-1)*pageSize` a un OFFSET
// negativo y Postgres respondía con un 500.
func normalizarPaginaAvisos(page, pageSize int) (int, int) {
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	if page <= 0 || page > paginaMaxima {
		page = 1
	}
	return page, pageSize
}

// MisAvisos devuelve los avisos que hizo el usuario (abiertos y resueltos), aunque el movimiento ya
// no esté en su alcance.
//
// El alcance NO recorta la lista —ese es el punto: el aviso resuelto reclasificando sale del
// alcance justo cuando tiene respuesta—, pero SÍ abre la puerta: un rol sin partidas asignadas no
// consulta nada por segmento, y eso incluye esto. Un alcance vacío cierra, nunca abre.
func (s *Service) MisAvisos(ctx context.Context, empresaID, usuarioID string, page, pageSize int) (ListaMisAvisos, error) {
	page, pageSize = normalizarPaginaAvisos(page, pageSize)
	alcance, err := s.repo.AlcanceDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return ListaMisAvisos{}, err
	}
	if len(alcance) == 0 {
		return ListaMisAvisos{Items: []MiAviso{}, Page: page, PageSize: pageSize, SinAlcance: true}, ErrSinAlcance
	}
	return s.repo.MisAvisos(ctx, empresaID, usuarioID, page, pageSize)
}

// ReportarSegmentacion registra el aviso «este movimiento no es de mi partida».
//
// Se verifica que el movimiento esté EN EL ALCANCE del usuario antes de crear el aviso: que sea un
// crédito de una partida suya, o sea, una fila que ya está viendo (ver MovimientoEnAlcance). Sin esa
// guarda, alguien podría reportar —y por lo tanto descubrir la existencia de— cualquier movimiento
// de la empresa mandando ids a mano: el permiso diría «solo mi partida» y el endpoint permitiría
// otra cosa. Para lo que NO ve —incluido lo que nadie clasificó— el camino es «Falta un movimiento»,
// que avisa sin mostrar nada.
func (s *Service) ReportarSegmentacion(ctx context.Context, empresaID, usuarioID, movID, motivo string) error {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return ErrMotivoRequerido
	}
	alcance, err := s.repo.AlcanceDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return err
	}
	if len(alcance) == 0 {
		return ErrSinAlcance
	}
	enAlcance, err := s.repo.MovimientoEnAlcance(ctx, empresaID, movID, alcance)
	if err != nil {
		return err
	}
	if !enAlcance {
		return ErrFueraDeAlcance
	}
	if _, err := s.repo.CrearReporteSegmentacion(ctx, empresaID, movID, usuarioID, motivo); err != nil {
		return err
	}
	// Queda en auditoría: el aviso es lo que después explica por qué se reclasificó un movimiento
	// de hace tres meses.
	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", movID, "REPORTAR_SEGMENTACION", usuarioID,
		map[string]any{"motivo": motivo})
	return nil
}

// BuscarFaltante contesta si existe un crédito de esa fecha y ese monto exactos (mig 0078).
//
// Los tres veredictos, en este orden de precedencia (si hay coincidencias de más de un grupo, gana
// el primero):
//
//   - EN_MI_PARTIDA — existe y es suyo: lo tapaba un filtro o el mes activo. Se devuelve completo,
//     porque ya tenía derecho a verlo.
//   - FUERA_DE_MI_PARTIDA — existe en la empresa y el usuario no lo ve: en otra partida, o todavía
//     sin partida. Se devuelve el veredicto y NADA más. Es el caso que hay que corregir, y por eso
//     el aviso sirve.
//   - NO_EXISTE — no hay ninguno. Va con `CargadoHasta`, porque sin esa fecha «no hay ninguno» no
//     distingue «no entró» de «no lo han importado», que es la mitad de la pregunta.
//
// Lo que NADIE clasificó cae en FUERA_DE_MI_PARTIDA desde el 23-set-2026. Tuvo veredicto propio un
// día —devolvía el movimiento completo, porque el usuario lo veía en «Todavía sin partida»—; al
// quitarse esa pestaña se quitó también, porque sin ella el usuario no lo ve en ningún lado y
// mostrarlo acá sería la misma divulgación por otra puerta.
//
// Toda consulta queda en auditoría con la fecha, el monto y el veredicto: es una búsqueda que roza
// datos que el usuario no puede ver, así que tiene que dejar rastro incluso cuando no encuentra nada.
func (s *Service) BuscarFaltante(ctx context.Context, empresaID, usuarioID, fecha, monto string) (ResultadoFaltante, error) {
	f, m, err := validarFechaYMonto(fecha, monto)
	if err != nil {
		return ResultadoFaltante{}, err
	}
	alcance, err := s.repo.AlcanceDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return ResultadoFaltante{}, err
	}
	if len(alcance) == 0 {
		return ResultadoFaltante{}, ErrSinAlcance
	}

	mios, fuera, err := s.repo.BuscarPorFechaYMonto(ctx, empresaID, f, m, alcance)
	if err != nil {
		return ResultadoFaltante{}, err
	}

	res := ResultadoFaltante{Movimientos: []MovimientoRow{}}
	switch {
	case len(mios) > 0:
		res.Veredicto = FaltanteEnMiPartida
		res.Movimientos = mios
	case fuera > 0:
		res.Veredicto = FaltanteFueraDeMiPartida
	default:
		res.Veredicto = FaltanteNoExiste
		// La cuenta del segmento más atrasada, igual que el encabezado de «Mi partida»: es la fecha
		// con la que la pantalla dice «conviene esperar antes de avisar», y si acá fuera otra el
		// diálogo y el encabezado se contradirían.
		carga, err := s.repo.CargaDeCuentasDelSegmento(ctx, empresaID, alcance)
		if err != nil {
			return ResultadoFaltante{}, err
		}
		res.CargadoHasta, res.CargadoHastaCuenta = cuentaMasAtrasada(carga)
	}
	// Las filas devueltas dicen lo mismo que en la lista: si ya hay un aviso abierto sobre una de
	// ellas, el diálogo lo muestra en vez de dejar que el equipo vuelva a avisar de lo mismo.
	if err := s.anotarAvisos(ctx, empresaID, usuarioID, res.Movimientos); err != nil {
		return ResultadoFaltante{}, err
	}

	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", "", "BUSCAR_FALTANTE", usuarioID,
		map[string]any{"fecha": f, "monto": m.String(), "veredicto": res.Veredicto})
	return res, nil
}

// ReportarFaltante avisa de un movimiento que el equipo espera y no ve.
//
// El servidor intenta ENGANCHAR el movimiento (si hay exactamente uno de esa fecha y ese monto en
// la empresa) para que quien clasifica no lo tenga que buscar. El id nunca pasa por el cliente: el
// equipo no puede ver ese movimiento, así que tampoco tiene por qué recibir su identificador.
func (s *Service) ReportarFaltante(ctx context.Context, empresaID, usuarioID, fecha, monto, referencia, motivo string) error {
	f, m, err := validarFechaYMonto(fecha, monto)
	if err != nil {
		return err
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return ErrMotivoRequerido
	}
	alcance, err := s.repo.AlcanceDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return err
	}
	if len(alcance) == 0 {
		return ErrSinAlcance
	}

	movID, err := s.repo.EngancharFaltante(ctx, empresaID, f, m)
	if err != nil {
		return err
	}
	if _, err := s.repo.CrearReporteFaltante(ctx, empresaID, usuarioID, f, m,
		strings.TrimSpace(referencia), motivo, movID); err != nil {
		return err
	}
	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", movID, "REPORTAR_FALTANTE", usuarioID,
		map[string]any{"fecha": f, "monto": m.String(), "motivo": motivo, "enganchado": movID != ""})
	return nil
}

// validarFechaYMonto valida en el borde del servicio lo que después va a un `::date` y a un
// `numeric`: sin esto un dato mal escrito llega al cast de Postgres y vuelve como 500.
func validarFechaYMonto(fecha, monto string) (string, decimal.Decimal, error) {
	fecha = strings.TrimSpace(fecha)
	// El mismo validador que los filtros: con un time.Parse suelto, «0000-09-01» pasaba y Postgres lo
	// rechazaba con 500 al guardar el aviso de «falta un movimiento».
	if !fechaISOValida(fecha) {
		return "", decimal.Zero, ErrFechaInvalida
	}
	// Se acepta lo que la gente escribe de verdad: «4 950,50», «4.950,50» y «4950.50» son el mismo
	// monto. Si no, el equipo tiene que adivinar el formato para poder buscar su propio depósito.
	limpio := strings.NewReplacer(" ", "", " ", "", "₡", "").Replace(strings.TrimSpace(monto))
	if strings.Contains(limpio, ",") {
		limpio = strings.ReplaceAll(limpio, ".", "")
		limpio = strings.ReplaceAll(limpio, ",", ".")
	}
	m, err := decimal.NewFromString(limpio)
	if err != nil || m.LessThanOrEqual(decimal.Zero) {
		return "", decimal.Zero, ErrMontoInvalido
	}
	return fecha, m, nil
}

// AlcanceConsulta devuelve lo que la columna del catálogo necesita en una sola llamada.
func (s *Service) AlcanceConsulta(ctx context.Context, empresaID string) (AlcanceConsulta, error) {
	roles, err := s.repo.RolesDeConsulta(ctx, empresaID)
	if err != nil {
		return AlcanceConsulta{}, err
	}
	asignaciones, err := s.repo.AsignacionesConsulta(ctx, empresaID)
	if err != nil {
		return AlcanceConsulta{}, err
	}
	return AlcanceConsulta{Roles: roles, Asignaciones: asignaciones}, nil
}

// GuardarConsultaDePartida fija quiénes consultan una partida (reemplaza la lista).
//
// Una lista vacía es válida y significa «esta partida no la consulta ningún equipo». Es el estado
// normal de casi todas: de las 170 clasificaciones de Valle de Paz se asignan las tres o cuatro que
// tienen equipo detrás.
func (s *Service) GuardarConsultaDePartida(ctx context.Context, empresaID, clasificacionID string, rolIDs []string, usuarioID string) error {
	// Deduplicar y limpiar antes de tocar la base: la misma lista con un rol repetido no puede
	// terminar en un error de conflicto que se lea como «el rol no existe».
	visto := map[string]bool{}
	limpios := make([]string, 0, len(rolIDs))
	for _, id := range rolIDs {
		id = strings.TrimSpace(id)
		if id == "" || visto[id] {
			continue
		}
		visto[id] = true
		limpios = append(limpios, id)
	}
	if err := s.repo.GuardarConsultaDePartida(ctx, empresaID, clasificacionID, limpios); err != nil {
		return err
	}
	// Cambiar quién ve plata es una decisión de acceso: queda en auditoría con los roles que
	// quedaron, no solo cuántos — «se lo quité a Cobros» es la pregunta que se hace después.
	s.auditarSegmento(ctx, empresaID, "clasificacion", clasificacionID, "CAMBIAR_CONSULTA_SEGMENTO", usuarioID,
		map[string]any{"roles": limpios})
	return nil
}

// ReportesSegmentacion lista los avisos para quien clasifica.
func (s *Service) ReportesSegmentacion(ctx context.Context, empresaID string, soloPendientes bool) ([]ReporteSegmentacion, error) {
	return s.repo.ListarReportesSegmentacion(ctx, empresaID, soloPendientes)
}

// ResolverReporte cierra un aviso.
//
// SIN_CAMBIO exige respuesta: cerrar «estaba bien» sin explicar por qué deja al equipo con la misma
// duda, y el mismo movimiento vuelve a reportarse el mes siguiente. RECLASIFICADO no la exige
// porque la corrección se ve sola en la partida del movimiento.
func (s *Service) ResolverReporte(ctx context.Context, empresaID, reporteID, usuarioID, resolucion, respuesta string) error {
	resolucion = strings.TrimSpace(strings.ToUpper(resolucion))
	if resolucion != ResolucionReclasificado && resolucion != ResolucionSinCambio {
		return ErrResolucionInvalida
	}
	respuesta = strings.TrimSpace(respuesta)
	if resolucion == ResolucionSinCambio && respuesta == "" {
		return ErrRespuestaRequerida
	}
	if err := s.repo.ResolverReporteSegmentacion(ctx, empresaID, reporteID, usuarioID, resolucion, respuesta); err != nil {
		return err
	}
	s.auditarSegmento(ctx, empresaID, "movimiento_reporte_segmentacion", reporteID, "RESOLVER_REPORTE_SEGMENTACION",
		usuarioID, map[string]any{"resolucion": resolucion, "respuesta": respuesta})
	return nil
}
