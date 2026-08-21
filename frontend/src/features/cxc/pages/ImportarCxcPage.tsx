/**
 * CxC — Importar cartera y generar cargos (/cxc/importar).
 *
 * Dos pasos que nunca se juntan:
 *   1. CARTERA: subir → previsualizar → conciliar → confirmar. Nada entra en silencio.
 *   2. CARGOS: el generador PREVISUALIZA cuántos crearía y exige una fecha de arranque.
 *      Generar desde el primer cobro de los contratos viejos son millones de filas, así
 *      que se elige con el número delante y se confirma ese número.
 */

import { useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  ConfirmDialog,
  ErrorState,
  Input,
  PageHeader,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
} from "@/components/ui";
import { useToast } from "@/components/ui";
import { formatFecha, formatMoneda, hoyCR } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import {
  useConfirmarContratos,
  useConfirmarCobros,
  useGenerarCargos,
  usePlanCargos,
  usePrevisualizarCobros,
  usePrevisualizarContratos,
} from "@/features/cxc/hooks";
import type { ConciliacionCobros, ConciliacionCxc } from "@/api/cxc";

export function ImportarCxcPage() {
  const toast = useToast();

  // ── Paso 1: cartera
  const [archivo, setArchivo] = useState<File | null>(null);
  const [importacionId, setImportacionId] = useState("");
  const [reporte, setReporte] = useState<ConciliacionCxc | null>(null);
  const previsualizar = usePrevisualizarContratos();
  const confirmar = useConfirmarContratos();
  const [confirmando, setConfirmando] = useState(false);

  // ── Paso 2: cargos
  const [desde, setDesde] = useState(`${hoyCR().slice(0, 4)}-01-01`);
  const [hasta, setHasta] = useState(hoyCR());
  const [pedirPlan, setPedirPlan] = useState(false);
  const planQ = usePlanCargos(pedirPlan ? desde : "", hasta);
  const generar = useGenerarCargos();
  const [generando, setGenerando] = useState(false);

  function elegirArchivo(f: File | null) {
    setArchivo(f);
    setReporte(null);
    setImportacionId("");
    if (!f) return;
    previsualizar.mutate(f, {
      onSuccess: (r) => {
        setReporte(r.reporte);
        setImportacionId(r.importacion_id);
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function aplicar() {
    if (!archivo) return;
    confirmar.mutate(
      { archivo, importacionId },
      {
        onSuccess: (r) => {
          toast.success(`Cartera actualizada: ${r.aplicado.nuevos} nuevos · ${r.aplicado.actualizados} actualizados.`);
          setReporte(null);
          setArchivo(null);
          setImportacionId("");
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
    setConfirmando(false);
  }

  // ── Paso 3: cobros. Mismo camino que la cartera: subir → previsualizar → confirmar.
  const [archivoPagos, setArchivoPagos] = useState<File | null>(null);
  const [impCobrosId, setImpCobrosId] = useState("");
  const [repCobros, setRepCobros] = useState<ConciliacionCobros | null>(null);
  const prevCobros = usePrevisualizarCobros();
  const confirmarCobros = useConfirmarCobros();
  const [confirmandoCobros, setConfirmandoCobros] = useState(false);

  function elegirPagos(f: File | null) {
    setArchivoPagos(f);
    setRepCobros(null);
    setImpCobrosId("");
    if (!f) return;
    prevCobros.mutate(f, {
      onSuccess: (r) => {
        setRepCobros(r.reporte);
        setImpCobrosId(r.importacion_id);
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function aplicarCobros() {
    if (!archivoPagos) return;
    confirmarCobros.mutate(
      { archivo: archivoPagos, importacionId: impCobrosId },
      {
        onSuccess: (r) => {
          const a = r.aplicado;
          toast.success(
            `${a.registrados} cobros registrados · ${formatMoneda(a.aplicado)} aplicados` +
              (a.repetidos > 0 ? ` · ${a.repetidos} ya estaban` : "") +
              (a.sin_identificar > 0 ? ` · ${a.sin_identificar} sin identificar` : ""),
          );
          if (r.fallas.length > 0) {
            toast.error(`${r.fallas.length} fila(s) no entraron: ${r.fallas[0]}`);
          }
          setRepCobros(null);
          setArchivoPagos(null);
          setImpCobrosId("");
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
    setConfirmandoCobros(false);
  }

  const plan = planQ.data;
  const nuevos = reporte?.resolucion?.sedes_nuevas ?? [];
  const asocNuevas = reporte?.resolucion?.asociaciones_nuevas ?? [];
  const modDesc = reporte?.resolucion?.modalidades_desconocidas ?? [];
  const formasDesc = reporte?.resolucion?.formas_pago_desconocidas ?? [];

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Importar cartera"
        description="El archivo del sistema de origen (CSV o Excel) y la API entran por el mismo camino. Se previsualiza, se concilia y se confirma."
      />

      {/* ═══ Paso 1 ═══ */}
      <Card>
        <CardContent className="flex flex-col gap-3 py-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="text-sm font-semibold">1 · Contratos</h3>
            <Badge tone="neutral">las columnas se resuelven por encabezado, no por posición</Badge>
          </div>
          <input
            type="file"
            accept=".csv,.xlsx,.xls,text/csv"
            onChange={(e) => elegirArchivo(e.target.files?.[0] ?? null)}
            className="block w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm file:mr-3 file:rounded-md file:border-0 file:bg-accent file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-white"
          />
          {previsualizar.isPending && <p className="text-sm text-content-muted">Leyendo el archivo…</p>}
          {previsualizar.isError && <ErrorState message={mensajeError(previsualizar.error)} />}

          {reporte && (
            <>
              <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
                <Dato k="Filas" v={reporte.filas} />
                <Dato k="Nuevos" v={reporte.nuevos} tono="positivo" />
                <Dato k="Actualizados" v={reporte.actualizados} />
                <Dato k="Duplicados" v={reporte.duplicados} nota="se ignoran" />
                <Dato k="Cuarentena" v={reporte.cuarentena} tono={reporte.cuarentena > 0 ? "pendiente" : undefined} nota="entran marcados" />
                <Dato k="Sin sede" v={reporte.sin_sede} />
              </div>

              {(nuevos.length > 0 || asocNuevas.length > 0) && (
                <div className="rounded-lg border border-accent/40 bg-accent/5 px-3 py-2 text-xs">
                  <b>Se van a crear:</b>{" "}
                  {nuevos.length > 0 && <>sedes «{nuevos.join("», «")}»{asocNuevas.length > 0 && " · "}</>}
                  {asocNuevas.length > 0 && <>asociaciones «{asocNuevas.join("», «")}»</>}
                </div>
              )}
              {(modDesc.length > 0 || formasDesc.length > 0) && (
                <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-xs">
                  <b>No se crean solas</b> (gobiernan el ciclo de cobro y el factor de recuperación; esas filas
                  quedan en revisión):{" "}
                  {modDesc.length > 0 && <>modalidades «{modDesc.join("», «")}»{formasDesc.length > 0 && " · "}</>}
                  {formasDesc.length > 0 && <>formas de pago «{formasDesc.join("», «")}»</>}
                </div>
              )}

              <div>
                <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-content-muted">
                  Cómo se entendieron las primeras filas
                </p>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Línea</TH>
                        <TH>Contrato</TH>
                        <TH>Cliente</TH>
                        <TH>Razón social · plaza</TH>
                        <TH>Modalidad · forma</TH>
                        <TH className="text-right">Cuota</TH>
                        <TH>Primer cobro</TH>
                        <TH>Motivos</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {reporte.muestra.map((f) => (
                        <TR key={f.linea}>
                          <TD className="tabular-nums text-content-muted">{f.linea}</TD>
                          <TD className="font-medium">{f.numero}</TD>
                          <TD className="max-w-[13rem] truncate">{f.cliente}</TD>
                          <TD className="max-w-[13rem] text-xs">
                            {f.razon_social || <span className="text-negativo">sin razón social</span>}
                            <span className="block text-content-muted">{f.plaza}</span>
                          </TD>
                          <TD className="text-xs">
                            {f.modalidad}
                            <span className="block text-content-muted">{f.forma_pago}</span>
                          </TD>
                          <TD className="text-right tabular-nums">{formatMoneda(f.cuota)}</TD>
                          <TD className="tabular-nums">{f.primer_cobro ? formatFecha(f.primer_cobro) : "—"}</TD>
                          <TD className="text-xs">
                            {f.motivos?.length ? (
                              <Badge tone="pendiente">{f.motivos.join(" · ")}</Badge>
                            ) : (
                              <span className="text-positivo">ok</span>
                            )}
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              </div>

              <div className="flex flex-wrap items-center gap-3">
                <Button onClick={() => setConfirmando(true)} loading={confirmar.isPending}>
                  Confirmar {reporte.filas - reporte.duplicados} contratos
                </Button>
                <Button variant="ghost" onClick={() => elegirArchivo(null)}>
                  Descartar
                </Button>
                <span className="text-xs text-content-muted">
                  Previsualizar no escribió nada todavía.
                </span>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* ═══ Paso 2 ═══ */}
      <Card>
        <CardContent className="flex flex-col gap-3 py-4">
          <h3 className="text-sm font-semibold">2 · Generar cargos</h3>
          <p className="text-xs text-content-muted">
            Crea el cargo de cada período de cada contrato activo según su ciclo (mensual, quincenal,
            trimestral, semestral o anual). Es <b>idempotente</b>: correrlo dos veces no duplica nada.
          </p>
          <div className="flex flex-wrap items-end gap-3">
            <Input label="Desde" type="date" value={desde} onChange={(e) => setDesde(e.target.value)} className="w-40" />
            <Input label="Hasta" type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} className="w-40" />
            <Button variant="secondary" onClick={() => setPedirPlan(true)} loading={planQ.isFetching && pedirPlan}>
              Calcular el plan
            </Button>
          </div>

          {pedirPlan && planQ.isError && <ErrorState message={mensajeError(planQ.error)} />}

          {plan && (
            <>
              <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
                <Dato k="Contratos" v={plan.contratos} />
                <Dato k="Cargos a crear" v={plan.cargos} tono="positivo" />
                <Dato k="Desde" v={formatFecha(plan.desde)} />
                <Dato k="Hasta" v={formatFecha(plan.hasta)} />
              </div>

              {plan.excluidos && Object.keys(plan.excluidos).length > 0 && (
                <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-xs">
                  <b>Contratos que no generan cargos:</b>{" "}
                  {Object.entries(plan.excluidos)
                    .map(([motivo, n]) => `${n} ${motivo}`)
                    .join(" · ")}
                </div>
              )}

              {plan.sobre_el_tope && (
                <div className="rounded-lg border border-negativo/40 bg-negativo/5 px-3 py-2 text-xs">
                  Son <b>{plan.cargos.toLocaleString("es-CR")}</b> cargos, más que el tope de{" "}
                  {plan.tope.toLocaleString("es-CR")}. Al confirmar se manda ese número exacto: si el plan
                  cambia entre que lo ves y lo aceptás, el servidor aborta.
                </div>
              )}

              <div className="flex flex-wrap items-center gap-3">
                <Button onClick={() => setGenerando(true)} disabled={plan.cargos === 0} loading={generar.isPending}>
                  Generar {plan.cargos.toLocaleString("es-CR")} cargos
                </Button>
                {plan.cargos === 0 && (
                  <span className="text-xs text-content-muted">
                    Nada que crear en ese rango (o ya están creados).
                  </span>
                )}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* ═══ Paso 3: cobros ═══ */}
      <Card>
        <CardContent className="flex flex-col gap-3 py-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="text-sm font-semibold">3 · Cobros (archivo de pagos)</h3>
            <Badge tone="neutral">idempotente: reimportar el mismo archivo no duplica</Badge>
          </div>
          <p className="text-xs text-content-muted">
            Se aplican al cargo <b>más viejo primero</b>; lo que sobre queda a favor del cliente. Un pago
            cuyo contrato no esté en la cartera <b>entra igual</b>, a la bandeja de «sin identificar»: la
            plata llegó de verdad.
          </p>
          <input
            type="file"
            accept=".csv,.xlsx,.xls,text/csv"
            onChange={(e) => elegirPagos(e.target.files?.[0] ?? null)}
            className="block w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm file:mr-3 file:rounded-md file:border-0 file:bg-accent file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-white"
          />
          {prevCobros.isPending && <p className="text-sm text-content-muted">Leyendo el archivo…</p>}
          {prevCobros.isError && <ErrorState message={mensajeError(prevCobros.error)} />}

          {repCobros && (
            <>
              <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
                <Dato k="Filas" v={repCobros.filas} />
                <Dato k="Monto" v={formatMoneda(repCobros.monto)} />
                <Dato k="Se aplican" v={repCobros.aplicables} tono="positivo" />
                <Dato
                  k="Sin identificar"
                  v={repCobros.sin_identificar}
                  tono={repCobros.sin_identificar > 0 ? "pendiente" : undefined}
                  nota="entran a su bandeja"
                />
                <Dato k="Con período" v={repCobros.con_detalle} nota="del campo Concepto" />
                <Dato
                  k="Cuarentena"
                  v={repCobros.cuarentena}
                  tono={repCobros.cuarentena > 0 ? "pendiente" : undefined}
                />
              </div>

              {repCobros.con_detalle > 0 && (
                <div className="rounded-lg border border-accent/40 bg-accent/5 px-3 py-2 text-xs">
                  <b>{repCobros.con_detalle} de {repCobros.filas}</b> traen el período en el campo
                  «Concepto» del sistema de origen (por ejemplo «M/JULIO» o «1Q/JULIO»). Es el dato que
                  permite saber a qué mes iba cada pago.
                </div>
              )}

              <TableContainer>
                <Table>
                  <THead>
                    <TR>
                      <TH>Línea</TH>
                      <TH>Contrato</TH>
                      <TH>Recibo</TH>
                      <TH className="text-right">Monto</TH>
                      <TH>Fecha bancaria</TH>
                      <TH>Períodos del Concepto</TH>
                      <TH>Canal</TH>
                      <TH>Motivos</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {repCobros.muestra.map((f) => (
                      <TR key={f.linea}>
                        <TD className="tabular-nums text-content-muted">{f.linea}</TD>
                        <TD className="font-medium">{f.contrato || <span className="text-content-muted">—</span>}</TD>
                        <TD className="tabular-nums">{f.consecutivo}</TD>
                        <TD className="text-right tabular-nums">{formatMoneda(f.monto)}</TD>
                        <TD className="tabular-nums">{f.fecha_bancaria ? formatFecha(f.fecha_bancaria) : "—"}</TD>
                        <TD className="text-xs">
                          {(f.aplicaciones ?? []).length > 0
                            ? (f.aplicaciones ?? [])
                                .map((a) => `${a.periodo} ${formatMoneda(a.monto)}${a.parcial ? " (parcial)" : ""}`)
                                .join(" · ")
                            : "—"}
                        </TD>
                        <TD className="max-w-[10rem] text-xs">
                          {f.forma_pago}
                          {f.asociacion && <span className="block text-content-muted">{f.asociacion}</span>}
                        </TD>
                        <TD className="text-xs">
                          {f.motivos?.length ? (
                            <Badge tone="pendiente">{f.motivos.join(" · ")}</Badge>
                          ) : (
                            <span className="text-positivo">ok</span>
                          )}
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </TableContainer>

              <div className="flex flex-wrap items-center gap-3">
                <Button onClick={() => setConfirmandoCobros(true)} loading={confirmarCobros.isPending}>
                  Registrar {repCobros.filas - repCobros.anulados} cobros
                </Button>
                <Button variant="ghost" onClick={() => elegirPagos(null)}>
                  Descartar
                </Button>
                {repCobros.anulados > 0 && (
                  <span className="text-xs text-content-muted">
                    {repCobros.anulados} vienen anulados del origen y no se registran.
                  </span>
                )}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {confirmandoCobros && repCobros && (
        <ConfirmDialog
          titulo={`Registrar ${repCobros.filas - repCobros.anulados} cobros`}
          descripcion={`Por ${formatMoneda(repCobros.monto)} en total.`}
          impacto={[
            `${repCobros.aplicables} se aplican a cargos (más viejo primero)`,
            repCobros.sin_identificar > 0
              ? `${repCobros.sin_identificar} entran a «sin identificar» para resolverlos después`
              : "",
            "Reimportar este mismo archivo no duplica nada: los cobros se reconocen por su consecutivo.",
          ].filter((x) => x !== "")}
          textoConfirmar="Registrar"
          pendiente={confirmarCobros.isPending}
          onConfirmar={aplicarCobros}
          onCancelar={() => setConfirmandoCobros(false)}
        />
      )}

      {confirmando && reporte && (
        <ConfirmDialog
          titulo="Confirmar la importación de cartera"
          descripcion={`Entran ${reporte.nuevos} contratos nuevos y se actualizan ${reporte.actualizados}.`}
          impacto={[
            reporte.cuarentena > 0 ? `${reporte.cuarentena} quedan marcados en revisión y no generan cargos` : null,
            nuevos.length > 0 ? `se crean ${nuevos.length} sede(s)` : null,
            asocNuevas.length > 0 ? `se crean ${asocNuevas.length} asociación(es)` : null,
            reporte.duplicados > 0 ? `${reporte.duplicados} filas duplicadas se ignoran` : null,
          ].filter((x): x is string => x !== null)}
          textoConfirmar="Confirmar"
          pendiente={confirmar.isPending}
          onConfirmar={aplicar}
          onCancelar={() => setConfirmando(false)}
        />
      )}

      {generando && plan && (
        <ConfirmDialog
          titulo={`Generar ${plan.cargos.toLocaleString("es-CR")} cargos`}
          descripcion={`De ${plan.contratos.toLocaleString("es-CR")} contratos, entre ${formatFecha(plan.desde)} y ${formatFecha(plan.hasta)}.`}
          impacto={[
            "Es idempotente: los cargos que ya existan no se duplican.",
            "El saldo de cada contrato se deriva de estos cargos.",
          ]}
          textoConfirmar="Generar"
          pendiente={generar.isPending}
          onConfirmar={() => {
            generar.mutate(
              { desde, hasta, total: plan.cargos },
              {
                onSuccess: (r) => toast.success(`${r.creados.toLocaleString("es-CR")} cargos creados.`),
                onError: (e) => toast.error(mensajeError(e)),
              },
            );
            setGenerando(false);
          }}
          onCancelar={() => setGenerando(false)}
        />
      )}
    </div>
  );
}

function Dato({
  k,
  v,
  nota,
  tono,
}: {
  k: string;
  v: number | string;
  nota?: string;
  tono?: "positivo" | "pendiente";
}) {
  return (
    <div className="rounded-lg border border-border bg-surface px-3 py-2">
      <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">{k}</p>
      <p
        className={
          "text-lg font-semibold tabular-nums " +
          (tono === "positivo" ? "text-positivo" : tono === "pendiente" ? "text-pendiente" : "text-content")
        }
      >
        {typeof v === "number" ? v.toLocaleString("es-CR") : v}
      </p>
      {nota && <p className="text-[10.5px] text-content-muted">{nota}</p>}
    </div>
  );
}
