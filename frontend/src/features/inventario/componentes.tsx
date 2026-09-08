/**
 * Piezas compartidas por las pantallas del inventario.
 *
 * Viven acá y no repetidas en cada pantalla porque son las que tienen que decir LO MISMO en todas:
 * un artículo «bajo el mínimo» no puede verse de un color en existencias y de otro en reposición.
 */

import { Badge } from "@/components/ui";
import type { BadgeTone } from "@/components/ui";
import { cn } from "@/lib/cn";
import type { EstadoUnidad, LecturaRotacion, ModoControl, NivelExistencia } from "@/api/inventario";

/** El semáforo de existencia, con el nombre que usa la gente. */
const NIVEL: Record<NivelExistencia, { tono: BadgeTone; texto: string }> = {
  SIN_NIVEL: { tono: "neutral", texto: "sin mínimo definido" },
  EN_RANGO: { tono: "positivo", texto: "en rango" },
  CERCA_DEL_MINIMO: { tono: "pendiente", texto: "cerca del mínimo" },
  BAJO_MINIMO: { tono: "negativo", texto: "bajo el mínimo" },
  SOBRE_MAXIMO: { tono: "pendiente", texto: "sobre el máximo" },
};

export function MarcaNivel({ estado }: { estado: NivelExistencia }) {
  const n = NIVEL[estado] ?? NIVEL.SIN_NIVEL;
  return <Badge tone={n.tono}>{n.texto}</Badge>;
}

/** El estado de una unidad física. */
const ESTADO_UNIDAD: Record<EstadoUnidad, BadgeTone> = {
  DISPONIBLE: "positivo",
  RESERVADA: "pendiente",
  EN_TRANSITO: "neutral",
  EXHIBICION: "accent",
  USADA: "neutral",
  DANADA: "negativo",
  DEVUELTA: "neutral",
  NO_APARECIO: "negativo",
};

export function MarcaEstadoUnidad({
  estado,
  legible,
}: {
  estado: EstadoUnidad;
  legible: string;
}) {
  return <Badge tone={ESTADO_UNIDAD[estado] ?? "neutral"}>{legible}</Badge>;
}

/** Cómo se cuenta el artículo. Es la distinción central del módulo, así que se ve siempre. */
export function MarcaModo({ modo }: { modo: ModoControl }) {
  return (
    <span className="text-xs text-content-muted">
      {modo === "UNIDAD" ? "por unidad" : "por cantidad"}
    </span>
  );
}

/** La lectura de rotación: importa más la frase que el número. */
const ROTACION: Record<LecturaRotacion, { tono: BadgeTone; texto: string }> = {
  SANO: { tono: "positivo", texto: "rota bien" },
  LENTO: { tono: "pendiente", texto: "rota lento" },
  DETENIDO: { tono: "negativo", texto: "detenido" },
  SIN_SALIDAS: { tono: "neutral", texto: "sin salidas" },
};

export function MarcaRotacion({ lectura }: { lectura: LecturaRotacion }) {
  const r = ROTACION[lectura] ?? ROTACION.SIN_SALIDAS;
  return <Badge tone={r.tono}>{r.texto}</Badge>;
}

/**
 * El aviso de la cabecera. Se muestra tal cual lo redacta el backend: la frase ya explica por qué
 * el número puede no ser confiable, y reescribirla acá abriría la puerta a que las dos versiones
 * dejen de coincidir.
 */
export function AvisoInventario({ texto }: { texto: string }) {
  if (!texto) return null;
  return (
    <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
      <p className="text-sm font-medium text-content">Ojo con estos números</p>
      <p className="mt-0.5 text-sm text-content-muted">{texto}</p>
    </div>
  );
}

/** Una barra que muestra la existencia contra su mínimo y su máximo. */
export function BarraNivel({
  hay,
  minimo,
  maximo,
  estado,
}: {
  hay: number;
  minimo: number;
  maximo: number;
  estado: NivelExistencia;
}) {
  if (minimo === 0 && maximo === 0) return null;
  const tope = Math.max(maximo, minimo, hay, 1);
  const pct = Math.min(100, Math.round((hay / tope) * 100));
  return (
    <span className="mt-1 block h-1 w-24 bg-surface-muted" aria-hidden="true">
      <span
        className={cn(
          "block h-1",
          estado === "BAJO_MINIMO"
            ? "bg-negativo"
            : estado === "CERCA_DEL_MINIMO" || estado === "SOBRE_MAXIMO"
              ? "bg-pendiente"
              : "bg-positivo",
        )}
        style={{ width: `${pct}%` }}
      />
    </span>
  );
}
