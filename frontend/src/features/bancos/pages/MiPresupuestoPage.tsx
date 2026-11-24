/**
 * «Mi presupuesto»: cómo va el gasto de mi área, sin entrar a Bancos (mig 0124).
 *
 * EL PEDIDO (Director Financiero, 16-nov-2026): «debo permitirle ver a los líderes, gerencias,
 * jefaturas cómo van sus gastos del mes, pero sin darle acceso al módulo de bancos… el tema de
 * los accesos no va tan ligado al perfil sino al usuario».
 *
 * ── ES UNA PANTALLA DE CONSULTA, COMO «MI PARTIDA» ───────────────────────────────────────────
 *
 * No hay aprobación ni flujo. Se mira, y si algo está mal se avisa por fuera. Y NO muestra
 * movimientos bancarios —cuenta, banco, proveedor, descripción— porque ese es justamente el
 * módulo del que la persona está afuera: acá se ve el agregado de su área contra su presupuesto.
 *
 * ── LO QUE HAY QUE EXPLICAR, PORQUE SI NO EL NÚMERO SE LEE MAL ───────────────────────────────
 *
 *   · SIN PRESUPUESTO NO HAY SEMÁFORO. Medido el 16-nov-2026: de 9 áreas con gasto, solo 2 tienen
 *     monto cargado. Para casi todo el mundo esta pantalla abre sin nada contra qué comparar, y
 *     decirlo es la diferencia entre «falta cargarlo» y «la pantalla no sirve».
 *   · UN MES A MEDIO CLASIFICAR CONSUME MENOS PRESUPUESTO DEL QUE CONSUMIÓ. El aviso viene del
 *     servidor y se muestra tal cual: sin él, un área «al 40 %» puede estar pasada.
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
import type { FilaControl, GastoPartidaDimension } from "@/api/bancos";
import { useMiPresupuesto, useMisPartidasDeArea } from "@/features/bancos/hooks";
import { mensajeError } from "@/lib/apiError";
import { etiquetaPeriodo, formatMoneda, toNumber } from "@/lib/format";

/** El mes actual como `YYYY-MM`. */
function mesActual(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

const TONO_ESTADO: Record<string, "positivo" | "pendiente" | "negativo" | "neutral"> = {
  EN_RANGO: "positivo",
  ALERTA: "pendiente",
  EXCEDIDO: "negativo",
  SIN_PRESUPUESTO: "neutral",
};

const ETIQUETA_ESTADO: Record<string, string> = {
  EN_RANGO: "en rango",
  ALERTA: "cerca del tope",
  EXCEDIDO: "se pasó",
  SIN_PRESUPUESTO: "sin presupuesto",
};

/**
 * El desglose de un área: en qué partidas se fue y en qué meses.
 *
 * Se pide SOLO al abrir la fila. Un jefe con cuatro áreas no necesita el detalle de las cuatro
 * para ver su total, y pedirlas todas al entrar haría lenta la pantalla que más se mira.
 */
function DesgloseDelArea({
  departamentoId,
  desde,
  hasta,
}: {
  departamentoId: string;
  desde: string;
  hasta: string;
}) {
  const q = useMisPartidasDeArea(desde, hasta, departamentoId);

  if (q.isPending) return <LoadingState label="Cargando el detalle" />;
  if (q.isError) return <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />;

  const filas = q.data ?? [];
  if (filas.length === 0) {
    return <EmptyState message="Esta área no registra gasto en el período." />;
  }

  return (
    <div className="flex flex-col gap-3 py-2">
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Partida</TH>
              <TH className="text-right">Gastado</TH>
              <TH className="text-right">Movs.</TH>
              {/* El mes que concentró el gasto. Contesta «¿en qué mes?», que es la segunda
                  pregunta y hasta ahora no tenía respuesta en ninguna de estas pantallas. */}
              <TH>Mes más alto</TH>
            </TR>
          </THead>
          <TBody>
            {filas.map((p) => (
              <TR key={p.clasificacion_id || p.clasificacion}>
                <TD>
                  <div className="font-medium">{p.clasificacion}</div>
                  <div className="text-xs text-content-muted">{p.concepto}</div>
                </TD>
                <TD className="text-right tabular-nums">{formatMoneda(p.gasto, "CRC")}</TD>
                <TD className="text-right tabular-nums text-content-muted">{p.movs}</TD>
                <TD className="text-sm">
                  {p.mes_pico ? (
                    etiquetaPeriodo(p.mes_pico)
                  ) : (
                    <span className="text-content-muted">—</span>
                  )}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>

      {/* El mes a mes del área entera, sumando sus partidas. */}
      <MesAMesDelArea filas={filas} />
    </div>
  );
}

/** Barras del gasto mes a mes del área: la forma del gasto, no solo su total. */
function MesAMesDelArea({ filas }: { filas: GastoPartidaDimension[] }) {
  // Las series vienen por partida; acá se suman por mes para ver el área completa.
  const porMes = new Map<string, number>();
  for (const f of filas) {
    for (const m of f.meses ?? []) {
      porMes.set(m.periodo, (porMes.get(m.periodo) ?? 0) + toNumber(m.gasto));
    }
  }
  const meses = [...porMes.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  // Con un solo mes la comparación no dice nada: no se dibuja.
  if (meses.length < 2) return null;
  const tope = Math.max(...meses.map(([, v]) => v));

  return (
    <div>
      <p className="mb-2 text-sm text-content-muted">Gasto mes a mes</p>
      <div className="flex items-end gap-2" style={{ height: 96 }}>
        {meses.map(([periodo, monto]) => (
          <div key={periodo} className="flex min-w-0 flex-1 flex-col items-center gap-1">
            <div
              className="w-full rounded-t bg-accent/70"
              style={{ height: `${tope > 0 ? Math.max(2, (monto / tope) * 72) : 2}px` }}
              title={`${etiquetaPeriodo(periodo)}: ${formatMoneda(String(monto), "CRC")}`}
            />
            <span className="truncate text-[11px] text-content-muted">{periodo.slice(5)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function FilaArea({
  f,
  abierta,
  onAlternar,
}: {
  f: FilaControl;
  abierta: boolean;
  onAlternar: () => void;
}) {
  const sinPresupuesto = f.estado === "SIN_PRESUPUESTO";
  return (
    <TR>
      <TD>
        <button
          type="button"
          onClick={onAlternar}
          className="flex items-center gap-1.5 font-medium text-accent hover:underline"
        >
          <span aria-hidden>{abierta ? "▾" : "▸"}</span>
          {f.departamento}
        </button>
      </TD>
      <TD className="text-right tabular-nums">
        {sinPresupuesto ? <span className="text-content-muted">—</span> : formatMoneda(f.presupuesto, "CRC")}
      </TD>
      <TD className="text-right tabular-nums">{formatMoneda(f.gasto, "CRC")}</TD>
      <TD className="text-right tabular-nums">
        {/* Sin presupuesto, «disponible» no significa nada: no se muestra un número inventado. */}
        {sinPresupuesto ? (
          <span className="text-content-muted">—</span>
        ) : (
          <span className={Number(f.disponible) < 0 ? "text-negativo" : ""}>
            {formatMoneda(f.disponible, "CRC")}
          </span>
        )}
      </TD>
      <TD>
        <Badge tone={TONO_ESTADO[f.estado] ?? "neutral"}>
          {ETIQUETA_ESTADO[f.estado] ?? f.estado}
          {!sinPresupuesto && f.consumido_pct ? ` · ${f.consumido_pct} %` : ""}
        </Badge>
      </TD>
    </TR>
  );
}

export function MiPresupuestoPage() {
  const [desde, setDesde] = useState(mesActual());
  const [hasta, setHasta] = useState(mesActual());
  // Un área abierta a la vez: el detalle es una tabla con su gráfico y dos abiertas obligan a
  // desplazarse para comparar, que es lo contrario de lo que se busca.
  const [abierta, setAbierta] = useState("");
  const q = useMiPresupuesto(desde, hasta);
  const mp = q.data;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Mi presupuesto"
        description="Cómo va el gasto de tu área este mes, contra lo presupuestado."
      />

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 pt-5">
          <Input label="Desde" type="month" value={desde} onChange={(e) => setDesde(e.target.value)} />
          <Input label="Hasta" type="month" value={hasta} onChange={(e) => setHasta(e.target.value)} />
        </CardContent>
      </Card>

      {q.isPending && <LoadingState label="Cargando tu presupuesto" />}
      {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />}

      {mp && (
        <>
          {/* Las áreas a cargo van ARRIBA y siempre: «mi área no aparece» y «mi área no gastó»
              son dos cosas distintas, y sin esta línea no se pueden distinguir. */}
          <Card>
            <CardContent className="py-4 text-sm">
              <span className="text-content-muted">Tenés a cargo: </span>
              {mp.areas.map((a, i) => (
                <span key={a.departamento_id}>
                  {i > 0 && ", "}
                  <b>{a.departamento}</b>
                  {a.rol === "SUPLENTE" && <span className="text-content-muted"> (suplente)</span>}
                </span>
              ))}
            </CardContent>
          </Card>

          {mp.aviso && (
            <p className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-4 py-3 text-sm">
              {mp.aviso}
            </p>
          )}

          {/* SIN PRESUPUESTO NO HAY SEMÁFORO, y hay que decir por qué. */}
          {mp.sin_presupuesto > 0 && (
            <p className="rounded-lg border border-border bg-surface-muted px-4 py-3 text-sm">
              {mp.sin_presupuesto === mp.areas.length
                ? "Todavía no hay presupuesto cargado para tu área, así que se muestra el gasto sin nada contra qué compararlo."
                : `${mp.sin_presupuesto} de tus áreas no tienen presupuesto cargado: ahí se muestra el gasto, pero no el semáforo.`}{" "}
              Lo carga quien administra el presupuesto en Bancos.
            </p>
          )}

          <Card>
            <CardContent className="pt-5">
              {mp.filas.length === 0 ? (
                <EmptyState message="Tu área no registra gasto en este período." />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Área</TH>
                        <TH className="text-right">Presupuesto</TH>
                        <TH className="text-right">Gastado</TH>
                        <TH className="text-right">Disponible</TH>
                        <TH>Estado</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {mp.filas.map((f) => (
                        <Fragment key={f.departamento_id}>
                          <FilaArea
                            f={f}
                            abierta={abierta === f.departamento_id}
                            onAlternar={() =>
                              setAbierta(abierta === f.departamento_id ? "" : f.departamento_id)
                            }
                          />
                          {abierta === f.departamento_id && (
                            <TR>
                              <TD colSpan={5} className="bg-surface-muted/40">
                                <DesgloseDelArea
                                  departamentoId={f.departamento_id}
                                  desde={desde}
                                  hasta={hasta}
                                />
                              </TD>
                            </TR>
                          )}
                        </Fragment>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              )}
              {mp.filas.length > 1 && (
                <p className="mt-3 text-right text-sm">
                  <span className="text-content-muted">Total de tus áreas: </span>
                  <b className="tabular-nums">{formatMoneda(mp.total_gasto, "CRC")}</b>
                  {mp.total_presupuesto !== "0.00" && (
                    <span className="text-content-muted">
                      {" "}
                      de {formatMoneda(mp.total_presupuesto, "CRC")}
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
