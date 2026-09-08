package bancos

// Consulta por segmento — la lógica.
//
// Acá vive la única regla que importa de verdad: el alcance lo pone el SERVIDOR desde el rol del
// usuario, y un alcance vacío cierra. El cliente puede afinar dentro de su alcance (un mes, una
// cuenta, una búsqueda) pero no puede ensancharlo.

import (
	"context"
	"strings"
	"time"

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

// MiSegmento devuelve los créditos de las partidas que el rol del usuario consulta.
//
// Dos cosas se fuerzan y no se piden:
//   - `Tipo = CREDITO`: la pantalla contesta «¿entró el dinero?». Los débitos son gasto de la
//     empresa y no son de este permiso (decisión del usuario, 2026-09-04).
//   - `Alcance`: sale del rol. Si el cliente manda clasificaciones, se intersecan con el alcance
//     porque las condiciones se suman con AND; nunca lo reemplazan.
func (s *Service) MiSegmento(ctx context.Context, empresaID, usuarioID string, f FiltrosMovimientos) (MiSegmento, error) {
	alcance, err := s.repo.AlcanceDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return MiSegmento{}, err
	}
	// Corte temprano: sin alcance no se consulta la tabla de movimientos. El repositorio también
	// cierra (condición imposible), pero esto lo deja explícito y le ahorra el viaje a la base.
	if len(alcance) == 0 {
		return MiSegmento{
			Partidas:    []PartidaDelSegmento{},
			Cuentas:     []CuentaDelSegmento{},
			Movimientos: ListaMovimientos{Items: []MovimientoRow{}, Page: 1, PageSize: 0},
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
	// Sin recortar por alcance a propósito (ver CargadoHasta en los tipos).
	cargadoHasta, err := s.repo.UltimaFechaCargada(ctx, empresaID)
	if err != nil {
		return MiSegmento{}, err
	}

	f.Alcance = alcance
	f.Tipo = "CREDITO"
	// El estado de clasificación no se filtra: todo lo que está en el alcance TIENE clasificación
	// por definición —el alcance ES una lista de clasificaciones—.
	lista, err := s.repo.ListarMovimientos(ctx, empresaID, f)
	if err != nil {
		return MiSegmento{}, err
	}

	// Los avisos ya abiertos de estos movimientos: la fila muestra el motivo escrito en vez de
	// ofrecer avisar otra vez. Es lo que evita tres reportes idénticos del mismo movimiento.
	ids := make([]string, 0, len(lista.Items))
	for _, m := range lista.Items {
		ids = append(ids, m.ID)
	}
	reportados, err := s.repo.ReportesDeMovimientos(ctx, empresaID, ids)
	if err != nil {
		return MiSegmento{}, err
	}
	for i := range lista.Items {
		if motivo, ok := reportados[lista.Items[i].ID]; ok {
			lista.Items[i].ReporteAbierto = motivo
		}
	}

	return MiSegmento{
		Partidas:     partidas,
		Cuentas:      cuentas,
		Movimientos:  lista,
		CargadoHasta: cargadoHasta,
	}, nil
}

// ReportarSegmentacion registra el aviso «este movimiento no es de mi partida».
//
// Se verifica que el movimiento esté EN EL ALCANCE del usuario antes de crear el aviso. Sin esa
// guarda, alguien podría reportar —y por lo tanto descubrir la existencia de— cualquier movimiento
// de la empresa mandando ids a mano: el permiso diría «solo mi partida» y el endpoint permitiría
// otra cosa.
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
// Los tres veredictos y por qué son tres y no dos:
//
//   - EN_MI_PARTIDA — existe y es suyo: lo tapaba un filtro o el mes activo. Se devuelve completo,
//     porque ya tenía derecho a verlo.
//   - FUERA_DE_MI_PARTIDA — existe en la empresa, en otra partida o sin clasificar. Se devuelve el
//     veredicto y NADA más. Es el único caso que hay que corregir, y por eso el aviso sirve.
//   - NO_EXISTE — no hay ninguno. Va con `CargadoHasta`, porque sin esa fecha «no hay ninguno» no
//     distingue «no entró» de «no lo han importado», que es la mitad de la pregunta.
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
		cargado, err := s.repo.UltimaFechaCargada(ctx, empresaID)
		if err != nil {
			return ResultadoFaltante{}, err
		}
		res.CargadoHasta = cargado
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
	if _, err := time.Parse("2006-01-02", fecha); err != nil {
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
