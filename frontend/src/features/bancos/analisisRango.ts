/**
 * El rango del análisis y la concentración por partida.
 *
 * Viven acá, fuera de la pantalla, porque son las dos piezas donde un error no se ve: un rango
 * corrido un mes y una concentración mal acumulada producen tablas que parecen correctas.
 */

import { componerPeriodo, partesPeriodo, toNumber } from "@/lib/format";
import type { TendenciaPartida } from "@/api/bancos";

/** Un tramo es una cantidad de meses, el año en curso, o un rango que elige la persona. */
export type Tramo = number | "YTD" | "LIBRE";

/** El tope del endpoint. Se repite acá para avisar ANTES de pedir un rango que va a fallar. */
export const MAX_MESES = 24;

/** Resta meses a un período YYYY-MM sin pasar por Date (que arrastra la zona horaria). */
export function restarMeses(periodo: string, n: number): string {
  const { anio, mes } = partesPeriodo(periodo);
  const total = anio * 12 + (mes - 1) - n;
  return componerPeriodo(Math.floor(total / 12), (total % 12) + 1);
}

/**
 * Desde qué mes arranca el rango.
 *
 * El `desde` se DERIVA del tramo en vez de guardarse suelto: así cambiar el mes final conserva el
 * corte elegido. Con `desde` como estado propio, mover «hasta» de marzo a septiembre dejaría un
 * rango de nueve meses mientras el botón sigue diciendo «Trimestre».
 */
export function desdeDelTramo(tramo: Tramo, hasta: string, desdeLibre: string): string {
  if (tramo === "LIBRE") return desdeLibre;
  // El año en curso arranca SIEMPRE en enero del año de `hasta`, no doce meses atrás: son dos
  // cosas distintas y la segunda no es el acumulado del año.
  if (tramo === "YTD") return componerPeriodo(partesPeriodo(hasta).anio, 1);
  return restarMeses(hasta, tramo - 1);
}

/** Meses de CALENDARIO del rango, con las dos puntas incluidas. */
export function mesesEntre(desde: string, hasta: string): number {
  const d = partesPeriodo(desde);
  const h = partesPeriodo(hasta);
  return (h.anio - d.anio) * 12 + (h.mes - d.mes) + 1;
}

/**
 * Por qué el rango no se puede pedir, o "" si está bien.
 *
 * Se comprueba acá y no se consulta: el servidor devolvería el mismo rechazo con peor redacción
 * y después de ir y volver.
 */
export function problemaDelRango(desde: string, hasta: string): string {
  const n = mesesEntre(desde, hasta);
  if (n < 1) return "El mes inicial es posterior al final.";
  if (n > MAX_MESES) return `El rango son ${n} meses y el máximo es ${MAX_MESES}.`;
  return "";
}

/** Una partida con su peso dentro del total. */
export interface FilaConcentracion {
  p: TendenciaPartida;
  monto: number;
  pct: number;
  pctAcum: number;
}

/**
 * La concentración: dónde está el dinero, que no es la misma pregunta que qué se movió.
 *
 * La tabla principal ordena por DESVÍO, así que una partida chica que se duplicó aparece antes
 * que la que se lleva un tercio del gasto. Medido en Memorial Pets el 27-oct-2026: 10 partidas de
 * 116 explican el 73 % del gasto, y Planilla sola el 31,4 %.
 *
 * Se calcula sobre lo FILTRADO para que el porcentaje sea el de lo que se está mirando: contra el
 * total de todas las naturalezas, los gastos sumarían 40 % y el número no diría nada.
 */
export function calcularConcentracion(partidas: TendenciaPartida[]): FilaConcentracion[] {
  const orden = [...partidas].sort((a, b) => toNumber(b.total) - toNumber(a.total));
  const total = orden.reduce((acc, p) => acc + toNumber(p.total), 0);
  let acum = 0;
  return orden.map((p) => {
    const monto = toNumber(p.total);
    acum += monto;
    return {
      p,
      monto,
      pct: total > 0 ? (monto * 100) / total : 0,
      pctAcum: total > 0 ? (acum * 100) / total : 0,
    };
  });
}

/**
 * Cuántas partidas explican el `objetivo` % del total: el titular de la concentración.
 *
 * Devuelve el total cuando ninguna lo alcanza, que pasa si los montos son cero o negativos.
 */
export function partidasQueExplican(filas: FilaConcentracion[], objetivo = 80): number {
  const i = filas.findIndex((c) => c.pctAcum >= objetivo);
  return i < 0 ? filas.length : i + 1;
}

// ── El puente: qué explica la variación ─────────────────────────────────────────────────────

/** Un tramo del puente: una partida y cuánto aportó al cambio. */
export interface TramoPuente {
  clasificacionId: string;
  clasificacion: string;
  concepto: string;
  /** Positivo = subió y empujó el total hacia arriba. */
  aporte: number;
  /** Qué parte del cambio total explica esta partida, en %. */
  pctDelCambio: number;
  /** La partida no existía antes: su aporte entero es «apareció». */
  esNueva: boolean;
}

export interface Puente {
  totalActual: number;
  totalAnterior: number;
  cambio: number;
  tramos: TramoPuente[];
  /** Cuánto del cambio queda fuera de los tramos mostrados. */
  resto: number;
  /** Partidas que no entraron en la lista corta. */
  restoPartidas: number;
}

/**
 * Qué explica la diferencia entre el rango y su comparación.
 *
 * ── POR QUÉ ES OTRA COSA QUE LA TABLA ───────────────────────────────────────────────────────
 *
 * La tabla dice cuánto varió CADA partida en porcentaje; el puente dice quién movió el TOTAL, en
 * plata. Son preguntas distintas y la segunda es la que se lleva a una reunión: «el gasto subió
 * ₡12 M — ₡8 M son planilla y ₡3 M mercadeo» se entiende de una vez, y una lista de 38 variaciones
 * porcentuales no.
 *
 * Se ordena por el VALOR ABSOLUTO del aporte: lo que más bajó importa tanto como lo que más
 * subió, y ordenar por el número con signo dejaría las caídas al final, fuera de la lista corta.
 *
 * NO INCLUYE las partidas que dejaron de usarse —no tienen fila de la que salir—; el aviso del
 * servidor las cuenta aparte, y acá se refleja en `resto`.
 */
export function calcularPuente(partidas: TendenciaPartida[], topeTramos = 8): Puente {
  let totalActual = 0;
  let totalAnterior = 0;
  const todos: TramoPuente[] = [];

  for (const p of partidas) {
    const actual = toNumber(p.total);
    const anterior = toNumber(p.total_anterior ?? "0");
    totalActual += actual;
    totalAnterior += anterior;
    const aporte = actual - anterior;
    if (aporte === 0) continue;
    todos.push({
      clasificacionId: p.clasificacion_id,
      clasificacion: p.clasificacion,
      concepto: p.concepto,
      aporte,
      pctDelCambio: 0,
      esNueva: !!p.sin_base_anterior,
    });
  }

  const cambio = totalActual - totalAnterior;
  todos.sort((a, b) => Math.abs(b.aporte) - Math.abs(a.aporte));
  for (const t of todos) {
    // Contra el valor ABSOLUTO del cambio: si el total casi no se movió porque las subidas y las
    // bajadas se cancelaron, dividir por el neto daría porcentajes de miles por ciento.
    t.pctDelCambio = cambio !== 0 ? (t.aporte * 100) / Math.abs(cambio) : 0;
  }

  const tramos = todos.slice(0, topeTramos);
  const resto = todos.slice(topeTramos).reduce((acc, t) => acc + t.aporte, 0);
  return {
    totalActual,
    totalAnterior,
    cambio,
    tramos,
    resto,
    restoPartidas: Math.max(todos.length - topeTramos, 0),
  };
}
