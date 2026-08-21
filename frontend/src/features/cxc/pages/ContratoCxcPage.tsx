/**
 * CxC — Ficha del contrato (/cxc/contratos/:numero).
 *
 * Es donde se ve el cambio de modelo: en vez de «cuota ₡5 600 · 108 días de mora», el
 * contrato muestra sus CARGOS, cada uno con su período, su vencimiento y su saldo. El
 * total de arriba es la suma de los de abajo, siempre.
 */

import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import {
  Badge,
  Button,
  Card,
  CardContent,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Select,
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
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useAnularArreglo,
  useAnularNota,
  useArreglos,
  useContrato,
  useEmitirNota,
  useEstadoSuspension,
  useGestiones,
  useNotasCredito,
  usePactarArreglo,
  useQuebrarArreglo,
  useReactivar,
  useSuspender,
} from "@/features/cxc/hooks";
import type { Arreglo, CargoCxc, NotaCredito } from "@/api/cxc";

/** El tono sale de los días de mora del propio cargo. */
function tonoCargo(dias: number, saldo: string): BadgeTone {
  if (toNumber(saldo) === 0) return "positivo";
  if (dias <= 0) return "neutral";
  if (dias <= 30) return "pendiente";
  return "negativo";
}

export function ContratoCxcPage() {
  const { numero = "" } = useParams();
  const [soloAbiertos, setSoloAbiertos] = useState(false);
  const q = useContrato(numero, soloAbiertos);

  if (q.isPending) return <LoadingState label="Cargando el contrato" />;
  if (q.isError) return <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />;
  if (!q.data) return <EmptyState message="No se encontró el contrato." />;

  const { contrato: c, cargos } = q.data;
  const abiertos = cargos.filter((g) => toNumber(g.saldo) > 0);
  const totalAbierto = abiertos.reduce((a, g) => a + toNumber(g.saldo), 0);
  // Lo VENCIDO, que es sobre lo que se pacta un arreglo: la cuota que todavía no vence no se
  // reprograma.
  const vencido = abiertos.filter((g) => g.dias_mora > 0).reduce((a, g) => a + toNumber(g.saldo), 0);

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={`${c.numero} · ${c.cliente_nombre || "sin nombre"}`}
        description={[
          c.documento && `Cédula ${c.documento}`,
          c.sede,
          c.servicio,
          c.modalidad && `${c.modalidad} ${formatMoneda(c.cuota_vigente)}`,
          c.forma_pago,
          c.asociacion,
          c.dia_pago ? `día de pago ${c.dia_pago}` : null,
        ]
          .filter(Boolean)
          .join(" · ")}
        actions={
          <Link to="/cxc/cartera" className="text-sm font-medium text-accent underline">
            Volver a la cartera
          </Link>
        }
      />

      {c.revision_pendiente && (
        <div className="rounded-xl border border-pendiente/40 bg-pendiente/5 px-4 py-3 text-sm">
          <b>En revisión:</b> {c.revision_motivo || "dato fuera de rango en la importación"}.
          <span className="block text-xs text-content-muted">
            Mientras esté marcado, este contrato no genera cargos: sería fabricar deuda sobre un dato que
            ya sabemos que está mal.
          </span>
        </div>
      )}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Kpi label="Saldo" valor={formatMoneda(c.saldo)} tono={toNumber(c.saldo) > 0 ? "negativo" : "positivo"} detalle={`${c.cargos_abiertos} cargos abiertos`} />
        <Kpi
          label="Más vencido"
          valor={c.cargos_abiertos === 0 ? "—" : c.dias_mora_max <= 0 ? "al día" : `${c.dias_mora_max} días`}
          detalle={abiertos[0] ? `vence ${formatFecha(abiertos[0].vence_en)}` : undefined}
        />
        <Kpi label="Cuota vigente" valor={formatMoneda(c.cuota_vigente)} detalle={c.modalidad} />
        <Kpi
          label="Del sistema de origen"
          valor={c.saldo_origen ? formatMoneda(c.saldo_origen) : "—"}
          detalle={[
            c.dias_vencidos_origen !== null ? `${c.dias_vencidos_origen} días` : null,
            c.score_origen !== null ? `score ${c.score_origen}` : null,
            c.morosidad_origen,
          ]
            .filter(Boolean)
            .join(" · ")}
        />
      </div>

      <Card>
        <CardContent className="py-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-sm font-semibold">Cargos del contrato</h3>
            <label className="flex items-center gap-2 text-xs text-content-muted">
              <input
                type="checkbox"
                checked={soloAbiertos}
                onChange={(e) => setSoloAbiertos(e.target.checked)}
                className="h-3.5 w-3.5 rounded border-border accent-accent"
              />
              Solo los que deben algo
            </label>
          </div>

          {cargos.length === 0 ? (
            <EmptyState
              message={
                c.revision_pendiente
                  ? "Sin cargos: el contrato está en revisión."
                  : "Sin cargos todavía. Se crean desde «Importar cartera» → generar cargos."
              }
            />
          ) : (
            <TableContainer className="mt-3">
              <Table>
                <THead>
                  <TR>
                    <TH>Período</TH>
                    <TH>Vence</TH>
                    <TH className="text-right">Monto</TH>
                    <TH className="text-right">Aplicado</TH>
                    <TH className="text-right">Saldo</TH>
                    <TH className="text-right">Días</TH>
                    <TH>Tramo</TH>
                    <TH>Origen</TH>
                  </TR>
                </THead>
                <TBody>
                  {cargos.map((g) => (
                    <TR key={g.id}>
                      <TD className="font-medium tabular-nums">{g.periodo}</TD>
                      <TD className="tabular-nums">{formatFecha(g.vence_en)}</TD>
                      <TD className="text-right tabular-nums">{formatMoneda(g.monto)}</TD>
                      <TD className="text-right tabular-nums text-content-muted">
                        {toNumber(g.monto_aplicado) > 0 ? formatMoneda(g.monto_aplicado) : "—"}
                      </TD>
                      <TD className={cn("text-right font-medium tabular-nums", toNumber(g.saldo) > 0 && "text-negativo")}>
                        {formatMoneda(g.saldo)}
                      </TD>
                      <TD className="text-right tabular-nums">{g.dias_mora}</TD>
                      <TD>
                        <Badge tone={tonoCargo(g.dias_mora, g.saldo)}>{g.tramo_etiqueta || g.tramo || "—"}</Badge>
                      </TD>
                      <TD className="text-[11px] text-content-muted">{g.origen}</TD>
                    </TR>
                  ))}
                  <TR>
                    <TD colSpan={4} className="font-semibold">
                      Total abierto
                    </TD>
                    <TD className="text-right font-semibold tabular-nums text-negativo">
                      {formatMoneda(String(totalAbierto))}
                    </TD>
                    <TD colSpan={3} />
                  </TR>
                </TBody>
              </Table>
            </TableContainer>
          )}

          <p className="mt-3 text-xs text-content-muted">
            El saldo del contrato es <b>la suma de estos cargos</b>, no un campo guardado: no se puede
            desincronizar.
          </p>
        </CardContent>
      </Card>

      <MoraYSuspension numero={numero} estadoContrato={c.estado} />

      <ArreglosDePago numero={numero} vencido={vencido} />

      <NotasDeCredito numero={numero} cargos={cargos} />

      <HistorialDeGestion numero={numero} />
    </div>
  );
}

/**
 * Lo que se le dijo a este cliente y qué contestó. Cada gestión guarda la FOTO del saldo,
 * la mora y el tramo de ese momento: la pregunta «¿cuánto debía cuando lo llamamos?» no se
 * puede reconstruir después, porque el saldo de hoy ya cambió.
 */
function HistorialDeGestion({ numero }: { numero: string }) {
  const q = useGestiones(numero);
  const gestiones = q.data ?? [];
  return (
    <Card>
      <CardContent>
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-semibold">Gestión de cobro</h2>
          <Link to="/cxc/cola" className="text-xs font-medium text-accent underline">
            Ir a la cola de cobro
          </Link>
        </div>
        {q.isPending ? (
          <LoadingState label="Cargando el historial" />
        ) : q.isError ? (
          <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
        ) : gestiones.length === 0 ? (
          <EmptyState message="Nadie ha gestionado este contrato todavía." />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Cuándo</TH>
                  <TH>Canal</TH>
                  <TH>Resultado</TH>
                  <TH className="text-right">Debía entonces</TH>
                  <TH>Promesa</TH>
                  <TH>Quién</TH>
                  <TH>Notas</TH>
                </TR>
              </THead>
              <TBody>
                {gestiones.map((g) => (
                  <TR key={g.id}>
                    <TD className="whitespace-nowrap text-xs">{g.fecha}</TD>
                    <TD className="text-xs">{g.canal}</TD>
                    <TD>
                      <Badge tone={g.es_contacto ? "accent" : "neutral"}>{g.resultado}</Badge>
                    </TD>
                    <TD className="text-right tabular-nums">
                      {formatMoneda(g.saldo_entonces)}
                      <span className="block text-[11px] text-content-muted">
                        {g.dias_mora_entonces} d · {g.tramo_entonces || "—"}
                      </span>
                    </TD>
                    <TD className="text-xs">
                      {g.promesa_fecha ? (
                        <>
                          <span className="block">
                            {formatFecha(g.promesa_fecha)}
                            {g.promesa_monto && ` · ${formatMoneda(g.promesa_monto)}`}
                          </span>
                          {/* El cumplimiento se DERIVA de los cobros: null = todavía en plazo. */}
                          <Badge
                            tone={g.promesa_cumplida === null ? "pendiente" : g.promesa_cumplida ? "positivo" : "negativo"}
                          >
                            {g.promesa_cumplida === null ? "en plazo" : g.promesa_cumplida ? "cumplió" : "no cumplió"}
                          </Badge>
                        </>
                      ) : (
                        <span className="text-content-muted">—</span>
                      )}
                    </TD>
                    <TD className="max-w-[10rem] truncate text-xs">{g.usuario || "—"}</TD>
                    <TD className="max-w-[18rem] text-xs text-content-muted">{g.notas || "—"}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
}

function Kpi({
  label,
  valor,
  detalle,
  tono,
}: {
  label: string;
  valor: string;
  detalle?: string;
  tono?: "negativo" | "positivo" | "pendiente";
}) {
  return (
    <div className="rounded-xl border border-border bg-surface-raised px-5 py-4 shadow-card">
      <p className="text-xs uppercase tracking-wide text-content-muted">{label}</p>
      <p
        className={cn(
          "mt-1 text-2xl font-semibold tabular-nums",
          tono === "negativo"
            ? "text-negativo"
            : tono === "positivo"
              ? "text-positivo"
              : tono === "pendiente"
                ? "text-pendiente"
                : "text-content",
        )}
      >
        {valor}
      </p>
      {detalle && <p className="text-xs text-content-muted">{detalle}</p>}
    </div>
  );
}

/**
 * Notas de crédito del contrato: bajar la deuda sin que entre plata.
 *
 * Las autoriza el supervisor de piso y NO tienen tope (decisión del negocio). Sin un límite
 * que proteja, la pantalla se apoya en lo que sí protege: exige un motivo con contenido,
 * muestra el consecutivo y quién la emitió, y la anulación devuelve los cargos a su saldo en
 * vez de borrar el documento.
 *
 * Emitirla desde acá y no desde una pantalla aparte es a propósito: la decisión de condonar se
 * toma mirando los cargos del cliente, que están justo arriba.
 */
function NotasDeCredito({ numero, cargos }: { numero: string; cargos: CargoCxc[] }) {
  const toast = useToast();
  const tiene = useTienePermiso();
  const puedeEmitir = tiene("cxc.notas_credito");
  const q = useNotasCredito({ contrato: numero, incluir_anuladas: true });
  const emitir = useEmitirNota();
  const anular = useAnularNota();
  const [monto, setMonto] = useState("");
  const [motivo, setMotivo] = useState("");
  const [cargoId, setCargoId] = useState("");
  const [aAnular, setAAnular] = useState<NotaCredito | null>(null);

  const notas = q.data?.items ?? [];
  const vigentes = notas.filter((n) => n.estado === "APLICADA");
  const abiertos = cargos.filter((c) => toNumber(c.saldo) > 0);

  function onEmitir(e: FormEvent) {
    e.preventDefault();
    emitir.mutate(
      {
        contrato: numero,
        monto: monto.trim(),
        motivo: motivo.trim(),
        ...(cargoId ? { cargo_id: cargoId } : {}),
      },
      {
        onSuccess: (n) => {
          const sobra = toNumber(n.sin_aplicar);
          toast.success(
            `${n.consecutivo} por ${formatMoneda(n.monto)}` +
              (n.aplicaciones.length > 0
                ? `: bajó ${n.aplicaciones.map((a) => a.periodo).join(" + ")}`
                : "") +
              (sobra > 0 ? ` · quedó sin aplicar ${formatMoneda(n.sin_aplicar)}` : "."),
          );
          setMonto("");
          setMotivo("");
          setCargoId("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 py-4">
        <div>
          <h2 className="text-sm font-semibold">Notas de crédito</h2>
          <p className="text-xs text-content-muted">
            Bajan la deuda sin que entre plata: condonar, corregir un cargo mal generado o aplicar un
            descuento pactado. Las autoriza el supervisor de piso y no tienen tope, así que el{" "}
            <b>motivo es obligatorio</b> — es lo único que va a explicar esto dentro de seis meses.
          </p>
        </div>

        {vigentes.length > 0 && (
          <p className="text-xs">
            Este contrato tiene <b>{vigentes.length}</b> nota(s) vigente(s) por{" "}
            <b className="text-negativo">
              {formatMoneda(String(vigentes.reduce((a, n) => a + toNumber(n.monto), 0)))}
            </b>
            .
          </p>
        )}

        {puedeEmitir && (
          <form onSubmit={onEmitir} className="flex flex-wrap items-end gap-3">
            <Input
              label="Monto"
              value={monto}
              onChange={(e) => setMonto(e.target.value)}
              placeholder="0.00"
              className="min-w-32"
            />
            <div className="min-w-[13rem]">
              <Select
                label="Aplicar a"
                value={cargoId}
                onChange={(e) => setCargoId(e.target.value)}
                options={[
                  { value: "", label: "El cargo más viejo (FIFO)" },
                  ...abiertos.map((c) => ({
                    value: c.id,
                    label: `${c.periodo} · debe ${formatMoneda(c.saldo)}`,
                  })),
                ]}
              />
            </div>
            <div className="min-w-[22rem] flex-1">
              <Input
                label="Motivo (obligatorio)"
                value={motivo}
                onChange={(e) => setMotivo(e.target.value)}
                placeholder="Por qué se baja esta deuda"
              />
            </div>
            <Button type="submit" loading={emitir.isPending} disabled={!monto.trim() || motivo.trim().length < 10}>
              Emitir nota
            </Button>
          </form>
        )}
        {!puedeEmitir && (
          <p className="text-xs text-content-muted">
            Emitir una nota de crédito requiere el permiso <span className="font-mono">cxc.notas_credito</span>,
            que trae el rol <b>Supervisor de Piso</b>.
          </p>
        )}

        {q.isPending ? (
          <LoadingState label="Cargando las notas" />
        ) : q.isError ? (
          <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
        ) : notas.length === 0 ? (
          <EmptyState message="Este contrato no tiene notas de crédito." />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>N.º</TH>
                  <TH>Fecha</TH>
                  <TH className="text-right">Monto</TH>
                  <TH>Bajó</TH>
                  <TH>Motivo</TH>
                  <TH>Quién</TH>
                  <TH>Estado</TH>
                  {puedeEmitir && <TH />}
                </TR>
              </THead>
              <TBody>
                {notas.map((n) => (
                  <TR key={n.id} className={cn(n.estado === "ANULADA" && "opacity-60")}>
                    <TD className="whitespace-nowrap font-mono text-xs">{n.consecutivo}</TD>
                    <TD className="whitespace-nowrap text-xs">{formatFecha(n.fecha)}</TD>
                    <TD className="text-right font-medium tabular-nums">
                      {formatMoneda(n.monto)}
                      {toNumber(n.sin_aplicar) > 0 && (
                        <span className="block text-[10.5px] text-pendiente">
                          sin aplicar {formatMoneda(n.sin_aplicar)}
                        </span>
                      )}
                    </TD>
                    <TD className="text-xs">
                      {n.aplicaciones.length === 0
                        ? "—"
                        : n.aplicaciones.map((a) => a.periodo).join(" + ")}
                    </TD>
                    <TD className="max-w-[20rem] text-xs text-content-muted">{n.motivo}</TD>
                    <TD className="max-w-[10rem] truncate text-xs">
                      {n.creada_por || "—"}
                      <span className="block text-[10.5px] text-content-muted">{n.creada_en}</span>
                    </TD>
                    <TD>
                      {n.estado === "ANULADA" ? (
                        <>
                          <Badge tone="neutral">Anulada</Badge>
                          <span className="block max-w-[12rem] truncate text-[10.5px] text-content-muted">
                            {n.anulada_por}: {n.anulacion_motivo}
                          </span>
                        </>
                      ) : (
                        <Badge tone="accent">Aplicada</Badge>
                      )}
                    </TD>
                    {puedeEmitir && (
                      <TD className="text-right">
                        {n.estado === "APLICADA" && (
                          <Button size="sm" variant="ghost" onClick={() => setAAnular(n)}>
                            Anular
                          </Button>
                        )}
                      </TD>
                    )}
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}

        {/* Anular pide el motivo con `pedirNota`: deshacer una condonación también hay que
            poder explicarlo. */}
        {aAnular && (
          <ConfirmDialog
            titulo={`¿Anular ${aAnular.consecutivo} por ${formatMoneda(aAnular.monto)}?`}
            descripcion="Los cargos vuelven a su saldo con su antigüedad original, así que el contrato puede volver a la cola de cobro. La nota no se borra: queda marcada con quién la anuló."
            impacto={["El saldo del contrato sube de nuevo", "Queda registrado en la auditoría"]}
            textoConfirmar="Anular la nota"
            tono="peligro"
            pendiente={anular.isPending}
            pedirNota
            notaPlaceholder="Por qué se anula (obligatorio)"
            onConfirmar={(nota) =>
              anular.mutate(
                { id: aAnular.id, motivo: nota },
                {
                  onSuccess: () => {
                    toast.success(`${aAnular.consecutivo} anulada: los cargos volvieron a su saldo.`);
                    setAAnular(null);
                  },
                  onError: (err) => toast.error(mensajeError(err)),
                },
              )
            }
            onCancelar={() => setAAnular(null)}
          />
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Mora acumulada y suspensión del servicio.
 *
 * Muestra las DOS medidas porque las dos hacen falta y no son la misma: los MESES son la regla
 * («18 meses de mora, o su equivalencia») y las CUOTAS son el hecho concreto que se le dice al
 * cliente. Con un quincenal difieren —18 cuotas son 9 meses— y con solo uno de los dos números
 * la decisión no se puede explicar.
 *
 * El sistema NO suspende solo: dice cuándo se puede y quien tiene el permiso decide, con motivo.
/**
 * Mora acumulada y suspensión del servicio.
 *
 * Muestra las DOS medidas porque las dos hacen falta y no son la misma: los MESES son la regla
 * («18 meses de mora, o su equivalencia») y las CUOTAS son el hecho concreto que se le dice al
 * cliente. Con un quincenal difieren —18 cuotas son 9 meses— y con solo uno de los dos números
 * la decisión no se puede explicar.
 *
 * El sistema NO suspende solo: dice cuándo se puede y quien tiene el permiso decide, con motivo.
 */
function MoraYSuspension({ numero, estadoContrato }: { numero: string; estadoContrato: string }) {
  const q = useEstadoSuspension(numero);
  const tiene = useTienePermiso();
  const puede = tiene("cxc.suspender");
  const suspender = useSuspender();
  const reactivar = useReactivar();
  const toast = useToast();
  const [accion, setAccion] = useState<"suspender" | "reactivar" | null>(null);

  const e = q.data;
  if (!e) return null;

  const suspendido = estadoContrato === "SUSPENDIDO";
  const meses = toNumber(e.meses_mora);
  const faltan = Math.max(0, e.tope_meses - meses);

  function aplicar(motivo: string) {
    const mut = accion === "suspender" ? suspender : reactivar;
    mut.mutate(
      { numero, motivo },
      {
        onSuccess: () => {
          toast.success(
            accion === "suspender"
              ? `Servicio suspendido · ${e!.meses_mora} meses de mora`
              : "Contrato reactivado",
          );
          setAccion(null);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Card>
      <CardContent className="py-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">Mora acumulada y suspensión</h3>
            <p className="mt-0.5 max-w-2xl text-xs text-content-muted">
              La regla del negocio son <b>{e.tope_meses} meses</b> de mora, o su equivalencia. En{" "}
              {e.modalidad || "esta modalidad"} cada cuota son <b>{e.meses_por_cuota} meses</b>, así que el
              tope equivale a <b>{e.cuotas_equivalentes || "—"} cuotas</b> de este contrato.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {suspendido ? (
              <>
                <Badge tone="negativo">Servicio suspendido</Badge>
                {puede && (
                  <Button variant="secondary" onClick={() => setAccion("reactivar")}>
                    Reactivar
                  </Button>
                )}
              </>
            ) : e.para_suspender ? (
              <>
                <Badge tone="negativo">Llegó al tope</Badge>
                {puede && <Button onClick={() => setAccion("suspender")}>Suspender el servicio</Button>}
              </>
            ) : (
              <>
                <Badge tone={meses > 0 ? "pendiente" : "positivo"}>{meses > 0 ? "Con mora" : "Al día"}</Badge>
                {puede && meses > 0 && (
                  <Button variant="secondary" onClick={() => setAccion("suspender")}>
                    Suspender por excepción
                  </Button>
                )}
              </>
            )}
          </div>
        </div>

        <div className="mt-3 grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Kpi
            label="Meses de mora"
            valor={`${e.meses_mora} de ${e.tope_meses}`}
            tono={e.para_suspender ? "negativo" : meses > 0 ? "pendiente" : "positivo"}
            detalle={e.para_suspender ? "llegó al tope" : `faltan ${faltan} meses`}
          />
          <Kpi label="Cuotas vencidas" valor={String(e.cuotas_vencidas)} detalle="sin pagar, total o parcial" />
          <Kpi label="Saldo" valor={formatMoneda(e.saldo)} tono={toNumber(e.saldo) > 0 ? "negativo" : "positivo"} />
          <Kpi label="Modalidad" valor={e.modalidad || "—"} detalle={`1 cuota = ${e.meses_por_cuota} meses`} />
        </div>

        {suspendido && e.suspendido_en && (
          <div className="mt-3 rounded-xl border border-negativo/30 bg-negativo/5 px-3 py-2 text-xs">
            Suspendido el <b>{e.suspendido_en}</b>
            {e.suspendido_por && <> por {e.suspendido_por}</>} con{" "}
            <b>
              {e.meses_al_suspender} meses ({e.cuotas_al_suspender} cuotas)
            </b>{" "}
            de mora. <span className="text-content-muted">Motivo: {e.motivo_suspension}</span>
          </div>
        )}

        <p className="mt-3 text-xs text-content-muted">
          El sistema <b>no suspende solo</b>: calcula cuándo se puede y lo muestra. Cortarle el servicio a
          una familia lo decide una persona, con su motivo, y queda registrado.
        </p>
      </CardContent>

      {accion !== null && (
        <ConfirmDialog
          titulo={accion === "suspender" ? `¿Suspender el servicio de ${numero}?` : `¿Reactivar ${numero}?`}
          descripcion={
            accion === "suspender"
              ? e.para_suspender
                ? `Llegó a ${e.meses_mora} meses de mora (${e.cuotas_vencidas} cuotas sin pagar), que es el tope de la regla.`
                : `Tiene ${e.meses_mora} de ${e.tope_meses} meses de mora: esto es una suspensión por EXCEPCIÓN, antes del tope.`
              : "El contrato vuelve a estar activo. La suspensión no se borra: queda cerrada con este motivo."
          }
          impacto={
            accion === "suspender"
              ? [
                  "El contrato queda SUSPENDIDO y pasa a cartera morosa",
                  "Sigue en la cola de cobro: cortar el servicio no borra la deuda",
                  `Se guarda la foto: ${e.meses_mora} meses y ${e.cuotas_vencidas} cuotas`,
                ]
              : ["El contrato vuelve a ACTIVO", "La suspensión queda cerrada con quién la levantó"]
          }
          textoConfirmar={accion === "suspender" ? "Suspender" : "Reactivar"}
          tono={accion === "suspender" ? "peligro" : "accent"}
          pendiente={suspender.isPending || reactivar.isPending}
          pedirNota
          notaPlaceholder="Por qué se toma esta decisión (obligatorio)"
          onConfirmar={aplicar}
          onCancelar={() => setAccion(null)}
        />
      )}
    </Card>
  );
}

const TONO_ARREGLO: Record<string, BadgeTone> = {
  AL_DIA: "positivo",
  EN_MORA: "negativo",
  CUMPLIDO: "positivo",
  QUEBRADO: "negativo",
  ANULADO: "neutral",
};

const ETIQUETA_ARREGLO: Record<string, string> = {
  AL_DIA: "Al día",
  EN_MORA: "En mora del plan",
  CUMPLIDO: "Cumplido",
  QUEBRADO: "Quebrado",
  ANULADO: "Anulado",
};

/**
 * Arreglos de pago del contrato.
 *
 * El arreglo NO reescribe los cargos: es un plan encima de la deuda. Por eso arriba se sigue
 * viendo la mora real aunque el arreglo esté al día, y eso es correcto — lo que se pactó es
 * CÓMO va a pagar, no que la deuda haya rejuvenecido.
 */
function ArreglosDePago({ numero, vencido }: { numero: string; vencido: number }) {
  const q = useArreglos({ contrato: numero });
  const pactar = usePactarArreglo();
  const quebrar = useQuebrarArreglo();
  const anular = useAnularArreglo();
  const tiene = useTienePermiso();
  const puedeGestionar = tiene("cxc.gestionar");
  const toast = useToast();

  const [abierto, setAbierto] = useState(false);
  const [plazo, setPlazo] = useState("");
  const [monto, setMonto] = useState("");
  const [prima, setPrima] = useState("");
  const [primera, setPrimera] = useState("");
  const [motivoAut, setMotivoAut] = useState("");
  const [obs, setObs] = useState("");
  const [cerrar, setCerrar] = useState<{ a: Arreglo; quebrar: boolean } | null>(null);

  const arreglos = q.data?.items ?? [];
  const plazos = q.data?.plazos;
  const vivo = arreglos.find((a) => a.estado === "AL_DIA" || a.estado === "EN_MORA");
  const plazoNum = Number(plazo);
  const esExcepcion = plazos && plazoNum >= 1 ? !plazos.estandar.includes(plazoNum) : false;

  // Las opciones se arman desde el servidor: los estándar siempre, y las excepciones solo si
  // este usuario las puede autorizar. Ofrecer un plazo que el servidor va a rechazar sería
  // hacerle perder el tiempo al gestor.
  const opcionesPlazo = [
    { value: "", label: "Elegir plazo…" },
    ...(plazos?.estandar ?? []).map((p) => ({
      value: String(p),
      label: `${p} ${p === 1 ? "cuota" : "cuotas"}`,
    })),
    ...(plazos?.puede_excepcion
      ? [2, 4, 5, 8, 10, 12, 18, 24]
          .filter((p) => !(plazos.estandar ?? []).includes(p) && p <= plazos.maximo)
          .map((p) => ({ value: String(p), label: `${p} cuotas · excepción` }))
      : []),
  ];

  function enviar(ev: FormEvent) {
    ev.preventDefault();
    pactar.mutate(
      {
        contrato: numero,
        plazo_cuotas: plazoNum,
        monto: monto.trim() || undefined,
        prima: prima.trim() || undefined,
        primera_cuota: primera || undefined,
        observaciones: obs.trim() || undefined,
        motivo_autorizacion: motivoAut.trim() || undefined,
      },
      {
        onSuccess: (a) => {
          toast.success(`Arreglo ${a.consecutivo}: ${formatMoneda(a.monto_arreglo)} en ${a.plazo_cuotas} cuotas`);
          setAbierto(false);
          setPlazo("");
          setMonto("");
          setPrima("");
          setPrimera("");
          setMotivoAut("");
          setObs("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function cerrarArreglo(motivo: string) {
    if (!cerrar) return;
    const mut = cerrar.quebrar ? quebrar : anular;
    mut.mutate(
      { id: cerrar.a.id, motivo },
      {
        onSuccess: () => {
          toast.success(
            cerrar.quebrar ? "Arreglo quebrado: el contrato pasa a cartera morosa" : "Arreglo anulado",
          );
          setCerrar(null);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Card>
      <CardContent className="py-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">Arreglos de pago</h3>
            <p className="mt-0.5 max-w-2xl text-xs text-content-muted">
              Plazos estándar {(plazos?.estandar ?? []).join("-")} cuotas. Cualquier otro plazo es{" "}
              <b>excepción</b> y lo autoriza el supervisor de piso. El arreglo <b>no toca los cargos</b>: la
              mora y la antigüedad de la deuda siguen corriendo.
            </p>
          </div>
          {puedeGestionar && !vivo && vencido > 0 && (
            <Button onClick={() => setAbierto((v) => !v)}>{abierto ? "Cancelar" : "Pactar arreglo"}</Button>
          )}
        </div>

        {abierto && (
          <form onSubmit={enviar} className="mt-3 grid gap-3 rounded-xl border border-border p-3 lg:grid-cols-3">
            <Select
              label="Plazo en cuotas"
              value={plazo}
              onChange={(ev) => setPlazo(ev.target.value)}
              options={opcionesPlazo}
              required
            />
            <Input
              label={`Monto (vacío = todo lo vencido, ${formatMoneda(String(vencido))})`}
              value={monto}
              onChange={(ev) => setMonto(ev.target.value)}
              placeholder={String(vencido)}
              inputMode="decimal"
            />
            <Input
              label="Prima o abono de entrada (opcional)"
              value={prima}
              onChange={(ev) => setPrima(ev.target.value)}
              inputMode="decimal"
            />
            <Input
              label="Primera cuota"
              type="date"
              value={primera}
              onChange={(ev) => setPrimera(ev.target.value)}
            />
            <Input label="Observaciones" value={obs} onChange={(ev) => setObs(ev.target.value)} />
            {esExcepcion && (
              <Input
                label="Motivo de la autorización"
                value={motivoAut}
                onChange={(ev) => setMotivoAut(ev.target.value)}
                placeholder="Por qué se sale de los plazos estándar"
              />
            )}
            <div className="flex flex-wrap items-center justify-between gap-2 lg:col-span-3">
              {esExcepcion ? (
                <span className="text-xs text-pendiente">
                  {plazoNum} cuotas no está entre los plazos estándar: queda marcado como excepción, con su
                  motivo y quién lo autorizó.
                </span>
              ) : (
                <span className="text-xs text-content-muted">
                  Las cuotas se reparten en partes iguales y el redondeo va en la última, para que el plan
                  sume exactamente lo pactado.
                </span>
              )}
              <Button type="submit" disabled={pactar.isPending || plazoNum < 1}>
                {pactar.isPending ? "Pactando…" : "Pactar"}
              </Button>
            </div>
          </form>
        )}

        {arreglos.length === 0 ? (
          <EmptyState
            message={
              vencido > 0
                ? "Sin arreglos. Se puede pactar un plan sobre la deuda vencida."
                : "Sin arreglos. Un arreglo de pago se hace sobre deuda vencida."
            }
          />
        ) : (
          <div className="mt-3 flex flex-col gap-3">
            {arreglos.map((a) => (
              <div key={a.id} className="rounded-xl border border-border p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex flex-wrap items-center gap-2 text-sm">
                    <b>Arreglo {a.consecutivo}</b>
                    <Badge tone={TONO_ARREGLO[a.estado] ?? "neutral"}>
                      {ETIQUETA_ARREGLO[a.estado] ?? a.estado}
                    </Badge>
                    {a.es_excepcion && <Badge tone="pendiente">Excepción</Badge>}
                    <span className="text-content-muted">
                      {formatMoneda(a.monto_arreglo)} en {a.plazo_cuotas} cuotas
                      {toNumber(a.prima) > 0 && <> · prima {formatMoneda(a.prima)}</>}
                    </span>
                  </div>
                  {(a.estado === "AL_DIA" || a.estado === "EN_MORA") && (
                    <div className="flex gap-2">
                      <Button size="sm" variant="secondary" onClick={() => setCerrar({ a, quebrar: true })}>
                        Quebrar
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => setCerrar({ a, quebrar: false })}>
                        Anular
                      </Button>
                    </div>
                  )}
                </div>

                <div className="mt-2 grid grid-cols-2 gap-3 lg:grid-cols-4">
                  <Kpi label="Pagado" valor={formatMoneda(a.pagado)} detalle={`de ${formatMoneda(a.monto_arreglo)}`} />
                  <Kpi
                    label="Esperado a hoy"
                    valor={formatMoneda(a.esperado_a_hoy)}
                    detalle={`${a.cuotas_cubiertas} de ${a.cuotas.length} cuotas cubiertas`}
                  />
                  <Kpi
                    label="Atraso"
                    valor={formatMoneda(a.atraso)}
                    tono={toNumber(a.atraso) > 0 ? "negativo" : "positivo"}
                  />
                  <Kpi
                    label="Próxima cuota"
                    valor={a.proxima_cuota ? formatFecha(a.proxima_cuota) : "—"}
                    detalle={a.proximo_monto ? formatMoneda(a.proximo_monto) : undefined}
                  />
                </div>

                <TableContainer className="mt-3">
                  <Table>
                    <THead>
                      <TR>
                        <TH>Cuota</TH>
                        <TH>Vence</TH>
                        <TH className="text-right">Monto</TH>
                        <TH>Estado</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {a.cuotas.map((cu) => (
                        <TR key={cu.numero}>
                          <TD className="font-medium tabular-nums">{cu.numero === 0 ? "Prima" : cu.numero}</TD>
                          <TD className="tabular-nums">{formatFecha(cu.vence_en)}</TD>
                          <TD className="text-right tabular-nums">{formatMoneda(cu.monto)}</TD>
                          <TD>
                            {cu.cubierta ? (
                              <Badge tone="positivo">Cubierta</Badge>
                            ) : cu.vencida ? (
                              <Badge tone="negativo">Vencida sin pagar</Badge>
                            ) : (
                              <Badge tone="neutral">Por vencer</Badge>
                            )}
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>

                <p className="mt-2 text-[11px] text-content-muted">
                  {a.autorizacion_motivo && (
                    <>
                      Autorizado por {a.autorizado_por}: {a.autorizacion_motivo}.{" "}
                    </>
                  )}
                  {a.quebranto_motivo && (
                    <>
                      Quebrado por {a.quebrado_por}: {a.quebranto_motivo}.{" "}
                    </>
                  )}
                  {a.anulacion_motivo && (
                    <>
                      Anulado por {a.anulado_por}: {a.anulacion_motivo}.{" "}
                    </>
                  )}
                  Al pactarse debía {formatMoneda(a.vencido_al_pactar)} con {a.meses_mora_al_pactar} meses de
                  mora.
                </p>
              </div>
            ))}
          </div>
        )}

        <p className="mt-3 text-xs text-content-muted">
          El cumplimiento <b>no se marca a mano</b>: sale de los cobros y se mide acumulado («a hoy debía
          haber pagado X, pagó Y»), así que quien adelanta una cuota no aparece en mora en la siguiente.
        </p>
      </CardContent>

      {cerrar && (
        <ConfirmDialog
          titulo={
            cerrar.quebrar
              ? `¿Quebrar el arreglo ${cerrar.a.consecutivo}?`
              : `¿Anular el arreglo ${cerrar.a.consecutivo}?`
          }
          descripcion={
            cerrar.quebrar
              ? "Quebrar declara el INCUMPLIMIENTO del cliente. Es distinto de anular."
              : "Anular es para el arreglo que no debió existir: se capturó mal, el cliente no firmó. No marca incumplimiento."
          }
          impacto={
            cerrar.quebrar
              ? [
                  "El contrato pasa a CARTERA MOROSA",
                  "La regla de los 18 meses sigue corriendo sobre los cargos originales",
                  `Llevaba pagado ${formatMoneda(cerrar.a.pagado)} de ${formatMoneda(cerrar.a.monto_arreglo)}`,
                ]
              : ["NO manda a cartera morosa", "Se puede pactar otro arreglo en su lugar"]
          }
          textoConfirmar={cerrar.quebrar ? "Quebrar" : "Anular"}
          tono={cerrar.quebrar ? "peligro" : "accent"}
          pendiente={quebrar.isPending || anular.isPending}
          pedirNota
          notaPlaceholder="Por qué se cierra este arreglo (obligatorio)"
          onConfirmar={cerrarArreglo}
          onCancelar={() => setCerrar(null)}
        />
      )}
    </Card>
  );
}
