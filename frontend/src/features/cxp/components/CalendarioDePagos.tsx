/**
 * «Calendario de pagos»: qué sale y cuándo, y qué venció esperando (16-nov-2026).
 *
 * EL PEDIDO: «un calendario que me indique dos modalidades: uno que ya está aprobado para pagar y
 * en qué fecha —así sea para este, el viernes que sigue o el otro— y qué está vencido que se
 * necesita pagar».
 *
 * ── POR QUÉ NO ES UNA CUADRÍCULA DE MES ──────────────────────────────────────────────────────
 *
 * Hay 38 facturas con fecha de pago repartidas en nueve días, y 4.471 sin ninguna. Un calendario
 * mensual sería una cuadrícula con tres casillas pintadas mientras el grueso del trabajo —lo
 * vencido sin fecha— no tendría dónde salir. Son dos preguntas y van en dos bloques.
 *
 * ── EL NÚMERO QUE IMPORTA DEL BLOQUE DE VENCIDO ──────────────────────────────────────────────
 *
 * Medido: de ₡998 M vencidos, solo ₡3,9 M se pueden pagar hoy; ₡984 M están en recepción sin
 * revisar. Por eso «listo para pagar» va destacado y aparte del total: un bloque que mostrara
 * ₡998 M diría que hay que pagar eso, cuando nadie puede, y escondería que lo accionable es
 * mover 4.301 facturas por el flujo.
 */
import {
  Badge,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  EmptyState,
  ErrorState,
  LoadingState,
} from "@/components/ui";
import type { GrupoVencido, TramoProgramado } from "@/api/cxp";
import { useCalendarioDePagos } from "@/features/cxp/hooks";
import { mensajeError } from "@/lib/apiError";
import { formatFecha, formatMoneda } from "@/lib/format";

/** El tono del tramo: lo atrasado se ve, lo de esta semana se destaca, lo demás acompaña. */
function tonoDelTramo(t: TramoProgramado): "negativo" | "pendiente" | "neutral" {
  if (t.atrasado) return "negativo";
  if (t.cuando === "esta semana") return "pendiente";
  return "neutral";
}

function FilaProgramada({ t }: { t: TramoProgramado }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border px-3 py-2">
      <div className="min-w-0">
        <span className="font-medium">{formatFecha(t.fecha)}</span>{" "}
        <span className="text-sm text-content-muted">({t.dia})</span>
        <Badge tone={tonoDelTramo(t)} className="ml-2">
          {t.cuando}
        </Badge>
      </div>
      <div className="text-right">
        <div className="font-medium tabular-nums">{formatMoneda(t.total_crc, "CRC")}</div>
        <div className="text-xs text-content-muted">{t.facturas} factura(s)</div>
      </div>
    </div>
  );
}

function FilaVencida({ g }: { g: GrupoVencido }) {
  const pagable = g.orden === 1;
  return (
    <div
      className={`flex flex-wrap items-center justify-between gap-2 rounded-md border px-3 py-2 ${
        pagable ? "border-positivo/40 bg-positivo/5" : "border-border"
      }`}
    >
      <div className="min-w-0">
        <span className="font-medium">{g.situacion}</span>
        {/* La acción, no el estado: «4.301 en RECIBIDO» no le dice a nadie qué hacer. */}
        <div className="text-sm text-content-muted">{g.que_falta}</div>
      </div>
      <div className="text-right">
        <div className={`tabular-nums ${pagable ? "font-semibold text-positivo" : "font-medium"}`}>
          {formatMoneda(g.total_crc, "CRC")}
        </div>
        <div className="text-xs text-content-muted">{g.facturas} factura(s)</div>
      </div>
    </div>
  );
}

export function CalendarioDePagos() {
  const q = useCalendarioDePagos();
  const cal = q.data;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Calendario de pagos</CardTitle>
        <CardDescription>
          Lo que ya tiene fecha y lo que venció esperando. <strong>No es lo mismo «vencido» que
          «vencido y se puede pagar»</strong>: el segundo bloque separa lo que está listo de lo que
          todavía falta tramitar.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {q.isPending && <LoadingState label="Armando el calendario" />}
        {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />}

        {cal && (
          <>
            <section className="space-y-2">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h3 className="text-sm font-semibold">Con fecha de pago</h3>
                <span className="text-sm text-content-muted">
                  {formatMoneda(cal.total_programado, "CRC")} en total
                </span>
              </div>
              {cal.programado.length === 0 ? (
                <EmptyState message="Todavía no hay facturas con fecha de pago asignada." />
              ) : (
                <div className="space-y-2">
                  {cal.programado.map((t) => (
                    <FilaProgramada key={t.fecha} t={t} />
                  ))}
                </div>
              )}
            </section>

            <section className="space-y-2 border-t border-border pt-4">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h3 className="text-sm font-semibold">Vencido sin fecha de pago</h3>
                <span className="text-sm text-content-muted">
                  {formatMoneda(cal.total_vencido, "CRC")} en total
                </span>
              </div>
              {/* EL NÚMERO DEL BLOQUE va primero y solo. Si alguien lee una sola cifra de acá,
                  tiene que ser esta: lo que se puede pagar sin esperar a nadie. */}
              <p className="rounded-md border border-positivo/40 bg-positivo/5 p-3 text-sm">
                De todo eso, <strong>{formatMoneda(cal.listo_para_pagar, "CRC")}</strong> está
                listo para pagarse: solo falta incluirlo en un corte. El resto está esperando algún
                paso del flujo.
              </p>
              {cal.vencido.length === 0 ? (
                <EmptyState message="No hay facturas vencidas sin fecha de pago." />
              ) : (
                <div className="space-y-2">
                  {cal.vencido.map((g) => (
                    <FilaVencida key={g.orden} g={g} />
                  ))}
                </div>
              )}
            </section>
          </>
        )}
      </CardContent>
    </Card>
  );
}
