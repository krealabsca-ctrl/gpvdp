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

/** Una factura leída del Excel de facturación (montos como string decimal). */
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
  estado: ImportEstado;
  proveedor_nuevo: boolean;
}

export interface ResumenImportacion {
  leidas: number;
  nuevas: number;
  duplicadas: number;
  proveedores_nuevos: number;
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
};
