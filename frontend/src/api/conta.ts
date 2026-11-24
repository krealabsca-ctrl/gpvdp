/**
 * Cliente del plan de cuentas (mig 0109, Fase 1 de la contabilidad).
 *
 * ESTA FASE NO CONTABILIZA NADA. Es el catálogo contable dentro del sistema: se carga, se navega
 * y se mantiene. El mapeo contra la preclasificación es la Fase 2 y los asientos la Fase 3.
 */

import { apiFetch } from "@/api/client";

/** Clases contables. Salen del primer segmento del código. */
export type ClaseContable =
  | "ACTIVO" | "PASIVO" | "PATRIMONIO" | "INGRESO" | "COSTO" | "GASTO" | "ORDEN";

export interface CuentaContable {
  id: string;
  /** Nulo = cuenta del GRUPO: la comparten las tres empresas. */
  empresa_id: string | null;
  codigo: string;
  nombre: string;
  padre_id: string | null;
  clase: ClaseContable;
  naturaleza: "DEUDORA" | "ACREEDORA";
  /** Solo estas admiten asientos. Una cuenta con hijas no puede. */
  acepta_movimiento: boolean;
  nivel: number;
  /** El código de esta misma cuenta en Softland. Puente a los saldos iniciales. */
  codigo_softland: string;
  activo: boolean;
  /** Lo llena el servidor al armar el árbol; no es una columna. */
  hijas?: CuentaContable[];
}

/** Una fila del Excel, con su problema si no va a entrar. */
export interface FilaPlan {
  fila: number;
  codigo: string;
  nombre: string;
  codigo_softland: string;
  problema: string;
}

/** Lo que pasaría al importar, sin escribir nada. */
export interface PrevisualizacionPlan {
  filas: FilaPlan[];
  a_crear: number;
  a_actualizar: number;
  de_movimiento: number;
  con_problema: number;
  por_clase: Record<string, number>;
}

export interface ResultadoImportacionPlan {
  creadas: number;
  actualizadas: number;
  de_movimiento: number;
  descartadas: FilaPlan[];
}

export const contaApi = {
  /** El plan como árbol, que es como se mira un plan de cuentas. */
  arbol(soloActivas = false): Promise<{ arbol: CuentaContable[] }> {
    return apiFetch<{ arbol: CuentaContable[] }>("/conta/cuentas", {
      method: "GET",
      query: { activas: soloActivas ? "1" : undefined },
    });
  },
  /** La lista plana, para selectores. */
  planas(soloActivas = false): Promise<{ items: CuentaContable[] }> {
    return apiFetch<{ items: CuentaContable[] }>("/conta/cuentas", {
      method: "GET",
      query: { formato: "plano", activas: soloActivas ? "1" : undefined },
    });
  },
  crear(c: Partial<CuentaContable>): Promise<CuentaContable> {
    return apiFetch<CuentaContable>("/conta/cuentas", { method: "POST", json: c });
  },
  actualizar(id: string, c: Partial<CuentaContable>): Promise<CuentaContable> {
    return apiFetch<CuentaContable>(`/conta/cuentas/${id}`, { method: "PATCH", json: c });
  },
  borrar(id: string): Promise<void> {
    return apiFetch<void>(`/conta/cuentas/${id}`, { method: "DELETE" });
  },
  /** NO escribe: dice qué pasaría con ese archivo. */
  previsualizar(archivo: File): Promise<PrevisualizacionPlan> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    return apiFetch<PrevisualizacionPlan>("/conta/cuentas/previsualizar", { method: "POST", raw: fd });
  },
  /**
   * Carga el plan. `destino` decide de quién son las cuentas: del grupo o de la empresa activa.
   *
   * Es la decisión que el mandato llama D1 («¿plan único o uno por empresa?»), tomada al cargar
   * y no al diseñar: cambiar de opinión es volver a importar.
   */
  importar(archivo: File, destino: "grupo" | "empresa"): Promise<ResultadoImportacionPlan> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    fd.append("destino", destino);
    return apiFetch<ResultadoImportacionPlan>("/conta/cuentas/importar", { method: "POST", raw: fd });
  },

  // ── Fase 2 · el mapeo partida → cuenta (mig 0110) ──────────────────────────────────────────

  /** La lista con su cobertura. Vienen juntas: separarlas dejaría el contador y la lista desfasados. */
  mapeo(opts: { pendientes?: boolean; conMovimientos?: boolean } = {}): Promise<RespuestaMapeo> {
    return apiFetch<RespuestaMapeo>("/conta/mapeo", {
      method: "GET",
      query: {
        pendientes: opts.pendientes ? "1" : undefined,
        con_movimientos: opts.conMovimientos ? "1" : undefined,
      },
    });
  },
  /** Una decisión o varias. El lote es lo que hace que un mapeo de 332 partidas se termine. */
  decidir(decisiones: DecisionDeMapeo[]): Promise<ResultadoDecisiones> {
    return apiFetch<ResultadoDecisiones>("/conta/mapeo", { method: "POST", json: { decisiones } });
  },
  /** Devuelve la partida a «falta decidir». */
  deshacer(clasificacionId: string): Promise<void> {
    return apiFetch<void>(`/conta/mapeo/${clasificacionId}`, { method: "DELETE" });
  },

  // ── Fase 3A · la contrapartida bancaria y los libros (mig 0111) ────────────────────────────

  /** Las cuentas bancarias del ERP con su cuenta contable y la sugerencia por IBAN. */
  bancos(): Promise<RespuestaBancos> {
    return apiFetch<RespuestaBancos>("/conta/bancos", { method: "GET" });
  },
  enlazarBanco(cuentaBancariaId: string, cuentaId: string): Promise<void> {
    return apiFetch<void>(`/conta/bancos/${cuentaBancariaId}`, {
      method: "PUT",
      json: { cuenta_id: cuentaId },
    });
  },
  desenlazarBanco(cuentaBancariaId: string): Promise<void> {
    return apiFetch<void>(`/conta/bancos/${cuentaBancariaId}`, { method: "DELETE" });
  },
  /** Acepta de una vez todo lo que el sistema dedujo por IBAN. */
  aceptarSugerencias(): Promise<{ enlazadas: number }> {
    return apiFetch<{ enlazadas: number }>("/conta/bancos/aceptar-sugerencias", { method: "POST" });
  },

  diario(f: FiltroDiario): Promise<{ items: Asiento[] }> {
    return apiFetch<{ items: Asiento[] }>("/conta/diario", {
      method: "GET",
      query: {
        desde: f.desde || undefined,
        hasta: f.hasta || undefined,
        estado: f.estado || undefined,
        origen: f.origen || undefined,
        limite: f.limite ? String(f.limite) : undefined,
      },
    });
  },
  asiento(id: string): Promise<Asiento> {
    return apiFetch<Asiento>(`/conta/asientos/${id}`, { method: "GET" });
  },
  mayor(cuentaId: string, desde: string, hasta: string): Promise<MayorDeCuenta> {
    return apiFetch<MayorDeCuenta>("/conta/mayor", {
      method: "GET",
      query: { cuenta: cuentaId, desde: desde || undefined, hasta: hasta || undefined },
    });
  },
  crearAsiento(a: NuevoAsiento): Promise<Asiento> {
    return apiFetch<Asiento>("/conta/asientos", { method: "POST", json: a });
  },
  /** Anular y no borrar: borrar deja un hueco en la numeración del libro. */
  anularAsiento(id: string, motivo: string): Promise<void> {
    return apiFetch<void>(`/conta/asientos/${id}/anular`, { method: "POST", json: { motivo } });
  },

  // ── Fase 3B · generar asientos desde el banco (mig 0112) ───────────────────────────────────

  /** NO escribe: dice qué pasaría con ese período. */
  previsualizarGeneracion(periodo: string): Promise<ResumenGeneracion> {
    return apiFetch<ResumenGeneracion>("/conta/generar", { method: "GET", query: { periodo } });
  },
  generar(periodo: string): Promise<ResultadoGeneracion> {
    return apiFetch<ResultadoGeneracion>("/conta/generar", { method: "POST", json: { periodo } });
  },
  /** Anula los asientos del período generados desde el banco; después se puede regenerar. */
  revertirGeneracion(periodo: string, motivo: string): Promise<{ periodo: string; anulados: number }> {
    return apiFetch<{ periodo: string; anulados: number }>("/conta/generar/revertir", {
      method: "POST",
      json: { periodo, motivo },
    });
  },

  // ── Fase 4 · balance, apertura y cierre de período (mig 0113) ─────────────────────────────

  balance(desde: string, hasta: string): Promise<BalanceComprobacion> {
    return apiFetch<BalanceComprobacion>("/conta/balance", {
      method: "GET",
      query: { desde, hasta },
    });
  },
  periodos(): Promise<{ items: PeriodoContable[] }> {
    return apiFetch<{ items: PeriodoContable[] }>("/conta/periodos", { method: "GET" });
  },
  cerrarPeriodo(periodo: string): Promise<void> {
    return apiFetch<void>("/conta/periodos/cerrar", { method: "POST", json: { periodo } });
  },
  reabrirPeriodo(periodo: string, motivo: string): Promise<void> {
    return apiFetch<void>("/conta/periodos/reabrir", { method: "POST", json: { periodo, motivo } });
  },
  /** NO escribe: dice qué saldos traería el archivo y si cuadran. */
  previsualizarApertura(periodo: string, archivo: File): Promise<PrevisualizacionApertura> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    fd.append("periodo", periodo);
    return apiFetch<PrevisualizacionApertura>("/conta/apertura/previsualizar", {
      method: "POST",
      raw: fd,
    });
  },
  cargarApertura(periodo: string, archivo: File): Promise<Asiento> {
    const fd = new FormData();
    fd.append("archivo", archivo);
    fd.append("periodo", periodo);
    return apiFetch<Asiento>("/conta/apertura", { method: "POST", raw: fd });
  },
};

/** Una cuenta en el balance de comprobación. */
export interface RenglonBalance {
  cuenta_id: string;
  codigo: string;
  nombre: string;
  clase: string;
  naturaleza: string;
  /** Lo acumulado ANTES del rango: sin esto el balance de un mes arranca en cero. */
  saldo_anterior_debe: string;
  saldo_anterior_haber: string;
  debe: string;
  haber: string;
  /** El saldo final va en la columna de su signo, no como un negativo en la otra. */
  saldo_final_debe: string;
  saldo_final_haber: string;
}

export interface BalanceComprobacion {
  desde: string;
  hasta: string;
  renglones: RenglonBalance[];
  total_anterior_debe: string;
  total_anterior_haber: string;
  total_debe: string;
  total_haber: string;
  total_final_debe: string;
  total_final_haber: string;
  /** Los tres pares de totales son iguales. Si no, ningún estado financiero sirve. */
  cuadra: boolean;
  diferencia?: string;
  /** No hay saldos iniciales: el balance es correcto pero incompleto. */
  sin_apertura: boolean;
  aviso?: string;
}

export interface PeriodoContable {
  anio: number;
  mes: number;
  estado: "ABIERTO" | "CERRADO";
  cerrado_en: string | null;
  cerrado_por_nombre: string;
  debe_al_cierre: string;
  haber_al_cierre: string;
  asientos_al_cierre: number;
  reabierto_en: string | null;
  reabierto_motivo: string;
  /** Lo que el período tiene HOY, esté cerrado o no. */
  asientos: number;
  debe: string;
  haber: string;
  cuadra: boolean;
}

export interface FilaApertura {
  fila: number;
  codigo: string;
  cuenta: string;
  debe: string;
  haber: string;
  problema: string;
}

export interface PrevisualizacionApertura {
  fecha: string;
  filas: FilaApertura[];
  cuentas: number;
  total_debe: string;
  total_haber: string;
  cuadra: boolean;
  diferencia?: string;
  con_problema: number;
  /** Ya hay una apertura: cargar otra exige anular la anterior. */
  ya_existe: boolean;
  aviso?: string;
}

/** Cuántos movimientos bloquea una causa, y por cuánta plata. */
export interface MotivoExcluido {
  motivo: string;
  etiqueta: string;
  /** Dónde se arregla. Sin esto, «1.843 sin mapeo» informa pero no se puede accionar. */
  donde_se_arregla: string;
  movimientos: number;
  monto_crc: string;
}

/** Lo que pasaría al generar, antes de generar. */
export interface ResumenGeneracion {
  periodo: string;
  total: number;
  /** Los que tienen todo lo que hace falta, ya tengan asiento o no. */
  contabilizables: number;
  /**
   * Los contabilizables que TODAVÍA no tienen asiento: lo que de verdad se va a escribir.
   * En un período ya generado, `contabilizables` y `nuevos` se separan, y el botón tiene que
   * usar este o promete algo que no va a pasar.
   */
  nuevos: number;
  monto_debitos: string;
  monto_creditos: string;
  por_motivo: MotivoExcluido[];
  /** Movimientos que ya tienen asiento vivo. Regenerar no los duplica. */
  ya_generados: number;
  aviso: string;
}

export interface ResultadoGeneracion {
  periodo: string;
  creados: number;
  omitidos: number;
  ya_existian: number;
  desde_numero: number;
  hasta_numero: number;
}

/** Una cuenta bancaria del ERP con su cuenta contable, si ya se decidió. */
export interface FilaBancoContable {
  cuenta_bancaria_id: string;
  alias: string;
  banco: string;
  moneda: string;
  iban: string;
  /** Movimientos vivos. Ordena el trabajo: enlazar primero la cuenta que más pesa. */
  movimientos: number;

  cuenta_id: string | null;
  cuenta_codigo: string;
  cuenta_nombre: string;

  /** Lo que el sistema propone comparando IBAN. No es una decisión hasta que alguien la acepta. */
  sugerida_id: string;
  sugerida_codigo: string;
  sugerida_nombre: string;

  /** Otra cuenta bancaria apunta a la misma cuenta contable. Avisa; no lo impide. */
  compartida: boolean;
}

export interface CoberturaBancos {
  cuentas: number;
  enlazadas: number;
  faltan: number;
  con_sugerencia: number;
  movimientos_cubiertos: number;
  movimientos_totales: number;
}

export interface RespuestaBancos {
  items: FilaBancoContable[];
  cobertura: CoberturaBancos;
}

export type EstadoAsiento = "BORRADOR" | "ASENTADO" | "ANULADO";

export interface LineaAsiento {
  id: string;
  cuenta_id: string;
  cuenta_codigo: string;
  cuenta_nombre: string;
  /** En colones, siempre: una columna que mezcla monedas no suma. */
  debe: string;
  haber: string;
  moneda_origen: string;
  monto_origen: string;
  tc_aplicado: string;
  detalle: string;
  orden: number;
}

export interface Asiento {
  id: string;
  numero: number;
  fecha: string;
  glosa: string;
  /** De qué documento salió. MANUAL es el que escribió una persona. */
  origen_tipo: string;
  origen_id: string | null;
  estado: EstadoAsiento;
  anulado_motivo: string;
  creado_por_nombre: string;
  creado_en: string;
  lineas?: LineaAsiento[];
  debe: string;
  haber: string;
}

export interface FiltroDiario {
  desde?: string;
  hasta?: string;
  estado?: string;
  origen?: string;
  limite?: number;
}

export interface RenglonMayor {
  asiento_id: string;
  numero: number;
  fecha: string;
  glosa: string;
  detalle: string;
  debe: string;
  haber: string;
  /** Acumulado con el signo de la NATURALEZA de la cuenta, no debe-menos-haber a secas. */
  saldo: string;
}

export interface MayorDeCuenta {
  cuenta: CuentaContable;
  saldo_inicial: string;
  renglones: RenglonMayor[];
  total_debe: string;
  total_haber: string;
  saldo_final: string;
}

export interface NuevaLineaAsiento {
  cuenta_id: string;
  debe: string;
  haber: string;
  detalle: string;
}

export interface NuevoAsiento {
  fecha: string;
  glosa: string;
  lineas: NuevaLineaAsiento[];
}

/** Las dos decisiones posibles. No hay una tercera: «falta decidir» es la ausencia de fila. */
export type EstadoMapeo = "MAPEADA" | "NO_CONTABILIZABLE";

/** Una clasificación del ERP con su decisión contable, si ya se tomó. */
export interface FilaDelMapeo {
  clasificacion_id: string;
  concepto: string;
  clasificacion: string;
  /**
   * Lo que el ERP declara. OJO: nunca viene vacía —su default es NEUTRO—, así que no alcanza con
   * leerla sola; lo que distingue «es neutro» de «nadie lo decidió» es `naturaleza_declarada`.
   */
  naturaleza_concepto: "INGRESO" | "GASTO" | "NEUTRO";
  naturaleza_declarada: boolean;
  /** Movimientos vivos que la usan. Es lo que ordena el trabajo: mapear primero lo que más pesa. */
  movimientos: number;

  /** "" = falta decidir. */
  estado: EstadoMapeo | "";
  motivo: string;
  cuenta_id: string | null;
  cuenta_codigo: string;
  cuenta_nombre: string;
  cuenta_clase: ClaseContable | "";

  /** La naturaleza del ERP no concuerda con la clase de la cuenta. Avisa; no bloquea. */
  cruzada: boolean;
  aviso_cruce: string;
}

/** El reporte que dice si se puede pasar a la Fase 3. */
export interface CoberturaMapeo {
  /** El denominador que importa: una clasificación que nadie usó no bloquea nada. */
  con_movimientos: number;
  mapeadas: number;
  no_contabilizables: number;
  sin_decidir: number;
  cruzadas: number;
  /** Conceptos que no declararon si son ingreso o gasto: ahí el detector de cruce no puede opinar. */
  sin_naturaleza: number;
  /** Incluye las que nadie usó, para no esconder que existen. */
  total: number;
}

export interface RespuestaMapeo {
  items: FilaDelMapeo[];
  cobertura: CoberturaMapeo;
}

export interface DecisionDeMapeo {
  clasificacion_id: string;
  cuenta_id?: string;
  estado: EstadoMapeo;
  motivo?: string;
}

export interface ResultadoDecisiones {
  guardadas: number;
  /** El motivo POR CLASIFICACIÓN: un conteo de fallos sin el porqué deja probando a ciegas. */
  no_se_pudo: Record<string, string>;
}

/** Lo resuelto sobre lo que de verdad bloquea. Sin nada que mapear, está todo resuelto. */
export function porcentajeResuelto(c: CoberturaMapeo): number {
  if (c.con_movimientos === 0) return 100;
  return Math.floor(((c.mapeadas + c.no_contabilizables) * 100) / c.con_movimientos);
}
