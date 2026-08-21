package nomina

// Incapacidades y vacaciones (Etapa 4). Pantalla «Incapacidades y Vacaciones» de la
// maqueta aprobada; política de subsidios confirmada por el Director Financiero: la
// empresa paga lo que la ley le obliga y el resto lo cubre la entidad (CCSS o INS).

import "errors"

// Entidades que cubren la incapacidad (espejo del CHECK de la migración 0037).
const (
	EntidadCCSS = "CCSS" // enfermedad y maternidad
	EntidadINS  = "INS"  // riesgo del trabajo
)

var (
	// ErrIncapacidadNoEncontrada indica que la incapacidad no existe o no es de la empresa.
	ErrIncapacidadNoEncontrada = errors.New("nomina: incapacidad no encontrada")
	// ErrVacacionNoEncontrada indica que el registro de vacaciones no existe.
	ErrVacacionNoEncontrada = errors.New("nomina: registro de vacaciones no encontrado")
	// ErrEntidadInvalida exige CCSS (enfermedad) o INS (riesgo del trabajo).
	ErrEntidadInvalida = errors.New("nomina: la entidad debe ser CCSS (enfermedad) o INS (riesgo del trabajo)")
	// ErrDiasInvalidos exige un plazo razonable de días.
	ErrDiasInvalidos = errors.New("nomina: los días deben ser mayores a cero y no pasar de 365")
	// ErrFechaInvalida exige una fecha en formato YYYY-MM-DD.
	ErrFechaInvalida = errors.New("nomina: fecha inválida (se espera YYYY-MM-DD)")
	// ErrAusenciaCorridaCerrada impide registrar o anular ausencias de un período cuya
	// corrida ya está aprobada o pagada: cambiarían números ya congelados.
	ErrAusenciaCorridaCerrada = errors.New("nomina: la corrida de ese mes ya está aprobada o pagada; anulala si necesitás corregir la ausencia")
	// ErrSinSaldoVacaciones impide registrar más días de disfrute que el saldo acumulado.
	ErrSinSaldoVacaciones = errors.New("nomina: el empleado no tiene tantos días de vacaciones acumulados")
)

// Incapacidad es una ausencia cubierta por la CCSS o el INS.
type Incapacidad struct {
	ID             string `json:"id"`
	EmpleadoID     string `json:"empleado_id"`
	EmpleadoNombre string `json:"empleado_nombre"`
	Entidad        string `json:"entidad"`
	FechaInicio    string `json:"fecha_inicio"`
	FechaFin       string `json:"fecha_fin"`
	Dias           int    `json:"dias"`
	Boleta         string `json:"boleta"`
	Observaciones  string `json:"observaciones"`
	Anulada        bool   `json:"anulada"`
	// Subsidio explica en palabras quién paga qué (se calcula, no se guarda).
	Subsidio string `json:"subsidio"`
	// DiasEmpresa son los días equivalentes que paga la empresa (3 días al 50% = 1,5).
	DiasEmpresa string `json:"dias_empresa"`
	CreadoEn    string `json:"creado_en"`
}

// IncapacidadInput son los datos para registrar una incapacidad.
type IncapacidadInput struct {
	EmpleadoID    string
	Entidad       string
	FechaInicio   string
	Dias          int
	Boleta        string
	Observaciones string
}

// Vacacion es un disfrute de vacaciones registrado.
type Vacacion struct {
	ID             string `json:"id"`
	EmpleadoID     string `json:"empleado_id"`
	EmpleadoNombre string `json:"empleado_nombre"`
	FechaInicio    string `json:"fecha_inicio"`
	Dias           string `json:"dias"`
	Observaciones  string `json:"observaciones"`
	Anulada        bool   `json:"anulada"`
	CreadoEn       string `json:"creado_en"`
}

// VacacionInput son los datos para registrar un disfrute de vacaciones.
type VacacionInput struct {
	EmpleadoID    string
	FechaInicio   string
	Dias          string // decimal (permite medios días)
	Observaciones string
}

// SaldoVacaciones es el saldo derivado de un empleado (nunca se almacena).
type SaldoVacaciones struct {
	EmpleadoID     string `json:"empleado_id"`
	Nombre         string `json:"nombre"`
	Identificacion string `json:"identificacion"`
	FechaIngreso   string `json:"fecha_ingreso"`
	MesesServicio  int    `json:"meses_servicio"`
	Acumulado      string `json:"acumulado"`
	Disfrutado     string `json:"disfrutado"`
	Pendiente      string `json:"pendiente"`
}
