package grupo

// Tipos que viajan al cliente. Los montos son strings decimales (nunca float64).

// EmpresaVisible es una empresa que el usuario puede ver, con el rol que le da el acceso.
type EmpresaVisible struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Rol    string `json:"rol"`
}

// FilaEmpresa es el aporte de una empresa al consolidado.
type FilaEmpresa struct {
	EmpresaID string `json:"empresa_id"`
	Empresa   string `json:"empresa"`
	// Ingresos y Gastos salen de la NATURALEZA declarada en el concepto, no del signo del movimiento:
	// el ahorro, las reservas y los aportes entre empresas son NEUTRO y no son gasto.
	IngresosCRC string `json:"ingresos_crc"`
	GastosCRC   string `json:"gastos_crc"`
	// EbitdaCRC = ingresos − gastos, con los NEUTRO afuera. Es la definición confirmada del proyecto.
	EbitdaCRC string `json:"ebitda_crc"`
	// NeutroCRC es lo que se movió sin ser ingreso ni gasto (traslados, ahorro, reservas). Va a la
	// vista porque en este grupo es el número más grande de todos y omitirlo hace pensar que falta
	// plata.
	NeutroCRC   string `json:"neutro_crc"`
	Movimientos int    `json:"movimientos"`
	// SinClasificar es la medida de confianza de la fila. Un EBITDA con un tercio del dinero sin
	// partida al lado no es un resultado: es una foto parcial, y la pantalla tiene que decirlo.
	SinClasificar    int    `json:"sin_clasificar"`
	SinClasificarCRC string `json:"sin_clasificar_crc"`
	PctClasificado   string `json:"pct_clasificado"`
	// Confiable: el período de esta empresa llega al 90 % clasificado, que es el umbral que el
	// proyecto ya usa para dar un número por bueno. Una empresa SIN datos nunca es confiable: no hay
	// nada que respalde su cero.
	Confiable bool `json:"confiable"`
	// SinDatos: ni un movimiento en el período. Es distinto de «todo clasificado» y hay que separarlo,
	// porque el porcentaje de un conjunto vacío da 100 % y pinta de verde un mes en el que nadie
	// cargó nada. Un mes vacío que se presenta como completo es justo la mentira que esta vista
	// existe para evitar.
	SinDatos bool `json:"sin_datos"`
}

// OperacionInterna es un movimiento clasificado con una partida que nombra a otra empresa del grupo.
//
// No se elimina del total (decisión del Director Financiero): se muestra aparte para que quien lea el
// consolidado sepa cuánto de ese número se movió entre bolsillos del mismo grupo.
type OperacionInterna struct {
	EmpresaID string `json:"empresa_id"`
	Empresa   string `json:"empresa"`
	// Contraparte es la empresa del grupo que la partida nombra.
	Contraparte string `json:"contraparte"`
	Partida     string `json:"partida"`
	Naturaleza  string `json:"naturaleza"`
	Movimientos int    `json:"movimientos"`
	MontoCRC    string `json:"monto_crc"`
	// AfectaEbitda: solo si la naturaleza es INGRESO o GASTO. Los traslados ya son NEUTRO y no
	// distorsionan el resultado; una regalía clasificada como GASTO sí.
	AfectaEbitda bool `json:"afecta_ebitda"`
}

// PartidaGrupo es una partida sumada en todas las empresas visibles, con su desglose.
type PartidaGrupo struct {
	Partida     string `json:"partida"`
	Concepto    string `json:"concepto"`
	Naturaleza  string `json:"naturaleza"`
	MontoCRC    string `json:"monto_crc"`
	Movimientos int    `json:"movimientos"`
	// PorEmpresa dice de dónde sale el número: una partida grande del grupo puede ser de una sola
	// empresa, y eso cambia por completo qué se hace con ella.
	PorEmpresa []AporteEmpresa `json:"por_empresa"`
}

// AporteEmpresa es cuánto puso una empresa en una partida del grupo.
type AporteEmpresa struct {
	Empresa  string `json:"empresa"`
	MontoCRC string `json:"monto_crc"`
}

// ResumenGrupo es lo que devuelve la vista.
type ResumenGrupo struct {
	Periodo string `json:"periodo"`
	// Empresas incluidas, en el orden en que se suman.
	Empresas []FilaEmpresa `json:"empresas"`
	// Totales del grupo: la suma de las filas de arriba, sin eliminar nada.
	IngresosCRC      string `json:"ingresos_crc"`
	GastosCRC        string `json:"gastos_crc"`
	EbitdaCRC        string `json:"ebitda_crc"`
	NeutroCRC        string `json:"neutro_crc"`
	Movimientos      int    `json:"movimientos"`
	SinClasificarCRC string `json:"sin_clasificar_crc"`
	// EntreEmpresas es cuánto del total se movió entre empresas del grupo, y el detalle.
	EntreEmpresasCRC       string             `json:"entre_empresas_crc"`
	EntreEmpresasEbitdaCRC string             `json:"entre_empresas_ebitda_crc"`
	EntreEmpresas          []OperacionInterna `json:"entre_empresas"`
	// Partidas más grandes del grupo.
	Partidas []PartidaGrupo `json:"partidas"`
	// ── Lo que la vista tiene que confesar ───────────────────────────────────
	//
	// EmpresasDelSistema vs. las incluidas: si el usuario no ve una, el total NO es el del grupo y
	// hay que decirlo con nombre y apellido.
	EmpresasDelSistema int      `json:"empresas_del_sistema"`
	Excluidas          []string `json:"excluidas"`
	// Aviso explica en una frase por qué el número puede no ser el que se espera.
	Aviso string `json:"aviso"`
	// Completo: todas las empresas del sistema están incluidas y todas respaldan su número (con datos
	// y al 90 % clasificado). Es la única bandera que autoriza a leer el total como «el del grupo».
	Completo bool `json:"completo"`
}
