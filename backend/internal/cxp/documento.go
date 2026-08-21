package cxp

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	// ErrDocumentoNoEncontrado indica que el documento no existe o no pertenece a la empresa.
	ErrDocumentoNoEncontrado = errors.New("cxp: documento no encontrado")
	// ErrDocumentoDuplicado indica que ya existe un documento con esa clave (Hacienda 4.4).
	ErrDocumentoDuplicado = errors.New("cxp: ya existe un documento con esa clave")
	// ErrTransicionInvalida indica que el documento no está en el estado requerido para la acción.
	ErrTransicionInvalida = errors.New("cxp: transición de estado no permitida")
	// ErrYaAprobado indica que el usuario ya aprobó este documento.
	ErrYaAprobado = errors.New("cxp: el usuario ya aprobó este documento")
	// ErrCatalogoInvalido indica que el concepto/clasificación no existe en la empresa.
	ErrCatalogoInvalido = errors.New("cxp: concepto o clasificación no válidos")
	// ErrSinPermisoCartera indica que el usuario pidió la cartera abierta (la deuda total de la
	// empresa) sin tener cxp.cartera_abierta.
	ErrSinPermisoCartera = errors.New("cxp: no tenés permiso para ver la cartera abierta completa")
	// ErrDeptoRequerido indica que falta asignar el departamento antes de validar por área.
	ErrDeptoRequerido = errors.New("cxp: asigná el departamento antes de validar")
	// ErrNoEsValidador indica que el usuario no es validador (titular/suplente) del departamento del documento.
	ErrNoEsValidador = errors.New("cxp: no sos validador de este departamento")
	// ErrRespaldoRequerido indica que falta adjuntar el respaldo para validar por área (regla dura).
	ErrRespaldoRequerido = errors.New("cxp: se requiere respaldo para validar")
	// ErrValidadorNoAprueba indica que quien validó el área no puede además aprobar la misma factura (segregación).
	ErrValidadorNoAprueba = errors.New("cxp: quien validó por el área no puede aprobar la misma factura")
	// ErrEscalamientoNoAplica indica que la factura no está trancada (hay validador y no está vencida): usar la validación normal.
	ErrEscalamientoNoAplica = errors.New("cxp: no procede escalamiento: el área tiene validador y la factura no está vencida")
	// ErrClaveRequerida indica que una factura electrónica (tipo CXP) debe traer la clave de Hacienda.
	ErrClaveRequerida = errors.New("cxp: la factura electrónica requiere la clave de Hacienda (50 dígitos)")
	// ErrMotivoAnticipoRequerido: la solicitud de anticipo exige motivo y respaldo (cotización/contrato).
	ErrMotivoAnticipoRequerido = errors.New("cxp: el anticipo requiere el motivo y su respaldo (cotización/contrato)")
	// ErrNoAprobadorContabilidad: la factura es «de Contabilidad» (se salta la validación de área)
	// y el usuario no tiene el permiso propio para aprobar esa excepción.
	ErrNoAprobadorContabilidad = errors.New("cxp: no tenés permiso para aprobar facturas de Contabilidad (se saltan la validación de área)")
	// ErrMotivoContabilidadRequerido: marcar UNA factura a mano exige decir por qué (queda en auditoría).
	ErrMotivoContabilidadRequerido = errors.New("cxp: indicá el motivo para marcar la factura como de Contabilidad")
	// ErrContabilidadNoModificable: la factura ya salió del tramo donde la marca cambia algo.
	ErrContabilidadNoModificable = errors.New("cxp: la marca solo se cambia antes de aprobar la factura")
	// ErrMarcadorNoAprueba: quien marcó a mano la factura como «de Contabilidad» no puede además
	// aprobarla (misma segregación que validador≠aprobador). Si no, un solo usuario con los dos
	// permisos decide la excepción y la firma.
	ErrMarcadorNoAprueba = errors.New("cxp: quien marcó esta factura como de Contabilidad no puede además aprobarla: que la firme otra persona")
	// ErrNoEsDeContabilidad: se usó la vía de Contabilidad en una factura que NO está marcada. Es
	// un candado, no una molestia: sin él, cxp.aprobar_contabilidad aprobaría cualquier factura y
	// se convertiría en un cxp.aprobar sin la validación de área.
	ErrNoEsDeContabilidad = errors.New("cxp: esta factura no está marcada como de Contabilidad: aprobala por la vía normal")
	// ErrParametroInvalido: la clave del umbral no existe o el valor no es un número >= 0.
	ErrParametroInvalido = errors.New("cxp: parámetro de validación no válido")
)

// Estados del documento CxP (flujo lineal).
const (
	EstRecibido = "RECIBIDO"
	EstRevisado = "REVISADO"
	// EstValidadoDepto: el área validó la conformidad (control operativo previo a la aprobación financiera).
	EstValidadoDepto = "VALIDADO_DEPTO"
	EstAprobado      = "APROBADO"
	EstProgramado    = "PROGRAMADO"
	EstPagado        = "PAGADO"
	EstConciliado    = "CONCILIADO"
	// Estados terminales fuera del flujo de pago (ciclo de revisión).
	EstDenegado  = "DENEGADO"
	EstAnulado   = "ANULADO"
	EstLiquidada = "LIQUIDADA" // viáticos/almuerzos ya pagados: se archivan sin pago
	EstRebotada  = "REBOTADA"  // el banco rechazó el pago dentro de un lote
)

// Tipos de factura (se marca al recibir; VIATICOS no genera pago).
// INTERNO: documento interno sin factura electrónica (liquidaciones, arreglos de pago,
// negociaciones internas) — no lleva clave de Hacienda; el sistema genera una referencia interna.
const (
	TipoCxP       = "CXP"
	TipoAnticipo  = "ANTICIPO"
	TipoViaticos  = "VIATICOS"
	TipoReintegro = "REINTEGRO"
	TipoInterno   = "INTERNO"
)

// esViaExpresa: documentos internos que gestiona Contabilidad (decisión del DF 2026-07-27,
// ampliada 2026-07-28: "hay facturas que no las validan los departamentos sino que fluyen en
// conta" — reintegros de caja chica, arreglos internos, anticipos). NO requieren validación de
// área: se aprueban directo con la matriz de firmas. Si un área igual los validó, también vale.
func esViaExpresa(tipo string) bool {
	return tipo == TipoAnticipo || tipo == TipoReintegro || tipo == TipoInterno
}

// Documento es la vista de un documento CxP hacia HTTP.
type Documento struct {
	ID                  string  `json:"id"`
	ProveedorID         string  `json:"proveedor_id"`
	Proveedor           string  `json:"proveedor"`
	Clave               string  `json:"clave"`
	Consecutivo         string  `json:"consecutivo"`
	Tipo                string  `json:"tipo"`
	FechaEmision        string  `json:"fecha_emision"`
	Moneda              string  `json:"moneda"`
	Subtotal            string  `json:"subtotal"`
	IVA                 string  `json:"iva"`
	Retencion           string  `json:"retencion"`
	Total               string  `json:"total"`
	TotalCRC            string  `json:"total_crc"`
	Estado              string  `json:"estado"`
	FechaPagoProgramada *string `json:"fecha_pago_programada"`
	FechaVencimiento    *string `json:"fecha_vencimiento"`
	Huella              string  `json:"huella"`
	Descripcion         string  `json:"descripcion"`
	// Clasificación de gasto (reusa el catálogo Concepto/Clasificación).
	ConceptoID         string  `json:"concepto_id"`
	Concepto           string  `json:"concepto"`
	ClasificacionID    string  `json:"clasificacion_id"`
	Clasificacion      string  `json:"clasificacion"`
	SubclasificacionID string  `json:"subclasificacion_id"`
	Subclasificacion   string  `json:"subclasificacion"`
	LoteID             string  `json:"lote_id"`
	LoteNumero         string  `json:"lote_numero"`
	TieneComprobante   bool    `json:"tiene_comprobante"`
	ComprobanteEnviado *string `json:"comprobante_enviado_en"`
	// ClasifAuto: la clasificación vino de la memoria del proveedor (pendiente de confirmar).
	ClasifAuto bool `json:"clasif_auto"`
	// Prioridad interna de pago: "AA" (sí o sí) · "A" (puede esperar) · "" (normal).
	Prioridad string `json:"prioridad"`
	// NotaRevision: motivo registrado al denegar/anular/liquidar.
	NotaRevision string `json:"nota_revision"`
	// Validación por departamento (control operativo de área).
	DepartamentoID         string  `json:"departamento_id"`
	Departamento           string  `json:"departamento"`
	ValidadoDeptoPor       string  `json:"validado_depto_por"`
	ValidadoDeptoPorNombre string  `json:"validado_depto_por_nombre"`
	ValidadoDeptoEn        *string `json:"validado_depto_en"`
	ValidacionRespaldo     string  `json:"validacion_respaldo"`
	// Anticipos: suma aplicada (CRC, activos) y neto a pagar/aprobar (total_crc − aplicados).
	AnticiposAplicados string `json:"anticipos_aplicados"`
	NetoCRC            string `json:"neto_crc"`
	// ProveedorAnticipoDisponible: el proveedor tiene algún anticipo con saldo (para marcar la fila).
	ProveedorAnticipoDisponible bool `json:"proveedor_anticipo_disponible"`

	// --- Factura «de Contabilidad»: no requiere validación de área ---
	//
	// ContabilidadOrigen dice DE DÓNDE sale la marca, y es el único dato que viaja: "" (no es de
	// Contabilidad), "FACTURA" (marcada a mano), "PROVEEDOR", "CLASIFICACION" o "CONCEPTO".
	// EsContabilidad se deriva de él, no se consulta aparte: dos campos calculados por separado
	// terminan discrepando y entonces la pantalla dice una cosa y el candado hace otra.
	ContabilidadOrigen string `json:"contabilidad_origen"`
	EsContabilidad     bool   `json:"es_contabilidad"`
	// ContabilidadMotivo es el motivo escrito al marcarla a mano (obligatorio en ese caso).
	ContabilidadMotivo string `json:"contabilidad_motivo"`
	// RequiereValidacion: si el ÁREA tiene que confirmar la conformidad. nil = todavía no evaluado
	// (se evalúa al revisar). El motivo dice por qué: MONTO, PROVEEDOR_NUEVO o DESVIO.
	RequiereValidacion *bool  `json:"requiere_validacion"`
	ValidacionMotivo   string `json:"validacion_motivo"`

	// ContabilidadMarcadoPor es quién marcó ESTA factura a mano. Existe para la segregación de
	// funciones: quien decide que una factura se salta la validación de área no puede además
	// firmarla. Sin esto, un solo usuario con los dos permisos cierra el ciclo completo.
	ContabilidadMarcadoPor string `json:"contabilidad_marcado_por"`
}

// Orígenes posibles de la marca «de Contabilidad».
const (
	ContaOrigenFactura       = "FACTURA"
	ContaOrigenProveedor     = "PROVEEDOR"
	ContaOrigenClasificacion = "CLASIFICACION"
	ContaOrigenConcepto      = "CONCEPTO"
)

// EtiquetaOrigenContabilidad explica la marca en una frase, para la pantalla y la auditoría.
func EtiquetaOrigenContabilidad(origen string) string {
	switch origen {
	case ContaOrigenFactura:
		return "marcada a mano en esta factura"
	case ContaOrigenProveedor:
		return "el proveedor está marcado como de Contabilidad"
	case ContaOrigenClasificacion:
		return "la clasificación está marcada como de Contabilidad"
	case ContaOrigenConcepto:
		return "el concepto está marcado como de Contabilidad"
	default:
		return ""
	}
}

// DocumentoInput son los datos para crear un documento (manual o desde el XML 4.4).
type DocumentoInput struct {
	ProveedorID  string
	Clave        string
	Consecutivo  string
	FechaEmision string // YYYY-MM-DD
	Moneda       string // CRC | USD
	Subtotal     decimal.Decimal
	IVA          decimal.Decimal
	Retencion    decimal.Decimal
	Total        decimal.Decimal
	TC           decimal.Decimal // requerido si moneda = USD
	Descripcion  string
	Vencimiento  string // YYYY-MM-DD o "" (para el archivo de pagos maestro)
	Tipo         string // "" => CXP por defecto
}

// FiltrosDocumentos filtra la hoja de documentos.
type FiltrosDocumentos struct {
	Estado      string
	Estados     []string // varios estados a la vez (pestañas de la Bandeja)
	Q           string   // búsqueda libre: proveedor, consecutivo o clave
	ProveedorID string
	// Filtros de la Bandeja: por categoría de gasto y por rango de monto (total_crc).
	ConceptoID      string
	ClasificacionID string
	MontoMin        string
	MontoMax        string
	LoteID          string // filtra las facturas de un lote de pago
	LoteFiltro      string // "sin" = sin lote asignado · "con" = con lote
	Orden           string // "vencimiento" => calendariza por fecha de vencimiento; default: emisión desc
	// Vencimiento: tramo de antigüedad del dashboard ("vencido" = todos los vencidos, o una
	// clave de tramo: v90, v61, v31, v1, s7, s30, futuro, sin_fecha). Hace navegable el
	// panel de vencimientos: del número se llega a las facturas que lo componen.
	Vencimiento string
	// Abierta limita a la CARTERA VIVA (lo que todavía se debe), con la misma frontera que
	// usa el tablero. Es lo que hace que el conteo de un tramo y su listado coincidan: sin
	// esto, el drill-down caía en una sola fase y perdía las facturas del mismo tramo que ya
	// habían avanzado en el flujo.
	Abierta bool
	// DepartamentoIDs: scoping por área del validador. nil = sin filtro (ve todo);
	// no-nil (aun vacío) = solo esos departamentos (vacío ⇒ no ve nada).
	DepartamentoIDs []string
	// Contabilidad filtra por la marca «de Contabilidad»: "si" = solo las que se saltan la
	// validación de área, "no" = solo las que la necesitan, "" = todas. Sin este filtro, las
	// facturas de Contabilidad quedan mezcladas entre miles y hay que buscarlas a ojo.
	Contabilidad string
	// RequiereValidacion filtra la cola del área: "si" = solo lo que un área debe confirmar,
	// "no" = lo que puede seguir sin esperarla, "" = todo.
	RequiereValidacion string
	// Fase filtra por la cola de trabajo de la Bandeja (rec/val/apr/cnt/pag/bco/pgd/arc), con la
	// misma expresión que cuenta el encabezado. Una fase ya no equivale a una lista de estados.
	Fase     string
	Page     int
	PageSize int
}

// GastoFrecuente es una categoría usada históricamente con un proveedor.
type GastoFrecuente struct {
	ConceptoID         string `json:"concepto_id"`
	Concepto           string `json:"concepto"`
	ClasificacionID    string `json:"clasificacion_id"`
	Clasificacion      string `json:"clasificacion"`
	SubclasificacionID string `json:"subclasificacion_id"`
	Subclasificacion   string `json:"subclasificacion"`
	Usos               int    `json:"usos"`
}

// FaseBandeja es el resumen (conteo + monto) de una fase de la Bandeja CxP.
type FaseBandeja struct {
	Fase     string `json:"fase"` // rec | apr | pag | bco | pgd | arc
	Cantidad int    `json:"cantidad"`
	Monto    string `json:"monto"`
}

// ListaDocumentos es la respuesta paginada.
type ListaDocumentos struct {
	Items    []Documento `json:"items"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}
