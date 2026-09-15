package cxp

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// ── RESPONSABILIDADES MENSUALES ────────────────────────────────────────────────────────────────
//
// Todo el resto del ERP conoce LO QUE LLEGÓ. Esto es lo primero que declara LO QUE SE ESPERA.
//
// El acuerdo (`responsabilidad_cxp`) se declara una vez y dura años; cada mes produce un período
// (`responsabilidad_periodo`) que alguien tiene que cerrar. El molde no se inventó acá: es el
// espejo de `cargo_cxc` del lado de cobrar, donde `UNIQUE (contrato_id, periodo)` hace que generar
// dos veces el mismo mes no duplique nada.
//
// Ver migración 0082 para el porqué medido (≈₡28 M/mes saliendo del banco sin factura).

// Estados del acuerdo. Nunca se borra: un acuerdo borrado se lleva con él la explicación de los
// meses que ya cerró.
const (
	RespActiva     = "ACTIVA"
	RespSuspendida = "SUSPENDIDA"
	RespFinalizada = "FINALIZADA"
)

// Estados de un período (lo que se guarda). El semáforo que ve el usuario NO es esto: se calcula
// al mirar, porque depende de la fecha de hoy y de hasta dónde está cargado el banco.
const (
	PerPendiente = "PENDIENTE"
	PerCumplida  = "CUMPLIDA"
	PerNoAplica  = "NO_APLICA"
)

// Cómo se probó el cumplimiento.
const (
	PruebaFactura    = "FACTURA"
	PruebaMovimiento = "MOVIMIENTO"
	PruebaAcuse      = "ACUSE"
)

// Semáforo: lo que ve la persona. Se deriva, no se almacena.
const (
	// SemCumplida / SemNoAplica: el mes está resuelto.
	SemCumplida = "CUMPLIDA"
	SemNoAplica = "NO_APLICA"
	// SemSinDato es la guarda de honestidad. Si el banco no está importado hasta el día de
	// vencimiento, el sistema NO puede afirmar que algo está vencido: pudo haberse pagado por
	// débito y todavía no lo sabemos. Un tablero que grita «VENCIDA» en falso se deja de mirar en
	// la primera semana, y después ya no hay forma de recuperar su autoridad.
	SemSinDato = "SIN_DATO"
	// SemVencida: pasó la fecha y sigue pendiente, y el banco SÍ alcanza para afirmarlo.
	SemVencida = "VENCIDA"
	// SemPorVencer: vence dentro de los próximos DiasPorVencer días.
	SemPorVencer = "POR_VENCER"
	// SemAlDia: todavía falta.
	SemAlDia = "AL_DIA"
	// SemSinAbrir: la responsabilidad existe y el mes NO tiene fila. Es el estado que atrapa el
	// olvido del olvido — nadie abrió el mes, así que nada aparece vencido y todo parece en orden.
	SemSinAbrir = "SIN_ABRIR"
)

// DiasPorVencer es la ventana de aviso temprano: dentro de estos días una obligación pasa de
// «al día» a «por vencer». Una semana es lo que tarda una transferencia en gestionarse sin correr.
const DiasPorVencer = 7

// Periodicidades admitidas y cada cuántos meses cae la obligación.
var pasoDePeriodicidad = map[string]int{
	"MENSUAL":    1,
	"BIMENSUAL":  2,
	"TRIMESTRAL": 3,
	"SEMESTRAL":  6,
	"ANUAL":      12,
}

var (
	// ErrResponsabilidadNoEncontrada indica que el acuerdo no existe en esta empresa.
	ErrResponsabilidadNoEncontrada = errors.New("cxp: la responsabilidad no existe")
	// ErrPeriodoNoEncontrado indica que ese mes no existe para esa responsabilidad.
	ErrPeriodoNoEncontrado = errors.New("cxp: el período no existe")
	// (El período con formato equivocado usa `ErrPeriodoInvalido`, que ya existe en dashboard.go
	// con exactamente el mismo significado: no hay razón para tener dos.)

	// ErrPeriodicidadInvalida indica una periodicidad fuera de la lista admitida.
	ErrPeriodicidadInvalida = errors.New("cxp: periodicidad no admitida")
	// ErrMotivoObligatorio indica que se quiso marcar «no aplica» sin escribir por qué. Sin este
	// freno, «no aplica» se vuelve el botón de tapar el olvido, que es justo lo que esto impide.
	ErrMotivoObligatorio = errors.New("cxp: marcar «no aplica» exige escribir el motivo")
	// ErrPruebaObligatoria indica que se quiso cerrar un mes sin decir con qué se cumplió.
	ErrPruebaObligatoria = errors.New("cxp: cerrar un mes exige la factura, el movimiento o el acuse")
	// ErrPeriodoYaCerrado indica que ese mes ya se resolvió.
	ErrPeriodoYaCerrado = errors.New("cxp: ese mes ya está resuelto")
	// ErrPruebaYaUsada indica que esa factura o ese movimiento ya cerró otra responsabilidad.
	// Sin esto, el alquiler y la póliza podrían «cumplirse» las dos con el mismo débito y el mes
	// cerraría en verde con una sola cosa pagada.
	ErrPruebaYaUsada = errors.New("cxp: esa factura o ese movimiento ya cerró otra responsabilidad")
	// ErrSinComprobanteNoDeducible indica que se marcó deducible un gasto sin respaldo. En Costa
	// Rica un gasto sin comprobante no lo acepta Hacienda.
	ErrSinComprobanteNoDeducible = errors.New("cxp: sin comprobante el gasto no puede ser deducible")
	// ErrRespaldoSinArchivo indica que se declaró un respaldo documental sin adjuntarlo.
	ErrRespaldoSinArchivo = errors.New("cxp: si hay contrato, acta o correo, el archivo va adjunto")
)

// Responsabilidad es el acuerdo: se declara una vez y dura años.
type Responsabilidad struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Contraparte string `json:"contraparte"`
	// ProveedorID es OPCIONAL a propósito. El arrendante de palabra no está en el maestro de
	// proveedores, y exigirlo reproduciría el mismo candado de la clave de Hacienda que es la
	// causa de que ≈₡28 M mensuales salgan del banco sin registro.
	ProveedorID     string `json:"proveedor_id,omitempty"`
	ProveedorNombre string `json:"proveedor_nombre,omitempty"`

	Tipo           string `json:"tipo"`
	Periodicidad   string `json:"periodicidad"`
	DiaVencimiento int    `json:"dia_vencimiento"`
	MesAncla       int    `json:"mes_ancla,omitempty"`

	Moneda string `json:"moneda"`
	// MontoEsperado viaja como texto: es dinero, y un número en coma flotante en el JSON pierde
	// centavos en el camino.
	MontoEsperado string `json:"monto_esperado"`
	MontoTipo     string `json:"monto_tipo"`

	RespaldoTipo    string `json:"respaldo_tipo"`
	RespaldoArchivo string `json:"respaldo_archivo,omitempty"`
	EsperaFactura   bool   `json:"espera_factura"`
	Deducible       bool   `json:"deducible"`

	ClasificacionID     string `json:"clasificacion_id,omitempty"`
	ClasificacionNombre string `json:"clasificacion_nombre,omitempty"`
	DepartamentoID      string `json:"departamento_id,omitempty"`
	DepartamentoNombre  string `json:"departamento_nombre,omitempty"`

	Estado       string `json:"estado"`
	MotivoEstado string `json:"motivo_estado,omitempty"`
	Notas        string `json:"notas,omitempty"`

	TitularID      string `json:"titular_id,omitempty"`
	TitularNombre  string `json:"titular_nombre,omitempty"`
	SuplenteID     string `json:"suplente_id,omitempty"`
	SuplenteNombre string `json:"suplente_nombre,omitempty"`

	CreadoEn string `json:"creado_en"`
}

// PeriodoResponsabilidad es el mes: una fila por responsabilidad y período.
type PeriodoResponsabilidad struct {
	ID                string `json:"id"`
	ResponsabilidadID string `json:"responsabilidad_id"`
	Nombre            string `json:"nombre"`
	Contraparte       string `json:"contraparte"`
	Periodo           string `json:"periodo"`
	VenceEn           string `json:"vence_en"`
	MontoEsperado     string `json:"monto_esperado"`
	Moneda            string `json:"moneda"`
	MontoTipo         string `json:"monto_tipo"`
	Estado            string `json:"estado"`
	CumplidaCon       string `json:"cumplida_con,omitempty"`
	DocumentoID       string `json:"documento_id,omitempty"`
	MovimientoID      string `json:"movimiento_id,omitempty"`
	AcuseArchivo      string `json:"acuse_archivo,omitempty"`
	Motivo            string `json:"motivo,omitempty"`
	RespaldoTipo      string `json:"respaldo_tipo,omitempty"`
	Deducible         bool   `json:"deducible"`
	TitularNombre     string `json:"titular_nombre,omitempty"`
	SuplenteNombre    string `json:"suplente_nombre,omitempty"`
	CerradoPorNombre  string `json:"cerrado_por_nombre,omitempty"`
	CerradoEn         string `json:"cerrado_en,omitempty"`
	// Semaforo se calcula al leer; nunca se guarda.
	Semaforo string `json:"semaforo"`
	// DiasDeAtraso es positivo cuando ya venció. Se manda calculado para que la pantalla no tenga
	// que hacer aritmética de fechas en el navegador, donde la zona horaria la puede correr un día.
	DiasDeAtraso int `json:"dias_de_atraso"`
}

// FechaDeVencimiento resuelve en qué día cae un período.
//
// LA TRAMPA: un acuerdo que vence «el 31» existe (varios proveedores cobran a fin de mes), pero
// febrero no tiene 31. Si esto se dejara en manos de time.Date, el 31 de febrero se normalizaría
// al 3 de marzo y la obligación aparecería venciendo en OTRO MES — con lo cual el mes de febrero
// nunca tendría su fila y el olvido quedaría invisible, que es exactamente lo contrario de para
// lo que existe este módulo. Acá se corta al último día del mes.
func FechaDeVencimiento(periodo string, dia int) (time.Time, error) {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrPeriodoInvalido, periodo)
	}
	if dia < 1 {
		dia = 1
	}
	// El día 0 del mes siguiente ES el último día de este mes.
	ultimo := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if dia > ultimo {
		dia = ultimo
	}
	return time.Date(t.Year(), t.Month(), dia, 0, 0, 0, 0, time.UTC), nil
}

// AplicaEnPeriodo dice si una responsabilidad cae en ese mes.
//
// Una mensual cae siempre. Una trimestral con ancla en marzo cae en marzo, junio, setiembre y
// diciembre — nunca en los demás. Si esto se equivocara, el sistema abriría meses que no
// corresponden y la lista se llenaría de obligaciones falsas que alguien tendría que ir marcando
// «no aplica» una por una, hasta que deje de mirarla.
func AplicaEnPeriodo(periodicidad string, mesAncla int, periodo string) (bool, error) {
	paso, ok := pasoDePeriodicidad[periodicidad]
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrPeriodicidadInvalida, periodicidad)
	}
	if paso == 1 {
		return true, nil
	}
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return false, fmt.Errorf("%w: %q", ErrPeriodoInvalido, periodo)
	}
	if mesAncla < 1 || mesAncla > 12 {
		return false, fmt.Errorf("%w: una %s necesita mes de ancla", ErrPeriodicidadInvalida, periodicidad)
	}
	// Módulo siempre positivo: en Go, (2-11)%3 da -0 o negativo según el caso, y una resta que
	// cruza el fin de año no puede cambiar la respuesta.
	diff := ((int(t.Month())-mesAncla)%paso + paso) % paso
	return diff == 0, nil
}

// Semaforo traduce el estado guardado al estado que ve la persona.
//
// `bancoHasta` es la última fecha hasta la que están importados los movimientos del banco (vacía
// si no hay ninguno). Es lo que separa «esto está vencido» de «todavía no puedo saberlo».
func Semaforo(estado, venceEn, hoy, bancoHasta string) string {
	switch estado {
	case PerCumplida:
		return SemCumplida
	case PerNoAplica:
		return SemNoAplica
	}
	vence, err := time.Parse("2006-01-02", venceEn)
	if err != nil {
		return SemSinDato
	}
	dHoy, err := time.Parse("2006-01-02", hoy)
	if err != nil {
		return SemSinDato
	}
	if dHoy.After(vence) {
		// Ya pasó la fecha. Antes de gritar «vencida», hay que poder afirmarlo: si el banco no
		// llega hasta el día de vencimiento, el pago pudo haber salido y no lo sabemos.
		banco, err := time.Parse("2006-01-02", bancoHasta)
		if err != nil || banco.Before(vence) {
			return SemSinDato
		}
		return SemVencida
	}
	if !vence.After(dHoy.AddDate(0, 0, DiasPorVencer)) {
		return SemPorVencer
	}
	return SemAlDia
}

// DiasDeAtraso devuelve cuántos días pasaron desde el vencimiento (negativo si todavía falta).
func DiasDeAtraso(venceEn, hoy string) int {
	vence, err := time.Parse("2006-01-02", venceEn)
	if err != nil {
		return 0
	}
	dHoy, err := time.Parse("2006-01-02", hoy)
	if err != nil {
		return 0
	}
	return int(dHoy.Sub(vence).Hours() / 24)
}

// ValidarRespaldo aplica las dos reglas fiscales que el Director fijó el 13-set-2026, en Go además
// de en la base: la base es el último freno, pero el mensaje que lee la persona sale de acá.
func ValidarRespaldo(respaldoTipo, respaldoArchivo string, deducible bool) error {
	switch respaldoTipo {
	case "VERBAL", "NINGUNO":
		if deducible {
			return ErrSinComprobanteNoDeducible
		}
	case "CONTRATO", "ACTA", "CORREO":
		if respaldoArchivo == "" {
			return ErrRespaldoSinArchivo
		}
	}
	return nil
}

// ResumenDelMes es el encabezado de la pantalla «El mes».
type ResumenDelMes struct {
	Periodo string `json:"periodo"`
	// SinAbrir cuenta las responsabilidades ACTIVAS que corresponden a este mes y NO tienen fila.
	// Es el número que delata que nadie abrió el mes.
	SinAbrir  int `json:"sin_abrir"`
	Pendiente int `json:"pendiente"`
	PorVencer int `json:"por_vencer"`
	Vencida   int `json:"vencida"`
	SinDato   int `json:"sin_dato"`
	Cumplida  int `json:"cumplida"`
	NoAplica  int `json:"no_aplica"`
	// MontoEsperado y MontoResuelto viajan como texto (dinero).
	MontoEsperado string `json:"monto_esperado"`
	MontoVencido  string `json:"monto_vencido"`
	// BancoHasta dice hasta cuándo alcanzan los datos del banco. La pantalla lo muestra: sin esto,
	// un «SIN DATO» parece un error del sistema en vez de una carga pendiente.
	BancoHasta string `json:"banco_hasta,omitempty"`
}

// ResumirMes arma el encabezado a partir de los períodos ya calculados y de cuántas quedaron sin
// abrir. Se separa del repositorio para poder probarlo sin base de datos.
func ResumirMes(periodo string, filas []PeriodoResponsabilidad, sinAbrir int, bancoHasta string) ResumenDelMes {
	r := ResumenDelMes{Periodo: periodo, SinAbrir: sinAbrir, BancoHasta: bancoHasta}
	esperado, vencido := decimal.Zero, decimal.Zero
	for _, f := range filas {
		monto, err := decimal.NewFromString(f.MontoEsperado)
		if err != nil {
			monto = decimal.Zero
		}
		// El esperado del mes NO incluye lo que ya se declaró que no aplica: sumarlo inflaría la
		// proyección de salida con plata que nadie va a pagar.
		if f.Estado != PerNoAplica {
			esperado = esperado.Add(monto)
		}
		switch f.Semaforo {
		case SemCumplida:
			r.Cumplida++
		case SemNoAplica:
			r.NoAplica++
		case SemVencida:
			r.Vencida++
			r.Pendiente++
			vencido = vencido.Add(monto)
		case SemSinDato:
			r.SinDato++
			r.Pendiente++
		case SemPorVencer:
			r.PorVencer++
			r.Pendiente++
		case SemAlDia:
			r.Pendiente++
		}
	}
	r.MontoEsperado = esperado.StringFixed(2)
	r.MontoVencido = vencido.StringFixed(2)
	return r
}
