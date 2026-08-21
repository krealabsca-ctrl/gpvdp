/**
 * CxP — Anticipos (billetera). Muestra los anticipos del proveedor en dos grupos:
 *  · Disponibles: ya pagados, con saldo a favor → se pueden aplicar a una factura.
 *  · En trámite: registrados pero todavía sin pagar → aún NO se pueden aplicar (no se
 *    netea plata que no ha salido); se listan para que no parezcan perdidos.
 * Desde acá se registra un anticipo nuevo y se aplica a una factura del mismo proveedor.
 */

import { useState } from "react";
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
import { formatFecha, formatMoneda, montoLegible, montoParaApi, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { ETIQUETA_ESTADO, TONO_ESTADO } from "@/features/cxp/dominio";
import { useAnticiposEmpresa, useAplicarAnticipo, useDocumentos } from "@/features/cxp/hooks";
import type { AnticipoSaldo } from "@/api/cxp";

/** Estados en los que el anticipo ya es plata desembolsada y puede aplicarse. */
const APLICABLE = ["PAGADO", "CONCILIADO"];

export function AnticiposPage() {
  const navigate = useNavigate();
  const tiene = useTienePermiso();
  const q = useAnticiposEmpresa();
  const [aplicar, setAplicar] = useState<AnticipoSaldo | null>(null);

  const items = q.data ?? [];
  const disponibles = items.filter((a) => APLICABLE.includes(a.estado ?? ""));
  const enTramite = items.filter((a) => !APLICABLE.includes(a.estado ?? ""));
  const totalSaldo = disponibles.reduce((a, x) => a + toNumber(x.saldo), 0);
  const puedeAplicar = tiene("cxp.anticipos");

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Anticipos"
        description="Adelantos pagados a proveedores que quedan como saldo a favor y se descuentan de la factura final."
        actions={
          puedeAplicar ? (
            <Button onClick={() => navigate("/cxp/documentos/nuevo?tipo=ANTICIPO")}>Nuevo anticipo</Button>
          ) : undefined
        }
      />

      {q.isPending ? (
        <LoadingState label="Cargando anticipos" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState message="Todavía no hay anticipos. Usá «Nuevo anticipo» para registrar un adelanto a un proveedor; cuando se pague, quedará acá como saldo a favor." />
      ) : (
        <>
          <Card>
            <CardContent className="flex flex-wrap items-baseline justify-between gap-2 py-4">
              <span className="text-sm text-content-muted">
                {disponibles.length} disponible{disponibles.length === 1 ? "" : "s"} para aplicar
                {enTramite.length > 0 && ` · ${enTramite.length} en trámite`}
              </span>
              <span className="text-sm text-content-muted">
                Saldo disponible:{" "}
                <span className="text-lg font-bold tabular-nums text-accent">
                  {formatMoneda(String(totalSaldo), "CRC")}
                </span>
              </span>
            </CardContent>
          </Card>

          {disponibles.length > 0 && (
            <TablaAnticipos
              titulo="Disponibles para aplicar"
              items={disponibles}
              onVer={(id) => navigate(`/cxp/documentos/${id}`)}
              onAplicar={puedeAplicar ? setAplicar : undefined}
            />
          )}

          {enTramite.length > 0 && (
            <>
              <TablaAnticipos
                titulo="En trámite (aún sin pagar)"
                items={enTramite}
                onVer={(id) => navigate(`/cxp/documentos/${id}`)}
                mostrarEstado
              />
              <p className="text-xs text-content-muted">
                Un anticipo solo se puede aplicar cuando ya se pagó: hasta entonces no hay plata desembolsada
                que descontar. Llevalo por la Bandeja hasta «Pagadas» y aparecerá arriba como disponible.
              </p>
            </>
          )}
        </>
      )}

      {aplicar && <AplicarDialog anticipo={aplicar} onCerrar={() => setAplicar(null)} />}
    </div>
  );
}

function TablaAnticipos({
  titulo,
  items,
  onVer,
  onAplicar,
  mostrarEstado,
}: {
  titulo: string;
  items: AnticipoSaldo[];
  onVer: (id: string) => void;
  onAplicar?: (a: AnticipoSaldo) => void;
  mostrarEstado?: boolean;
}) {
  return (
    <div className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold text-content">{titulo}</h2>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Proveedor</TH>
              <TH>Anticipo</TH>
              <TH>{mostrarEstado ? "Estado" : "Pagado"}</TH>
              <TH className="text-right">Monto</TH>
              <TH className="text-right">Aplicado</TH>
              <TH className="text-right">{mostrarEstado ? "Pendiente" : "Saldo disponible"}</TH>
              <TH className="text-right">Acción</TH>
            </TR>
          </THead>
          <TBody>
            {items.map((a) => (
              <TR key={a.id}>
                <TD className="font-medium">{a.proveedor}</TD>
                <TD className="font-mono text-[11px] text-content-muted">{a.consecutivo || a.id.slice(0, 8)}</TD>
                <TD>
                  {mostrarEstado ? (
                    <Badge tone={(a.estado && TONO_ESTADO[a.estado]) || "neutral"}>
                      {(a.estado && ETIQUETA_ESTADO[a.estado]) || a.estado}
                    </Badge>
                  ) : (
                    <span className="tabular-nums">{formatFecha(a.fecha_pago)}</span>
                  )}
                </TD>
                <TD className="text-right tabular-nums text-content-muted">{formatMoneda(a.total_crc, "CRC")}</TD>
                <TD className="text-right tabular-nums text-content-muted">{formatMoneda(a.aplicado, "CRC")}</TD>
                <TD className="text-right font-semibold tabular-nums text-accent">{formatMoneda(a.saldo, "CRC")}</TD>
                <TD className="text-right">
                  <div className="flex justify-end gap-2">
                    {onAplicar && (
                      <Button size="sm" onClick={() => onAplicar(a)}>
                        Aplicar a factura
                      </Button>
                    )}
                    <Button size="sm" variant="ghost" onClick={() => onVer(a.id)}>
                      Ver
                    </Button>
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </div>
  );
}

/** Elige la factura destino del proveedor y el monto a aplicar (el backend revalida). */
function AplicarDialog({ anticipo, onCerrar }: { anticipo: AnticipoSaldo; onCerrar: () => void }) {
  const toast = useToast();
  const aplicar = useAplicarAnticipo();
  // Solo facturas del mismo proveedor y previas a la aprobación: ahí es donde se netea.
  const facturasQ = useDocumentos({
    proveedor_id: anticipo.proveedor_id,
    estados: "RECIBIDO,REVISADO,VALIDADO_DEPTO",
    page_size: 100,
  });
  const candidatas = (facturasQ.data?.items ?? []).filter((d) => d.tipo !== "ANTICIPO" && d.moneda === "CRC");
  const [facturaId, setFacturaId] = useState("");
  const [monto, setMonto] = useState("");

  const factura = candidatas.find((d) => d.id === facturaId);
  function elegir(id: string) {
    setFacturaId(id);
    const f = candidatas.find((d) => d.id === id);
    if (f) {
      const sugerido = Math.min(toNumber(anticipo.saldo), toNumber(f.neto_crc));
      setMonto(sugerido > 0 ? montoLegible(String(sugerido)) : "");
    }
  }
  function confirmar() {
    const m = montoParaApi(monto); // tolera "480 000" / "480.000,00" y manda decimal plano
    if (!facturaId || !m) return;
    aplicar.mutate(
      { id: facturaId, anticipoId: anticipo.id, monto: m },
      {
        onSuccess: () => {
          toast.success("Anticipo aplicado a la factura.");
          onCerrar();
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onCerrar}>
      <div
        className="w-full max-w-lg rounded-xl border border-border bg-surface-raised p-5 shadow-lifted"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-content">Aplicar anticipo a una factura</h2>
        <p className="mt-1 text-sm text-content-muted">
          {anticipo.proveedor} · saldo disponible{" "}
          <span className="font-semibold tabular-nums text-accent">{formatMoneda(anticipo.saldo, "CRC")}</span>
        </p>

        <div className="mt-4 flex flex-col gap-3">
          {facturasQ.isPending ? (
            <LoadingState label="Buscando facturas del proveedor" />
          ) : candidatas.length === 0 ? (
            <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm text-content-muted">
              Este proveedor no tiene facturas pendientes a las que aplicar el anticipo. Los anticipos se aplican
              antes de aprobar la factura.
            </p>
          ) : (
            <>
              <Select
                label="Factura destino"
                placeholder="Elegí la factura…"
                value={facturaId}
                onChange={(e) => elegir(e.target.value)}
                options={candidatas.map((d) => ({
                  value: d.id,
                  label: `${d.consecutivo || d.clave.slice(0, 12)} · neto ${formatMoneda(d.neto_crc, "CRC")}`,
                }))}
              />
              <Input
                label="Monto a aplicar"
                value={monto}
                onChange={(e) => setMonto(e.target.value)}
                onBlur={() => setMonto(montoLegible(monto))}
                inputMode="decimal"
                placeholder="0,00"
                className="text-right tabular-nums"
              />
              {factura && (
                <p className="text-xs text-content-muted">
                  Neto actual de la factura: {formatMoneda(factura.neto_crc, "CRC")} · quedaría en{" "}
                  <span className="font-semibold text-accent">
                    {formatMoneda(String(Math.max(0, toNumber(factura.neto_crc) - toNumber(monto))), "CRC")}
                  </span>
                </p>
              )}
            </>
          )}
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <Button variant="ghost" onClick={onCerrar}>
            Cancelar
          </Button>
          <Button onClick={confirmar} loading={aplicar.isPending} disabled={!facturaId || !monto.trim()}>
            Aplicar
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
