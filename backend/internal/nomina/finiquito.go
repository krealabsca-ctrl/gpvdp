package nomina

// Finiquito (liquidación de cese) conforme al Código de Trabajo — Etapa 3, pantalla
// "Liquidación / Prestaciones" de la maqueta aprobada. El cálculo usa el SALARIO
// PROMEDIO REAL (incluye comisiones y bonos — base CCSS de las últimas liquidaciones
// pagadas), nunca una base reducida (guardarraíl).

import "errors"

// Motivos de cese y estados del finiquito (espejo de los CHECK de la migración 0034).
const (
	MotivoDespido     = "DESPIDO_RESPONSABILIDAD"
	MotivoRenuncia    = "RENUNCIA"
	MotivoFinContrato = "FIN_CONTRATO"
	MotivoMutuo       = "MUTUO_ACUERDO"

	FinBorrador = "BORRADOR"
	FinAprobado = "APROBADO"
	FinPagado   = "PAGADO"
	FinAnulado  = "ANULADO"
)

var (
	// ErrFiniquitoNoEncontrado indica que el finiquito no existe o no es de la empresa.
	ErrFiniquitoNoEncontrado = errors.New("nomina: finiquito no encontrado")
	// ErrFiniquitoDuplicado indica que el empleado ya tiene un finiquito vivo.
	ErrFiniquitoDuplicado = errors.New("nomina: el empleado ya tiene un finiquito vivo (anulá el anterior para rehacerlo)")
	// ErrFiniquitoNoEditable exige BORRADOR para modificar o recalcular.
	ErrFiniquitoNoEditable = errors.New("nomina: el finiquito ya no está en borrador; no se puede modificar")
	// ErrFiniquitoNoAprobable exige BORRADOR para aprobar.
	ErrFiniquitoNoAprobable = errors.New("nomina: solo un finiquito en borrador se puede aprobar")
	// ErrFiniquitoNoPagable exige APROBADO para pagar.
	ErrFiniquitoNoPagable = errors.New("nomina: solo un finiquito aprobado se puede marcar pagado")
	// ErrFiniquitoNoAnulable impide anular un finiquito pagado.
	ErrFiniquitoNoAnulable = errors.New("nomina: un finiquito pagado no se puede anular")
	// ErrMotivoInvalido exige uno de los 4 motivos de cese del Código de Trabajo.
	ErrMotivoInvalido = errors.New("nomina: motivo de cese inválido")
	// ErrFechaSalidaInvalida exige fecha de salida posterior al ingreso.
	ErrFechaSalidaInvalida = errors.New("nomina: la fecha de salida debe ser posterior a la fecha de ingreso")
	// ErrDiasVacacionesInvalidos exige un saldo de vacaciones razonable (0 a 365 días).
	ErrDiasVacacionesInvalidos = errors.New("nomina: los días de vacaciones pendientes deben estar entre 0 y 365")
	// ErrFiniquitoRespaldaCorrida impide anular un finiquito en el que ya se apoyó una
	// liquidación aprobada o pagada (esa liquidación omitió al empleado contando con él).
	ErrFiniquitoRespaldaCorrida = errors.New("nomina: una liquidación aprobada o pagada se apoyó en este finiquito; no se puede anular (anulá primero esa corrida)")
	// ErrFiniquitoModificado indica que otra sesión editó el borrador mientras se aprobaba.
	ErrFiniquitoModificado = errors.New("nomina: el finiquito cambió mientras se aprobaba; revisá los datos y volvé a aprobar")
)

// Finiquito es la liquidación de cese de un empleado.
type Finiquito struct {
	ID              string `json:"id"`
	EmpleadoID      string `json:"empleado_id"`
	EmpleadoNombre  string `json:"empleado_nombre"`
	Identificacion  string `json:"identificacion"`
	FechaIngreso    string `json:"fecha_ingreso"`
	Motivo          string `json:"motivo"`
	FechaSalida     string `json:"fecha_salida"`
	Estado          string `json:"estado"`
	DiasVacaciones  string `json:"dias_vacaciones"`
	SalarioPromedio string `json:"salario_promedio"`
	SalarioDiario   string `json:"salario_diario"`
	AniosServicio   int    `json:"anios_servicio"`
	Preaviso        string `json:"preaviso"`
	Cesantia        string `json:"cesantia"`
	Vacaciones      string `json:"vacaciones"`
	Aguinaldo       string `json:"aguinaldo"`
	BaseCCSS        string `json:"base_ccss"`
	CCSSObrero      string `json:"ccss_obrero"`
	Renta           string `json:"renta"`
	// Patronal es la carga patronal sobre las vacaciones pagadas al cese: no se le resta a
	// la persona, pero SÍ entra a la planilla CCSS del mes de la salida.
	Patronal   string         `json:"patronal"`
	Descuentos string         `json:"descuentos"`
	Total      string         `json:"total"`
	Detalle    []DetalleLinea `json:"detalle"`
	// Provisiones acumuladas del empleado (corridas pagadas) para el comparativo
	// "calculado vs provisionado" de la maqueta.
	Provisionado string `json:"provisionado"`
	// DiasManual distingue los días de vacaciones digitados por RRHH (se respetan al
	// aprobar) del saldo automático (se recalcula, por si disfrutó días entre medio).
	DiasManual bool   `json:"dias_vacaciones_manual"`
	CreadoEn   string `json:"creado_en"`
	AprobadoEn string `json:"aprobado_en"`
	PagadoEn   string `json:"pagado_en"`
}

// FiniquitoInput son los datos capturados para crear/recalcular un finiquito.
type FiniquitoInput struct {
	EmpleadoID     string
	Motivo         string
	FechaSalida    string
	DiasVacaciones string // vacío = usar el saldo pendiente calculado
	// DiasManual lo fija el servicio: true si los días vinieron del usuario.
	DiasManual bool
}

// EsMotivoValido valida el motivo de cese.
func EsMotivoValido(m string) bool {
	return m == MotivoDespido || m == MotivoRenuncia || m == MotivoFinContrato || m == MotivoMutuo
}
