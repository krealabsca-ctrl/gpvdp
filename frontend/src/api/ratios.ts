/**
 * Cliente de los ratios del ciclo de efectivo.
 *
 * Cruzan CxC y CxP, así que no viven bajo ningún módulo: tienen su propia superficie, igual que
 * el consolidado del grupo y el flujo de caja.
 *
 * Los días son strings decimales por la misma razón que los montos: redondear en el cliente haría
 * que la pantalla y el reporte digan cifras distintas.
 */

import { apiFetch } from "@/api/client";

/** Un indicador con su cálculo a la vista. */
export interface Ratio {
  /** El valor principal: el calculado por agotamiento (countback). */
  dias: string;
  /** La fórmula de libro, para contrastar. Cuando se separa del principal, los últimos meses no se parecen al promedio. */
  dias_simple: string;
  /** El numerador: lo que está pendiente hoy. */
  cartera: string;
  /** El promedio mensual de la ventana. */
  base_mensual: string;
  /** Cuántos meses tuvo que agotar. Si se comió la ventana entera, el valor es un piso. */
  meses_usados: number;
  /** Sin base suficiente no se muestra el ratio, en vez de mostrar cero. */
  calculable: boolean;
  aviso?: string;
}

/** Los tres ratios y el ciclo. */
export interface CicloDeEfectivo {
  desde: string;
  hasta: string;
  meses: number;
  dso: Ratio;
  dpo: Ratio;
  dio: Ratio;
  /** DSO + DIO − DPO. Positivo: financiás vos. Negativo: te financian los proveedores. */
  ciclo: string;
  ciclo_calculable: boolean;
  avisos: string[];
}

/** Trae el ciclo de la ventana pedida (3, 6, 12 o 24 meses). */
export function getCicloDeEfectivo(meses: number): Promise<CicloDeEfectivo> {
  return apiFetch<CicloDeEfectivo>("/ratios/ciclo", { query: { meses } });
}

// ── Drivers de la cartera ───────────────────────────────────────────────────────────────────
//
// El negocio por lo que lo mueve —contratos, tipo de servicio, modalidad, sede— en vez de por
// partida contable. Sin drivers, presupuestar es «el año pasado más un diez por ciento».

export type DimensionDriver = "tipo_servicio" | "modalidad" | "sede";

/** Un corte de la cartera. */
export interface FilaDriver {
  clave: string;
  nombre: string;
  contratos: number;
  /** Lo que DEBERÍA entrar al mes por este corte. */
  cuota_contratada: string;
  /** El ticket recurrente: lo que distingue un plan caro de uno barato. */
  cuota_promedio: string;
  contratos_facturados: number;
  cuota_facturada: string;
  /** Qué proporción de estos contratos generó cargo. Explica la brecha contratado/facturado. */
  cobertura_pct: string;
  /** Cuánto pesa este corte en el recurrente total. */
  aporte_pct: string;
}

/** Cuántos contratos entraron en un mes. */
export interface AltaMensual {
  periodo: string;
  altas: number;
  cuota_promedio: string;
  cuota_incorporada: string;
}

/** El panel completo. */
export interface DriversDeCartera {
  dimension: DimensionDriver;
  periodo_facturado: string;
  contratos_activos: number;
  cuota_contratada: string;
  cuota_promedio: string;
  contratos_facturados: number;
  cuota_facturada: string;
  cobertura_pct: string;
  filas: FilaDriver[];
  altas: AltaMensual[];
  contratos_sin_fecha_alta: number;
  avisos: string[];
}

/** Trae los drivers por la dimensión pedida. */
export function getDrivers(dimension: DimensionDriver, meses: number): Promise<DriversDeCartera> {
  return apiFetch<DriversDeCartera>("/ratios/drivers", { query: { dimension, meses } });
}
