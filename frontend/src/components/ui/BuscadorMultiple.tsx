/**
 * BuscadorMultiple — elegir varios de una lista larga, escribiendo.
 *
 * Existe porque la parrilla de chips no escala: para 22 conceptos alcanzaba, pero hay 168
 * clasificaciones vivas en Valle de Paz y van a ser más («fácilmente 400»). Pintarlas todas es
 * ilegible y buscar la que se necesita con la vista es trabajo manual.
 *
 * Cómo se usa:
 *  · Se escribe y la lista se reduce (calza por nombre y por grupo, sin tildes ni mayúsculas).
 *  · Lo elegido se ve arriba como ficha con su ✕, así que nunca hay que abrir la lista para saber
 *    qué está pedido.
 *  · Nada seleccionado = TODOS. Es la convención del armador de reportes y se dice en el rótulo.
 *
 * `grupo` agrupa las opciones con un encabezado (clasificación bajo su concepto): la misma
 * clasificación puede llamarse igual en dos conceptos y sin el grupo no se distinguen.
 */

import { useMemo, useRef, useState } from "react";
import { cn } from "@/lib/cn";

export interface OpcionBuscador {
  value: string;
  label: string;
  /** Encabezado bajo el que se agrupa (opcional). También se busca por acá. */
  grupo?: string;
}

export interface BuscadorMultipleProps {
  label: string;
  opciones: OpcionBuscador[];
  seleccion: string[];
  onChange: (valores: string[]) => void;
  /** Texto del campo de búsqueda. */
  placeholder?: string;
  /** Qué significa no elegir nada (se muestra al lado del rótulo). */
  leyendaVacio?: string;
  /** Cuántas opciones se listan a la vez antes de pedir que se afine la búsqueda. */
  maxVisibles?: number;
  className?: string;
}

/** Normaliza para buscar: sin tildes, sin mayúsculas. «cesión» encuentra «Cesion». */
function normalizar(s: string): string {
  return s
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase();
}

export function BuscadorMultiple({
  label,
  opciones,
  seleccion,
  onChange,
  placeholder = "Escribí para buscar…",
  leyendaVacio = "todas",
  maxVisibles = 60,
  className,
}: BuscadorMultipleProps) {
  const [q, setQ] = useState("");
  const [abierto, setAbierto] = useState(false);
  const cont = useRef<HTMLDivElement>(null);

  const porValue = useMemo(() => {
    const m = new Map<string, OpcionBuscador>();
    for (const o of opciones) m.set(o.value, o);
    return m;
  }, [opciones]);

  // Filtrado + agrupado en una pasada; se corta en maxVisibles para no pintar 400 filas.
  //
  // El corte va DESPUÉS de ordenar por relevancia, y eso es lo que evita el problema clásico:
  // el texto también se busca en el nombre del grupo, así que escribir «gas» calza con las 112
  // clasificaciones del concepto «Gastos», y un corte sobre la lista alfabética esconde la
  // clasificación que se llama exactamente «Gas». Lo que uno escribe tiene que quedar arriba.
  const { grupos, mostradas, coinciden } = useMemo(() => {
    const needle = normalizar(q.trim());
    let filtradas = opciones;
    if (needle) {
      filtradas = opciones.filter(
        (o) => normalizar(o.label).includes(needle) || normalizar(o.grupo ?? "").includes(needle),
      );
      const relevancia = (o: OpcionBuscador): number => {
        const l = normalizar(o.label);
        if (l === needle) return 0;
        if (l.startsWith(needle)) return 1;
        if (l.includes(needle)) return 2;
        return 3; // solo calza por el nombre del grupo
      };
      filtradas = [...filtradas].sort((a, b) => relevancia(a) - relevancia(b));
    }
    const visibles = filtradas.slice(0, maxVisibles);
    const g = new Map<string, OpcionBuscador[]>();
    for (const o of visibles) {
      const k = o.grupo ?? "";
      const arr = g.get(k);
      if (arr) arr.push(o);
      else g.set(k, [o]);
    }
    return { grupos: [...g.entries()], mostradas: visibles.length, coinciden: filtradas.length };
  }, [opciones, q, maxVisibles]);

  function alternar(value: string) {
    onChange(seleccion.includes(value) ? seleccion.filter((v) => v !== value) : [...seleccion, value]);
  }

  return (
    <div className={cn("flex flex-col gap-2", className)} ref={cont}>
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-semibold uppercase tracking-wide text-content-muted">
          {label}{" "}
          {seleccion.length === 0 ? (
            <span className="font-normal normal-case">· {leyendaVacio}</span>
          ) : (
            <span className="font-normal normal-case">
              · {seleccion.length} seleccionada{seleccion.length === 1 ? "" : "s"}
            </span>
          )}
        </p>
        {seleccion.length > 0 && (
          <button type="button" className="text-xs text-accent underline" onClick={() => onChange([])}>
            Quitar la selección
          </button>
        )}
      </div>

      {/* Lo elegido, siempre visible: la lista puede estar cerrada y la selección no se esconde. */}
      {seleccion.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {seleccion.map((v) => {
            const o = porValue.get(v);
            return (
              <span
                key={v}
                className="inline-flex items-center gap-1.5 rounded-lg border border-accent bg-accent/10 px-2 py-1 text-xs font-medium text-accent"
              >
                {o?.grupo && <span className="font-normal opacity-70">{o.grupo} ›</span>}
                {o?.label ?? v}
                <button
                  type="button"
                  aria-label={`Quitar ${o?.label ?? v}`}
                  className="rounded px-0.5 leading-none hover:bg-accent/20"
                  onClick={() => alternar(v)}
                >
                  ✕
                </button>
              </span>
            );
          })}
        </div>
      )}

      <input
        type="search"
        value={q}
        placeholder={placeholder}
        onChange={(e) => {
          setQ(e.target.value);
          setAbierto(true);
        }}
        onFocus={() => setAbierto(true)}
        onBlur={(e) => {
          // Se cierra al salir del componente, no al soltar el input: si no, el clic en una
          // opción cerraría la lista antes de registrarse.
          if (!cont.current?.contains(e.relatedTarget as Node | null)) setAbierto(false);
        }}
        className={cn(
          "h-10 w-full rounded-lg border border-border bg-surface-raised px-3 text-sm text-content shadow-sm",
          "transition-colors hover:border-content-muted/50 focus-visible:outline-none focus-visible:ring-2",
          "focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-surface",
        )}
      />

      {abierto && (
        <div className="max-h-72 overflow-y-auto rounded-xl border border-border bg-surface-raised p-1 shadow-sm">
          {coinciden === 0 && (
            <p className="px-2 py-3 text-sm text-content-muted">Nada calza con «{q}».</p>
          )}
          {grupos.map(([grupo, items]) => (
            <div key={grupo || "_"}>
              {grupo && (
                <p className="sticky top-0 bg-surface-raised px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-content-muted">
                  {grupo}
                </p>
              )}
              {items.map((o) => {
                const sel = seleccion.includes(o.value);
                return (
                  <button
                    key={o.value}
                    type="button"
                    onClick={() => alternar(o.value)}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm",
                      sel ? "bg-accent/10 font-medium text-accent" : "text-content hover:bg-surface-muted",
                    )}
                  >
                    <span
                      aria-hidden
                      className={cn(
                        "flex h-4 w-4 shrink-0 items-center justify-center rounded border text-[10px] leading-none",
                        sel ? "border-accent bg-accent text-white" : "border-border",
                      )}
                    >
                      {sel ? "✓" : ""}
                    </span>
                    {o.label}
                  </button>
                );
              })}
            </div>
          ))}
          {coinciden > mostradas && (
            <p className="px-2 py-2 text-xs text-content-muted">
              Se muestran {mostradas} de {coinciden}. Seguí escribiendo para afinar.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
