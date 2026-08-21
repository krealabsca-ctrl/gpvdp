/**
 * CxP — Caja chica (/cxp/cajas). Sistema de fondo fijo (maqueta aprobada 2026-07-27):
 * cada caja es un fondo con custodio; los gastos menores son VALES contra el fondo
 * (comprobante electrónico o recibo manual); la REPOSICIÓN (documento Reintegro al
 * custodio) es lo único que viaja por CxP y, al pagarse, restaura el fondo.
 */

import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { createPortal } from "react-dom";
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
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda, montoLegible, montoParaApi, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { GastoCombobox, type GastoElegido } from "@/features/cxp/components/GastoCombobox";
import {
  useActualizarFondo,
  useAnularVale,
  useCrearFondo,
  useCrearVale,
  useDepartamentos,
  useDesactivarFondo,
  useFondosCaja,
  useGenerarReposicion,
  useTodosProveedores,
  useUsuariosEmpresa,
  useValesCaja,
} from "@/features/cxp/hooks";
import type { FondoCajaChica, FondoInput } from "@/api/cxp";

export function CajaChicaPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const tiene = useTienePermiso();
  const fondosQ = useFondosCaja();
  const generar = useGenerarReposicion();
  const desactivar = useDesactivarFondo();

  const [selId, setSelId] = useState<string | null>(null);
  const [editar, setEditar] = useState<FondoCajaChica | "nuevo" | null>(null);

  const puedeAdministrar = tiene("cxp.caja_administrar");
  const puedeVale = tiene("cxp.caja_vale");
  const puedeReponer = tiene("cxp.caja_reponer");

  const fondos = fondosQ.data ?? [];
  const sel = fondos.find((f) => f.id === selId) ?? null;
  const bajoUmbral = fondos.filter((f) => f.activo && nivelPct(f) <= toNumber(f.umbral_pct)).length;

  function reponer(f: FondoCajaChica) {
    generar.mutate(f.id, {
      onSuccess: (doc) => {
        toast.success(`Reposición creada (${f.vales_pendientes} vales). Sigue el flujo normal de CxP.`);
        navigate(`/cxp/documentos/${doc.id}`);
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Caja chica"
        description="Fondos fijos con custodio. Los gastos menores se registran como vales; la reposición es lo único que entra al flujo de pagos y restaura el fondo."
        actions={puedeAdministrar ? <Button onClick={() => setEditar("nuevo")}>Nuevo fondo</Button> : undefined}
      />

      {fondosQ.isPending ? (
        <LoadingState label="Cargando fondos" />
      ) : fondosQ.isError ? (
        <ErrorState message={mensajeError(fondosQ.error)} onRetry={() => fondosQ.refetch()} />
      ) : fondos.length === 0 ? (
        <EmptyState
          message={
            puedeAdministrar
              ? "No hay fondos todavía. Usá «Nuevo fondo» para constituir la primera caja (monto fijo, custodio, umbral y límite por vale)."
              : "No tenés fondos de caja chica asignados."
          }
        />
      ) : (
        <>
          {bajoUmbral > 0 && (
            <div className="flex items-center gap-2 rounded-lg border border-pendiente/30 bg-pendiente/10 px-3 py-2 text-sm text-content">
              ⏳ <b>{bajoUmbral} fondo{bajoUmbral === 1 ? "" : "s"} bajo el umbral</b> — generá la reposición para restaurarlo{bajoUmbral === 1 ? "" : "s"}.
            </div>
          )}
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Fondo</TH>
                  <TH>Custodio</TH>
                  <TH>Departamento</TH>
                  <TH className="text-right">Monto fijo</TH>
                  <TH className="text-right">En vales</TH>
                  <TH className="text-right">Disponible</TH>
                  <TH>Nivel</TH>
                  <TH className="text-right">Acción</TH>
                </TR>
              </THead>
              <TBody>
                {fondos.map((f) => {
                  const pct = nivelPct(f);
                  const alerta = pct <= toNumber(f.umbral_pct);
                  return (
                    <TR key={f.id} className={cn(!f.activo && "opacity-50", selId === f.id && "bg-accent/5")}>
                      <TD className="font-medium">
                        {f.nombre}
                        {!f.activo && <span className="ml-2 text-[10px] uppercase text-content-muted">inactivo</span>}
                      </TD>
                      <TD>{f.custodio || "—"}</TD>
                      <TD>{f.departamento || "—"}</TD>
                      <TD className="text-right tabular-nums">{formatMoneda(f.monto_asignado, "CRC")}</TD>
                      <TD className="text-right tabular-nums text-content-muted">{formatMoneda(f.en_vales, "CRC")}</TD>
                      <TD className="text-right font-semibold tabular-nums">{formatMoneda(f.disponible, "CRC")}</TD>
                      <TD>
                        <Nivel pct={pct} alerta={alerta} critico={pct <= 15} />
                      </TD>
                      <TD className="text-right">
                        <div className="flex justify-end gap-2">
                          {puedeReponer && f.activo && (
                            <Button
                              size="sm"
                              disabled={f.vales_pendientes === 0}
                              title={f.vales_pendientes === 0 ? "No hay vales pendientes de reponer" : `${f.vales_pendientes} vales por ${formatMoneda(f.monto_pendiente, "CRC")}`}
                              onClick={() => reponer(f)}
                              loading={generar.isPending}
                            >
                              Generar reposición
                            </Button>
                          )}
                          <Button size="sm" variant="secondary" onClick={() => setSelId(selId === f.id ? null : f.id)}>
                            {selId === f.id ? "Ocultar vales" : "Vales"}
                          </Button>
                          {puedeAdministrar && (
                            <Button size="sm" variant="ghost" onClick={() => setEditar(f)}>
                              Editar
                            </Button>
                          )}
                        </div>
                      </TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          </TableContainer>
        </>
      )}

      {sel && <ValesCard fondo={sel} puedeVale={puedeVale} />}

      {editar && (
        <FondoDialog
          fondo={editar === "nuevo" ? null : editar}
          onCerrar={() => setEditar(null)}
          onDesactivar={
            editar !== "nuevo" && editar.activo
              ? () =>
                  desactivar.mutate(editar.id, {
                    onSuccess: () => {
                      toast.success("Fondo desactivado (el histórico de vales se conserva).");
                      setEditar(null);
                    },
                    onError: (e) => toast.error(mensajeError(e)),
                  })
              : undefined
          }
        />
      )}
    </div>
  );
}

/** % disponible del fondo (para el semáforo). */
function nivelPct(f: FondoCajaChica): number {
  const m = toNumber(f.monto_asignado);
  return m > 0 ? Math.round((toNumber(f.disponible) / m) * 100) : 0;
}

function Nivel({ pct, alerta, critico }: { pct: number; alerta: boolean; critico: boolean }) {
  const color = critico ? "bg-negativo" : alerta ? "bg-brand-gold" : "bg-accent";
  return (
    <div className="flex items-center gap-2">
      <span className="relative inline-block h-2 w-24 overflow-hidden rounded-full bg-surface-muted">
        <span className={cn("absolute inset-y-0 left-0 rounded-full", color)} style={{ width: `${Math.max(4, pct)}%` }} />
      </span>
      <span className="text-[11px] tabular-nums text-content-muted">{pct}%</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Vales del fondo seleccionado: registro con comprobante y límite bloqueante
// ---------------------------------------------------------------------------

const ETIQUETA_VALE: Record<string, { label: string; tone: "neutral" | "pendiente" | "accent" | "positivo" | "negativo" }> = {
  PENDIENTE: { label: "Pendiente", tone: "pendiente" },
  EN_REPOSICION: { label: "En reposición", tone: "accent" },
  REPUESTO: { label: "Repuesto", tone: "positivo" },
  ANULADO: { label: "Anulado", tone: "negativo" },
};

function ValesCard({ fondo, puedeVale }: { fondo: FondoCajaChica; puedeVale: boolean }) {
  const toast = useToast();
  const valesQ = useValesCaja(fondo.id);
  const crear = useCrearVale();
  const anular = useAnularVale();

  const [detalle, setDetalle] = useState("");
  const [gasto, setGasto] = useState<GastoElegido | null>(null);
  const [comprobante, setComprobante] = useState<"FE" | "RECIBO">("FE");
  const [monto, setMonto] = useState("");

  const limite = toNumber(fondo.limite_vale);
  const vales = valesQ.data ?? [];

  function onRegistrar(e: FormEvent) {
    e.preventDefault();
    const m = montoParaApi(monto);
    if (!detalle.trim()) return toast.error("Indicá el detalle del gasto.");
    if (!gasto) return toast.error("Clasificá el gasto del vale.");
    if (!m || Number(m) <= 0) return toast.error("Indicá el monto del vale.");
    if (limite > 0 && Number(m) > limite)
      return toast.error(`El vale supera el límite del fondo (${formatMoneda(fondo.limite_vale, "CRC")}) — esa compra va por CxP normal con factura.`);
    if (Number(m) > toNumber(fondo.disponible))
      return toast.error(`El fondo no alcanza (disponible ${formatMoneda(fondo.disponible, "CRC")}) — generá la reposición.`);
    crear.mutate(
      {
        fondoId: fondo.id,
        input: {
          detalle: detalle.trim(),
          monto_crc: m,
          concepto_id: gasto.conceptoId,
          clasificacion_id: gasto.clasificacionId,
          subclasificacion_id: gasto.subclasificacionId || undefined,
          comprobante,
        },
      },
      {
        onSuccess: () => {
          toast.success(comprobante === "FE" ? "Vale registrado." : "Vale registrado (recibo manual: no deducible).");
          setDetalle("");
          setGasto(null);
          setMonto("");
        },
        onError: (e2) => toast.error(mensajeError(e2)),
      },
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 py-4">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="text-sm font-semibold text-content">Vales — {fondo.nombre}</h2>
          <p className="text-xs text-content-muted">
            💵 Efectivo <b className="tabular-nums text-content">{formatMoneda(fondo.disponible, "CRC")}</b> + 🧾 vales{" "}
            <b className="tabular-nums text-content">{formatMoneda(fondo.en_vales, "CRC")}</b> = fondo{" "}
            <b className="tabular-nums text-content">{formatMoneda(fondo.monto_asignado, "CRC")}</b>
            {limite > 0 && <> · límite por vale {formatMoneda(fondo.limite_vale, "CRC")}</>}
          </p>
        </div>

        {puedeVale && (
          <form onSubmit={onRegistrar} className="flex flex-wrap items-end gap-3 rounded-lg border border-dashed border-border px-3 py-3">
            <div className="min-w-[220px] flex-1">
              <Input label="Detalle *" value={detalle} onChange={(e) => setDetalle(e.target.value)} placeholder="Ej. taxi trámite municipalidad" />
            </div>
            <div className="flex flex-col gap-1">
              <span className="text-xs font-medium text-content-muted">Gasto *</span>
              <GastoCombobox actual={gasto?.ruta ?? ""} onElegir={setGasto} />
            </div>
            <div className="w-56">
              <Select
                label="Comprobante *"
                value={comprobante}
                onChange={(e) => setComprobante(e.target.value as "FE" | "RECIBO")}
                options={[
                  { value: "FE", label: "Factura electrónica (deducible)" },
                  { value: "RECIBO", label: "Recibo manual (no deducible)" },
                ]}
              />
            </div>
            <div className="w-36">
              <Input
                label="Monto *"
                value={monto}
                onChange={(e) => setMonto(e.target.value)}
                onBlur={() => setMonto(montoLegible(monto))}
                inputMode="decimal"
                placeholder="0,00"
                className="text-right tabular-nums"
              />
            </div>
            <Button type="submit" loading={crear.isPending}>
              Registrar vale
            </Button>
          </form>
        )}

        {valesQ.isPending ? (
          <LoadingState label="Cargando vales" />
        ) : vales.length === 0 ? (
          <p className="text-sm text-content-muted">Sin vales registrados en este fondo.</p>
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Fecha</TH>
                  <TH>Detalle</TH>
                  <TH>Gasto</TH>
                  <TH>Comprobante</TH>
                  <TH>Estado</TH>
                  <TH className="text-right">Monto</TH>
                  <TH className="text-right">Acción</TH>
                </TR>
              </THead>
              <TBody>
                {vales.map((v) => {
                  const et = ETIQUETA_VALE[v.estado] ?? { label: v.estado, tone: "neutral" as const };
                  return (
                    <TR key={v.id} className={cn(v.estado === "ANULADO" && "opacity-50")}>
                      <TD className="tabular-nums">{formatFecha(v.fecha)}</TD>
                      <TD className="max-w-72 whitespace-normal">{v.detalle}</TD>
                      <TD className="text-sm">{v.concepto ? `${v.concepto} › ${v.clasificacion}` : "—"}</TD>
                      <TD>
                        {v.comprobante === "FE" ? (
                          <Badge tone="positivo">FE · deducible</Badge>
                        ) : (
                          <Badge tone="negativo">Recibo · no deducible</Badge>
                        )}
                      </TD>
                      <TD>
                        <Badge tone={et.tone}>{et.label}</Badge>
                      </TD>
                      <TD className="text-right tabular-nums">{formatMoneda(v.monto_crc, "CRC")}</TD>
                      <TD className="text-right">
                        {puedeVale && v.estado === "PENDIENTE" && (
                          <Button
                            size="sm"
                            variant="ghost"
                            className="text-negativo"
                            onClick={() =>
                              anular.mutate(
                                { fondoId: fondo.id, valeId: v.id },
                                {
                                  onSuccess: () => toast.success("Vale anulado."),
                                  onError: (e) => toast.error(mensajeError(e)),
                                },
                              )
                            }
                            loading={anular.isPending}
                          >
                            Anular
                          </Button>
                        )}
                      </TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Crear / editar fondo (cxp.caja_administrar — lo constituye el DF)
// ---------------------------------------------------------------------------

function FondoDialog({
  fondo,
  onCerrar,
  onDesactivar,
}: {
  fondo: FondoCajaChica | null;
  onCerrar: () => void;
  onDesactivar?: () => void;
}) {
  const toast = useToast();
  const crear = useCrearFondo();
  const actualizar = useActualizarFondo();
  const usuariosQ = useUsuariosEmpresa();
  const departamentosQ = useDepartamentos(true);
  const proveedoresQ = useTodosProveedores();

  const [nombre, setNombre] = useState(fondo?.nombre ?? "");
  const [custodio, setCustodio] = useState(fondo?.custodio_id ?? "");
  const [departamento, setDepartamento] = useState(fondo?.departamento_id ?? "");
  const [proveedor, setProveedor] = useState(fondo?.proveedor_id ?? "");
  const [monto, setMonto] = useState(fondo ? montoLegible(fondo.monto_asignado) : "");
  const [umbral, setUmbral] = useState(fondo ? String(toNumber(fondo.umbral_pct)) : "30");
  const [limite, setLimite] = useState(fondo && toNumber(fondo.limite_vale) > 0 ? montoLegible(fondo.limite_vale) : "");

  const pendiente = crear.isPending || actualizar.isPending;

  function onGuardar(e: FormEvent) {
    e.preventDefault();
    const m = montoParaApi(monto);
    if (!nombre.trim()) return toast.error("Indicá el nombre del fondo.");
    if (!m || Number(m) <= 0) return toast.error("Indicá el monto fijo del fondo.");
    const input: FondoInput = {
      nombre: nombre.trim(),
      custodio_id: custodio || undefined,
      departamento_id: departamento || undefined,
      proveedor_id: proveedor || undefined,
      monto_asignado: m,
      umbral_pct: umbral || undefined,
      limite_vale: montoParaApi(limite) || undefined,
    };
    const done = {
      onSuccess: () => {
        toast.success(fondo ? "Fondo actualizado." : "Fondo constituido.");
        onCerrar();
      },
      onError: (err: unknown) => toast.error(mensajeError(err)),
    };
    if (fondo) actualizar.mutate({ id: fondo.id, input }, done);
    else crear.mutate(input, done);
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onCerrar}>
      <div
        className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-xl border border-border bg-surface-raised p-5 shadow-lifted"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-content">{fondo ? `Editar fondo — ${fondo.nombre}` : "Nuevo fondo de caja chica"}</h2>
        <form onSubmit={onGuardar} className="mt-4 flex flex-col gap-3">
          <Input label="Nombre / sede *" value={nombre} onChange={(e) => setNombre(e.target.value)} placeholder="Ej. Caja chica Sede Central" />
          <Select
            label="Custodio (usuario responsable)"
            placeholder={usuariosQ.isPending ? "Cargando…" : "Seleccioná…"}
            value={custodio}
            onChange={(e) => setCustodio(e.target.value)}
            options={(usuariosQ.data ?? []).map((u) => ({ value: u.id, label: u.nombre || u.email }))}
          />
          <Select
            label="Departamento (valida sus reposiciones)"
            placeholder="Seleccioná…"
            value={departamento}
            onChange={(e) => setDepartamento(e.target.value)}
            options={(departamentosQ.data ?? []).map((d) => ({ value: d.id, label: d.nombre }))}
          />
          <Select
            label="Proveedor interno (a quién se le paga la reposición)"
            placeholder={proveedoresQ.isPending ? "Cargando…" : "Seleccioná…"}
            value={proveedor}
            onChange={(e) => setProveedor(e.target.value)}
            options={(proveedoresQ.data ?? []).filter((p) => p.activo).map((p) => ({ value: p.id, label: p.nombre }))}
          />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Input label="Monto fijo *" value={monto} onChange={(e) => setMonto(e.target.value)} onBlur={() => setMonto(montoLegible(monto))} inputMode="decimal" placeholder="200 000,00" className="text-right tabular-nums" />
            <Input label="Umbral alerta (%)" value={umbral} onChange={(e) => setUmbral(e.target.value)} inputMode="numeric" placeholder="30" className="text-right tabular-nums" />
            <Input label="Límite por vale" value={limite} onChange={(e) => setLimite(e.target.value)} onBlur={() => setLimite(montoLegible(limite))} inputMode="decimal" placeholder="40 000,00 (vacío = sin límite)" className="text-right tabular-nums" />
          </div>
          <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-content-muted">
            El custodio cobra la reposición como proveedor interno (con su IBAN en la ficha del proveedor). Regla
            sana: un fondo bien dimensionado repone 1–2 veces al mes.
          </p>
          <div className="flex justify-between gap-2 pt-1">
            <div>
              {onDesactivar && (
                <Button type="button" variant="ghost" className="text-negativo" onClick={onDesactivar}>
                  Desactivar fondo
                </Button>
              )}
            </div>
            <div className="flex gap-2">
              <Button type="button" variant="ghost" onClick={onCerrar}>
                Cancelar
              </Button>
              <Button type="submit" loading={pendiente}>
                {fondo ? "Guardar" : "Constituir fondo"}
              </Button>
            </div>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
