// Package nomina implementa RRHH / Nómina (Fase 3, Etapa 1: fundamentos).
//
// GUARDARRAÍL LEGAL (CLAUDE.md): la base contributiva CCSS se calcula conforme a la ley —
// comisiones y bonificaciones habituales SON salario. Los conceptos de sistema llevan sus
// banderas de afectación BLOQUEADAS (el servicio rechaza editarlos), y un ingreso NO afecto
// a CCSS exige base legal. PROHIBIDO construir funciones o reportes de "ahorro por no
// reportar" (subdeclaración).
package nomina

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	// ErrEmpleadoNoEncontrado indica que el empleado no existe o no es de la empresa.
	ErrEmpleadoNoEncontrado = errors.New("nomina: empleado no encontrado")
	// ErrEmpleadoDuplicado indica otra ficha con la misma identificación en la empresa.
	ErrEmpleadoDuplicado = errors.New("nomina: ya existe un empleado con esa identificación")
	// ErrSalarioInvalido exige salario base positivo (sin él no hay nómina calculable).
	ErrSalarioInvalido = errors.New("nomina: el salario base debe ser mayor a cero")
	// ErrParametrosNoEncontrados indica que la empresa no tiene parámetros guardados para el año.
	ErrParametrosNoEncontrados = errors.New("nomina: parámetros no encontrados para ese año")
	// ErrCargaInvalida indica una carga social mal formada (código/tipo/porcentaje).
	ErrCargaInvalida = errors.New("nomina: carga social inválida (código, nombre, tipo OBRERO|PATRONAL y % entre 0 y 100)")
	// ErrCargasIncompletas exige al menos una carga obrera y una patronal.
	ErrCargasIncompletas = errors.New("nomina: los parámetros requieren al menos una carga obrera y una patronal")
	// ErrTramosInvalidos exige tramos de renta ascendentes con un único tramo final abierto.
	ErrTramosInvalidos = errors.New("nomina: tramos de renta inválidos (límites ascendentes y solo el último abierto)")
	// ErrConceptoNoEncontrado indica que el concepto no existe o no es de la empresa.
	ErrConceptoNoEncontrado = errors.New("nomina: concepto no encontrado")
	// ErrConceptoDuplicado indica otro concepto con ese nombre en la empresa.
	ErrConceptoDuplicado = errors.New("nomina: ya existe un concepto con ese nombre")
	// ErrConceptoDeSistema es el guardarraíl: los conceptos de sistema (salario, extras,
	// comisiones, bonos habituales…) no se editan ni desactivan — son salario por ley.
	ErrConceptoDeSistema = errors.New("nomina: los conceptos de sistema están bloqueados por ley y no pueden editarse ni desactivarse")
	// ErrBaseLegalRequerida es el guardarraíl: un INGRESO no afecto a CCSS exige citar su base legal.
	ErrBaseLegalRequerida = errors.New("nomina: un ingreso no afecto a CCSS requiere indicar su base legal")
	// ErrDeduccionNoEncontrada indica que la deducción no existe o no es del empleado.
	ErrDeduccionNoEncontrada = errors.New("nomina: deducción no encontrada")
	// ErrDeduccionInvalida exige cuota positiva y, si hay saldo total, que sea positivo.
	ErrDeduccionInvalida = errors.New("nomina: deducción inválida (cuota > 0; saldo total, si se indica, > 0)")
	// ErrConceptoNoEsDeduccion exige que la deducción recurrente cuelgue de un concepto DEDUCCION activo.
	ErrConceptoNoEsDeduccion = errors.New("nomina: el concepto elegido no es una deducción activa")
)

// Tipos de concepto y jornadas (espejo de los CHECK de la migración 0032).
const (
	ConceptoIngreso   = "INGRESO"
	ConceptoDeduccion = "DEDUCCION"
	CargaObrero       = "OBRERO"
	CargaPatronal     = "PATRONAL"
)

// Empleado es la ficha básica para calcular y pagar nómina.
type Empleado struct {
	ID                 string `json:"id"`
	Nombre             string `json:"nombre"`
	TipoIdentificacion string `json:"tipo_identificacion"`
	Identificacion     string `json:"identificacion"`
	Email              string `json:"email"`
	Telefono           string `json:"telefono"`
	IBAN               string `json:"iban"`
	DepartamentoID     string `json:"departamento_id"`
	DepartamentoNombre string `json:"departamento_nombre"`
	Puesto             string `json:"puesto"`
	FechaIngreso       string `json:"fecha_ingreso"`
	FechaSalida        string `json:"fecha_salida"`
	SalarioBase        string `json:"salario_base"`
	Jornada            string `json:"jornada"`
	// Créditos fiscales por familia (Renta 45333-H).
	Hijos   int  `json:"hijos"`
	Conyuge bool `json:"conyuge"`
	Activo  bool `json:"activo"`
	// DeduccionesActivas es el conteo de deducciones recurrentes vigentes (para la tabla).
	DeduccionesActivas int `json:"deducciones_activas"`
}

// EmpleadoInput son los datos para crear/editar un empleado.
type EmpleadoInput struct {
	Nombre             string
	TipoIdentificacion string
	Identificacion     string
	Email              string
	Telefono           string
	IBAN               string
	DepartamentoID     string
	Puesto             string
	FechaIngreso       string
	SalarioBase        decimal.Decimal
	Jornada            string
	Hijos              int
	Conyuge            bool
}

// Carga es una carga social parametrizada (% sobre el salario afecto a CCSS).
// Pct viaja como texto decimal para no pasar dinero/porcentajes por float.
type Carga struct {
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"` // OBRERO | PATRONAL
	Pct    string `json:"pct"`
}

// TramoRenta es un tramo del impuesto al salario. Hasta nil = tramo final abierto.
type TramoRenta struct {
	Hasta *string `json:"hasta"`
	Pct   string  `json:"pct"`
}

// RentaConfig agrupa los tramos y créditos familiares del impuesto al salario.
type RentaConfig struct {
	Tramos         []TramoRenta `json:"tramos"`
	CreditoHijo    string       `json:"credito_hijo"`
	CreditoConyuge string       `json:"credito_conyuge"`
}

// Parametros son los parámetros de nómina de una empresa para un año (versionados).
type Parametros struct {
	ID            string      `json:"id"`
	Anio          int         `json:"anio"`
	Cargas        []Carga     `json:"cargas"`
	Renta         RentaConfig `json:"renta"`
	INSRiesgosPct string      `json:"ins_riesgos_pct"`
	AplicaINA     bool        `json:"aplica_ina"`
	AdelantoPct   string      `json:"adelanto_pct"`
	AdelantoBase  string      `json:"adelanto_base"` // SALARIO_BASE | BRUTO
	Redondeo      string      `json:"redondeo"`      // COLON | CENTIMO
	ProvisionBase string      `json:"provision_base"`
	// Provisiones informativas de la corrida (maqueta: 8.33 / 4.16 / 1.50).
	AguinaldoPct  string `json:"aguinaldo_pct"`
	VacacionesPct string `json:"vacaciones_pct"`
	CesantiaPct   string `json:"cesantia_pct"`
	// Días de vacaciones que se acumulan por mes trabajado (CT art. 153; default 1).
	VacacionesDiasMes string `json:"vacaciones_dias_mes"`
	// Horas extra (CT art. 139). HorasJornadaMes es el divisor para sacar el valor de la hora
	// del salario mensual (240 = 30 × 8, el uso corriente en CR). FactorHoraExtra es el
	// «tiempo y medio»: nunca menos de 1,5 (mínimo legal), más sí se permite.
	HorasJornadaMes string `json:"horas_jornada_mes"`
	FactorHoraExtra string `json:"factor_hora_extra"`
	// Origen: EMPRESA si están guardados; DEFAULT si son los legales de referencia sin guardar.
	Origen string `json:"origen"`
}

// ParametrosInput son los datos para guardar los parámetros de un año.
type ParametrosInput struct {
	Cargas            []Carga
	Renta             RentaConfig
	INSRiesgosPct     decimal.Decimal
	AplicaINA         bool
	AdelantoPct       decimal.Decimal
	AdelantoBase      string
	Redondeo          string
	ProvisionBase     string
	AguinaldoPct      decimal.Decimal
	VacacionesPct     decimal.Decimal
	CesantiaPct       decimal.Decimal
	VacacionesDiasMes decimal.Decimal
	// Horas extra (art. 139). Vacíos = se conservan los valores de referencia (240 y 1,5).
	HorasJornadaMes string
	FactorHoraExtra string
}

// ConceptoNomina es un concepto de ingreso o deducción con sus banderas de afectación.
type ConceptoNomina struct {
	ID              string `json:"id"`
	Nombre          string `json:"nombre"`
	Tipo            string `json:"tipo"`
	AfectaCCSS      bool   `json:"afecta_ccss"`
	AfectaRenta     bool   `json:"afecta_renta"`
	AfectaAguinaldo bool   `json:"afecta_aguinaldo"`
	BaseLegal       string `json:"base_legal"`
	DeSistema       bool   `json:"de_sistema"`
	Activo          bool   `json:"activo"`
	// PorHoras: la novedad se captura en HORAS y el monto lo deriva el motor (horas × valor hora
	// × factor, art. 139 CT). Es una bandera explícita y no una corazonada sobre el nombre: si la
	// pantalla adivinara por el texto del nombre, renombrar el concepto haría que se capturara un
	// monto y la hora extra se pagaría sin el recargo, sin que nada fallara.
	PorHoras bool `json:"por_horas"`
}

// ConceptoInput son los datos para crear/editar un concepto (nunca de sistema).
type ConceptoInput struct {
	Nombre          string
	Tipo            string
	AfectaCCSS      bool
	AfectaRenta     bool
	AfectaAguinaldo bool
	BaseLegal       string
}

// DeduccionEmpleado es una deducción recurrente de un empleado (préstamo, ahorro, pensión…).
type DeduccionEmpleado struct {
	ID             string `json:"id"`
	EmpleadoID     string `json:"empleado_id"`
	ConceptoID     string `json:"concepto_id"`
	ConceptoNombre string `json:"concepto_nombre"`
	Etiqueta       string `json:"etiqueta"`
	Cuota          string `json:"cuota"`
	SaldoTotal     string `json:"saldo_total"`    // "" = recurrente sin tope
	SaldoRestante  string `json:"saldo_restante"` // "" = sin tope
	Prioridad      int    `json:"prioridad"`
	// Frecuencia de cobro: AMBAS (cada quincena) | PRIMERA | SEGUNDA | MENSUAL.
	Frecuencia string `json:"frecuencia"`
	Activo     bool   `json:"activo"`
}

// DeduccionInput son los datos para crear/editar una deducción recurrente.
type DeduccionInput struct {
	ConceptoID string
	Etiqueta   string
	Cuota      decimal.Decimal
	SaldoTotal *decimal.Decimal // nil = sin tope
	Prioridad  int
	Frecuencia string // vacío = MENSUAL
}

// ParametrosDefault2026 son los parámetros legales de referencia CR 2026, verificados en la
// maqueta aprobada (obrero 10.83%, patronal SICERE 25.83% + INS variable; renta Decreto
// 45333-H). Se muestran como DEFAULT hasta que el Director Financiero los guarde para el año.
func ParametrosDefault2026(anio int) Parametros {
	hasta := func(s string) *string { return &s }
	return Parametros{
		Anio: anio,
		Cargas: []Carga{
			{Codigo: "SEM_OBR", Nombre: "CCSS · SEM (Enfermedad y Maternidad)", Tipo: CargaObrero, Pct: "5.50"},
			{Codigo: "IVM_OBR", Nombre: "CCSS · IVM (Invalidez, Vejez y Muerte)", Tipo: CargaObrero, Pct: "4.33"},
			{Codigo: "BP_OBR", Nombre: "Banco Popular obrero (Ley 4351)", Tipo: CargaObrero, Pct: "1.00"},
			{Codigo: "SEM_PAT", Nombre: "CCSS · SEM (Salud)", Tipo: CargaPatronal, Pct: "9.25"},
			{Codigo: "IVM_PAT", Nombre: "CCSS · IVM", Tipo: CargaPatronal, Pct: "5.58"},
			{Codigo: "FODESAF", Nombre: "Asignaciones Familiares (FODESAF)", Tipo: CargaPatronal, Pct: "5.00"},
			{Codigo: "INA", Nombre: "INA", Tipo: CargaPatronal, Pct: "1.50"},
			{Codigo: "IMAS", Nombre: "IMAS", Tipo: CargaPatronal, Pct: "0.50"},
			{Codigo: "BP_PAT", Nombre: "Banco Popular patronal (Ley 4351)", Tipo: CargaPatronal, Pct: "0.25"},
			{Codigo: "BP_LPT", Nombre: "Banco Popular LPT (Ley 7983)", Tipo: CargaPatronal, Pct: "0.25"},
			{Codigo: "FCL", Nombre: "Fondo de Capitalización Laboral · FCL", Tipo: CargaPatronal, Pct: "1.50"},
			{Codigo: "ROP", Nombre: "Pensión Complementaria · ROP/OPC", Tipo: CargaPatronal, Pct: "2.00"},
		},
		Renta: RentaConfig{
			Tramos: []TramoRenta{
				{Hasta: hasta("918000"), Pct: "0"},
				{Hasta: hasta("1347000"), Pct: "10"},
				{Hasta: hasta("2364000"), Pct: "15"},
				{Hasta: hasta("4727000"), Pct: "20"},
				{Hasta: nil, Pct: "25"},
			},
			CreditoHijo:    "1710",
			CreditoConyuge: "2590",
		},
		INSRiesgosPct:     "1.000",
		AplicaINA:         true,
		AdelantoPct:       "50",
		AdelantoBase:      "SALARIO_BASE",
		Redondeo:          "COLON",
		ProvisionBase:     "REMUNERACION_TOTAL",
		AguinaldoPct:      "8.33",
		VacacionesPct:     "4.16",
		CesantiaPct:       "1.50",
		VacacionesDiasMes: "1.00",
		Origen:            "DEFAULT",
	}
}

// conceptoBase describe un concepto que se siembra por empresa (idempotente).
type conceptoBase struct {
	Nombre          string
	Tipo            string
	AfectaCCSS      bool
	AfectaRenta     bool
	AfectaAguinaldo bool
	BaseLegal       string
	DeSistema       bool
}

// ConceptosBase es el catálogo inicial. Los de sistema van con banderas BLOQUEADAS:
// comisiones y bonificaciones habituales SON salario (guardarraíl CCSS).
var ConceptosBase = []conceptoBase{
	{"Salario ordinario", ConceptoIngreso, true, true, true, "Código de Trabajo, art. 162 y ss.", true},
	{"Horas extra", ConceptoIngreso, true, true, true, "Código de Trabajo, art. 139", true},
	{"Comisiones", ConceptoIngreso, true, true, true, "Código de Trabajo, art. 164 — la comisión es salario", true},
	{"Bonificación habitual", ConceptoIngreso, true, true, true, "Jurisprudencia Sala Segunda — el bono habitual es salario", true},
	{"Vacaciones disfrutadas", ConceptoIngreso, true, true, true, "Código de Trabajo, art. 153 y ss. — las vacaciones son salario", true},
	{"Aguinaldo", ConceptoIngreso, false, false, false, "Ley 2412 — exento de cargas y de renta hasta la doceava parte", true},
	// Deducciones con prelación legal (de sistema: no se tocan).
	{"Pensión alimentaria", ConceptoDeduccion, false, false, false, "Código de Familia — orden judicial, prelación máxima", true},
	{"Embargo judicial", ConceptoDeduccion, false, false, false, "Código de Trabajo, art. 172 — sobre el salario embargable", true},
	// Deducciones voluntarias típicas (editables por la empresa).
	{"Asociación solidarista", ConceptoDeduccion, false, false, false, "", false},
	{"Ahorro", ConceptoDeduccion, false, false, false, "", false},
	{"Préstamo", ConceptoDeduccion, false, false, false, "", false},
	{"Soda / comedor", ConceptoDeduccion, false, false, false, "", false},
}
