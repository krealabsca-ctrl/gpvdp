package conta

// El asiento contable (mig 0111, Fase 3A).
//
// ── LO QUE ESTA FASE HACE Y LO QUE NO ───────────────────────────────────────────────────────
//
// HACE: el lugar donde viven los asientos, con partida doble imposible de violar; el enlace
// cuenta bancaria → cuenta contable, que es la contrapartida que le faltaba al mapeo de la Fase
// 2; la idempotencia por documento; y los libros diario y mayor.
//
// NO HACE: generar asientos. De qué documento sale cada asiento y en qué dirección lo decide la
// matriz de reglas de contabilización (A3 de la propuesta), que es criterio contable y no está
// tomada. Todo lo de acá es cierto bajo cualquier respuesta a esa pregunta: se contabilice por
// caja o por devengo, el asiento tiene que cuadrar, el banco tiene que tener su cuenta y el
// diario tiene que leerse.

import (
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Estados del asiento.
const (
	// EstadoBorrador: existe y ya cuadra, pero todavía no es parte del libro.
	EstadoBorrador = "BORRADOR"
	// EstadoAsentado: está en el libro diario.
	EstadoAsentado = "ASENTADO"
	// EstadoAnulado: se dejó sin efecto. NO se borra: borrar deja un hueco en la numeración y un
	// mayor que ya no se puede reconstruir.
	EstadoAnulado = "ANULADO"
)

// Orígenes posibles. Son los documentos que HOY existen en el sistema; cuál de ellos genera
// asiento lo decide A3.
const (
	OrigenManual     = "MANUAL"
	OrigenMovimiento = "MOVIMIENTO_BANCARIO"
	OrigenCxPFactura = "CXP_FACTURA"
	OrigenCxPPago    = "CXP_PAGO"
	OrigenCajaChica  = "CAJA_CHICA_VALE"
	OrigenNomina     = "NOMINA"
	OrigenCxCCargo   = "CXC_CARGO"
	OrigenCxCCobro   = "CXC_COBRO"
	OrigenApertura   = "APERTURA"
)

// Asiento es la cabecera.
type Asiento struct {
	ID        string `json:"id"`
	EmpresaID string `json:"empresa_id"`
	Numero    int64  `json:"numero"`
	// Fecha va como texto «2006-01-02»: es una fecha contable, no un instante, y pasarla por
	// time.Time la arrastra a una zona horaria que puede moverla de día.
	Fecha string `json:"fecha"`
	Glosa string `json:"glosa"`

	OrigenTipo string  `json:"origen_tipo"`
	OrigenID   *string `json:"origen_id"`

	Estado        string `json:"estado"`
	AnuladoMotivo string `json:"anulado_motivo"`

	CreadoPorNombre string    `json:"creado_por_nombre"`
	CreadoEn        time.Time `json:"creado_en"`

	Lineas []LineaAsiento `json:"lineas,omitempty"`

	// Debe y Haber son los totales del asiento, en colones. Van como string por la regla del
	// proyecto: el dinero nunca viaja como número en JSON.
	Debe  string `json:"debe"`
	Haber string `json:"haber"`
}

// LineaAsiento es una línea: una cuenta, y un monto al debe o al haber.
type LineaAsiento struct {
	ID        string `json:"id"`
	AsientoID string `json:"asiento_id"`
	CuentaID  string `json:"cuenta_id"`
	// El código y el nombre vienen con la línea porque un asiento sin ellos es ilegible: nadie
	// lee un mayor por UUID.
	CuentaCodigo string `json:"cuenta_codigo"`
	CuentaNombre string `json:"cuenta_nombre"`

	Debe  string `json:"debe"`
	Haber string `json:"haber"`

	// MonedaOrigen y MontoOrigen son trazabilidad, no contabilidad: el asiento cuadra en colones.
	MonedaOrigen string `json:"moneda_origen"`
	MontoOrigen  string `json:"monto_origen"`
	TCAplicado   string `json:"tc_aplicado"`

	Detalle string `json:"detalle"`
	Orden   int    `json:"orden"`
}

var (
	// ErrAsientoDescuadrado: el debe y el haber no son iguales.
	ErrAsientoDescuadrado = errors.New("conta: el asiento no cuadra: el debe y el haber tienen que dar lo mismo")
	// ErrAsientoSinLineas: un asiento necesita al menos dos líneas.
	ErrAsientoSinLineas = errors.New("conta: el asiento necesita al menos una línea al debe y otra al haber")
	// ErrGlosaRequerida: sin glosa el asiento existe pero no se puede explicar.
	ErrGlosaRequerida = errors.New("conta: escribí la glosa: es lo que alguien va a leer en el diario dentro de seis meses")
	// ErrFechaAsientoRequerida: la fecha decide en qué período cae.
	ErrFechaAsientoRequerida = errors.New("conta: el asiento necesita fecha")
	// ErrLineaSinMonto: una línea sin monto no dice nada.
	ErrLineaSinMonto = errors.New("conta: cada línea va al debe o al haber, con un monto mayor que cero")
	// ErrLineaDebeYHaber: una línea con las dos cosas se netea sola y esconde el movimiento.
	ErrLineaDebeYHaber = errors.New("conta: una línea va al debe o al haber, no a los dos")
	// ErrAsientoNoEncontrado: no existe, o es de otra empresa.
	ErrAsientoNoEncontrado = errors.New("conta: ese asiento no existe")
	// ErrAsientoYaAnulado: anular dos veces no es anular más.
	ErrAsientoYaAnulado = errors.New("conta: ese asiento ya está anulado")
	// ErrMotivoAnulacionRequerido: anular es una decisión y queda escrita.
	ErrMotivoAnulacionRequerido = errors.New("conta: decí por qué se anula este asiento")
	// ErrCuentaBancariaNoMapeada: falta la contrapartida del banco.
	ErrCuentaBancariaNoMapeada = errors.New("conta: esa cuenta bancaria todavía no tiene cuenta contable")
	// ErrFechaInvalida: una fecha que Postgres no entiende revienta la consulta y sale como 500.
	// `bancos` tiene la suya; los paquetes no comparten centinelas.
	ErrFechaInvalida = errors.New("conta: la fecha tiene que venir como 2026-10-27")
)

// NuevoAsiento es lo que llega desde la pantalla para crear uno a mano.
type NuevoAsiento struct {
	Fecha  string       `json:"fecha"`
	Glosa  string       `json:"glosa"`
	Lineas []NuevaLinea `json:"lineas"`
}

// NuevaLinea es una línea pedida. Los montos llegan como texto y se convierten a decimal: nunca
// float64 para dinero.
type NuevaLinea struct {
	CuentaID string `json:"cuenta_id"`
	Debe     string `json:"debe"`
	Haber    string `json:"haber"`
	Detalle  string `json:"detalle"`
}

// LineaValidada es una línea ya convertida a decimal, lista para escribir.
type LineaValidada struct {
	CuentaID string
	Debe     decimal.Decimal
	Haber    decimal.Decimal
	Detalle  string
	Orden    int
}

// Validar comprueba el asiento ANTES de que viaje a la base.
//
// La base lo vuelve a comprobar con su disparador diferido y esa es la que manda —vale para todo
// escritor, no solo para este servicio—. Esta validación existe para otra cosa: devolver un
// mensaje que diga QUÉ está mal. El error de Postgres llega al commit, cuando ya no se sabe qué
// línea lo causó.
func (n *NuevoAsiento) Validar() ([]LineaValidada, error) {
	n.Glosa = strings.TrimSpace(n.Glosa)
	n.Fecha = strings.TrimSpace(n.Fecha)
	if n.Glosa == "" {
		return nil, ErrGlosaRequerida
	}
	if n.Fecha == "" {
		return nil, ErrFechaAsientoRequerida
	}
	if _, err := time.Parse("2006-01-02", n.Fecha); err != nil {
		return nil, ErrFechaInvalida
	}
	if len(n.Lineas) < 2 {
		return nil, ErrAsientoSinLineas
	}

	out := make([]LineaValidada, 0, len(n.Lineas))
	sumaDebe, sumaHaber := decimal.Zero, decimal.Zero
	for i, l := range n.Lineas {
		debe, err := montoDeTexto(l.Debe)
		if err != nil {
			return nil, err
		}
		haber, err := montoDeTexto(l.Haber)
		if err != nil {
			return nil, err
		}
		positivoDebe := debe.GreaterThan(decimal.Zero)
		positivoHaber := haber.GreaterThan(decimal.Zero)
		switch {
		case positivoDebe && positivoHaber:
			return nil, ErrLineaDebeYHaber
		case !positivoDebe && !positivoHaber:
			return nil, ErrLineaSinMonto
		}
		if strings.TrimSpace(l.CuentaID) == "" {
			return nil, ErrCuentaRequerida
		}
		sumaDebe = sumaDebe.Add(debe)
		sumaHaber = sumaHaber.Add(haber)
		out = append(out, LineaValidada{
			CuentaID: strings.TrimSpace(l.CuentaID),
			Debe:     debe, Haber: haber,
			Detalle: strings.TrimSpace(l.Detalle), Orden: i,
		})
	}
	if !sumaDebe.Equal(sumaHaber) {
		return nil, ErrAsientoDescuadrado
	}
	return out, nil
}

// montoDeTexto convierte el monto que llegó como string. Vacío es cero, no un error: una línea
// que va al debe manda el haber vacío.
func montoDeTexto(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, ErrMontoInvalido
	}
	if d.IsNegative() {
		// Un monto negativo al debe es un monto positivo al haber, y escribirlo de las dos formas
		// hace que el mismo asiento se vea distinto según quién lo cargó.
		return decimal.Zero, ErrMontoNegativo
	}
	return d.Round(2), nil
}

var (
	// ErrMontoInvalido: lo que llegó no es un número.
	ErrMontoInvalido = errors.New("conta: el monto no se pudo leer")
	// ErrMontoNegativo: la dirección la da la columna, no el signo.
	ErrMontoNegativo = errors.New("conta: el monto no puede ser negativo: para invertir la dirección, pasalo a la otra columna")
)

// ── El enlace cuenta bancaria → cuenta contable ─────────────────────────────────────────────

// FilaBancoContable es una cuenta bancaria del ERP con su cuenta contable, si ya se decidió.
type FilaBancoContable struct {
	CuentaBancariaID string `json:"cuenta_bancaria_id"`
	Alias            string `json:"alias"`
	Banco            string `json:"banco"`
	Moneda           string `json:"moneda"`
	IBAN             string `json:"iban"`
	// Movimientos ordena el trabajo: enlazar primero la cuenta que más pesa.
	Movimientos int `json:"movimientos"`

	CuentaID     *string `json:"cuenta_id"`
	CuentaCodigo string  `json:"cuenta_codigo"`
	CuentaNombre string  `json:"cuenta_nombre"`

	// Sugerida es la cuenta que el sistema propone por IBAN cuando todavía no hay decisión. Ver
	// SugerirPorIBAN.
	SugeridaID     string `json:"sugerida_id"`
	SugeridaCodigo string `json:"sugerida_codigo"`
	SugeridaNombre string `json:"sugerida_nombre"`

	// Compartida avisa que otra cuenta bancaria apunta a la misma cuenta contable. No lo impide:
	// agrupar dos cuentas del mismo banco puede ser criterio del contador.
	Compartida bool `json:"compartida"`
}

// CoberturaBancos dice cuánto falta para que los asientos de banco puedan existir.
type CoberturaBancos struct {
	Cuentas              int `json:"cuentas"`
	Enlazadas            int `json:"enlazadas"`
	Faltan               int `json:"faltan"`
	ConSugerencia        int `json:"con_sugerencia"`
	MovimientosCubiertos int `json:"movimientos_cubiertos"`
	MovimientosTotales   int `json:"movimientos_totales"`
}

// SugerirPorIBAN propone la cuenta contable de una cuenta bancaria comparando IBAN.
//
// ── POR QUÉ FUNCIONA ────────────────────────────────────────────────────────────────────────
//
// El plan contable real trae el IBAN escrito dentro del nombre de la cuenta («Banco Nacional
// Colones CR15015108710010019890»). Medido el 26-oct-2026 sobre Valle de Paz: 5 de 16 cuentas
// bancarias calzan así, y esas 5 cubren 15.873 de los 20.082 movimientos —el 79 %—.
//
// ── POR QUÉ SUGIERE Y NO DECIDE ─────────────────────────────────────────────────────────────
//
// Porque el IBAN dentro de un nombre libre es una coincidencia afortunada, no un dato estructural:
// nadie garantiza que el contador lo siga escribiendo ahí. Enlazar solo la cuenta equivocada
// mandaría todos los asientos de un banco a otro, y el error recién se vería al conciliar.
func SugerirPorIBAN(iban string, cuentas []Cuenta) (Cuenta, bool) {
	limpio := normalizarIBAN(iban)
	// Un IBAN de Costa Rica son 22 caracteres. Debajo de eso lo que haya no identifica nada y
	// buscarlo como subcadena calzaría con cualquier cosa.
	if len(limpio) < 10 {
		return Cuenta{}, false
	}
	var hallada Cuenta
	n := 0
	for _, c := range cuentas {
		if !c.AceptaMovimiento {
			continue
		}
		if strings.Contains(normalizarIBAN(c.Nombre), limpio) {
			hallada = c
			n++
		}
	}
	// Dos cuentas contables con el mismo IBAN en el nombre es un catálogo con un problema, y
	// elegir una de las dos al azar lo escondería.
	if n != 1 {
		return Cuenta{}, false
	}
	return hallada, true
}

// normalizarIBAN quita espacios y guiones y sube a mayúsculas. El mismo IBAN se escribe de varias
// formas en el nombre de una cuenta y en el campo del ERP.
func normalizarIBAN(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if r == ' ' || r == '-' || r == '.' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ── Los libros ──────────────────────────────────────────────────────────────────────────────

// FiltroDiario acota el libro diario.
type FiltroDiario struct {
	Desde      string
	Hasta      string
	Estado     string
	OrigenTipo string
	Limite     int
	Desplaz    int
}

// FiltroMayor acota el mayor de una cuenta.
type FiltroMayor struct {
	CuentaID string
	Desde    string
	Hasta    string
}

// RenglonMayor es una línea del mayor, con el saldo corrido.
type RenglonMayor struct {
	AsientoID string `json:"asiento_id"`
	Numero    int64  `json:"numero"`
	Fecha     string `json:"fecha"`
	Glosa     string `json:"glosa"`
	Detalle   string `json:"detalle"`
	Debe      string `json:"debe"`
	Haber     string `json:"haber"`
	// Saldo es el acumulado hasta este renglón, con el signo de la NATURALEZA de la cuenta: una
	// cuenta acreedora con saldo acreedor da positivo. Mostrarlo siempre como debe-menos-haber
	// dejaría todos los pasivos en negativo, que no es como se lee un mayor.
	Saldo string `json:"saldo"`
}

// Mayor es el movimiento de una cuenta en un rango.
type Mayor struct {
	Cuenta       Cuenta         `json:"cuenta"`
	SaldoInicial string         `json:"saldo_inicial"`
	Renglones    []RenglonMayor `json:"renglones"`
	TotalDebe    string         `json:"total_debe"`
	TotalHaber   string         `json:"total_haber"`
	SaldoFinal   string         `json:"saldo_final"`
}

// saldoSegunNaturaleza convierte debe-menos-haber al signo que corresponde a la cuenta.
func saldoSegunNaturaleza(debe, haber decimal.Decimal, naturaleza string) decimal.Decimal {
	if naturaleza == Acreedora {
		return haber.Sub(debe)
	}
	return debe.Sub(haber)
}
