/**
 * Bancos — Mi partida (/mi-segmento).
 *
 * La pantalla del equipo de una partida: Asociaciones, Depósitos, Emergencias. Contesta una sola
 * pregunta —«¿entró el dinero?»— y no abre nada más del módulo.
 *
 * Lo que el equipo puede hacer acá es mirar y AVISAR. Corregir la partida sigue siendo de quien
 * clasifica: el aviso cae en la pestaña «Reportados» de Clasificar. Esa separación es a propósito
 * (la segmentación tiene que quedar bien en Bancos, y no la arregla quien la señala).
 *
 * El recorte por alcance lo aplica el servidor en su propio endpoint. Esta pantalla no filtra por
 * partida ni podría: no recibe las que no le tocan.
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
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { etiquetaPeriodo, formatFecha, formatMoneda, formatMonto } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import {
  useBuscarFaltante,
  useMiSegmento,
  useReportarFaltante,
  useReportarSegmentacion,
} from "@/features/bancos/hooks";
import type { MovimientoRow, ResultadoFaltante } from "@/api/bancos";

export function MiSegmentoPage() {
  const { periodo } = usePeriodoActivo();
  const [q, setQ] = useState("");
  const [buscado, setBuscado] = useState("");
  // Filtros propios de la pantalla, los mismos que la hoja de trabajo: cuenta, rango de fechas y
  // orden. NO hay filtro de partida: el alcance ya la fija y ofrecer un selector con una sola
  // opción es ruido.
  const [cuentaId, setCuentaId] = useState("");
  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const [orden, setOrden] = useState("");

  // El período global manda salvo que se escriba un rango de fechas: con «desde/hasta» puestos, el
  // mes activo dejaría afuera lo que se está buscando (y buscar un depósito viejo es justo para lo
  // que sirve el rango).
  const usaRango = Boolean(desde || hasta);
  const query = useMiSegmento({
    periodo: usaRango ? undefined : periodo,
    desde: desde || undefined,
    hasta: hasta || undefined,
    cuenta_bancaria_id: cuentaId || undefined,
    orden: orden || undefined,
    q: buscado || undefined,
    page_size: 200,
  });
  const [reportando, setReportando] = useState<MovimientoRow | null>(null);
  const [buscandoFaltante, setBuscandoFaltante] = useState(false);

  const mesActivo = etiquetaPeriodo(periodo);

  if (query.isPending) return <LoadingState label="Cargando tu partida" />;
  if (query.isError) {
    return <ErrorState message={mensajeError(query.error)} onRetry={() => query.refetch()} />;
  }

  const data = query.data;
  const items = data?.movimientos.items ?? [];
  const hayFiltro = Boolean(buscado || cuentaId || desde || hasta);

  function limpiarFiltros() {
    setQ("");
    setBuscado("");
    setCuentaId("");
    setDesde("");
    setHasta("");
    setOrden("");
  }

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Mi partida en bancos"
        description="Lo que entró al banco en las partidas que consultás. Solo ingresos, y solo los tuyos."
        actions={
          <div className="flex items-center gap-2">
            {(data?.partidas ?? []).map((p) => (
              <Badge key={p.clasificacion_id} tone="accent">
                {p.clasificacion}
              </Badge>
            ))}
            {/* Fuera de la tabla a propósito: es el aviso de algo que NO tiene fila. */}
            {!data?.sin_alcance && (
              <Button variant="secondary" onClick={() => setBuscandoFaltante(true)}>
                Falta un movimiento
              </Button>
            )}
          </div>
        }
      />

      {/* Un rol sin partidas asignadas NO es un error: es el estado inicial. Se dice qué falta y
          quién lo hace, en vez de mostrar una tabla vacía que parece una falla del sistema. */}
      {data?.sin_alcance ? (
        <Card>
          <CardContent className="flex flex-col gap-2 py-8 text-center">
            <p className="text-base font-medium text-content">
              Todavía no tenés ninguna partida asignada
            </p>
            <p className="mx-auto max-w-prose text-sm text-content-muted">
              {data.aviso ??
                "Pedile a Dirección Financiera que marque tus partidas en el catálogo de Bancos."}
            </p>
          </CardContent>
        </Card>
      ) : (
        <>
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              setBuscado(q.trim());
            }}
          >
            <Input
              label="Buscar en el detalle del banco"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Ej. SOLIDARIS, COOPENAE, SINPE…"
              className="min-w-56"
            />
            {/* Las cuentas salen de SU segmento, no del catálogo: este rol no tiene acceso al
                catálogo de cuentas y el desplegable le respondería 403. */}
            <Select
              label="Cuenta"
              value={cuentaId}
              onChange={(e) => setCuentaId(e.target.value)}
              options={[
                { value: "", label: "Todas mis cuentas" },
                ...(data?.cuentas ?? []).map((c) => ({
                  value: c.id,
                  label: `${c.banco} · ${c.cuenta}`,
                })),
              ]}
              className="min-w-48"
            />
            <Input
              label="Desde"
              type="date"
              value={desde}
              onChange={(e) => setDesde(e.target.value)}
              className="w-36"
            />
            <Input
              label="Hasta"
              type="date"
              value={hasta}
              onChange={(e) => setHasta(e.target.value)}
              className="w-36"
            />
            <Select
              label="Ordenar por"
              value={orden}
              onChange={(e) => setOrden(e.target.value)}
              options={[
                { value: "", label: "Más recientes" },
                { value: "fecha_asc", label: "Más antiguos" },
                { value: "monto_desc", label: "Monto: mayor a menor" },
                { value: "monto_asc", label: "Monto: menor a mayor" },
              ]}
              className="min-w-44"
            />
            <div className="flex items-end gap-2 pb-0.5">
              <Button type="submit" variant="secondary">
                Buscar
              </Button>
              {hayFiltro && (
                <Button type="button" variant="ghost" onClick={limpiarFiltros}>
                  Quitar filtros
                </Button>
              )}
            </div>
          </form>

          {/* El dato que separa «todavía no entró» de «entró y no es mío». Va como texto y no como
              tarjeta: es una referencia, no una cifra que haya que mirar. */}
          <p className="text-xs text-content-muted">
            {usaRango ? (
              <>
                Mostrando el rango de fechas que pediste, no el mes activo.{" "}
                <button
                  type="button"
                  className="underline hover:text-content"
                  onClick={() => {
                    setDesde("");
                    setHasta("");
                  }}
                >
                  Volver a {mesActivo}
                </button>
                {" · "}
              </>
            ) : (
              <>Mostrando {mesActivo}. · </>
            )}
            {data?.cargado_hasta ? (
              <>
                El banco está cargado hasta el{" "}
                <b className="text-content">{formatFecha(data.cargado_hasta)}</b>: lo posterior a esa
                fecha todavía no se importó.
              </>
            ) : (
              <>Todavía no hay ningún movimiento importado.</>
            )}
          </p>

          {items.length === 0 ? (
            <EmptyState
              message={
                hayFiltro
                  ? "Ningún movimiento de tu partida coincide con lo que filtraste. Probá quitando los filtros antes de dar por perdido el movimiento."
                  : `Todavía no entró nada de tu partida en ${mesActivo}.`
              }
            />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Fecha</TH>
                    <TH>Cuenta</TH>
                    <TH>Detalle del banco</TH>
                    <TH className="text-right">Entró</TH>
                    <TH className="text-right">Segmentación</TH>
                  </TR>
                </THead>
                <TBody>
                  {items.map((m) => (
                    <TR key={m.id} className={m.reporte_abierto ? "bg-pendiente/5" : undefined}>
                      <TD className="tabular-nums whitespace-nowrap">{m.fecha}</TD>
                      <TD className="text-sm text-content-muted whitespace-nowrap">
                        {m.banco} · {m.cuenta}
                      </TD>
                      <TD className="text-sm">{m.descripcion || "—"}</TD>
                      <TD className="text-right tabular-nums whitespace-nowrap">
                        {formatMonto(m.credito)}
                      </TD>
                      <TD className="text-right">
                        {m.reporte_abierto ? (
                          // Ya avisado: se muestra el motivo escrito en vez de ofrecer avisar otra
                          // vez. Es lo que evita tres avisos idénticos del mismo movimiento.
                          <div className="flex flex-col items-end gap-0.5">
                            <Badge tone="pendiente">En revisión</Badge>
                            <span className="text-[11px] text-content-muted">
                              «{m.reporte_abierto}»
                            </span>
                          </div>
                        ) : (
                          <Button size="sm" variant="ghost" onClick={() => setReportando(m)}>
                            Está mal segmentado
                          </Button>
                        )}
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )}

          <p className="text-xs text-content-muted">
            Si un movimiento no es de tu partida —o falta alguno que sí lo es— avisalo desde acá. Lo
            corrige quien clasifica en Bancos, y te queda la respuesta en esta misma pantalla.
          </p>
        </>
      )}

      {reportando && (
        <ReportarDialog movimiento={reportando} onCerrar={() => setReportando(null)} />
      )}
      {buscandoFaltante && <FaltanteDialog onCerrar={() => setBuscandoFaltante(false)} />}
    </div>
  );
}

/**
 * «Falta un movimiento»: buscar primero, avisar después.
 *
 * El orden importa. Avisar a ciegas llena la cola de quien clasifica con plata que todavía no
 * entró al banco; buscar primero separa las cuatro razones por las que un movimiento no aparece:
 *
 *   · está y es suyo      → lo tapaba un filtro o el mes. Se muestra y no hay nada que avisar.
 *   · está en otra partida → es el caso a corregir, y el aviso sirve.
 *   · no está             → o no se depositó, o no se importó ese día. La fecha de carga lo dice.
 *
 * Fecha y monto EXACTOS a propósito: sin rangos no se puede tantear la existencia de movimientos
 * ajenos, que es lo que hace aceptable que el sistema conteste.
 */
function FaltanteDialog({ onCerrar }: { onCerrar: () => void }) {
  const toast = useToast();
  const buscar = useBuscarFaltante();
  const avisar = useReportarFaltante();

  const [fecha, setFecha] = useState("");
  const [monto, setMonto] = useState("");
  const [referencia, setReferencia] = useState("");
  const [motivo, setMotivo] = useState("");
  const [resultado, setResultado] = useState<ResultadoFaltante | null>(null);

  const listoParaBuscar = Boolean(fecha && monto.trim());

  function hacerBusqueda() {
    buscar.mutate(
      { fecha, monto },
      {
        onSuccess: (r) => setResultado(r),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function enviarAviso() {
    avisar.mutate(
      { fecha, monto, referencia, motivo },
      {
        onSuccess: () => {
          toast.success("Aviso enviado. Queda en revisión de quien clasifica.");
          onCerrar();
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}
    >
      <div className="flex max-h-[90vh] w-full max-w-lg flex-col gap-3 overflow-y-auto rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <div>
          <h2 className="text-base font-semibold text-content">Falta un movimiento</h2>
          <p className="mt-1 text-sm text-content-muted">
            Poné la fecha y el monto exactos de lo que esperás. Primero lo busco en el banco.
          </p>
        </div>

        <div className="flex flex-wrap gap-3">
          <Input
            label="Fecha del movimiento *"
            type="date"
            value={fecha}
            onChange={(e) => {
              setFecha(e.target.value);
              setResultado(null);
            }}
            className="w-40"
          />
          <div className="min-w-40 flex-1">
            <Input
              label="Monto exacto *"
              value={monto}
              onChange={(e) => {
                setMonto(e.target.value);
                setResultado(null);
              }}
              placeholder="4 950,00"
              hint="Como está en el recibo, hasta los céntimos."
            />
          </div>
        </div>

        {!resultado ? (
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={onCerrar}>
              Cancelar
            </Button>
            <Button onClick={hacerBusqueda} loading={buscar.isPending} disabled={!listoParaBuscar}>
              Buscar en el banco
            </Button>
          </div>
        ) : (
          <>
            {resultado.veredicto === "EN_MI_PARTIDA" && (
              <div className="rounded-lg border border-accent/40 bg-accent/5 px-3 py-2 text-sm">
                <p className="font-medium text-content">Sí está, y es de tu partida.</p>
                <p className="mt-0.5 text-content-muted">
                  No aparecía por el mes o los filtros que tenías puestos. No hay nada que avisar.
                </p>
                <ul className="mt-2 flex flex-col gap-1">
                  {resultado.movimientos.map((m) => (
                    <li key={m.id} className="text-content">
                      <span className="tabular-nums">{formatFecha(m.fecha)}</span> · {m.banco} ·{" "}
                      {m.cuenta} — {m.descripcion || "sin detalle"} ·{" "}
                      <b className="tabular-nums">{formatMoneda(m.credito)}</b>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {resultado.veredicto === "FUERA_DE_MI_PARTIDA" && (
              // Se dice que existe y nada más. El detalle no se muestra porque no es de su partida:
              // eso es exactamente lo que hay que corregir, y lo corrige quien clasifica.
              <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-3 py-2 text-sm">
                <p className="font-medium text-content">
                  Sí entró ese monto ese día, pero está en otra partida.
                </p>
                <p className="mt-0.5 text-content-muted">
                  Por eso no te aparece. Avisá y quien clasifica lo corrige; el movimiento ya queda
                  identificado en el aviso.
                </p>
              </div>
            )}

            {resultado.veredicto === "NO_EXISTE" && (
              <div className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
                <p className="font-medium text-content">No hay ningún ingreso de ese monto ese día.</p>
                <p className="mt-0.5 text-content-muted">
                  {resultado.cargado_hasta && fecha > resultado.cargado_hasta
                    ? `El banco está cargado hasta el ${formatFecha(resultado.cargado_hasta)}, así que ese día todavía no se importó: conviene esperar antes de avisar.`
                    : "Puede que no se haya depositado, o que el monto o la fecha sean otros. Revisá el recibo antes de avisar."}
                </p>
              </div>
            )}

            {resultado.veredicto !== "EN_MI_PARTIDA" && (
              <div className="flex flex-col gap-3 border-t border-border pt-3">
                <Input
                  label="¿Qué esperabas? *"
                  value={motivo}
                  onChange={(e) => setMotivo(e.target.value)}
                  placeholder="Ej. depósito de ventanilla del cliente Rojas"
                  hint="Lo lee quien clasifica en Bancos."
                />
                <Input
                  label="Referencia (opcional)"
                  value={referencia}
                  onChange={(e) => setReferencia(e.target.value)}
                  placeholder="Nº de recibo, comprobante, SINPE…"
                />
                <div className="flex justify-end gap-2">
                  <Button variant="secondary" onClick={onCerrar} disabled={avisar.isPending}>
                    Cancelar
                  </Button>
                  <Button onClick={enviarAviso} loading={avisar.isPending} disabled={!motivo.trim()}>
                    Enviar el aviso
                  </Button>
                </div>
              </div>
            )}

            {resultado.veredicto === "EN_MI_PARTIDA" && (
              <div className="flex justify-end">
                <Button onClick={onCerrar}>Listo</Button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

/**
 * El aviso. El motivo es obligatorio y el servidor también lo exige: sin él no se puede corregir
 * nada, y un aviso vacío obliga a quien clasifica a adivinar o a llamar por teléfono.
 */
function ReportarDialog({
  movimiento,
  onCerrar,
}: {
  movimiento: MovimientoRow;
  onCerrar: () => void;
}) {
  const toast = useToast();
  const reportar = useReportarSegmentacion();
  const [motivo, setMotivo] = useState("");

  function enviar() {
    reportar.mutate(
      { movimientoId: movimiento.id, motivo },
      {
        onSuccess: () => {
          toast.success("Aviso enviado. Queda en revisión de quien clasifica.");
          onCerrar();
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}
    >
      <div className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="text-base font-semibold text-content">Avisar que está mal segmentado</h2>
        <div className="mt-3 rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
          <p className="tabular-nums text-content-muted">
            {movimiento.fecha} · {movimiento.banco} · {movimiento.cuenta}
          </p>
          <p className="mt-0.5 text-content">{movimiento.descripcion || "—"}</p>
          <p className="mt-0.5 font-semibold tabular-nums text-content">
            {formatMoneda(movimiento.credito)}
          </p>
        </div>
        <div className="mt-3">
          <Input
            label="¿Qué está mal? *"
            value={motivo}
            onChange={(e) => setMotivo(e.target.value)}
            placeholder="Ej. esto es de Emergencias, no de Asociaciones"
            hint="Lo lee quien clasifica en Bancos. Con el detalle correcto se corrige de una."
          />
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="secondary" onClick={onCerrar} disabled={reportar.isPending}>
            Cancelar
          </Button>
          <Button onClick={enviar} loading={reportar.isPending} disabled={!motivo.trim()}>
            Enviar el aviso
          </Button>
        </div>
      </div>
    </div>
  );
}
