/**
 * RRHH — Corrida quincenal (/rrhh/corridas). Maqueta aprobada: dos corridas por mes —
 * ADELANTO (día 15, % del salario base, sin deducciones) y LIQUIDACIÓN (día 30: mes
 * completo con CCSS, renta, deducciones con prelación y descuento del adelanto pagado).
 * Ciclo: BORRADOR (novedades/recalcular) → APROBADA (congela) → PAGADA (descuenta saldos).
 */

import { useMemo, useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
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
  type BadgeTone,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { descargarBlob } from "@/lib/csv";
import { formatFecha, formatMoneda, montoLegible, montoParaApi, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { rrhhApi } from "@/api/rrhh";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useAnularCorrida,
  useAprobarCorrida,
  useConceptosNomina,
  useCorrida,
  useCorridas,
  useCrearCorrida,
  useEmpleados,
  useEnviarBoletas,
  useGuardarNovedades,
  usePagarCorrida,
  useRecalcularCorrida,
} from "@/features/rrhh/hooks";
import { ETIQUETA_TRATAMIENTO } from "@/api/rrhh";
import type {
  Corrida,
  CorridaDetalle,
  LineaCorrida,
  NovedadInput,
  ResultadoEnvioBoletas,
  TipoCorrida,
} from "@/api/rrhh";

const ANIO_BASE = 2026;

const MESES = [
  "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
  "Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre",
];

const TONO_ESTADO: Record<string, BadgeTone> = {
  BORRADOR: "pendiente",
  APROBADA: "accent",
  PAGADA: "positivo",
  ANULADA: "neutral",
};

const ETIQUETA_ESTADO: Record<string, string> = {
  BORRADOR: "Borrador",
  APROBADA: "Aprobada",
  PAGADA: "Pagada",
  ANULADA: "Anulada",
};

export function CorridasPage() {
  const tiene = useTienePermiso();
  const puedeCorrer = tiene("rrhh.corrida");
  const [anio, setAnio] = useState(ANIO_BASE);
  const [selId, setSelId] = useState<string | null>(null);
  const [nueva, setNueva] = useState(false);

  const corridasQ = useCorridas(anio);
  const corridas = corridasQ.data ?? [];

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Corrida quincenal"
        description="Dos corridas por mes. Quien tiene jornada quincenal recibe dos salarios reales, cada uno con su CCSS, renta y deducciones; a quien se paga por mes se le adelanta el día 15 y se le liquida el 30."
        actions={
          <div className="flex items-center gap-2">
            <Select
              value={String(anio)}
              onChange={(e) => {
                setAnio(Number(e.target.value));
                setSelId(null);
              }}
              options={[ANIO_BASE - 1, ANIO_BASE, ANIO_BASE + 1].map((a) => ({ value: String(a), label: String(a) }))}
            />
            {puedeCorrer && <Button onClick={() => setNueva(true)}>Nueva corrida</Button>}
          </div>
        }
      />

      {corridasQ.isPending ? (
        <LoadingState label="Cargando corridas" />
      ) : corridasQ.isError ? (
        <ErrorState message={mensajeError(corridasQ.error)} onRetry={() => corridasQ.refetch()} />
      ) : corridas.length === 0 ? (
        <EmptyState
          message={
            puedeCorrer
              ? `No hay corridas en ${anio}. Usá «Nueva corrida» para abrir el adelanto o la liquidación del mes.`
              : `No hay corridas en ${anio}.`
          }
        />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Mes</TH>
                <TH>Corrida</TH>
                <TH>Estado</TH>
                <TH>Fecha de pago</TH>
                <TH className="text-center">Empleados</TH>
                <TH className="text-right">Bruto</TH>
                <TH className="text-right">Neto a depositar</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {corridas.map((c) => (
                <TR key={c.id} className={cn(c.estado === "ANULADA" && "opacity-50", selId === c.id && "bg-accent/5")}>
                  <TD className="font-medium">{MESES[c.mes - 1]}</TD>
                  <TD>{c.tipo === "ADELANTO" ? "1ª quincena · día 15" : "2ª quincena · día 30"}</TD>
                  <TD>
                    <Badge tone={TONO_ESTADO[c.estado]}>{ETIQUETA_ESTADO[c.estado]}</Badge>
                  </TD>
                  <TD className="text-xs">{formatFecha(c.fecha_pago)}</TD>
                  <TD className="text-center">{c.empleados}</TD>
                  <TD className="text-right tabular-nums">{formatMoneda(c.total_bruto, "CRC")}</TD>
                  <TD className="text-right font-semibold tabular-nums">{formatMoneda(c.total_neto, "CRC")}</TD>
                  <TD className="text-right">
                    <Button size="sm" variant="secondary" onClick={() => setSelId(selId === c.id ? null : c.id)}>
                      {selId === c.id ? "Ocultar" : "Abrir"}
                    </Button>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {/* key: al cambiar de corrida el árbol se re-monta — el estado local (filas de
          novedades, colilla abierta) JAMÁS se fuga de una corrida a otra. */}
      {selId && <CorridaCard key={selId} id={selId} puedeCorrer={puedeCorrer} onAnulada={() => setSelId(null)} />}

      {nueva && (
        <NuevaCorridaDialog
          anio={anio}
          existentes={corridas}
          onCerrar={() => setNueva(false)}
          onCreada={(id) => {
            setNueva(false);
            setSelId(id);
          }}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Detalle de la corrida: totales, novedades (liquidación en borrador) y colillas
// ---------------------------------------------------------------------------

function CorridaCard({ id, puedeCorrer, onAnulada }: { id: string; puedeCorrer: boolean; onAnulada: () => void }) {
  const toast = useToast();
  const corridaQ = useCorrida(id);
  const recalcular = useRecalcularCorrida();
  const aprobar = useAprobarCorrida();
  const pagar = usePagarCorrida();
  const anular = useAnularCorrida();
  const [confirmar, setConfirmar] = useState<"aprobar" | "pagar" | "anular" | null>(null);

  if (corridaQ.isPending) return <LoadingState label="Cargando corrida" />;
  if (corridaQ.isError)
    return <ErrorState message={mensajeError(corridaQ.error)} onRetry={() => corridaQ.refetch()} />;
  const c = corridaQ.data;
  const esBorrador = c.estado === "BORRADOR";
  const titulo = `${c.tipo === "ADELANTO" ? "1ª quincena" : "2ª quincena"} · ${MESES[c.mes - 1]} ${c.anio}`;

  const onError = (err: unknown) => toast.error(mensajeError(err));

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardContent className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-content">{titulo}</h3>
              <Badge tone={TONO_ESTADO[c.estado]}>{ETIQUETA_ESTADO[c.estado]}</Badge>
              <span className="text-xs text-content-muted">Pago: {formatFecha(c.fecha_pago)}</span>
            </div>
            {puedeCorrer && (
              <div className="flex gap-2">
                {esBorrador && (
                  <>
                    <Button
                      size="sm"
                      variant="secondary"
                      loading={recalcular.isPending}
                      onClick={() => recalcular.mutate(c.id, { onError })}
                    >
                      Recalcular
                    </Button>
                    <Button size="sm" onClick={() => setConfirmar("aprobar")}>
                      Aprobar ✓
                    </Button>
                  </>
                )}
                {c.estado === "APROBADA" && (
                  <Button size="sm" onClick={() => setConfirmar("pagar")}>
                    Marcar pagada
                  </Button>
                )}
                {(esBorrador || c.estado === "APROBADA") && (
                  <Button size="sm" variant="ghost" className="text-negativo" onClick={() => setConfirmar("anular")}>
                    Anular
                  </Button>
                )}
              </div>
            )}
          </div>

          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-7">
            <Kpi label="Bruto" valor={c.total_bruto} />
            <Kpi label="CCSS obrero" valor={c.total_ccss_obrero} negativo />
            <Kpi label="Renta" valor={c.total_renta} negativo />
            <Kpi label="Deducciones" valor={c.total_deducciones} negativo />
            <Kpi label="Adelanto" valor={c.total_adelanto} negativo />
            <Kpi label="Neto a depositar" valor={c.total_neto} destacado />
            <Kpi label="Costo patronal" valor={c.total_patronal} />
          </div>
          {c.tipo === "LIQUIDACION" && (
            <p className="text-xs text-content-muted">
              Provisiones del mes (aguinaldo + vacaciones + cesantía): {formatMoneda(c.total_provisiones, "CRC")} —
              informativas, se asientan en la Etapa 3.
            </p>
          )}
        </CardContent>
      </Card>

      {c.tipo === "LIQUIDACION" && esBorrador && puedeCorrer && <NovedadesCard key={c.id} corrida={c} />}

      {(c.estado === "APROBADA" || c.estado === "PAGADA") && <PagosCard corrida={c} puedeCorrer={puedeCorrer} />}

      <ColillasCard lineas={c.lineas} />

      {confirmar === "aprobar" && (
        <ConfirmDialog
          titulo="Aprobar corrida"
          descripcion={`${titulo}: se congelan las colillas y los parámetros usados.`}
          impacto={[
            `${c.empleados} empleados`,
            `Neto a depositar ${formatMoneda(c.total_neto, "CRC")}`,
            "Ya no se podrán capturar novedades ni recalcular",
          ]}
          textoConfirmar="Aprobar corrida"
          pendiente={aprobar.isPending}
          onConfirmar={() => {
            aprobar.mutate(c.id, {
              onSuccess: () => toast.success("Corrida aprobada."),
              onError,
            });
            setConfirmar(null);
          }}
          onCancelar={() => setConfirmar(null)}
        />
      )}
      {confirmar === "pagar" && (
        <ConfirmDialog
          titulo="Marcar corrida pagada"
          descripcion={`${titulo}: confirmá cuando los depósitos estén hechos.`}
          impacto={[
            `Neto depositado ${formatMoneda(c.total_neto, "CRC")}`,
            "Se descuentan los saldos de préstamos y deducciones aplicadas",
            "Este paso no se puede deshacer",
          ]}
          textoConfirmar="Marcar pagada"
          pendiente={pagar.isPending}
          onConfirmar={() => {
            pagar.mutate(c.id, {
              onSuccess: () => toast.success("Corrida pagada. Saldos de deducciones actualizados."),
              onError,
            });
            setConfirmar(null);
          }}
          onCancelar={() => setConfirmar(null)}
        />
      )}
      {confirmar === "anular" && (
        <ConfirmDialog
          titulo="Anular corrida"
          descripcion={`${titulo} queda anulada (el histórico se conserva) y el mes se puede rehacer.`}
          textoConfirmar="Anular"
          tono="peligro"
          pendiente={anular.isPending}
          onConfirmar={() => {
            anular.mutate(c.id, {
              onSuccess: () => {
                toast.success("Corrida anulada.");
                onAnulada();
              },
              onError,
            });
            setConfirmar(null);
          }}
          onCancelar={() => setConfirmar(null)}
        />
      )}
    </div>
  );
}

function Kpi({ label, valor, negativo, destacado }: { label: string; valor: string; negativo?: boolean; destacado?: boolean }) {
  return (
    <div className={cn("rounded-lg border border-border px-3 py-2", destacado && "border-accent/40 bg-accent/5")}>
      <div className="text-[11px] uppercase tracking-wide text-content-muted">{label}</div>
      <div className={cn("text-sm font-semibold tabular-nums", negativo && "text-negativo", destacado && "text-accent")}>
        {negativo && "−"}
        {formatMoneda(valor, "CRC")}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pagos SINPE y planilla CCSS (maqueta "Reportes y Pagos SINPE"): el archivo cuadra
// 1:1 con el neto de la corrida congelada; sin IBAN la línea se bloquea.
// ---------------------------------------------------------------------------

function PagosCard({ corrida, puedeCorrer }: { corrida: CorridaDetalle; puedeCorrer: boolean }) {
  const toast = useToast();
  const [descargando, setDescargando] = useState<"sinpe" | "ccss" | null>(null);
  // Envío de boletas: la boleta va al correo de la ficha de cada colaborador. El texto se
  // configura en Configuración → Notificaciones.
  const [confirmandoBoletas, setConfirmandoBoletas] = useState(false);
  const [envio, setEnvio] = useState<ResultadoEnvioBoletas | null>(null);
  const enviarBoletas = useEnviarBoletas();

  const conCorreo = corrida.lineas.length;
  const pagables = corrida.lineas.filter((l) => l.iban && toNumber(l.neto) > 0);
  const bloqueados = corrida.lineas.filter((l) => !l.iban && toNumber(l.neto) > 0);
  const netoPagable = pagables.reduce((s, l) => s + toNumber(l.neto), 0);

  async function descargar(tipo: "sinpe" | "ccss") {
    setDescargando(tipo);
    try {
      // El nombre viene del backend (lleva tipo de corrida y consecutivo de bitácora):
      // renombrarlo aquí rompería la trazabilidad del pago.
      const descarga =
        tipo === "sinpe" ? await rrhhApi.archivoPagoXLSX(corrida.id) : await rrhhApi.planillaCCSSXLSX(corrida.id);
      const respaldo =
        tipo === "sinpe"
          ? `nomina-sinpe-${corrida.anio}-${String(corrida.mes).padStart(2, "0")}-${corrida.tipo}.xlsx`
          : `planilla-ccss-${corrida.anio}-${String(corrida.mes).padStart(2, "0")}.xlsx`;
      descargarBlob(descarga.filename || respaldo, descarga.blob);
      toast.success(
        tipo === "sinpe"
          ? "Archivo de pago generado (queda en bitácora con su consecutivo)."
          : "Planilla CCSS generada.",
      );
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setDescargando(null);
    }
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h3 className="text-sm font-semibold text-content">Pagos SINPE y planilla</h3>
            <p className="text-xs text-content-muted">
              El archivo cuadra 1:1 con el neto de la corrida: cada línea nace de una colilla. Sin IBAN, la línea se bloquea.
            </p>
          </div>
          <div className="flex gap-2">
            {corrida.tipo === "LIQUIDACION" && (
              <Button size="sm" variant="secondary" loading={descargando === "ccss"} onClick={() => descargar("ccss")}>
                Planilla CCSS (.xlsx)
              </Button>
            )}
            {puedeCorrer && (
              <Button size="sm" loading={descargando === "sinpe"} disabled={pagables.length === 0} onClick={() => descargar("sinpe")}>
                Archivo SINPE (.xlsx)
              </Button>
            )}
            {puedeCorrer && (
              <Button size="sm" variant="secondary" onClick={() => setConfirmandoBoletas(true)}>
                Enviar boletas
              </Button>
            )}
          </div>
        </div>

        {envio && (
          <div className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs">
            <p className="font-medium text-content">
              {envio.enviados === 0
                ? "No se envió ninguna boleta."
                : `Se enviaron ${envio.enviados} boleta${envio.enviados === 1 ? "" : "s"}.`}
            </p>
            {envio.sin_correo.length > 0 && (
              <p className="mt-0.5 text-pendiente">
                Sin correo en su ficha ({envio.sin_correo.length}): {envio.sin_correo.join(", ")}
              </p>
            )}
            {envio.fallidos.length > 0 && (
              <p className="mt-0.5 text-negativo">
                No se pudo entregar ({envio.fallidos.length}): {envio.fallidos.join(", ")}
              </p>
            )}
          </div>
        )}
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <div className="rounded-lg border border-border px-3 py-2">
            <div className="text-[11px] uppercase tracking-wide text-content-muted">Registros pagables</div>
            <div className="text-sm font-semibold tabular-nums">{pagables.length}</div>
          </div>
          <div className="rounded-lg border border-border px-3 py-2">
            <div className="text-[11px] uppercase tracking-wide text-content-muted">Neto pagable (con IBAN)</div>
            <div className="text-sm font-semibold tabular-nums">{formatMoneda(String(netoPagable), "CRC")}</div>
          </div>
          <div className={cn("rounded-lg border px-3 py-2", bloqueados.length > 0 ? "border-negativo/40 bg-negativo/5" : "border-border")}>
            <div className="text-[11px] uppercase tracking-wide text-content-muted">Bloqueados sin IBAN</div>
            <div className={cn("text-sm font-semibold tabular-nums", bloqueados.length > 0 && "text-negativo")}>
              {bloqueados.length}
              {bloqueados.length > 0 && (
                <span className="ml-2 text-xs font-normal">{bloqueados.map((l) => l.nombre).join(", ")}</span>
              )}
            </div>
          </div>
        </div>
      </CardContent>

      {confirmandoBoletas && (
        <ConfirmDialog
          titulo="Enviar las boletas de pago"
          descripcion={`Se le manda a cada colaborador de la corrida el detalle de su pago, al correo que tiene en su ficha.`}
          impacto={[
            `${conCorreo} colaborador(es) en la corrida`,
            "Quien no tenga correo en su ficha NO recibe nada y se reporta",
            "El texto se configura en Configuración → Notificaciones",
          ]}
          textoConfirmar="Enviar boletas"
          pendiente={enviarBoletas.isPending}
          onConfirmar={() => {
            setConfirmandoBoletas(false);
            enviarBoletas.mutate(corrida.id, {
              onSuccess: (r) => {
                setEnvio(r);
                toast.success(
                  r.enviados === 0
                    ? "No se envió ninguna boleta; revisá el detalle."
                    : `Se enviaron ${r.enviados} boleta${r.enviados === 1 ? "" : "s"}.`,
                );
              },
              onError: (err) => toast.error(mensajeError(err)),
            });
          }}
          onCancelar={() => setConfirmandoBoletas(false)}
        />
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Novedades del mes (comisiones, extras, bonos, viáticos…) — reemplaza el set y recalcula
// ---------------------------------------------------------------------------

function NovedadesCard({ corrida }: { corrida: CorridaDetalle }) {
  const toast = useToast();
  const guardar = useGuardarNovedades();
  const empleadosQ = useEmpleados({ estado: "activo" });
  const conceptosQ = useConceptosNomina();

  const [filas, setFilas] = useState<NovedadInput[]>(
    corrida.novedades.map((n) => ({
      empleado_id: n.empleado_id,
      concepto_id: n.concepto_id,
      monto: montoLegible(n.monto),
      // Las novedades por HORAS conservan sus horas; su monto lo calcula el sistema.
      cantidad: toNumber(n.cantidad) > 0 ? String(toNumber(n.cantidad)) : "",
    })),
  );

  // Los conceptos que se pagan por HORAS: el sistema calcula el monto (art. 139).
  // Se pregunta por la BANDERA del concepto, no por su nombre. Antes esto era /hora/i sobre el
  // nombre: renombrar «Horas extra» a «Tiempo extraordinario» hacía que el campo pidiera un monto
  // en vez de horas, y el pago salía sin el recargo del art. 139 sin que nada fallara.
  const esPorHoras = (conceptoId: string) =>
    (conceptosQ.data ?? []).find((c) => c.id === conceptoId)?.por_horas === true;

  const empleados = empleadosQ.data ?? [];
  const conceptosIngreso = (conceptosQ.data ?? []).filter((c) => c.tipo === "INGRESO" && c.activo && c.nombre !== "Salario ordinario" && c.nombre !== "Aguinaldo");

  function setFila(i: number, cambio: Partial<NovedadInput>) {
    setFilas((prev) => prev.map((f, j) => (j === i ? { ...f, ...cambio } : f)));
  }

  function onGuardar(e: FormEvent) {
    e.preventDefault();
    const novedades: NovedadInput[] = [];
    for (const f of filas) {
      if (!f.empleado_id && !f.concepto_id && !f.monto && !f.cantidad) continue; // fila vacía
      if (!f.empleado_id || !f.concepto_id) {
        toast.error("Cada novedad necesita empleado y concepto.");
        return;
      }
      if (esPorHoras(f.concepto_id)) {
        const horas = Number((f.cantidad ?? "").replace(",", "."));
        if (!horas || horas <= 0) {
          toast.error("Las horas extra se registran por HORAS: indicá cuántas.");
          return;
        }
        // El monto lo calcula el sistema con el salario vigente: acá no se manda.
        novedades.push({ empleado_id: f.empleado_id, concepto_id: f.concepto_id, monto: "", cantidad: String(horas) });
        continue;
      }
      const monto = montoParaApi(f.monto);
      if (!monto || Number(monto) <= 0) {
        toast.error("Cada novedad necesita un monto mayor a cero.");
        return;
      }
      novedades.push({ empleado_id: f.empleado_id, concepto_id: f.concepto_id, monto });
    }
    guardar.mutate(
      { id: corrida.id, novedades },
      {
        onSuccess: () => toast.success("Novedades guardadas y corrida recalculada."),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold text-content">Horas extra, comisiones y bonos del período</h3>
          <span className="max-w-xl text-xs text-content-muted">
            Acá se registra lo que se le suma al salario ESTE período. Las <b>horas extra</b> se
            anotan en HORAS y el pago lo calcula el sistema (horas × valor de la hora × 1,5 como
            mínimo, art. 139 CT): elegí el concepto «Horas extra» y el campo cambia a horas solo.
            Comisiones y bonos habituales SON salario y entran a la base de la CCSS por ley.
            <br />
            Las <b>vacaciones</b> y las <b>incapacidades</b> NO se registran acá: van en
            «Vacaciones e incapacidades» del menú, y desde ahí entran solas a la corrida.
            <br />
            Guardar reemplaza todas las novedades del período y recalcula la corrida.
          </span>
        </div>
        <form onSubmit={onGuardar} className="flex flex-col gap-2">
          {filas.map((f, i) => (
            <div key={i} className="grid grid-cols-1 items-end gap-2 sm:grid-cols-[2fr,2fr,1fr,auto]">
              <Select
                label={i === 0 ? "Empleado" : undefined}
                placeholder="Seleccioná…"
                value={f.empleado_id}
                onChange={(e) => setFila(i, { empleado_id: e.target.value })}
                options={empleados.map((emp) => ({ value: emp.id, label: emp.nombre }))}
              />
              <Select
                label={i === 0 ? "Concepto" : undefined}
                placeholder="Seleccioná…"
                value={f.concepto_id}
                onChange={(e) => setFila(i, { concepto_id: e.target.value })}
                options={conceptosIngreso.map((con) => ({ value: con.id, label: con.nombre }))}
              />
              {esPorHoras(f.concepto_id) ? (
                <Input
                  label={i === 0 ? "Horas" : undefined}
                  value={f.cantidad ?? ""}
                  onChange={(e) => setFila(i, { cantidad: e.target.value })}
                  inputMode="decimal"
                  placeholder="6"
                  title="El pago lo calcula el sistema: horas × valor de la hora × 1,5 (art. 139 CT)"
                  className="text-right tabular-nums"
                />
              ) : (
                <Input
                  label={i === 0 ? "Monto del mes" : undefined}
                  value={f.monto}
                  onChange={(e) => setFila(i, { monto: e.target.value })}
                  onBlur={() => setFila(i, { monto: montoLegible(f.monto) })}
                  inputMode="decimal"
                  placeholder="420 000,00"
                  className="text-right tabular-nums"
                />
              )}
              <Button type="button" variant="ghost" className="text-negativo" onClick={() => setFilas((prev) => prev.filter((_, j) => j !== i))}>
                Quitar
              </Button>
            </div>
          ))}
          <div className="flex justify-between pt-1">
            <Button type="button" variant="secondary" size="sm" onClick={() => setFilas((prev) => [...prev, { empleado_id: "", concepto_id: "", monto: "", cantidad: "" }])}>
              + Agregar novedad
            </Button>
            <Button type="submit" loading={guardar.isPending}>
              Guardar novedades y recalcular
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Colillas por empleado (detalle expandible renglón a renglón)
// ---------------------------------------------------------------------------

function ColillasCard({ lineas }: { lineas: LineaCorrida[] }) {
  const [abierta, setAbierta] = useState<string | null>(null);
  const sel = useMemo(() => lineas.find((l) => l.id === abierta) ?? null, [lineas, abierta]);
  // Las columnas de retenciones se muestran si alguna colilla las tiene: en la misma
  // corrida pueden convivir pagos quincenales reales y adelantos sin deducciones.
  const hayRetenciones = lineas.some((l) => l.tratamiento !== "ADELANTO");
  const hayAdelantos = lineas.some((l) => toNumber(l.adelanto) > 0);

  return (
    <Card>
      <CardContent className="flex flex-col gap-3">
        <h3 className="text-sm font-semibold text-content">Colillas por empleado</h3>
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Empleado</TH>
                <TH>Se le paga</TH>
                <TH className="text-right">Bruto</TH>
                {hayRetenciones && (
                  <>
                    <TH className="text-right">CCSS obrero</TH>
                    <TH className="text-right">Renta</TH>
                    <TH className="text-right">Deducc.</TH>
                  </>
                )}
                {hayAdelantos && <TH className="text-right">Adelanto</TH>}
                <TH className="text-right">Neto</TH>
                <TH className="text-right">Colilla</TH>
              </TR>
            </THead>
            <TBody>
              {lineas.map((l) => (
                <TR key={l.id} className={cn(abierta === l.id && "bg-accent/5")}>
                  <TD>
                    <div className="font-medium">{l.nombre}</div>
                    <div className="text-xs text-content-muted">
                      {l.puesto || l.identificacion}
                      {l.departamento && ` · ${l.departamento}`}
                    </div>
                  </TD>
                  <TD>
                    <Badge tone={l.tratamiento === "ADELANTO" ? "pendiente" : "accent"}>
                      {ETIQUETA_TRATAMIENTO[l.tratamiento]}
                    </Badge>
                  </TD>
                  <TD className="text-right tabular-nums">{formatMoneda(l.bruto, "CRC")}</TD>
                  {hayRetenciones && (
                    <>
                      <TD className="text-right tabular-nums text-content-muted">{formatMoneda(l.ccss_obrero, "CRC")}</TD>
                      <TD className="text-right tabular-nums text-content-muted">{formatMoneda(l.renta, "CRC")}</TD>
                      <TD className="text-right tabular-nums text-content-muted">{formatMoneda(l.deducciones, "CRC")}</TD>
                    </>
                  )}
                  {hayAdelantos && (
                    <TD className="text-right tabular-nums text-content-muted">{formatMoneda(l.adelanto, "CRC")}</TD>
                  )}
                  <TD className="text-right font-semibold tabular-nums">{formatMoneda(l.neto, "CRC")}</TD>
                  <TD className="text-right">
                    <Button size="sm" variant="ghost" onClick={() => setAbierta(abierta === l.id ? null : l.id)}>
                      {abierta === l.id ? "Cerrar" : "Ver"}
                    </Button>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>

        {sel && <Colilla linea={sel} />}
      </CardContent>
    </Card>
  );
}

const ETIQUETA_TIPO_DETALLE: Record<string, string> = {
  INGRESO: "Ingresos",
  CCSS: "CCSS del trabajador",
  RENTA: "Impuesto al salario",
  DEDUCCION: "Deducciones",
  ADELANTO: "Adelanto",
  PATRONAL: "Cargas patronales (costo de la empresa)",
  PROVISION: "Provisiones",
};

const ORDEN_TIPOS = ["INGRESO", "CCSS", "RENTA", "DEDUCCION", "ADELANTO", "PATRONAL", "PROVISION"];

function Colilla({ linea }: { linea: LineaCorrida }) {
  const grupos = useMemo(() => {
    const porTipo = new Map<string, typeof linea.detalle>();
    for (const d of linea.detalle) {
      const lista = porTipo.get(d.tipo) ?? [];
      lista.push(d);
      porTipo.set(d.tipo, lista);
    }
    return ORDEN_TIPOS.filter((t) => porTipo.has(t)).map((t) => ({ tipo: t, items: porTipo.get(t)! }));
  }, [linea]);

  const resta = (t: string) => t === "CCSS" || t === "RENTA" || t === "DEDUCCION" || t === "ADELANTO";

  return (
    <div className="rounded-lg border border-border bg-surface-muted p-4">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-sm font-semibold text-content">Colilla — {linea.nombre}</div>
          <div className="text-xs text-content-muted">
            Base CCSS {formatMoneda(linea.base_ccss, "CRC")} · Base renta {formatMoneda(linea.base_renta, "CRC")}
            {linea.hijos > 0 && ` · ${linea.hijos} hijo${linea.hijos === 1 ? "" : "s"}`}
            {linea.conyuge && " · cónyuge"}
            {linea.iban && ` · ${linea.iban}`}
          </div>
        </div>
        <div className="text-right">
          <div className="text-[11px] uppercase tracking-wide text-content-muted">Neto a depositar</div>
          <div className="text-base font-bold tabular-nums text-accent">{formatMoneda(linea.neto, "CRC")}</div>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-x-8 gap-y-1 lg:grid-cols-2">
        {grupos.map((g) => (
          <div key={g.tipo}>
            <div className="mt-2 text-[11px] font-semibold uppercase tracking-wide text-content-muted">
              {ETIQUETA_TIPO_DETALLE[g.tipo] ?? g.tipo}
            </div>
            {g.items.map((d, i) => (
              <div key={i} className="flex items-center justify-between border-b border-dashed border-border py-1 text-sm">
                <span className="text-content">{d.nombre}</span>
                <span className={cn("tabular-nums", resta(d.tipo) ? "text-negativo" : "text-content")}>
                  {resta(d.tipo) && "−"}
                  {formatMoneda(d.monto, "CRC")}
                </span>
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Nueva corrida (ADELANTO / LIQUIDACION del mes)
// ---------------------------------------------------------------------------

function NuevaCorridaDialog({
  anio,
  existentes,
  onCerrar,
  onCreada,
}: {
  anio: number;
  existentes: Corrida[];
  onCerrar: () => void;
  onCreada: (id: string) => void;
}) {
  const toast = useToast();
  const crear = useCrearCorrida();
  const [mes, setMes] = useState(new Date().getMonth() + 1);
  const [tipo, setTipo] = useState<TipoCorrida>("ADELANTO");
  const [fecha, setFecha] = useState("");

  const fechaSugerida = useMemo(() => {
    if (tipo === "ADELANTO") return `${anio}-${String(mes).padStart(2, "0")}-15`;
    const ultimo = new Date(anio, mes, 0).getDate();
    return `${anio}-${String(mes).padStart(2, "0")}-${ultimo}`;
  }, [anio, mes, tipo]);

  const yaExiste = existentes.some((c) => c.mes === mes && c.tipo === tipo && c.estado !== "ANULADA");

  function onCrear(e: FormEvent) {
    e.preventDefault();
    crear.mutate(
      { anio, mes, tipo, fecha_pago: fecha || fechaSugerida },
      {
        onSuccess: (det) => {
          toast.success(`Corrida calculada: ${det.empleados} empleados, neto ${formatMoneda(det.total_neto, "CRC")}.`);
          onCreada(det.id);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onCerrar}>
      <div
        className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-content">Nueva corrida — {anio}</h2>
        <form onSubmit={onCrear} className="mt-4 flex flex-col gap-3">
          <Select
            label="Mes"
            value={String(mes)}
            onChange={(e) => setMes(Number(e.target.value))}
            options={MESES.map((m, i) => ({ value: String(i + 1), label: m }))}
          />
          <Select
            label="Corrida"
            value={tipo}
            onChange={(e) => setTipo(e.target.value as TipoCorrida)}
            options={[
              { value: "ADELANTO", label: "1ª quincena (día 15)" },
              { value: "LIQUIDACION", label: "2ª quincena (día 30)" },
            ]}
          />
          <Input
            label="Fecha de pago"
            type="date"
            value={fecha || fechaSugerida}
            onChange={(e) => setFecha(e.target.value)}
          />
          {yaExiste && (
            <p className="rounded-lg border border-pendiente/30 bg-pendiente/10 px-3 py-2 text-xs text-content">
              ⚠️ Ya existe una corrida viva de ese tipo para {MESES[mes - 1]}. Anulala primero si querés rehacerla.
            </p>
          )}
          <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-content-muted">
            {tipo === "ADELANTO"
              ? "El adelanto paga el % configurado del salario base, sin deducciones; se descuenta completo en la liquidación."
              : "La liquidación calcula el mes completo (CCSS, renta y deducciones con prelación) y descuenta el adelanto ya aprobado o pagado del mes."}
          </p>
          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="ghost" onClick={onCerrar}>
              Cancelar
            </Button>
            <Button type="submit" loading={crear.isPending} disabled={yaExiste}>
              Crear y calcular
            </Button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
