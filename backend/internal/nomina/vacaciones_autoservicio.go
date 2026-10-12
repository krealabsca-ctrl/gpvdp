package nomina

// Autoservicio de vacaciones (mig 0092): el colaborador pide, la jefatura aprueba, RRHH confirma.
//
// Los tres pasos existen porque los tres responden preguntas distintas. El colaborador sabe cuándo
// quiere faltar; la jefatura sabe si el equipo aguanta esa semana; RRHH sabe si de verdad se tomó
// esos días. Juntarlos en uno solo obligaría a que alguien decida sobre algo que no le consta.

import "errors"

var (
	// ErrSinEmpleadoLigado: el usuario no está ligado a ninguna ficha de empleado de esta empresa.
	//
	// NO es un fallo del sistema y por eso tiene su propio centinela: es alguien que entra al ERP
	// y no está en planilla, o una ficha a la que todavía no le asignaron su usuario. La pantalla
	// tiene que poder decirle qué pedirle a RRHH en vez de mostrar un error técnico.
	ErrSinEmpleadoLigado = errors.New("nomina: tu usuario no está ligado a ninguna ficha de empleado; pedíselo a Recursos Humanos")
	// ErrSinDepartamentosACargo: tiene el permiso de aprobar, pero no es jefatura de ningún
	// departamento. El permiso abre la puerta; la jefatura decide de quién.
	ErrSinDepartamentosACargo = errors.New("nomina: no estás asignado como jefatura de ningún departamento")
	// ErrTransicionVacacionInvalida: la vacación no está en el estado que esa acción requiere, o
	// es de un departamento que no tenés a cargo.
	ErrTransicionVacacionInvalida = errors.New("nomina: esa vacación ya cambió de estado o no es de tu equipo")
	// ErrJefaturaNoEncontrada: no existe esa asignación de jefatura.
	ErrJefaturaNoEncontrada = errors.New("nomina: no se encontró esa jefatura")
	// ErrMotivoRechazoRequerido: rechazar sin decir por qué deja al colaborador sin saber si
	// puede volver a pedir las mismas fechas.
	ErrMotivoRechazoRequerido = errors.New("nomina: decí por qué se rechaza la solicitud")
	// ErrNoEsTuVacacion: se intentó cancelar una solicitud de otra persona.
	ErrNoEsTuVacacion = errors.New("nomina: esa solicitud no es tuya")
	// ErrRolJefaturaInvalido: el rol de una jefatura solo puede ser TITULAR o SUPLENTE.
	ErrRolJefaturaInvalido = errors.New("nomina: el rol de la jefatura tiene que ser «TITULAR» o «SUPLENTE»")
	// ErrDepartamentoNoEncontrado: ese departamento no existe en esta empresa. Es su propio
	// centinela y no un 500 porque también cubre el intento de asignar una jefatura sobre un
	// departamento ajeno, que tiene que verse como «no existe», no como un fallo del servidor.
	ErrDepartamentoNoEncontrado = errors.New("nomina: no se encontró ese departamento")
)

// Roles de una jefatura. Los dos aprueban igual: el suplente existe para que una semana de
// ausencia del titular no congele las vacaciones de todo un departamento.
const (
	JefaturaTitular  = "TITULAR"
	JefaturaSuplente = "SUPLENTE"
)

// RolJefaturaValido acota lo que puede llegar del cliente. Un rol desconocido NO se acepta como
// «el de siempre»: quién aprueba días libres no se decide por un default.
func RolJefaturaValido(r string) bool {
	return r == JefaturaTitular || r == JefaturaSuplente
}

// SolicitudVacacion es una vacación vista desde el autoservicio.
//
// LO QUE NO TRAE es la parte importante: no hay salario, ni deducciones, ni identificación, ni
// fecha de ingreso. Una jefatura aprobando días libres no necesita nada de eso, y lo que no viaja
// no se puede filtrar por un descuido de la pantalla.
type SolicitudVacacion struct {
	ID             string `json:"id"`
	EmpleadoID     string `json:"empleado_id"`
	EmpleadoNombre string `json:"empleado_nombre"`
	Departamento   string `json:"departamento"`
	FechaInicio    string `json:"fecha_inicio"`
	// FechaFin se DERIVA de inicio + días. No se guarda para que no pueda contradecir a los días,
	// que son los que cuentan contra el saldo.
	FechaFin      string `json:"fecha_fin"`
	Dias          string `json:"dias"`
	Observaciones string `json:"observaciones"`
	Estado        string `json:"estado"`
	Anulada       bool   `json:"anulada"`
	CreadoEn      string `json:"creado_en"`
	SolicitadaPor string `json:"solicitada_por"`
	AprobadaPor   string `json:"aprobada_por"`
	AprobadaEn    string `json:"aprobada_en"`
	MotivoRechazo string `json:"motivo_rechazo"`
}

// Jefatura es quién tiene a cargo un departamento para efectos de vacaciones.
type Jefatura struct {
	DepartamentoID string `json:"departamento_id"`
	Departamento   string `json:"departamento"`
	UsuarioID      string `json:"usuario_id"`
	UsuarioNombre  string `json:"usuario_nombre"`
	UsuarioEmail   string `json:"usuario_email"`
	Rol            string `json:"rol"`
}

// MiVacaciones es todo lo que ve el colaborador: su saldo y sus solicitudes. Nada más.
type MiVacaciones struct {
	Saldo       SaldoVacaciones     `json:"saldo"`
	Solicitudes []SolicitudVacacion `json:"solicitudes"`
}

// VacacionesEquipo es lo que ve la jefatura de su departamento.
type VacacionesEquipo struct {
	// Pendientes son las que esperan su decisión: con eso entra a la pantalla.
	Pendientes []SolicitudVacacion `json:"pendientes"`
	// Calendario es todo lo del equipo, para decidir si puede dar más días esa semana.
	Calendario []SolicitudVacacion `json:"calendario"`
	// Saldos del equipo: días disponibles, sin un solo dato de dinero.
	Saldos []SaldoVacaciones `json:"saldos"`
}
