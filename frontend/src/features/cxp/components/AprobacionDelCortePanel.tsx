/**
 * La aprobación del corte: el paso que faltaba entre armar el listado y pagar (mig 0123).
 *
 * EL PEDIDO (Director Financiero, 13-nov-2026): «ese listado se pasa a validación a la gerencia
 * financiera para que vea lo que va a pagarse, cuánto y todos los pormenores… pero hay
 * posibilidades que a veces una de esas facturas se rechacen».
 *
 * ── EL CONTROL ESTÁ EN LA MACRO, NO EN ESTA PANTALLA ─────────────────────────────────────────
 *
 * Lo que hace que este flujo no sea ceremonia es que el servidor no entrega el archivo del banco
 * mientras el corte no esté APROBADO. Antes se bajaba de un corte ABIERTO y nadie había dicho que
 * sí. Esta pantalla es la puerta; la cerradura está del otro lado.
 *
 * ── POR QUÉ DEVOLVER Y EXCLUIR SON DOS BOTONES ───────────────────────────────────────────────
 *
 * Son dos respuestas distintas y confundirlas cuesta trabajo:
 *   · EXCLUIR saca UNA factura y deja el resto del corte vivo. Es lo normal: «esta no, todavía».
 *   · DEVOLVER rechaza el corte ENTERO. Es para cuando lo que está mal es el corte —la fecha, el
 *     banco de origen, la mitad de las líneas— y arreglarlo de a una sería más lento.
 *
 * Un solo verbo obligaría a excluir veinte facturas de a una para rechazar un corte.
 *
 * ── Y POR QUÉ EXCLUIR NO DICE «DENEGAR» ──────────────────────────────────────────────────────
 *
 * La factura vuelve a la bandeja en PROGRAMADO, con su aprobación y su validación de área
 * intactas: entra al próximo corte sin que nadie la vuelva a tramitar. Llamarlo «denegar» haría
 * creer que hay que empezarla de cero, que es justo el reproceso que esto evita.
 */
import { useState } from "react";

import {
  Badge,
  Button,
  ErrorState,
  Input,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import type { LotePago } from "@/api/cxp";
import {
  useAprobarLote,
  useDevolverLote,
  useExcluidasDelLote,
  useMandarLoteAAprobar,
} from "@/features/cxp/hooks";
import { useTienePermiso } from "@/features/auth/permisos";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda } from "@/lib/format";

/** El largo mínimo del motivo, el MISMO que exige el servidor. */
const MOTIVO_MINIMO = 20;

/** Una línea del rastro: quién hizo qué y cuándo. */
function Paso({ rotulo, quien, cuando }: { rotulo: string; quien?: string; cuando?: string }) {
  if (!cuando) return null;
  return (
    <span className="text-xs text-content-muted">
      {rotulo} <b className="text-content">{quien || "—"}</b>
      {" · "}
      {cuando.slice(0, 16).replace("T", " ")}
    </span>
  );
}

export function AprobacionDelCortePanel({ lote }: { lote: LotePago }) {
  const toast = useToast();
  const tienePermiso = useTienePermiso();
  const puedeAprobar = tienePermiso("cxp.aprobar_lote");
  const puedeTesoreria = tienePermiso("cxp.tesoreria");

  const mandar = useMandarLoteAAprobar();
  const aprobar = useAprobarLote();
  const devolver = useDevolverLote();
  const excluidasQ = useExcluidasDelLote(lote.id);

  const [motivo, setMotivo] = useState("");
  const [devolviendo, setDevolviendo] = useState(false);

  const excluidas = excluidasQ.data?.items ?? [];
  const motivoCorto = motivo.trim().length < MOTIVO_MINIMO;

  function error(e: unknown) {
    toast.error(mensajeError(e));
  }

  const estado = lote.estado;

  return (
    <div className="rounded-lg border border-border">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border bg-surface-muted/60 px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">Aprobación del corte</span>
          {estado === "ABIERTO" && <Badge tone="neutral">Sin mandar a aprobar</Badge>}
          {estado === "EN_APROBACION" && <Badge tone="pendiente">Esperando a la gerencia</Badge>}
          {estado === "APROBADO" && <Badge tone="positivo">Aprobado</Badge>}
          {estado === "CERRADO" && <Badge tone="neutral">Pagado</Badge>}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {estado === "ABIERTO" && puedeTesoreria && (
            <Button
              size="sm"
              onClick={() =>
                mandar.mutate(lote.id, {
                  onSuccess: () => toast.success("El corte quedó esperando a la gerencia"),
                  onError: error,
                })
              }
              loading={mandar.isPending}
            >
              Mandar a aprobar
            </Button>
          )}
          {estado === "EN_APROBACION" && puedeAprobar && (
            <>
              <Button
                size="sm"
                onClick={() =>
                  aprobar.mutate(lote.id, {
                    onSuccess: () => toast.success("Corte aprobado: ya se puede bajar el archivo del banco"),
                    onError: error,
                  })
                }
                loading={aprobar.isPending}
              >
                Aprobar el corte
              </Button>
              <Button size="sm" variant="secondary" onClick={() => setDevolviendo((v) => !v)}>
                Devolver
              </Button>
            </>
          )}
        </div>
      </div>

      <div className="space-y-3 px-4 py-3">
        {/* EL AVISO DE QUE LA MACRO ESTÁ TRANCADA va primero: es la consecuencia práctica del
            estado, y sin decirlo alguien busca el botón de la macro y cree que se rompió. */}
        {(estado === "ABIERTO" || estado === "EN_APROBACION") && (
          <p className="rounded-md border border-pendiente/40 bg-pendiente/5 p-3 text-sm">
            El archivo del banco no se puede bajar hasta que la gerencia apruebe este corte.
            {estado === "ABIERTO" && " Todavía no se le mandó."}
          </p>
        )}

        {/* El rastro: quién hizo cada paso. Es lo que convierte esto en respaldo. */}
        <div className="flex flex-col gap-1">
          <Paso rotulo="Mandado a aprobar por" quien={lote.enviado_aprobacion_por} cuando={lote.enviado_aprobacion_en} />
          <Paso rotulo="Devuelto por" quien={lote.devuelto_por} cuando={lote.devuelto_en} />
          {lote.devolucion_motivo && (
            <span className="text-xs text-content-muted">
              Lo que había que corregir: <i>{lote.devolucion_motivo}</i>
            </span>
          )}
          <Paso rotulo="Aprobado por" quien={lote.aprobado_por} cuando={lote.aprobado_en} />
        </div>

        {devolviendo && estado === "EN_APROBACION" && (
          <div className="flex flex-wrap items-end gap-2 rounded-md border border-border p-3">
            <div className="min-w-[18rem] flex-1">
              <label className="mb-1 block text-xs text-content-muted" htmlFor="motivo-devolucion">
                ¿Qué hay que corregir? (lo lee quien rehace el corte)
              </label>
              <Input
                id="motivo-devolucion"
                value={motivo}
                onChange={(e) => setMotivo(e.target.value)}
                placeholder="Ej.: la fecha de corte es del viernes pasado, rehacelo con la de esta semana"
              />
            </div>
            <Button
              variant="secondary"
              disabled={motivoCorto || devolver.isPending}
              onClick={() =>
                devolver.mutate(
                  { loteId: lote.id, motivo: motivo.trim() },
                  {
                    onSuccess: () => {
                      toast.success("Corte devuelto a tesorería");
                      setMotivo("");
                      setDevolviendo(false);
                    },
                    onError: error,
                  },
                )
              }
            >
              Devolver el corte
            </Button>
            {motivoCorto && motivo.length > 0 && (
              <p className="w-full text-xs text-pendiente">
                Escribí un poco más: quien lo rehaga tiene que saber qué arreglar, o lo manda igual
                otra vez.
              </p>
            )}
          </div>
        )}

        {excluidasQ.isError && <ErrorState message={mensajeError(excluidasQ.error)} />}

        {excluidas.length > 0 && (
          <div className="space-y-2">
            <p className="text-sm">
              <b>{excluidas.length} factura(s) salieron de este corte.</b> Volvieron a la bandeja
              con su aprobación intacta: entran al próximo sin volver a tramitarse.
            </p>
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Proveedor</TH>
                    <TH className="text-right">Monto</TH>
                    <TH>Por qué salió</TH>
                    <TH>Quién</TH>
                  </TR>
                </THead>
                <TBody>
                  {excluidas.map((e) => (
                    <TR key={e.documento_id}>
                      <TD>
                        {e.proveedor}
                        <span className="ml-2 font-mono text-xs text-content-muted">{e.consecutivo}</span>
                      </TD>
                      <TD className="text-right tabular-nums">{formatMoneda(e.monto_crc, "CRC")}</TD>
                      <TD className="text-sm">{e.motivo}</TD>
                      <TD className="text-xs text-content-muted">
                        {e.excluido_por}
                        <br />
                        {e.excluido_en}
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          </div>
        )}
      </div>
    </div>
  );
}
