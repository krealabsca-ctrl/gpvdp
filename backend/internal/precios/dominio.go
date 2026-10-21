// Package precios ancla el precio de venta al costo real: el catálogo de insumos, la receta de
// cada plan y el margen que resulta.
//
// ── POR QUÉ EXISTE ──────────────────────────────────────────────────────────────────────────
//
// Medido sobre el archivo de costeo vigente (abril 2025): de 693 líneas, 675 no tienen costo, y
// en 665 el «margen» se calcula desde el precio y el precio desde el precio. De las 18 líneas con
// costo real, el margen promedia 47,6 % cuando la hoja declara 80 %. La lista con la que se
// cotiza tiene 230 líneas y cero fórmulas.
//
// ── LAS TRES REGLAS DEL PAQUETE ─────────────────────────────────────────────────────────────
//
//  1. **El costo se DERIVA del historial.** Ningún tipo de este paquete guarda un costo "actual".
//     El costo vigente a una fecha se calcula leyendo insumo_costo. Es el mismo principio que la
//     existencia en Inventario y el saldo diario en Bancos.
//
//  2. **Todo lo guardado está en base SIN IVA.** El IVA soportado es crédito fiscal, así que no
//     es costo. Quien captura declara la base del insumo y el servicio convierte ANTES de
//     escribir, para que todo lo guardado sea comparable sin que nadie recuerde la regla.
//
//  3. **Variable y fijo nunca se suman sin decirlo.** El piso de precio es el costo VARIABLE: es
//     la cifra que contesta hasta dónde se puede descontar sin perder plata. Mezclarlo con el
//     fijo da un piso inflado y mata ventas que sí convenían.
package precios

import (
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Tipos de costo de un insumo.
const (
	TipoVariable = "VARIABLE"
	TipoFijo     = "FIJO"
)

// Bases de IVA en las que puede venir un costo capturado.
const (
	BaseSinIVA = "SIN_IVA"
	BaseConIVA = "CON_IVA"
)

// Modalidades de un plan.
const (
	ModalidadDirecto     = "DIRECTO"
	ModalidadSuscripcion = "SUSCRIPCION"
)

// Orígenes de un costo.
const (
	OrigenManual    = "MANUAL"
	OrigenCxP       = "CXP"
	OrigenImportado = "IMPORTADO"
)

// TasaIVA es la tasa general de Costa Rica. Se usa para convertir un costo capturado en base
// CON_IVA a la base SIN_IVA en que se guarda todo.
var TasaIVA = decimal.RequireFromString("0.13")

// Errores de dominio.
var (
	ErrInsumoNoEncontrado  = errors.New("precios: insumo no encontrado")
	ErrPlanNoEncontrado    = errors.New("precios: plan no encontrado")
	ErrCodigoDuplicado     = errors.New("precios: ya existe otro con ese código")
	ErrNombreVacio         = errors.New("precios: el nombre es obligatorio")
	ErrCodigoVacio         = errors.New("precios: el código es obligatorio")
	ErrTipoCostoInvalido   = errors.New("precios: el tipo de costo debe ser VARIABLE o FIJO")
	ErrBaseIVAInvalida     = errors.New("precios: la base de IVA debe ser SIN_IVA o CON_IVA")
	ErrModalidadInvalida   = errors.New("precios: la modalidad debe ser DIRECTO o SUSCRIPCION")
	ErrCostoNegativo       = errors.New("precios: el costo no puede ser negativo")
	ErrCantidadNoPositiva  = errors.New("precios: la cantidad debe ser mayor que cero")
	ErrFechaVigenciaVacia  = errors.New("precios: hay que decir desde cuándo rige el costo")
	ErrInsumoRepetido      = errors.New("precios: ese insumo ya está en la receta; cambiá la cantidad en vez de agregarlo otra vez")
	ErrInsumoEnUso         = errors.New("precios: el insumo está en la receta de algún plan; quitalo de ahí antes de eliminarlo")
	ErrPrecioNegativo      = errors.New("precios: el precio no puede ser negativo")
	ErrInsumoDeOtraEmpresa = errors.New("precios: el insumo no pertenece a esta empresa")

	// Importador.
	ErrArchivoVacio           = errors.New("precios: el archivo está vacío o no tiene filas")
	ErrArchivoIlegible        = errors.New("precios: no se pudo leer el archivo; tiene que ser un Excel (.xlsx)")
	ErrHojaNoEncontrada       = errors.New("precios: esa hoja no existe en el archivo")
	ErrColumnaNombreNoHallada = errors.New("precios: no se encontró la columna del nombre; indicá cuál es")
	// La confirmación lleva el número exacto de insumos del plan. Es la misma defensa que usa
	// rehacer cargos en CxC: entre que la pantalla muestra el plan y el usuario aprieta, el
	// archivo pudo cambiar, y escribir 693 filas sobre una base equivocada no tiene vuelta atrás.
	ErrEncabezadoDePlanesNoHallado = errors.New("precios: no se encontró la fila que lista los planes; esta hoja no parece la matriz de costeo")
	ErrConfirmacionNoCoincide      = errors.New("precios: el número de confirmación no coincide con el plan; volvé a revisarlo")
)

// Insumo es una línea del catálogo. NO lleva costo: ver regla 1.
type Insumo struct {
	ID          string  `json:"id"`
	Codigo      string  `json:"codigo"`
	Nombre      string  `json:"nombre"`
	Unidad      string  `json:"unidad"`
	TipoCosto   string  `json:"tipo_costo"`
	BaseIVA     string  `json:"base_iva"`
	ProveedorID *string `json:"proveedor_id"`
	Proveedor   string  `json:"proveedor"`
	CABYS       *string `json:"cabys"`
	Notas       string  `json:"notas"`
	Activo      bool    `json:"activo"`

	// CostoVigente es el costo a la fecha consultada, ya en base sin IVA. Es nil cuando el
	// insumo todavía no tiene ningún costo capturado — el caso que hoy afecta al 97,4 % del
	// catálogo, y por eso se distingue de cero en vez de mostrarse como ₡0.
	CostoVigente *string `json:"costo_vigente"`
	VigenteDesde *string `json:"vigente_desde"`
	Moneda       string  `json:"moneda"`

	// UltimaCompraCxP es lo que de verdad se pagó la última vez, para contrastar el modelo
	// contra la factura. nil cuando no hay con qué contrastar.
	UltimaCompraCxP *string `json:"ultima_compra_cxp"`
	UltimaCompraEn  *string `json:"ultima_compra_en"`
}

// TieneCosto dice si el insumo puede entrar a un costeo. Un insumo sin costo no vale cero:
// vuelve incompleto el costo de todo plan que lo use, y eso hay que decirlo.
func (i Insumo) TieneCosto() bool { return i.CostoVigente != nil }

// Plan es un producto que se vende.
type Plan struct {
	ID               string  `json:"id"`
	Codigo           string  `json:"codigo"`
	Nombre           string  `json:"nombre"`
	Familia          string  `json:"familia"`
	Modalidad        string  `json:"modalidad"`
	PrecioVenta      *string `json:"precio_venta"`
	CuotaMensual     *string `json:"cuota_mensual"`
	Moneda           string  `json:"moneda"`
	Beneficiarios    *int    `json:"beneficiarios"`
	PermanenciaMeses *int    `json:"permanencia_meses"`
	Notas            string  `json:"notas"`
	Activo           bool    `json:"activo"`
}

// LineaReceta es un insumo dentro de la receta de un plan, ya costeado.
type LineaReceta struct {
	ID        string `json:"id"`
	InsumoID  string `json:"insumo_id"`
	Codigo    string `json:"codigo"`
	Nombre    string `json:"nombre"`
	Unidad    string `json:"unidad"`
	TipoCosto string `json:"tipo_costo"`
	Cantidad  string `json:"cantidad"`
	Ordinal   int    `json:"ordinal"`
	Nota      string `json:"nota"`

	// CostoUnitario es nil cuando el insumo no tiene costo capturado. Extendido es
	// cantidad × costo unitario, también nil en ese caso: no se infiere cero.
	CostoUnitario *string `json:"costo_unitario"`
	Extendido     *string `json:"extendido"`
}

// Costeo es el resultado de costear un plan a una fecha. Es lo que el Excel no puede dar hoy.
type Costeo struct {
	Plan   Plan          `json:"plan"`
	Fecha  string        `json:"fecha"`
	Lineas []LineaReceta `json:"lineas"`

	CostoVariable string `json:"costo_variable"`
	CostoFijo     string `json:"costo_fijo"`
	CostoTotal    string `json:"costo_total"`

	// Precio de referencia: el de venta en un plan directo, o cuota × permanencia en uno de
	// suscripción. nil si el plan todavía no tiene precio.
	PrecioReferencia *string `json:"precio_referencia"`

	// Margen y porcentaje, nil cuando no hay precio o la cobertura es incompleta.
	Margen    *string `json:"margen"`
	MargenPct *string `json:"margen_pct"`

	// ContribucionMarginal = precio − costo variable. Es el número que contesta hasta dónde se
	// puede descontar: lo fijo ya está pagado véndase o no.
	ContribucionMarginal *string `json:"contribucion_marginal"`
	// PisoDePrecio es el costo variable: por debajo de eso, cada venta pierde dinero.
	PisoDePrecio string `json:"piso_de_precio"`

	// LA CIFRA MÁS IMPORTANTE DE TODA LA ESTRUCTURA. Una receta armada con el 30 % de sus
	// insumos costeados se lee IGUAL que una completa: los totales son números correctos, solo
	// que de menos líneas. Sin esto, «Oro cuesta ₡400.000» puede ser ₡900.000 y nada lo delata.
	LineasTotal     int      `json:"lineas_total"`
	LineasConCosto  int      `json:"lineas_con_costo"`
	LineasSinCosto  int      `json:"lineas_sin_costo"`
	CoberturaPct    string   `json:"cobertura_pct"`
	InsumosSinCosto []string `json:"insumos_sin_costo"`
}

// EstaCompleto dice si todas las líneas tienen costo. Cuando es falso, el margen no se publica:
// un margen calculado sobre un costo parcial es más alto que el real y empuja a vender barato.
func (c Costeo) EstaCompleto() bool {
	return c.LineasTotal > 0 && c.LineasSinCosto == 0
}

// ── Validación y normalización ──────────────────────────────────────────────────────────────

// InsumoInput es lo que llega del cliente para crear o editar un insumo.
type InsumoInput struct {
	Codigo      string  `json:"codigo"`
	Nombre      string  `json:"nombre"`
	Unidad      string  `json:"unidad"`
	TipoCosto   string  `json:"tipo_costo"`
	BaseIVA     string  `json:"base_iva"`
	ProveedorID *string `json:"proveedor_id"`
	CABYS       *string `json:"cabys"`
	Notas       string  `json:"notas"`
	Activo      *bool   `json:"activo"`
}

// Normalizar limpia y valida. Devuelve el error de dominio que corresponda.
func (in *InsumoInput) Normalizar() error {
	in.Codigo = strings.TrimSpace(in.Codigo)
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Unidad = strings.TrimSpace(in.Unidad)
	in.Notas = strings.TrimSpace(in.Notas)

	if in.Codigo == "" {
		return ErrCodigoVacio
	}
	if in.Nombre == "" {
		return ErrNombreVacio
	}
	if in.Unidad == "" {
		in.Unidad = "UNIDAD"
	}
	if in.TipoCosto == "" {
		in.TipoCosto = TipoVariable
	}
	in.TipoCosto = strings.ToUpper(strings.TrimSpace(in.TipoCosto))
	if in.TipoCosto != TipoVariable && in.TipoCosto != TipoFijo {
		return ErrTipoCostoInvalido
	}
	if in.BaseIVA == "" {
		in.BaseIVA = BaseSinIVA
	}
	in.BaseIVA = strings.ToUpper(strings.TrimSpace(in.BaseIVA))
	if in.BaseIVA != BaseSinIVA && in.BaseIVA != BaseConIVA {
		return ErrBaseIVAInvalida
	}
	if in.CABYS != nil {
		c := strings.TrimSpace(*in.CABYS)
		if c == "" {
			in.CABYS = nil
		} else {
			in.CABYS = &c
		}
	}
	if in.ProveedorID != nil && strings.TrimSpace(*in.ProveedorID) == "" {
		in.ProveedorID = nil
	}
	return nil
}

// CostoInput es la captura de un costo para un insumo.
type CostoInput struct {
	Costo        string  `json:"costo"`
	Moneda       string  `json:"moneda"`
	VigenteDesde string  `json:"vigente_desde"`
	Origen       string  `json:"origen"`
	Nota         string  `json:"nota"`
	DocumentoCxP *string `json:"documento_cxp_id"`
}

// ACostoSinIVA valida la captura y devuelve el costo YA convertido a la base sin IVA, según la
// base declarada en el insumo. Acá vive la regla 2: la conversión ocurre una sola vez, al
// escribir, y no se reparte entre las consultas.
func (in *CostoInput) ACostoSinIVA(baseDelInsumo string) (decimal.Decimal, error) {
	in.Moneda = strings.ToUpper(strings.TrimSpace(in.Moneda))
	if in.Moneda == "" {
		in.Moneda = "CRC"
	}
	in.Origen = strings.ToUpper(strings.TrimSpace(in.Origen))
	if in.Origen == "" {
		in.Origen = OrigenManual
	}
	in.Nota = strings.TrimSpace(in.Nota)

	if strings.TrimSpace(in.VigenteDesde) == "" {
		return decimal.Zero, ErrFechaVigenciaVacia
	}
	if _, err := time.Parse("2006-01-02", in.VigenteDesde); err != nil {
		return decimal.Zero, ErrFechaVigenciaVacia
	}

	bruto, err := decimal.NewFromString(strings.TrimSpace(in.Costo))
	if err != nil {
		return decimal.Zero, ErrCostoNegativo
	}
	if bruto.IsNegative() {
		return decimal.Zero, ErrCostoNegativo
	}

	if strings.ToUpper(baseDelInsumo) == BaseConIVA {
		// El capturado trae IVA adentro: se le quita para guardar la base comparable.
		return bruto.Div(decimal.NewFromInt(1).Add(TasaIVA)).Round(2), nil
	}
	return bruto.Round(2), nil
}
