package bancos

// Conciliación bancaria mensual: el saldo que dice el banco contra el que dicen los libros,
// con las partidas en tránsito que explican la diferencia.
//
// Decisiones del Director Financiero (2026-07-31):
//   · El acta es documento imprimible/firmable Y pantalla de control.
//   · «Se debe cerrar todo e identificar todo»: si una cuenta queda con diferencia sin
//     explicar, el período NO cierra.
//   · El saldo capturado se congela al revisarlo.

import (
	"errors"
	"fmt"
)

var (
	// ErrPartidaInvalida exige tipo válido, descripción y monto > 0.
	ErrPartidaInvalida = errors.New("bancos: partida de conciliación inválida (tipo, descripción y monto > 0)")
	// ErrSignoRequerido exige el signo cuando el tipo es OTRA (los demás lo fija el sistema).
	ErrSignoRequerido = errors.New("bancos: en una partida «otra» hay que indicar si suma o resta")
	// ErrPartidaNoEncontrada indica que la partida no existe o no es de la empresa.
	ErrPartidaNoEncontrada = errors.New("bancos: partida de conciliación no encontrada")
	// ErrActaNoCuadra impide firmar un acta con diferencia sin explicar.
	ErrActaNoCuadra = errors.New("bancos: el acta no cuadra; registrá las partidas que explican la diferencia antes de firmar")
	// ErrSaldoDelMesFaltante indica que falta capturar el saldo del último día del mes.
	ErrSaldoDelMesFaltante = errors.New("bancos: falta el saldo de cierre del mes de esa cuenta")
	// ErrConciliacionPendiente impide cerrar el período con actas sin firmar.
	ErrConciliacionPendiente = errors.New("bancos: hay cuentas sin conciliar; el período no se puede cerrar hasta que todas cuadren y estén firmadas")
	// ErrSaldoCongelado impide corregir un saldo ya revisado por Dirección Financiera.
	ErrSaldoCongelado = errors.New("bancos: ese saldo ya fue revisado y está congelado; descongelalo para corregirlo")
)

// ErrorConciliacion detalla QUÉ cuentas impiden cerrar el período. Cumple errors.Is con
// ErrConciliacionPendiente para que el handler lo mapee sin conocer el detalle.
type ErrorConciliacion struct {
	Pendientes int
	Cuentas    []string
}

func (e *ErrorConciliacion) Error() string {
	return fmt.Sprintf("%v (%d: %v)", ErrConciliacionPendiente, e.Pendientes, e.Cuentas)
}

// Is hace que errors.Is(err, ErrConciliacionPendiente) sea verdadero.
func (e *ErrorConciliacion) Is(target error) bool { return target == ErrConciliacionPendiente }

// Tipos de partida en tránsito. El signo va del saldo del BANCO al de LIBROS.
const (
	// PartidaDepositoNoAcreditado: libros ya lo tiene, el banco todavía no → suma.
	PartidaDepositoNoAcreditado = "DEPOSITO_NO_ACREDITADO"
	// PartidaTransferenciaNoPresentada: se giró y el banco no la debitó → resta.
	PartidaTransferenciaNoPresentada = "TRANSFERENCIA_NO_PRESENTADA"
	// PartidaCargoBancoNoRegistrado: el banco ya cobró y libros no lo registró → suma
	// (para llegar del saldo del banco al de libros hay que devolver ese cargo).
	PartidaCargoBancoNoRegistrado = "CARGO_BANCO_NO_REGISTRADO"
	// PartidaAbonoBancoNoRegistrado: el banco ya acreditó y libros no lo registró → resta.
	PartidaAbonoBancoNoRegistrado = "ABONO_BANCO_NO_REGISTRADO"
	// PartidaOtra: cualquier otro hecho; acá el signo lo indica quien la registra.
	PartidaOtra = "OTRA"
)

// signoPorTipo fija el signo de los tipos conocidos: un mismo hecho no puede entrar con signo
// distinto según quién lo capture. Devuelve 0 para OTRA (lo aporta el usuario).
func signoPorTipo(tipo string) int {
	switch tipo {
	case PartidaDepositoNoAcreditado, PartidaCargoBancoNoRegistrado:
		return 1
	case PartidaTransferenciaNoPresentada, PartidaAbonoBancoNoRegistrado:
		return -1
	default:
		return 0
	}
}

// tipoPartidaValido acepta solo los tipos del CHECK de la migración.
func tipoPartidaValido(tipo string) bool {
	switch tipo {
	case PartidaDepositoNoAcreditado, PartidaTransferenciaNoPresentada,
		PartidaCargoBancoNoRegistrado, PartidaAbonoBancoNoRegistrado, PartidaOtra:
		return true
	}
	return false
}

// PartidaConciliacion es un hecho que explica la diferencia banco↔libros.
type PartidaConciliacion struct {
	ID            string `json:"id"`
	CuentaID      string `json:"cuenta_id"`
	Tipo          string `json:"tipo"`
	Descripcion   string `json:"descripcion"`
	Monto         string `json:"monto"`
	Signo         int    `json:"signo"`
	RegistradoEn  string `json:"registrado_en"`
	RegistradoPor string `json:"registrado_por"`
}

// PartidaInput es la captura de una partida.
type PartidaInput struct {
	CuentaID    string `json:"cuenta_id"`
	Anio        int    `json:"anio"`
	Mes         int    `json:"mes"`
	Tipo        string `json:"tipo"`
	Descripcion string `json:"descripcion"`
	Monto       string `json:"monto"`
	// Signo solo se usa (y se exige) cuando el tipo es OTRA.
	Signo int `json:"signo"`
}

// ActaConciliacion es la conciliación de UNA cuenta en UN mes: el documento y su estado.
type ActaConciliacion struct {
	CuentaID string `json:"cuenta_id"`
	Alias    string `json:"alias"`
	Banco    string `json:"banco"`
	Moneda   string `json:"moneda"`
	Anio     int    `json:"anio"`
	Mes      int    `json:"mes"`
	// SaldoBanco: el saldo capturado el último día del mes (lo que dice el estado de cuenta).
	SaldoBanco string `json:"saldo_banco"`
	FechaBanco string `json:"fecha_banco"`
	// SaldoLibros = saldo de cierre del mes anterior + movimientos del mes ya cargados.
	SaldoLibros  string `json:"saldo_libros"`
	SaldoInicial string `json:"saldo_inicial"`
	FechaInicial string `json:"fecha_inicial"`
	EntradasMes  string `json:"entradas_mes"`
	SalidasMes   string `json:"salidas_mes"`
	// AjustePartidas es la suma de las partidas con su signo.
	AjustePartidas string                `json:"ajuste_partidas"`
	Partidas       []PartidaConciliacion `json:"partidas"`
	// DiferenciaSinExplicar = (banco + ajuste) − libros. Cero = el acta cuadra.
	DiferenciaSinExplicar string `json:"diferencia_sin_explicar"`
	Cuadra                bool   `json:"cuadra"`
	// Firma del acta (vacío si todavía no se firmó).
	FirmadoEn  string `json:"firmado_en"`
	FirmadoPor string `json:"firmado_por"`
	// Motivo por el que el acta todavía no se puede armar (falta el saldo de cierre, etc.).
	Impedimento string `json:"impedimento"`
}

// Impedimentos para armar un acta. Ninguno es un error del sistema: son datos que faltan.
const (
	// ImpedimentoSinSaldoBanco: nadie capturó el saldo del cierre del mes en esa cuenta.
	ImpedimentoSinSaldoBanco = "SIN_SALDO_BANCO"
	// ImpedimentoSinSaldoInicial: no hay saldo capturado antes del mes, así que no hay punto
	// de partida para los libros. Se resuelve capturando el saldo del último día del mes
	// anterior (es la forma de declarar el saldo de apertura la primera vez).
	ImpedimentoSinSaldoInicial = "SIN_SALDO_INICIAL"
)

// Conciliacion es la pantalla de control: todas las actas del mes y su estado agregado.
type Conciliacion struct {
	Anio    int    `json:"anio"`
	Mes     int    `json:"mes"`
	Periodo string `json:"periodo"`
	// Cerrado: si el período ya cerró, las actas son historia (no se tocan).
	Cerrado bool               `json:"cerrado"`
	Actas   []ActaConciliacion `json:"actas"`
	// Semáforo del mes.
	Cuentas       int `json:"cuentas"`
	Firmadas      int `json:"firmadas"`
	Cuadran       int `json:"cuadran"`
	ConDiferencia int `json:"con_diferencia"`
	Incompletas   int `json:"incompletas"`
	// PuedeCerrar: todas las cuentas activas tienen su acta firmada («cerrar todo e
	// identificar todo»). Es la misma condición que evalúa CerrarPeriodo.
	PuedeCerrar bool `json:"puede_cerrar"`
}
