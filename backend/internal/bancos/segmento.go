package bancos

import "errors"

// Errores de la consulta por segmento.
var (
	// ErrSinAlcance: al rol del usuario no le asignaron ninguna partida. No es un error de
	// programación ni un 403: es el estado de casi todos los roles, y la pantalla lo explica.
	ErrSinAlcance = errors.New("bancos: tu rol todavía no tiene partidas asignadas para consulta")
	// ErrFueraDeAlcance: el movimiento existe, pero no en las partidas de este rol. Se responde
	// 404 y no 403 a propósito, igual que la puerta de Contabilidad al catálogo: para este rol ese
	// movimiento no existe, y un 403 confirmaría que sí.
	ErrFueraDeAlcance = errors.New("bancos: ese movimiento no está entre las partidas que consultás")
	// ErrMotivoRequerido: avisar sin decir qué está mal no se puede corregir.
	ErrMotivoRequerido = errors.New("bancos: hace falta decir qué está mal segmentado")
	// ErrReporteYaAbierto: ese movimiento ya tiene un aviso sin resolver.
	ErrReporteYaAbierto = errors.New("bancos: ese movimiento ya está reportado y en revisión")
	// ErrReporteNoEncontrado: el aviso no existe, no es de esta empresa o ya lo resolvieron.
	ErrReporteNoEncontrado = errors.New("bancos: ese reporte no existe o ya fue resuelto")
	// ErrResolucionInvalida: cerrar un aviso es RECLASIFICADO o SIN_CAMBIO, nada más.
	ErrResolucionInvalida = errors.New("bancos: la resolución debe ser RECLASIFICADO o SIN_CAMBIO")
	// ErrRespuestaRequerida: cerrar SIN_CAMBIO sin explicar por qué garantiza el mismo aviso otra vez.
	ErrRespuestaRequerida = errors.New("bancos: para cerrar sin cambio hay que explicarle al equipo por qué la partida está bien")
	// ErrRolDeConsultaNoEncontrado: el rol no existe o no es de esta empresa.
	ErrRolDeConsultaNoEncontrado = errors.New("bancos: ese rol no existe en esta empresa")
	// ErrMontoInvalido: buscar o avisar de un faltante necesita un monto mayor que cero.
	ErrMontoInvalido = errors.New("bancos: el monto tiene que ser un número mayor que cero")
	// ErrFaltanteYaAvisado: esa persona ya tiene un aviso abierto por esa fecha y ese monto.
	ErrFaltanteYaAvisado = errors.New("bancos: ya avisaste de ese monto en esa fecha y sigue en revisión")
)

// Consulta de bancos POR SEGMENTO — los tipos.
//
// El equipo de una partida (Asociaciones, Depósitos, Emergencias) no entra al módulo Bancos:
// consulta los créditos de SU partida y avisa cuando alguno quedó mal segmentado. Corregir sigue
// siendo de quien clasifica.
//
// Dos capas, y la distinción es la que hace que sea seguro:
//   · el permiso `bancos.ver_mi_segmento` dice QUÉ PANTALLA abre;
//   · el alcance `rol_clasificacion_consulta` dice CUÁLES FILAS ve (mig 0077).

// RolDeConsulta es un rol que puede consultar por segmento: los que ofrece el catálogo en la
// columna «quién la consulta». Se listan los que TIENEN el permiso, no todos los roles de la
// empresa: ofrecer un rol que no puede abrir la pantalla es ofrecer un acceso que no existe.
type RolDeConsulta struct {
	ID       string `json:"id"`
	Codigo   string `json:"codigo"`
	Nombre   string `json:"nombre"`
	Usuarios int    `json:"usuarios"`
}

// AsignacionConsulta es «esta partida la consulta este rol»: una fila del alcance, ya resuelta a
// nombres para que la pantalla no tenga que cruzar nada.
type AsignacionConsulta struct {
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	ConceptoID      string `json:"concepto_id"`
	Concepto        string `json:"concepto"`
	RolID           string `json:"rol_id"`
	RolNombre       string `json:"rol_nombre"`
}

// AlcanceConsulta es lo que necesita la columna del catálogo en UNA sola llamada: los roles que se
// pueden elegir y lo que ya está asignado.
type AlcanceConsulta struct {
	Roles        []RolDeConsulta      `json:"roles"`
	Asignaciones []AsignacionConsulta `json:"asignaciones"`
}

// MiSegmento es lo que ve el equipo: sus movimientos y de qué partidas son.
//
// `Partidas` no es decorativo: contesta «¿esto es todo lo que me toca?» sin que el equipo tenga que
// adivinar por qué no aparece algo. Vacío = a su rol no le asignaron ninguna partida, que es un
// estado legítimo y distinto de «no hay movimientos este mes».
type MiSegmento struct {
	Partidas    []PartidaDelSegmento `json:"partidas"`
	Movimientos ListaMovimientos     `json:"movimientos"`
	// Cuentas donde SU segmento recibe plata, para el filtro de la pantalla.
	//
	// Sale de sus propios movimientos y no del catálogo de cuentas: este rol no tiene `bancos.ver`,
	// así que un desplegable alimentado del catálogo le daría 403 exactamente a quien lo usa. Es la
	// misma coartada cruzada que ya nos costó una vez con las sedes en Inventario.
	Cuentas []CuentaDelSegmento `json:"cuentas"`
	// CargadoHasta es la fecha del último movimiento importado DE LA EMPRESA, no de su segmento.
	//
	// Es el dato que separa «todavía no entró» de «entró y no es mío»: si el equipo espera un
	// depósito del 3 y el banco está cargado hasta el 5, el problema no es que falte cargar. Usar la
	// última fecha de SU segmento sería engañoso —diría «cargado hasta el 3» solo porque su partida
	// no tuvo movimiento el 4 y el 5— y es justo el caso en que la pantalla tiene que ser exacta.
	// No revela ningún monto ni movimiento: es un hecho de la operación.
	CargadoHasta string `json:"cargado_hasta"`
}

// CuentaDelSegmento es una cuenta donde el segmento del rol recibe movimientos.
type CuentaDelSegmento struct {
	ID     string `json:"id"`
	Banco  string `json:"banco"`
	Cuenta string `json:"cuenta"`
}

// PartidaDelSegmento es una partida que el rol consulta, con lo que lleva en el período pedido.
type PartidaDelSegmento struct {
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Concepto        string `json:"concepto"`
}

// ReporteSegmentacion es el aviso del equipo: «este movimiento no es de mi partida».
//
// El estado se DERIVA de `ResueltoEn`: nulo = pendiente. No hay columna «estado» que mantener
// sincronizada — es el defecto que ya nos costó seis veces en este sistema.
type ReporteSegmentacion struct {
	ID     string `json:"id"`
	Motivo string `json:"motivo"`
	// Quién avisó y cuándo.
	Usuario  string `json:"usuario"`
	CreadoEn string `json:"creado_en"`
	// El movimiento, con la partida que tiene HOY (que es lo que hay que juzgar).
	MovimientoID  string `json:"movimiento_id"`
	Fecha         string `json:"fecha"`
	Descripcion   string `json:"descripcion"`
	MontoCRC      string `json:"monto_crc"`
	Banco         string `json:"banco"`
	Cuenta        string `json:"cuenta"`
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
	// Resolución (vacío mientras está pendiente).
	Resolucion  string `json:"resolucion"`
	Respuesta   string `json:"respuesta"`
	ResueltoPor string `json:"resuelto_por"`
	ResueltoEn  string `json:"resuelto_en"`
	Pendiente   bool   `json:"pendiente"`
	// Aviso de FALTANTE (mig 0078): el equipo esperaba un movimiento y no lo vio. `MovimientoID`
	// puede venir vacío —no se pudo enganchar— y entonces esto es todo lo que hay para buscarlo.
	EsFaltante    bool   `json:"es_faltante"`
	FechaEsperada string `json:"fecha_esperada"`
	MontoEsperado string `json:"monto_esperado"`
	Referencia    string `json:"referencia"`
}

// ── Buscar un movimiento que no aparece (mig 0078) ──────────────────────────
//
// El equipo espera un depósito y no lo ve. Puede ser por cuatro razones que no puede distinguir: no
// se depositó, no se importó el banco de ese día, entró y quedó sin clasificar, o entró en otra
// partida. Esta consulta separa las dos últimas de las dos primeras.
//
// DIVULGACIÓN MÍNIMA, y es lo que la hace aceptable (decisión del usuario, 2026-09-07):
//
//	· exige fecha Y monto EXACTOS — no hay rangos ni aproximados, así que no se puede tantear;
//	· solo mira créditos;
//	· si el movimiento NO es de su partida, la respuesta es el veredicto y nada más: ni
//	  descripción, ni cuenta, ni banco, ni partida, ni id. Solo «existe uno así, no es tuyo»;
//	· si SÍ es de su partida se devuelve completo (ya tenía derecho a verlo: lo tapaba un filtro);
//	· toda consulta queda en auditoría con la fecha, el monto y el veredicto.
const (
	// FaltanteEnMiPartida: existe y es suyo. Lo estaba tapando un filtro o el mes activo.
	FaltanteEnMiPartida = "EN_MI_PARTIDA"
	// FaltanteFueraDeMiPartida: existe en la empresa pero está en otra partida (o sin clasificar).
	// Es el caso que hay que corregir, y por eso el aviso sirve.
	FaltanteFueraDeMiPartida = "FUERA_DE_MI_PARTIDA"
	// FaltanteNoExiste: no hay ningún crédito de esa fecha y ese monto en la empresa.
	FaltanteNoExiste = "NO_EXISTE"
)

// ResultadoFaltante es la respuesta de la búsqueda: un veredicto y, solo si el movimiento es suyo,
// el movimiento.
type ResultadoFaltante struct {
	Veredicto string `json:"veredicto"`
	// Movimientos viene con datos SOLO cuando el veredicto es EN_MI_PARTIDA.
	Movimientos []MovimientoRow `json:"movimientos"`
	// CargadoHasta acompaña al veredicto NO_EXISTE: sin esa fecha, «no hay ninguno» no distingue
	// «no entró» de «no lo han importado», que es la mitad de la pregunta.
	CargadoHasta string `json:"cargado_hasta"`
}

// Resoluciones posibles de un reporte.
const (
	// ResolucionReclasificado: tenía razón, la partida se corrigió.
	ResolucionReclasificado = "RECLASIFICADO"
	// ResolucionSinCambio: la partida estaba bien, y la respuesta explica por qué. Cerrar con
	// explicación es lo que evita que el mismo movimiento se reporte tres veces.
	ResolucionSinCambio = "SIN_CAMBIO"
)
