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
	// ErrVistaInvalida: «Mi partida» tiene UNA vista. El parámetro sigue existiendo justamente para
	// poder RECHAZAR lo que ya no se ofrece —en particular `vista=sin_clasificar`, que existió entre
	// el 22 y el 23 de setiembre de 2026—: si cayera en la principal «por las dudas», un cliente viejo
	// o una URL a mano leerían los créditos de la partida creyendo que miran otra lista.
	ErrVistaInvalida = errors.New("bancos: la vista tiene que ser «partida»")
)

// VistaPartida es la ÚNICA vista de «Mi partida en bancos»: los créditos clasificados en las
// partidas del alcance, y nada más.
//
// Hubo una segunda, «sin_clasificar», con los créditos todavía sin partida de las cuentas del
// segmento. El Director Financiero la mandó quitar el 23-set-2026 —«esto no debe ser visible por
// ningún motivo a los consultores»—, así que lo que NADIE clasificó todavía no se muestra por esta
// puerta: ni en lista, ni de a uno en «Falta un movimiento», ni se puede avisar sobre ello. Quien
// espera un depósito y no lo ve sigue teniendo el aviso de faltante, que es el camino que no divulga
// nada (ver los veredictos más abajo).
const VistaPartida = "partida"

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
	// CargadoHasta es la fecha de la CUENTA DEL SEGMENTO MÁS ATRASADA (decisión del Director
	// Financiero, 22-set-2026): por cada cuenta del segmento, el último día con movimientos
	// INCLUIDOS; y de esas fechas, la MÍNIMA. Vacío = el segmento no tiene ninguna cuenta.
	//
	// Antes era el último día importado de toda la empresa, y eso afirmaba de más: con Davivienda al
	// 11 y el BN al 9, decía «cargado hasta el 11» y el equipo que esperaba un depósito del 10 en el
	// BN leía «no entró» cuando lo que pasaba era que el BN no se había cargado. El dato que sirve es
	// el de la cuenta que más atrasa, NOMBRÁNDOLA (`CuentaMasAtrasada`).
	//
	// Por cuenta se mira TODO lo incluido de la cuenta, no solo lo de la partida: así no cae en la
	// trampa de «la última fecha de mi segmento», que diría «cargado hasta el 3» solo porque la
	// partida no tuvo movimiento el 4 y el 5. No revela ningún monto ni movimiento: es un hecho de la
	// operación. Es la MISMA fecha que acompaña a «no existe» en «Falta un movimiento»
	// (ResultadoFaltante): si fueran dos cálculos, el encabezado y el diálogo se contradirían.
	CargadoHasta string `json:"cargado_hasta"`
	// CuentaMasAtrasada es la cuenta cuya fecha es `CargadoHasta` (nil si no hay cuentas). En un
	// empate se nombra la primera por banco y cuenta.
	CuentaMasAtrasada *CuentaCargadaHasta `json:"cargado_hasta_cuenta"`
	// CargaPorCuenta es la fecha de cada cuenta del segmento, de la más atrasada a la más al día.
	//
	// NO se lista en la pantalla: el Director Financiero quitó el desplegable «hasta cuándo está
	// cargada cada una de tus cuentas» el 23-set-2026 («no es un tema de interés a los consultores»).
	// Viaja porque de acá sale la REDACCIÓN de la única línea que sí se muestra: con todas las cuentas
	// al mismo día se dice «tus 8 cuentas están cargadas hasta el…», y solo si alguna atrasa se nombra
	// a esa. Sin este dato habría que nombrar una cuenta como «la más atrasada» aunque no lo sea.
	CargaPorCuenta []CuentaCargadaHasta `json:"carga_por_cuenta"`
	// Vista es la vista que se devolvió (siempre VistaPartida), ya normalizada.
	Vista string `json:"vista"`
}

// CuentaCargadaHasta dice hasta qué día está importada UNA cuenta del segmento: el último día con
// movimientos incluidos de esa cuenta (de cualquier tipo y de cualquier partida).
type CuentaCargadaHasta struct {
	ID           string `json:"id"`
	Banco        string `json:"banco"`
	Cuenta       string `json:"cuenta"`
	CargadoHasta string `json:"cargado_hasta"`
}

// AvisoAbiertoDeFila es lo que la fila de «Mi partida» puede saber del aviso ABIERTO de su
// movimiento (el índice `uq_reporte_abierto_por_movimiento` deja uno solo por movimiento).
//
// Que HAY un aviso abierto se dice siempre, sea de quien sea: con ese índice avisar otra vez da 409,
// así que el botón no puede reaparecer. El MOTIVO, en cambio, solo viaja si el aviso es de quien
// pregunta y no es un faltante (Propio). El de otra persona es suyo; el de un faltante lo enganchó el
// servidor a un movimiento que el usuario nunca eligió. Decisión conservadora hasta que el Director
// Financiero decida si se abre (22-set-2026): el repositorio ni siquiera trae el texto ajeno.
type AvisoAbiertoDeFila struct {
	Propio bool
	// Motivo va vacío cuando no es Propio.
	Motivo string
}

// TextoAvisoAbiertoDeOtro es lo que dice la fila cuando el aviso abierto no es del usuario o es un
// faltante: que existe, y nada de lo que escribió otra persona.
const TextoAvisoAbiertoDeOtro = "Ya hay un aviso abierto sobre este movimiento."

// AvisoResuelto es el último aviso RESUELTO de un movimiento, para la fila de «Mi partida».
//
// Antes, al resolverse un aviso, el botón volvía a salir y la respuesta no se veía en ningún lado.
// Se muestra solo cuando el movimiento NO tiene un aviso abierto: el abierto es lo vigente.
//
// Solo los avisos de QUIEN PREGUNTA y que no son faltantes (22-set-2026): el motivo y la respuesta de
// otra persona son de ella, y un faltante resuelto vive en «Mis avisos», donde se lee como lo que se
// escribió y no pegado a un movimiento que el usuario no eligió.
type AvisoResuelto struct {
	Motivo     string `json:"motivo"`
	Resolucion string `json:"resolucion"`
	// Respuesta puede venir vacía: RECLASIFICADO no la exige (la corrección se ve en la partida).
	Respuesta  string `json:"respuesta"`
	ResueltoEn string `json:"resuelto_en"`
}

// MiAviso es un aviso que hizo ESTE usuario, tal como lo vio al avisar («Mis avisos»).
//
// Existe porque la resolución más común es reclasificar el movimiento a OTRA partida: sale del
// alcance, la fila desaparece de «Mi partida» y la respuesta llegaba justo adonde ya no se veía.
//
// Solo lleva lo que el usuario ya vio al avisar. A propósito NO lleva la partida actual del
// movimiento, ni su descripción, ni su id: después de reclasificarlo, ese movimiento ya no es de su
// segmento y lo único que tiene derecho a saber es la respuesta a lo que preguntó.
type MiAviso struct {
	ID string `json:"id"`
	// EsFaltante: aviso de «falta un movimiento» (mig 0078). Entonces Fecha y Monto son lo que el
	// usuario ESCRIBIÓ, y Documento/Banco/Cuenta van vacíos siempre —aunque el servidor haya
	// enganchado un movimiento—, porque ese movimiento el usuario nunca lo vio.
	EsFaltante bool   `json:"es_faltante"`
	Motivo     string `json:"motivo"`
	CreadoEn   string `json:"creado_en"`
	Fecha      string `json:"fecha"`
	Documento  string `json:"documento"`
	// Monto es el crédito en la moneda de la cuenta (como se veía en la fila), o el monto esperado
	// en un faltante. Moneda va vacía en un faltante: el usuario escribió un número, no una moneda.
	Monto      string `json:"monto"`
	Moneda     string `json:"moneda"`
	Banco      string `json:"banco"`
	Cuenta     string `json:"cuenta"`
	Referencia string `json:"referencia"`
	// Estado: AvisoEnRevision o AvisoResueltoEstado. Se DERIVA de `resuelto_en`.
	Estado     string `json:"estado"`
	Resolucion string `json:"resolucion"`
	Respuesta  string `json:"respuesta"`
	ResueltoEn string `json:"resuelto_en"`
}

// Estados de un aviso en «Mis avisos».
const (
	AvisoEnRevision     = "EN_REVISION"
	AvisoResueltoEstado = "RESUELTO"
)

// ListaMisAvisos es la página de «Mis avisos», con el total REAL (no el de la página).
type ListaMisAvisos struct {
	Items     []MiAviso `json:"items"`
	Total     int       `json:"total"`
	Abiertos  int       `json:"abiertos"`
	Resueltos int       `json:"resueltos"`
	Page      int       `json:"page"`
	PageSize  int       `json:"page_size"`
	// SinAlcance: el rol no tiene partidas asignadas. Un alcance vacío cierra también acá.
	SinAlcance bool `json:"sin_alcance"`
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
	// MovExcluido: el movimiento enganchado está marcado como no incluido (p. ej. el mismo estado
	// de cuenta importado en dos cuentas y corregido después). Se muestra, no se esconde: para
	// muchos de estos avisos «se excluyó por duplicado» ES la respuesta, y quien resuelve la cola
	// necesita verla. Falso cuando el aviso no tiene movimiento enganchado.
	MovExcluido bool `json:"mov_excluido"`
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
//	· si el movimiento NO es algo que el usuario ya ve, la respuesta es el veredicto y nada más: ni
//	  descripción, ni cuenta, ni banco, ni partida, ni id. Solo «existe uno así, no es tuyo»;
//	· si SÍ lo ve —es un crédito de su partida— se devuelve completo: ya tenía derecho a verlo;
//	· toda consulta queda en auditoría con la fecha, el monto y el veredicto.
//
// Precedencia cuando hay más de uno de esa fecha y ese monto: EN_MI_PARTIDA, después
// FUERA_DE_MI_PARTIDA, y si no hay ninguno NO_EXISTE.
//
// Entre el 22 y el 23-set-2026 hubo un cuarto veredicto, TODAVIA_SIN_PARTIDA, que devolvía completo
// el crédito que nadie había clasificado en una cuenta del segmento. Nació para mandar a la pestaña
// «Todavía sin partida»; al quitarse la pestaña se quitó también él, porque era la misma divulgación
// por otra puerta: ahora esos créditos caen en FUERA_DE_MI_PARTIDA, que existe y no muestra nada.
const (
	// FaltanteEnMiPartida: existe y es suyo. Lo estaba tapando un filtro o el mes activo.
	FaltanteEnMiPartida = "EN_MI_PARTIDA"
	// FaltanteFueraDeMiPartida: existe en la empresa pero el usuario no lo ve: está clasificado en otra
	// partida, o todavía sin partida. Es el caso que hay que corregir, y por eso el aviso de faltante
	// sirve: el equipo dice qué esperaba y quien clasifica en Bancos lo resuelve.
	FaltanteFueraDeMiPartida = "FUERA_DE_MI_PARTIDA"
	// FaltanteNoExiste: no hay ningún crédito de esa fecha y ese monto en la empresa.
	FaltanteNoExiste = "NO_EXISTE"
)

// ResultadoFaltante es la respuesta de la búsqueda: un veredicto y, solo si el usuario ya ve ese
// movimiento, el movimiento.
type ResultadoFaltante struct {
	Veredicto string `json:"veredicto"`
	// Movimientos viene con datos SOLO cuando el veredicto es EN_MI_PARTIDA: los créditos de su
	// partida, que el usuario ya ve en la lista. En los otros dos veredictos va vacío.
	Movimientos []MovimientoRow `json:"movimientos"`
	// CargadoHasta acompaña al veredicto NO_EXISTE: sin esa fecha, «no hay ninguno» no distingue
	// «no entró» de «no lo han importado», que es la mitad de la pregunta.
	//
	// Es la de la cuenta del segmento MÁS ATRASADA, la misma que el encabezado de «Mi partida» (ver
	// MiSegmento.CargadoHasta): es la que decide «conviene esperar antes de avisar».
	CargadoHasta string `json:"cargado_hasta"`
	// CargadoHastaCuenta nombra esa cuenta (nil fuera de NO_EXISTE o si el segmento no tiene cuentas).
	CargadoHastaCuenta *CuentaCargadaHasta `json:"cargado_hasta_cuenta"`
}

// Resoluciones posibles de un reporte.
const (
	// ResolucionReclasificado: tenía razón, la partida se corrigió.
	ResolucionReclasificado = "RECLASIFICADO"
	// ResolucionSinCambio: la partida estaba bien, y la respuesta explica por qué. Cerrar con
	// explicación es lo que evita que el mismo movimiento se reporte tres veces.
	ResolucionSinCambio = "SIN_CAMBIO"
)
