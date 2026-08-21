/**
 * Cliente tipado del módulo BANCOS (Fase 1).
 *
 * Node NO está disponible en este entorno, así que NO se corre `gen:api`
 * (openapi-typescript). Igual que se hizo con `auth.ts`, los tipos se
 * mirrorean A MANO desde `openapi/openapi-bancos.yaml` (fuente de verdad) y las
 * funciones usan `apiFetch` de `client.ts` (que ya inyecta el Bearer scopeado a
 * la empresa activa y maneja 401/refresh + ApiError).
 *
 * Convención de montos: el backend envía decimales como STRING (nunca float).
 * Aquí se conservan como string; el formateo a número vive en lib/format.ts.
 */

import { apiFetch } from "@/api/client";
import type { Moneda } from "@/lib/format";

// ---------------------------------------------------------------------------
// Tipos (mirror de components.schemas del OpenAPI)
// ---------------------------------------------------------------------------

export type EstadoClasificacion = "NO_IDENTIFICADO" | "AUTO" | "REVISADO";
export type Tipo = "DEBITO" | "CREDITO";
export type AplicaA = "DEBITO" | "CREDITO" | "MIXTO";
export type EstadoDuplicado = "NUEVO" | "DUPLICADO_REAL" | "REIMPORTACION";
export type FuenteTC = "BCCR" | "MANUAL";
export type EstadoTC = "PROVISIONAL" | "CONGELADO";

export interface CuentaBancaria {
  id: string;
  alias: string;
  banco: string;
  iban: string;
  moneda: Moneda;
  /** Una cuenta desactivada conserva su historia pero no aparece en el importador. */
  activo: boolean;
}

export interface BancoItem {
  id: string;
  nombre: string;
  activo: boolean;
}

/** Lo que cuelga de una cuenta. Decide si se puede eliminar o solo desactivar. */
export interface UsoDeCuenta {
  movimientos: number;
  importaciones: number;
  saldos: number;
  actas: number;
  partidas: number;
}

/** Campos a corregir de una cuenta. Lo que no se manda, no se toca. */
export interface CambioDeCuenta {
  alias?: string;
  iban?: string;
  moneda?: Moneda;
  banco_id?: string;
}

/** Qué arrastró una fusión del catálogo. Se muestra al usuario: es irreversible. */
export interface ResumenFusion {
  origen: string;
  destino: string;
  movimientos: number;
  reglas: number;
  documentos_cxp: number;
  gastos_proveedor: number;
  vales_caja_chica: number;
  proveedores: number;
  clasificaciones: number;
  subclasificaciones: number;
}

export interface CuentaInput {
  banco_id: string;
  alias: string;
  iban?: string;
  moneda: Moneda;
}

export interface ResumenImportacion {
  leidas: number;
  nuevas: number;
  duplicados_reales: number;
  reimportacion: number;
  /** Líneas con problemas de integridad §19 que no se insertan. */
  invalidas: number;
}

export interface MovimientoPreview {
  natural_key: string;
  fecha: string;
  documento: string;
  descripcion: string;
  debito: string;
  credito: string;
  moneda: Moneda;
  indice_ocurrencia: number;
  estado_duplicado: EstadoDuplicado;
  /** Problemas de integridad §19 de esta línea (vacío = línea sana; no se importa si tiene). */
  advertencias: string[];
}

export interface PreviewResult {
  importacion_id: string;
  banco: string;
  iban_archivo: string;
  resumen: ResumenImportacion;
  movimientos: MovimientoPreview[];
  /** Avisos a nivel de archivo (líneas inválidas, cuenta USD sin TC). */
  advertencias: string[];
}

// --- Parámetros por empresa (Fase D) ---
export interface Parametros {
  tolerancia_traslado: string;
  tolerancia_traslado_pct: string;
  cierre_bloqueante: boolean;
}

// --- Sync BCCR (Fase D) ---
export interface ResultadoSync {
  fecha: string;
  indicador: string;
  valor: string;
  exito: boolean;
  omitido: boolean;
  mensaje: string;
}
export interface BCCRSyncLog {
  fecha: string;
  indicador: string;
  valor: string;
  exito: boolean;
  mensaje: string;
  creado_en: string;
}
export interface UltimoSync {
  sincronizado: boolean;
  log?: BCCRSyncLog;
}

export interface ConfirmarResult {
  importacion_id: string;
  insertados: number;
}

export interface MovimientoRow {
  id: string;
  fecha: string;
  documento: string;
  descripcion: string;
  banco: string;
  cuenta: string;
  debito: string;
  credito: string;
  moneda: Moneda;
  monto_crc: string;
  concepto_id: string | null;
  concepto: string;
  clasificacion_id: string | null;
  clasificacion: string;
  estado_clasificacion: EstadoClasificacion;
  confianza: string | null;
  es_traslado: boolean;
}

/** Totales del filtro, EN COLONES (salen de `monto_crc`, no de débito/crédito). */
export interface TotalesMovimientos {
  total_debitos: string;
  total_creditos: string;
  diferencia: string;
  /** Movimientos en otra moneda todavía sin tipo de cambio: entran al total como CERO. */
  sin_tipo_cambio: number;
  /** Ese monto en su moneda original (no se puede sumar al total en colones). */
  monto_sin_convertir: string;
}

export interface ListaMovimientos {
  totales: TotalesMovimientos;
  items: MovimientoRow[];
  total: number;
  page: number;
  page_size: number;
}

export interface FiltrosMovimientos {
  desde?: string;
  hasta?: string;
  periodo?: string;
  concepto_id?: string;
  clasificacion_id?: string;
  /**
   * Las versiones en PLURAL: «uno, varios o todos» (decisión del negocio para los reportes).
   * Vacío o ausente = sin restricción. Viajan como lista separada por comas.
   */
  periodos?: string[];
  conceptos?: string[];
  /** El nivel fino: hay cientos de clasificaciones, así que se eligen buscando, no en parrilla. */
  clasificaciones?: string[];
  /** Filtra el grupo completo de cuentas de un banco. */
  banco_id?: string;
  /** Afina a una sola cuenta (dentro del banco elegido, si hay uno). */
  cuenta_bancaria_id?: string;
  /** "CLASIFICADO" es un pseudo-estado: todo lo que ya tiene clasificación (AUTO o REVISADO). */
  estado_clasificacion?: EstadoClasificacion | "CLASIFICADO";
  tipo?: Tipo;
  /** "si" = solo traslados emparejados · "no" = solo lo que no es traslado. */
  traslado?: "si" | "no";
  q?: string;
  orden?: string;
  page?: number;
  page_size?: number;
}

/**
 * Presentación del detalle del reporte. Las dos tienen que funcionar:
 *  · "partida" — bandas Concepto › Clasificación con subtotal de cada una (vista de análisis).
 *  · "ninguno" — listado corrido por fecha, sin subtotales, con la partida en columnas y
 *    autofiltro (vista de trabajo: se ordena, se filtra y se hace tabla dinámica en Excel).
 */
export type AgruparReporte = "partida" | "ninguno";

/** Descarga cuyo nombre lo decide el servidor (Content-Disposition). */
export interface DescargaArchivo {
  blob: Blob;
  filename: string;
}

export interface ConceptoCatalogo {
  id: string;
  nombre: string;
  /** Si contabilidad lo ve desde el clasificador de gastos de CxP. */
  visible_cxp: boolean;
  /**
   * Qué es el concepto para el EBITDA. Lo declara el usuario: el dashboard suma como ingreso SOLO
   * lo que esté en INGRESO y como gasto solo lo que esté en GASTO. NEUTRO no entra (tesorería,
   * ahorro, reservas, aportes entre empresas). El default es NEUTRO para que un concepto nuevo no
   * se cuele al número sin que nadie lo haya decidido.
   */
  naturaleza: NaturalezaConcepto;
  /**
   * false = NADIE la declaró y el valor viene del default. Separa la decisión del silencio: sin
   * esto, «no entra al EBITDA a propósito» (Ahorro, Utilidades, Proyecto) y «falta decidir» son el
   * mismo dato, y la pantalla le informa al usuario una decisión que nadie tomó.
   */
  naturaleza_declarada: boolean;
}

export type NaturalezaConcepto = "INGRESO" | "GASTO" | "NEUTRO";

/** Qué hace cada naturaleza, en una frase (mismo texto que el backend). */
export function etiquetaNaturaleza(n: NaturalezaConcepto): string {
  switch (n) {
    case "INGRESO":
      return "Suma a los ingresos del EBITDA";
    case "GASTO":
      return "Suma a los gastos del EBITDA";
    default:
      return "No entra al EBITDA (tesorería, ahorro, reservas, aportes)";
  }
}

/** Estado de una fila del archivo de clasificación en bloque. */
export type EstadoClasifExcel =
  | "CLASIFICA"
  | "RECLASIFICA"
  | "SIN_CAMBIO"
  | "SIN_MOVIMIENTO"
  | "PARTIDA_DESCONOCIDA"
  | "CUENTA_DESCONOCIDA"
  | "FILA_INVALIDA"
  | "AMBIGUO"
  | "YA_CLASIFICADO"
  | "SIN_LLENAR";

/** Una fila del archivo: lo que traía y lo que el servidor resolvió. */
export interface FilaClasifExcel {
  linea: number;
  cuenta: string;
  fecha: string;
  documento: string;
  debito: string;
  credito: string;
  concepto: string;
  clasificacion: string;
  estado: EstadoClasifExcel;
  detalle: string;
  /** Descripción del MOVIMIENTO hallado: la prueba de que calzó con el correcto. */
  descripcion: string;
  /** Qué partida tenía antes (vacío = estaba sin clasificar). */
  partida_actual: string;
}

/** Qué va a pasar (o qué pasó) con el archivo entero. */
export interface PlanClasifExcel {
  filas: number;
  detalle: FilaClasifExcel[];
  /** true = la tabla se recortó; los contadores siguen siendo del total. */
  detalle_truncado: boolean;
  clasifica: number;
  reclasifica: number;
  sin_cambio: number;
  sin_movimiento: number;
  sin_partida: number;
  sin_cuenta: number;
  invalidas: number;
  ambiguas: number;
  protegidas: number;
  /** Filas de la plantilla que quedaron en blanco. No son un problema. */
  sin_llenar: number;
  /** false = fue una previsualización y no se escribió nada. */
  aplicado: boolean;
  clasificados: number;
  /** Hoja que se leyó y todas las del libro: se lee UNA sola. */
  hoja: string;
  hojas: string[];
  /** Qué hay que mirar antes de aplicar, en una frase (vacío = nada). */
  aviso: string;
}

/** Ámbito del catálogo: "cxp" limita a lo visible para contabilidad. */
export type AmbitoCatalogo = "cxp" | undefined;

export interface ClasificacionCatalogo {
  id: string;
  concepto_id: string;
  concepto: string;
  nombre: string;
}

export interface ConceptoInput {
  nombre: string;
  visible_cxp?: boolean;
}

export interface ClasificacionInput {
  concepto_id: string;
  nombre: string;
  cuenta_contable_futura?: string;
}

export interface ReglaClasificacionInput {
  nombre: string;
  aplica_a: AplicaA;
  concepto_id: string;
  clasificacion_id: string;
  prioridad?: number;
  palabras_clave: string[];
}

export interface ReglaResult {
  regla_id: string;
  clasificados: number;
}

export interface Regla {
  id: string;
  nombre: string;
  aplica_a: AplicaA;
  concepto_id: string;
  concepto: string;
  clasificacion_id: string;
  clasificacion: string;
  prioridad: number;
  palabras_clave: string[];
  activo: boolean;
  aciertos: number;
}

/** Edición parcial de una regla (solo lo presente se toca). */
export interface ReglaUpdateInput {
  prioridad?: number;
  activo?: boolean;
  agregar_palabras?: string[];
  quitar_palabras?: string[];
}

/** Propuesta de aprendizaje tras clasificar a mano un movimiento. */
export interface SugerenciaRegla {
  sugerible: boolean;
  motivo?: string;
  palabra_clave: string;
  aplica_a: AplicaA;
  concepto_id: string;
  concepto: string;
  clasificacion_id: string;
  clasificacion: string;
  similares: number;
  nombre_sugerido: string;
}

/** Conteos por estado para el KPI de auto-clasificación. */
export interface ResumenClasif {
  total: number;
  no_identificados: number;
  auto: number;
  revisados: number;
  traslados: number;
}

export interface CotizacionInput {
  fecha: string;
  valor: string;
  fuente: FuenteTC;
}

export interface Cotizacion {
  fecha: string;
  valor: string;
  fuente: FuenteTC;
}

export interface TipoCambioMes {
  anio: number;
  mes: number;
  estado: EstadoTC;
  valor_congelado: string | null;
  cotizaciones: Cotizacion[];
}

export interface CuadreConcepto {
  concepto_id: string;
  concepto: string;
  total_creditos: string;
  total_debitos: string;
}

export interface CuadreClasifNodo {
  clasificacion_id: string;
  clasificacion: string;
  debito: string;
  credito: string;
  movs: number;
}

export interface CuadreConceptoNodo {
  concepto_id: string;
  concepto: string;
  debito: string;
  credito: string;
  movs: number;
  clasificaciones: CuadreClasifNodo[];
}

/** Cómo se agrupan los dos niveles del desglose de la selección. */
export type AgruparResumen = "concepto" | "cuenta";

/**
 * Resumen de LO QUE ESTÁS VIENDO en la hoja de trabajo. Sale del mismo filtro que la
 * lista, así que sus números son siempre los de las filas en pantalla. Montos en CRC
 * (no se suman colones con dólares).
 */
export interface ResumenSeleccion {
  agrupar: AgruparResumen;
  movs: number;
  total_debito: string;
  total_credito: string;
  neto: string;
  conceptos: CuadreConceptoNodo[];
}

export interface CuadreArbol {
  periodo: string;
  total_debito: string;
  total_credito: string;
  movs: number;
  conceptos: CuadreConceptoNodo[];
}

export interface DashboardComparativo {
  periodo_anterior: string;
  ingresos: string;
  gastos: string;
  ebitda: string;
}

export interface DashboardData {
  periodo: string;
  /** SOLO los conceptos declarados como INGRESO (ver `naturaleza` del concepto). */
  ingresos: string;
  /** SOLO los conceptos declarados como GASTO. */
  gastos: string;
  ebitda: string;
  no_identificados: number;
  /** Cuánto quedó FUERA del EBITDA por conceptos en NEUTRO o sin clasificar (aviso, no error). */
  fuera_del_ebitda: string;
  movs_fuera_del_ebitda: number;
  /** Conceptos EN USO que todavía no declararon su naturaleza: la acción pendiente. */
  conceptos_sin_declarar: number;
  comparativo: DashboardComparativo;
}

/** Un mes de la tendencia (Fase B). Ingresos/gastos excluyen traslados emparejados. */
export interface SerieMensualPunto {
  periodo: string; // YYYY-MM
  ingresos: string;
  gastos: string;
  ebitda: string;
  movimientos: number;
  no_identificados: number;
}

/** Un mes de la serie de UNA partida (Concepto › Clasificación). */
export interface PuntoPartida {
  periodo: string; // YYYY-MM
  monto: string;
  movs: number;
}

/**
 * Salud de un mes: qué porcentaje tiene su partida asignada y si sirve para comparar.
 * Un mes a medio clasificar muestra menos gasto del que tuvo: compararlo inventa anomalías.
 */
export interface SaludMes {
  periodo: string;
  movs: number;
  pct_clasificado: string;
  comparable: boolean;
}

/** Una partida con su serie mensual, su promedio y cuánto se apartó el último mes. */
export interface TendenciaPartida {
  concepto_id: string;
  concepto: string;
  clasificacion_id: string;
  clasificacion: string;
  naturaleza: string;
  /** false = nadie declaró la naturaleza del concepto: no es «no entra», es «falta decidir». */
  naturaleza_declarada: boolean;
  serie: PuntoPartida[];
  total: string;
  promedio: string;
  ultimo: string;
  desvio_pct: string;
  /** Meses anteriores comparables sobre los que se calculó el promedio. */
  meses_con_dato: number;
  /** false = no hay historia suficiente y el desvío no significa nada. */
  confiable: boolean;
}

/** Análisis de partidas en el tiempo: la tendencia de cada gasto y qué se salió de su cauce. */
export interface AnalisisPartidas {
  desde: string;
  hasta: string;
  meses: SaludMes[];
  partidas: TendenciaPartida[];
  meses_comparables: number;
  /** Por qué el análisis puede no ser confiable (vacío si está bien). */
  aviso: string;
}

/** Un día del calendario de flujo (Fase B). */
export interface DiaCalendario {
  fecha: string; // YYYY-MM-DD
  creditos: string;
  debitos: string;
  neto: string;
  movimientos: number;
}

/** Totales del período por cuenta bancaria (Fase B). */
export interface CuentaResumen {
  cuenta_id: string;
  banco: string;
  alias: string;
  creditos: string;
  debitos: string;
  movimientos: number;
}

// --- Proyecciones (Fase C) ---

export type MetodoProyeccion = "RITMO" | "HISTORICO" | "PROMEDIO" | "COINCIDENCIA";

export interface PuntoSenda {
  dia: number;
  acumulado: string;
}

export interface LineaProyeccion {
  clasificacion_id: string;
  nombre: string;
  real: string;
  cierre: string;
  meta: string;
  brecha: string;
}

export interface ProyeccionResultado {
  periodo: string;
  dias_mes: number;
  dia_calculo: number;
  sin_datos: boolean;
  metodo: MetodoProyeccion;
  metodo_efectivo: MetodoProyeccion;
  mes_gemelo?: string;
  metodos_disponibles: MetodoProyeccion[];
  real_acumulado: string;
  cierre_proyectado: string;
  meta_pct: string;
  meta_monto: string;
  brecha: string;
  senda_real: PuntoSenda[];
  senda_proyeccion: PuntoSenda[];
  por_linea: LineaProyeccion[];
}

export interface EscenarioGuardado {
  id: string;
  periodo: string;
  metodo: MetodoProyeccion;
  metodo_efectivo: MetodoProyeccion;
  meta_pct: string;
  dia_calculo: number;
  real_acumulado: string;
  cierre_proyectado: string;
  meta_monto: string;
  real_cierre: string;
  creado_en: string;
}

/**
 * Veredicto de una propuesta de traslado (espejo de PuntuarTraslado en el backend):
 *  · PROBABLE — la descripción dice traslado, los números calzan y es el único candidato.
 *  · REVISAR  — calzan los números pero la descripción no lo respalda: lo confirma una persona.
 *  · AMBIGUO  — el mismo movimiento tiene varias parejas posibles; hay que elegir cuál.
 * Los descartados (cobros a clientes, montos recurrentes) no llegan al frontend.
 */
export type VeredictoTraslado = "PROBABLE" | "REVISAR" | "AMBIGUO";

export interface PropuestaTraslado {
  debito_id: string;
  credito_id: string;
  fecha_debito: string;
  fecha_credito: string;
  cuenta_debito: string;
  cuenta_credito: string;
  monto_debito: string;
  monto_credito: string;
  descripcion_debito: string;
  descripcion_credito: string;
  /** Puntaje y las razones que lo explican, para no emparejar a ciegas. */
  puntaje: number;
  veredicto: VeredictoTraslado;
  razones: string[] | null;
}

export interface PeriodoEstado {
  anio: number;
  mes: number;
  cerrado: boolean;
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

export const bancosApi = {
  // --- Cuentas / catálogo ---
  /** `incluirInactivas` solo lo usa el catálogo: el importador nunca debe ver las apagadas. */
  cuentas(incluirInactivas = false): Promise<CuentaBancaria[]> {
    return apiFetch<CuentaBancaria[]>("/bancos/cuentas", {
      method: "GET",
      query: incluirInactivas ? { incluir_inactivas: true } : {},
    });
  },
  // --- Administración de bancos y cuentas (catálogo) ---
  bancos(incluirInactivos = false): Promise<BancoItem[]> {
    return apiFetch<BancoItem[]>("/bancos/catalogo/bancos", {
      method: "GET",
      query: incluirInactivos ? { incluir_inactivos: true } : {},
    });
  },
  crearBanco(nombre: string): Promise<BancoItem> {
    return apiFetch<BancoItem>("/bancos/catalogo/bancos", { method: "POST", json: { nombre } });
  },
  renombrarBanco(id: string, nombre: string): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/bancos/${id}`, { method: "PATCH", json: { nombre } });
  },
  eliminarBanco(id: string): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/bancos/${id}`, { method: "DELETE" });
  },
  cambiarActivoBanco(id: string, activo: boolean): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/bancos/${id}/activo`, { method: "POST", json: { activo } });
  },
  crearCuenta(input: CuentaInput): Promise<CuentaBancaria> {
    return apiFetch<CuentaBancaria>("/bancos/catalogo/cuentas", { method: "POST", json: input });
  },
  /** Alias y banco siempre; moneda e IBAN solo si la cuenta no tiene movimientos. */
  actualizarCuenta(id: string, cambio: CambioDeCuenta): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/cuentas/${id}`, { method: "PATCH", json: cambio });
  },
  usoDeCuenta(id: string): Promise<UsoDeCuenta> {
    return apiFetch<UsoDeCuenta>(`/bancos/catalogo/cuentas/${id}/uso`, { method: "GET" });
  },
  eliminarCuenta(id: string): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/cuentas/${id}`, { method: "DELETE" });
  },
  cambiarActivoCuenta(id: string, activo: boolean): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/cuentas/${id}/activo`, { method: "POST", json: { activo } });
  },
  /** Mueve TODO lo del concepto al destino y borra el origen. Irreversible. */
  fusionarConcepto(id: string, destinoId: string): Promise<ResumenFusion> {
    return apiFetch<ResumenFusion>(`/bancos/catalogo/conceptos/${id}/fusionar`, {
      method: "POST",
      json: { destino_id: destinoId },
    });
  },
  /** Si el destino vive en otro concepto, los movimientos cambian de concepto: hay que confirmarlo. */
  fusionarClasificacion(id: string, destinoId: string, confirmarCambioDeConcepto = false): Promise<ResumenFusion> {
    return apiFetch<ResumenFusion>(`/bancos/catalogo/clasificaciones/${id}/fusionar`, {
      method: "POST",
      json: { destino_id: destinoId, confirmar_cambio_de_concepto: confirmarCambioDeConcepto },
    });
  },
  conceptos(ambito?: AmbitoCatalogo): Promise<ConceptoCatalogo[]> {
    return apiFetch<ConceptoCatalogo[]>("/bancos/catalogo/conceptos", {
      method: "GET",
      query: ambito ? { ambito } : {},
    });
  },
  clasificaciones(ambito?: AmbitoCatalogo): Promise<ClasificacionCatalogo[]> {
    return apiFetch<ClasificacionCatalogo[]>("/bancos/catalogo/clasificaciones", {
      method: "GET",
      query: ambito ? { ambito } : {},
    });
  },
  cambiarVisibilidadCxP(id: string, visible: boolean): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/conceptos/${id}`, {
      method: "PATCH",
      json: { visible_cxp: visible },
    });
  },
  /** Declara si el concepto suma a ingresos, a gastos, o no entra al EBITDA. */
  cambiarNaturaleza(id: string, naturaleza: NaturalezaConcepto): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/conceptos/${id}`, {
      method: "PATCH",
      json: { naturaleza },
    });
  },
  crearConcepto(input: ConceptoInput): Promise<ConceptoCatalogo> {
    return apiFetch<ConceptoCatalogo>("/bancos/catalogo/conceptos", { method: "POST", json: input });
  },
  crearClasificacion(input: ClasificacionInput): Promise<ClasificacionCatalogo> {
    return apiFetch<ClasificacionCatalogo>("/bancos/catalogo/clasificaciones", {
      method: "POST",
      json: input,
    });
  },
  renombrarConcepto(id: string, nombre: string): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/conceptos/${id}`, { method: "PATCH", json: { nombre } });
  },
  /** Elimina un concepto SIN referencias (en uso → 422 con el detalle). */
  eliminarConcepto(id: string): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/conceptos/${id}`, { method: "DELETE" });
  },
  renombrarClasificacion(id: string, nombre: string): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/clasificaciones/${id}`, { method: "PATCH", json: { nombre } });
  },
  /** Mueve una clasificación (sin uso) a otro concepto (en uso → 422 con el detalle). */
  reasignarConceptoClasificacion(id: string, conceptoId: string): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/clasificaciones/${id}`, { method: "PATCH", json: { concepto_id: conceptoId } });
  },
  /** Elimina una clasificación SIN referencias (en uso → 422 con el detalle). */
  eliminarClasificacion(id: string): Promise<void> {
    return apiFetch<void>(`/bancos/catalogo/clasificaciones/${id}`, { method: "DELETE" });
  },

  // --- Importaciones ---
  /** multipart: sube el .xlsx y devuelve el PreviewResult. */
  importar(cuentaBancariaId: string, archivo: File): Promise<PreviewResult> {
    const fd = new FormData();
    fd.append("cuenta_bancaria_id", cuentaBancariaId);
    fd.append("archivo", archivo);
    return apiFetch<PreviewResult>("/bancos/importaciones", { method: "POST", raw: fd });
  },
  preview(importacionId: string): Promise<PreviewResult> {
    return apiFetch<PreviewResult>(`/bancos/importaciones/${importacionId}/preview`, {
      method: "GET",
    });
  },
  confirmar(importacionId: string, excluir: string[]): Promise<ConfirmarResult> {
    return apiFetch<ConfirmarResult>(`/bancos/importaciones/${importacionId}/confirmar`, {
      method: "POST",
      json: { excluir },
    });
  },

  // --- Movimientos ---
  /** Resumen (cuántos y cuánto) de la selección activa: mismos filtros que la lista. */
  resumenSeleccion(filtros: FiltrosMovimientos, agrupar: AgruparResumen): Promise<ResumenSeleccion> {
    // page/page_size no aplican: el resumen es de TODO el filtro, no de la página.
    const { page: _p, page_size: _ps, ...resto } = filtros;
    return apiFetch<ResumenSeleccion>("/bancos/movimientos/resumen", {
      method: "GET",
      query: { ...resto, agrupar },
    });
  },
  movimientos(filtros: FiltrosMovimientos): Promise<ListaMovimientos> {
    return apiFetch<ListaMovimientos>("/bancos/movimientos", {
      method: "GET",
      query: { ...filtros },
    });
  },
  /** Trae TODOS los movimientos que cumplen el filtro, paginando hasta agotar (para exportar). */
  async todosMovimientos(filtros: FiltrosMovimientos): Promise<MovimientoRow[]> {
    const acc: MovimientoRow[] = [];
    const size = 500;
    let page = 1;
    for (;;) {
      const r = await bancosApi.movimientos({ ...filtros, page, page_size: size });
      acc.push(...r.items);
      if (r.items.length === 0 || acc.length >= r.total) break;
      page++;
    }
    return acc;
  },
  clasificar(
    movimientoId: string,
    conceptoId: string,
    clasificacionId: string,
  ): Promise<void> {
    return apiFetch<void>(`/bancos/movimientos/${movimientoId}/clasificacion`, {
      method: "PATCH",
      json: { concepto_id: conceptoId, clasificacion_id: clasificacionId },
    });
  },

  // --- Reglas (motor que aprende) ---
  reglas(): Promise<Regla[]> {
    return apiFetch<Regla[]>("/bancos/reglas", { method: "GET" });
  },
  crearRegla(regla: ReglaClasificacionInput): Promise<ReglaResult> {
    return apiFetch<ReglaResult>("/bancos/reglas", { method: "POST", json: regla });
  },
  actualizarRegla(id: string, input: ReglaUpdateInput): Promise<void> {
    return apiFetch<void>(`/bancos/reglas/${id}`, { method: "PATCH", json: input });
  },
  eliminarRegla(id: string): Promise<void> {
    return apiFetch<void>(`/bancos/reglas/${id}`, { method: "DELETE" });
  },
  /** Propuesta de regla tras clasificar a mano (banner "¿Crear regla?"). */
  sugerenciaRegla(movimientoId: string): Promise<SugerenciaRegla> {
    return apiFetch<SugerenciaRegla>("/bancos/reglas/sugerencia", {
      method: "GET",
      query: { movimiento_id: movimientoId },
    });
  },
  /** Clasifica un bloque de movimientos en un solo golpe (quedan REVISADO). */
  clasificarMasivo(
    movimientoIds: string[],
    conceptoId: string,
    clasificacionId: string,
  ): Promise<{ clasificados: number }> {
    return apiFetch<{ clasificados: number }>("/bancos/movimientos/clasificar-masivo", {
      method: "POST",
      json: { movimiento_ids: movimientoIds, concepto_id: conceptoId, clasificacion_id: clasificacionId },
    });
  },
  /** Conteos por estado (KPI % auto-clasificado). Sin período = histórico completo. */
  resumenClasificacion(periodo?: string): Promise<ResumenClasif> {
    return apiFetch<ResumenClasif>("/bancos/clasificacion/resumen", {
      method: "GET",
      query: periodo ? { periodo } : {},
    });
  },

  // --- Tipo de cambio ---
  registrarCotizacion(cotizacion: CotizacionInput): Promise<void> {
    return apiFetch<void>("/bancos/cotizaciones", { method: "POST", json: cotizacion });
  },
  tipoCambio(anio: number, mes: number): Promise<TipoCambioMes> {
    return apiFetch<TipoCambioMes>(`/bancos/tipo-cambio/${anio}/${mes}`, { method: "GET" });
  },
  congelarTipoCambio(anio: number, mes: number): Promise<void> {
    return apiFetch<void>(`/bancos/tipo-cambio/${anio}/${mes}/congelar`, { method: "POST" });
  },
  sincronizarBCCR(fecha?: string): Promise<ResultadoSync> {
    return apiFetch<ResultadoSync>("/bancos/tipo-cambio/sync", {
      method: "POST",
      json: fecha ? { fecha } : {},
    });
  },
  ultimoSyncBCCR(): Promise<UltimoSync> {
    return apiFetch<UltimoSync>("/bancos/tipo-cambio/ultimo-sync", { method: "GET" });
  },

  // --- Parámetros por empresa (Fase D) ---
  parametros(): Promise<Parametros> {
    return apiFetch<Parametros>("/bancos/parametros", { method: "GET" });
  },
  actualizarTolerancia(toleranciaPct: string): Promise<void> {
    return apiFetch<void>("/bancos/parametros/tolerancia", {
      method: "PATCH",
      json: { tolerancia_pct: toleranciaPct },
    });
  },

  // --- Exportaciones .xlsx (Fase D) — devuelven el binario para descargar ---
  /**
   * Exporta con los MISMOS filtros de la hoja de trabajo, más las listas en plural. Las listas
   * van separadas por comas: el backend acepta esa forma y también el parámetro repetido.
   */
  /**
   * `agrupar` es PRESENTACIÓN, no filtro: "partida" saca bandas Concepto › Clasificación con
   * subtotales, "ninguno" saca el listado corrido por fecha (con la partida en columnas y
   * autofiltro). Por defecto agrupado, que es la forma ya aprobada.
   */
  exportarMovimientosXLSX(f: FiltrosMovimientos, agrupar: AgruparReporte = "partida"): Promise<DescargaArchivo> {
    return apiFetch<DescargaArchivo>("/bancos/exportaciones/movimientos", {
      method: "GET",
      query: { ...f, agrupar },
      // El nombre lo pone el SERVIDOR, que es el único que sabe si los meses pedidos son
      // contiguos («2026-01_a_2026-03») o sueltos («seleccion-3-periodos»). El cliente lo
      // reinventaba como primero_a_último y con huecos bajaba un archivo que anunciaba ocho
      // meses trayendo dos.
      blobConNombre: true,
    });
  },
  exportarCuadreXLSX(periodo: string): Promise<Blob> {
    return apiFetch<Blob>("/bancos/exportaciones/cuadre", {
      method: "GET",
      query: { periodo },
      blob: true,
    });
  },

  // --- Cuadre / dashboard ---
  cuadre(periodo: string): Promise<CuadreConcepto[]> {
    return apiFetch<CuadreConcepto[]>("/bancos/cuadre", { method: "GET", query: { periodo } });
  },
  cuadreArbol(periodo: string): Promise<CuadreArbol> {
    return apiFetch<CuadreArbol>("/bancos/cuadre/arbol", { method: "GET", query: { periodo } });
  },
  dashboard(periodo: string): Promise<DashboardData> {
    return apiFetch<DashboardData>("/bancos/dashboard", { method: "GET", query: { periodo } });
  },

  // --- Análisis visual (Fase B) ---
  serieMensual(hasta: string, meses = 12): Promise<SerieMensualPunto[]> {
    return apiFetch<SerieMensualPunto[]>("/bancos/analisis/serie-mensual", {
      method: "GET",
      query: { hasta, meses },
    });
  },
  analisisPartidas(desde: string, hasta: string): Promise<AnalisisPartidas> {
    return apiFetch<AnalisisPartidas>("/bancos/analisis/partidas", {
      method: "GET",
      query: { desde, hasta },
    });
  },
  calendarioDiario(periodo: string): Promise<DiaCalendario[]> {
    return apiFetch<DiaCalendario[]>("/bancos/analisis/calendario", {
      method: "GET",
      query: { periodo },
    });
  },
  resumenPorCuenta(periodo: string): Promise<CuentaResumen[]> {
    return apiFetch<CuentaResumen[]>("/bancos/analisis/cuentas", {
      method: "GET",
      query: { periodo },
    });
  },

  // --- Proyecciones (Fase C) ---
  proyeccion(periodo: string, metodo: MetodoProyeccion, metaPct: string): Promise<ProyeccionResultado> {
    return apiFetch<ProyeccionResultado>("/bancos/proyecciones", {
      method: "GET",
      query: { periodo, metodo, meta_pct: metaPct },
    });
  },
  guardarEscenario(
    periodo: string,
    metodo: MetodoProyeccion,
    metaPct: string,
  ): Promise<{ escenario_id: string; resultado: ProyeccionResultado }> {
    return apiFetch<{ escenario_id: string; resultado: ProyeccionResultado }>("/bancos/proyecciones", {
      method: "POST",
      json: { periodo, metodo, meta_crecimiento_pct: metaPct },
    });
  },
  escenarios(periodo?: string): Promise<EscenarioGuardado[]> {
    return apiFetch<EscenarioGuardado[]>("/bancos/proyecciones/escenarios", {
      method: "GET",
      query: periodo ? { periodo } : {},
    });
  },

  // --- Traslados ---
  propuestasTraslado(periodo: string): Promise<PropuestaTraslado[]> {
    return apiFetch<PropuestaTraslado[]>("/bancos/traslados/propuestas", {
      method: "GET",
      query: { periodo },
    });
  },
  emparejar(movimientoDebitoId: string, movimientoCreditoId: string): Promise<void> {
    return apiFetch<void>("/bancos/traslados/emparejar", {
      method: "POST",
      json: {
        movimiento_debito_id: movimientoDebitoId,
        movimiento_credito_id: movimientoCreditoId,
      },
    });
  },
  desemparejar(movimientoId: string): Promise<void> {
    return apiFetch<void>("/bancos/traslados/desemparejar", {
      method: "POST",
      json: { movimiento_id: movimientoId },
    });
  },

  // --- Períodos ---
  periodo(anio: number, mes: number): Promise<PeriodoEstado> {
    return apiFetch<PeriodoEstado>(`/bancos/periodos/${anio}/${mes}`, { method: "GET" });
  },
  cerrarPeriodo(anio: number, mes: number): Promise<void> {
    return apiFetch<void>(`/bancos/periodos/${anio}/${mes}/cerrar`, { method: "POST" });
  },

  // --- Diccionario del catálogo (Concepto › Clasificación + palabras clave) ---
  // --- Traer la clasificación hecha en Excel ---

  /** Baja la plantilla que se llena en Excel y se vuelve a subir. El nombre lo pone el servidor. */
  plantillaClasificacion(
    desde: string,
    hasta: string,
    soloSinClasificar = true,
  ): Promise<{ blob: Blob; filename: string }> {
    return apiFetch<{ blob: Blob; filename: string }>(
      "/bancos/movimientos/plantilla-clasificacion",
      {
        method: "GET",
        query: { desde, hasta, solo_sin_clasificar: soloSinClasificar ? "true" : "false" },
        blobConNombre: true,
      },
    );
  },

  /**
   * Sube el Excel con las partidas. Sin `aplicar` solo previsualiza: no escribe nada.
   * `reemplazar` cambia también lo que YA tenía otra partida; por defecto eso no se toca.
   */
  clasificarDesdeExcel(
    archivo: File,
    opts: { cuentaBancariaId?: string; reemplazar?: boolean; aplicar?: boolean } = {},
  ): Promise<PlanClasifExcel> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    if (opts.cuentaBancariaId) fd.append("cuenta_bancaria_id", opts.cuentaBancariaId);
    return apiFetch<PlanClasifExcel>("/bancos/movimientos/clasificar-excel", {
      method: "POST",
      raw: fd,
      query: {
        ...(opts.aplicar ? { aplicar: "true" } : {}),
        ...(opts.reemplazar ? { reemplazar: "true" } : {}),
      },
    });
  },

  exportarDiccionario(): Promise<Blob> {
    return apiFetch<Blob>("/bancos/catalogo/diccionario", { method: "GET", blob: true });
  },
  /** aplicar=false previsualiza; true escribe. El plan que se ve es el que se aplica. */
  importarDiccionario(archivo: File, aplicar: boolean): Promise<PlanDiccionario> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    return apiFetch<PlanDiccionario>("/bancos/catalogo/diccionario", {
      method: "POST",
      raw: fd,
      query: aplicar ? { aplicar: "true" } : {},
    });
  },

  // --- Patrones sin clasificar ---
  patrones(periodo?: string): Promise<PatronSugerido[]> {
    return apiFetch<PatronSugerido[]>("/bancos/patrones", {
      method: "GET",
      query: periodo ? { periodo } : {},
    });
  },

  // --- Tesorería: saldo diario, checklist de carga y conciliación (Tanda 1) ---
  tesoreria(fecha?: string): Promise<Tesoreria> {
    return apiFetch<Tesoreria>("/bancos/tesoreria", { method: "GET", query: fecha ? { fecha } : {} });
  },
  guardarSaldos(fecha: string, saldos: SaldoInput[]): Promise<{ guardados: number }> {
    return apiFetch<{ guardados: number }>("/bancos/saldos", { method: "PUT", json: { fecha, saldos } });
  },
  carga(periodo?: string): Promise<CargaCuenta[]> {
    return apiFetch<CargaCuenta[]>("/bancos/carga", { method: "GET", query: periodo ? { periodo } : {} });
  },
  revisarSaldos(fecha: string, congelar: boolean, motivo?: string): Promise<{ cuentas: number; congelados: boolean }> {
    return apiFetch<{ cuentas: number; congelados: boolean }>("/bancos/saldos/revisar", {
      method: "POST",
      json: { fecha, congelar, motivo: motivo ?? "" },
    });
  },
  conciliacion(periodo: string): Promise<Conciliacion> {
    return apiFetch<Conciliacion>("/bancos/conciliacion", { method: "GET", query: { periodo } });
  },
  registrarPartida(input: PartidaInput): Promise<{ id: string }> {
    return apiFetch<{ id: string }>("/bancos/conciliacion/partidas", { method: "POST", json: input });
  },
  anularPartida(id: string): Promise<void> {
    return apiFetch<void>(`/bancos/conciliacion/partidas/${id}`, { method: "DELETE" });
  },
  firmarActa(cuentaId: string, periodo: string): Promise<void> {
    return apiFetch<void>("/bancos/conciliacion/firmar", {
      method: "POST",
      json: { cuenta_id: cuentaId, periodo },
    });
  },
};

// --- Diccionario del catálogo ---

export interface AccionDiccionario {
  linea: number;
  concepto: string;
  clasificacion: string;
  crear_concepto: boolean;
  crear_clasificacion: boolean;
  crear_regla: boolean;
  palabras: string;
  aplica_a: string;
  /** Vacío = la fila se puede aplicar; con texto = se omite y dice por qué. */
  problema: string;
}

export interface AccionDiccionarioNaturaleza {
  /** El concepto no tenía naturaleza declarada y esta fila la declara. */
  declarar_naturaleza: boolean;
  naturaleza: string;
  /** La fila trae otra naturaleza que la ya declarada: se avisa, no se cambia. */
  aviso_naturaleza: string;
}

export interface PlanDiccionario {
  filas: number;
  acciones: AccionDiccionario[];
  conceptos_nuevos: number;
  clasificaciones_nuevas: number;
  reglas_nuevas: number;
  /** Conceptos cuya naturaleza NADIE había declarado y el archivo declara. */
  naturalezas_declaradas: number;
  /** Filas cuya naturaleza discrepa de la ya declarada: se avisan y NO se cambian. */
  naturalezas_en_conflicto: number;
  sin_cambios: number;
  omitidas: number;
  /** Movimientos que las reglas creadas dejaron clasificados (solo al aplicar). */
  clasificados: number;
  /** false = fue una previsualización. */
  aplicado: boolean;
}

// --- Patrones sin clasificar ---

/** Por qué un grupo no trae palabra clave propuesta. */
export type MotivoPatron = "" | "SOLO_REFERENCIAS" | "SIN_PALABRA_SEGURA";

export interface PatronSugerido {
  /** Palabra clave propuesta. Vacía cuando no hay una segura (ver `motivo`). */
  patron: string;
  motivo: MotivoPatron;
  /** El otro candidato (más corto o más específico), por si se prefiere. */
  alterna: string;
  aplica_a: AplicaA;
  movimientos: number;
  creditos: number;
  debitos: number;
  monto: string;
  ejemplos: string[];
  /** El patrón arrastra un año: dejaría de calzar cuando cambie. */
  aviso_anio: boolean;
  /** Movimientos de la empresa que contienen el patrón (los ya clasificados incluidos). */
  alcance: number;
  /** De esos, cuántos son de otra forma. Cero es lo esperado. */
  ajenos: number;
}

// --- Tesorería (Tanda 1) ---

/** Veredicto del cuadre de un saldo capturado contra los movimientos cargados. */
export type Cuadre = "CUADRA" | "DIFIERE" | "SIN_CAPTURA" | "SIN_ANTERIOR";

/** Antigüedad de la carga de estados de cuenta de una cuenta. */
export type EstadoCarga = "AL_DIA" | "ATRASADA" | "REZAGADA" | "SIN_CARGA";

export interface SaldoDelDia {
  cuenta_id: string;
  alias: string;
  banco: string;
  moneda: string;
  saldo_anterior: string;
  fecha_anterior: string;
  entradas_dia: string;
  salidas_dia: string;
  saldo_esperado: string;
  saldo: string;
  nota: string;
  capturado_en: string;
  congelado: boolean;
  revisado_en: string;
  diferencia: string;
  cuadre: Cuadre;
  ultimo_movimiento: string;
  dias_sin_cargar: number;
}

export interface TotalMoneda {
  moneda: string;
  monto: string;
  cuentas: number;
  capturadas: number;
}

export interface TotalBanco {
  banco: string;
  monto_crc: string;
  monto_usd: string;
  cuentas: number;
  sin_capturar: number;
}

export interface PuntoSaldo {
  fecha: string;
  monto_crc: string;
  capturadas: number;
  es_hoy: boolean;
}

export interface Tesoreria {
  fecha: string;
  hoy: string;
  saldos: SaldoDelDia[];
  totales: TotalMoneda[];
  bancos: TotalBanco[];
  serie: PuntoSaldo[];
  cuentas: number;
  sin_capturar: number;
  no_cuadran: number;
  atrasadas: number;
  rezagadas: number;
  congeladas: number;
  dia_revisado: boolean;
}

export interface SaldoInput {
  cuenta_id: string;
  saldo: string;
  nota?: string;
}

export interface CargaCuenta {
  cuenta_id: string;
  alias: string;
  banco: string;
  moneda: string;
  movimientos: number;
  ultimo_movimiento: string;
  dias_sin_cargar: number;
  estado: EstadoCarga;
}

// --- Conciliación bancaria mensual ---

/** Tipos de partida en tránsito. El signo lo fija el tipo, salvo OTRA. */
export type TipoPartida =
  | "DEPOSITO_NO_ACREDITADO"
  | "TRANSFERENCIA_NO_PRESENTADA"
  | "CARGO_BANCO_NO_REGISTRADO"
  | "ABONO_BANCO_NO_REGISTRADO"
  | "OTRA";

export interface PartidaConciliacion {
  id: string;
  cuenta_id: string;
  tipo: TipoPartida;
  descripcion: string;
  monto: string;
  signo: number;
  registrado_en: string;
  registrado_por: string;
}

export interface PartidaInput {
  cuenta_id: string;
  periodo: string;
  tipo: TipoPartida;
  descripcion: string;
  monto: string;
  /** Solo se usa (y se exige) cuando el tipo es OTRA. */
  signo?: number;
}

export interface ActaConciliacion {
  cuenta_id: string;
  alias: string;
  banco: string;
  moneda: string;
  anio: number;
  mes: number;
  saldo_banco: string;
  fecha_banco: string;
  saldo_libros: string;
  saldo_inicial: string;
  fecha_inicial: string;
  entradas_mes: string;
  salidas_mes: string;
  ajuste_partidas: string;
  partidas: PartidaConciliacion[];
  diferencia_sin_explicar: string;
  cuadra: boolean;
  firmado_en: string;
  firmado_por: string;
  impedimento: "" | "SIN_SALDO_BANCO" | "SIN_SALDO_INICIAL";
}

export interface Conciliacion {
  anio: number;
  mes: number;
  periodo: string;
  cerrado: boolean;
  actas: ActaConciliacion[];
  cuentas: number;
  firmadas: number;
  cuadran: number;
  con_diferencia: number;
  incompletas: number;
  puede_cerrar: boolean;
}
