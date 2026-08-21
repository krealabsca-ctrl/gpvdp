/**
 * CxC — Asociaciones (/cxc/asociaciones). El canal dominante de cobro.
 *
 * El descuento por asociación solidarista es el 100 % de los pagos de la muestra real: la
 * asociación deduce de la planilla del trabajador, manda UN depósito y un detalle con
 * cientos de contratos.
 *
 * Lo que esta pantalla contesta, y ninguna otra puede:
 *   ¿cuánto DEBERÍA traer cada asociación este mes, cuánto trajo, y quién no mandó nada?
 *
 * La última pregunta es la importante: si ASEPAN no envía planilla, sus clientes caen en
 * mora **sin haber hecho nada**. Llamarlos como morosos sería un error de gestión y de
 * trato — el problema es con un tercero y se gestiona distinto.
 */

import { useState } from "react";
import {
  Badge,
  Button,
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
  useToast,
} from "@/components/ui";
import type { BadgeTone } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useAbrirPlanilla,
  useCandidatosDeposito,
  useDesvincularDeposito,
  usePanoramaAsociaciones,
  usePlanilla,
  useVincularDeposito,
} from "@/features/cxc/hooks";

/** Los cinco estados que puede tener una planilla, derivados de los tres montos. */
const ETIQUETA_ESTADO: Record<string, string> = {
  SIN_CARGOS: "Sin cargos",
  NO_ENVIO: "No ha enviado",
  SIN_DEPOSITO: "Falta el depósito",
  CONCILIADA: "Conciliada",
  CON_DIFERENCIA: "Con diferencia",
};
const TONO_ESTADO: Record<string, BadgeTone> = {
  SIN_CARGOS: "neutral",
  NO_ENVIO: "negativo",
  SIN_DEPOSITO: "pendiente",
  CONCILIADA: "positivo",
  CON_DIFERENCIA: "negativo",
};

export function AsociacionesPage() {
  const { periodo } = usePeriodoActivo();
  const q = usePanoramaAsociaciones(periodo);
  const p = q.data;
  const tiene = useTienePermiso();
  const puedeConciliar = tiene("cxc.aplicar");
  const puedeAbrir = tiene("cxc.cobros");
  const [abierta, setAbierta] = useState("");

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Asociaciones solidaristas"
        description="Lo que cada asociación debería traer este mes contra lo que trajo. Es el canal por donde entra la mayoría del recaudo."
        actions={<Badge tone="accent">período {periodo}</Badge>}
      />

      {q.isPending ? (
        <LoadingState label="Cruzando cargos contra cobros" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : !p || p.filas.length === 0 ? (
        <EmptyState message="Todavía no hay asociaciones. Se crean solas al importar la cartera." />
      ) : (
        <>
          <div className="flex flex-wrap items-start gap-x-8 gap-y-3 rounded-xl border border-border bg-surface-raised px-4 py-3 shadow-card">
            <Cifra etiqueta="Asociaciones" valor={p.asociaciones.toLocaleString("es-CR")} nota="activas" />
            <Cifra
              etiqueta="Con planilla"
              valor={`${p.con_planilla} de ${p.asociaciones}`}
              tono={p.con_planilla === p.asociaciones ? "positivo" : undefined}
              nota="ya mandaron algo"
            />
            <Cifra etiqueta="Esperado" valor={formatMoneda(p.esperado)} nota="cargos que vencen" />
            <Cifra etiqueta="Cobrado" valor={formatMoneda(p.cobrado)} tono="positivo" nota="entró de este canal" />
            <Cifra
              etiqueta="Depositado"
              valor={formatMoneda(p.depositado)}
              tono={toNumber(p.depositado) > 0 ? "positivo" : undefined}
              nota="entró al banco de verdad"
            />
            <Cifra
              etiqueta="En riesgo"
              valor={formatMoneda(p.en_riesgo)}
              tono={toNumber(p.en_riesgo) > 0 ? "negativo" : undefined}
              nota={`${p.sin_planilla} sin planilla · ${p.contratos_en_riesgo} contratos`}
            />
            <Cifra
              etiqueta="Conciliación"
              valor={`${p.conciliadas} de ${p.asociaciones}`}
              tono={p.con_diferencia > 0 ? "negativo" : p.conciliadas > 0 ? "positivo" : undefined}
              nota={`${p.sin_deposito} sin depósito · ${p.con_diferencia} con diferencia`}
            />
          </div>

          {p.sin_planilla > 0 && (
            <div className="rounded-xl border border-negativo/40 bg-negativo/5 px-4 py-3 text-sm">
              <b>
                {p.sin_planilla} asociación(es) no han enviado planilla y arrastran{" "}
                {p.contratos_en_riesgo.toLocaleString("es-CR")} contratos por{" "}
                {formatMoneda(p.en_riesgo)}.
              </b>
              <span className="mt-1 block text-xs text-content-muted">
                Esos clientes van a aparecer en mora sin haber hecho nada. No corresponde llamarlos como
                morosos: hay que gestionar con la asociación.
              </span>
            </div>
          )}

          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Asociación</TH>
                  <TH className="text-right">Contratos</TH>
                  <TH className="text-right">Cargos del mes</TH>
                  <TH className="text-right">Esperado</TH>
                  <TH className="text-right">Registrado</TH>
                  <TH className="text-right">Depositado</TH>
                  <TH className="text-right">Diferencia</TH>
                  <TH>Entró</TH>
                  <TH>Estado</TH>
                  <TH />
                </TR>
              </THead>
              <TBody>
                {p.filas.map((f) => {
                  const dif = toNumber(f.diferencia);
                  return (
                    <TR key={f.asociacion_id}>
                      <TD>
                        <span className="font-medium">{f.asociacion}</span>
                        {f.patrono && <span className="block text-[11px] text-content-muted">{f.patrono}</span>}
                      </TD>
                      <TD className="text-right tabular-nums">{f.contratos.toLocaleString("es-CR")}</TD>
                      <TD className="text-right tabular-nums">{f.cargos_del_periodo.toLocaleString("es-CR")}</TD>
                      <TD className="text-right tabular-nums">{formatMoneda(f.esperado)}</TD>
                      <TD className="text-right tabular-nums text-positivo">
                        {toNumber(f.cobrado) > 0 ? formatMoneda(f.cobrado) : "—"}
                      </TD>
                      {/* Depositado: lo que entró al banco de verdad. Es el único de los tres
                          que no depende de lo que diga la asociación. */}
                      <TD className="text-right tabular-nums">
                        {toNumber(f.depositado) > 0 ? (
                          <>
                            <span className="font-medium">{formatMoneda(f.depositado)}</span>
                            {f.depositos > 1 && (
                              <span className="block text-[10.5px] text-content-muted">
                                {f.depositos} depósitos
                              </span>
                            )}
                          </>
                        ) : (
                          <span className="text-content-muted">—</span>
                        )}
                      </TD>
                      <TD
                        className={cn(
                          "text-right font-medium tabular-nums",
                          dif < 0 ? "text-negativo" : dif > 0 ? "text-pendiente" : "text-content-muted",
                        )}
                      >
                        {dif === 0 ? "—" : formatMoneda(f.diferencia)}
                      </TD>
                      <TD className="text-xs tabular-nums">
                        {f.fechas_bancarias.length === 0
                          ? "—"
                          : f.fechas_bancarias.map((d) => formatFecha(d)).join(" y ")}
                        {/* Una planilla puede llegar en varias transferencias: el dato real
                            trae «08/07 y 11/07» en un mismo registro. */}
                        {f.fechas_bancarias.length > 1 && (
                          <span className="block text-[10.5px] text-content-muted">
                            llegó en {f.fechas_bancarias.length} transferencias
                          </span>
                        )}
                      </TD>
                      {/* El estado sale de los TRES montos: lo calcula el servidor con la
                          misma función que usa la ficha, para que no digan cosas distintas. */}
                      <TD>
                        <Badge tone={TONO_ESTADO[f.estado] ?? "neutral"}>
                          {ETIQUETA_ESTADO[f.estado] ?? f.estado}
                        </Badge>
                        {f.referencia && (
                          <span className="block max-w-[10rem] truncate text-[10.5px] text-content-muted">
                            {f.referencia}
                          </span>
                        )}
                      </TD>
                      <TD className="text-right">
                        <Button
                          size="sm"
                          variant={abierta === f.asociacion_id ? "primary" : "secondary"}
                          onClick={() => setAbierta((a) => (a === f.asociacion_id ? "" : f.asociacion_id))}
                        >
                          {abierta === f.asociacion_id ? "Cerrar" : "Conciliar"}
                        </Button>
                      </TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          </TableContainer>

          {abierta !== "" && (
            <PanelConciliacion
              asociacionId={abierta}
              periodo={periodo}
              puedeConciliar={puedeConciliar}
              puedeAbrir={puedeAbrir}
            />
          )}

          <p className="text-xs text-content-muted">
            Los tres montos se calculan en vivo. <b>Esperado</b> = los cargos que vencen en {periodo} para
            los contratos de esa asociación. <b>Registrado</b> = lo que se importó del detalle que ella
            manda. <b>Depositado</b> = los movimientos de Bancos que se vincularon a la planilla: es el
            único que no depende de lo que ella diga. Cuando registrado y depositado no coinciden, la
            planilla queda <b>con diferencia</b> — es un hallazgo para revisar, no un error del sistema.
          </p>
        </>
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
    <div className="min-w-[9rem]">
      <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">{etiqueta}</p>
      <p
        className={cn(
          "text-lg font-semibold tabular-nums",
          tono === "positivo" ? "text-positivo" : tono === "negativo" ? "text-negativo" : "text-content",
        )}
      >
        {valor}
      </p>
      {nota && <p className="text-[10.5px] text-content-muted">{nota}</p>}
    </div>
  );
}

/**
 * El panel donde se concilia: los tres montos, los depósitos ya vinculados y los candidatos
 * de Bancos.
 *
 * Por qué el operador elige y el sistema no empareja solo: con los datos reales de la
 * empresa, la descripción del banco casi nunca dice de qué asociación es el depósito («TEF
 * DE:ASOCIACION SOLIDARISTA» aparece 31 veces). El sistema ordena los candidatos por si el
 * monto calza y si el texto la nombra, y muestra esas señales — pero la decisión es humana,
 * porque un falso positivo daría por conciliada a la asociación equivocada.
 */
function PanelConciliacion({
  asociacionId,
  periodo,
  puedeConciliar,
  puedeAbrir,
}: {
  asociacionId: string;
  periodo: string;
  puedeConciliar: boolean;
  puedeAbrir: boolean;
}) {
  const toast = useToast();
  const fichaQ = usePlanilla(asociacionId, periodo);
  const abrir = useAbrirPlanilla();
  const vincular = useVincularDeposito();
  const desvincular = useDesvincularDeposito();
  const [referencia, setReferencia] = useState("");
  const ficha = fichaQ.data;
  const candidatosQ = useCandidatosDeposito(ficha?.id ?? "");

  if (fichaQ.isPending) return <LoadingState label="Cargando la conciliación" />;
  if (fichaQ.isError) return <ErrorState message={mensajeError(fichaQ.error)} onRetry={() => fichaQ.refetch()} />;
  if (!ficha) return null;

  const falta = toNumber(ficha.registrado) - toNumber(ficha.depositado);

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 py-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold text-content">
              {ficha.asociacion} · {periodo}
            </h3>
            <p className="text-xs text-content-muted">
              El monto del depósito no se captura: sale del movimiento que ya está en Bancos. Lo único
              que hace falta es decir cuál es.
            </p>
          </div>
          <Badge tone={TONO_ESTADO[ficha.estado] ?? "neutral"}>
            {ETIQUETA_ESTADO[ficha.estado] ?? ficha.estado}
          </Badge>
        </div>

        <div className="flex flex-wrap items-start gap-x-8 gap-y-3">
          <Cifra etiqueta="Esperado" valor={formatMoneda(ficha.esperado)} nota="cargos que vencen" />
          <Cifra etiqueta="Registrado" valor={formatMoneda(ficha.registrado)} nota="detalle que mandó" />
          <Cifra
            etiqueta="Depositado"
            valor={formatMoneda(ficha.depositado)}
            tono={toNumber(ficha.depositado) > 0 ? "positivo" : undefined}
            nota="entró al banco"
          />
          <Cifra
            etiqueta="Falta por conciliar"
            valor={formatMoneda(String(falta))}
            tono={falta === 0 ? "positivo" : "negativo"}
            nota={falta === 0 ? "cuadra" : falta > 0 ? "no ha entrado" : "entró de más"}
          />
        </div>

        {/* Sin planilla abierta no hay dónde colgar el depósito. */}
        {ficha.id === "" ? (
          puedeAbrir ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                abrir.mutate(
                  { asociacionId, periodo, referencia: referencia.trim() },
                  {
                    onSuccess: () => toast.success("Planilla registrada. Ahora vinculá el depósito."),
                    onError: (err) => toast.error(mensajeError(err)),
                  },
                );
              }}
              className="flex flex-wrap items-end gap-3"
            >
              <Input
                label="Referencia del comprobante"
                value={referencia}
                onChange={(e) => setReferencia(e.target.value)}
                placeholder="El número que viene en el correo"
                className="min-w-64"
              />
              <Button type="submit" loading={abrir.isPending}>
                Registrar que llegó la planilla
              </Button>
            </form>
          ) : (
            <p className="text-xs text-content-muted">
              La planilla de este período no está registrada. Hace falta el permiso{" "}
              <span className="font-mono">cxc.cobros</span> para registrarla.
            </p>
          )
        ) : (
          <>
            {ficha.referencia && (
              <p className="text-xs text-content-muted">
                Comprobante: <span className="font-medium text-content">{ficha.referencia}</span>
              </p>
            )}

            {ficha.movimientos.length > 0 && (
              <div className="flex flex-col gap-2">
                <h4 className="text-xs font-semibold uppercase tracking-wide text-content-muted">
                  Depósitos vinculados
                </h4>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Fecha</TH>
                        <TH>Descripción del banco</TH>
                        <TH>Cuenta</TH>
                        <TH className="text-right">Monto</TH>
                        {puedeConciliar && <TH />}
                      </TR>
                    </THead>
                    <TBody>
                      {ficha.movimientos.map((m) => (
                        <TR key={m.id}>
                          <TD className="whitespace-nowrap text-xs">{formatFecha(m.fecha)}</TD>
                          <TD className="max-w-[26rem] truncate text-xs">{m.descripcion || "—"}</TD>
                          <TD className="text-xs text-content-muted">
                            {m.banco} · {m.cuenta}
                          </TD>
                          <TD className="text-right font-medium tabular-nums">{formatMoneda(m.monto)}</TD>
                          {puedeConciliar && (
                            <TD className="text-right">
                              <Button
                                size="sm"
                                variant="ghost"
                                loading={desvincular.isPending}
                                onClick={() =>
                                  desvincular.mutate(
                                    { planillaId: ficha.id, movimientoId: m.id },
                                    {
                                      onSuccess: () =>
                                        toast.success("Depósito desvinculado. El movimiento sigue en Bancos."),
                                      onError: (err) => toast.error(mensajeError(err)),
                                    },
                                  )
                                }
                              >
                                Quitar
                              </Button>
                            </TD>
                          )}
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              </div>
            )}

            {puedeConciliar ? (
              <div className="flex flex-col gap-2">
                <h4 className="text-xs font-semibold uppercase tracking-wide text-content-muted">
                  Créditos de Bancos que podrían ser este depósito
                </h4>
                {candidatosQ.isPending ? (
                  <LoadingState label="Buscando en Bancos" />
                ) : candidatosQ.isError ? (
                  <ErrorState message={mensajeError(candidatosQ.error)} onRetry={() => candidatosQ.refetch()} />
                ) : (candidatosQ.data ?? []).length === 0 ? (
                  <EmptyState message="No hay créditos sin vincular en este período." />
                ) : (
                  <TableContainer>
                    <Table>
                      <THead>
                        <TR>
                          <TH>Fecha</TH>
                          <TH>Descripción del banco</TH>
                          <TH>Clasificación</TH>
                          <TH className="text-right">Monto</TH>
                          <TH>Señales</TH>
                          <TH />
                        </TR>
                      </THead>
                      <TBody>
                        {(candidatosQ.data ?? []).slice(0, 20).map((c) => (
                          <TR key={c.id} className={cn(c.calza_monto && "bg-positivo/5")}>
                            <TD className="whitespace-nowrap text-xs">{formatFecha(c.fecha)}</TD>
                            <TD className="max-w-[24rem] truncate text-xs">{c.descripcion || "—"}</TD>
                            <TD className="max-w-[10rem] truncate text-xs text-content-muted">
                              {c.clasificacion || "sin clasificar"}
                            </TD>
                            <TD className="text-right tabular-nums">{formatMoneda(c.monto)}</TD>
                            <TD className="whitespace-nowrap">
                              {c.calza_monto && <Badge tone="positivo">calza exacto</Badge>}
                              {c.nombra_la_asociacion && <Badge tone="accent">la nombra</Badge>}
                              {!c.calza_monto && !c.nombra_la_asociacion && (
                                <span className="text-[11px] text-content-muted">
                                  {toNumber(c.diferencia) > 0 ? "+" : ""}
                                  {formatMoneda(c.diferencia)}
                                </span>
                              )}
                            </TD>
                            <TD className="text-right">
                              <Button
                                size="sm"
                                variant={c.calza_monto ? "primary" : "secondary"}
                                loading={vincular.isPending}
                                onClick={() =>
                                  vincular.mutate(
                                    { planillaId: ficha.id, movimientoId: c.id },
                                    {
                                      onSuccess: (res) =>
                                        toast.success(
                                          res.estado === "CONCILIADA"
                                            ? `Depósito vinculado: ${ficha.asociacion} quedó conciliada.`
                                            : `Depósito vinculado. Todavía falta ${formatMoneda(
                                                String(toNumber(res.registrado) - toNumber(res.depositado)),
                                              )}.`,
                                        ),
                                      onError: (err) => toast.error(mensajeError(err)),
                                    },
                                  )
                                }
                              >
                                Es este
                              </Button>
                            </TD>
                          </TR>
                        ))}
                      </TBody>
                    </Table>
                  </TableContainer>
                )}
                <p className="text-xs text-content-muted">
                  Los candidatos son créditos del período (con margen para los que entran a inicios del
                  mes siguiente) que <b>no están vinculados a ninguna otra planilla</b>. Un mismo depósito
                  no puede conciliar dos asociaciones.
                </p>
              </div>
            ) : (
              <p className="text-xs text-content-muted">
                Vincular un depósito requiere el permiso <span className="font-mono">cxc.aplicar</span>.
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
