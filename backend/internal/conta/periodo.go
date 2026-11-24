package conta

// FASE 4 · Apertura, balance de comprobación y cierre de período.
//
// ── LAS TRES PIEZAS Y PARA QUÉ SIRVE CADA UNA ───────────────────────────────────────────────
//
//  · BALANCE DE COMPROBACIÓN: la prueba de que el libro está sano. Saldo anterior + movimientos
//    del mes = saldo final, cuenta por cuenta, y los totales cuadran en cero. Es lo que se mira
//    ANTES de emitir cualquier estado financiero.
//
//  · APERTURA: los saldos con los que arranca la contabilidad. Sin ellos, el balance muestra solo
//    lo que pasó desde que el sistema empezó a registrar, y el activo no es el activo.
//
//  · CIERRE DE PERÍODO: congelar un mes ya publicado. Si el saldo de agosto se emitió y después
//    entra un asiento con fecha de agosto, dos personas con el mismo reporte ven números
//    distintos y ninguna de las dos sabe cuál es.

import (
	"errors"
	"strings"
	"time"
)

// Estados del período contable.
const (
	PeriodoAbierto = "ABIERTO"
	PeriodoCerrado = "CERRADO"
)

// PeriodoContable es el estado del libro en un mes.
type PeriodoContable struct {
	Anio   int    `json:"anio"`
	Mes    int    `json:"mes"`
	Estado string `json:"estado"`

	CerradoEn        *time.Time `json:"cerrado_en"`
	CerradoPorNombre string     `json:"cerrado_por_nombre"`
	// Lo que tenía el libro al cerrar. Si el mes se reabre y se vuelve a cerrar, comparar estos
	// números dice qué cambió.
	DebeAlCierre     string `json:"debe_al_cierre"`
	HaberAlCierre    string `json:"haber_al_cierre"`
	AsientosAlCierre int    `json:"asientos_al_cierre"`

	ReabiertoEn     *time.Time `json:"reabierto_en"`
	ReabiertoMotivo string     `json:"reabierto_motivo"`

	// Asientos y montos que tiene HOY el período, esté cerrado o no.
	Asientos int    `json:"asientos"`
	Debe     string `json:"debe"`
	Haber    string `json:"haber"`
	// Cuadra: debe = haber. Un período que no cuadra no debería cerrarse.
	Cuadra bool `json:"cuadra"`
}

// RenglonBalance es una cuenta en el balance de comprobación.
type RenglonBalance struct {
	CuentaID   string `json:"cuenta_id"`
	Codigo     string `json:"codigo"`
	Nombre     string `json:"nombre"`
	Clase      string `json:"clase"`
	Naturaleza string `json:"naturaleza"`

	// SaldoAnterior: lo acumulado ANTES del rango. Sin esta columna el balance de un mes
	// arranca en cero y su saldo final es el del mes, no el de la cuenta.
	SaldoAnteriorDebe  string `json:"saldo_anterior_debe"`
	SaldoAnteriorHaber string `json:"saldo_anterior_haber"`
	// Movimientos del rango.
	Debe  string `json:"debe"`
	Haber string `json:"haber"`
	// SaldoFinal, repartido en la columna que corresponde a su signo. Una cuenta no tiene saldo
	// «deudor de −500»: tiene saldo acreedor de 500, y mostrarlo al revés obliga a traducir
	// mentalmente cada fila.
	SaldoFinalDebe  string `json:"saldo_final_debe"`
	SaldoFinalHaber string `json:"saldo_final_haber"`
}

// BalanceComprobacion es el balance completo con sus totales.
type BalanceComprobacion struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`

	Renglones []RenglonBalance `json:"renglones"`

	TotalAnteriorDebe  string `json:"total_anterior_debe"`
	TotalAnteriorHaber string `json:"total_anterior_haber"`
	TotalDebe          string `json:"total_debe"`
	TotalHaber         string `json:"total_haber"`
	TotalFinalDebe     string `json:"total_final_debe"`
	TotalFinalHaber    string `json:"total_final_haber"`

	// CUADRA ES EL PUNTO DEL REPORTE. Los tres pares de totales tienen que ser iguales entre sí;
	// si uno no lo es, el libro tiene un problema y ningún estado financiero sirve.
	Cuadra bool `json:"cuadra"`
	// Diferencia, cuando no cuadra: decir «no cuadra» sin el monto obliga a restarlo a mano.
	Diferencia string `json:"diferencia,omitempty"`
	// SinApertura avisa que no hay saldos iniciales cargados: el balance muestra solo lo que pasó
	// desde que el sistema registra, y el activo no es el activo.
	SinApertura bool   `json:"sin_apertura"`
	Aviso       string `json:"aviso,omitempty"`
}

// FilaApertura es una línea del Excel de saldos iniciales.
type FilaApertura struct {
	Fila     int    `json:"fila"`
	Codigo   string `json:"codigo"`
	Cuenta   string `json:"cuenta"`
	Debe     string `json:"debe"`
	Haber    string `json:"haber"`
	Problema string `json:"problema"`
}

// PrevisualizacionApertura es lo que pasaría al cargar los saldos iniciales.
type PrevisualizacionApertura struct {
	Fecha string         `json:"fecha"`
	Filas []FilaApertura `json:"filas"`
	// Cuentas que entran y su suma.
	Cuentas    int    `json:"cuentas"`
	TotalDebe  string `json:"total_debe"`
	TotalHaber string `json:"total_haber"`
	// CUADRA: un asiento de apertura descuadrado no se puede escribir —lo impide la base— así
	// que conviene decirlo antes de intentarlo, con la diferencia.
	Cuadra      bool   `json:"cuadra"`
	Diferencia  string `json:"diferencia,omitempty"`
	ConProblema int    `json:"con_problema"`
	// YaExiste: ya hay un asiento de apertura. Cargar otro exige anular el anterior.
	YaExiste bool   `json:"ya_existe"`
	Aviso    string `json:"aviso,omitempty"`
}

var (
	// ErrPeriodoCerradoYa: cerrar dos veces no cierra más.
	ErrPeriodoCerradoYa = errors.New("conta: ese período ya está cerrado")
	// ErrPeriodoNoCerrado: no se reabre lo que nunca se cerró.
	ErrPeriodoNoCerrado = errors.New("conta: ese período no está cerrado")
	// ErrPeriodoNoCuadra: cerrar un mes descuadrado congelaría el problema.
	ErrPeriodoNoCuadra = errors.New("conta: el período no cuadra: el debe y el haber no dan lo mismo, y cerrarlo así dejaría el error congelado")
	// ErrMotivoReaperturaRequerido: reabrir es la pregunta que un auditor hace primero.
	ErrMotivoReaperturaRequerido = errors.New("conta: decí por qué se reabre este período")
	// ErrAperturaYaExiste: dos aperturas serían dos puntos de partida.
	ErrAperturaYaExiste = errors.New("conta: ya hay un asiento de apertura; para cargar otro, anulá el anterior desde el libro diario")
	// ErrAperturaDescuadrada: el activo inicial tiene que igualar al pasivo más el patrimonio.
	ErrAperturaDescuadrada = errors.New("conta: los saldos iniciales no cuadran: el total del debe y el del haber tienen que dar lo mismo")
	// ErrAperturaVacia: no hay ninguna fila utilizable.
	ErrAperturaVacia = errors.New("conta: el archivo no trae ningún saldo que se pueda cargar")
	// ErrPeriodoEstaCerrado: se intentó tocar un mes congelado. Distinto de ErrPeriodoCerradoYa,
	// que es «cerrar dos veces»: este es «no se puede escribir acá».
	ErrPeriodoEstaCerrado = errors.New("conta: ese período está cerrado: para corregirlo hay que reabrirlo, y eso queda registrado")
)

// periodoCerrado lleva el texto del disparador —que nombra el MES— y es reconocible con
// errors.Is. Mismo patrón que deAgrupacion: el mensaje sirve a la persona y la identidad al
// handler, y quedarse con uno solo arruina al otro.
type periodoCerrado struct{ msg string }

func (e periodoCerrado) Error() string { return "conta: " + e.msg }
func (e periodoCerrado) Unwrap() error { return ErrPeriodoEstaCerrado }

// motivoReaperturaMinimo es el mismo mínimo que el resto del sistema. Reabrir un período es de
// las decisiones más visibles que hay: el «ok» reflejo no explica nada a quien pregunte después.
const motivoReaperturaMinimo = motivoMinimo

// validarMotivoReapertura comprueba que haya una explicación de verdad.
func validarMotivoReapertura(m string) (string, error) {
	m = strings.TrimSpace(m)
	if len(m) < motivoReaperturaMinimo {
		return "", ErrMotivoReaperturaRequerido
	}
	return m, nil
}

// partesDeFecha saca año y mes de «2006-01-02». El llamador ya validó la forma.
func partesDeFecha(fecha string) (int, int) {
	t, err := time.Parse("2006-01-02", fecha)
	if err != nil {
		return 0, 0
	}
	return t.Year(), int(t.Month())
}

// partesDePeriodo saca año y mes de «YYYY-MM». El llamador ya validó la forma con esPeriodo.
func partesDePeriodo(periodo string) (int, int) {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return 0, 0
	}
	return t.Year(), int(t.Month())
}

// montoLegible pone separadores de miles a un monto que ya viene como «1234567.89».
//
// Los avisos los lee una persona, y «sobran 1234567.89 en el debe» obliga a contar dígitos para
// saber si son millones. El paquete `bancos` tiene el suyo; duplicar diez líneas es preferible a
// que dos módulos dependan uno del otro por un formato.
func montoLegible(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	entero, decimales := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		entero, decimales = s[:i], s[i+1:]
	}
	var b strings.Builder
	for i, r := range entero {
		if i > 0 && (len(entero)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := "₡" + b.String()
	if decimales != "" {
		out += "," + decimales
	}
	// El signo va DELANTE del símbolo: «−₡1.000,50» y no «₡−1.000,50», que se lee como si el
	// símbolo fuera parte del número.
	if neg {
		out = "-" + out
	}
	return out
}

// finDeMesDe devuelve el último día del mes de un período «YYYY-MM».
//
// Es la fecha natural del asiento de apertura y del corte de un balance: los saldos iniciales son
// los del CIERRE del mes anterior, no los de un día cualquiera.
func finDeMesDe(periodo string) string {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 1, -1).Format("2006-01-02")
}
