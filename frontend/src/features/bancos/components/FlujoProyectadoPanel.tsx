/**
 * El flujo esperado de los próximos meses.
 *
 * ── POR QUÉ NO PROYECTA POR VENCIMIENTOS ────────────────────────────────────────────────────
 *
 * Porque no funcionaría. Medido el 28-oct-2026 sobre la cartera real de Valle de Paz: de lo
 * pendiente, **₡936,9 M ya venció y solo ₡61,6 M vence a futuro**. Una proyección por vencimiento
 * pondría casi todo en «este mes» y nada después: no proyectaría, repetiría que la cartera está
 * vencida, que ya lo dice el tablero de CxP.
 *
 * Se proyecta por PATRÓN: lo que cada partida suele mover, corregido por el mes del calendario.
 *
 * ── LO QUE DECLARA ──────────────────────────────────────────────────────────────────────────
 *
 * Sobre cuántos meses de historia se calculó, cuántas partidas tienen ritmo propio y cuántas no,
 * y si pudo corregir por estacionalidad. Una proyección que no dice de qué está hecha invita a
 * leerla como un compromiso.
 */

import { useState } from "react";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  ErrorState,
  LoadingState,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { etiquetaPeriodo, formatMoneda, formatMonto, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useFlujoProyectado } from "@/features/bancos/hooks";
import { restarMeses } from "@/features/bancos/analisisRango";

const HORIZONTES = [3, 6, 12];

export function FlujoProyectadoPanel({ periodo }: { periodo: string }) {
  const [meses, setMeses] = useState(6);
  // El patrón sale de los últimos 12 meses cerrados. Un rango más corto deja fuera los pagos
  // anuales —aguinaldos, seguros— y la proyección saldría optimista por omisión.
  const desde = restarMeses(periodo, 11);
  const q = useFlujoProyectado(desde, periodo, meses);
  const d = q.data;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Los próximos meses · estimado por patrón</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-sm text-content-muted">Proyectar</span>
          <div className="flex gap-1">
            {HORIZONTES.map((n) => (
              <button
                key={n}
                type="button"
                onClick={() => setMeses(n)}
                className={cn(
                  "rounded-md border px-3 py-1.5 text-sm font-medium transition-colors",
                  meses === n
                    ? "border-accent bg-accent text-white"
                    : "border-border bg-surface text-content-muted hover:text-content",
                )}
              >
                {n} meses
              </button>
            ))}
          </div>
          {d && (
            <span className="text-xs text-content-muted">
              patrón de {etiquetaPeriodo(d.desde_base)} a {etiquetaPeriodo(d.hasta_base)}
            </span>
          )}
        </div>

        {q.isPending ? (
          <LoadingState label="Estimando el flujo" />
        ) : q.isError ? (
          <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
        ) : !d ? null : (
          <>
            {/* El aviso va ANTES de los números: dice de qué está hecha la estimación, y leerlo
                después es leerlo cuando ya se sacó la conclusión. */}
            {d.aviso && (
              <div className="rounded-lg border border-border bg-surface-muted/40 px-4 py-3">
                <p className="text-sm text-content-muted">{d.aviso}</p>
              </div>
            )}

            {d.meses.length === 0 ? (
              <EmptyState message="Todavía no hay suficiente historia clasificada para estimar el flujo de esta empresa." />
            ) : (
              <div className="flex flex-col gap-1.5">
                {d.meses.map((m) => {
                  const neto = toNumber(m.neto);
                  return (
                    <div key={m.periodo} className="flex flex-wrap items-center gap-3">
                      <span className="w-32 shrink-0 text-sm text-content">
                        {etiquetaPeriodo(m.periodo)}
                      </span>
                      <span className="w-32 shrink-0 text-right tabular-nums text-sm text-positivo">
                        +{formatMonto(m.ingresos)}
                      </span>
                      <span className="w-32 shrink-0 text-right tabular-nums text-sm text-negativo">
                        −{formatMonto(m.gastos)}
                      </span>
                      <span
                        className={cn(
                          "w-36 shrink-0 text-right font-medium tabular-nums",
                          neto >= 0 ? "text-positivo" : "text-negativo",
                        )}
                      >
                        {neto >= 0 ? "+" : "−"}
                        {formatMonto(String(Math.abs(neto)))}
                      </span>
                      {m.indice_estacional && (
                        <span className="text-xs text-content-muted">
                          corregido por el mes ({m.indice_estacional})
                        </span>
                      )}
                    </div>
                  );
                })}
                <div className="mt-1 flex flex-wrap gap-6 border-t border-border pt-3 text-xs text-content-muted">
                  <span>{d.partidas_proyectadas} partida(s) con ritmo propio</span>
                  {d.partidas_esporadicas > 0 && (
                    <span>
                      {d.partidas_esporadicas} esporádica(s), como bloque de{" "}
                      {formatMonto(d.aporte_esporadicas)} al mes
                    </span>
                  )}
                  <span>
                    {d.con_estacionalidad
                      ? "corregido por estacionalidad"
                      : "sin corrección estacional"}
                  </span>
                </div>
              </div>
            )}

            {/* La cartera va SEPARADA de la proyección y se ve aunque no se pueda proyectar.
                Mezclarla haría que el mes que viene pareciera catastrófico por una deuda que se
                arrastra desde hace años; esconderla, que una empresa sin historia salga en
                blanco debiendo casi mil millones. */}
            {(toNumber(d.cxp_vencido) > 0 || toNumber(d.cxp_por_vencer) > 0) && (
              <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
                <p className="text-sm font-medium text-content">
                  Aparte de esto, ya se debe a proveedores
                </p>
                <p className="mt-1 flex flex-wrap gap-6 text-sm text-content-muted">
                  <span>
                    Vencido:{" "}
                    <b className="tabular-nums text-negativo">{formatMoneda(d.cxp_vencido)}</b>
                  </span>
                  <span>
                    Por vencer:{" "}
                    <b className="tabular-nums text-content">{formatMoneda(d.cxp_por_vencer)}</b>
                  </span>
                </p>
                <p className="mt-1 text-xs text-content-muted">
                  No entra en la proyección de arriba: es lo que ya se arrastra, no un gasto del
                  mes que viene.
                </p>
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
