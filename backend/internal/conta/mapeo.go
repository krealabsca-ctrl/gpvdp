package conta

// El mapeo: qué cuenta contable corresponde a cada partida del ERP (mig 0110, Fase 2).
//
// SIGUE SIN CONTABILIZAR NADA. Esta fase construye la equivalencia y mide cuánto falta; generar
// asientos es la Fase 3. El valor de terminarla es que, al hacerlo, se sabe con un número si la
// contabilidad se puede generar o no.

import (
	"errors"
	"strings"
	"time"
)

// Estados de un mapeo.
const (
	// EstadoMapeada: la partida tiene su cuenta contable.
	EstadoMapeada = "MAPEADA"
	// EstadoNoContabilizable: la partida NO produce asiento, y hay un motivo escrito. No es lo
	// mismo que «falta decidir»: es una decisión tomada.
	EstadoNoContabilizable = "NO_CONTABILIZABLE"
)

// Naturalezas que el ERP declara en el concepto. Son las del módulo de Bancos.
const (
	NatIngreso = "INGRESO"
	NatGasto   = "GASTO"
	NatNeutro  = "NEUTRO"
)

// Mapeo es la decisión tomada sobre una clasificación.
type Mapeo struct {
	ID                string    `json:"id"`
	ClasificacionID   string    `json:"clasificacion_id"`
	CuentaID          *string   `json:"cuenta_id"`
	Estado            string    `json:"estado"`
	Motivo            string    `json:"motivo"`
	DecididoEn        time.Time `json:"decidido_en"`
	DecididoPorNombre string    `json:"decidido_por_nombre"`
}

// FilaDelMapeo es una clasificación con su decisión, si la hay. Es lo que la pantalla lista.
type FilaDelMapeo struct {
	ClasificacionID string `json:"clasificacion_id"`
	Concepto        string `json:"concepto"`
	Clasificacion   string `json:"clasificacion"`
	// NaturalezaConcepto es lo que el ERP declara: INGRESO, GASTO o NEUTRO.
	//
	// OJO CON LEERLA SOLA: la columna nunca viene vacía —su default es NEUTRO—, así que «NEUTRO»
	// no significa «se decidió que es neutro». Lo que distingue las dos cosas es
	// `NaturalezaDeclarada`, y hoy está en falso en 58 de los 64 conceptos de la base (26-oct-2026).
	// Por eso el detector de cruce calla cuando no está declarada: opinar sobre un NEUTRO que
	// nadie eligió llenaría la pantalla de avisos falsos.
	NaturalezaConcepto  string `json:"naturaleza_concepto"`
	NaturalezaDeclarada bool   `json:"naturaleza_declarada"`
	// Movimientos dice cuántos movimientos vivos usan esta clasificación. Es lo que ordena el
	// trabajo: mapear primero lo que más se usa.
	Movimientos int `json:"movimientos"`

	Estado       string  `json:"estado"` // "" = falta decidir
	Motivo       string  `json:"motivo"`
	CuentaID     *string `json:"cuenta_id"`
	CuentaCodigo string  `json:"cuenta_codigo"`
	CuentaNombre string  `json:"cuenta_nombre"`
	CuentaClase  string  `json:"cuenta_clase"`

	// Cruzada: la naturaleza que el ERP declara no concuerda con la clase de la cuenta elegida.
	// Ver NaturalezaCruzada.
	Cruzada    bool   `json:"cruzada"`
	AvisoCruce string `json:"aviso_cruce"`
}

// Cobertura es el reporte que dice cuánto falta. Es el entregable que permite decidir con datos
// si se puede pasar a la Fase 3.
type Cobertura struct {
	// ConMovimientos es el denominador que importa: una clasificación que nadie usó no bloquea
	// nada, y contarla haría que el porcentaje nunca llegara al 100 %.
	ConMovimientos    int `json:"con_movimientos"`
	Mapeadas          int `json:"mapeadas"`
	NoContabilizables int `json:"no_contabilizables"`
	SinDecidir        int `json:"sin_decidir"`
	// Cruzadas NO comparte denominador con los tres de arriba: se cuenta sobre el TOTAL, porque
	// un cruce en una partida que todavía nadie usó sigue siendo un cruce y va a doler igual el
	// día que se use. Solo una partida ya mapeada puede estar cruzada —sin cuenta no hay con qué
	// comparar—, así que el conjunto en el que vive es «las mapeadas», y la pantalla lo rotula
	// así. Medido el 26-oct-2026 en Valle de Paz: 2 cruzadas con 1 sola mapeada entre las usadas.
	Cruzadas int `json:"cruzadas"`
	// SinNaturaleza son los conceptos que no declararon si son ingreso o gasto. Sin eso, el
	// detector de cruce no puede opinar sobre sus clasificaciones.
	SinNaturaleza int `json:"sin_naturaleza"`
	// Total incluye las que nadie usó todavía, para no esconder que existen.
	Total int `json:"total"`
}

// Listas devuelve el porcentaje resuelto sobre lo que de verdad bloquea.
func (c Cobertura) PorcentajeResuelto() int {
	if c.ConMovimientos == 0 {
		return 100
	}
	return (c.Mapeadas + c.NoContabilizables) * 100 / c.ConMovimientos
}

var (
	// ErrMotivoRequerido: declarar que algo no se contabiliza exige decir por qué.
	ErrMotivoRequerido = errors.New("conta: decí por qué esta partida no se contabiliza")
	// ErrCuentaRequerida: un mapeo sin cuenta no es un mapeo.
	ErrCuentaRequerida = errors.New("conta: elegí la cuenta contable")
	// ErrEstadoInvalido: solo hay dos decisiones posibles.
	ErrEstadoInvalido = errors.New("conta: la decisión tiene que ser «mapeada» o «no contabilizable»")
	// ErrCuentaDeAgrupacion: no se mapea a una cuenta que agrupa a otras.
	ErrCuentaDeAgrupacion = errors.New("conta: esa cuenta agrupa a otras, así que no admite asientos: elegí una cuenta de movimiento")
	// ErrMapeoNoEncontrado: no hay decisión tomada sobre esa clasificación.
	ErrMapeoNoEncontrado = errors.New("conta: esa partida todavía no tiene decisión")
)

// motivoMinimo descarta el «ok» reflejo. Mismo criterio que la liberación de facturas.
const motivoMinimo = 15

// DecisionDeMapeo es lo que llega desde la pantalla.
type DecisionDeMapeo struct {
	ClasificacionID string `json:"clasificacion_id"`
	CuentaID        string `json:"cuenta_id"`
	Estado          string `json:"estado"`
	Motivo          string `json:"motivo"`
}

// Validar comprueba la invariante antes de que viaje: o cuenta, o motivo.
func (d *DecisionDeMapeo) Validar() error {
	d.Motivo = strings.TrimSpace(d.Motivo)
	d.CuentaID = strings.TrimSpace(d.CuentaID)
	switch d.Estado {
	case EstadoMapeada:
		if d.CuentaID == "" {
			return ErrCuentaRequerida
		}
		// El motivo no estorba en una decisión positiva: a veces explica por qué ESA cuenta y no
		// otra, que es justo lo que hace falta en los casos raros.
		return nil
	case EstadoNoContabilizable:
		if len(d.Motivo) < motivoMinimo {
			return ErrMotivoRequerido
		}
		d.CuentaID = ""
		return nil
	default:
		return ErrEstadoInvalido
	}
}

// NaturalezaCruzada dice si lo que el ERP declara choca con la clase de la cuenta elegida.
//
// ── POR QUÉ ESTO EXISTE ─────────────────────────────────────────────────────────────────────
//
// Medido sobre el catálogo real de Valle de Paz (23-oct-2026): 23 clasificaciones «Caja Chica»
// cuelgan del concepto GASTOS en el ERP, y en el plan contable la caja chica es ACTIVO
// (1-01-01-001-xxx, 15 cuentas). Reponer una caja chica NO es un gasto: es mover plata de Banco a
// Caja Chica. El gasto nace al liquidar los vales.
//
// Mapear esas 23 a una cuenta de gasto infla el estado de resultados y deja la caja chica fuera
// del balance, y el error no se nota hasta que el balance no cuadra —meses después—.
//
// ── POR QUÉ AVISA Y NO BLOQUEA ──────────────────────────────────────────────────────────────
//
// Porque el cruce NO siempre es un error. El caso de la caja chica es legítimo al revés: la
// clasificación está en «Gastos» porque responde «¿a dónde se fue la plata del banco?», y la
// cuenta es de activo porque responde «¿qué tengo?». Las dos tienen razón. Lo que no puede pasar
// es que nadie lo mire. Esto lo pone sobre la mesa; decidir es del contador.
func NaturalezaCruzada(naturalezaConcepto string, declarada bool, claseCuenta string) (bool, string) {
	if !declarada || naturalezaConcepto == "" || claseCuenta == "" {
		return false, ""
	}
	switch naturalezaConcepto {
	case NatGasto:
		// Gasto contra activo o pasivo: es el caso de la caja chica y el de pagar un préstamo.
		switch claseCuenta {
		case ClaseGasto, ClaseCosto:
			return false, ""
		case ClaseActivo:
			return true, "en el ERP es GASTO y la cuenta es de ACTIVO: revisá si es un gasto o si es mover plata de un lado a otro (una reposición de caja chica, por ejemplo, no es gasto)"
		case ClasePasivo:
			return true, "en el ERP es GASTO y la cuenta es de PASIVO: pagar una deuda no es un gasto, baja el pasivo"
		default:
			return true, "en el ERP es GASTO y la cuenta es de " + claseCuenta
		}
	case NatIngreso:
		switch claseCuenta {
		case ClaseIngreso:
			return false, ""
		case ClasePasivo:
			return true, "en el ERP es INGRESO y la cuenta es de PASIVO: un cobro por adelantado no es ingreso todavía"
		case ClaseActivo:
			return true, "en el ERP es INGRESO y la cuenta es de ACTIVO: cobrar una cuenta pendiente no genera ingreso, lo generó la venta"
		default:
			return true, "en el ERP es INGRESO y la cuenta es de " + claseCuenta
		}
	case NatNeutro:
		// Neutro contra resultado: un traslado entre cuentas propias no puede ser ingreso ni gasto.
		switch claseCuenta {
		case ClaseIngreso, ClaseGasto, ClaseCosto:
			return true, "en el ERP es NEUTRO (no afecta el resultado) y la cuenta es de " + claseCuenta + ": una de las dos cosas está mal"
		default:
			return false, ""
		}
	}
	return false, ""
}
