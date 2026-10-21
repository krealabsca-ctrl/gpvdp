package precios

// Políticas de comisión y su costo real para la empresa.
//
// ── LA REGLA QUE GOBIERNA ESTE ARCHIVO ──────────────────────────────────────────────────────
//
// La comisión NO cuesta lo que recibe el vendedor.
//
// En nómina, el concepto «Comisiones» está marcado como afecta CCSS, afecta renta y afecta
// aguinaldo en las tres empresas — correcto, porque en Costa Rica las comisiones habituales son
// salario. El costeo tiene que sumarle las cargas patronales: ₡135.000 de comisión en el plan
// Oro le cuestan a la empresa ₡171.221.
//
// Por eso el factor no se teclea en cada política. Se pide a nómina, que ya tiene los parámetros
// versionados por año. Si cada política lo llevara adentro, bastaría que una se olvidara para
// que ese plan quede costeado de menos y nada lo delate.
//
// ── EL PUERTO HACIA NÓMINA ──────────────────────────────────────────────────────────────────
//
// `precios` declara la interfaz y `main.go` la satisface con el paquete `nomina`, igual que
// Inventario declara su facturador y se satisface con CxP. Si esa conexión no estuviera, el
// módulo sigue funcionando: muestra la comisión bruta y dice que no pudo calcular el costo
// patronal, en vez de inventar un factor.

import (
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Tipos de comisión.
const (
	ComisionPorcentaje = "PORCENTAJE"
	ComisionMontoFijo  = "MONTO_FIJO"
)

// Bases sobre las que se calcula.
const (
	BasePrecio       = "PRECIO"
	BaseMargen       = "MARGEN"
	BasePrimeraCuota = "PRIMERA_CUOTA"
)

// Errores de las comisiones.
var (
	ErrPoliticaNoEncontrada  = errors.New("precios: política de comisión no encontrada")
	ErrTipoComisionInvalido  = errors.New("precios: el tipo debe ser PORCENTAJE o MONTO_FIJO")
	ErrBaseComisionInvalida  = errors.New("precios: la base debe ser PRECIO, MARGEN o PRIMERA_CUOTA")
	ErrValorComisionInvalido = errors.New("precios: el valor de la comisión no puede ser negativo")
	ErrPorcentajeMayorACien  = errors.New("precios: un porcentaje de comisión mayor a 100 % pagaría más de lo que entra")
	ErrAjusteInvalido        = errors.New("precios: el ajuste de costos tiene que ser un número")
	ErrFrecuenciaInvalida    = errors.New("precios: la frecuencia de uso tiene que ser un número no negativo")
	ErrSinPrecioParaSimular  = errors.New("precios: el plan no tiene precio ni cuota; no hay contra qué comparar el costo")
	ErrVigenciaInvertida     = errors.New("precios: la fecha de fin no puede ser anterior a la de inicio")
)

// PoliticaComision es cuánto se le paga a quién por vender qué.
type PoliticaComision struct {
	ID           string  `json:"id"`
	PlanID       *string `json:"plan_id"`
	Plan         string  `json:"plan"`
	VendedorID   *string `json:"vendedor_id"`
	Vendedor     string  `json:"vendedor"`
	Canal        string  `json:"canal"`
	Tipo         string  `json:"tipo"`
	Base         string  `json:"base"`
	Valor        string  `json:"valor"`
	VigenteDesde string  `json:"vigente_desde"`
	VigenteHasta *string `json:"vigente_hasta"`
	Notas        string  `json:"notas"`
	Activo       bool    `json:"activo"`

	// Especificidad ordena la resolución: cuanto más alta, más manda. Se calcula al leer y no se
	// guarda, para que agregar una dimensión nueva no obligue a recalcular lo ya escrito.
	Especificidad int `json:"especificidad"`
}

// especificidadDe puntúa cuán específica es una política. El vendedor pesa más que el plan
// porque una política personal es una negociación con alguien, y esa gana sobre la regla general
// del producto.
func especificidadDe(planID, vendedorID *string, canal string) int {
	n := 0
	if planID != nil {
		n += 2
	}
	if vendedorID != nil {
		n += 4
	}
	if strings.TrimSpace(canal) != "" {
		n++
	}
	return n
}

// Resolver elige la política que aplica a un plan y un vendedor en una fecha.
//
// Gana la MÁS ESPECÍFICA; a igual especificidad, la de vigencia más reciente. Devuelve nil
// cuando ninguna aplica, que es distinto de «comisión cero»: sin política no se sabe cuánto se
// paga, y costear con cero daría un margen más alto que el real.
func Resolver(politicas []PoliticaComision, planID, vendedorID string, fecha time.Time) *PoliticaComision {
	var mejor *PoliticaComision
	for i := range politicas {
		p := &politicas[i]
		if !p.Activo || !p.AplicaA(planID, vendedorID, fecha) {
			continue
		}
		if mejor == nil ||
			p.Especificidad > mejor.Especificidad ||
			(p.Especificidad == mejor.Especificidad && p.VigenteDesde > mejor.VigenteDesde) {
			mejor = p
		}
	}
	return mejor
}

// AplicaA dice si la política cubre ese plan, ese vendedor y esa fecha.
func (p PoliticaComision) AplicaA(planID, vendedorID string, fecha time.Time) bool {
	if p.PlanID != nil && *p.PlanID != planID {
		return false
	}
	if p.VendedorID != nil && *p.VendedorID != vendedorID {
		return false
	}
	f := fecha.Format("2006-01-02")
	if p.VigenteDesde > f {
		return false
	}
	if p.VigenteHasta != nil && *p.VigenteHasta < f {
		return false
	}
	return true
}

// CostoDeComision es lo que la comisión le cuesta a la empresa, abierto en sus partes.
type CostoDeComision struct {
	PoliticaID string `json:"politica_id"`
	Tipo       string `json:"tipo"`
	Base       string `json:"base"`
	Valor      string `json:"valor"`

	// MontoBase es sobre qué se calculó (el precio, el margen o la primera cuota).
	MontoBase string `json:"monto_base"`
	// Bruto es lo que recibe el vendedor.
	Bruto string `json:"bruto"`
	// CargaPatronal es lo que la empresa paga ADEMÁS: CCSS, INS y las demás.
	CargaPatronal *string `json:"carga_patronal"`
	FactorCarga   *string `json:"factor_carga"`
	// CostoTotal es bruto + carga. nil cuando no se pudo obtener el factor: ahí se muestra el
	// bruto y se dice por qué falta el resto, en vez de hacer pasar el bruto por costo total.
	CostoTotal *string `json:"costo_total"`
	Motivo     string  `json:"motivo"`
}

// CalcularComision aplica una política sobre una base y le suma la carga patronal.
//
// `factorCarga` es el porcentaje patronal (26.83 para CR 2026); nil cuando nómina no lo pudo dar.
func CalcularComision(p PoliticaComision, montoBase decimal.Decimal, factorCarga *decimal.Decimal) (CostoDeComision, error) {
	valor, err := decimal.NewFromString(p.Valor)
	if err != nil {
		return CostoDeComision{}, ErrValorComisionInvalido
	}

	var bruto decimal.Decimal
	switch p.Tipo {
	case ComisionPorcentaje:
		bruto = montoBase.Mul(valor).Div(cien).Round(2)
	case ComisionMontoFijo:
		bruto = valor.Round(2)
	default:
		return CostoDeComision{}, ErrTipoComisionInvalido
	}
	// Una comisión sobre una base negativa (un margen en rojo) no se paga: el piso es cero. Pagar
	// un porcentaje de un número negativo daría una comisión negativa, que no significa nada.
	if bruto.IsNegative() {
		bruto = decimal.Zero
	}

	c := CostoDeComision{
		PoliticaID: p.ID, Tipo: p.Tipo, Base: p.Base, Valor: p.Valor,
		MontoBase: montoBase.StringFixed(2), Bruto: bruto.StringFixed(2),
	}
	if factorCarga == nil {
		c.Motivo = "no se pudo obtener el porcentaje patronal de nómina: el costo total está incompleto"
		return c, nil
	}

	carga := bruto.Mul(*factorCarga).Div(cien).Round(2)
	total := bruto.Add(carga)
	cs, fs, ts := carga.StringFixed(2), factorCarga.StringFixed(2), total.StringFixed(2)
	c.CargaPatronal, c.FactorCarga, c.CostoTotal = &cs, &fs, &ts
	return c, nil
}

// PoliticaInput es lo que llega del cliente.
type PoliticaInput struct {
	PlanID       *string `json:"plan_id"`
	VendedorID   *string `json:"vendedor_id"`
	Canal        string  `json:"canal"`
	Tipo         string  `json:"tipo"`
	Base         string  `json:"base"`
	Valor        string  `json:"valor"`
	VigenteDesde string  `json:"vigente_desde"`
	VigenteHasta *string `json:"vigente_hasta"`
	Notas        string  `json:"notas"`
	Activo       *bool   `json:"activo"`
}

// Normalizar limpia y valida.
func (in *PoliticaInput) Normalizar() error {
	in.Canal = strings.TrimSpace(in.Canal)
	in.Notas = strings.TrimSpace(in.Notas)
	in.Tipo = strings.ToUpper(strings.TrimSpace(in.Tipo))
	in.Base = strings.ToUpper(strings.TrimSpace(in.Base))

	if in.Tipo == "" {
		in.Tipo = ComisionPorcentaje
	}
	if in.Tipo != ComisionPorcentaje && in.Tipo != ComisionMontoFijo {
		return ErrTipoComisionInvalido
	}
	if in.Base == "" {
		in.Base = BasePrecio
	}
	if in.Base != BasePrecio && in.Base != BaseMargen && in.Base != BasePrimeraCuota {
		return ErrBaseComisionInvalida
	}

	v, err := decimal.NewFromString(strings.TrimSpace(in.Valor))
	if err != nil || v.IsNegative() {
		return ErrValorComisionInvalido
	}
	// Un porcentaje mayor a 100 no es un error de dedo cualquiera: pagaría más de lo que entra
	// por la venta, y el costeo mostraría un margen negativo sin que nadie sepa por qué.
	if in.Tipo == ComisionPorcentaje && v.GreaterThan(cien) {
		return ErrPorcentajeMayorACien
	}

	if _, err := time.Parse("2006-01-02", strings.TrimSpace(in.VigenteDesde)); err != nil {
		return ErrFechaVigenciaVacia
	}
	if in.VigenteHasta != nil {
		h := strings.TrimSpace(*in.VigenteHasta)
		if h == "" {
			in.VigenteHasta = nil
		} else {
			if _, err := time.Parse("2006-01-02", h); err != nil {
				return ErrFechaVigenciaVacia
			}
			if h < in.VigenteDesde {
				return ErrVigenciaInvertida
			}
			in.VigenteHasta = &h
		}
	}
	for _, p := range []**string{&in.PlanID, &in.VendedorID} {
		if *p != nil && strings.TrimSpace(**p) == "" {
			*p = nil
		}
	}
	return nil
}
