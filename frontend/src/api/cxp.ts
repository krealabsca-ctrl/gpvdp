/**
 * Cliente tipado del módulo CxP (Fase 2 — Cuentas por Pagar).
 *
 * Igual que `bancos.ts`: Node no está en el host, así que los tipos se mirrorean
 * A MANO desde `docs/openapi-cxp.yaml` (fuente de verdad) y las funciones usan
 * `apiFetch` (Bearer scopeado a la empresa activa + 401/refresh + ApiError).
 *
 * Montos: el backend los envía como STRING decimal (nunca float). Se conservan
 * como string; el formateo vive en lib/format.ts.
 */

import { apiFetch } from "@/api/client";
import type { Moneda } from "@/lib/format";
// El catálogo de gasto es el MISMO de Bancos, con dos puertas: se reusan sus tipos a propósito,
// para que la forma del rubro no pueda divergir entre la puerta de Bancos y la de Contabilidad.
import type { ClasificacionCatalogo, ConceptoCatalogo } from "@/api/bancos";

// ---------------------------------------------------------------------------
// Tipos (mirror de components.schemas del OpenAPI CxP)
// ---------------------------------------------------------------------------

export type TipoIdentificacion = "FISICA" | "JURIDICA" | "DIMEX" | "NITE";

export type EstadoDocumento =
  | "RECIBIDO"
  | "REVISADO"
  | "VALIDADO_DEPTO"
  | "APROBADO"
  | "PROGRAMADO"
  | "PAGADO"
  | "CONCILIADO"
  // Terminales del ciclo de revisión (fuera del flujo de pago)
  | "DENEGADO"
  | "ANULADO"
  | "LIQUIDADA"
  | "REBOTADA";

export type TipoFactura = "CXP" | "ANTICIPO" | "VIATICOS" | "REINTEGRO" | "INTERNO";

export interface Proveedor {
  id: string;
  nombre: string;
  tipo_identificacion: string;
  identificacion: string;
  email: string;
  telefono: string;
  iban: string;
  retencion_renta_pct: string;
  exento_iva: boolean;
  activo: boolean;
  /** Condiciones de crédito: CONTADO o CREDITO + plazo en días. */
  condicion_pago: "CONTADO" | "CREDITO";
  plazo_credito_dias: number;
  /** Gasto predeterminado (memoria AUTO): sus facturas nacen pre-clasificadas con esto. */
  gasto_concepto_id: string;
  gasto_clasificacion_id: string;
  gasto_subclasificacion_id: string;
  /** Departamento: área de la empresa que ordena el gasto (segmento adicional al gasto). */
  departamento: string;
  /**
   * Sus facturas son «de Contabilidad»: no requieren validación de área. Es la marca que captura
   * el «siempre» —se pone una vez y las facturas siguientes nacen así—, y es retroactiva sobre las
   * que todavía no se aprobaron, que son justamente las que quedaban trancadas.
   */
  es_contabilidad: boolean;
}

export interface ProveedorInput {
  nombre: string;
  tipo_identificacion?: string;
  identificacion?: string;
  email?: string;
  telefono?: string;
  iban?: string;
  retencion_renta_pct?: string;
  exento_iva?: boolean;
  condicion_pago?: string;
  plazo_credito_dias?: number;
  gasto_concepto_id?: string;
  gasto_clasificacion_id?: string;
  gasto_subclasificacion_id?: string;
  departamento?: string;
}

/** Departamento (centro de costo) de la empresa — catálogo administrable. */
export interface Departamento {
  id: string;
  nombre: string;
  codigo: string;
  centro_costo: string;
  activo: boolean;
}

export interface DepartamentoInput {
  nombre: string;
  codigo?: string;
  centro_costo?: string;
}

/** Validador (titular/suplente) de un departamento. */
export interface Validador {
  usuario_id: string;
  nombre: string;
  email: string;
  rol: "TITULAR" | "SUPLENTE";
}

/** Usuario que opera la empresa (para el selector de validadores). */
export interface UsuarioRef {
  id: string;
  nombre: string;
  email: string;
}

/** Filtros del listado de proveedores (todos opcionales; vacío = sin filtrar). */
export interface FiltrosProveedores {
  q?: string;
  estado?: "activo" | "inactivo";
  iva?: "grava" | "exento";
  condicion?: "CONTADO" | "CREDITO";
  retencion?: "con" | "sin";
  iban?: "con" | "sin";
  gasto?: "con" | "sin";
  departamento?: string;
}

export interface ListaProveedores {
  items: Proveedor[];
  total: number;
  page: number;
  page_size: number;
}

export interface Documento {
  id: string;
  proveedor_id: string;
  proveedor: string;
  clave: string;
  consecutivo: string;
  tipo: TipoFactura;
  fecha_emision: string;
  moneda: Moneda;
  subtotal: string;
  iva: string;
  retencion: string;
  total: string;
  total_crc: string;
  estado: EstadoDocumento;
  fecha_pago_programada: string | null;
  fecha_vencimiento: string | null;
  huella: string;
  descripcion: string;
  concepto_id: string;
  concepto: string;
  clasificacion_id: string;
  clasificacion: string;
  subclasificacion_id: string;
  subclasificacion: string;
  lote_id: string;
  lote_numero: string;
  tiene_comprobante: boolean;
  comprobante_enviado_en: string | null;
  /** La clasificación vino de la memoria del proveedor (pendiente de confirmar). */
  clasif_auto: boolean;
  /** Prioridad interna de pago: "AA" (sí o sí) · "A" (puede esperar) · "" (normal). */
  prioridad: "" | "A" | "AA";
  /** Motivo registrado al denegar/anular/liquidar/rebotar. */
  nota_revision: string;
  /** Validación por departamento (control operativo de área). */
  departamento_id: string;
  departamento: string;
  validado_depto_por: string;
  /** Nombre (o correo) de quien validó el área — para mostrar "validó X" en fases posteriores. */
  validado_depto_por_nombre: string;
  validado_depto_en: string | null;
  validacion_respaldo: string;
  /** Anticipos: suma aplicada (CRC) y neto a pagar/aprobar (total_crc − aplicados). */
  anticipos_aplicados: string;
  neto_crc: string;
  /** El proveedor tiene algún anticipo con saldo disponible (para marcar la fila en la Bandeja). */
  proveedor_anticipo_disponible: boolean;
  /**
   * Factura «de Contabilidad»: no requiere validación de área (honorarios contables, timbres,
   * comisiones bancarias, Hacienda, auditoría). `contabilidad_origen` dice DE DÓNDE sale la marca
   * —"" (no lo es), "FACTURA", "PROVEEDOR", "CLASIFICACION", "CONCEPTO"— y `es_contabilidad` se
   * deriva de él en el servidor, no se calcula aparte.
   */
  contabilidad_origen: OrigenContabilidad;
  es_contabilidad: boolean;
  /** Motivo escrito al marcarla a mano (obligatorio en ese caso). */
  contabilidad_motivo: string;
  /**
   * Validación por riesgo: la factura solo espera la conformidad del área si disparó un criterio
   * (monto, proveedor esporádico o desvío contra su propio histórico). El veredicto se CONGELA al
   * revisar —mover un umbral hoy no reabre lo que ya pasó—, por eso viaja en el documento y no se
   * recalcula en pantalla. `null` = documento anterior a la regla, todavía sin evaluar.
   */
  requiere_validacion: boolean | null;
  validacion_motivo: MotivoValidacion;
  /**
   * La recepción de la que salió esta factura, si entró por el buzón y conserva su XML.
   * Vacío = entró por Excel o a mano: por ese camino el XML nunca llegó al ERP, así que NO HAY
   * comprobante que mostrar. Es lo que decide si la fila ofrece «Ver factura».
   */
  recepcion_id?: string;
}

export type MotivoValidacion = "" | "MONTO" | "PROVEEDOR_NUEVO" | "DESVIO";

/** Por qué esta factura llegó a la cola del área (mismo texto que escribe la auditoría). */
export function etiquetaMotivoValidacion(motivo: MotivoValidacion): string {
  switch (motivo) {
    case "MONTO":
      return "supera el umbral de monto";
    case "PROVEEDOR_NUEVO":
      return "proveedor nuevo o esporádico";
    case "DESVIO":
      return "se aparta del histórico de este proveedor";
    default:
      return "";
  }
}

export type OrigenContabilidad = "" | "FACTURA" | "PROVEEDOR" | "CLASIFICACION" | "CONCEPTO";

/** Explica la marca en una frase (mismo texto que usa la auditoría del servidor). */
export function etiquetaOrigenContabilidad(origen: OrigenContabilidad): string {
  switch (origen) {
    case "FACTURA":
      return "marcada a mano en esta factura";
    case "PROVEEDOR":
      return "el proveedor está marcado como de Contabilidad";
    case "CLASIFICACION":
      return "la clasificación está marcada como de Contabilidad";
    case "CONCEPTO":
      return "el concepto está marcado como de Contabilidad";
    default:
      return "";
  }
}

/** Umbral configurable de la validación por riesgo. El valor viaja como texto: es un decimal. */
export interface ParametroCxP {
  clave: string;
  valor: string;
  descripcion: string;
}

/** Cuántas facturas —y cuánto dinero— trajo cada criterio de riesgo. */
export interface EfectoMotivoValidacion {
  motivo: MotivoValidacion;
  etiqueta: string;
  cantidad: number;
  monto: string;
}

/**
 * A cuánto gasto le está pidiendo confirmación la regla vigente, medido sobre las facturas YA
 * evaluadas. Es lo que convierte la pantalla de umbrales en una decisión y no en un formulario.
 */
export interface EfectoValidacion {
  total: number;
  total_monto: string;
  requieren: number;
  requieren_monto: string;
  por_motivo: EfectoMotivoValidacion[];
}

/** Una fila de la carga masiva de IBAN, con el veredicto que le puso el servidor. */
export interface FilaIBAN {
  fila: number;
  identificacion: string;
  nombre: string;
  iban: string;
  /** OK · SIN_CAMBIO · INVALIDO · NO_ENCONTRADO · DUPLICADO */
  estado: string;
  detalle: string;
  proveedor_id: string;
  /** Qué IBAN se está reemplazando: cambiar una cuenta a ciegas es lo que no se quiere. */
  iban_anterior: string;
}

export interface ResumenIBAN {
  filas: FilaIBAN[];
  a_cargar: number;
  sin_cambio: number;
  invalidos: number;
  no_hallados: number;
  duplicados: number;
}

/** Un proveedor al que todavía no se le puede transferir. */
export interface ProveedorSinIBAN {
  ID: string;
  Nombre: string;
  IBAN: string;
}

/** Una entrada marcada del catálogo o del maestro de proveedores. */
export interface MarcaContabilidad {
  id: string;
  nombre: string;
  /** Acompaña a la clasificación: el mismo nombre puede existir en dos conceptos. */
  concepto?: string;
  /**
   * Desactivar un proveedor o un rubro NO le quita la marca: la excepción sigue vigente para sus
   * facturas abiertas. Se listan igual, señalados, porque una excepción escondida no se audita.
   */
  activo: boolean;
}

/** El cuadro de lo que hoy está marcado como «de Contabilidad». */
export interface MarcasContabilidad {
  proveedores: MarcaContabilidad[];
  conceptos: MarcaContabilidad[];
  clasificaciones: MarcaContabilidad[];
}

/** Anticipo pagado del proveedor con saldo disponible (billetera). */
export interface AnticipoSaldo {
  id: string;
  consecutivo: string;
  fecha_pago: string;
  total_crc: string;
  aplicado: string;
  saldo: string;
  /** Solo en la billetera global (todos los proveedores). */
  proveedor_id?: string;
  proveedor?: string;
  /** Estado del documento anticipo: solo PAGADO/CONCILIADO son aplicables. */
  estado?: EstadoDocumento;
}

/** Fondo fijo de caja chica con su estado derivado. */
export interface FondoCajaChica {
  id: string;
  nombre: string;
  custodio_id: string;
  custodio: string;
  departamento_id: string;
  departamento: string;
  proveedor_id: string;
  proveedor: string;
  monto_asignado: string;
  umbral_pct: string;
  limite_vale: string;
  activo: boolean;
  /** Vales que aún no han sido repuestos (pendientes + en reposición sin pagar). */
  en_vales: string;
  disponible: string;
  /** Vales elegibles para una nueva reposición y su suma. */
  vales_pendientes: number;
  monto_pendiente: string;
}

export interface FondoInput {
  nombre: string;
  custodio_id?: string;
  departamento_id?: string;
  proveedor_id?: string;
  monto_asignado: string;
  umbral_pct?: string;
  limite_vale?: string;
}

/** Vale de caja chica con su estado derivado. */
export interface ValeCajaChica {
  id: string;
  fondo_id: string;
  fecha: string;
  detalle: string;
  monto_crc: string;
  concepto_id: string;
  concepto: string;
  clasificacion_id: string;
  clasificacion: string;
  /** FE = factura electrónica (deducible) · RECIBO = recibo manual (no deducible). */
  comprobante: "FE" | "RECIBO";
  registrado_por: string;
  reposicion_id: string;
  anulado: boolean;
  estado: "PENDIENTE" | "EN_REPOSICION" | "REPUESTO" | "ANULADO";
}

export interface ValeInput {
  fecha?: string;
  detalle: string;
  monto_crc: string;
  concepto_id: string;
  clasificacion_id: string;
  subclasificacion_id?: string;
  comprobante: "FE" | "RECIBO";
}

/** Anticipo aplicado (activo) a una factura. */
export interface AplicacionAnticipo {
  id: string;
  anticipo_id: string;
  anticipo_consecutivo: string;
  monto_crc: string;
  aplicado_por_nombre: string;
  aplicado_en: string;
}

/** Categoría usada históricamente con un proveedor (gastos frecuentes). */
export interface GastoFrecuente {
  concepto_id: string;
  concepto: string;
  clasificacion_id: string;
  clasificacion: string;
  subclasificacion_id: string;
  subclasificacion: string;
  usos: number;
}

/** Resumen de una fase de la Bandeja CxP. */
export interface FaseBandeja {
  /** Espejo de ResumenBandeja del backend: «val» es la validación por área. */
  fase: "rec" | "val" | "apr" | "pag" | "bco" | "pgd" | "arc";
  cantidad: number;
  monto: string;
}

/** Lote/corte de pago. */
export interface LotePago {
  id: string;
  numero: number;
  fecha_corte: string;
  estado: string;
  cantidad: number;
  total_crc: string;
  creado_en: string;
  pagadas: number;
  rebotadas: number;
  pendientes: number;
}

/** 3er nivel del catálogo de gasto (cuelga de una Clasificación). Exclusivo de CxP. */
export interface Subclasificacion {
  id: string;
  clasificacion_id: string;
  nombre: string;
}

export interface DocumentoInput {
  proveedor_id: string;
  /** Vacío para documentos sin factura electrónica (el backend genera referencia interna). */
  clave: string;
  /** CXP (factura electrónica) · ANTICIPO · INTERNO · VIATICOS · REINTEGRO. Vacío = CXP. */
  tipo?: string;
  consecutivo?: string;
  fecha_emision: string;
  moneda: Moneda;
  subtotal?: string;
  iva?: string;
  retencion?: string;
  total: string;
  /** Requerido si moneda = USD. */
  tc?: string;
  descripcion?: string;
  fecha_vencimiento?: string;
}

export interface FiltrosDocumentos {
  estado?: EstadoDocumento;
  /** Varios estados separados por coma (pestañas de la Bandeja). */
  estados?: string;
  /** Búsqueda libre: proveedor, consecutivo o clave. */
  q?: string;
  proveedor_id?: string;
  concepto_id?: string;
  clasificacion_id?: string;
  monto_min?: string;
  monto_max?: string;
  lote_id?: string;
  /** "sin" = sin lote asignado · "con" = con lote. */
  lote?: string;
  /** "vencimiento" => calendariza por fecha de vencimiento (archivo de pagos maestro). */
  orden?: string;
  /**
   * Tramo de antigüedad del tablero: "vencido" (todos) o una clave de tramo (v90, v61,
   * v31, v1, s7, s30, futuro, sin_fecha). Permite llegar del número a sus facturas.
   */
  vencimiento?: string;
  /**
   * Marca «de Contabilidad»: "si" = solo las que se saltan la validación de área, "no" = solo las
   * que la necesitan, ausente = todas. Es lo que hace encontrable el gasto de Contabilidad entre
   * miles de facturas.
   */
  contabilidad?: "si" | "no";
  /**
   * Prioridad de pago: "AA" (sí o sí) · "A" (puede esperar) · "AA_A" (las dos) · "sin" (sin
   * priorizar) · ausente = todas.
   *
   * Es lo que permite armar el corte «solo las AA». La prioridad se veía y ordenaba desde siempre,
   * pero no se podía filtrar: con miles de facturas abiertas, lo urgente había que buscarlo a ojo.
   */
  prioridad?: "AA" | "A" | "AA_A" | "sin";
  /**
   * Validación por riesgo: "si" = solo las que esperan la conformidad del área, "no" = las que
   * fluyen derecho a aprobación. Es lo que separa la cola del validador del resto del ciclo.
   */
  requiere_validacion?: "si" | "no";
  /**
   * Cola de trabajo de la Bandeja (rec/val/apr/cnt/pag/bco/pgd/arc). El backend la resuelve con
   * la MISMA expresión que cuenta el encabezado, así que la pestaña y su número nunca discrepan.
   * Una fase ya no equivale a una lista de estados: «Por aprobar» junta lo que el área validó
   * con lo que nunca necesitó pasar por el área.
   */
  fase?: string;
  /**
   * true = solo la CARTERA VIVA (lo que todavía se debe, en cualquier etapa del flujo), con
   * la misma frontera que usa el tablero. Es lo que hace que el conteo de un tramo de
   * vencimiento y su listado traigan los mismos documentos.
   */
  abierta?: boolean;
  page?: number;
  page_size?: number;
}

export interface ListaDocumentos {
  items: Documento[];
  total: number;
  page: number;
  page_size: number;
}

/** Resultado de POST /cxp/conciliacion/match. `documento` solo viene si concilió. */
export interface ConciliarResult {
  conciliado: boolean;
  documento?: Documento;
}

// --- Trazabilidad y dashboard ejecutivo ---

/** Entrada de la línea de tiempo de un documento. */
export interface EventoHistorial {
  accion: string;
  usuario: string;
  fecha: string;
  /** Motivo/comentario del evento (p. ej. la nota de devolución), si lo hubo. */
  nota?: string;
}

export interface ConteoEstado {
  estado: EstadoDocumento;
  cantidad: number;
  monto: string;
}

/** KPIs ejecutivos del módulo CxP (montos como string decimal en CRC). */
/** Conteo de documentos con su monto (decimal como string). */
export interface Cubo {
  cantidad: number;
  monto: string;
}

/** Claves de los tramos de antigüedad, en el orden en que se presentan. */
export const TRAMOS_VENCIMIENTO = ["v90", "v61", "v31", "v1", "s7", "s30", "futuro", "sin_fecha"] as const;
export type TramoClave = (typeof TRAMOS_VENCIMIENTO)[number];

export interface TramoVencimiento {
  clave: TramoClave;
  vencido: boolean;
  cantidad: number;
  monto: string;
}

export interface ProveedorCartera {
  nombre: string;
  cantidad: number;
  monto: string;
  vencidos: number;
}

export interface PuntoMesCxp {
  periodo: string;
  cantidad: number;
  monto: string;
  en_curso: boolean;
}

/** Lo que se debe HOY (stock). No depende del período elegido. */
export interface CarteraCxp {
  /** Neto a pagar (total − retención − anticipos aplicados) y el bruto que lo explica. */
  abierta: Cubo;
  bruto: string;
  retencion: string;
  anticipos: string;
  vencido: Cubo;
  vence_semana: Cubo;
  rebotadas: Cubo;
  prioridad_aa: Cubo;
  aa_vencidas: number;
  dias_mas_antigua: number;
  sin_departamento: Cubo;
  sin_clasificar: Cubo;
  tramos: TramoVencimiento[] | null;
  top_proveedores: ProveedorCartera[] | null;
}

/** Lo que pasó en el período elegido (flujo). */
export interface MovimientoCxp {
  recibidas: Cubo;
  pagadas: Cubo;
  ciclo_dias: string;
  /** Pagados sin evento de pago en la auditoría: no se pueden fechar (carga histórica). */
  pagadas_sin_evento: number;
  serie: PuntoMesCxp[] | null;
}

export interface DashboardCxp {
  periodo: string;
  /** Día de Costa Rica con el que se calculó la cartera. */
  hoy: string;
  cartera: CarteraCxp;
  /** Cola por fase, de la MISMA fuente que las pestañas de la Bandeja. */
  cola: FaseBandeja[] | null;
  movimiento: MovimientoCxp;
  por_estado: ConteoEstado[] | null;
  total_documentos: number;
  total_monto: string;
  proveedores_activos: number;
  /** true = el tablero está recortado a los departamentos del usuario (como su Bandeja). */
  alcance_limitado: boolean;
}

// --- Transición masiva del flujo ---

export type AccionMasiva =
  | "revisar"
  | "aprobar"
  | "programar"
  | "pagar"
  | "conciliar"
  | "denegar"
  | "anular"
  | "liquidar"
  | "rebotar"
  | "reintentar";

/** Resultado de la acción masiva sobre un documento. */
export interface ResultadoTransicion {
  id: string;
  ok: boolean;
  /** Estado resultante si ok (p. ej. tras aprobar puede seguir en REVISADO si falta 2ª firma). */
  estado?: EstadoDocumento;
  /** Motivo si falló. */
  error?: string;
}

/** Agregado de POST /cxp/documentos/transicion-masiva. */
export interface ResultadoMasivo {
  exitosos: number;
  fallidos: number;
  resultados: ResultadoTransicion[];
}

// --- Importador de facturación (Excel) ---

export type ImportEstado = "NUEVO" | "DUPLICADO";

/** Una factura leída del Excel de facturación o de un XML (montos como string decimal). */
export interface FilaImportada {
  clave: string;
  consecutivo: string;
  fecha_emision: string;
  proveedor: string;
  cedula: string;
  moneda: string;
  subtotal: string;
  iva: string;
  total: string;
  condicion: string;
  vencimiento: string;
  /** Tipo de cambio DE LA FACTURA. Solo en moneda extranjera. */
  tc: string;
  estado: ImportEstado;
  proveedor_nuevo: boolean;

  // ── Solo del camino XML ──
  /** Raíz del comprobante: FacturaElectronica, NotaCreditoElectronica, … */
  tipo_documento?: string;
  /** Cédula del receptor: a quién se le facturó. El Excel lo perdía por completo. */
  receptor?: string;
  receptor_nombre?: string;
  /** Por qué la aritmética del comprobante no cierra. Vacío o ausente = cierra. */
  descuadre?: string;
}

export interface ResumenImportacion {
  leidas: number;
  nuevas: number;
  duplicadas: number;
  proveedores_nuevos: number;
  /**
   * Traza de lectura: de dónde salieron las filas y qué se descartó.
   *
   * Existe para poder EXPLICAR un faltante. Antes, un archivo de 500 facturas del que se leían 85
   * se veía igual que un archivo de 85, porque las filas sin clave se descartaban en silencio.
   */
  hoja: string;
  hojas: string[];
  filas_hoja: number;
  sin_clave: number;
  sin_fecha: number;
  /**
   * Filas donde la columna de fecha del Excel no coincidía con la fecha que trae la clave numérica
   * (posiciones 4-9, `ddmmaa`) y se tomó la de la clave, que no admite dos lecturas.
   */
  fecha_corregida: number;

  // ── Solo cuando se subieron XML de comprobante (ausentes en el camino del Excel) ──
  /** Versiones del esquema de Hacienda que se encontraron: v4.2 / v4.3 / v4.4. */
  versiones?: string[];
  /** Comprobantes leídos que NO generan cuenta por pagar, por tipo (nota de crédito, tiquete…). */
  descartados?: Record<string, number>;
  /** Archivos que no se pudieron decodificar como XML. */
  xml_ilegibles?: number;
  /** El mismo comprobante venía dos veces en la misma entrega. */
  repetidas_en_archivo?: number;
  /** Comprobantes cuya aritmética no cierra (ver FilaImportada.descuadre). */
  descuadres?: number;
  /** Comprobantes que no dicen a quién se le facturó: no se puede cotejar la empresa. */
  sin_receptor?: number;
}


// ---------------------------------------------------------------------------
// Recepción de facturas por buzón de correo (migración 0081)
// ---------------------------------------------------------------------------

/**
 * Estados de una recepción. Nada se borra: lo que no se pudo procesar queda con su motivo.
 *
 * PARQUEADA es la cola de errores y es lo único reintentable. DESCARTADA es lo que
 * legítimamente no es una cuenta por pagar (nota de crédito, recibo de pago) y conserva el XML.
 */
export type EstadoRecepcion = "PENDIENTE" | "PROCESADA" | "DUPLICADA" | "PARQUEADA" | "DESCARTADA";

export interface Recepcion {
  id: string;
  fuente_id?: string;
  fuente_nombre?: string;
  clave?: string;
  tipo_documento?: string;
  version_schema?: string;
  /** La cédula del receptor que dice el XML: es lo que se coteja contra las de la empresa. */
  receptor?: string;
  message_id?: string;
  asunto?: string;
  remitente?: string;
  buzon?: string;
  estado: EstadoRecepcion;
  motivo?: string;
  documento_id?: string;
  consecutivo?: string;
  proveedor?: string;
  total?: string;
  moneda?: string;
  /** El IVA del documento. Texto, como todo el dinero. */
  total_impuesto?: string;
  intentos: number;
  tiene_xml: boolean;
  tiene_pdf: boolean;
  creado_en: string;
  procesado_en?: string;
}

export interface ResumenRecepcion {
  pendientes: number;
  procesadas: number;
  duplicadas: number;
  parqueadas: number;
  descartadas: number;
  ultima_en?: string;
}

export interface BandejaRecepcion {
  resumen: ResumenRecepcion;
  recepciones: Recepcion[];
  /** Las cédulas contra las que se coteja. Vacío = el guardarraíl no puede funcionar. */
  cedulas: string[];
}

export interface ResultadoRecepcion {
  recepcion_id: string;
  estado: EstadoRecepcion;
  motivo?: string;
  repetido: boolean;
  clave?: string;
  documento_id?: string;
  consecutivo?: string;
}

/** Un buzón dado de alta para la empresa. La fuente ES la credencial. */
export interface FuenteRecepcion {
  id: string;
  nombre: string;
  correo: string;
  activo: boolean;
  /** El LATIDO: la última vez que el script llamó, aunque no trajera facturas. */
  ultimo_contacto?: string;
  creado_en: string;
  recibidas: number;
  parqueadas: number;
}

export interface FuentesResponse {
  fuentes: FuenteRecepcion[];
  cedulas: string[];
}

/** El token viaja UNA sola vez, al crear o al rotar: la base solo guarda su hash. */
export interface FuenteCreada {
  fuente: FuenteRecepcion;
  token: string;
}

/** Preview de la importación (POST /cxp/importaciones): no crea nada. */
export interface PreviewImportacion {
  resumen: ResumenImportacion;
  filas: FilaImportada[];
}

/** Resultado de confirmar (POST /cxp/importaciones/confirmar). */
export interface ResultadoImportacion {
  creados: number;
  omitidos_duplicados: number;
  proveedores_creados: number;
  /** El backend puede enviar null cuando no hubo errores; normalizar a [] al usar. */
  errores: string[] | null;
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

export const cxpApi = {
  // --- Proveedores ---
  proveedores(f: FiltrosProveedores = {}, page = 1, pageSize = 100): Promise<ListaProveedores> {
    return apiFetch<ListaProveedores>("/cxp/proveedores", {
      method: "GET",
      query: {
        q: f.q || undefined,
        estado: f.estado || undefined,
        iva: f.iva || undefined,
        condicion: f.condicion || undefined,
        retencion: f.retencion || undefined,
        iban: f.iban || undefined,
        gasto: f.gasto || undefined,
        departamento: f.departamento || undefined,
        page,
        page_size: pageSize,
      },
    });
  },
  /**
   * Trae TODOS los proveedores paginando hasta agotar el total. Se usa para poblar
   * los <select> (filtro de documentos, alta de documento) sin el tope silencioso de
   * una sola página: si hubiera >100 proveedores, el 101+ quedaría inseleccionable.
   */
  async todosProveedores(): Promise<Proveedor[]> {
    const acc: Proveedor[] = [];
    const size = 500; // tope máximo del backend por página
    let page = 1;
    for (;;) {
      const r = await cxpApi.proveedores({}, page, size);
      acc.push(...r.items);
      if (r.items.length === 0 || acc.length >= r.total) break;
      page++;
    }
    return acc;
  },
  proveedor(id: string): Promise<Proveedor> {
    return apiFetch<Proveedor>(`/cxp/proveedores/${id}`, { method: "GET" });
  },
  crearProveedor(input: ProveedorInput): Promise<Proveedor> {
    return apiFetch<Proveedor>("/cxp/proveedores", { method: "POST", json: input });
  },
  actualizarProveedor(id: string, input: ProveedorInput): Promise<Proveedor> {
    return apiFetch<Proveedor>(`/cxp/proveedores/${id}`, { method: "PATCH", json: input });
  },
  desactivarProveedor(id: string): Promise<void> {
    return apiFetch<void>(`/cxp/proveedores/${id}/desactivar`, { method: "POST" });
  },

  // --- Departamentos (catálogo) ---
  departamentos(soloActivos = false): Promise<Departamento[]> {
    return apiFetch<Departamento[]>("/cxp/departamentos", {
      method: "GET",
      query: { activos: soloActivos ? 1 : undefined },
    });
  },
  crearDepartamento(input: DepartamentoInput): Promise<Departamento> {
    return apiFetch<Departamento>("/cxp/departamentos", { method: "POST", json: input });
  },
  actualizarDepartamento(id: string, input: DepartamentoInput): Promise<Departamento> {
    return apiFetch<Departamento>(`/cxp/departamentos/${id}`, { method: "PATCH", json: input });
  },
  desactivarDepartamento(id: string): Promise<void> {
    return apiFetch<void>(`/cxp/departamentos/${id}/desactivar`, { method: "POST" });
  },
  validadores(departamentoId: string): Promise<Validador[]> {
    return apiFetch<Validador[]>(`/cxp/departamentos/${departamentoId}/validadores`, { method: "GET" });
  },
  usuariosEmpresa(): Promise<UsuarioRef[]> {
    return apiFetch<UsuarioRef[]>("/cxp/usuarios", { method: "GET" });
  },
  asignarValidador(departamentoId: string, usuarioId: string, rol: "TITULAR" | "SUPLENTE"): Promise<void> {
    return apiFetch<void>(`/cxp/departamentos/${departamentoId}/validadores`, {
      method: "POST",
      json: { usuario_id: usuarioId, rol },
    });
  },
  quitarValidador(departamentoId: string, usuarioId: string): Promise<void> {
    return apiFetch<void>(`/cxp/departamentos/${departamentoId}/validadores/${usuarioId}`, { method: "DELETE" });
  },

  // --- Validación por departamento (transiciones del documento) ---
  asignarDepartamentoDoc(id: string, departamentoId: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/departamento`, {
      method: "PATCH",
      json: { departamento_id: departamentoId },
    });
  },
  validarDepto(id: string, respaldo: string, nota?: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/validar-depto`, {
      method: "POST",
      json: { respaldo, nota },
    });
  },
  validarEscalado(id: string, motivo: string, respaldo?: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/validar-escalado`, {
      method: "POST",
      json: { motivo, respaldo },
    });
  },

  // --- Facturas «de Contabilidad» (sin validación de área) ---
  /**
   * Marca UNA factura. `valor` tiene TRES estados y por eso se manda explícitamente:
   *   true  → es de Contabilidad (motivo obligatorio)
   *   false → la valida el área, aunque el proveedor o el rubro estén marcados
   *   null  → vuelve a heredar de proveedor/concepto/clasificación
   */
  marcarDocContabilidad(id: string, valor: boolean | null, motivo?: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/contabilidad`, {
      method: "PATCH",
      json: { es_contabilidad: valor, motivo: motivo ?? "" },
    });
  },
  /** Aprueba una factura marcada, saltándose la validación de área (la matriz de firmas sigue). */
  aprobarContabilidad(id: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/aprobar-contabilidad`, { method: "POST" });
  },
  marcarProveedorContabilidad(id: string, valor: boolean): Promise<void> {
    return apiFetch<void>(`/cxp/proveedores/${id}/contabilidad`, {
      method: "PATCH",
      json: { es_contabilidad: valor },
    });
  },
  marcarConceptoContabilidad(id: string, valor: boolean): Promise<void> {
    return apiFetch<void>(`/cxp/contabilidad/conceptos/${id}`, {
      method: "PATCH",
      json: { es_contabilidad: valor },
    });
  },
  marcarClasificacionContabilidad(id: string, valor: boolean): Promise<void> {
    return apiFetch<void>(`/cxp/contabilidad/clasificaciones/${id}`, {
      method: "PATCH",
      json: { es_contabilidad: valor },
    });
  },
  // --- Umbrales de la validación por riesgo ---
  // `efecto` puede faltar si la medición falló: los umbrales se muestran igual.
  parametrosValidacion(): Promise<{ parametros: ParametroCxP[]; efecto?: EfectoValidacion }> {
    return apiFetch<{ parametros: ParametroCxP[]; efecto?: EfectoValidacion }>("/cxp/parametros", {
      method: "GET",
    });
  },
  guardarParametroValidacion(clave: string, valor: string): Promise<void> {
    return apiFetch<void>(`/cxp/parametros/${clave}`, { method: "PUT", json: { valor } });
  },

  // --- Cuentas IBAN de proveedores (carga masiva) ---
  previsualizarIBAN(filas: FilaIBAN[]): Promise<ResumenIBAN> {
    return apiFetch<ResumenIBAN>("/cxp/proveedores/iban/preview", { method: "POST", json: { filas } });
  },
  cargarIBAN(filas: FilaIBAN[]): Promise<{ actualizados: number }> {
    return apiFetch<{ actualizados: number }>("/cxp/proveedores/iban", { method: "POST", json: { filas } });
  },
  proveedoresSinIBAN(): Promise<{ proveedores: ProveedorSinIBAN[]; total: number }> {
    return apiFetch<{ proveedores: ProveedorSinIBAN[]; total: number }>("/cxp/proveedores/sin-iban", { method: "GET" });
  },

  marcasContabilidad(): Promise<MarcasContabilidad> {
    return apiFetch<MarcasContabilidad>("/cxp/contabilidad/marcas", { method: "GET" });
  },
  devolverDoc(id: string, nota?: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/devolver`, { method: "POST", json: { nota } });
  },

  // --- Anticipos (netting) ---
  anticiposDisponibles(proveedorId: string): Promise<AnticipoSaldo[]> {
    return apiFetch<{ items: AnticipoSaldo[] }>(
      `/cxp/anticipos/disponibles?proveedor_id=${encodeURIComponent(proveedorId)}`,
      { method: "GET" },
    ).then((r) => r.items ?? []);
  },
  anticiposEmpresa(): Promise<AnticipoSaldo[]> {
    return apiFetch<{ items: AnticipoSaldo[] }>("/cxp/anticipos", { method: "GET" }).then((r) => r.items ?? []);
  },
  aplicacionesDocumento(id: string): Promise<AplicacionAnticipo[]> {
    return apiFetch<{ items: AplicacionAnticipo[] }>(`/cxp/documentos/${id}/anticipos`, { method: "GET" }).then(
      (r) => r.items ?? [],
    );
  },
  aplicarAnticipo(id: string, anticipoId: string, monto: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/anticipos`, {
      method: "POST",
      json: { anticipo_id: anticipoId, monto },
    });
  },
  /** Aplica varios anticipos a la misma factura en una sola operación (todo-o-nada). */
  aplicarAnticiposLote(id: string, aplicaciones: { anticipo_id: string; monto: string }[]): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/anticipos/lote`, {
      method: "POST",
      json: { aplicaciones },
    });
  },
  // --- Caja chica (fondo fijo) ---
  fondosCaja(): Promise<FondoCajaChica[]> {
    return apiFetch<{ items: FondoCajaChica[] }>("/cxp/cajas", { method: "GET" }).then((r) => r.items ?? []);
  },
  crearFondo(input: FondoInput): Promise<FondoCajaChica> {
    return apiFetch<FondoCajaChica>("/cxp/cajas", { method: "POST", json: input });
  },
  actualizarFondo(id: string, input: FondoInput): Promise<FondoCajaChica> {
    return apiFetch<FondoCajaChica>(`/cxp/cajas/${id}`, { method: "PATCH", json: input });
  },
  desactivarFondo(id: string): Promise<void> {
    return apiFetch<void>(`/cxp/cajas/${id}/desactivar`, { method: "POST" });
  },
  valesCaja(fondoId: string): Promise<ValeCajaChica[]> {
    return apiFetch<{ items: ValeCajaChica[] }>(`/cxp/cajas/${fondoId}/vales`, { method: "GET" }).then(
      (r) => r.items ?? [],
    );
  },
  crearVale(fondoId: string, input: ValeInput): Promise<{ id: string }> {
    return apiFetch<{ id: string }>(`/cxp/cajas/${fondoId}/vales`, { method: "POST", json: input });
  },
  anularVale(fondoId: string, valeId: string): Promise<void> {
    return apiFetch<void>(`/cxp/cajas/${fondoId}/vales/${valeId}/anular`, { method: "POST" });
  },
  generarReposicion(fondoId: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/cajas/${fondoId}/reposicion`, { method: "POST" });
  },

  reversarAnticipo(id: string, aplicacionId: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/anticipos/${aplicacionId}`, { method: "DELETE" });
  },

  // --- Documentos ---
  documentos(filtros: FiltrosDocumentos): Promise<ListaDocumentos> {
    return apiFetch<ListaDocumentos>("/cxp/documentos", { method: "GET", query: { ...filtros } });
  },
  documento(id: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}`, { method: "GET" });
  },
  crearDocumento(input: DocumentoInput): Promise<Documento> {
    return apiFetch<Documento>("/cxp/documentos", { method: "POST", json: input });
  },

  // --- Transiciones de estado ---
  revisar(id: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/revisar`, { method: "POST" });
  },
  aprobar(id: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/aprobar`, { method: "POST" });
  },
  programar(id: string, fechaPagoProgramada: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/programar`, {
      method: "POST",
      json: { fecha_pago_programada: fechaPagoProgramada },
    });
  },
  pagar(id: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/pagar`, { method: "POST" });
  },
  conciliar(id: string): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/conciliar`, { method: "POST" });
  },

  // --- Pagos (SINPE) y conciliación por huella ---
  /** Descarga el CSV de pago de los documentos PROGRAMADOS (opcionalmente hasta `fecha`). */
  descargarArchivoPago(fecha?: string): Promise<Blob> {
    return apiFetch<Blob>("/cxp/pagos/archivo", {
      method: "GET",
      query: { fecha: fecha || undefined },
      blob: true,
    });
  },
  /** Empareja un movimiento bancario (por su descripción, que contiene la huella) con un pago CxP. */
  conciliarMatch(descripcion: string, monto?: string, fecha?: string): Promise<ConciliarResult> {
    return apiFetch<ConciliarResult>("/cxp/conciliacion/match", {
      method: "POST",
      json: { descripcion, monto: monto || undefined, fecha: fecha || undefined },
    });
  },

  // --- Transición masiva del flujo ---
  /** Aplica una acción del flujo a un lote. `fecha` solo para "programar"; `nota` = motivo del archivo. */
  transicionMasiva(accion: AccionMasiva, ids: string[], fecha?: string, nota?: string): Promise<ResultadoMasivo> {
    return apiFetch<ResultadoMasivo>("/cxp/documentos/transicion-masiva", {
      method: "POST",
      json: { accion, ids, fecha_pago_programada: fecha || undefined, nota: nota || undefined },
    });
  },
  /** Fija la prioridad interna de pago (AA / A / "" normal) de un lote de facturas. */
  prioridadMasiva(ids: string[], prioridad: "" | "A" | "AA"): Promise<ResultadoMasivo> {
    return apiFetch<ResultadoMasivo>("/cxp/documentos/prioridad-masiva", {
      method: "POST",
      json: { ids, prioridad: prioridad || undefined },
    });
  },
  /** Categorías frecuentes del proveedor (para clasificar rápido). */
  gastosProveedor(proveedorId: string): Promise<GastoFrecuente[]> {
    return apiFetch<GastoFrecuente[]>(`/cxp/proveedores/${proveedorId}/gastos`, { method: "GET" });
  },
  /** Descarga la macro (.txt) de los documentos PROGRAMADOS indicados por id (macro ad-hoc). */
  descargarArchivoPagoLote(ids: string[]): Promise<Blob> {
    return apiFetch<Blob>("/cxp/pagos/archivo", { method: "POST", json: { ids }, blob: true });
  },

  // --- Lotes de pago (corte) ---
  /** Arma un lote de pago (corte) con las facturas seleccionadas. */
  crearLote(fechaCorte: string, ids: string[]): Promise<LotePago> {
    return apiFetch<LotePago>("/cxp/lotes", { method: "POST", json: { fecha_corte: fechaCorte, ids } });
  },
  /** Lista los lotes de pago de la empresa. */
  lotes(): Promise<LotePago[]> {
    return apiFetch<LotePago[]>("/cxp/lotes", { method: "GET" });
  },
  /** Descarga la macro (.txt) de un lote. */
  descargarMacroLote(loteId: string): Promise<Blob> {
    return apiFetch<Blob>(`/cxp/lotes/${loteId}/macro`, { method: "GET", blob: true });
  },

  // --- Comprobante de pago ---
  /** Adjunta el comprobante de pago (PDF) a una factura pagada/conciliada. */
  adjuntarComprobante(id: string, archivo: File): Promise<{ ok: boolean; filename: string }> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    return apiFetch(`/cxp/documentos/${id}/comprobante`, { method: "POST", raw: fd });
  },
  /** Descarga el comprobante adjunto de una factura. */
  descargarComprobante(id: string): Promise<Blob> {
    return apiFetch<Blob>(`/cxp/documentos/${id}/comprobante`, { method: "GET", blob: true });
  },
  /** Envía el comprobante al correo del proveedor. */
  enviarComprobante(id: string): Promise<{ ok: boolean }> {
    return apiFetch(`/cxp/documentos/${id}/comprobante/enviar`, { method: "POST" });
  },

  // --- Clasificación de gasto ---
  /** Asigna concepto/clasificación/subclasificación de gasto (pasar "" para dejar sin asignar). */
  clasificar(id: string, conceptoId: string, clasificacionId: string, subclasificacionId = ""): Promise<Documento> {
    return apiFetch<Documento>(`/cxp/documentos/${id}/clasificacion`, {
      method: "PATCH",
      json: {
        concepto_id: conceptoId || undefined,
        clasificacion_id: clasificacionId || undefined,
        subclasificacion_id: subclasificacionId || undefined,
      },
    });
  },
  /** Aplica la misma clasificación de gasto (3 niveles) a un lote de documentos. */
  clasificarMasivo(ids: string[], conceptoId: string, clasificacionId: string, subclasificacionId = ""): Promise<ResultadoMasivo> {
    return apiFetch<ResultadoMasivo>("/cxp/documentos/clasificar-masivo", {
      method: "POST",
      json: {
        ids,
        concepto_id: conceptoId || undefined,
        clasificacion_id: clasificacionId || undefined,
        subclasificacion_id: subclasificacionId || undefined,
      },
    });
  },
  /** Lista las subclasificaciones (3er nivel), opcionalmente de una clasificación. */
  subclasificaciones(clasificacionId?: string): Promise<Subclasificacion[]> {
    return apiFetch<Subclasificacion[]>("/cxp/catalogo/subclasificaciones", {
      method: "GET",
      query: { clasificacion_id: clasificacionId || undefined },
    });
  },
  /** Crea una subclasificación bajo una clasificación (idempotente por nombre). */
  crearSubclasificacion(clasificacionId: string, nombre: string): Promise<Subclasificacion> {
    return apiFetch<Subclasificacion>("/cxp/catalogo/subclasificaciones", {
      method: "POST",
      json: { clasificacion_id: clasificacionId, nombre },
    });
  },

  // --- Catálogo de gasto: la puerta de Contabilidad (permiso cxp.catalogo) ---
  //
  // El catálogo es UNO y vive en Bancos; estos endpoints son el otro acceso, con alcance recortado:
  // crear y renombrar, solo sobre lo visible para CxP. Apagar, fusionar y declarar la naturaleza
  // (el EBITDA) siguen siendo de `bancos.catalogo`. Lo creado acá nace visible para CxP.
  /** Abre un rubro de gasto nuevo. Nace visible para CxP: el backend no acepta otra cosa. */
  crearConceptoGasto(nombre: string): Promise<ConceptoCatalogo> {
    return apiFetch<ConceptoCatalogo>("/cxp/catalogo/conceptos", { method: "POST", json: { nombre } });
  },
  /** Renombra un rubro. 404 si no es de los visibles para CxP. */
  renombrarConceptoGasto(id: string, nombre: string): Promise<void> {
    return apiFetch<void>(`/cxp/catalogo/conceptos/${id}`, { method: "PATCH", json: { nombre } });
  },
  /** Cuelga una clasificación de un rubro visible para CxP. */
  crearClasificacionGasto(conceptoId: string, nombre: string): Promise<ClasificacionCatalogo> {
    return apiFetch<ClasificacionCatalogo>("/cxp/catalogo/clasificaciones", {
      method: "POST",
      json: { concepto_id: conceptoId, nombre },
    });
  },
  renombrarClasificacionGasto(id: string, nombre: string): Promise<void> {
    return apiFetch<void>(`/cxp/catalogo/clasificaciones/${id}`, { method: "PATCH", json: { nombre } });
  },
  /** Marca el tipo de factura (CXP/ANTICIPO/VIATICOS/REINTEGRO) de un lote. */
  tipoMasivo(ids: string[], tipo: TipoFactura): Promise<ResultadoMasivo> {
    return apiFetch<ResultadoMasivo>("/cxp/documentos/tipo-masivo", { method: "POST", json: { ids, tipo } });
  },

  // --- Trazabilidad y dashboard ---
  /** Línea de tiempo (auditoría) de un documento. */
  historial(id: string): Promise<{ eventos: EventoHistorial[] }> {
    return apiFetch<{ eventos: EventoHistorial[] }>(`/cxp/documentos/${id}/historial`, { method: "GET" });
  },
  /** KPIs ejecutivos del módulo CxP de la empresa activa. */
  /** Tablero de CxP. El período (selector global) manda sobre el MOVIMIENTO; la cartera es a hoy. */
  dashboard(periodo: string): Promise<DashboardCxp> {
    return apiFetch<DashboardCxp>("/cxp/dashboard", { method: "GET", query: { periodo } });
  },
  /** Resumen por fase de la Bandeja (conteo + monto por pestaña). */
  bandeja(): Promise<{ fases: FaseBandeja[] | null }> {
    return apiFetch<{ fases: FaseBandeja[] | null }>("/cxp/bandeja", { method: "GET" });
  },


  // --- Recepción de facturas por buzón de correo ---
  /** La bandeja de lo que llegó por correo, con el resumen y la cola de errores. */
  recepciones(filtros: { estado?: string; q?: string; limite?: number } = {}): Promise<BandejaRecepcion> {
    const p = new URLSearchParams();
    if (filtros.estado) p.set("estado", filtros.estado);
    if (filtros.q) p.set("q", filtros.q);
    if (filtros.limite) p.set("limite", String(filtros.limite));
    const qs = p.toString();
    return apiFetch<BandejaRecepcion>("/cxp/recepciones" + (qs ? "?" + qs : ""));
  },
  /** Reprocesa una recepción PARQUEADA con el XML que ya está guardado. */
  reintentarRecepcion(id: string): Promise<ResultadoRecepcion> {
    return apiFetch<ResultadoRecepcion>(`/cxp/recepciones/${id}/reintentar`, { method: "POST" });
  },
  /**
   * Descarga el XML o el PDF originales del comprobante recibido.
   *
   * Va por `apiFetch` con `blob` y no por un <a href>: la ruta exige el Bearer de la sesión, así
   * que un enlace directo daría 401.
   */
  descargarArchivoRecepcion(id: string, cual: "xml" | "pdf"): Promise<Blob> {
    return apiFetch<Blob>(`/cxp/recepciones/${id}/archivo?cual=${cual}`, { method: "GET", blob: true });
  },

  // --- Buzones de recepción (configuración) ---
  fuentes(): Promise<FuentesResponse> {
    return apiFetch<FuentesResponse>("/cxp/fuentes");
  },
  /** Da de alta un buzón. La respuesta trae el token EN CLARO: es la única vez que se puede ver. */
  crearFuente(nombre: string, correo: string): Promise<FuenteCreada> {
    return apiFetch<FuenteCreada>("/cxp/fuentes", { method: "POST", json: { nombre, correo } });
  },
  /** Nueva credencial: el token viejo deja de servir de inmediato. */
  rotarTokenFuente(id: string): Promise<{ token: string }> {
    return apiFetch<{ token: string }>(`/cxp/fuentes/${id}/rotar`, { method: "POST" });
  },
  cambiarEstadoFuente(id: string, activo: boolean): Promise<{ ok: boolean }> {
    return apiFetch<{ ok: boolean }>(`/cxp/fuentes/${id}`, { method: "PATCH", json: { activo } });
  },

  // --- Importador de facturación (Excel) ---
  /** Sube el .xlsx y devuelve el preview (marca duplicados y proveedores nuevos). No crea nada. */
  previsualizarImportacion(archivo: File): Promise<PreviewImportacion> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    return apiFetch<PreviewImportacion>("/cxp/importaciones", { method: "POST", raw: fd });
  },
  /** Confirma la importación: crea documentos nuevos y da de alta proveedores faltantes. */
  confirmarImportacion(archivo: File): Promise<ResultadoImportacion> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    return apiFetch<ResultadoImportacion>("/cxp/importaciones/confirmar", { method: "POST", raw: fd });
  },

  // ── Responsabilidades mensuales (mig 0082) ──────────────────────────────────────────────────
  /** El VISOR: la factura ya interpretada desde el XML que guarda la recepción. */
  comprobanteRecepcion(id: string): Promise<VistaComprobante> {
    return apiFetch<VistaComprobante>(`/cxp/recepciones/${id}/comprobante`, { method: "GET" });
  },

  responsabilidades(filtros: FiltrosResponsabilidades = {}): Promise<Responsabilidad[]> {
    return apiFetch<Responsabilidad[]>("/cxp/responsabilidades", { method: "GET", query: { ...filtros } });
  },
  responsabilidad(id: string): Promise<Responsabilidad> {
    return apiFetch<Responsabilidad>(`/cxp/responsabilidades/${id}`, { method: "GET" });
  },
  crearResponsabilidad(input: ResponsabilidadInput): Promise<Responsabilidad> {
    return apiFetch<Responsabilidad>("/cxp/responsabilidades", { method: "POST", json: input });
  },
  actualizarResponsabilidad(id: string, input: ResponsabilidadInput): Promise<Responsabilidad> {
    return apiFetch<Responsabilidad>(`/cxp/responsabilidades/${id}`, { method: "PUT", json: input });
  },
  cambiarEstadoResponsabilidad(id: string, estado: EstadoResponsabilidad, motivo: string): Promise<{ ok: boolean }> {
    return apiFetch<{ ok: boolean }>(`/cxp/responsabilidades/${id}/estado`, { method: "POST", json: { estado, motivo } });
  },
  /** La pantalla «El mes»: encabezado, filas con semáforo y lo que nadie abrió. */
  mesDeResponsabilidades(periodo: string): Promise<VistaDelMes> {
    return apiFetch<VistaDelMes>("/cxp/responsabilidades/mes", { method: "GET", query: { periodo } });
  },
  /** La lista corta: solo las que la persona lleva. No pide permiso de módulo. */
  misResponsabilidades(periodo: string): Promise<PeriodoResponsabilidad[]> {
    return apiFetch<PeriodoResponsabilidad[]>("/cxp/responsabilidades/mias", { method: "GET", query: { periodo } });
  },
  /** Qué va a hacer «Abrir el mes» ANTES de hacerlo, con lo que queda afuera explicado. */
  planDelMes(periodo: string): Promise<PlanDelMes> {
    return apiFetch<PlanDelMes>("/cxp/responsabilidades/mes/plan", { method: "GET", query: { periodo } });
  },
  /** `esperadas` es el total que la persona vio: si el plan cambió, el servidor se detiene. */
  abrirMes(periodo: string, esperadas: number): Promise<PlanDelMes> {
    return apiFetch<PlanDelMes>("/cxp/responsabilidades/mes/abrir", { method: "POST", json: { periodo, esperadas } });
  },
  cerrarPeriodo(id: string, cierre: CierrePeriodo): Promise<{ ok: boolean }> {
    return apiFetch<{ ok: boolean }>(`/cxp/responsabilidades/periodos/${id}/cerrar`, { method: "POST", json: cierre });
  },
  reabrirPeriodo(id: string, motivo: string): Promise<{ ok: boolean }> {
    return apiFetch<{ ok: boolean }>(`/cxp/responsabilidades/periodos/${id}/reabrir`, { method: "POST", json: { motivo } });
  },
};

// ── Responsabilidades mensuales: tipos ─────────────────────────────────────────────────────────
//
// Todo el resto del módulo describe LO QUE LLEGÓ. Esto describe LO QUE SE ESPERA, que es lo que
// permite ver un olvido: hoy, olvidar significa no crear la factura, y no crear la factura
// significa que la obligación nunca existió para el sistema.

export type EstadoResponsabilidad = "ACTIVA" | "SUSPENDIDA" | "FINALIZADA";
export type TipoResponsabilidad = "PAGO" | "TRAMITE";
export type Periodicidad = "MENSUAL" | "BIMENSUAL" | "TRIMESTRAL" | "SEMESTRAL" | "ANUAL";
export type MontoTipo = "FIJO" | "VARIABLE";
export type RespaldoTipo = "CONTRATO" | "ACTA" | "CORREO" | "VERBAL" | "NINGUNO";
export type EstadoPeriodo = "PENDIENTE" | "CUMPLIDA" | "NO_APLICA";
export type PruebaCumplimiento = "FACTURA" | "MOVIMIENTO" | "ACUSE";

/**
 * El semáforo lo calcula el servidor al leer; nunca se guarda. SIN_DATO es la guarda de
 * honestidad: cuando el banco no está importado hasta el día de vencimiento, el sistema no puede
 * afirmar que algo está vencido, así que no lo afirma.
 */
export type Semaforo =
  | "CUMPLIDA"
  | "NO_APLICA"
  | "SIN_DATO"
  | "VENCIDA"
  | "POR_VENCER"
  | "AL_DIA"
  | "SIN_ABRIR";

export interface FiltrosResponsabilidades {
  estado?: EstadoResponsabilidad;
  tipo?: TipoResponsabilidad;
  q?: string;
}

export interface Responsabilidad {
  id: string;
  nombre: string;
  /** Texto libre: el arrendante de palabra no está en el maestro de proveedores. */
  contraparte: string;
  proveedor_id?: string;
  proveedor_nombre?: string;
  tipo: TipoResponsabilidad;
  periodicidad: Periodicidad;
  dia_vencimiento: number;
  mes_ancla?: number;
  moneda: "CRC" | "USD";
  /** Dinero como texto: un número en coma flotante pierde centavos en el camino. */
  monto_esperado: string;
  monto_tipo: MontoTipo;
  respaldo_tipo: RespaldoTipo;
  respaldo_archivo?: string;
  espera_factura: boolean;
  deducible: boolean;
  clasificacion_id?: string;
  clasificacion_nombre?: string;
  departamento_id?: string;
  departamento_nombre?: string;
  estado: EstadoResponsabilidad;
  motivo_estado?: string;
  notas?: string;
  titular_id?: string;
  titular_nombre?: string;
  suplente_id?: string;
  suplente_nombre?: string;
  creado_en: string;
}

export interface ResponsabilidadInput {
  nombre: string;
  contraparte: string;
  proveedor_id?: string;
  tipo?: TipoResponsabilidad;
  periodicidad?: Periodicidad;
  dia_vencimiento: number;
  mes_ancla?: number;
  moneda?: "CRC" | "USD";
  monto_esperado?: string;
  monto_tipo?: MontoTipo;
  respaldo_tipo?: RespaldoTipo;
  respaldo_archivo?: string;
  espera_factura?: boolean;
  deducible?: boolean;
  clasificacion_id?: string;
  departamento_id?: string;
  notas?: string;
  titular_id?: string;
  suplente_id?: string;
}

export interface PeriodoResponsabilidad {
  id: string;
  responsabilidad_id: string;
  nombre: string;
  contraparte: string;
  periodo: string;
  vence_en: string;
  monto_esperado: string;
  moneda: "CRC" | "USD";
  monto_tipo: MontoTipo;
  estado: EstadoPeriodo;
  cumplida_con?: PruebaCumplimiento;
  documento_id?: string;
  movimiento_id?: string;
  acuse_archivo?: string;
  motivo?: string;
  respaldo_tipo?: RespaldoTipo;
  deducible: boolean;
  titular_nombre?: string;
  suplente_nombre?: string;
  cerrado_por_nombre?: string;
  cerrado_en?: string;
  semaforo: Semaforo;
  /** Positivo cuando ya venció. Lo calcula el servidor: la zona horaria del navegador lo corre un día. */
  dias_de_atraso: number;
}

export interface ResumenDelMes {
  periodo: string;
  /** Activas que tocan este mes y NO tienen fila: el número que delata que nadie abrió el mes. */
  sin_abrir: number;
  pendiente: number;
  por_vencer: number;
  vencida: number;
  sin_dato: number;
  cumplida: number;
  no_aplica: number;
  monto_esperado: string;
  monto_vencido: string;
  /** Hasta cuándo alcanzan los datos del banco. Sin esto, «SIN DATO» parece un error del sistema. */
  banco_hasta?: string;
}

export interface VistaDelMes {
  resumen: ResumenDelMes;
  filas: PeriodoResponsabilidad[];
  sin_abrir: Responsabilidad[];
}

export interface MotivoAfuera {
  razon: string;
  cuantas: number;
  nombres: string[];
}

export interface PlanDelMes {
  periodo: string;
  va_a_crear: number;
  ya_estaban: number;
  monto_esperado: string;
  /** Lo que queda afuera, agrupado por razón: «38 de 41» sin explicar las 3 no le sirve a nadie. */
  afuera: MotivoAfuera[];
}

export interface CierrePeriodo {
  estado: Exclude<EstadoPeriodo, "PENDIENTE">;
  cumplida_con?: PruebaCumplimiento;
  documento_id?: string;
  movimiento_id?: string;
  acuse_archivo?: string;
  motivo?: string;
}

// ── EL VISOR DEL COMPROBANTE ───────────────────────────────────────────────────────────────────
//
// La factura ya interpretada, para poder VER qué se compró. Se arma en el servidor a partir del
// XML que la recepción guarda: parsearlo en el navegador perdería el ISO-8859-1 en que emite
// Hacienda (convirtiendo «SEÑOR» en basura sin avisar) y mostraría solo el primero cuando el
// archivo trae varios comprobantes pegados.
//
// TODO monto viaja como STRING tal como vino del XML. Cadena vacía significa EL ELEMENTO NO VINO,
// y es distinto de "0": no usar toNumber() para decidir si mostrar un campo, porque devuelve 0
// para los dos casos.

/** COMPROBANTE = se puede pintar · DESCONOCIDO = no es un comprobante · SIN_XML = ya no se conserva. */
export type ModoVisor = "COMPROBANTE" | "DESCONOCIDO" | "SIN_XML";

export interface ImpuestoComprobante {
  codigo: string;
  /** El PORCENTAJE, tomado del propio XML. Vacío si el comprobante no lo declara. */
  tarifa: string;
  /** El código de catálogo, CRUDO: no hay tabla oficial para traducirlo sin inventar. */
  codigo_tarifa: string;
  monto: string;
}

export interface DescuentoComprobante {
  monto: string;
  codigo: string;
  /** Tal como lo escribió el emisor en el XML ("Descuento al cliente"). */
  naturaleza: string;
}

export interface LineaComprobante {
  numero: string;
  detalle: string;
  cabys: string;
  /** El código del proveedor: es el que sirve para cotejar contra inventario. */
  codigo_comercial: string;
  cantidad: string;
  unidad_medida: string;
  precio_unitario: string;
  monto_total: string;
  descuentos: DescuentoComprobante[];
  subtotal: string;
  base_imponible: string;
  /** Repetible: así es como UNA línea lleva más de una tarifa. */
  impuestos: ImpuestoComprobante[];
  monto_total_linea: string;
}

export interface ParteComprobante {
  nombre: string;
  nombre_comercial?: string;
  tipo_identificacion?: string;
  identificacion: string;
}

export interface MedioPagoComprobante {
  tipo: string;
  monto: string;
}

export interface Comprobante {
  tipo: string;
  tipo_nombre: string;
  version: string;
  clave: string;
  /** 50 dígitos exactos. Si es false, NO cortar la clave por posiciones fijas. */
  clave_valida: boolean;
  consecutivo: string;
  fecha_emision: string;
  condicion_venta: string;
  /** Vacía cuando el código no está entre los confirmados: ahí se muestra el código crudo. */
  condicion_venta_etiqueta: string;
  plazo_credito: string;
  emisor: ParteComprobante;
  /** null en un tiquete electrónico, que normalmente no trae receptor. */
  receptor: ParteComprobante | null;
  moneda: string;
  tipo_cambio: string;
  lineas: LineaComprobante[];
  /** El desglose por tarifa: LA respuesta a los «varios IVAs». */
  desglose: ImpuestoComprobante[];
  medios_pago: MedioPagoComprobante[];
  total_venta: string;
  total_descuentos: string;
  total_venta_neta: string;
  total_impuesto: string;
  total_otros_cargos: string;
  total_iva_devuelto: string;
  total_comprobante: string;
  total_gravado: string;
  total_exento: string;
  total_exonerado: string;
  /** Vacío = los números del comprobante cuadran. No se corrigió nada. */
  descuadre: string;
}

export interface VistaComprobante {
  modo: ModoVisor;
  /** Lo que el visor NO pudo leer, en palabras. Nunca null. */
  avisos: string[];
  /** Cuántos comprobantes MÁS traía el mismo archivo (normal: 0). */
  otros_en_el_archivo: number;
  comprobante: Comprobante | null;
}
