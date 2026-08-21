/**
 * Clasificador bancario de UN SOLO CAMPO (mismo patrón que GastoCombobox de CxP,
 * validado en la maqueta de la Bandeja de clasificación): clic en la celda →
 * popover con búsqueda → elegís "Concepto › Clasificación" y se aplica al instante.
 * Si no existe, "+ Crear «…»" la crea ahí mismo (find-or-create en 2 niveles; con
 * «›» o «/» se separan; 1 solo nivel cae bajo "Gastos" si es débito o "Ingresos"
 * si es crédito). Portal con posición fija para que la tabla no lo recorte.
 */

import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { createPortal } from "react-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { bancosApi } from "@/api/bancos";
import { queryKeys } from "@/api/queryKeys";
import { useConceptos, useClasificaciones } from "@/features/bancos/hooks";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

export interface ClasifElegida {
  conceptoId: string;
  clasificacionId: string;
  ruta: string;
}

/** Normaliza para buscar/comparar: minúsculas y sin tildes ("Depósito" ≡ "deposito"). */
function norm(s: string): string {
  return s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "");
}

interface Opcion extends ClasifElegida {
  buscable: string;
  /** Solo el nombre de la clasificación, normalizado: con esto se mide la relevancia. */
  soloNombre: string;
}

export function ClasifCombobox({
  actual,
  auto,
  esDebito,
  onElegir,
  disabled,
  placeholder = "Clasificar",
}: {
  /** Ruta actual mostrada en la celda ("" = sin clasificar). */
  actual: string;
  /** La clasificación vigente vino del motor (chip AUTO). */
  auto?: boolean;
  /** Signo del movimiento: decide el concepto por defecto al crear con 1 nivel. */
  esDebito?: boolean;
  onElegir: (c: ClasifElegida) => void;
  disabled?: boolean;
  placeholder?: string;
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
          "inline-flex max-w-64 items-center gap-1.5 rounded-md border border-dashed border-border px-2 py-1 text-left text-sm transition-colors hover:bg-surface-muted",
          !actual && "font-medium text-pendiente",
        )}
        title={actual ? "Cambiar clasificación" : "Clasificar movimiento"}
      >
        <span className="truncate">{actual || placeholder}</span>
        {auto && actual && (
          <span className="rounded bg-accent/15 px-1 text-[9px] font-bold uppercase tracking-wide text-accent">
            auto
          </span>
        )}
        <span className="text-[10px] text-content-muted">{actual ? "✎" : "▾"}</span>
      </button>
      {abierto && (
        <ClasifPopover
          anchor={anchorRef.current}
          esDebito={esDebito}
          onCerrar={() => setAbierto(false)}
          onElegir={(c) => {
            setAbierto(false);
            onElegir(c);
          }}
        />
      )}
    </>
  );
}

function ClasifPopover({
  anchor,
  esDebito,
  onCerrar,
  onElegir,
}: {
  anchor: HTMLButtonElement | null;
  esDebito?: boolean;
  onCerrar: () => void;
  onElegir: (c: ClasifElegida) => void;
}) {
  const toast = useToast();
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  const conceptosQ = useConceptos();
  const clasifsQ = useClasificaciones();

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

  const opciones = useMemo<Opcion[]>(
    () =>
      (clasifsQ.data ?? [])
        .map((c) => ({
          conceptoId: c.concepto_id,
          clasificacionId: c.id,
          ruta: `${c.concepto} › ${c.nombre}`,
          buscable: norm(`${c.concepto} ${c.nombre}`),
          soloNombre: norm(c.nombre),
        }))
        .sort((a, b) => a.ruta.localeCompare(b.ruta)),
    [clasifsQ.data],
  );

  const texto = q.trim();
  const MAX_VISIBLES = 40;

  /**
   * Filtrado por relevancia. El orden importa tanto como el filtro.
   *
   * El texto se busca en «concepto + clasificación», así que escribir «gas» también calza con
   * las 112 clasificaciones del concepto «Gastos». Antes se cortaba en 40 sobre la lista
   * alfabética y la clasificación «Gas» —que cae en la posición 55— nunca se veía: el usuario
   * escribía su nombre exacto y el sistema le decía que no existía.
   *
   * Ahora primero se ordena por qué tan bien calza con el NOMBRE de la clasificación, y recién
   * después se corta. Lo que uno escribe aparece arriba, y lo que se pierde por el tope es lo
   * que solo coincidía por el nombre del concepto.
   */
  const { filtradas, coinciden } = useMemo(() => {
    const t = norm(texto);
    if (!t) return { filtradas: opciones.slice(0, MAX_VISIBLES), coinciden: opciones.length };
    const calzan = opciones.filter((o) => o.buscable.includes(t));
    const relevancia = (o: Opcion): number => {
      if (o.soloNombre === t) return 0; // el nombre exacto
      if (o.soloNombre.startsWith(t)) return 1; // el nombre empieza así
      if (o.soloNombre.includes(t)) return 2; // el nombre lo contiene
      return 3; // solo calza por el concepto
    };
    const ordenadas = [...calzan].sort(
      (a, b) => relevancia(a) - relevancia(b) || a.ruta.localeCompare(b.ruta),
    );
    return { filtradas: ordenadas.slice(0, MAX_VISIBLES), coinciden: calzan.length };
  }, [opciones, texto]);
  const exacta = opciones.some((o) => norm(o.ruta) === norm(texto));

  // Find-or-create de la ruta escrita: "Concepto › Clasificación" (o con "/").
  // Con 1 solo nivel, el concepto se infiere del FLUJO del movimiento: débito → "Gastos",
  // crédito → "Ingresos". Si el flujo NO es único (lote mixto), exige el concepto explícito
  // en vez de degradar a "Gastos" (un ingreso nunca debe caer bajo un concepto de gasto).
  async function crearRuta() {
    if (!texto || creando) return;
    setCreando(true);
    try {
      const partes = texto.split(/›|\//).map((s) => s.trim()).filter(Boolean);
      let nc: string;
      let ncl: string;
      if (partes.length >= 2) {
        [nc, ncl] = [partes[0]!, partes[1]!];
      } else if (esDebito === undefined) {
        toast.error(`Indicá el concepto: «Ingresos › ${partes[0]}» o «Gastos › ${partes[0]}» (la selección mezcla créditos y débitos).`);
        return;
      } else {
        nc = esDebito ? "Gastos" : "Ingresos";
        ncl = partes[0]!;
      }

      // Concepto (find-or-create insensible a mayúsculas y tildes)
      let conceptos = conceptosQ.data ?? [];
      let con = conceptos.find((c) => norm(c.nombre) === norm(nc));
      if (!con) {
        try {
          // Concepto creado desde la clasificación BANCARIA: no se expone a CxP
          // (si contabilidad lo necesita, se enciende en Catálogo).
          con = await bancosApi.crearConcepto({ nombre: nc, visible_cxp: false });
        } catch {
          conceptos = await bancosApi.conceptos();
          con = conceptos.find((c) => norm(c.nombre) === norm(nc));
        }
      }
      if (!con) throw new Error("no se pudo crear el concepto");

      // Clasificación (find-or-create insensible a mayúsculas y tildes)
      let clasifs = clasifsQ.data ?? [];
      let cla = clasifs.find((c) => c.concepto_id === con!.id && norm(c.nombre) === norm(ncl));
      if (!cla) {
        try {
          cla = await bancosApi.crearClasificacion({ concepto_id: con.id, nombre: ncl });
        } catch {
          clasifs = await bancosApi.clasificaciones();
          cla = clasifs.find((c) => c.concepto_id === con!.id && norm(c.nombre) === norm(ncl));
        }
      }
      if (!cla) throw new Error("no se pudo crear la clasificación");

      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conceptosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.clasificacionesRaiz(empresaId) });
      const ruta = `${con.nombre} › ${cla.nombre}`;
      toast.success(`Clasificación lista: ${ruta}`);
      onElegir({ conceptoId: con.id, clasificacionId: cla.id, ruta });
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
        placeholder="Buscar o crear clasificación…"
        className="w-full border-b border-border bg-transparent px-3 py-2.5 text-sm text-content placeholder:text-content-muted focus:outline-none"
      />
      <ul className="max-h-64 overflow-y-auto p-1.5">
        {conceptosQ.isPending || clasifsQ.isPending ? (
          <li className="px-2 py-3 text-center text-xs text-content-muted">Cargando catálogo…</li>
        ) : (
          <>
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
                Sin clasificaciones todavía — escribí para crear la primera.
              </li>
            )}
            {/* Decir que la lista está recortada. Sin este aviso, no ver algo que sí existe se
                lee como «no existe», y el usuario termina creando un duplicado. */}
            {coinciden > filtradas.length && (
              <li className="px-2 py-2 text-center text-xs text-content-muted">
                Mostrando {filtradas.length} de {coinciden} — seguí escribiendo para afinar.
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
        Niveles con «›» o «/»: <span className="font-mono">Gastos › Electricidad</span>
      </p>
    </div>,
    document.body,
  );
}
