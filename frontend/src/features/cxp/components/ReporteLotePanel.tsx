/**
 * «Lo que se va a pagar» de un corte, para revisarlo ANTES de subir el archivo al banco.
 *
 * Muestra una tarjeta por TRANSFERENCIA —que no es lo mismo que una factura cuando el corte se
 * agrupó por proveedor— con el detalle de qué cubre: gasto, factura, vencimiento y montos.
 *
 * Dos cosas se dicen fuerte y arriba, porque son las que cambian lo que hay que hacer:
 *   · cuántas facturas salen en cuántas transferencias (el ahorro del modo agrupado);
 *   · cuántas transferencias van sin IBAN, que el banco rechaza y hay que arreglar ANTES de subir.
 */
import { useState } from "react";

import { Badge, Button, LoadingState, ErrorState } from "@/components/ui";
import { formatFecha, formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { cxpApi } from "@/api/cxp";
import { useReporteLote } from "@/features/cxp/hooks";
import { useAuth } from "@/features/auth/AuthContext";
import { abrirReporteLoteImprimible } from "@/features/cxp/components/ReporteLoteImprimible";

export function ReporteLotePanel({
  loteId,
  numero,
  onCerrar,
}: {
  loteId: string;
  numero: string;
  onCerrar: () => void;
}) {
  const q = useReporteLote(loteId);
  const { empresaActiva } = useAuth();
  const [bajando, setBajando] = useState(false);
  // Un solo aviso para las dos acciones del encabezado (bajar la planilla, abrir el documento):
  // nunca pasan a la vez y cada una escribe su mensaje completo.
  const [aviso, setAviso] = useState("");

  function imprimir() {
    if (!q.data) return;
    setAviso("");
    // El navegador puede bloquear la ventana: decirlo, porque si no el botón parece no hacer nada.
    if (!abrirReporteLoteImprimible(q.data, empresaActiva?.nombre ?? "")) {
      setAviso("El navegador bloqueó la ventana; permitile abrir ventanas emergentes a este sitio.");
    }
  }

  async function descargar() {
    setBajando(true);
    setAviso("");
    try {
      const blob = await cxpApi.descargarReporteLote(loteId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `lote-${numero}-por-pagar.csv`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      // Decir «descargado» cuando no se descargó es peor que no descargarlo.
      setAviso("No se pudo descargar la planilla: " + mensajeError(err));
    } finally {
      setBajando(false);
    }
  }

  const r = q.data;
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4">
      <div className="mt-8 w-full max-w-4xl rounded-xl border border-border bg-surface-raised shadow-lifted">
        <div className="flex items-center justify-between gap-4 border-b border-border px-5 py-3">
          <div>
            <h2 className="text-base font-semibold text-content">Lote #{numero} — lo que se va a pagar</h2>
            {r && (
              <p className="text-sm text-content-muted">
                Corte del {formatFecha(r.fecha_corte)} ·{" "}
                <b className="text-content">{r.facturas} factura(s)</b> en{" "}
                <b className="text-content">{r.transferencias} transferencia(s)</b> ·{" "}
                {formatMoneda(r.total_crc, "CRC")}
              </p>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {/* Primero el documento: es el que se firma. La planilla queda para cruzar números. */}
            <Button size="sm" variant="primary" onClick={imprimir} disabled={!r}>
              Imprimir / PDF
            </Button>
            <Button size="sm" variant="secondary" onClick={() => void descargar()} loading={bajando} disabled={!r}>
              Planilla (.csv)
            </Button>
            <Button size="sm" variant="ghost" onClick={onCerrar}>
              Cerrar
            </Button>
          </div>
        </div>

        <div className="max-h-[70vh] overflow-y-auto px-5 py-4">
          {q.isPending ? (
            <LoadingState label="Armando el detalle del corte…" />
          ) : q.isError || !r ? (
            <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />
          ) : (
            <div className="flex flex-col gap-4">
              {aviso && <p className="text-sm text-negativo">{aviso}</p>}
              {r.sin_iban > 0 && (
                <p className="rounded-lg border border-negativo/40 bg-negativo/5 px-3 py-2 text-sm text-negativo">
                  ⚠ {r.sin_iban} transferencia(s) sin cuenta IBAN. El banco las rechaza: cargá el IBAN
                  de esos proveedores antes de subir el archivo.
                </p>
              )}
              {r.pagos.map((p) => (
                <div key={p.huella || p.referencia} className="rounded-lg border border-border">
                  <div className="flex flex-wrap items-baseline justify-between gap-2 border-b border-border bg-surface-muted/60 px-3 py-2">
                    <div>
                      <span className="font-medium text-content">{p.proveedor}</span>{" "}
                      <span className="font-mono text-xs text-content-muted">{p.referencia}</span>
                      {p.lineas.length > 1 && (
                        <Badge tone="accent" className="ml-2">
                          {p.lineas.length} facturas en un pago
                        </Badge>
                      )}
                      {!p.iban && (
                        <Badge tone="negativo" className="ml-2">
                          sin IBAN
                        </Badge>
                      )}
                    </div>
                    <span className="font-semibold tabular-nums text-content">
                      {formatMoneda(p.total_crc, "CRC")}
                    </span>
                  </div>
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="text-left text-[11px] uppercase tracking-wider text-content-muted">
                        <th className="px-3 py-1.5 font-semibold">Factura</th>
                        <th className="px-3 py-1.5 font-semibold">Gasto</th>
                        <th className="px-3 py-1.5 font-semibold">Vence</th>
                        <th className="px-3 py-1.5 text-right font-semibold">Total</th>
                        <th className="px-3 py-1.5 text-right font-semibold">Neto</th>
                      </tr>
                    </thead>
                    <tbody>
                      {p.lineas.map((l) => (
                        <tr key={l.consecutivo} className="border-t border-border/60">
                          <td className="px-3 py-1.5 font-mono text-[11px]">{l.consecutivo || "—"}</td>
                          <td className="px-3 py-1.5">{l.gasto || "—"}</td>
                          <td className="px-3 py-1.5 tabular-nums">
                            {l.fecha_vencimiento ? formatFecha(l.fecha_vencimiento) : "—"}
                          </td>
                          <td className="px-3 py-1.5 text-right tabular-nums">
                            {formatMoneda(l.total_crc, "CRC")}
                          </td>
                          <td className="px-3 py-1.5 text-right tabular-nums">
                            {formatMoneda(l.neto_crc, "CRC")}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ))}
              {/* El código interno de cada pago, junto, para poder buscarlo en el estado de cuenta.
                  Va al final y en pequeño: sirve al conciliar, no al revisar. */}
              <p className="text-xs text-content-muted">
                Códigos internos de este corte:{" "}
                <span className="font-mono">{r.pagos.map((p) => p.huella).filter(Boolean).join(" · ") || "—"}</span>
                . En el banco pueden aparecer sin el guion.
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
