package bancos

// Saldos diarios por cuenta bancaria y posición de tesorería (Tanda 1, maqueta aprobada).
//
// El flujo real: la tesorera captura todos los días el saldo que le muestra el banco. El
// sistema calcula el saldo que ESPERA según los movimientos ya cargados y compara: si no
// cuadra, o faltan movimientos del día o hubo un error de digitación. Ese es el control de
// completitud que el módulo no tenía.
//
// Solo se guarda el hecho declarado (el saldo). Todo lo demás se deriva, así que cuando
// entran los movimientos que faltaban la diferencia se cierra sola.

import "errors"

// ErrFechaInvalida y ErrCuentaNoEncontrada ya existen en el módulo (errors_pipeline.go) y se
// reutilizan: un mismo problema no debe tener dos errores distintos.
var (
	// ErrSaldoInvalido exige un monto numérico (puede ser negativo: un sobregiro existe).
	ErrSaldoInvalido = errors.New("bancos: saldo inválido")
	// ErrSinCuentas indica que no se envió ningún saldo que guardar.
	ErrSinCuentas = errors.New("bancos: no se envió ningún saldo")
)

// Estados del cuadre de un saldo capturado contra lo que esperaban los movimientos.
const (
	// CuadreOK: el saldo declarado coincide con el esperado.
	CuadreOK = "CUADRA"
	// CuadreDifiere: hay diferencia — faltan movimientos o hay un error de captura.
	CuadreDifiere = "DIFIERE"
	// CuadreSinCaptura: nadie declaró el saldo de esa cuenta ese día.
	CuadreSinCaptura = "SIN_CAPTURA"
	// CuadreSinAnterior: no hay saldo previo, así que todavía no se puede esperar nada
	// (el primer día de uso de cada cuenta).
	CuadreSinAnterior = "SIN_ANTERIOR"
)

// SaldoDelDia es la fila de una cuenta en la pantalla de saldos del día.
type SaldoDelDia struct {
	CuentaID string `json:"cuenta_id"`
	Alias    string `json:"alias"`
	Banco    string `json:"banco"`
	Moneda   string `json:"moneda"`
	// SaldoAnterior es el último saldo capturado ANTES de la fecha pedida, con su día.
	SaldoAnterior string `json:"saldo_anterior"`
	FechaAnterior string `json:"fecha_anterior"`
	// Movimientos del día ya cargados para esa cuenta (en la moneda de la cuenta).
	EntradasDia string `json:"entradas_dia"`
	SalidasDia  string `json:"salidas_dia"`
	// SaldoEsperado = anterior + entradas − salidas. Vacío si no hay saldo anterior.
	SaldoEsperado string `json:"saldo_esperado"`
	// Saldo capturado por la tesorera (vacío si todavía no se capturó) y su nota.
	Saldo       string `json:"saldo"`
	Nota        string `json:"nota"`
	CapturadoEn string `json:"capturado_en"`
	// Congelado: Dirección Financiera ya revisó ese saldo, así que no se edita (la UI
	// bloquea el campo y el repositorio rechaza la sobrescritura).
	Congelado  bool   `json:"congelado"`
	RevisadoEn string `json:"revisado_en"`
	// Diferencia = saldo − esperado (vacío si falta alguno de los dos).
	Diferencia string `json:"diferencia"`
	Cuadre     string `json:"cuadre"`
	// Señal de carga: hasta cuándo llegan los movimientos de esa cuenta.
	UltimoMovimiento string `json:"ultimo_movimiento"`
	DiasSinCargar    int    `json:"dias_sin_cargar"`
}

// TotalMoneda es el disponible de una moneda (nunca se mezclan: convertir exigiría un TC del
// día, que es una decisión pendiente del Director Financiero).
type TotalMoneda struct {
	Moneda     string `json:"moneda"`
	Monto      string `json:"monto"`
	Cuentas    int    `json:"cuentas"`
	Capturadas int    `json:"capturadas"`
}

// TotalBanco es el disponible concentrado en un banco (riesgo de concentración).
type TotalBanco struct {
	Banco       string `json:"banco"`
	MontoCRC    string `json:"monto_crc"`
	MontoUSD    string `json:"monto_usd"`
	Cuentas     int    `json:"cuentas"`
	SinCapturar int    `json:"sin_capturar"`
}

// PuntoSaldo es un día de la serie del disponible (solo colones: la serie compara consigo misma).
type PuntoSaldo struct {
	Fecha      string `json:"fecha"`
	MontoCRC   string `json:"monto_crc"`
	Capturadas int    `json:"capturadas"`
	EsHoy      bool   `json:"es_hoy"`
}

// Tesoreria es todo lo que necesitan las pantallas «Saldos del día» y «Posición».
type Tesoreria struct {
	Fecha string `json:"fecha"`
	// Hoy: día de operación de Costa Rica según la base, para rotular sin ambigüedad.
	Hoy    string        `json:"hoy"`
	Saldos []SaldoDelDia `json:"saldos"`
	// Totales por moneda y por banco, sobre lo efectivamente capturado.
	Totales []TotalMoneda `json:"totales"`
	Bancos  []TotalBanco  `json:"bancos"`
	Serie   []PuntoSaldo  `json:"serie"`
	// Resumen del día: cuántas cuentas faltan, cuántas no cuadran y cómo viene la carga.
	// Atrasadas y Rezagadas usan EXACTAMENTE los mismos cortes que el checklist de carga
	// (estadoCarga): un mismo hecho no puede llamarse distinto en dos pantallas.
	Cuentas     int `json:"cuentas"`
	SinCapturar int `json:"sin_capturar"`
	NoCuadran   int `json:"no_cuadran"`
	Atrasadas   int `json:"atrasadas"`
	Rezagadas   int `json:"rezagadas"`
	// Congeladas: cuántas ya revisó Dirección Financiera. Si son todas las capturadas, el día
	// está cerrado y la pantalla lo muestra en modo lectura.
	Congeladas  int  `json:"congeladas"`
	DiaRevisado bool `json:"dia_revisado"`
}

// SaldoInput es la captura de una cuenta.
type SaldoInput struct {
	CuentaID string `json:"cuenta_id"`
	Saldo    string `json:"saldo"`
	Nota     string `json:"nota"`
}

// CargaCuenta es la fila del checklist de carga del mes.
type CargaCuenta struct {
	CuentaID         string `json:"cuenta_id"`
	Alias            string `json:"alias"`
	Banco            string `json:"banco"`
	Moneda           string `json:"moneda"`
	Movimientos      int    `json:"movimientos"`
	UltimoMovimiento string `json:"ultimo_movimiento"`
	DiasSinCargar    int    `json:"dias_sin_cargar"`
	// Estado: AL_DIA (≤7 días) · ATRASADA (8-14) · REZAGADA (>14) · SIN_CARGA (0 movimientos).
	Estado string `json:"estado"`
}

// Estados del checklist de carga.
const (
	CargaAlDia    = "AL_DIA"
	CargaAtrasada = "ATRASADA"
	CargaRezagada = "REZAGADA"
	CargaSinCarga = "SIN_CARGA"
)

// estadoCarga clasifica la antigüedad de la última carga de una cuenta. Los cortes (7 y 14
// días) son de operación, no contables: sirven para que nadie cierre un mes creyendo que está
// completo cuando al banco le faltan los últimos días.
func estadoCarga(movimientos, diasSinCargar int) string {
	switch {
	case movimientos == 0:
		return CargaSinCarga
	case diasSinCargar > 14:
		return CargaRezagada
	case diasSinCargar > 7:
		return CargaAtrasada
	default:
		return CargaAlDia
	}
}
