package cxp

import (
	"errors"
	"time"
)

// ErrPeriodoInvalido exige el período en formato YYYY-MM.
var ErrPeriodoInvalido = errors.New("cxp: período inválido (formato YYYY-MM)")

// offsetCR es la diferencia fija de Costa Rica con UTC (−6, sin horario de verano). Se usa
// para resolver el mes en curso sin depender de la base de datos de husos del contenedor.
const offsetCR = -6 * time.Hour

// PeriodoActualCR devuelve el mes en curso (YYYY-MM) según el calendario de Costa Rica.
func PeriodoActualCR() string {
	return time.Now().UTC().Add(offsetCR).Format("2006-01")
}

// periodoValido acepta solo YYYY-MM de un año razonable.
func periodoValido(periodo string) bool {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return false
	}
	return t.Year() >= 2000 && t.Year() <= 2100
}

// ultimosPeriodos devuelve los n períodos (YYYY-MM) que terminan en el indicado, del más
// antiguo al más reciente.
func ultimosPeriodos(periodo string, n int) ([]string, error) {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return nil, ErrPeriodoInvalido
	}
	if n < 1 {
		n = 1
	}
	out := make([]string, 0, n)
	inicio := t.AddDate(0, -(n - 1), 0)
	for i := 0; i < n; i++ {
		out = append(out, inicio.AddDate(0, i, 0).Format("2006-01"))
	}
	return out, nil
}

// EventoHistorial es una entrada de la línea de tiempo (trazabilidad) de un documento:
// quién hizo qué y cuándo. Se arma desde auditoria_evento (append-only).
type EventoHistorial struct {
	Accion  string `json:"accion"`
	Usuario string `json:"usuario"`
	Fecha   string `json:"fecha"`          // ISO 8601 con offset
	Nota    string `json:"nota,omitempty"` // motivo/comentario del evento (p. ej. devolución)
}

// ConteoEstado agrega cantidad y monto (total_crc) de documentos en un estado.
type ConteoEstado struct {
	Estado   string `json:"estado"`
	Cantidad int    `json:"cantidad"`
	Monto    string `json:"monto"` // decimal como string (nunca float)
}

// ── Dashboard CxP ──────────────────────────────────────────────────────────────
//
// El tablero distingue dos naturalezas que antes estaban mezcladas en un solo número:
//
//   - CARTERA (stock): lo que se debe HOY. No depende del período elegido — filtrarla por
//     mes escondería el arrastre, que es justo la deuda que hay que trabajar.
//   - MOVIMIENTO (flujo): lo que entró y lo que se pagó EN EL PERÍODO elegido.
//
// Montos: la cartera y el vencimiento van en NETO (total − retención − anticipos aplicados,
// lo que de verdad sale del banco, misma fórmula que el archivo de pago); la cola de trabajo
// y el movimiento van al monto de la factura, igual que la Bandeja y el detalle del documento.
// El tablero expone las dos cifras de la cartera para que la diferencia nunca quede oculta.

// Cubo es un conteo de documentos con su monto (decimal como string, nunca float).
type Cubo struct {
	Cantidad int    `json:"cantidad"`
	Monto    string `json:"monto"`
}

// Claves de los tramos de antigüedad de la cartera (orden de presentación en la UI).
const (
	TramoV90      = "v90"       // vencido hace más de 90 días
	TramoV61      = "v61"       // vencido 61 a 90
	TramoV31      = "v31"       // vencido 31 a 60
	TramoV1       = "v1"        // vencido 1 a 30
	TramoSemana   = "s7"        // vence en los próximos 7 días
	TramoMes      = "s30"       // vence en 8 a 30 días
	TramoFuturo   = "futuro"    // vence en más de 30 días
	TramoSinFecha = "sin_fecha" // sin fecha de vencimiento registrada
)

// TramoVencimiento es un tramo de antigüedad de la cartera abierta (monto neto).
type TramoVencimiento struct {
	Clave    string `json:"clave"`
	Vencido  bool   `json:"vencido"`
	Cantidad int    `json:"cantidad"`
	Monto    string `json:"monto"`
}

// ProveedorCartera es un proveedor con saldo en la cartera abierta (concentración).
type ProveedorCartera struct {
	Nombre   string `json:"nombre"`
	Cantidad int    `json:"cantidad"`
	Monto    string `json:"monto"`
	Vencidos int    `json:"vencidos"`
}

// PuntoMesCxP es un mes de la serie de facturas recibidas (por fecha de emisión).
type PuntoMesCxP struct {
	Periodo  string `json:"periodo"` // YYYY-MM
	Cantidad int    `json:"cantidad"`
	Monto    string `json:"monto"`
	EnCurso  bool   `json:"en_curso"`
}

// CarteraCxP es la foto de lo que se debe hoy.
type CarteraCxP struct {
	// Abierta en neto; Bruto es la suma de las facturas (para explicar la diferencia).
	Abierta Cubo   `json:"abierta"`
	Bruto   string `json:"bruto"`
	// Retención y anticipos aplicados que explican neto vs bruto.
	Retencion string `json:"retencion"`
	Anticipos string `json:"anticipos"`
	// Vencido y lo que vence en los próximos 7 días.
	Vencido     Cubo `json:"vencido"`
	VenceSemana Cubo `json:"vence_semana"`
	// Rebotadas por el banco: siguen abiertas y exigen acción.
	Rebotadas Cubo `json:"rebotadas"`
	// Prioridad AA («se paga sí o sí») y cuántas de esas ya están vencidas.
	PrioridadAA    Cubo `json:"prioridad_aa"`
	AAVencidas     int  `json:"aa_vencidas"`
	DiasMasAntigua int  `json:"dias_mas_antigua"`
	// Trabas de la cola: sin área asignada no hay quien valide; sin concepto no hay gasto.
	SinDepartamento Cubo               `json:"sin_departamento"`
	SinClasificar   Cubo               `json:"sin_clasificar"`
	Tramos          []TramoVencimiento `json:"tramos"`
	TopProveedores  []ProveedorCartera `json:"top_proveedores"`
}

// MovimientoCxP es lo que pasó en el período elegido.
type MovimientoCxP struct {
	Recibidas Cubo `json:"recibidas"`
	Pagadas   Cubo `json:"pagadas"`
	// CicloDias: promedio de días entre la emisión de la factura y su pago efectivo.
	CicloDias string `json:"ciclo_dias"`
	// PagadasSinEvento: documentos pagados sin evento de pago en la auditoría (importados),
	// que por eso no se pueden fechar ni entran en el ciclo. Se declara, no se esconde.
	PagadasSinEvento int           `json:"pagadas_sin_evento"`
	Serie            []PuntoMesCxP `json:"serie"`
}

// DashboardCxP es el tablero del módulo CxP de una empresa.
type DashboardCxP struct {
	Periodo string `json:"periodo"` // YYYY-MM del movimiento
	// Hoy es el día calendario de Costa Rica con el que se calculó la cartera.
	Hoy                string         `json:"hoy"`
	Cartera            CarteraCxP     `json:"cartera"`
	Cola               []FaseBandeja  `json:"cola"`
	Movimiento         MovimientoCxP  `json:"movimiento"`
	PorEstado          []ConteoEstado `json:"por_estado"`
	TotalDocumentos    int            `json:"total_documentos"`
	TotalMonto         string         `json:"total_monto"`
	ProveedoresActivos int            `json:"proveedores_activos"`
	// AlcanceLimitado avisa que el tablero está recortado a los departamentos del usuario
	// (mismo alcance que su Bandeja): así el número nunca se lee como si fuera de la empresa.
	AlcanceLimitado bool `json:"alcance_limitado"`
}
