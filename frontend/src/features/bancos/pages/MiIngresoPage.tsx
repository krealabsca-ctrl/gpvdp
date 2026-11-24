/**
 * «Mi ingreso»: cómo va el cobro de mis partidas, sin entrar a Bancos (mig 0125).
 *
 * EL PEDIDO (Director Financiero, 16-nov-2026): «en algunos casos aplica solo para gastos, pero en
 * otras ingresos también… el tema de los accesos no va tan ligado al perfil sino al usuario».
 *
 * ── PANTALLA APARTE DE «MI PRESUPUESTO», NO UNA PESTAÑA ──────────────────────────────────────
 *
 * El gasto se mide por ÁREA (`departamento_validador`) y el ingreso por PARTIDA
 * (`usuario_clasificacion_consulta`): dos ejes y dos listas de alcance distintas. En una sola
 * pantalla convivirían «tenés a cargo: Mercadeo» y «tus partidas: Datáfonos», que no se leen
 * igual. Y los permisos son dos justamente para poder dar uno sin el otro.
 *
 * ── POSITIVO ES BUENO, AL REVÉS DEL GASTO ────────────────────────────────────────────────────
 *
 * Pasarse de la meta es lo que se busca. Un 95 % acá es casi un logro, no una alerta, así que la
 * diferencia a favor va en verde y la que falta en rojo: exactamente al revés de «Mi presupuesto».
 *
 * ── SIN META NO HAY SEMÁFORO ─────────────────────────────────────────────────────────────────
 *
 * Medido el 16-nov-2026: CERO líneas de meta de ingreso cargadas en el sistema. Para todo el
 * mundo esta pantalla abre hoy sin nada contra qué comparar, y decirlo es la diferencia entre
 * «falta cargarlo» y «la pantalla no sirve».
 */
import { Fragment, useState } from "react";

import {
  Badge,
  Card,
  CardContent,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
} from "@/components/ui";
import type { FilaIngreso, PuntoPartida } from "@/api/bancos";
import { useMiIngreso, useMiSerieDeIngreso } from "@/features/bancos/hooks";
import { mensajeError } from "@/lib/apiError";
import { etiquetaPeriodo, formatMoneda, toNumber } from "@/lib/format";

/** El mes actual como `YYYY-MM`. */
function mesActual(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

const TONO_ESTADO: Record<string, "positivo" | "pendiente" | "negativo" | "neutral"> = {
  CUMPLIDO: "positivo",
  EN_CAMINO: "pendiente",
  CORTO: "negativo",
  SIN_META: "neutral",
};

const ETIQUETA_ESTADO: Record<string, string> = {
  CUMPLIDO: "cumplida",
  EN_CAMINO: "en camino",
  CORTO: "por debajo",
  SIN_META: "sin meta",
};

/**
 * El mes a mes de una partida, en barras.
 *
 * Contesta «¿cuándo entró?», que el total del rango no contesta: un trimestre donde el cobro se
 * desplomó en el último mes se ve idéntico a uno parejo si solo se mira la suma.
 */
function MesAMesDeLaPartida({ serie }: { serie: PuntoPartida[] }) {
  // Con un solo mes no hay nada que comparar.
  if (serie.length < 2) return null;
  const tope = Math.max(...serie.map((p) => toNumber(p.monto)));

  return (
    <div className="py-2">
      <p className="mb-2 text-sm text-content-muted">Ingreso mes a mes</p>
      <div className="flex items-end gap-2" style={{ height: 96 }}>
        {serie.map((p) => (
          <div key={p.periodo} className="flex min-w-0 flex-1 flex-col items-center gap-1">
            <div
              className="w-full rounded-t bg-positivo/70"
              style={{ height: `${tope > 0 ? Math.max(2, (toNumber(p.monto) / tope) * 72) : 2}px` }}
              title={`${etiquetaPeriodo(p.periodo)}: ${formatMoneda(p.monto, "CRC")} · ${p.movs} mov.`}
            />
            <span className="truncate text-[11px] text-content-muted">{p.periodo.slice(5)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function FilaPartida({
  f,
  abierta,
  onAlternar,
}: {
  f: FilaIngreso;
  abierta: boolean;
  onAlternar: () => void;
}) {
  const sinMeta = f.estado === "SIN_META";
  return (
    <TR>
      <TD>
        <button
          type="button"
          onClick={onAlternar}
          className="flex items-center gap-1.5 font-medium text-accent hover:underline"
        >
          <span aria-hidden>{abierta ? "▾" : "▸"}</span>
          {f.clasificacion}
        </button>
      </TD>
      <TD className="text-right tabular-nums">
        {sinMeta ? <span className="text-content-muted">—</span> : formatMoneda(f.meta, "CRC")}
      </TD>
      <TD className="text-right tabular-nums">{formatMoneda(f.real, "CRC")}</TD>
      <TD className="text-right tabular-nums">
        {/* Sin meta, «diferencia» no significa nada: no se muestra un número inventado. */}
        {sinMeta ? (
          <span className="text-content-muted">—</span>
        ) : (
          // POSITIVO ES BUENO: superar la meta va en verde, quedarse corto en rojo.
          <span className={Number(f.diferencia) < 0 ? "text-negativo" : "text-positivo"}>
            {Number(f.diferencia) > 0 ? "+" : ""}
            {formatMoneda(f.diferencia, "CRC")}
          </span>
        )}
      </TD>
      <TD className="text-right tabular-nums text-content-muted">{f.movs}</TD>
      <TD>
        <Badge tone={TONO_ESTADO[f.estado] ?? "neutral"}>
          {ETIQUETA_ESTADO[f.estado] ?? f.estado}
          {!sinMeta && f.cumplido_pct ? ` · ${f.cumplido_pct} %` : ""}
        </Badge>
      </TD>
    </TR>
  );
}

export function MiIngresoPage() {
  const [desde, setDesde] = useState(mesActual());
  const [hasta, setHasta] = useState(mesActual());
  // Una partida abierta a la vez, igual que en la de gasto.
  const [abierta, setAbierta] = useState("");
  const q = useMiIngreso(desde, hasta);
  // La serie trae TODAS sus partidas de una vez: quien tiene este permiso maneja dos o tres
  // canales de cobro, y una llamada por partida convertiría abrir la pantalla en varios viajes.
  // Se pide solo cuando ya hay una fila abierta.
  const serieQ = useMiSerieDeIngreso(desde, hasta, abierta !== "");
  const mi = q.data;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Mi ingreso"
        description="Cuánto entró por tus partidas este mes, contra la meta."
      />

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 pt-5">
          <Input label="Desde" type="month" value={desde} onChange={(e) => setDesde(e.target.value)} />
          <Input label="Hasta" type="month" value={hasta} onChange={(e) => setHasta(e.target.value)} />
        </CardContent>
      </Card>

      {q.isPending && <LoadingState label="Cargando tu ingreso" />}
      {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />}

      {mi && (
        <>
          {/* Las partidas a cargo van ARRIBA y siempre: «mi partida no aparece» y «mi partida no
              cobró» son dos cosas distintas, y sin esta línea no se pueden distinguir. */}
          <Card>
            <CardContent className="py-4 text-sm">
              <span className="text-content-muted">Tus partidas: </span>
              {mi.partidas.map((p, i) => (
                <span key={p.clasificacion_id}>
                  {i > 0 && ", "}
                  <b>{p.clasificacion}</b>
                </span>
              ))}
            </CardContent>
          </Card>

          {mi.aviso && (
            <p className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-4 py-3 text-sm">
              {mi.aviso}
            </p>
          )}

          <Card>
            <CardContent className="pt-5">
              {mi.lineas.length === 0 ? (
                <EmptyState message="Tus partidas no registran ingreso en este período." />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Partida</TH>
                        <TH className="text-right">Meta</TH>
                        <TH className="text-right">Ingresó</TH>
                        <TH className="text-right">Diferencia</TH>
                        <TH className="text-right">Movs.</TH>
                        <TH>Estado</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {mi.lineas.map((f) => (
                        <Fragment key={f.clasificacion_id}>
                          <FilaPartida
                            f={f}
                            abierta={abierta === f.clasificacion_id}
                            onAlternar={() =>
                              setAbierta(abierta === f.clasificacion_id ? "" : f.clasificacion_id)
                            }
                          />
                          {abierta === f.clasificacion_id && (
                            <TR>
                              <TD colSpan={6} className="bg-surface-muted/40">
                                {serieQ.isPending ? (
                                  <LoadingState label="Cargando el mes a mes" />
                                ) : (
                                  <MesAMesDeLaPartida
                                    serie={
                                      serieQ.data?.partidas.find(
                                        (p) => p.clasificacion_id === f.clasificacion_id,
                                      )?.serie ?? []
                                    }
                                  />
                                )}
                              </TD>
                            </TR>
                          )}
                        </Fragment>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              )}
              {mi.lineas.length > 1 && (
                <p className="mt-3 text-right text-sm">
                  <span className="text-content-muted">Total de tus partidas: </span>
                  <b className="tabular-nums">{formatMoneda(mi.real_total, "CRC")}</b>
                  {mi.meta_total !== "0.00" && (
                    <span className="text-content-muted">
                      {" "}
                      de {formatMoneda(mi.meta_total, "CRC")}
                    </span>
                  )}
                </p>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
