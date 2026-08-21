/**
 * Resumen de LA SELECCIÓN ACTIVA de la hoja de trabajo.
 *
 * El problema que resuelve: la lista decía «1398 movimiento(s)» y nada más. Si estabas
 * trabajando Ingresos no sabías cuánto era, ni cómo se repartía entre débitos y créditos,
 * sin irte a otra pantalla. Ahora el encabezado responde a lo que tenés filtrado.
 *
 * Sale del MISMO filtro que la lista (mismo endpoint, mismas condiciones en SQL), así que
 * el número de arriba y las filas de abajo no pueden contradecirse.
 *
 * El desglose se agrupa según el área de trabajo:
 *   · «Por clasificar» → Banco › cuenta (agrupar por concepto no diría nada: todo es
 *     «sin concepto» por definición, y lo que uno quiere saber es de qué cuenta viene el bulto).
 *   · «Clasificados» y «Traslados» → Concepto › Clasificación, igual que el Cuadre.
 */

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatMoneda, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useResumenSeleccion } from "@/features/bancos/hooks";
import { CuadreArbolView } from "@/features/bancos/components/CuadreArbol";
import type { AgruparResumen, FiltrosMovimientos } from "@/api/bancos";

const ETIQUETA_PADRE: Record<AgruparResumen, string> = {
  concepto: "Concepto",
  cuenta: "Banco · cuenta",
};

export function ResumenSeleccion({
  filtros,
  agrupar,
  /** Texto que describe qué se está mirando (p. ej. «Ingresos» o «solo traslados»). */
  descripcion,
}: {
  filtros: FiltrosMovimientos;
  agrupar: AgruparResumen;
  descripcion?: string;
}) {
  const [abierto, setAbierto] = useState(false);
  const q = useResumenSeleccion(filtros, agrupar);
  const r = q.data;

  // Sin datos todavía: no se pinta un cero que después salta a otro número.
  if (q.isError) {
    return (
      <p className="rounded-lg border border-border bg-surface-raised px-4 py-2 text-xs text-negativo">
        No se pudo calcular el resumen: {mensajeError(q.error)}
      </p>
    );
  }

  const neto = r ? toNumber(r.neto) : 0;
  const cargando = q.isPending;

  return (
    <div className="rounded-xl border border-border bg-surface-raised shadow-card">
      <div className="flex flex-wrap items-stretch gap-x-8 gap-y-3 px-4 py-3">
        <Cifra
          etiqueta="Movimientos"
          valor={cargando ? "—" : (r?.movs ?? 0).toLocaleString("es-CR")}
          nota={descripcion}
        />
        <Cifra
          etiqueta="Débitos"
          valor={cargando ? "—" : formatMoneda(r?.total_debito ?? "0")}
          tono="negativo"
          nota="lo que salió"
        />
        <Cifra
          etiqueta="Créditos"
          valor={cargando ? "—" : formatMoneda(r?.total_credito ?? "0")}
          tono="positivo"
          nota="lo que entró"
        />
        <Cifra
          etiqueta="Neto"
          valor={cargando ? "—" : formatMoneda(String(neto))}
          tono={neto >= 0 ? "positivo" : "negativo"}
          nota="créditos − débitos"
        />

        <div className="ml-auto flex items-center gap-3">
          {q.isFetching && !cargando && <span className="text-xs text-accent">actualizando…</span>}
          <button
            type="button"
            onClick={() => setAbierto((v) => !v)}
            disabled={cargando || (r?.movs ?? 0) === 0}
            className={cn(
              "flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium transition-colors",
              "hover:border-accent hover:text-accent disabled:cursor-not-allowed disabled:opacity-50",
              abierto && "border-accent text-accent",
            )}
            aria-expanded={abierto}
          >
            {abierto ? (
              <ChevronDown className="h-3.5 w-3.5" aria-hidden />
            ) : (
              <ChevronRight className="h-3.5 w-3.5" aria-hidden />
            )}
            Desglose por {agrupar === "cuenta" ? "cuenta" : "concepto"}
          </button>
        </div>
      </div>

      {abierto && r && (
        <div className="border-t border-border px-4 py-3">
          <CuadreArbolView
            data={{
              periodo: "",
              total_debito: r.total_debito,
              total_credito: r.total_credito,
              movs: r.movs,
              conceptos: r.conceptos,
            }}
            etiquetaPadre={ETIQUETA_PADRE[r.agrupar] ?? ETIQUETA_PADRE[agrupar]}
            nota={
              <>
                Es el desglose de <b>lo que tenés filtrado ahora</b>, no del período completo. Montos en
                colones (los movimientos en dólares entran a su equivalente, para no sumar monedas
                distintas).
              </>
            }
          />
        </div>
      )}
    </div>
  );
}

function Cifra({
  etiqueta,
  valor,
  nota,
  tono,
}: {
  etiqueta: string;
  valor: string;
  nota?: string;
  tono?: "positivo" | "negativo";
}) {
  return (
    <div className="min-w-[8.5rem]">
      <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">{etiqueta}</p>
      <p
        className={cn(
          "text-lg font-semibold tabular-nums",
          tono === "negativo" ? "text-negativo" : tono === "positivo" ? "text-positivo" : "text-content",
        )}
      >
        {valor}
      </p>
      {nota && <p className="text-[10.5px] text-content-muted">{nota}</p>}
    </div>
  );
}
