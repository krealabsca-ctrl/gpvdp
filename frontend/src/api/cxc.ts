/**
 * Cliente de Cuentas por Cobrar (fase 1: cartera y cargos).
 *
 * El modelo son PARTIDAS ABIERTAS: cada período de cada contrato es un cargo con su
 * vencimiento y su saldo. El saldo del contrato, los días de mora y el tramo NO se
 * guardan: los deriva el servidor de los cargos abiertos.
 */

import { apiFetch } from "@/api/client";

export interface ContratoCxc {
  id: string;
  numero: string;
  sede_id: string;
  sede: string;
  cliente_nombre: string;
  documento: string;
  telefonos: string;
  correos: string;
  servicio: string;
  tipo_servicio: string;
  modalidad: string;
  forma_pago: string;
  asociacion: string;
  dia_pago: number | null;
  cuota_vigente: string;
  fecha_inicial: string;
  fecha_primer_cobro: string;
  tarjeta_vence: string;
  estado: string;
  /** Derivados de los cargos abiertos. */
  cargos_abiertos: number;
  saldo: string;
  dias_mora_max: number;
  tramo: string;
  revision_pendiente: boolean;
  revision_motivo: string;
  /** Del sistema de origen: informativos, para la corrida en paralelo. */
  score_origen: number | null;
  morosidad_origen: string;
  dias_vencidos_origen: number | null;
  saldo_origen: string | null;
}

export interface CargoCxc {
  id: string;
  periodo: string;
  vence_en: string;
  monto: string;
  monto_aplicado: string;
  saldo: string;
  estado: string;
  origen: string;
  dias_mora: number;
  tramo: string;
  tramo_etiqueta: string;
}

export interface ResumenCartera {
  contratos: number;
  con_saldo: number;
  saldo: string;
  vencido: string;
  por_vencer: string;
  cargos_abiertos: number;
}

export interface ListaContratosCxc {
  resumen: ResumenCartera;
  items: ContratoCxc[];
  total: number;
  page: number;
  page_size: number;
}

export interface FiltrosCartera {
  q?: string;
  sede_id?: string;
  modalidad_id?: string;
  forma_pago_id?: string;
  asociacion_id?: string;
  estado?: string;
  con_saldo?: boolean;
  en_revision?: boolean;
  orden?: string;
  page?: number;
  page_size?: number;
}

export interface ItemCatalogoCxc {
  id: string;
  nombre: string;
  contratos: number;
}

export interface TramoCxc {
  codigo: string;
  etiqueta: string;
  dias_min: number;
  dias_max: number;
  prob_recuperacion: string;
}

export interface CatalogosCxc {
  sedes: ItemCatalogoCxc[];
  modalidades: ItemCatalogoCxc[];
  formas_pago: ItemCatalogoCxc[];
  asociaciones: ItemCatalogoCxc[];
  tramos: TramoCxc[];
}

/** Una fila del archivo ya interpretada, con los motivos por los que iría a cuarentena. */
export interface FilaContratoCxc {
  linea: number;
  numero: string;
  cliente: string;
  documento: string;
  sede_cruda: string;
  razon_social: string;
  plaza: string;
  modalidad: string;
  forma_pago: string;
  asociacion: string;
  dia_pago: number;
  cuota: string;
  primer_cobro: string;
  tarjeta_vence: string;
  motivos: string[];
}

export interface ResolucionCxc {
  sedes_nuevas: string[] | null;
  asociaciones_nuevas: string[] | null;
  modalidades_desconocidas: string[] | null;
  formas_pago_desconocidas: string[] | null;
}

/** El reporte que se muestra ANTES de confirmar: nada entra en silencio. */
export interface ConciliacionCxc {
  filas: number;
  nuevos: number;
  actualizados: number;
  duplicados: number;
  cuarentena: number;
  sin_sede: number;
  resolucion: ResolucionCxc;
  muestra: FilaContratoCxc[];
  problemas: FilaContratoCxc[];
}

export interface PrevisualizacionCxc {
  importacion_id: string;
  reporte: ConciliacionCxc;
}

export interface AplicadoCxc {
  nuevos: number;
  actualizados: number;
}

/** Plan del generador de cargos: qué crearía ANTES de crearlo. */
export interface PlanCargos {
  desde: string;
  hasta: string;
  contratos: number;
  cargos: number;
  excluidos: Record<string, number> | null;
  sobre_el_tope: boolean;
  tope: number;
}

export interface Contrato360 {
  contrato: ContratoCxc;
  cargos: CargoCxc[];
}

export const cxcApi = {
  catalogos(): Promise<CatalogosCxc> {
    return apiFetch<CatalogosCxc>("/cxc/catalogos", { method: "GET" });
  },
  contratos(filtros: FiltrosCartera): Promise<ListaContratosCxc> {
    return apiFetch<ListaContratosCxc>("/cxc/contratos", { method: "GET", query: { ...filtros } });
  },
  contrato(numero: string, soloAbiertos = false): Promise<Contrato360> {
    return apiFetch<Contrato360>(`/cxc/contratos/${encodeURIComponent(numero)}`, {
      method: "GET",
      query: { solo_abiertos: soloAbiertos },
    });
  },
  /** Lee el archivo y devuelve el reporte SIN escribir en la cartera. */
  previsualizarContratos(archivo: File): Promise<PrevisualizacionCxc> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    return apiFetch<PrevisualizacionCxc>("/cxc/importaciones/contratos/previsualizar", {
      method: "POST",
      raw: fd,
    });
  },
  /** Se manda el MISMO archivo: la fuente de verdad es el archivo, no un estado a medias. */
  confirmarContratos(archivo: File, importacionId: string): Promise<{ reporte: ConciliacionCxc; aplicado: AplicadoCxc }> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    fd.append("importacion_id", importacionId);
    return apiFetch<{ reporte: ConciliacionCxc; aplicado: AplicadoCxc }>(
      "/cxc/importaciones/contratos/confirmar",
      { method: "POST", raw: fd },
    );
  },
  planCargos(desde: string, hasta?: string): Promise<PlanCargos> {
    return apiFetch<PlanCargos>("/cxc/cargos/plan", { method: "GET", query: { desde, hasta } });
  },
  /** `total` es el número que el usuario vio en el plan: si cambió, el servidor aborta. */
  generarCargos(desde: string, hasta: string, total: number): Promise<{ plan: PlanCargos; creados: number }> {
    return apiFetch<{ plan: PlanCargos; creados: number }>("/cxc/cargos/generar", {
      method: "POST",
      json: { desde, hasta, total },
    });
  },
};

// ───────────────────────── Cobros (fase 2) ─────────────────────────

export interface CobroFila {
  id: string;
  contrato: string;
  /** El número que traía el archivo. Si el contrato no está en la cartera, es la pista. */
  contrato_origen: string;
  cliente: string;
  consecutivo: string;
  fecha_pago: string;
  fecha_bancaria: string;
  monto: string;
  aplicado: string;
  saldo_a_favor: string;
  forma_pago: string;
  asociacion: string;
  referencia: string;
  concepto_origen: string;
  estado: "APLICADO" | "SIN_IDENTIFICAR" | "REVERSADO";
  origen: string;
  /** A qué períodos fue el cobro, ya resuelto por el servidor («2026-07 + 2026-08»). */
  periodos: string;
}

export interface ResumenCobros {
  cobros: number;
  monto: string;
  aplicado: string;
  saldo_a_favor: string;
  sin_identificar: number;
  reversados: number;
}

export interface ListaCobros {
  resumen: ResumenCobros;
  items: CobroFila[];
  total: number;
  page: number;
  page_size: number;
}

export interface FiltrosCobros {
  q?: string;
  contrato?: string;
  asociacion_id?: string;
  estado?: string;
  desde?: string;
  hasta?: string;
  sin_identificar?: boolean;
  page?: number;
  page_size?: number;
}

/** Una aplicación: qué parte del cobro fue a qué cargo. */
export interface AplicacionCobro {
  cargo_id: string;
  periodo: string;
  monto: string;
  parcial: boolean;
  estado_final: string;
}

export interface CobroRegistrado {
  id: string;
  contrato: string;
  consecutivo: string;
  monto: string;
  estado: string;
  aplicaciones: AplicacionCobro[];
  saldo_a_favor: string;
  /** true = el cobro ya existía y se devolvió el mismo (idempotencia). */
  repetido: boolean;
}

export interface NuevoCobro {
  contrato?: string;
  consecutivo?: string;
  fecha_pago: string;
  fecha_bancaria?: string;
  monto: string;
  forma_pago?: string;
  asociacion?: string;
  referencia?: string;
  concepto?: string;
  origen?: string;
  /** Cargos elegidos por el operador. Vacío = FIFO (más viejo primero). */
  destinos?: string[];
}

export interface ConciliacionCobros {
  filas: number;
  aplicables: number;
  sin_identificar: number;
  repetidos: number;
  anulados: number;
  cuarentena: number;
  monto: string;
  /** Cuántos traen el período en el campo Concepto del origen. */
  con_detalle: number;
  muestra: FilaCobroLeida[];
  problemas: FilaCobroLeida[];
}

export interface FilaCobroLeida {
  linea: number;
  contrato: string;
  cliente: string;
  consecutivo: string;
  forma_pago: string;
  asociacion: string;
  monto: string;
  fecha_pago: string;
  fecha_bancaria: string;
  fecha_registro: string;
  concepto: string;
  aplicaciones: { periodo: string; monto: string; parcial: boolean }[] | null;
  motivos: string[] | null;
}

export interface AplicadoCobros {
  registrados: number;
  repetidos: number;
  sin_identificar: number;
  aplicado: string;
  saldo_a_favor: string;
}

export const cobrosApi = {
  listar(filtros: FiltrosCobros): Promise<ListaCobros> {
    return apiFetch<ListaCobros>("/cxc/cobros", { method: "GET", query: { ...filtros } });
  },
  registrar(cobro: NuevoCobro): Promise<CobroRegistrado> {
    return apiFetch<CobroRegistrado>("/cxc/cobros", { method: "POST", json: cobro });
  },
  reversar(id: string, motivo: string): Promise<{ ok: boolean }> {
    return apiFetch<{ ok: boolean }>(`/cxc/cobros/${id}/reversar`, { method: "POST", json: { motivo } });
  },
  identificar(id: string, contrato: string): Promise<CobroRegistrado> {
    return apiFetch<CobroRegistrado>(`/cxc/cobros/${id}/identificar`, { method: "POST", json: { contrato } });
  },
  previsualizar(archivo: File): Promise<{ importacion_id: string; reporte: ConciliacionCobros }> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    return apiFetch<{ importacion_id: string; reporte: ConciliacionCobros }>(
      "/cxc/importaciones/cobros/previsualizar",
      { method: "POST", raw: fd },
    );
  },
  confirmar(archivo: File, importacionId: string): Promise<{ reporte: ConciliacionCobros; aplicado: AplicadoCobros; fallas: string[] }> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    fd.append("importacion_id", importacionId);
    return apiFetch<{ reporte: ConciliacionCobros; aplicado: AplicadoCobros; fallas: string[] }>(
      "/cxc/importaciones/cobros/confirmar",
      { method: "POST", raw: fd },
    );
  },
};

// ─────────────── Panorama de asociaciones (el canal dominante) ───────────────

export interface FilaAsociacion {
  asociacion_id: string;
  asociacion: string;
  patrono: string;
  contratos: number;
  /** Suma de los cargos que VENCEN en el período para los contratos de esa asociación. */
  esperado: string;
  cargos_del_periodo: number;
  cobrado: string;
  cobros: number;
  /** cobrado − esperado. Negativa = faltó plata. */
  diferencia: string;
  fechas_bancarias: string[];
  /** Tiene cargos del período pero NO llegó ningún cobro: no envió planilla. */
  sin_planilla: boolean;
  /** Lo que de verdad entró al banco: los movimientos VINCULADOS a la planilla. */
  depositado: string;
  /** depositado − cobrado. Si no es cero, es un hallazgo que hay que mirar. */
  diferencia_deposito: string;
  planilla_id: string;
  referencia: string;
  depositos: number;
  /** SIN_CARGOS · NO_ENVIO · SIN_DEPOSITO · CONCILIADA · CON_DIFERENCIA */
  estado: string;
}

export interface PanoramaAsociaciones {
  periodo: string;
  asociaciones: number;
  con_planilla: number;
  sin_planilla: number;
  esperado: string;
  cobrado: string;
  /** El esperado de las que no enviaron: plata que no entró por un tercero. */
  en_riesgo: string;
  contratos_en_riesgo: number;
  /** Total que de verdad entró al banco por este canal en el período. */
  depositado: string;
  conciliadas: number;
  sin_deposito: number;
  con_diferencia: number;
  filas: FilaAsociacion[];
}

export const asociacionesApi = {
  panorama(periodo: string): Promise<PanoramaAsociaciones> {
    return apiFetch<PanoramaAsociaciones>("/cxc/asociaciones/panorama", { method: "GET", query: { periodo } });
  },
};

// ─────────────── Gestión de cobro (fase 3): la cola por valor esperado ───────────────

/**
 * Una fila de la cola. El orden lo decide `valor_esperado`:
 *
 *     vencido × probabilidad del tramo × factor de la forma de pago
 *
 * Ordenar por antigüedad pondría primero los casos MENOS recuperables (a los +180 días se
 * les recupera un 5 %); ordenar por valor esperado pone primero los que cambian el mes.
 */
export interface FilaCola {
  contrato_id: string;
  numero: string;
  cliente: string;
  documento: string;
  telefonos: string;
  correos: string;
  sede: string;
  forma_pago: string;
  asociacion: string;
  modalidad: string;
  saldo: string;
  /** La parte del saldo YA exigible. Es la base del valor esperado. */
  vencido: string;
  cargos_abiertos: number;
  dias_mora: number;
  tramo: string;
  tramo_etiqueta: string;
  /** Qué hacer y por dónde, según el tramo (viene del catálogo, no del código). */
  estrategia: string;
  canal_sugerido: string;
  valor_esperado: string;
  ultima_gestion: string;
  ultimo_resultado: string;
  dias_sin_gestion: number | null;
  gestiones: number;
  promesa_fecha: string;
  promesa_monto: string;
  /** La fecha prometida pasó (con su tolerancia) y no entró el pago. */
  promesa_incumplida: boolean;
  /** Prometió y la fecha todavía no llega: razón para NO llamar hoy. */
  promesa_vigente: boolean;
  tarjeta_vence: string;
  tarjeta_vencida: boolean;
  /** Domiciliado cuya tarjeta caduca pronto: renovarla es más barato que cobrar el rechazo. */
  tarjeta_por_vencer: boolean;
  /** Cuántas cuotas vencieron sin pagarse. Es el hecho que se le dice al cliente. */
  cuotas_vencidas: number;
  /**
   * Esas cuotas convertidas a MESES según la modalidad. Es la medida que decide la suspensión,
   * porque la regla son 18 meses «o su equivalencia»: 18 cuotas de un quincenal son 9 meses.
   */
  meses_mora: string;
  /** Llegó al tope de meses. NO se suspende solo: lo decide una persona. */
  para_suspender: boolean;
  /** Ya se le cortó el servicio, pero sigue debiendo y sigue en la cola. */
  suspendido: boolean;
  /** '' si no tiene arreglo, o AL_DIA | EN_MORA | CUMPLIDO | QUEBRADO | ANULADO. */
  arreglo_estado: string;
  /** Quebró su arreglo de pago o llegó al tope de meses de mora. */
  en_cartera_morosa: boolean;
}

export interface ResumenCola {
  contratos: number;
  saldo: string;
  vencido: string;
  valor_esperado: string;
  sin_gestionar: number;
  con_promesa_incumplida: number;
  con_promesa_vigente: number;
  tarjetas_vencidas: number;
  tarjetas_por_vencer: number;
  para_suspender: number;
  suspendidos: number;
  arreglo_al_dia: number;
  arreglo_en_mora: number;
  cartera_morosa: number;
  /** El universo que la cola deja fuera a propósito: deben, pero todavía no vence. */
  por_vencer_contratos: number;
  por_vencer_monto: string;
}

export interface ListaCola {
  resumen: ResumenCola;
  items: FilaCola[];
  total: number;
  page: number;
  page_size: number;
}

export interface FiltrosCola {
  q?: string;
  sede_id?: string;
  forma_pago_id?: string;
  asociacion_id?: string;
  tramo?: string;
  sin_gestionar?: boolean;
  promesa_incumplida?: boolean;
  tarjeta_vencida?: boolean;
  tarjeta_por_vencer?: boolean;
  para_suspender?: boolean;
  /** Cartera morosa: quebró el arreglo o llegó al tope de meses. */
  morosa?: boolean;
  /** AL_DIA | EN_MORA | CON | SIN. */
  arreglo?: string;
  orden?: string;
  page?: number;
  page_size?: number;
}

export interface ResultadoGestion {
  id: string;
  codigo: string;
  etiqueta: string;
  /** Un «no contesta» no es contacto: por eso la contactabilidad se puede medir. */
  es_contacto: boolean;
  /** Si es true, la gestión no se puede guardar sin fecha de promesa. */
  exige_promesa: boolean;
}

export interface CatalogosGestion {
  canales: { id: string; nombre: string }[];
  resultados: ResultadoGestion[];
}

export interface NuevaGestion {
  contrato: string;
  canal_id: string;
  resultado_id: string;
  notas?: string;
  promesa_fecha?: string;
  promesa_monto?: string;
}

export interface GestionRegistrada {
  id: string;
  contrato: string;
  resultado: string;
  es_contacto: boolean;
  /** La foto del saldo al momento de gestionar: no se puede reconstruir después. */
  saldo_al_gestionar: string;
  tramo_al_gestionar: string;
  promesa_id: string;
}

export interface GestionFila {
  id: string;
  fecha: string;
  canal: string;
  resultado: string;
  es_contacto: boolean;
  notas: string;
  usuario: string;
  saldo_entonces: string;
  tramo_entonces: string;
  dias_mora_entonces: number;
  promesa_fecha: string;
  promesa_monto: string;
  /** null mientras la fecha (más su tolerancia) no pasa: todavía no se puede juzgar. */
  promesa_cumplida: boolean | null;
}

export const colaApi = {
  listar(filtros: FiltrosCola): Promise<ListaCola> {
    return apiFetch<ListaCola>("/cxc/cola", { method: "GET", query: { ...filtros } });
  },
  catalogosGestion(): Promise<CatalogosGestion> {
    return apiFetch<CatalogosGestion>("/cxc/gestiones/catalogos", { method: "GET" });
  },
  registrar(g: NuevaGestion): Promise<GestionRegistrada> {
    return apiFetch<GestionRegistrada>("/cxc/gestiones", { method: "POST", json: g });
  },
  gestiones(numero: string): Promise<GestionFila[]> {
    return apiFetch<GestionFila[]>(`/cxc/contratos/${encodeURIComponent(numero)}/gestiones`, { method: "GET" });
  },
};

// ─────────── Configuración del módulo (parámetros, tramos, sedes, frontera) ───────────

/**
 * Un parámetro clave/valor. `editable` lo decide el SERVIDOR según si el motor lee esa
 * clave: un parámetro que nadie lee se muestra bloqueado con su motivo, para que la
 * pantalla no prometa un cambio que no ocurre.
 */
export interface ParametroCxc {
  clave: string;
  valor: string;
  descripcion: string;
  actualizado_en: string;
  actualizado_por: string;
  editable: boolean;
  /** Dónde lo usa el motor. Vacío si todavía no lo usa nadie. */
  leido_por: string;
  /** Qué falta para que sirva (solo en los bloqueados). */
  nota: string;
  tipo: "entero" | "monto" | "fecha_opcional" | "opciones" | "";
  opciones: string[] | null;
}

export interface TramoConfig {
  codigo: string;
  etiqueta: string;
  dias_min: number;
  dias_max: number;
  orden: number;
  prob_recuperacion: string;
  estrategia: string;
  canal_sugerido: string;
  /** Cuántos contratos caen HOY en este tramo: sin eso, cambiar la probabilidad es a ciegas. */
  contratos: number;
  vencido: string;
}

export interface FormaPagoConfig {
  id: string;
  nombre: string;
  factor_recuperacion: string;
  es_asociacion: boolean;
  es_domiciliado: boolean;
  activa: boolean;
  contratos: number;
}

export interface SedeConfig {
  id: string;
  nombre: string;
  razon_social: string;
  plaza: string;
  activa: boolean;
  contratos: number;
  usuarios: number;
}

export interface UsuarioSedes {
  usuario_id: string;
  nombre: string;
  email: string;
  rol: string;
  /** Su rol ve TODA la cartera: asignarle sedes no lo limita. */
  ve_todas_sedes: boolean;
  /** Sin cxc.ver no entra al módulo: asignarle sedes no sirve de nada. */
  puede_ver_cxc: boolean;
  sede_ids: string[];
}

export interface ConfigCxc {
  parametros: ParametroCxc[];
  tramos: TramoConfig[];
  formas_pago: FormaPagoConfig[];
  sedes: SedeConfig[];
  usuarios: UsuarioSedes[];
}

export interface CambioTramo {
  prob_recuperacion?: string;
  estrategia?: string;
  canal_sugerido?: string;
  dias_min?: number;
  dias_max?: number;
}

export const configCxcApi = {
  cargar(): Promise<ConfigCxc> {
    return apiFetch<ConfigCxc>("/cxc/parametros", { method: "GET" });
  },
  /** Upsert de las claves mandadas. Si una está mal, el servidor rechaza el lote completo. */
  guardarParametros(valores: Record<string, string>): Promise<{ cambiados: number }> {
    return apiFetch<{ cambiados: number }>("/cxc/parametros", { method: "PUT", json: { valores } });
  },
  actualizarTramo(codigo: string, cambio: CambioTramo): Promise<void> {
    return apiFetch<void>(`/cxc/tramos/${encodeURIComponent(codigo)}`, { method: "PATCH", json: cambio });
  },
  actualizarFormaPago(id: string, factor: string): Promise<void> {
    return apiFetch<void>(`/cxc/formas-pago/${id}`, { method: "PATCH", json: { factor_recuperacion: factor } });
  },
  crearSede(nombre: string, plaza?: string): Promise<SedeConfig> {
    return apiFetch<SedeConfig>("/cxc/sedes", { method: "POST", json: { nombre, plaza: plaza ?? "" } });
  },
  actualizarSede(id: string, cambio: { nombre?: string; activa?: boolean }): Promise<void> {
    return apiFetch<void>(`/cxc/sedes/${id}`, { method: "PATCH", json: cambio });
  },
  /** Lista COMPLETA de sedes del usuario: lo que no va, se le quita. */
  asignarSedes(usuarioId: string, sedeIds: string[]): Promise<{ sedes: number }> {
    return apiFetch<{ sedes: number }>(`/cxc/usuarios/${usuarioId}/sedes`, {
      method: "PUT",
      json: { sede_ids: sedeIds },
    });
  },
};

// ─────────── Planillas de asociación: conciliación contra el depósito ───────────

/** Un depósito bancario ya vinculado a una planilla. */
export interface MovimientoVinculado {
  id: string;
  fecha: string;
  descripcion: string;
  monto: string;
  cuenta: string;
  banco: string;
}

/**
 * Un crédito de Bancos que PODRÍA ser el depósito de esta planilla.
 *
 * El operador elige, el sistema no adivina: con los datos reales de la empresa la
 * descripción del banco casi nunca dice de qué asociación es («TEF DE:ASOCIACION
 * SOLIDARISTA»), así que emparejar solo por nombre daría por conciliada a la equivocada.
 */
export interface CandidatoDeposito extends MovimientoVinculado {
  clasificacion: string;
  /** El monto es igual a lo que falta por conciliar: la señal más fuerte. */
  calza_monto: boolean;
  /** La descripción menciona el nombre de la asociación. */
  nombra_la_asociacion: boolean;
  diferencia: string;
}

/** La planilla de una asociación en un período, con los TRES montos. */
export interface PlanillaDetalle {
  id: string;
  asociacion_id: string;
  asociacion: string;
  periodo: string;
  referencia: string;
  nota: string;
  esperado: string;
  registrado: string;
  depositado: string;
  estado: string;
  movimientos: MovimientoVinculado[];
  creado_en: string;
}

export const planillasApi = {
  ficha(asociacionId: string, periodo: string): Promise<PlanillaDetalle> {
    return apiFetch<PlanillaDetalle>(`/cxc/asociaciones/${asociacionId}/planilla`, {
      method: "GET",
      query: { periodo },
    });
  },
  /** Registra que llegó la planilla, con la referencia del comprobante del correo. */
  abrir(asociacionId: string, periodo: string, referencia: string, nota = ""): Promise<PlanillaDetalle> {
    return apiFetch<PlanillaDetalle>(`/cxc/asociaciones/${asociacionId}/planilla`, {
      method: "POST",
      json: { periodo, referencia, nota },
    });
  },
  candidatos(planillaId: string): Promise<CandidatoDeposito[]> {
    return apiFetch<CandidatoDeposito[]>(`/cxc/planillas/${planillaId}/candidatos`, { method: "GET" });
  },
  vincular(planillaId: string, movimientoId: string): Promise<PlanillaDetalle> {
    return apiFetch<PlanillaDetalle>(`/cxc/planillas/${planillaId}/depositos`, {
      method: "POST",
      json: { movimiento_id: movimientoId },
    });
  },
  desvincular(planillaId: string, movimientoId: string): Promise<PlanillaDetalle> {
    return apiFetch<PlanillaDetalle>(`/cxc/planillas/${planillaId}/depositos/${movimientoId}`, {
      method: "DELETE",
    });
  },
};

// ─────────── Notas de crédito: bajar deuda sin que entre plata ───────────

/**
 * Una nota de crédito. Las autoriza el supervisor de piso y NO tienen tope de monto, así que
 * el control es otro: motivo obligatorio con contenido, consecutivo propio, y anulación que
 * devuelve los cargos a su saldo en vez de borrar el documento.
 */
export interface NotaCredito {
  id: string;
  /** NC-000001, por empresa y sin huecos. */
  consecutivo: string;
  contrato: string;
  cliente: string;
  fecha: string;
  monto: string;
  motivo: string;
  estado: "APLICADA" | "ANULADA";
  aplicaciones: AplicacionCobro[];
  /** La parte que no encontró cargos abiertos: se informa, no se rechaza. */
  sin_aplicar: string;
  creada_por: string;
  creada_en: string;
  anulada_por: string;
  anulada_en: string;
  anulacion_motivo: string;
}

/** Con autorización sin tope, este resumen ES el control. */
export interface ResumenNotas {
  notas: number;
  monto: string;
  anuladas: number;
  /** Quién condonó cuánto: la pregunta de una auditoría. */
  por_usuario: { usuario: string; notas: number; monto: string }[];
}

export interface ListaNotas {
  resumen: ResumenNotas;
  items: NotaCredito[];
  total: number;
}

export interface NuevaNota {
  contrato: string;
  /** Si viene, la nota va a ESE cargo; si no, al más viejo (FIFO). */
  cargo_id?: string;
  fecha?: string;
  monto: string;
  motivo: string;
}

export const notasApi = {
  listar(filtros: { contrato?: string; desde?: string; hasta?: string; incluir_anuladas?: boolean }): Promise<ListaNotas> {
    return apiFetch<ListaNotas>("/cxc/notas-credito", { method: "GET", query: { ...filtros } });
  },
  emitir(nota: NuevaNota): Promise<NotaCredito> {
    return apiFetch<NotaCredito>("/cxc/notas-credito", { method: "POST", json: nota });
  },
  /** No borra: marca la nota y devuelve los cargos a su saldo con su antigüedad original. */
  anular(id: string, motivo: string): Promise<{ ok: boolean }> {
    return apiFetch<{ ok: boolean }>(`/cxc/notas-credito/${id}/anular`, { method: "POST", json: { motivo } });
  },
};

// ─── Suspensión por mora: 18 MESES, o su equivalencia según la modalidad ───

/**
 * Estado de la mora de un contrato. Las dos medidas van juntas a propósito: los MESES son la
 * regla («18 meses o su equivalencia») y las CUOTAS son el hecho concreto que el operador le
 * dice al cliente. Con un quincenal los dos números son distintos —18 cuotas son 9 meses— y
 * mostrar solo uno haría imposible explicar la decisión.
 */
export interface EstadoSuspension {
  contrato: string;
  estado: string;
  cuotas_vencidas: number;
  meses_mora: string;
  modalidad: string;
  /** Cuántos meses cubre una cuota de este contrato: 1 mensual, 0,5 quincenal, 12 anual. */
  meses_por_cuota: string;
  tope_meses: number;
  /** A cuántas cuotas de ESTE contrato equivale el tope. */
  cuotas_equivalentes: string;
  /** Llegó al tope. El sistema NO suspende solo: lo decide una persona con su motivo. */
  para_suspender: boolean;
  saldo: string;
  suspendido_en: string;
  suspendido_por: string;
  motivo_suspension: string;
  cuotas_al_suspender: number;
  meses_al_suspender: string;
}

export const suspensionApi = {
  estado(numero: string): Promise<EstadoSuspension> {
    return apiFetch<EstadoSuspension>(`/cxc/contratos/${encodeURIComponent(numero)}/suspension`, {
      method: "GET",
    });
  },
  suspender(numero: string, motivo: string): Promise<EstadoSuspension> {
    return apiFetch<EstadoSuspension>(`/cxc/contratos/${encodeURIComponent(numero)}/suspender`, {
      method: "POST",
      json: { motivo },
    });
  },
  reactivar(numero: string, motivo: string): Promise<EstadoSuspension> {
    return apiFetch<EstadoSuspension>(`/cxc/contratos/${encodeURIComponent(numero)}/reactivar`, {
      method: "POST",
      json: { motivo },
    });
  },
};

// ─────────────── Arreglos de pago: 1-3-6-9, y las excepciones ───────────────

/** Una cuota del plan. La 0 es la prima, si hubo. */
export interface CuotaArreglo {
  numero: number;
  vence_en: string;
  monto: string;
  vencida: boolean;
  /** El acumulado pagado alcanzó a cubrirla. Se deriva, no se marca. */
  cubierta: boolean;
}

/**
 * Un arreglo de pago. NO reescribe los cargos: es un plan ENCIMA de la deuda, así que la mora,
 * el tramo y la regla de los 18 meses no se borran por firmarlo. El cumplimiento se deriva de
 * los cobros y se mide acumulado.
 */
export interface Arreglo {
  id: string;
  consecutivo: string;
  contrato: string;
  cliente: string;
  sede: string;
  estado: "AL_DIA" | "EN_MORA" | "CUMPLIDO" | "QUEBRADO" | "ANULADO";
  monto_arreglo: string;
  prima: string;
  plazo_cuotas: number;
  /** El plazo no estaba entre los estándar: lo autorizó el supervisor de piso. */
  es_excepcion: boolean;
  pagado: string;
  /** Lo que a hoy debía haber pagado según el plan. */
  esperado_a_hoy: string;
  atraso: string;
  falta: string;
  cuotas_cubiertas: number;
  proxima_cuota: string;
  proximo_monto: string;
  saldo_al_pactar: string;
  vencido_al_pactar: string;
  meses_mora_al_pactar: string;
  cuotas: CuotaArreglo[];
  observaciones: string;
  autorizado_por: string;
  autorizacion_motivo: string;
  creado_por: string;
  creado_en: string;
  quebrado_por: string;
  quebrado_en: string;
  quebranto_motivo: string;
  anulado_por: string;
  anulado_en: string;
  anulacion_motivo: string;
}

export interface ResumenArreglos {
  arreglos: number;
  pactado: string;
  pagado: string;
  al_dia: number;
  en_mora: number;
  cumplidos: number;
  quebrados: number;
  anulados: number;
  /** Con autorización sin tope, el acumulado de excepciones ES el control. */
  excepciones: number;
  atraso_total: string;
}

/** Lo que la pantalla necesita para no ofrecer un plazo que el servidor va a rechazar. */
export interface PlazosDeArreglo {
  estandar: number[];
  maximo: number;
  puede_excepcion: boolean;
}

export interface ListaArreglos {
  resumen: ResumenArreglos;
  items: Arreglo[];
  total: number;
  plazos: PlazosDeArreglo;
}

export interface NuevoArreglo {
  contrato: string;
  /** Vacío = todo lo vencido, que es el caso normal. */
  monto?: string;
  prima?: string;
  prima_fecha?: string;
  plazo_cuotas: number;
  primera_cuota?: string;
  observaciones?: string;
  /** Obligatorio si el plazo no es estándar. */
  motivo_autorizacion?: string;
}

export const arreglosApi = {
  listar(filtros: {
    contrato?: string;
    estado?: string;
    excepciones?: boolean;
    page?: number;
    page_size?: number;
  }): Promise<ListaArreglos> {
    return apiFetch<ListaArreglos>("/cxc/arreglos", { method: "GET", query: { ...filtros } });
  },
  detalle(id: string): Promise<Arreglo> {
    return apiFetch<Arreglo>(`/cxc/arreglos/${id}`, { method: "GET" });
  },
  pactar(a: NuevoArreglo): Promise<Arreglo> {
    return apiFetch<Arreglo>("/cxc/arreglos", { method: "POST", json: a });
  },
  /** Declara el incumplimiento: el contrato pasa a cartera morosa. */
  quebrar(id: string, motivo: string): Promise<Arreglo> {
    return apiFetch<Arreglo>(`/cxc/arreglos/${id}/quebrar`, { method: "POST", json: { motivo } });
  },
  /** El arreglo que no debió existir. NO marca incumplimiento ni manda a cartera morosa. */
  anular(id: string, motivo: string): Promise<Arreglo> {
    return apiFetch<Arreglo>(`/cxc/arreglos/${id}/anular`, { method: "POST", json: { motivo } });
  },
};

// ───────── Contacto preventivo: avisar ANTES de que la cuota se venza ─────────

/**
 * Un contrato al que hay que avisarle antes del vencimiento. Es el universo que la cola de
 * cobro excluye a propósito, y son conjuntos disjuntos: ningún contrato sale en las dos, así
 * que nadie recibe dos llamadas contradictorias el mismo día.
 */
export interface FilaPreventiva {
  contrato_id: string;
  numero: string;
  cliente: string;
  documento: string;
  telefonos: string;
  correos: string;
  sede: string;
  forma_pago: string;
  asociacion: string;
  modalidad: string;
  saldo: string;
  proxima_cuota: string;
  proximo_monto: string;
  dias_para_vencer: number;
  /** POR_VENCER | TARJETA: dice qué hay que decirle al cliente. */
  motivo: string;
  tarjeta_vence: string;
  ultima_gestion: string;
  gestiones: number;
  /** A un domiciliado el aviso es distinto: que tenga fondos, no que pague. */
  domiciliado: boolean;
}

export interface ResumenPreventivo {
  contratos: number;
  monto: string;
  por_vencer: number;
  tarjetas: number;
  /** Sin teléfono ni correo: un aviso preventivo sin canal no se puede dar. */
  sin_contactar: number;
  /** La ventana en días que se está usando, para que la pantalla no la adivine. */
  dias: number;
}

export interface ListaPreventiva {
  resumen: ResumenPreventivo;
  items: FilaPreventiva[];
  total: number;
}

export const preventivoApi = {
  listar(filtros: {
    sede_id?: string;
    motivo?: string;
    q?: string;
    page?: number;
    page_size?: number;
  }): Promise<ListaPreventiva> {
    return apiFetch<ListaPreventiva>("/cxc/preventivo", { method: "GET", query: { ...filtros } });
  },
};
