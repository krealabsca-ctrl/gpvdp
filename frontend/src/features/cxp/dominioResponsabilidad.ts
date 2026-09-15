import type { BadgeTone } from "@/components/ui";
import type { Periodicidad, RespaldoTipo, Semaforo } from "@/api/cxp";

/**
 * Vocabulario de las responsabilidades mensuales, en un solo lugar.
 *
 * Vive aparte de las pantallas porque las mismas etiquetas se usan en «El mes», en «Los acuerdos»
 * y en «Mis responsabilidades»: repetirlas en tres archivos garantiza que un día digan tres cosas
 * distintas para el mismo estado.
 */

/** Qué dice cada semáforo y con qué color. El texto importa tanto como el color: hay gente que no
 *  distingue rojo de verde, y «SIN DATO» pintado de gris sin explicación se lee como un error. */
export const SEMAFORO: Record<Semaforo, { etiqueta: string; tono: BadgeTone; ayuda: string }> = {
  CUMPLIDA: {
    etiqueta: "Cumplida",
    tono: "positivo",
    ayuda: "Resuelta: hay una factura, un débito o un acuse que lo prueba.",
  },
  NO_APLICA: {
    etiqueta: "No aplicaba",
    tono: "neutral",
    ayuda: "Alguien declaró que este mes no correspondía, con su motivo escrito.",
  },
  VENCIDA: {
    etiqueta: "Vencida",
    tono: "negativo",
    ayuda: "Pasó la fecha y sigue sin resolverse. Los datos del banco alcanzan para afirmarlo.",
  },
  POR_VENCER: {
    etiqueta: "Por vencer",
    tono: "pendiente",
    ayuda: "Vence dentro de los próximos 7 días.",
  },
  AL_DIA: {
    etiqueta: "Al día",
    tono: "accent",
    ayuda: "Todavía falta para su fecha.",
  },
  SIN_DATO: {
    etiqueta: "Sin dato",
    tono: "neutral",
    ayuda:
      "Ya pasó la fecha, pero el banco no está importado hasta ese día: pudo haberse pagado por débito y todavía no se puede saber. No es un error del sistema, es una carga pendiente.",
  },
  SIN_ABRIR: {
    etiqueta: "Sin abrir",
    tono: "negativo",
    ayuda:
      "La responsabilidad existe y este mes no tiene fila: nadie abrió el mes. Sin esto, un mes sin abrir se ve igual que un mes sin nada pendiente.",
  },
};

export const ETIQUETA_PERIODICIDAD: Record<Periodicidad, string> = {
  MENSUAL: "Todos los meses",
  BIMENSUAL: "Cada 2 meses",
  TRIMESTRAL: "Cada 3 meses",
  SEMESTRAL: "Cada 6 meses",
  ANUAL: "Una vez al año",
};

export const ETIQUETA_RESPALDO: Record<RespaldoTipo, string> = {
  CONTRATO: "Contrato",
  ACTA: "Acta",
  CORREO: "Correo",
  VERBAL: "De palabra",
  NINGUNO: "Sin respaldo",
};

/**
 * El respaldo se muestra EN LA FILA, no escondido en el detalle: un acuerdo de ₡2,7 millones al
 * mes cuyo único respaldo es de palabra tiene que verse como tal de un vistazo.
 */
export function tonoRespaldo(t: RespaldoTipo | undefined): BadgeTone {
  if (!t || t === "NINGUNO" || t === "VERBAL") return "negativo";
  return "neutral";
}

/** Los meses, para el selector de ancla de las que no son mensuales. */
export const MESES = [
  "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
  "Julio", "Agosto", "Setiembre", "Octubre", "Noviembre", "Diciembre",
];

/** Etiqueta legible de un período AAAA-MM («2026-09» → «Setiembre 2026»). */
export function etiquetaPeriodo(periodo: string): string {
  const [a, m] = periodo.split("-");
  const i = Number(m) - 1;
  if (!a || i < 0 || i > 11) return periodo;
  return `${MESES[i]} ${a}`;
}

/** Corre un período N meses (positivo o negativo), para las flechas de navegación del mes. */
export function correrPeriodo(periodo: string, meses: number): string {
  const [a, m] = periodo.split("-").map(Number);
  if (!a || !m) return periodo;
  // Date con día 1 y aritmética de meses: el día 1 siempre existe, así que no hay sorpresas de
  // desbordamiento como las del 31.
  const d = new Date(Date.UTC(a, m - 1 + meses, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}
