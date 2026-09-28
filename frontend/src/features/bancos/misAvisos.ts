/**
 * La respuesta a los avisos del equipo, en palabras (22-set-2026).
 *
 * Antes, cuando quien clasifica resolvía un aviso, el botón volvía a salir y la respuesta no se
 * veía en ningún lado, mientras el pie de la pantalla prometía «te queda la respuesta en esta misma
 * pantalla». Ahora la respuesta está en dos lugares: en la fila del movimiento (si sigue en lo que
 * el equipo ve) y en «Mis avisos» (siempre, aunque la corrección lo haya pasado a otra partida).
 *
 * El caso que obliga a cuidar las palabras es el más común: RECLASIFICADO a OTRA partida. El
 * movimiento sale del alcance y deja de aparecer en «Mi partida». Si «Mis avisos» no lo explica,
 * se lee como «se perdió el depósito» —un error— cuando es exactamente lo que se pidió.
 *
 * Funciones puras y aparte de la página para poder probarlas sin navegador.
 */

import type { BadgeTone } from "@/components/ui/Badge";
import type { AvisoResuelto, MiAviso } from "@/api/bancos";

/** Lo corto que va en la fila, al lado de la fecha: «Corregido · 20/09/2026 14:30». */
export function tituloAvisoResuelto(a: Pick<AvisoResuelto, "resolucion">): string {
  switch (a.resolucion) {
    case "RECLASIFICADO":
      return "Corregido";
    case "SIN_CAMBIO":
      return "Revisado, sin cambio";
    default:
      return "Respondido";
  }
}

/** El estado de un aviso en «Mis avisos»: la etiqueta, su tono y qué significa. */
export interface EstadoDeAviso {
  etiqueta: string;
  tono: BadgeTone;
  explicacion: string;
}

export function estadoDeAviso(a: Pick<MiAviso, "estado" | "resolucion" | "es_faltante">): EstadoDeAviso {
  if (a.estado !== "RESUELTO") {
    return {
      etiqueta: "En revisión",
      tono: "pendiente",
      explicacion: "Todavía no lo respondió quien clasifica.",
    };
  }
  if (a.resolucion === "RECLASIFICADO") {
    return {
      etiqueta: "Corregido",
      tono: "positivo",
      explicacion: a.es_faltante
        ? "Quien clasifica encontró el movimiento y le corrigió la partida."
        : // El caso que más confunde: la corrección lo saca de lo que el equipo ve (o, si venía de
          // «Todavía sin partida», lo pasa a «Mi partida»). El servidor no dice adónde fue, a
          // propósito, así que la frase no lo afirma: explica por qué pudo moverse.
          "Le corrigieron la partida, así que puede haber cambiado de lugar en esta pantalla. Si ya no lo ves en «Mi partida», es porque quedó en la partida que corresponde: no se perdió.",
    };
  }
  if (a.resolucion === "SIN_CAMBIO") {
    return {
      etiqueta: "Sin cambio",
      tono: "neutral",
      explicacion: a.es_faltante
        ? "Quien clasifica lo revisó y no cambió nada."
        : "Quien clasifica lo revisó y dejó la partida como estaba.",
    };
  }
  return { etiqueta: "Respondido", tono: "neutral", explicacion: "" };
}

// ── «Respuesta nueva» ────────────────────────────────────────────────────────
//
// El servidor no guarda si el usuario ya leyó una respuesta (no hay columna para eso). Lo que se
// puede decir sin inventar es «esta respuesta no la viste todavía EN ESTE NAVEGADOR»: se recuerdan
// los ids de los avisos resueltos que ya se mostraron en «Mis avisos». Es una comodidad por
// persona y por navegador, no un estado del aviso.
//
// Si el almacenamiento del navegador no está disponible (ventana privada, sitio bloqueado), no se
// marca NADA como nuevo: sin memoria no hay forma de saberlo, y marcar todo sería mentir.

/** Cuántos ids se recuerdan como máximo (los más recientes). */
export const MAX_VISTOS = 1000;

/** La clave es por empresa Y por usuario: en una computadora compartida, cada quien lo suyo. */
export function claveVistos(empresaId: string, usuarioId: string): string {
  return `gpvdp.mis-avisos.vistos.${empresaId}.${usuarioId}`;
}

/** El localStorage si se puede usar; null si el acceso falla (el accessor puede lanzar). */
export function almacenamientoDelNavegador(): Storage | null {
  try {
    return typeof window !== "undefined" ? window.localStorage : null;
  } catch {
    return null;
  }
}

/**
 * Los ids ya vistos. `null` = no se puede saber (sin almacenamiento, sin clave, o el navegador no
 * deja leer); un conjunto vacío = se puede saber y todavía no vio ninguna. Un valor guardado que no
 * se entiende cuenta como vacío (y se reescribe al mostrar la lista): tratarlo como «no se puede
 * saber» dejaría la marca apagada para siempre.
 */
export function leerVistos(
  storage: Pick<Storage, "getItem"> | null,
  clave: string | null,
): Set<string> | null {
  if (!storage || !clave) return null;
  let crudo: string | null;
  try {
    crudo = storage.getItem(clave);
  } catch {
    return null;
  }
  if (!crudo) return new Set();
  try {
    const lista: unknown = JSON.parse(crudo);
    if (!Array.isArray(lista)) return new Set();
    return new Set(lista.filter((x): x is string => typeof x === "string"));
  } catch {
    return new Set();
  }
}

/** Guarda los vistos (los últimos MAX_VISTOS). Si falla, no pasa nada: es una comodidad. */
export function escribirVistos(
  storage: Pick<Storage, "setItem"> | null,
  clave: string | null,
  vistos: Set<string> | null,
): void {
  if (!storage || !clave || !vistos) return;
  try {
    storage.setItem(clave, JSON.stringify([...vistos].slice(-MAX_VISTOS)));
  } catch {
    // Cuota llena o almacenamiento bloqueado: se pierde el «ya lo vi», nada más.
  }
}

/**
 * Suma ids a los vistos. Devuelve el MISMO conjunto si no hay nada nuevo, para que React no vuelva
 * a dibujar (y un efecto que dependa de esto no entre en bucle). Sin memoria sigue sin memoria.
 */
export function unirVistos(prev: Set<string> | null, ids: string[]): Set<string> | null {
  if (!prev) return null;
  if (ids.every((id) => prev.has(id))) return prev;
  return new Set([...prev, ...ids]);
}

/** Los avisos resueltos de la lista que todavía no se vieron. */
export function respuestasNuevas(
  items: Pick<MiAviso, "id" | "estado">[],
  vistos: Set<string> | null,
): Set<string> {
  if (!vistos) return new Set();
  return new Set(items.filter((a) => a.estado === "RESUELTO" && !vistos.has(a.id)).map((a) => a.id));
}

/** Los ids resueltos de una página (lo que se da por visto al mostrarla). */
export function idsResueltos(items: Pick<MiAviso, "id" | "estado">[]): string[] {
  return items.filter((a) => a.estado === "RESUELTO").map((a) => a.id);
}
