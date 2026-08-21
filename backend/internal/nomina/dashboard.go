package nomina

// Dashboard de RRHH (Etapa 5, pantalla aprobada de la maqueta): el COSTO REAL de la
// planilla del mes, la tendencia, a dónde va cada colón y qué falta del ciclo.
//
// GUARDARRAÍL: aquí no hay ningún indicador de "ahorro por no reportar". Lo que se mide es
// el costo real y completo del patrono (bruto + cargas + provisiones) sobre la base
// contributiva íntegra; la única mención a exclusiones es la alerta que verifica que cada
// concepto no afecto tenga base legal registrada.

// Dashboard es el resumen del mes para la pantalla de inicio de RRHH.
type Dashboard struct {
	Anio int `json:"anio"`
	Mes  int `json:"mes"`
	// Empleados activos hoy (headcount) y cuántos entran en la corrida del mes.
	Empleados        int `json:"empleados"`
	EmpleadosPagados int `json:"empleados_pagados"`
	// Costo real del mes = bruto devengado + cargas patronales + provisiones. NO incluye
	// finiquitos (van aparte: su cesantía y aguinaldo ya se venían provisionando).
	CostoReal string `json:"costo_real"`
	Bruto     string `json:"bruto"`
	Patronal  string `json:"patronal"`
	// BaseCCSS es la base contributiva del mes; PatronalPct es la carga sobre ella.
	BaseCCSS       string `json:"base_ccss"`
	PatronalPct    string `json:"patronal_pct"`
	Provisiones    string `json:"provisiones"`
	ProvAguinaldo  string `json:"prov_aguinaldo"`
	ProvVacaciones string `json:"prov_vacaciones"`
	ProvCesantia   string `json:"prov_cesantia"`
	// Neto desembolsado en el mes (adelanto + liquidación) y neto de la liquidación.
	Neto            string `json:"neto"`
	NetoLiquidacion string `json:"neto_liquidacion"`
	// CostoPor100 son los colones de costo real por cada ₡100 de salario bruto.
	CostoPor100 string             `json:"costo_por_100"`
	Tendencia   []DashboardMes     `json:"tendencia"`
	Ciclo       DashboardCiclo     `json:"ciclo"`
	Finiquitos  DashboardFiniquito `json:"finiquitos"`
	Headcount   []DashboardDepto   `json:"headcount"`
	Alertas     []DashboardAlerta  `json:"alertas"`
}

// DashboardMes es un punto de la tendencia del costo de planilla.
type DashboardMes struct {
	Anio     int    `json:"anio"`
	Mes      int    `json:"mes"`
	Etiqueta string `json:"etiqueta"` // "Feb", "Mar"…
	Costo    string `json:"costo"`
	EnCurso  bool   `json:"en_curso"`
}

// DashboardCiclo es el estado del ciclo quincenal del mes.
type DashboardCiclo struct {
	Adelanto    DashboardPaso `json:"adelanto"`
	Liquidacion DashboardPaso `json:"liquidacion"`
	Planilla    DashboardPaso `json:"planilla"`
}

// DashboardPaso es un paso del ciclo: estado + la corrida a la que lleva el botón.
type DashboardPaso struct {
	// Estado: SIN_CREAR | BORRADOR | APROBADA | PAGADA (la planilla usa PENDIENTE | LISTA).
	Estado    string `json:"estado"`
	CorridaID string `json:"corrida_id"`
	Etiqueta  string `json:"etiqueta"`
}

// DashboardFiniquito resume los ceses del mes (se pagan y se reportan aparte del salario).
type DashboardFiniquito struct {
	Cantidad       int    `json:"cantidad"`
	Total          string `json:"total"`
	Patronal       string `json:"patronal"`
	PendientesPago int    `json:"pendientes_pago"`
}

// DashboardDepto es el headcount de un departamento.
type DashboardDepto struct {
	Departamento string `json:"departamento"`
	Empleados    int    `json:"empleados"`
}

// DashboardAlerta es un aviso accionable antes de pagar.
type DashboardAlerta struct {
	Tono  string `json:"tono"` // WARN | INFO | LEGAL
	Icono string `json:"icono"`
	Texto string `json:"texto"`
}

// Estados de los pasos del ciclo que no vienen de la tabla de corridas.
const (
	PasoSinCrear  = "SIN_CREAR"
	PasoPendiente = "PENDIENTE"
	PasoLista     = "LISTA"
)

// ResumenMes son los totales del mes agregados por el repositorio.
type ResumenMes struct {
	// Bruto EXCLUYE las colillas de tratamiento ADELANTO: en jornada mensual el adelanto
	// es un pago a cuenta del mismo salario que la liquidación ya devenga íntegro (sumarlo
	// contaría el mes 1.5 veces). En jornada quincenal las dos mitades sí suman.
	Bruto           string
	BaseCCSS        string
	Patronal        string
	ProvAguinaldo   string
	ProvVacaciones  string
	ProvCesantia    string
	Neto            string
	NetoLiquidacion string
	Empleados       int
}

// CostoMes es el costo real de planilla de un mes (para la tendencia).
type CostoMes struct {
	Anio  int
	Mes   int
	Costo string
}

// EstadoCorridaMes es el tipo/estado de una corrida viva del mes.
type EstadoCorridaMes struct {
	ID     string
	Tipo   string
	Estado string
}

// AvisosNomina son los datos crudos de las alertas (el servicio los redacta). Todos son
// hechos verificables de la base: nada de umbrales inventados.
type AvisosNomina struct {
	SinIBAN            int
	NombresSinIBAN     []string
	DeduccionesActivas int
	SaldoDeducciones   string
	ConceptosNoAfectos int
	SinBaseLegal       int
	IncapacidadesMes   int
}
