/**
 * Cuadre: TODOS los conceptos del período (incluye traslados/overnight) con su
 * Débito, Crédito y Neto (crédito − débito), y # de movimientos. Filas
 * desplegables a clasificación. Totales al pie. Los traslados aparecen y cuadran
 * (débito ≈ crédito); no afectan el EBITDA (eso lo maneja el KPI).
 * Fase B: con `periodo`, cada clasificación abre un drill-down con los
 * movimientos que la componen.
 */

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda, toNumber } from "@/lib/format";
import { BuscadorMultiple } from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useMovimientos } from "@/features/bancos/hooks";
import type { CuadreArbol as CuadreArbolData } from "@/api/bancos";

const GRID = "grid grid-cols-[minmax(160px,1fr)_3.5rem_11rem_11rem_11rem] items-center gap-3";

function netoTone(n: number): string {
  return n >= 0 ? "text-positivo" : "text-negativo";
}

function monto(v: string): string {
  return toNumber(v) !== 0 ? formatMoneda(v) : "—";
}

export function CuadreArbolView({
  data,
  periodo,
  etiquetaPadre = "Concepto",
  nota,
}: {
  data: CuadreArbolData;
  periodo?: string;
  /** Qué representa el primer nivel. El resumen de la selección lo reusa con «Banco · cuenta». */
  etiquetaPadre?: string;
  /** Nota al pie. Si no se pasa, se usa la del Cuadre (ingresos/gastos/traslados). */
  nota?: React.ReactNode;
}) {
  const [expandidos, setExpandidos] = useState<Set<string>>(new Set());
  const [drill, setDrill] = useState<string | null>(null); // "conceptoId:clasifId"
  const [elegidos, setElegidos] = useState<string[]>([]);

  /**
   * Selección de partidas: uno o VARIOS conceptos, para comparar sin el ruido del resto.
   *
   * Es selección múltiple y no un campo de texto porque el caso real es «Gastos contra Ingresos»:
   * con texto libre solo se puede mirar un patrón a la vez y la comparación no se puede armar.
   * Se escribe para encontrar el concepto entre los 20, pero lo que queda es una lista.
   *
   * Las filas NO se abren solas al filtrar: lo que se quiere validar acá son los totales por
   * concepto, y desplegar las clasificaciones tapa justamente eso.
   */
  const opcConceptos = data.conceptos.map((c) => ({
    value: c.concepto_id || "sin-concepto",
    label: c.concepto,
  }));
  const conceptosVisibles =
    elegidos.length > 0
      ? data.conceptos.filter((c) => elegidos.includes(c.concepto_id || "sin-concepto"))
      : data.conceptos;
  const filtrando = elegidos.length > 0;

  // Con la selección puesta, los totales del pie miden LO ELEGIDO: si siguieran mostrando el total
  // del período, el número de abajo no tendría nada que ver con las filas de arriba — y el sentido
  // de esta pantalla es comparar partidas, así que el subtotal de la comparación es el dato.
  const totales = filtrando
    ? conceptosVisibles.reduce(
        (acc, c) => ({
          movs: acc.movs + c.movs,
          debito: acc.debito + toNumber(c.debito),
          credito: acc.credito + toNumber(c.credito),
        }),
        { movs: 0, debito: 0, credito: 0 },
      )
    : {
        movs: data.conceptos.reduce((a, c) => a + c.movs, 0),
        debito: toNumber(data.total_debito),
        credito: toNumber(data.total_credito),
      };
  const totalNeto = totales.credito - totales.debito;

  function toggle(id: string) {
    setExpandidos((prev) => {
      const n = new Set(prev);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-end gap-3">
        <BuscadorMultiple
          label={`Ver solo ${etiquetaPadre.toLowerCase()}`}
          leyendaVacio="todos"
          placeholder={`Elegí uno o varios para comparar…`}
          opciones={opcConceptos}
          seleccion={elegidos}
          onChange={setElegidos}
          className="w-full max-w-md"
        />
        {filtrando && (
          <div className="flex items-center gap-2 pb-1.5">
            <span className="text-xs text-content-muted">
              {conceptosVisibles.length} de {data.conceptos.length} · los totales miden lo elegido
            </span>
            <button type="button" onClick={() => setElegidos([])} className="text-xs text-accent underline">
              Ver todo
            </button>
          </div>
        )}
      </div>
      <div className="overflow-x-auto">
      <div className="min-w-[820px] overflow-hidden rounded-xl border border-border">
        <div
          className={cn(
            GRID,
            "border-b border-border bg-surface-muted px-4 py-2 text-[11px] font-semibold uppercase tracking-wide text-content-muted",
          )}
        >
          <span>{etiquetaPadre}</span>
          <span className="text-right">Movs</span>
          <span className="text-right">Débito</span>
          <span className="text-right">Crédito</span>
          <span className="text-right">Neto</span>
        </div>

        {conceptosVisibles.map((c) => {
          const abierto = expandidos.has(c.concepto_id);
          const tieneHijos = c.clasificaciones.length > 0;
          const neto = toNumber(c.credito) - toNumber(c.debito);
          return (
            <div key={c.concepto_id || "sin-concepto"}>
              <button
                type="button"
                onClick={() => tieneHijos && toggle(c.concepto_id)}
                className={cn(
                  GRID,
                  "w-full border-b border-border px-4 py-2 text-left transition-colors",
                  tieneHijos && "hover:bg-surface-muted",
                )}
              >
                <span className="flex items-center gap-2">
                  {tieneHijos ? (
                    abierto ? (
                      <ChevronDown className="h-4 w-4 shrink-0 text-content-muted" aria-hidden />
                    ) : (
                      <ChevronRight className="h-4 w-4 shrink-0 text-content-muted" aria-hidden />
                    )
                  ) : (
                    <span className="w-4 shrink-0" />
                  )}
                  <span className="truncate font-medium text-content">{c.concepto}</span>
                </span>
                <span className="text-right whitespace-nowrap tabular-nums text-content-muted">{c.movs}</span>
                <span className="text-right whitespace-nowrap tabular-nums text-content-muted">{monto(c.debito)}</span>
                <span className="text-right whitespace-nowrap tabular-nums text-content-muted">{monto(c.credito)}</span>
                <span className={cn("text-right font-medium whitespace-nowrap tabular-nums", netoTone(neto))}>
                  {formatMoneda(String(neto))}
                </span>
              </button>

              {abierto &&
                c.clasificaciones.map((cl) => {
                  const netoCl = toNumber(cl.credito) - toNumber(cl.debito);
                  const key = `${c.concepto_id}:${cl.clasificacion_id}`;
                  const drillAbierto = drill === key;
                  return (
                    <div key={key}>
                      <button
                        type="button"
                        onClick={() => periodo && setDrill(drillAbierto ? null : key)}
                        className={cn(
                          GRID,
                          "w-full border-b border-border bg-surface px-4 py-1.5 text-left text-sm",
                          periodo && "cursor-pointer hover:bg-surface-muted",
                          drillAbierto && "bg-accent/5",
                        )}
                        title={periodo ? "Ver los movimientos que componen esta clasificación" : undefined}
                      >
                        <span className="truncate pl-6 text-content-muted">
                          {cl.clasificacion}
                          {periodo && <span className="ml-1.5 text-[10px] text-accent">{drillAbierto ? "▾" : "▸"}</span>}
                        </span>
                        <span className="text-right whitespace-nowrap tabular-nums text-content-muted">{cl.movs}</span>
                        <span className="text-right whitespace-nowrap tabular-nums text-content-muted">{monto(cl.debito)}</span>
                        <span className="text-right whitespace-nowrap tabular-nums text-content-muted">{monto(cl.credito)}</span>
                        <span className={cn("text-right whitespace-nowrap tabular-nums", netoTone(netoCl))}>
                          {formatMoneda(String(netoCl))}
                        </span>
                      </button>
                      {drillAbierto && periodo && (
                        <DrillMovimientos
                          periodo={periodo}
                          conceptoId={c.concepto_id}
                          clasificacionId={cl.clasificacion_id}
                          total={cl.movs}
                        />
                      )}
                    </div>
                  );
                })}
            </div>
          );
        })}

        <div className={cn(GRID, "bg-surface-muted px-4 py-2.5 font-semibold")}>
          <span className="text-content">Totales</span>
          <span className="text-right whitespace-nowrap tabular-nums text-content-muted">{totales.movs}</span>
          <span className="text-right whitespace-nowrap tabular-nums text-negativo">{formatMoneda(String(totales.debito))}</span>
          <span className="text-right whitespace-nowrap tabular-nums text-positivo">{formatMoneda(String(totales.credito))}</span>
          <span className={cn("text-right whitespace-nowrap tabular-nums", netoTone(totalNeto))}>
            {formatMoneda(String(totalNeto))}
          </span>
        </div>
      </div>
      </div>

      <p className="text-xs text-content-muted">
        {nota ?? (
          <>
            Ingresos = total de créditos ({formatMoneda(data.total_credito)}) · Gastos = total de débitos (
            {formatMoneda(data.total_debito)}). Incluye traslados/overnight (por eso su débito ≈ crédito y
            cuadran); esos no afectan el EBITDA de los KPIs.
          </>
        )}
      </p>
    </div>
  );
}

/** Drill-down: los movimientos de una clasificación dentro del período. */
function DrillMovimientos({
  periodo,
  conceptoId,
  clasificacionId,
  total,
}: {
  periodo: string;
  conceptoId: string;
  clasificacionId: string;
  total: number;
}) {
  const q = useMovimientos({
    periodo,
    concepto_id: conceptoId,
    clasificacion_id: clasificacionId,
    page: 1,
    page_size: 8,
  });
  const items = q.data?.items ?? [];
  return (
    <div className="border-b border-border bg-surface-muted/40 px-4 py-2 pl-12">
      {q.isPending ? (
        <p className="py-1 text-xs text-content-muted">Cargando movimientos…</p>
      ) : q.isError ? (
        <p className="py-1 text-xs text-negativo">{mensajeError(q.error)}</p>
      ) : items.length === 0 ? (
        <p className="py-1 text-xs text-content-muted">Sin movimientos.</p>
      ) : (
        <>
          <table className="w-full text-xs">
            <tbody>
              {items.map((m) => (
                <tr key={m.id} className="border-b border-border/60 last:border-0">
                  <td className="w-20 py-1 pr-3 whitespace-nowrap tabular-nums text-content-muted">
                    {formatFecha(m.fecha)}
                  </td>
                  <td className="max-w-0 truncate py-1 pr-3" title={m.descripcion}>
                    {m.descripcion}
                  </td>
                  <td className="w-28 py-1 text-right whitespace-nowrap tabular-nums font-medium">
                    {formatMoneda(m.monto_crc)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {total > items.length && (
            <p className="pt-1.5 text-[11px] text-content-muted">
              Mostrando {items.length} de {total} — el detalle completo vive en la Bandeja de clasificación.
            </p>
          )}
        </>
      )}
    </div>
  );
}
