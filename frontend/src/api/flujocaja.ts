/**
 * Cliente del flujo de caja a 13 semanas.
 *
 * El reporte cruza bancos, CxP, CxC y nómina, así que no vive bajo ningún módulo: tiene su propia
 * superficie, igual que la vista del grupo.
 *
 * Los montos son strings decimales (nunca number: un saldo en colones no admite el redondeo del
 * punto flotante).
 */

import { apiFetch } from "@/api/client";

/** Capa de certeza de una línea del flujo. */
export type CapaFlujo = "COMPROMETIDO" | "ESPERADO" | "REPRESA";

/** De dónde sale cada línea. */
export type OrigenFlujo =
  | "PLANILLA"
  | "CXP_PROGRAMADA"
  | "CXP_ATRASO"
  | "CXC_CARGO"
  | "CXC_CUOTA";

/** Cuánto puso cada fuente en una semana. */
export interface AporteOrigen {
  origen: OrigenFlujo;
  capa: CapaFlujo;
  monto: string;
  entrada: boolean;
}

/** Una semana del horizonte con su saldo de cierre. */
export interface SemanaCaja {
  numero: number;
  desde: string;
  hasta: string;
  saldo_inicial: string;
  entradas_comprometidas: string;
  entradas_esperadas: string;
  entradas: string;
  salidas_comprometidas: string;
  salidas_represa: string;
  salidas: string;
  neto: string;
  /** El dato de la fila: todo lo demás existe para explicarlo. */
  saldo_final: string;
  bajo_minimo: boolean;
  en_rojo: boolean;
  por_origen: AporteOrigen[];
}

/** Modo del tope de pago de atrasos. */
export type ModoPolitica = "MONTO_FIJO" | "PCT_INGRESOS";

/** La decisión de tesorería que hace proyectable la cartera atrasada. */
export interface PoliticaCaja {
  modo: ModoPolitica;
  tope_semanal: string;
  tope_pct: string;
  minimo_operativo: string;
  moneda: string;
  nota: string;
  /** Distingue «no hay política» de «la política es cero». */
  definida: boolean;
}

/** Fuente del saldo de partida. SIN_DATOS significa que la curva arranca en cero. */
export type FuenteSaldo = "DECLARADO" | "SIN_DATOS";

/** La proyección completa de una moneda. */
export interface FlujoSemanal {
  moneda: string;
  desde: string;
  hasta: string;
  saldo_inicial: string;
  fuente_saldo: FuenteSaldo;
  fecha_saldo: string;
  semanas: SemanaCaja[];
  /** 0 si nunca pasa. Es lo primero que se mira. */
  primera_semana_bajo_minimo: number;
  primera_semana_en_rojo: number;
  saldo_final_horizonte: string;
  /** El mínimo del horizonte no siempre es el final. */
  punto_mas_bajo: string;
  semana_punto_mas_bajo: number;
  politica: PoliticaCaja;
  represa_total: string;
  represa_desagotada: string;
  represa_remanente: string;
  /** -1 significa NUNCA: con tope cero la cartera atrasada no baja. */
  semanas_para_vaciar: number;
  tasa_cobro: string;
  tasa_cobro_medida: boolean;
  contratos_activos: number;
  cuentas_con_saldo: number;
  cuentas_sin_saldo: number;
  avisos: string[];
}

/** Trae la proyección de una moneda. */
export function getFlujoCaja(moneda: string): Promise<FlujoSemanal> {
  return apiFetch<FlujoSemanal>("/flujo-caja", { query: { moneda } });
}

/** Lee la política vigente de esa moneda. */
export function getPoliticaCaja(moneda: string): Promise<PoliticaCaja> {
  return apiFetch<PoliticaCaja>("/flujo-caja/politica", { query: { moneda } });
}

/** Guarda la política. Devuelve 204 sin cuerpo. */
export function putPoliticaCaja(p: PoliticaCaja): Promise<void> {
  return apiFetch<void>("/flujo-caja/politica", { method: "PUT", json: p });
}
