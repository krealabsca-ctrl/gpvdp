/**
 * «Cronograma de pago»: el cuadro que se manda ANTES de pagar (mig 0122).
 *
 * «Los cronogramas de pago se envían por correo con lo que se pretende pagar y los bancos de donde
 * salen los fondos.»
 *
 * LA CASCADA ES EL DOCUMENTO. Lo que se comunica no es la lista: es el último número de cuatro
 * restas encadenadas, y por eso el resumen va a la par de la tabla y no escondido al pie. En el
 * archivo real que esto reemplaza, de ₡52,1 M que salen del banco solo ₡10,9 M entran al
 * cronograma: quien mira la lista sin la cascada se lleva una idea cinco veces equivocada.
 *
 * Las tres decisiones se toman acá mismo, sobre la fila: de qué cuenta sale, si se paga por fuera,
 * y con qué tipo de cambio se colonizan los dólares.
 */
import { useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
  LoadingState,
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
import type { TotalesCronograma } from "@/api/cxp";
import { cxpApi } from "@/api/cxp";
import { useCronograma, useDecisionesDelCronograma } from "@/features/cxp/hooks";
import { useCuentas } from "@/features/bancos/hooks";
import { mensajeError } from "@/lib/apiError";
import { descargarBlob } from "@/lib/csv";
import { formatMoneda } from "@/lib/format";

/** Un escalón de la cascada. `resta` lo dibuja como lo que se le quita al de arriba. */
function Escalon({
  rotulo,
  monto,
  resta,
  destacado,
}: {
  rotulo: string;
  monto: string;
  resta?: boolean;
  destacado?: boolean;
}) {
  return (
    <div
      className={`flex items-center justify-between gap-4 px-3 py-1.5 text-sm ${
        destacado ? "rounded-md bg-surface-muted font-semibold" : ""
      }`}
    >
      <span className={resta ? "text-content-muted" : ""}>
        {resta && <span className="mr-1">−</span>}
        {rotulo}
      </span>
      <span className="tabular-nums">{formatMoneda(monto, "CRC")}</span>
    </div>
  );
}

function Cascada({ t }: { t: TotalesCronograma }) {
  return (
    <div className="space-y-1 rounded-lg border border-border p-3">
      <h3 className="px-3 pb-1 text-sm font-medium">Resumen del pago programado</h3>
      <Escalon rotulo="Total colones" monto={t.total_colones} />
      {t.total_dolares !== "0.00" && (
        <Escalon
          rotulo={`Total dólares colonizados (a ${t.tipo_cambio})`}
          monto={t.dolares_colonizados}
        />
      )}
      <Escalon rotulo="Total colonizado" monto={t.total_colonizado} destacado />
      <Escalon rotulo="Reserva" monto={t.reserva} resta />
      <Escalon rotulo="Total flujo normal" monto={t.flujo_normal} destacado />
      <Escalon rotulo="Solicitudes por fuera" monto={t.por_fuera} resta />
      {/* El número que se comunica. Va destacado porque es el único que sale en el correo. */}
      <Escalon rotulo="CRONOGRAMA TOTAL FLUJO NORMAL" monto={t.cronograma_total} destacado />
      {t.total_dolares !== "0.00" && (
        <Escalon rotulo="Cronograma colones flujo normal" monto={t.cronograma_en_colones} />
      )}
    </div>
  );
}

export function CronogramaPanel({ loteId }: { loteId: string }) {
  const toast = useToast();
  const q = useCronograma(loteId);
  const cuentasQ = useCuentas();
  const { tipoCambio, cuenta, porFuera } = useDecisionesDelCronograma(loteId);
  const [tc, setTc] = useState("");
  const [descargando, setDescargando] = useState(false);

  const cr = q.data;
  const cuentas = cuentasQ.data ?? [];
  const falta = cr?.totales.total_dolares !== "0.00" && cr?.totales.tipo_cambio === "0.0000";

  function error(e: unknown) {
    toast.error(mensajeError(e));
  }

  /** El archivo es lo que de verdad se manda: la pantalla sirve para decidir, esto para comunicar. */
  async function descargar() {
    setDescargando(true);
    try {
      const blob = await cxpApi.descargarCronogramaXLSX(loteId);
      const num = String(cr?.lote_numero ?? 0).padStart(4, "0");
      descargarBlob(`cronograma-pago-corte-${num}.xlsx`, blob);
    } catch (e) {
      error(e);
    } finally {
      setDescargando(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Cronograma de pago</CardTitle>
        <CardDescription>
          Lo que se pretende pagar y de qué bancos sale. Lo que se comunica es el{" "}
          <strong>cronograma total de flujo normal</strong>: lo que sale de cuentas de reserva y lo
          marcado «por fuera» se paga igual, pero no entra en ese número.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {q.isPending && <LoadingState label="Armando el cronograma" />}
        {q.isError && <ErrorState message={mensajeError(q.error)} />}

        {/* Sin tipo de cambio y con dólares, la cascada no se puede sumar: se pide antes de
            mostrar nada, porque un cuadro a medias se lee como si estuviera completo. */}
        {falta && (
          <div className="flex flex-wrap items-end gap-3 rounded-md border border-pendiente/40 bg-pendiente/5 p-3">
            <div>
              <label className="mb-1 block text-xs font-medium" htmlFor="tc-crono">
                Este corte tiene pagos en dólares: ¿a cuánto se colonizan?
              </label>
              <Input
                id="tc-crono"
                value={tc}
                onChange={(e) => setTc(e.target.value)}
                placeholder="450"
                className="w-32"
              />
            </div>
            <Button
              onClick={() =>
                tipoCambio.mutate(tc, {
                  onSuccess: () => toast.success("Tipo de cambio guardado"),
                  onError: error,
                })
              }
              disabled={tipoCambio.isPending || tc.trim() === ""}
            >
              Guardar
            </Button>
          </div>
        )}

        {cr && cr.lineas.length === 0 && (
          <p className="py-6 text-center text-sm text-content-muted">
            Este corte no tiene nada programado todavía.
          </p>
        )}

        {cr && cr.lineas.length > 0 && (
          <>
            {/* Las líneas sin cuenta cuentan como flujo normal; decirlo evita que un cuadro a
                medio asignar se lea como definitivo. */}
            {cr.sin_cuenta > 0 && (
              <p className="rounded-md bg-surface-muted p-3 text-sm text-content-muted">
                {cr.sin_cuenta} línea(s) todavía sin cuenta asignada. Por ahora cuentan como flujo
                normal, así que el total de abajo puede cambiar cuando las asignes.
              </p>
            )}

            <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
              <TableContainer>
                <Table>
                  <THead>
                    <TR>
                      <TH>Proveedor</TH>
                      <TH className="text-right">Monto</TH>
                      <TH>Sale de</TH>
                      <TH>En el cronograma</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {cr.lineas.map((l) => (
                      <TR key={`${l.proveedor_id}-${l.moneda}`}>
                        <TD className="font-medium">
                          {l.proveedor}
                          {l.moneda === "USD" && (
                            <span className="ml-2 text-xs text-content-muted">
                              USD {l.monto} → {formatMoneda(l.monto_colonizado, "CRC")}
                            </span>
                          )}
                        </TD>
                        <TD className="text-right tabular-nums">
                          {formatMoneda(l.monto, l.moneda === "USD" ? "USD" : "CRC")}
                        </TD>
                        <TD>
                          <Select
                            value=""
                            onChange={(e) =>
                              cuenta.mutate(
                                { proveedorId: l.proveedor_id, cuentaId: e.target.value },
                                { onError: error },
                              )
                            }
                            options={[
                              { value: "", label: l.cuenta || "— elegir cuenta —" },
                              ...cuentas.map((c) => ({
                                value: c.id,
                                label: `${c.banco} · ${c.alias}`,
                              })),
                            ]}
                            aria-label={`Cuenta de pago de ${l.proveedor}`}
                            className="min-w-44"
                          />
                          {l.es_reserva && (
                            <Badge tone="positivo" className="ml-2">
                              Reserva
                            </Badge>
                          )}
                        </TD>
                        <TD>
                          <label className="flex items-center gap-2 text-sm">
                            <input
                              type="checkbox"
                              checked={!l.por_fuera}
                              onChange={(e) =>
                                porFuera.mutate(
                                  { proveedorId: l.proveedor_id, porFuera: !e.target.checked },
                                  { onError: error },
                                )
                              }
                              aria-label={`${l.proveedor} entra en el cronograma`}
                            />
                            {l.por_fuera ? (
                              <span className="text-pendiente">Por fuera</span>
                            ) : (
                              <span className="text-content-muted">Entra</span>
                            )}
                          </label>
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </TableContainer>

              {/* La cascada va AL LADO de la tabla, no al pie: es lo que se comunica, y al pie de
                  una lista de cien proveedores no la ve nadie. */}
              <div className="space-y-3">
                <Cascada t={cr.totales} />
                {/* El archivo baja con la cascada adentro: lo que se manda no es la lista. */}
                <Button
                  variant="secondary"
                  className="w-full"
                  onClick={() => void descargar()}
                  loading={descargando}
                  disabled={falta}
                >
                  Descargar para enviar (.xlsx)
                </Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
