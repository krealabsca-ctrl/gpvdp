/**
 * CxP — Detalle de documento (/cxp/documentos/:id).
 * Muestra el comprobante, el stepper del flujo, la regla de aprobación por monto
 * y la acción siguiente (revisar → aprobar → programar → pagar → conciliar),
 * habilitada según el rol del usuario (espejo del RBAC del backend, que reverifica).
 */

import { useEffect, useRef, useState, type ChangeEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useNavigate, useParams } from "react-router-dom";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import {
  centimosALegible,
  centimosAPlano,
  formatFecha,
  formatMoneda,
  montoACentimos,
  montoLegible,
  montoParaApi,
  toNumber,
} from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  ETIQUETA_ESTADO,
  FLUJO_ESTADOS,
  TONO_ESTADO,
  accionSiguiente,
  etiquetaAccion,
  puedeAccion,
  textoRequisitoAprobacion,
} from "@/features/cxp/dominio";
import {
  useAdjuntarComprobante,
  useAnticiposDisponibles,
  useAplicacionesDocumento,
  useAplicarAnticiposLote,
  useAprobarDocumento,
  useConciliarDocumento,
  useCrearDocumento,
  useDocumento,
  useEnviarComprobante,
  useHistorialDocumento,
  usePagarDocumento,
  useProgramarDocumento,
  useReversarAnticipo,
  useRevisarDocumento,
  useAprobarContabilidad,
  useMarcarDocContabilidad,
} from "@/features/cxp/hooks";
import { cxpApi, etiquetaOrigenContabilidad, type Documento } from "@/api/cxp";

export function DocumentoDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const tiene = useTienePermiso();

  const docQuery = useDocumento(id);
  const historialQuery = useHistorialDocumento(id);
  const revisar = useRevisarDocumento();
  const aprobar = useAprobarDocumento();
  const programar = useProgramarDocumento();
  const pagar = usePagarDocumento();
  const conciliar = useConciliarDocumento();

  const [fechaPago, setFechaPago] = useState("");

  // Si se llegó con #anticipos (atajo "Aplicar anticipo" de la Bandeja), enfocar esa sección.
  useEffect(() => {
    if (docQuery.data && window.location.hash === "#anticipos") {
      const t = setTimeout(() => document.getElementById("anticipos")?.scrollIntoView({ block: "start" }), 120);
      return () => clearTimeout(t);
    }
  }, [docQuery.data]);

  const onErr = (err: unknown) => toast.error(mensajeError(err));

  if (docQuery.isPending) return <LoadingState label="Cargando documento" />;
  if (docQuery.isError) {
    return (
      <ErrorState message={mensajeError(docQuery.error)} onRetry={() => docQuery.refetch()} />
    );
  }
  const doc = docQuery.data;
  if (!doc) return <LoadingState label="Cargando documento" />;

  const siguiente = accionSiguiente(doc.estado);
  const permitido = siguiente ? puedeAccion(tiene, siguiente.accion) : false;
  const actualIdx = FLUJO_ESTADOS.indexOf(doc.estado);

  const transicionPendiente =
    revisar.isPending || aprobar.isPending || pagar.isPending || conciliar.isPending;

  function ejecutarSimple() {
    if (!siguiente) return;
    switch (siguiente.accion) {
      case "revisar":
        return revisar.mutate(doc.id, {
          onSuccess: () => toast.success("Documento revisado."),
          onError: onErr,
        });
      case "aprobar":
        return aprobar.mutate(doc.id, {
          onSuccess: (data) => {
            const actualizado = data as Documento;
            toast.success(
              actualizado.estado === "APROBADO"
                ? "Documento aprobado."
                : "Aprobación registrada; faltan más firmas.",
            );
          },
          onError: onErr,
        });
      case "pagar":
        return pagar.mutate(doc.id, {
          onSuccess: () => toast.success("Documento marcado como pagado."),
          onError: onErr,
        });
      case "conciliar":
        return conciliar.mutate(doc.id, {
          onSuccess: () => toast.success("Documento conciliado."),
          onError: onErr,
        });
      default:
        return;
    }
  }

  function programarPago() {
    if (!fechaPago) {
      toast.error("Elegí la fecha de pago programada.");
      return;
    }
    programar.mutate(
      { id: doc.id, fecha: fechaPago },
      {
        onSuccess: () => toast.success("Pago programado; se generó la huella para el banco."),
        onError: onErr,
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={`Documento ${doc.consecutivo || ""}`.trim()}
        description={doc.proveedor}
        actions={
          <div className="flex items-center gap-2">
            <Badge tone={TONO_ESTADO[doc.estado]}>{ETIQUETA_ESTADO[doc.estado]}</Badge>
            <Button variant="secondary" onClick={() => navigate("/cxp/documentos")}>
              Volver
            </Button>
          </div>
        }
      />

      {/* Stepper del flujo */}
      <Card>
        <CardContent className="py-4">
          <ol className="flex flex-wrap items-center gap-2">
            {FLUJO_ESTADOS.map((e, i) => (
              <li key={e} className="flex items-center gap-2">
                <span
                  className={cn(
                    "rounded px-2 py-1 text-xs font-medium",
                    i === actualIdx
                      ? "bg-accent text-accent-fg"
                      : i < actualIdx
                        ? "bg-accent/10 text-accent"
                        : "bg-surface-muted text-content-muted",
                  )}
                >
                  {ETIQUETA_ESTADO[e]}
                </span>
                {i < FLUJO_ESTADOS.length - 1 && (
                  <span className="text-content-muted" aria-hidden>
                    →
                  </span>
                )}
              </li>
            ))}
          </ol>
        </CardContent>
      </Card>

      {/* Motivo de devolución: cuando la factura volvió a Contabilidad para corregir. */}
      {doc.estado === "RECIBIDO" && doc.nota_revision && (
        <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
          <p className="text-sm font-medium text-content">↩ Devuelta a Contabilidad — motivo</p>
          <p className="mt-0.5 text-sm text-content-muted">{doc.nota_revision}</p>
        </div>
      )}

      {/* Datos del comprobante */}
      <Card>
        <CardHeader>
          <CardTitle>Comprobante</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Campo label="Proveedor">{doc.proveedor}</Campo>
            <Campo label="Consecutivo">
              <span className="font-mono text-xs">{doc.consecutivo || "—"}</span>
            </Campo>
            <Campo label="Clave">
              <span className="break-all font-mono text-xs">{doc.clave || "—"}</span>
            </Campo>
            <Campo label="Fecha de emisión">
              <span className="tabular-nums">{formatFecha(doc.fecha_emision)}</span>
            </Campo>
            <Campo label="Vencimiento">
              <span className="tabular-nums">{formatFecha(doc.fecha_vencimiento)}</span>
            </Campo>
            <Campo label="Gasto">
              {doc.concepto || doc.clasificacion
                ? `${doc.concepto}${doc.clasificacion ? " › " + doc.clasificacion : ""}${doc.subclasificacion ? " › " + doc.subclasificacion : ""}`
                : "Sin clasificar"}
            </Campo>
            <Campo label="Moneda">{doc.moneda}</Campo>
            <Campo label="Subtotal">
              <span className="tabular-nums">{formatMoneda(doc.subtotal, doc.moneda)}</span>
            </Campo>
            <Campo label="IVA">
              <span className="tabular-nums">{formatMoneda(doc.iva, doc.moneda)}</span>
            </Campo>
            <Campo label="Retención">
              <span className="tabular-nums">{formatMoneda(doc.retencion, doc.moneda)}</span>
            </Campo>
            <Campo label="Total">
              <span className="font-semibold tabular-nums">{formatMoneda(doc.total, doc.moneda)}</span>
            </Campo>
            {doc.moneda === "USD" && (
              <Campo label="Total en colones">
                <span className="tabular-nums">{formatMoneda(doc.total_crc, "CRC")}</span>
              </Campo>
            )}
            <Campo label="Fecha de pago programada">
              <span className="tabular-nums">{formatFecha(doc.fecha_pago_programada)}</span>
            </Campo>
            <Campo label="Huella (banco)">
              {doc.huella ? (
                <span className="font-mono text-xs text-accent">{doc.huella}</span>
              ) : (
                "—"
              )}
            </Campo>
            {doc.descripcion && (
              <div className="sm:col-span-2 lg:col-span-3">
                <Campo label="Descripción">{doc.descripcion}</Campo>
              </div>
            )}
          </dl>
        </CardContent>
      </Card>

      {doc.tipo !== "ANTICIPO" &&
        (toNumber(doc.anticipos_aplicados) > 0 ||
          (tiene("cxp.anticipos") &&
            (doc.estado === "RECIBIDO" || doc.estado === "REVISADO" || doc.estado === "VALIDADO_DEPTO"))) && (
          <div id="anticipos" className="scroll-mt-24">
            <AnticiposCard doc={doc} />
          </div>
        )}

      {/* Facturas «de Contabilidad»: el check y la aprobación sin validación de área */}
      <ContabilidadCard doc={doc} />

      {/* Acción siguiente */}
      <Card>
        <CardHeader>
          <CardTitle>Flujo</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-sm text-content-muted">
            Aprobación requerida por monto: {textoRequisitoAprobacion(toNumber(doc.neto_crc))}
          </p>

          {!siguiente ? (
            <Badge tone="positivo">Documento conciliado — flujo completo.</Badge>
          ) : siguiente.accion === "programar" ? (
            <div className="flex flex-wrap items-end gap-3">
              <Input
                label="Fecha de pago programada"
                type="date"
                value={fechaPago}
                onChange={(e) => setFechaPago(e.target.value)}
                className="max-w-52"
              />
              <Button
                onClick={programarPago}
                loading={programar.isPending}
                disabled={!permitido || !fechaPago}
                title={permitido ? undefined : "Tu rol no puede programar pagos"}
              >
                Programar pago
              </Button>
            </div>
          ) : (
            <div>
              <Button
                onClick={ejecutarSimple}
                loading={transicionPendiente}
                disabled={!permitido}
                title={permitido ? undefined : "Tu rol no puede ejecutar esta acción"}
              >
                {siguiente.label}
              </Button>
            </div>
          )}

          {siguiente && !permitido && (
            <p className="text-xs text-content-muted">
              Esta acción la realiza otro rol. El sistema la habilitará al usuario autorizado.
            </p>
          )}
        </CardContent>
      </Card>

      {/* Comprobante de pago (solo facturas pagadas / conciliadas) */}
      {(doc.estado === "PAGADO" || doc.estado === "CONCILIADO") && <ComprobanteCard doc={doc} />}

      {/* Línea de tiempo / trazabilidad */}
      <Card>
        <CardHeader>
          <CardTitle>Historial</CardTitle>
        </CardHeader>
        <CardContent>
          {historialQuery.isPending ? (
            <LoadingState label="Cargando historial" />
          ) : historialQuery.isError ? (
            <p className="text-sm text-content-muted">No se pudo cargar el historial.</p>
          ) : (historialQuery.data?.eventos.length ?? 0) === 0 ? (
            <p className="text-sm text-content-muted">Sin eventos registrados aún.</p>
          ) : (
            <ol className="flex flex-col gap-3">
              {historialQuery.data!.eventos.map((ev, i) => (
                <li key={i} className="flex items-start gap-3">
                  <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-accent" aria-hidden />
                  <div className="flex flex-col">
                    <span className="text-sm font-medium text-content">{etiquetaAccion(ev.accion)}</span>
                    <span className="text-xs text-content-muted">
                      {ev.usuario} · {formatFechaHora(ev.fecha)}
                    </span>
                    {ev.nota && (
                      <span className="mt-1 rounded-md border-l-2 border-accent/40 bg-surface-muted px-2 py-1 text-xs text-content">
                        “{ev.nota}”
                      </span>
                    )}
                  </div>
                </li>
              ))}
            </ol>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

/** Formatea un timestamp ISO a fecha y hora local legible (es-CR). */
function formatFechaHora(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("es-CR", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// Netting de anticipos: desglose Total/−Anticipos/=Neto + aplicar/reversar anticipos del proveedor.
function AnticiposCard({ doc }: { doc: Documento }) {
  const toast = useToast();
  const tiene = useTienePermiso();
  const disponiblesQ = useAnticiposDisponibles(doc.proveedor_id);
  const aplicacionesQ = useAplicacionesDocumento(doc.id);
  const aplicarLote = useAplicarAnticiposLote();
  const reversar = useReversarAnticipo();
  // Selección múltiple (patrón aprobado en la maqueta): casilla + monto por anticipo, con el
  // neto recalculándose en vivo. Se aplican todos juntos en una sola operación (todo-o-nada).
  const [marcados, setMarcados] = useState<Record<string, boolean>>({});
  const [montos, setMontos] = useState<Record<string, string>>({});
  const [abonoAbierto, setAbonoAbierto] = useState(false);

  const preAprobacion = doc.estado === "RECIBIDO" || doc.estado === "REVISADO" || doc.estado === "VALIDADO_DEPTO";
  const puede = tiene("cxp.anticipos") && preAprobacion && doc.moneda === "CRC";
  const aplicados = toNumber(doc.anticipos_aplicados);
  const aplicaciones = aplicacionesQ.data ?? [];
  const disponibles = disponiblesQ.data ?? [];

  const netoCent = montoACentimos(doc.neto_crc);
  // Recorte en cascada: cada línea marcada se topa por su saldo y por lo que reste del neto.
  const lineas = disponibles
    .filter((a) => marcados[a.id])
    .reduce<{ id: string; consecutivo: string; cent: number }[]>((acc, a) => {
      const usado = acc.reduce((s, l) => s + l.cent, 0);
      const resta = Math.max(0, netoCent - usado);
      const pedido = montoACentimos(montos[a.id] ?? "");
      const cent = Math.min(pedido, montoACentimos(a.saldo), resta);
      if (cent > 0) acc.push({ id: a.id, consecutivo: a.consecutivo, cent });
      return acc;
    }, []);
  const aplicandoCent = lineas.reduce((s, l) => s + l.cent, 0);
  const netoResultante = Math.max(0, netoCent - aplicandoCent);

  function marcar(a: (typeof disponibles)[number], on: boolean) {
    setMarcados((p) => ({ ...p, [a.id]: on }));
    if (on && !montos[a.id]) {
      // Sugerir el menor entre el saldo del anticipo y lo que falta del neto.
      const usado = lineas.reduce((s, l) => s + l.cent, 0);
      const sug = Math.min(montoACentimos(a.saldo), Math.max(0, netoCent - usado));
      setMontos((p) => ({ ...p, [a.id]: sug > 0 ? centimosALegible(sug) : "" }));
    }
  }
  function onAplicar() {
    if (!lineas.length) return;
    aplicarLote.mutate(
      { id: doc.id, aplicaciones: lineas.map((l) => ({ anticipo_id: l.id, monto: centimosAPlano(l.cent) })) },
      {
        onSuccess: () => {
          toast.success(lineas.length === 1 ? "Anticipo aplicado." : `${lineas.length} anticipos aplicados.`);
          setMarcados({});
          setMontos({});
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }
  function onQuitar(aplicacionId: string) {
    reversar.mutate(
      { id: doc.id, aplicacionId },
      { onSuccess: () => toast.success("Aplicación reversada."), onError: (e) => toast.error(mensajeError(e)) },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Anticipos</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* Desglose del neto */}
        <div className="max-w-sm overflow-hidden rounded-lg border border-border">
          <div className="flex items-baseline justify-between px-4 py-2.5 text-sm">
            <span className="text-content-muted">Total factura</span>
            <span className="font-medium tabular-nums">{formatMoneda(doc.total_crc, "CRC")}</span>
          </div>
          {/* Una línea por anticipo ya aplicado (desglose, como en la maqueta). */}
          {aplicaciones.map((a) => (
            <div key={a.id} className="flex items-baseline justify-between border-t border-border px-4 py-2.5 text-sm">
              <span className="text-content-muted">− {a.anticipo_consecutivo || "Anticipo"} aplicado</span>
              <span className="tabular-nums text-negativo">−{formatMoneda(a.monto_crc, "CRC")}</span>
            </div>
          ))}
          {aplicados > 0 && aplicaciones.length === 0 && (
            <div className="flex items-baseline justify-between border-t border-border px-4 py-2.5 text-sm">
              <span className="text-content-muted">− Anticipos aplicados</span>
              <span className="tabular-nums text-negativo">−{formatMoneda(doc.anticipos_aplicados, "CRC")}</span>
            </div>
          )}
          {aplicandoCent > 0 && (
            <div className="flex items-baseline justify-between border-t border-border px-4 py-2.5 text-sm">
              <span className="text-content-muted">− Por aplicar ahora ({lineas.length})</span>
              <span className="tabular-nums text-negativo">−{formatMoneda(centimosAPlano(aplicandoCent), "CRC")}</span>
            </div>
          )}
          <div className="flex items-baseline justify-between border-t border-border bg-accent/5 px-4 py-2.5">
            <span className="text-sm font-semibold text-accent">= Neto a pagar</span>
            <span className="text-lg font-bold tabular-nums text-accent">
              {formatMoneda(centimosAPlano(netoResultante), "CRC")}
            </span>
          </div>
        </div>
        {aplicandoCent > 0 && netoResultante === 0 && (
          <p className="rounded-lg border border-brand-gold/40 bg-brand-gold/10 px-3 py-2 text-sm text-content">
            ✓ La factura queda <b>totalmente cubierta</b> por los anticipos: no genera pago. Si sobra saldo, vuelve a
            la billetera del proveedor.
          </p>
        )}

        {/* Anticipos ya aplicados */}
        {aplicaciones.length > 0 && (
          <ul className="flex flex-col gap-2">
            {aplicaciones.map((a) => (
              <li
                key={a.id}
                className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2 text-sm"
              >
                <span>
                  <span className="font-medium">{a.anticipo_consecutivo || "Anticipo"}</span>{" "}
                  <span className="tabular-nums text-content-muted">· {formatMoneda(a.monto_crc, "CRC")}</span>
                  {a.aplicado_por_nombre && (
                    <span className="block text-[11px] text-content-muted">aplicó {a.aplicado_por_nombre}</span>
                  )}
                </span>
                {puede && (
                  <Button size="sm" variant="ghost" className="text-negativo" onClick={() => onQuitar(a.id)} loading={reversar.isPending}>
                    Quitar
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}

        {/* Registrar un abono: adelanto parcial sobre ESTA factura. Nace como documento tipo
            «Anticipo» del mismo proveedor y, una vez pagado, se aplica acá bajando el neto. */}
        {puede && (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-dashed border-border px-3 py-2">
            <span className="text-xs text-content-muted">
              ¿Vas a pagar solo una parte ahora? Registrá un abono: se paga por el flujo normal y se descuenta
              de esta factura.
            </span>
            <Button size="sm" variant="secondary" onClick={() => setAbonoAbierto(true)}>
              Registrar abono
            </Button>
          </div>
        )}
        {abonoAbierto && <AbonoDialog doc={doc} onCerrar={() => setAbonoAbierto(false)} />}

        {/* Aplicar anticipos: uno o varios a la vez, con monto por fila (patrón de la maqueta). */}
        {puede ? (
          disponibles.length > 0 ? (
            <div className="flex flex-col gap-2 border-t border-border pt-4">
              <p className="text-[11px] font-semibold uppercase tracking-wide text-content-muted">
                Anticipos disponibles de este proveedor
              </p>
              {disponibles.map((a) => {
                const on = !!marcados[a.id];
                return (
                  <label
                    key={a.id}
                    className="flex items-center gap-3 rounded-lg border border-border px-3 py-2 has-[:checked]:border-accent/50 has-[:checked]:bg-accent/5"
                  >
                    <input
                      type="checkbox"
                      checked={on}
                      onChange={(e) => marcar(a, e.target.checked)}
                      className="h-4 w-4 rounded accent-accent"
                      aria-label={`Aplicar ${a.consecutivo || "anticipo"}`}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block text-sm font-medium">{a.consecutivo || "Anticipo"}</span>
                      <span className="block text-[11px] text-content-muted">
                        pagado {formatFecha(a.fecha_pago)} · saldo disponible {formatMoneda(a.saldo, "CRC")}
                      </span>
                    </span>
                    <Input
                      value={montos[a.id] ?? ""}
                      onChange={(e) => setMontos((p) => ({ ...p, [a.id]: e.target.value }))}
                      onBlur={() => {
                        // Tope en vivo: nunca más que el saldo del anticipo ni que el neto que
                        // queda por cubrir (contando lo que ya toman las otras filas marcadas).
                        const otras = lineas.filter((l) => l.id !== a.id).reduce((s, l) => s + l.cent, 0);
                        const tope = Math.min(montoACentimos(a.saldo), Math.max(0, netoCent - otras));
                        setMontos((p) => ({
                          ...p,
                          [a.id]: centimosALegible(Math.min(montoACentimos(p[a.id] ?? ""), tope)),
                        }));
                      }}
                      disabled={!on}
                      inputMode="decimal"
                      placeholder="0,00"
                      className="w-32 text-right tabular-nums"
                      aria-label={`Monto a aplicar de ${a.consecutivo || "anticipo"}`}
                    />
                  </label>
                );
              })}
              <div className="flex items-center justify-between gap-3 pt-1">
                <span className="text-xs text-content-muted">
                  {aplicandoCent > 0
                    ? `Se aplicarán ${formatMoneda(centimosAPlano(aplicandoCent), "CRC")} en ${lineas.length} anticipo(s).`
                    : "Marcá los anticipos que querés aplicar."}
                </span>
                <Button onClick={onAplicar} loading={aplicarLote.isPending} disabled={!lineas.length}>
                  Aplicar y registrar neto
                </Button>
              </div>
            </div>
          ) : (
            <p className="border-t border-border pt-4 text-sm text-content-muted">
              Este proveedor no tiene anticipos con saldo disponible. Registrá un documento tipo «Anticipo», pagalo,
              y quedará acá para aplicar.
            </p>
          )
        ) : (
          aplicados === 0 && (
            <p className="text-sm text-content-muted">
              {doc.moneda !== "CRC"
                ? "El neteo de anticipos está disponible solo en colones (CRC)."
                : "Los anticipos se aplican antes de aprobar la factura."}
            </p>
          )
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Registrar un abono (pago parcial) sobre esta factura. Se materializa como un documento tipo
 * «Anticipo» del mismo proveedor: sigue el flujo de pago normal (control interno intacto) y al
 * quedar pagado se aplica contra la factura, bajando su neto.
 */
function AbonoDialog({ doc, onCerrar }: { doc: Documento; onCerrar: () => void }) {
  const toast = useToast();
  const crear = useCrearDocumento();
  const [monto, setMonto] = useState("");
  const [nota, setNota] = useState("");
  const hoy = new Date().toISOString().slice(0, 10);
  const ref = doc.consecutivo || doc.clave.slice(0, 12);

  function confirmar() {
    const m = montoParaApi(monto); // tolera "480 000" / "480.000,00" y manda decimal plano
    if (!m || Number(m) <= 0) return toast.error("Indicá el monto del abono.");
    if (Number(m) > toNumber(doc.neto_crc))
      return toast.error(`El abono no puede superar el neto de la factura (${formatMoneda(doc.neto_crc, "CRC")}).`);
    crear.mutate(
      {
        proveedor_id: doc.proveedor_id,
        tipo: "ANTICIPO",
        clave: "",
        fecha_emision: hoy,
        moneda: "CRC",
        total: m,
        descripcion: nota.trim() || `Abono a factura ${ref}`,
      },
      {
        onSuccess: () => {
          toast.success("Abono registrado. Pagalo desde la Bandeja y volvé acá para aplicarlo.");
          onCerrar();
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onCerrar}>
      <div
        className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-content">Registrar abono</h2>
        <p className="mt-1 text-sm text-content-muted">
          {doc.proveedor} · factura {ref} · neto actual{" "}
          <span className="font-semibold tabular-nums text-accent">{formatMoneda(doc.neto_crc, "CRC")}</span>
        </p>
        <div className="mt-4 flex flex-col gap-3">
          <Input
            label="Monto del abono *"
            value={monto}
            onChange={(e) => setMonto(e.target.value)}
            onBlur={() => setMonto(montoLegible(monto))}
            inputMode="decimal"
            className="text-right tabular-nums"
            placeholder="0,00"
          />
          <Input
            label="Detalle (opcional)"
            value={nota}
            onChange={(e) => setNota(e.target.value)}
            placeholder={`Abono a factura ${ref}`}
          />
          <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-content-muted">
            Se crea como documento tipo «Anticipo» del proveedor. Pasa por el flujo de pago como cualquier
            desembolso y, una vez pagado, lo aplicás acá: el neto de esta factura baja y solo se paga el resto.
          </p>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="ghost" onClick={onCerrar}>
            Cancelar
          </Button>
          <Button onClick={confirmar} loading={crear.isPending} disabled={!monto.trim()}>
            Registrar abono
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

function ComprobanteCard({ doc }: { doc: Documento }) {
  const toast = useToast();
  const tiene = useTienePermiso();
  const adjuntar = useAdjuntarComprobante();
  const enviar = useEnviarComprobante();
  const fileRef = useRef<HTMLInputElement>(null);
  const [descargando, setDescargando] = useState(false);
  const puede = puedeAccion(tiene, "pagar"); // adjuntar/enviar: nivel Tesorería/Dirección

  function onFile(e: ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    if (!f) return;
    adjuntar.mutate(
      { id: doc.id, archivo: f },
      {
        onSuccess: () => toast.success("Comprobante adjuntado."),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
    if (fileRef.current) fileRef.current.value = "";
  }

  async function descargar() {
    setDescargando(true);
    try {
      const blob = await cxpApi.descargarComprobante(doc.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `comprobante-${doc.consecutivo || doc.id.slice(0, 8)}.pdf`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setDescargando(false);
    }
  }

  function enviarAlProveedor() {
    enviar.mutate(doc.id, {
      onSuccess: () => toast.success("Comprobante enviado al proveedor."),
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Comprobante de pago</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          {doc.tiene_comprobante ? (
            <Badge tone="positivo">Adjuntado</Badge>
          ) : (
            <Badge tone="pendiente">Sin comprobante</Badge>
          )}
          {doc.comprobante_enviado_en && (
            <span className="text-sm text-content-muted">
              Enviado al proveedor el {formatFecha(doc.comprobante_enviado_en.slice(0, 10))}
            </span>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {puede && (
            <>
              <input
                ref={fileRef}
                type="file"
                accept="application/pdf,.pdf"
                onChange={onFile}
                className="sr-only"
                aria-label="Comprobante de pago (PDF)"
              />
              <Button
                variant="secondary"
                size="sm"
                onClick={() => fileRef.current?.click()}
                loading={adjuntar.isPending}
              >
                {doc.tiene_comprobante ? "Reemplazar comprobante" : "Adjuntar comprobante"}
              </Button>
            </>
          )}
          {doc.tiene_comprobante && (
            <Button variant="secondary" size="sm" onClick={descargar} loading={descargando}>
              Descargar
            </Button>
          )}
          {doc.tiene_comprobante && puede && (
            <Button size="sm" onClick={enviarAlProveedor} loading={enviar.isPending}>
              Enviar al proveedor
            </Button>
          )}
        </div>
        {!puede && (
          <p className="text-xs text-content-muted">Adjuntar/enviar el comprobante lo hace Tesorería/Dirección.</p>
        )}
      </CardContent>
    </Card>
  );
}

function Campo({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-content-muted">{label}</dt>
      <dd className="mt-0.5 text-sm text-content">{children}</dd>
    </div>
  );
}

/**
 * Facturas «de Contabilidad»: el gasto que no tiene área operativa que lo valide (honorarios
 * contables, timbres, comisiones bancarias, Hacienda, auditoría). Antes se quedaban trancadas
 * esperando a un validador de área que no existe.
 *
 * Dos cosas en una tarjeta, porque son la misma decisión:
 *  · el CHECK (con motivo obligatorio cuando se marca a mano), y
 *  · el botón de aprobar por la vía propia, que se salta la validación de área pero NO la matriz
 *    de firmas por monto.
 *
 * Solo se muestra en el tramo donde la marca cambia algo: después de aprobar ya no.
 */
function ContabilidadCard({ doc }: { doc: Documento }) {
  const toast = useToast();
  const tiene = useTienePermiso();
  const marcar = useMarcarDocContabilidad();
  const aprobarConta = useAprobarContabilidad();
  const [motivo, setMotivo] = useState("");

  const puedeMarcar = tiene("cxp.marcar_contabilidad");
  const puedeAprobar = tiene("cxp.aprobar_contabilidad");
  const enTramo = doc.estado === "RECIBIDO" || doc.estado === "REVISADO" || doc.estado === "VALIDADO_DEPTO";

  // Sin permisos y sin marca no hay nada que decir: la tarjeta no aparece.
  if (!doc.es_contabilidad && !puedeMarcar) return null;
  if (!enTramo && !doc.es_contabilidad) return null;

  const heredada = doc.es_contabilidad && doc.contabilidad_origen !== "FACTURA";

  function marcarComoConta() {
    if (!motivo.trim()) {
      toast.error("Escribí el motivo: queda en la auditoría de la factura.");
      return;
    }
    marcar.mutate(
      { id: doc.id, valor: true, motivo: motivo.trim() },
      {
        onSuccess: () => {
          setMotivo("");
          toast.success("Marcada como de Contabilidad: ya no espera validación de área.");
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Factura de Contabilidad</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {doc.es_contabilidad ? (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone="accent">🧾 No requiere validación de área</Badge>
              <span className="text-sm text-content-muted">
                {etiquetaOrigenContabilidad(doc.contabilidad_origen)}
              </span>
            </div>
            {doc.contabilidad_motivo && (
              <p className="text-sm text-content">
                <span className="text-content-muted">Motivo:</span> {doc.contabilidad_motivo}
              </p>
            )}
            <p className="text-xs text-content-muted">
              Se salta el control operativo del área. La aprobación por monto se aplica igual:{" "}
              {textoRequisitoAprobacion(toNumber(doc.neto_crc))}.
            </p>
            {enTramo && (
              <div className="flex flex-wrap items-center gap-2">
                {puedeAprobar && (
                  <Button
                    onClick={() =>
                      aprobarConta.mutate(doc.id, {
                        onSuccess: (d) =>
                          toast.success(
                            (d as Documento).estado === "APROBADO"
                              ? "Aprobada sin validación de área."
                              : "Firma registrada; faltan más firmas.",
                          ),
                        onError: (e) => toast.error(mensajeError(e)),
                      })
                    }
                    loading={aprobarConta.isPending}
                  >
                    Aprobar como Contabilidad
                  </Button>
                )}
                {puedeMarcar && (
                  <Button
                    variant="secondary"
                    onClick={() =>
                      marcar.mutate(
                        { id: doc.id, valor: heredada ? false : null },
                        {
                          onSuccess: () =>
                            toast.success(
                              heredada
                                ? "Esta factura sí la valida el área (el proveedor/rubro sigue marcado)."
                                : "Marca quitada.",
                            ),
                          onError: (e) => toast.error(mensajeError(e)),
                        },
                      )
                    }
                    loading={marcar.isPending}
                  >
                    {heredada ? "Que la valide el área" : "Quitar la marca"}
                  </Button>
                )}
              </div>
            )}
          </>
        ) : (
          <>
            <p className="text-sm text-content-muted">
              Marcá la factura si es gasto de Contabilidad y ningún área tiene que confirmarla
              (honorarios contables, timbres, comisiones bancarias, Hacienda, auditoría). Deja de
              esperar validación de área; la firma por monto se aplica igual.
            </p>
            <div className="flex flex-wrap items-end gap-3">
              <Input
                label="Motivo (queda en la auditoría)"
                value={motivo}
                onChange={(e) => setMotivo(e.target.value)}
                placeholder="p. ej. honorarios contables del mes"
                className="min-w-64"
              />
              <Button onClick={marcarComoConta} loading={marcar.isPending} disabled={!enTramo}>
                Marcar como de Contabilidad
              </Button>
            </div>
            {!enTramo && (
              <p className="text-xs text-content-muted">
                La marca solo se cambia antes de aprobar: después ya no cambiaría nada del flujo y
                borraría el rastro.
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
