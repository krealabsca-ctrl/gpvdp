package nomina

// Corrida quincenal (Etapa 2, maqueta aprobada): ADELANTO (día 15, % del salario base,
// sin deducciones) y LIQUIDACION (día 30: mes completo — CCSS obrero sobre la base afecta,
// renta por tramos con créditos familiares, deducciones recurrentes con prelación, y el
// descuento del adelanto realmente pagado en el mes).

import "errors"

// Tipos y estados de corrida (espejo de los CHECK de la migración 0033).
const (
	CorridaAdelanto    = "ADELANTO"
	CorridaLiquidacion = "LIQUIDACION"

	EstBorrador = "BORRADOR"
	EstAprobada = "APROBADA"
	EstPagada   = "PAGADA"
	EstAnulada  = "ANULADA"
)

var (
	// ErrCorridaNoEncontrada indica que la corrida no existe o no es de la empresa.
	ErrCorridaNoEncontrada = errors.New("nomina: corrida no encontrada")
	// ErrCorridaDuplicada indica que ya existe una corrida viva para ese mes y tipo.
	ErrCorridaDuplicada = errors.New("nomina: ya existe una corrida de ese tipo para el mes (anulá la anterior para rehacerla)")
	// ErrCorridaNoEditable exige BORRADOR para recalcular o capturar novedades.
	ErrCorridaNoEditable = errors.New("nomina: la corrida ya no está en borrador; no se puede modificar")
	// ErrCorridaNoAprobable exige BORRADOR para aprobar.
	ErrCorridaNoAprobable = errors.New("nomina: solo una corrida en borrador se puede aprobar")
	// ErrCorridaNoPagable exige APROBADA para pagar.
	ErrCorridaNoPagable = errors.New("nomina: solo una corrida aprobada se puede marcar pagada")
	// ErrCorridaNoAnulable impide anular una corrida ya pagada.
	ErrCorridaNoAnulable = errors.New("nomina: una corrida pagada no se puede anular")
	// ErrCorridaSinEmpleados exige al menos un empleado activo para calcular.
	ErrCorridaSinEmpleados = errors.New("nomina: no hay empleados activos para calcular la corrida")
	// ErrNovedadSoloLiquidacion: las novedades del mes se capturan en la liquidación.
	ErrNovedadSoloLiquidacion = errors.New("nomina: las novedades solo se capturan en la corrida de liquidación")
	// ErrNovedadInvalida exige monto positivo y un concepto INGRESO activo de la empresa.
	ErrNovedadInvalida = errors.New("nomina: novedad inválida (monto > 0 y concepto de ingreso activo)")
	// ErrMesInvalido exige un período calendario válido.
	ErrMesInvalido = errors.New("nomina: período inválido (año >= 2024, mes 1-12)")
	// ErrAdelantoPendiente impide aprobar la liquidación con el adelanto del mes aún en
	// borrador: primero se aprueba (y se descuenta) o se anula — evita pagar el mes 1.5 veces.
	ErrAdelantoPendiente = errors.New("nomina: el adelanto del mes sigue en borrador; aprobalo o anulalo antes de aprobar la liquidación")
	// ErrLiquidacionCerrada impide crear o aprobar un ADELANTO cuando la liquidación del mes
	// ya está aprobada o pagada: ese adelanto jamás se descontaría (el mes se pagaría 1.5x).
	ErrLiquidacionCerrada = errors.New("nomina: la liquidación del mes ya está aprobada o pagada; un adelanto de ese mes jamás se descontaría")
	// ErrAdelantoDescontado impide anular un adelanto aprobado que una liquidación aprobada o
	// pagada ya descontó: anularlo dejaría al empleado sin un monto que ya se le retuvo.
	ErrAdelantoDescontado = errors.New("nomina: la liquidación del mes ya descontó este adelanto; debe pagarse (anularlo dejaría al empleado sin ese monto)")
	// ErrNetoNegativo bloquea aprobar una corrida con colillas en negativo (adelanto muy alto
	// o novedades insuficientes): se corrige en borrador, nunca se congela un depósito negativo.
	ErrNetoNegativo = errors.New("nomina: hay colillas con neto negativo; ajustá el adelanto, las novedades o las deducciones antes de aprobar")
	// ErrAdelantoSinColilla bloquea aprobar la liquidación si hay adelantos pagados del mes de
	// empleados que ya no aparecen en la corrida (baja posterior al adelanto): sin esta guarda,
	// ese salario pagado quedaría sin cotizar CCSS ni descontarse (subdeclaración silenciosa).
	ErrAdelantoSinColilla = errors.New("nomina: hay adelantos del mes de empleados fuera de la corrida (baja posterior); reactivá al empleado o resolvé su liquidación final antes de aprobar")
)

// Corrida es una corrida de nómina (cabecera con totales).
type Corrida struct {
	ID         string `json:"id"`
	Anio       int    `json:"anio"`
	Mes        int    `json:"mes"`
	Tipo       string `json:"tipo"`
	Estado     string `json:"estado"`
	FechaPago  string `json:"fecha_pago"`
	Empleados  int    `json:"empleados"`
	TotalBruto string `json:"total_bruto"`
	TotalCCSS  string `json:"total_ccss_obrero"`
	TotalRenta string `json:"total_renta"`
	TotalDeduc string `json:"total_deducciones"`
	TotalAdel  string `json:"total_adelanto"`
	TotalNeto  string `json:"total_neto"`
	TotalPatr  string `json:"total_patronal"`
	TotalProv  string `json:"total_provisiones"`
	CreadoEn   string `json:"creado_en"`
	AprobadoEn string `json:"aprobado_en"`
	PagadoEn   string `json:"pagado_en"`
}

// LineaCorrida es la colilla de un empleado dentro de la corrida.
type LineaCorrida struct {
	ID             string `json:"id"`
	EmpleadoID     string `json:"empleado_id"`
	Nombre         string `json:"nombre"`
	Identificacion string `json:"identificacion"`
	IBAN           string `json:"iban"`
	Departamento   string `json:"departamento"`
	Puesto         string `json:"puesto"`
	SalarioBase    string `json:"salario_base"`
	Hijos          int    `json:"hijos"`
	Conyuge        bool   `json:"conyuge"`
	Bruto          string `json:"bruto"`
	BaseCCSS       string `json:"base_ccss"`
	BaseRenta      string `json:"base_renta"`
	CCSSObrero     string `json:"ccss_obrero"`
	Renta          string `json:"renta"`
	Deducciones    string `json:"deducciones"`
	Adelanto       string `json:"adelanto"`
	Neto           string `json:"neto"`
	Patronal       string `json:"patronal"`
	ProvAguinaldo  string `json:"prov_aguinaldo"`
	ProvVacaciones string `json:"prov_vacaciones"`
	ProvCesantia   string `json:"prov_cesantia"`
	// Tratamiento aplicado: QUINCENA_1 | QUINCENA_2 (salario quincenal real) ·
	// ADELANTO (anticipo sin deducciones) · MENSUAL (liquidación del mes).
	Tratamiento string         `json:"tratamiento"`
	Detalle     []DetalleLinea `json:"detalle"`
}

// Tratamientos de la colilla (espejo del CHECK de la migración 0036).
const (
	TratQuincena1 = "QUINCENA_1"
	TratQuincena2 = "QUINCENA_2"
	TratAdelanto  = "ADELANTO"
	TratMensual   = "MENSUAL"
	// JornadaQuincenal recibe dos salarios reales por mes (decisión del DF 2026-07-29).
	JornadaQuincenal = "QUINCENAL"
)

// DetalleLinea es un renglón de la colilla (ingreso, carga, renta, deducción, adelanto).
// Montos como string decimal (nunca float). DeduccionID enlaza la deducción recurrente
// aplicada, para descontar su saldo al marcar la corrida como PAGADA.
type DetalleLinea struct {
	Tipo        string `json:"tipo"` // INGRESO | CCSS | RENTA | DEDUCCION | ADELANTO | PATRONAL | PROVISION
	Nombre      string `json:"nombre"`
	Monto       string `json:"monto"`
	DeduccionID string `json:"deduccion_id,omitempty"`
}

// CorridaDetalle es la corrida con sus colillas.
type CorridaDetalle struct {
	Corrida
	Lineas    []LineaCorrida   `json:"lineas"`
	Novedades []NovedadCorrida `json:"novedades"`
}

// NovedadCorrida es un monto del mes por empleado y concepto (comisiones, extras…).
type NovedadCorrida struct {
	EmpleadoID     string `json:"empleado_id"`
	ConceptoID     string `json:"concepto_id"`
	ConceptoNombre string `json:"concepto_nombre"`
	Monto          string `json:"monto"`
	// Cantidad son las HORAS cuando la novedad se registró por horas (extra); "0" si el monto
	// se digitó directo. El monto de una novedad por horas se recalcula en cada corrida.
	Cantidad string `json:"cantidad"`
}

// NovedadInput es la captura de novedades (reemplaza el set completo de la corrida).
type NovedadInput struct {
	EmpleadoID string
	ConceptoID string
	Monto      string // decimal plano; se valida al parsear
	// Cantidad: horas trabajadas. Con cantidad, el MONTO lo calcula el sistema (art. 139) y
	// lo que venga en Monto se ignora — nadie teclea el pago de las horas extra.
	Cantidad string
}
