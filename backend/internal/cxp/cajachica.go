package cxp

// Caja chica bajo el sistema de fondo fijo (imprest). El fondo tiene monto fijo y custodio;
// los gastos menores se registran como vales; la REPOSICIÓN (documento tipo REINTEGRO al
// custodio) es lo único que viaja por CxP y, al pagarse, restaura el fondo.

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	ErrFondoNoEncontrado    = errors.New("cxp: fondo de caja chica no encontrado")
	ErrFondoDuplicado       = errors.New("cxp: ya existe un fondo con ese nombre")
	ErrFondoInactivo        = errors.New("cxp: el fondo está desactivado")
	ErrValeSobreLimite      = errors.New("cxp: el vale supera el límite del fondo — esa compra va por CxP normal con factura")
	ErrFondoInsuficiente    = errors.New("cxp: el fondo no alcanza para este vale — generá la reposición")
	ErrFondoSinProveedor    = errors.New("cxp: el fondo no tiene proveedor interno (custodio) asignado para pagarle la reposición")
	ErrSinValesPendientes   = errors.New("cxp: no hay vales pendientes de reponer")
	ErrValeNoEncontrado     = errors.New("cxp: vale no encontrado")
	ErrValeYaEnReposicion   = errors.New("cxp: el vale ya está en una reposición; no se puede anular")
	ErrNoEsCustodio         = errors.New("cxp: solo el custodio del fondo (o Contabilidad) puede operar esta caja")
	ErrValeGastoRequerido   = errors.New("cxp: clasificá el gasto del vale (concepto y clasificación)")
	ErrValeDetalleRequerido = errors.New("cxp: el vale requiere el detalle del gasto")
)

// FondoCajaChica es un fondo fijo con su estado derivado (en vales / disponible).
type FondoCajaChica struct {
	ID             string `json:"id"`
	Nombre         string `json:"nombre"`
	CustodioID     string `json:"custodio_id"`
	Custodio       string `json:"custodio"`
	DepartamentoID string `json:"departamento_id"`
	Departamento   string `json:"departamento"`
	ProveedorID    string `json:"proveedor_id"`
	Proveedor      string `json:"proveedor"`
	MontoAsignado  string `json:"monto_asignado"`
	UmbralPct      string `json:"umbral_pct"`
	LimiteVale     string `json:"limite_vale"`
	Activo         bool   `json:"activo"`
	// Derivados: vales que aún no han sido repuestos (pendientes + en reposición sin pagar).
	EnVales    string `json:"en_vales"`
	Disponible string `json:"disponible"`
	// Vales elegibles para una nueva reposición (pendientes) y su suma.
	ValesPendientes int    `json:"vales_pendientes"`
	MontoPendiente  string `json:"monto_pendiente"`
}

// FondoInput son los datos para crear/editar un fondo (los define el DF).
type FondoInput struct {
	Nombre         string
	CustodioID     string
	DepartamentoID string
	ProveedorID    string
	MontoAsignado  decimal.Decimal
	UmbralPct      decimal.Decimal
	LimiteVale     decimal.Decimal
}

// ValeCajaChica es un gasto menor contra el fondo, con su comprobante y estado derivado.
type ValeCajaChica struct {
	ID              string `json:"id"`
	FondoID         string `json:"fondo_id"`
	Fecha           string `json:"fecha"`
	Detalle         string `json:"detalle"`
	MontoCRC        string `json:"monto_crc"`
	ConceptoID      string `json:"concepto_id"`
	Concepto        string `json:"concepto"`
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	// FE = factura electrónica (deducible) · RECIBO = recibo manual (no deducible).
	Comprobante   string `json:"comprobante"`
	RegistradoPor string `json:"registrado_por"`
	ReposicionID  string `json:"reposicion_id"`
	Anulado       bool   `json:"anulado"`
	// Estado derivado: PENDIENTE · EN_REPOSICION · REPUESTO · ANULADO.
	Estado string `json:"estado"`
}

// ValeInput son los datos para registrar un vale.
type ValeInput struct {
	Fecha              string // YYYY-MM-DD; "" = hoy
	Detalle            string
	MontoCRC           decimal.Decimal
	ConceptoID         string
	ClasificacionID    string
	SubclasificacionID string // opcional
	Comprobante        string // FE | RECIBO
}
