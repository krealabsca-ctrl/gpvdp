/**
 * Contabilidad → Generar asientos (/conta/generar). FASE 3B.
 *
 * ── ESTA ES LA PANTALLA DONDE EL SISTEMA EMPIEZA A CONTABILIZAR ─────────────────────────────
 *
 * Y lo hace POR CAJA: el asiento nace cuando la plata se movió en el banco, no cuando llegó la
 * factura. Es la decisión del Director Financiero del 28-oct-2026 —«no es cuando debo sino cuánto
 * pagué»— y es cómo opera la empresa hoy.
 *
 * Por caja NO cumple NIIF, y la pantalla lo dice en vez de esconderlo. El destino es devengo.
 *
 * ── POR QUÉ PRIMERO SE MIRA Y DESPUÉS SE CONFIRMA ───────────────────────────────────────────
 *
 * Generar toca miles de movimientos de una vez. La previsualización no es un trámite: es la
 * única oportunidad de ver qué queda fuera ANTES de escribir, y sobre todo POR QUÉ —porque cada
 * motivo se arregla en otra pantalla, y saber cuál conviene atacar primero depende de cuántos
 * movimientos bloquea cada uno.
 */

import { useState } from "react";
import { Link } from "react-router-dom";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  ErrorState,
  LoadingState,
  PageHeader,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { etiquetaPeriodo, formatMoneda, formatMonto } from "@/lib/format";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useTienePermiso } from "@/features/auth/permisos";
import { PeriodoSelector } from "@/features/bancos/components/PeriodoSelector";
import {
  useGenerarAsientos,
  usePrevisualizarGeneracion,
  useRevertirGeneracion,
} from "@/features/conta/hooks";

/** A dónde lleva cada motivo. La pantalla que lo arregla, no una explicación. */
const DESTINO: Record<string, string> = {
  SIN_CLASIFICAR: "/clasificar",
  SIN_MAPEO: "/conta/mapeo",
  SIN_CUENTA_BANCO: "/conta/bancos",
  SIN_TIPO_CAMBIO: "/tipo-cambio",
};

export function GenerarAsientosPage() {
  const toast = useToast();
  const { periodo: periodoActivo } = usePeriodoActivo();
  const [periodo, setPeriodo] = useState(periodoActivo);
  const puedeGenerar = useTienePermiso()("conta.generar");

  const q = usePrevisualizarGeneracion(periodo);
  const generar = useGenerarAsientos();
  const revertir = useRevertirGeneracion();
  const d = q.data;

  function confirmarGenerar() {
    generar.mutate(periodo, {
      onSuccess: (r) => {
        if (r.creados === 0) {
          toast.success(
            r.ya_existian > 0
              ? `No había nada nuevo: los ${r.ya_existian} movimientos ya tenían asiento.`
              : "No se generó nada.",
          );
          return;
        }
        toast.success(
          `${r.creados} asiento(s) generados, del ${r.desde_numero} al ${r.hasta_numero}.`,
        );
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function confirmarRevertir() {
    const motivo = window.prompt(
      `Revertir los asientos de ${etiquetaPeriodo(periodo)} generados desde el banco.\n\n` +
        "No se borran: quedan en el libro, sin efecto y con este motivo. Después se puede " +
        "volver a generar.\n\n¿Por qué se revierte? (mínimo 15 caracteres)",
    );
    if (motivo === null) return;
    revertir.mutate(
      { periodo, motivo },
      {
        onSuccess: (r) => toast.success(`${r.anulados} asiento(s) anulados.`),
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Generar asientos"
        description="Convertir los movimientos del banco de un período en asientos contables."
        actions={<PeriodoSelector label="Período" value={periodo} onChange={setPeriodo} id="gen-periodo" />}
      />

      {/* Lo primero que hay que decir, y no al pie: por caja no cumple NIIF. */}
      <div className="rounded-lg border border-accent/40 bg-accent/5 px-4 py-3 text-sm text-content">
        <b>Esto contabiliza por caja.</b> El asiento nace cuando la plata se movió en el banco, no
        cuando llegó la factura — que es como opera la empresa hoy. Sirve para ver el movimiento
        real, pero <b>no cumple NIIF</b> y no es comparable con un estado financiero por devengo.
        El devengo es el paso siguiente y no obliga a rehacer lo que se genere acá.
      </div>

      {q.isPending ? (
        <LoadingState label="Revisando el período" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : !d ? null : (
        <>
          <Card>
            <CardHeader>
              <CardTitle>{etiquetaPeriodo(d.periodo)}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div className="flex flex-wrap items-end gap-8">
                <div>
                  <p className="text-3xl font-semibold tabular-nums text-content">
                    {d.contabilizables.toLocaleString("es-CR")}
                  </p>
                  <p className="text-xs text-content-muted">
                    asiento(s) a generar, de {d.total.toLocaleString("es-CR")} movimientos
                  </p>
                </div>
                <div>
                  <p className="text-xl font-semibold tabular-nums text-negativo">
                    {formatMonto(d.monto_debitos)}
                  </p>
                  <p className="text-xs text-content-muted">salidas del banco</p>
                </div>
                <div>
                  <p className="text-xl font-semibold tabular-nums text-positivo">
                    {formatMonto(d.monto_creditos)}
                  </p>
                  <p className="text-xs text-content-muted">entradas al banco</p>
                </div>
                {d.ya_generados > 0 && (
                  <div>
                    <p className="text-xl font-semibold tabular-nums text-content-muted">
                      {d.ya_generados.toLocaleString("es-CR")}
                    </p>
                    <p className="text-xs text-content-muted">ya tienen asiento</p>
                  </div>
                )}
              </div>

              {d.aviso && <p className="text-sm text-content-muted">{d.aviso}</p>}

              {puedeGenerar && (
                <div className="flex flex-wrap gap-2">
                  {/* El botón promete los NUEVOS, no los contabilizables: en un período ya
                      generado los dos números se separan, y «Generar 6.340» para después crear
                      cero es una mentira aunque ninguna cifra esté mal. */}
                  <Button onClick={confirmarGenerar} disabled={d.nuevos === 0 || generar.isPending}>
                    {generar.isPending
                      ? "Generando…"
                      : d.nuevos === 0
                        ? "Ya está generado"
                        : `Generar ${d.nuevos.toLocaleString("es-CR")} asiento(s)`}
                  </Button>
                  {d.ya_generados > 0 && (
                    <Button variant="secondary" onClick={confirmarRevertir} disabled={revertir.isPending}>
                      Revertir lo generado
                    </Button>
                  )}
                  <Link
                    to="/conta/libros"
                    className="inline-flex h-10 items-center rounded-lg px-3 text-sm font-medium text-content-muted transition-colors hover:text-content"
                  >
                    Ver el libro diario →
                  </Link>
                </div>
              )}
            </CardContent>
          </Card>

          {/* ── Lo que queda fuera ──────────────────────────────────────────────────────
              Ordenado por cuántos movimientos bloquea cada causa, que es el orden en que
              conviene atacarlas. Y cada fila lleva el enlace a la pantalla que lo arregla:
              sin eso, «1.560 sin cuenta bancaria» informa pero no se puede accionar. */}
          {(d.por_motivo ?? []).length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Lo que todavía no se puede contabilizar</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                <p className="text-sm text-content-muted">
                  No se pierde nada: cuando se complete, se genera el período otra vez y entra lo
                  que falta. Lo de arriba no se duplica.
                </p>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Qué falta</TH>
                        <TH className="text-right">Movimientos</TH>
                        <TH className="text-right">Monto</TH>
                        <TH>Dónde se arregla</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {(d.por_motivo ?? []).map((m) => (
                        <TR key={m.motivo}>
                          <TD className="max-w-md whitespace-normal">
                            {m.etiqueta}
                            {m.motivo === "NO_CONTABILIZABLE" && (
                              <Badge tone="neutral" className="ml-2">
                                es una decisión, no un faltante
                              </Badge>
                            )}
                          </TD>
                          <TD className="text-right tabular-nums">
                            {m.movimientos.toLocaleString("es-CR")}
                          </TD>
                          <TD className="text-right tabular-nums">{formatMoneda(m.monto_crc)}</TD>
                          <TD>
                            {DESTINO[m.motivo] ? (
                              <Link
                                to={DESTINO[m.motivo]!}
                                className="text-sm font-medium text-accent underline"
                              >
                                {m.donde_se_arregla} →
                              </Link>
                            ) : (
                              <span className="text-sm text-content-muted">{m.donde_se_arregla}</span>
                            )}
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              </CardContent>
            </Card>
          )}

          {d.total === 0 && (
            <EmptyState message="No hay movimientos vivos en este período." />
          )}
        </>
      )}
    </div>
  );
}
