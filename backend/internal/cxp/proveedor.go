// Package cxp implementa el módulo de Cuentas por Pagar (Fase 2).
// Por ahora incluye el maestro de proveedores; el flujo de documentos y
// aprobaciones se agrega cuando el Director Financiero defina las reglas.
package cxp

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	// ErrProveedorNoEncontrado indica que el proveedor no existe o no pertenece a la empresa.
	ErrProveedorNoEncontrado = errors.New("cxp: proveedor no encontrado")
	// ErrProveedorDuplicado indica que ya existe un proveedor con esa identificación.
	ErrProveedorDuplicado = errors.New("cxp: ya existe un proveedor con esa identificación")
)

// Proveedor es la vista de un proveedor hacia HTTP.
type Proveedor struct {
	ID                 string `json:"id"`
	Nombre             string `json:"nombre"`
	TipoIdentificacion string `json:"tipo_identificacion"`
	Identificacion     string `json:"identificacion"`
	Email              string `json:"email"`
	Telefono           string `json:"telefono"`
	IBAN               string `json:"iban"`
	RetencionRentaPct  string `json:"retencion_renta_pct"`
	ExentoIVA          bool   `json:"exento_iva"`
	Activo             bool   `json:"activo"`
	// Condiciones de crédito: CONTADO o CREDITO + plazo en días (calcula vencimientos).
	CondicionPago    string `json:"condicion_pago"`
	PlazoCreditoDias int    `json:"plazo_credito_dias"`
	// Gasto predeterminado (memoria AUTO): sus facturas nacen pre-clasificadas con esto.
	GastoConceptoID         string `json:"gasto_concepto_id"`
	GastoClasificacionID    string `json:"gasto_clasificacion_id"`
	GastoSubclasificacionID string `json:"gasto_subclasificacion_id"`
	// Departamento: área de la empresa que ordena el gasto (Logística, Mercadeo, Ventas…).
	// Segmento adicional al gasto, para reportes y filtros.
	Departamento string `json:"departamento"`
	// EsContabilidad: sus facturas son de Contabilidad y no requieren validación de área. Es la
	// marca que captura el «siempre»: se pone una vez y las facturas siguientes nacen así.
	EsContabilidad bool `json:"es_contabilidad"`
}

// ProveedorInput son los datos para crear/actualizar un proveedor.
type ProveedorInput struct {
	Nombre             string
	TipoIdentificacion string
	Identificacion     string
	Email              string
	Telefono           string
	IBAN               string
	RetencionRentaPct  decimal.Decimal
	ExentoIVA          bool
	CondicionPago      string // "" => CONTADO
	PlazoCreditoDias   int
	// Gasto predeterminado ("" = sin asignar / limpiar). Validado contra la empresa en SQL.
	GastoConceptoID         string
	GastoClasificacionID    string
	GastoSubclasificacionID string
	// Departamento ("" = sin asignar). Vocabulario controlado desde el frontend.
	Departamento string
}

// FiltrosProveedor acota el listado por los criterios visibles de la tabla.
// Cadenas vacías = sin filtrar por ese criterio.
type FiltrosProveedor struct {
	Q            string // nombre o identificación
	Estado       string // "activo" | "inactivo"
	IVA          string // "grava" | "exento"
	Condicion    string // "CONTADO" | "CREDITO"
	Retencion    string // "con" | "sin"
	IBAN         string // "con" | "sin"
	Gasto        string // "con" | "sin" (tiene gasto predeterminado)
	Departamento string // valor exacto
}

// ListaProveedores es la respuesta paginada del listado.
type ListaProveedores struct {
	Items    []Proveedor `json:"items"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}
