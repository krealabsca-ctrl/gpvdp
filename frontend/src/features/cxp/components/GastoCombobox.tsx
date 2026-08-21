/**
 * Clasificador de gasto de UN SOLO CAMPO (patrón enterprise, validado en la maqueta):
 * clic en la celda → popover con búsqueda → elegís la ruta completa ("Concepto › Clasificación
 * › Subclasif") y se aplica al instante. Si no existe, "+ Crear «…»" la crea ahí mismo
 * (find-or-create en los 3 niveles; con «›» o «/» se separan niveles, si no, cae en "Gastos").
 * El popover se renderiza en un portal con posición fija para no quedar oculto por la tabla.
 */

import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { createPortal } from "react-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { bancosApi } from "@/api/bancos";
import { cxpApi } from "@/api/cxp";
import { queryKeys } from "@/api/queryKeys";
import { useConceptos, useClasificaciones } from "@/features/bancos/hooks";
import { useGastosProveedor, useSubclasificacionesTodas } from "@/features/cxp/hooks";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

export interface GastoElegido {
  conceptoId: string;
  clasificacionId: string;
  subclasificacionId: string;
  ruta: string;
}

interface Opcion extends GastoElegido {
  buscable: string;
}

export function GastoCombobox({
  actual,
  auto,
  onElegir,
  disabled,
  proveedorId,
}: {
  /** Ruta actual mostrada en la celda ("" = sin clasificar). */
  actual: string;
  /** Vino de la memoria del proveedor (chip AUTO). */
  auto?: boolean;
  onElegir: (g: GastoElegido) => void;
  disabled?: boolean;
  /** Si se pasa, el popover muestra primero los gastos frecuentes del proveedor. */
  proveedorId?: string;
}) {
  const [abierto, setAbierto] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        disabled={disabled}
        onClick={() => setAbierto((v) => !v)}
        className={cn(
          "inline-flex max-w-60 items-center gap-1.5 rounded-md border border-dashed border-border px-2 py-1 text-left text-sm transition-colors hover:bg-surface-muted",
          !actual && "font-medium text-pendiente",
        )}
        title={actual ? "Cambiar segmentación" : "Clasificar gasto"}
      >
        <span className="truncate">{actual || "Clasificar gasto"}</span>
        {auto && actual && (
          <span className="rounded bg-pendiente/15 px-1 text-[9px] font-bold uppercase tracking-wide text-pendiente">
            auto
          </span>
        )}
        <span className="text-[10px] text-content-muted">{actual ? "✎" : "▾"}</span>
      </button>
      {abierto && (
        <GastoPopover
          anchor={anchorRef.current}
          proveedorId={proveedorId}
          onCerrar={() => setAbierto(false)}
          onElegir={(g) => {
            setAbierto(false);
            onElegir(g);
          }}
        />
      )}
    </>
  );
}

function GastoPopover({
  anchor,
  proveedorId,
  onCerrar,
  onElegir,
}: {
  anchor: HTMLButtonElement | null;
  proveedorId?: string;
  onCerrar: () => void;
  onElegir: (g: GastoElegido) => void;
}) {
  const toast = useToast();
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  // Vista CxP del catálogo: solo conceptos visibles para contabilidad (los
  // sensibles del catálogo bancario no aparecen aquí).
  const conceptosQ = useConceptos("cxp");
  const clasifsQ = useClasificaciones("cxp");
  const subsQ = useSubclasificacionesTodas();
  const frecuentesQ = useGastosProveedor(proveedorId);

  const [q, setQ] = useState("");
  const [creando, setCreando] = useState(false);
  const popRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Posición fija bajo el ancla (portal → nunca lo recorta la tabla).
  const rect = anchor?.getBoundingClientRect();
  const estilo = rect
    ? {
        left: Math.min(rect.left, window.innerWidth - 320),
        top: rect.bottom + 6 + 300 > window.innerHeight ? Math.max(8, rect.top - 306) : rect.bottom + 6,
      }
    : { left: 0, top: 0 };

  useEffect(() => {
    inputRef.current?.focus();
    function fuera(e: MouseEvent) {
      if (popRef.current && !popRef.current.contains(e.target as Node)) onCerrar();
    }
    function tecla(e: globalThis.KeyboardEvent) {
      if (e.key === "Escape") onCerrar();
    }
    document.addEventListener("mousedown", fuera);
    document.addEventListener("keydown", tecla);
    return () => {
      document.removeEventListener("mousedown", fuera);
      document.removeEventListener("keydown", tecla);
    };
  }, [onCerrar]);

  // Opciones: cada clasificación (Concepto › Clasif) + cada subclasificación (ruta completa).
  const opciones = useMemo<Opcion[]>(() => {
    const clasifs = clasifsQ.data ?? [];
    const subs = subsQ.data ?? [];
    const out: Opcion[] = clasifs.map((c) => ({
      conceptoId: c.concepto_id,
      clasificacionId: c.id,
      subclasificacionId: "",
      ruta: `${c.concepto} › ${c.nombre}`,
      buscable: `${c.concepto} ${c.nombre}`.toLowerCase(),
    }));
    for (const s of subs) {
      const c = clasifs.find((x) => x.id === s.clasificacion_id);
      if (!c) continue;
      out.push({
        conceptoId: c.concepto_id,
        clasificacionId: c.id,
        subclasificacionId: s.id,
        ruta: `${c.concepto} › ${c.nombre} › ${s.nombre}`,
        buscable: `${c.concepto} ${c.nombre} ${s.nombre}`.toLowerCase(),
      });
    }
    return out.sort((a, b) => a.ruta.localeCompare(b.ruta));
  }, [clasifsQ.data, subsQ.data]);

  const texto = q.trim();
  const filtradas = useMemo(() => {
    const t = texto.toLowerCase();
    return (t ? opciones.filter((o) => o.buscable.includes(t)) : opciones).slice(0, 40);
  }, [opciones, texto]);
  const exacta = opciones.some((o) => o.ruta.toLowerCase() === texto.toLowerCase());

  // Find-or-create de la ruta escrita: "A › B › C" (o con "/"), 1 nivel cae bajo "Gastos".
  async function crearRuta() {
    if (!texto || creando) return;
    setCreando(true);
    try {
      const partes = texto.split(/›|\//).map((s) => s.trim()).filter(Boolean);
      const [nc, ncl, ns] =
        partes.length >= 3 ? partes : partes.length === 2 ? [partes[0]!, partes[1]!, ""] : ["Gastos", partes[0]!, ""];

      // Concepto
      let conceptos = conceptosQ.data ?? [];
      let con = conceptos.find((c) => c.nombre.toLowerCase() === nc!.toLowerCase());
      if (!con) {
        try {
          // Creado desde CxP → visible para CxP.
          con = await bancosApi.crearConcepto({ nombre: nc!, visible_cxp: true });
        } catch {
          conceptos = await bancosApi.conceptos("cxp");
          con = conceptos.find((c) => c.nombre.toLowerCase() === nc!.toLowerCase());
        }
      }
      if (!con) throw new Error("no se pudo crear el concepto");

      // Clasificación
      let clasifs = clasifsQ.data ?? [];
      let cla = clasifs.find((c) => c.concepto_id === con!.id && c.nombre.toLowerCase() === ncl!.toLowerCase());
      if (!cla) {
        try {
          cla = await bancosApi.crearClasificacion({ concepto_id: con.id, nombre: ncl! });
        } catch {
          clasifs = await bancosApi.clasificaciones("cxp");
          cla = clasifs.find((c) => c.concepto_id === con!.id && c.nombre.toLowerCase() === ncl!.toLowerCase());
        }
      }
      if (!cla) throw new Error("no se pudo crear la clasificación");

      // Subclasificación (opcional) — el backend es idempotente por nombre.
      let subId = "";
      let ruta = `${con.nombre} › ${cla.nombre}`;
      if (ns) {
        const sub = await cxpApi.crearSubclasificacion(cla.id, ns);
        subId = sub.id;
        ruta += ` › ${sub.nombre}`;
      }

      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conceptosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.clasificacionesRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.subclasificacionesRaiz(empresaId) });
      toast.success(`Categoría lista: ${ruta}`);
      onElegir({ conceptoId: con.id, clasificacionId: cla.id, subclasificacionId: subId, ruta });
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setCreando(false);
    }
  }

  function onKey(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key !== "Enter") return;
    e.preventDefault();
    const primera = filtradas[0];
    if (primera) onElegir(primera);
    else void crearRuta();
  }

  return createPortal(
    <div
      ref={popRef}
      style={{ position: "fixed", left: estilo.left, top: estilo.top, zIndex: 90 }}
      className="w-80 overflow-hidden rounded-xl border border-border bg-surface-raised shadow-lifted"
    >
      <input
        ref={inputRef}
        value={q}
        onChange={(e) => setQ(e.target.value)}
        onKeyDown={onKey}
        placeholder="Buscar o crear categoría…"
        className="w-full border-b border-border bg-transparent px-3 py-2.5 text-sm text-content placeholder:text-content-muted focus:outline-none"
      />
      <ul className="max-h-64 overflow-y-auto p-1.5">
        {conceptosQ.isPending || clasifsQ.isPending ? (
          <li className="px-2 py-3 text-center text-xs text-content-muted">Cargando catálogo…</li>
        ) : (
          <>
            {/* Gastos frecuentes del proveedor (clasificación en 1 clic) */}
            {!texto && (frecuentesQ.data?.length ?? 0) > 0 && (
              <>
                <li className="px-2 pb-0.5 pt-1 text-[10px] font-bold uppercase tracking-wider text-content-muted">
                  ★ Frecuentes de este proveedor
                </li>
                {frecuentesQ.data!.map((f) => {
                  const ruta = [f.concepto, f.clasificacion, f.subclasificacion].filter(Boolean).join(" › ");
                  return (
                    <li key={"f" + ruta}>
                      <button
                        type="button"
                        onClick={() =>
                          onElegir({
                            conceptoId: f.concepto_id,
                            clasificacionId: f.clasificacion_id,
                            subclasificacionId: f.subclasificacion_id,
                            ruta,
                          })
                        }
                        className="block w-full rounded-md bg-pendiente/10 px-2.5 py-1.5 text-left text-sm font-medium hover:bg-accent/10 hover:text-accent"
                      >
                        {ruta} <span className="text-[10px] text-content-muted">×{f.usos}</span>
                      </button>
                    </li>
                  );
                })}
                <li className="my-1 border-t border-border" aria-hidden />
              </>
            )}
            {filtradas.map((o, i) => (
              <li key={o.ruta}>
                <button
                  type="button"
                  onClick={() => onElegir(o)}
                  className={cn(
                    "block w-full rounded-md px-2.5 py-1.5 text-left text-sm hover:bg-accent/10 hover:text-accent",
                    i === 0 && "bg-accent/5",
                  )}
                >
                  {o.ruta}
                </button>
              </li>
            ))}
            {!filtradas.length && !texto && (
              <li className="px-2 py-3 text-center text-xs text-content-muted">
                Sin categorías todavía — escribí para crear la primera.
              </li>
            )}
            {texto && !exacta && (
              <li className="mt-1 border-t border-border pt-1">
                <button
                  type="button"
                  onClick={() => void crearRuta()}
                  disabled={creando}
                  className="block w-full rounded-md px-2.5 py-1.5 text-left text-sm font-semibold text-accent hover:bg-accent/10"
                >
                  {creando ? "Creando…" : `+ Crear «${texto}»`}
                </button>
              </li>
            )}
          </>
        )}
      </ul>
      <p className="border-t border-border px-3 py-1.5 text-[11px] text-content-muted">
        Niveles con «›» o «/»: <span className="font-mono">Combustible › Operaciones</span>
      </p>
    </div>,
    document.body,
  );
}
