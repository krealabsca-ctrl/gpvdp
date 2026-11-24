/**
 * Qué se le debe de verdad a este proveedor por esta factura.
 *
 * Cierra la tanda B de la auditoría Bancos↔CxP (hallazgos 3.5, 3.6 y 3.8). Las tres son el mismo
 * enunciado: **el neto no siempre es el total**, y hasta hoy no había dónde verlo ni dónde
 * moverlo.
 *
 * ── POR QUÉ ES UNA RESTA Y NO UNA CIFRA ─────────────────────────────────────────────────────
 *
 * La tentación es mostrar «Saldo: ₡39.987» y listo. No sirve: cuando el número no coincide con lo
 * que alguien tiene en la cabeza —o con lo que el proveedor reclama— hace falta poder señalar la
 * línea que explica la diferencia. Por eso se muestra la resta completa, con cada descuento
 * nombrado, y el total de la factura arriba de todo.
 *
 * Es la misma razón por la que el servidor devuelve neto, pagado y saldo juntos en vez de solo el
 * último: tres números que cierran entre sí se pueden verificar; uno solo hay que creerlo.
 *
 * ── LAS LÍNEAS QUE NO ESTÁN EN CERO SON LAS QUE SE MUESTRAN ─────────────────────────────────
 *
 * Una factura normal —sin retención, sin anticipo, sin nota, sin pago parcial— no tiene nada que
 * explicar, y la tarjeta entera no aparece. Mostrar cinco líneas en cero en las 4.542 facturas
 * corrientes convertiría la señal en ruido justo donde tiene que destacar.
 */

import { useState } from "react";

import { Badge, Button, Card, CardContent, CardHeader, CardTitle, Input, useToast } from "@/components/ui";
import type { Documento, NotaCreditoSaldo } from "@/api/cxp";
import {
  useAplicarNotaCredito,
  useCobrarSaldo,
  useNotasDeCreditoDisponibles,
  useNotasDeFactura,
  useReversarNotaCredito,
  useSaldoDeFactura,
} from "@/features/cxp/hooks";
import { useTienePermiso } from "@/features/auth/permisos";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda, toNumber } from "@/lib/format";

/** El mínimo que el servidor exige en cualquier motivo. Se refleja para avisar antes de enviar. */
const MOTIVO_MINIMO = 20;

export function SaldoDeLaFactura({ doc }: { doc: Documento }) {
  const tiene = useTienePermiso();
  const saldoQ = useSaldoDeFactura(doc.id);
  const notasQ = useNotasDeFactura(doc.id);

  const moneda = (doc.moneda || "CRC") as "CRC" | "USD";
  const retencion = toNumber(doc.retencion);
  const anticipos = toNumber(doc.anticipos_aplicados);
  const notas = (notasQ.data ?? []).reduce((s, n) => s + toNumber(n.monto), 0);
  const parciales = saldoQ.data ? toNumber(saldoQ.data.pagado) : 0;
  const saldo = saldoQ.data ? toNumber(saldoQ.data.saldo) : toNumber(doc.neto_crc);

  // Solo antes de aprobar se puede mover el neto: lo que se aprueba ES el neto.
  const preAprobacion = doc.estado === "RECIBIDO" || doc.estado === "REVISADO" || doc.estado === "VALIDADO_DEPTO";
  const puedeNotas = tiene("cxp.anticipos") && preAprobacion && doc.tipo !== "NOTA_CREDITO";
  const hayDescuentos = retencion > 0 || anticipos > 0 || notas > 0 || parciales > 0;

  // La tarjeta aparece cuando hay algo que explicar, o cuando se puede agregar una nota. Una
  // factura corriente no la muestra.
  if (!hayDescuentos && !puedeNotas) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Lo que se le debe por esta factura</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="max-w-md space-y-1 text-sm">
          <Linea label="Total de la factura" monto={toNumber(doc.total)} moneda={moneda} />
          {retencion > 0 && (
            <Linea label="Retención (se entera a Hacienda)" monto={-retencion} moneda={moneda} />
          )}
          {anticipos > 0 && <Linea label="Anticipos ya entregados" monto={-anticipos} moneda={moneda} />}
          {notas > 0 && <Linea label="Notas de crédito del proveedor" monto={-notas} moneda={moneda} />}
          {parciales > 0 && <Linea label="Ya debitado por el banco" monto={-parciales} moneda={moneda} />}
          <div className="flex items-baseline justify-between border-t border-border pt-1.5 font-semibold text-content">
            <dt>Saldo a pagar</dt>
            <dd className="tabular-nums">{formatMoneda(String(saldo), moneda)}</dd>
          </div>
        </dl>

        {parciales > 0 && <ParcialesYSaldo doc={doc} saldo={saldo} />}
        {notasQ.data && notasQ.data.length > 0 && (
          <NotasAplicadas doc={doc} puedeQuitar={puedeNotas} />
        )}
        {puedeNotas && <AplicarNota doc={doc} saldo={saldo} />}
      </CardContent>
    </Card>
  );
}

function Linea({ label, monto, moneda }: { label: string; monto: number; moneda: "CRC" | "USD" }) {
  const resta = monto < 0;
  return (
    <div className="flex items-baseline justify-between">
      <dt className="text-content-muted">{label}</dt>
      <dd className={"tabular-nums " + (resta ? "text-negativo" : "text-content")}>
        {resta ? "−" : ""}
        {formatMoneda(String(Math.abs(monto)), moneda)}
      </dd>
    </div>
  );
}

/** El detalle de lo que el banco ya debitó, y la puerta para volver a cobrar lo que falta. */
function ParcialesYSaldo({ doc, saldo }: { doc: Documento; saldo: number }) {
  const toast = useToast();
  const tiene = useTienePermiso();
  const saldoQ = useSaldoDeFactura(doc.id);
  const cobrar = useCobrarSaldo();
  const [motivo, setMotivo] = useState("");
  const [abierto, setAbierto] = useState(false);

  const parciales = saldoQ.data?.parciales ?? [];
  const moneda = (doc.moneda || "CRC") as "CRC" | "USD";
  // Volver a cobrar es decir «el resto se sigue debiendo», y eso NO es automático: si la
  // diferencia era una nota de crédito o una retención, cobrarla otra vez le paga de más al
  // proveedor. Por eso lleva su propio permiso y su motivo.
  const puedeCobrar = tiene("cxp.cobrar_saldo") && saldo > 0 && doc.estado === "PAGADO";

  return (
    <div className="rounded-lg border border-border bg-surface-muted/40 p-3">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-content-muted">
        Lo que el banco ya debitó
      </h3>
      <ul className="space-y-1.5 text-sm">
        {parciales.map((p) => (
          <li key={p.id} className="flex flex-wrap items-baseline gap-x-3">
            <span className="tabular-nums text-content-muted">{p.fecha}</span>
            <span className="min-w-0 flex-1 truncate text-content">{p.descripcion}</span>
            <span className="tabular-nums font-medium text-content">
              {formatMoneda(p.monto, moneda)}
            </span>
            <span className="w-full text-xs text-content-muted">{p.motivo}</span>
          </li>
        ))}
      </ul>

      {saldo > 0 && (
        <div className="mt-3 border-t border-border pt-3">
          {!puedeCobrar ? (
            <p className="text-xs text-content-muted">
              Quedan {formatMoneda(String(saldo), moneda)} sin cobrar. Mientras no se decida que se
              siguen debiendo, no vuelven a la cola de pago.
            </p>
          ) : !abierto ? (
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-xs text-content-muted">
                ¿El resto se sigue debiendo? Si la diferencia era una nota de crédito o una
                retención, <b className="text-content">no hay que cobrarla</b>.
              </p>
              <Button variant="secondary" size="sm" onClick={() => setAbierto(true)}>
                Mandar a cobrar el saldo
              </Button>
            </div>
          ) : (
            <div className="space-y-2">
              <Input
                label="Por qué el saldo se sigue debiendo"
                value={motivo}
                onChange={(e) => setMotivo(e.target.value)}
                placeholder="Ej.: el banco rechazó la línea; el proveedor confirma que el resto sigue pendiente"
              />
              <div className="flex items-center justify-between gap-3">
                <span className="text-[11px] text-content-muted">
                  La factura vuelve al circuito y el próximo corte paga{" "}
                  {formatMoneda(String(saldo), moneda)}, no el total.
                </span>
                <div className="flex gap-2">
                  <Button variant="secondary" size="sm" onClick={() => setAbierto(false)}>
                    Cancelar
                  </Button>
                  <Button
                    size="sm"
                    loading={cobrar.isPending}
                    disabled={motivo.trim().length < MOTIVO_MINIMO}
                    onClick={() =>
                      cobrar.mutate(
                        { id: doc.id, motivo },
                        {
                          onSuccess: () => {
                            toast.success("El saldo volvió al circuito de pago.");
                            setAbierto(false);
                            setMotivo("");
                          },
                          onError: (e) => toast.error(mensajeError(e)),
                        },
                      )
                    }
                  >
                    Mandar a cobrar
                  </Button>
                </div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/** Las notas ya aplicadas, con su vuelta. */
function NotasAplicadas({ doc, puedeQuitar }: { doc: Documento; puedeQuitar: boolean }) {
  const toast = useToast();
  const notasQ = useNotasDeFactura(doc.id);
  const reversar = useReversarNotaCredito();
  const moneda = (doc.moneda || "CRC") as "CRC" | "USD";

  return (
    <div className="rounded-lg border border-border bg-surface-muted/40 p-3">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-content-muted">
        Notas de crédito aplicadas
      </h3>
      <ul className="space-y-1.5 text-sm">
        {(notasQ.data ?? []).map((n) => (
          <li key={n.id} className="flex flex-wrap items-center gap-x-3">
            <span className="min-w-0 flex-1 truncate text-content">
              {n.consecutivo || "Nota de crédito"}
            </span>
            <span className="tabular-nums font-medium text-negativo">
              −{formatMoneda(n.monto, moneda)}
            </span>
            {puedeQuitar && (
              <Button
                variant="ghost"
                size="sm"
                loading={reversar.isPending}
                onClick={() =>
                  reversar.mutate(
                    { id: doc.id, aplicacionId: n.id },
                    {
                      onSuccess: () => toast.success("Se quitó la nota: la factura vuelve a deber ese monto."),
                      onError: (e) => toast.error(mensajeError(e)),
                    },
                  )
                }
              >
                Quitar
              </Button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Aplicar una nota de crédito del mismo proveedor. */
function AplicarNota({ doc, saldo }: { doc: Documento; saldo: number }) {
  const toast = useToast();
  const disponiblesQ = useNotasDeCreditoDisponibles(doc.proveedor_id);
  const aplicar = useAplicarNotaCredito();
  const [elegida, setElegida] = useState<NotaCreditoSaldo | null>(null);
  const [monto, setMonto] = useState("");

  // Solo las del MISMO proveedor y la MISMA moneda: una nota rebaja lo que le debemos a quien la
  // emitió, y no se convierte entre monedas. Filtrarlas acá evita ofrecer lo que el servidor va a
  // rechazar — una lista que propone lo imposible es peor que una lista corta.
  const disponibles = (disponiblesQ.data ?? []).filter((n) => n.moneda === (doc.moneda || "CRC"));
  const moneda = (doc.moneda || "CRC") as "CRC" | "USD";

  if (disponibles.length === 0) {
    return (
      <p className="text-xs text-content-muted">
        Este proveedor no tiene notas de crédito con saldo. Cuando emita una, se carga como
        documento y desde acá se descuenta de la factura que corrija.
      </p>
    );
  }

  return (
    <div className="rounded-lg border border-border p-3">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-content-muted">
        Aplicar una nota de crédito
      </h3>
      <ul className="mb-3 space-y-1">
        {disponibles.map((n) => (
          <li key={n.id}>
            <button
              type="button"
              onClick={() => {
                setElegida(n);
                // Se sugiere el menor entre lo que la nota tiene y lo que la factura debe: pedir
                // más de cualquiera de los dos lo rechaza el servidor.
                const sug = Math.min(toNumber(n.saldo), saldo);
                setMonto(sug > 0 ? String(sug) : "");
              }}
              className={
                "flex w-full flex-wrap items-center gap-x-3 rounded-lg border px-3 py-2 text-left text-sm " +
                (elegida?.id === n.id
                  ? "border-accent bg-accent/5"
                  : "border-border hover:bg-surface-muted")
              }
            >
              <span className="min-w-0 flex-1 truncate text-content">
                {n.consecutivo || n.clave || "Nota de crédito"}
              </span>
              <span className="text-xs text-content-muted">{n.fecha_emision}</span>
              <Badge tone="neutral">disponible {formatMoneda(n.saldo, moneda)}</Badge>
            </button>
          </li>
        ))}
      </ul>

      {elegida && (
        <div className="flex flex-wrap items-end gap-3">
          <Input
            label="Cuánto de esa nota se descuenta"
            value={monto}
            onChange={(e) => setMonto(e.target.value)}
            className="w-44"
          />
          <Button
            loading={aplicar.isPending}
            disabled={!monto.trim()}
            onClick={() =>
              aplicar.mutate(
                { id: doc.id, notaId: elegida.id, monto },
                {
                  onSuccess: () => {
                    toast.success("Nota de crédito aplicada: la factura debe menos.");
                    setElegida(null);
                    setMonto("");
                  },
                  onError: (e) => toast.error(mensajeError(e)),
                },
              )
            }
          >
            Aplicar
          </Button>
          <span className="text-[11px] text-content-muted">
            No puede pasarse de lo que la nota tiene disponible ni de lo que la factura debe.
          </span>
        </div>
      )}
    </div>
  );
}
