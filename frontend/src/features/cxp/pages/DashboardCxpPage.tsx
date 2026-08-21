/**
 * CxP — Dashboard (/cxp/dashboard). Reescrito sobre la maqueta aprobada por el usuario.
 *
 * Dos naturalezas separadas a propósito, porque antes estaban mezcladas en un solo número:
 *   · CARTERA: lo que se debe HOY. No depende del período (filtrarla por mes escondería el
 *     arrastre, que es justo la deuda que hay que trabajar).
 *   · MOVIMIENTO: lo que entró y lo que se pagó en el PERÍODO del selector global.
 *
 * Cada paso del flujo tiene su propio número —y sale del mismo resumen que las pestañas de
 * la Bandeja, así las dos pantallas no pueden contradecirse— y cada cifra es navegable: del
 * indicador se llega a las facturas que lo componen.
 */

import { Link } from "react-router-dom";
import {
  Badge,
  Button,
  Card,
  CardContent,
  ErrorState,
  LoadingState,
  PageHeader,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { etiquetaPeriodo, formatFecha, formatMoneda, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { ETIQUETA_ESTADO, ESTADOS_TERMINALES, FLUJO_ESTADOS, TONO_ESTADO } from "@/features/cxp/dominio";
import { useDashboardCxp } from "@/features/cxp/hooks";
import { useTienePermiso } from "@/features/auth/permisos";
import type {
  CarteraCxp,
  ConteoEstado,
  Cubo,
  DashboardCxp,
  EstadoDocumento,
  FaseBandeja,
  MovimientoCxp,
  TramoClave,
  TramoVencimiento,
} from "@/api/cxp";

/** Cola: MISMAS fases que las pestañas de la Bandeja, rotuladas por la acción pendiente. */
const ETIQUETA_FASE: Record<FaseBandeja["fase"], string> = {
  rec: "Por revisar",
  val: "Por validar (área)",
  apr: "Por aprobar",
  pag: "Por pagar",
  bco: "En banco",
  pgd: "Pagadas",
  arc: "Archivo",
};

/** Orden del flujo en la cola de trabajo (el archivo va en la tabla de estados, no acá). */
const FASES_COLA: FaseBandeja["fase"][] = ["rec", "val", "apr", "pag", "bco"];

const ETIQUETA_TRAMO: Record<TramoClave, { titulo: string; nota: string }> = {
  v90: { titulo: "+90 días", nota: "vencido" },
  v61: { titulo: "61 a 90", nota: "vencido" },
  v31: { titulo: "31 a 60", nota: "vencido" },
  v1: { titulo: "1 a 30", nota: "vencido" },
  s7: { titulo: "Vence esta semana", nota: "hoy y 7 días" },
  s30: { titulo: "Vence en 30 días", nota: "al día" },
  futuro: { titulo: "Más de 30 días", nota: "al día" },
  sin_fecha: { titulo: "Sin fecha", nota: "sin vencimiento registrado" },
};

/**
 * Estados de cierre de la tabla: los que ya NO se deben. REBOTADA queda fuera a propósito
 * (sigue siendo cartera abierta) y se lista con el flujo. Entre las dos listas se cubren los
 * 11 estados posibles, así que el total de la tabla es el universo completo.
 */
const ESTADOS_CIERRE: EstadoDocumento[] = ESTADOS_TERMINALES.filter((e) => e !== "REBOTADA");

/** Color del tramo: rampa de severidad, independiente del acento de marca. */
const COLOR_TRAMO: Record<TramoClave, string> = {
  v90: "bg-negativo",
  v61: "bg-negativo/75",
  v31: "bg-pendiente/85",
  v1: "bg-pendiente/65",
  s7: "bg-accent/70",
  s30: "bg-accent/50",
  futuro: "bg-accent/35",
  sin_fecha: "bg-content-muted/40",
};

/** Ruta a la Bandeja con la fase y (opcional) el tramo de vencimiento ya aplicados. */
function irABandeja(fase: FaseBandeja["fase"], vencimiento?: string): string {
  const p = new URLSearchParams({ fase });
  if (vencimiento) p.set("vencimiento", vencimiento);
  return `/cxp/bandeja?${p.toString()}`;
}

/**
 * Ruta a la CARTERA ABIERTA de la Bandeja filtrada por tramo de vencimiento.
 *
 * Importante: los cortes de vencimiento cruzan todas las etapas del flujo, así que el
 * drill-down NO puede caer en una pestaña de fase — traería un subconjunto y el número del
 * tablero no cuadraría con la lista (que es justo el defecto que este tablero vino a
 * corregir). La pestaña «Cartera abierta» tiene exactamente la misma población.
 *
 * `puede` = tiene cxp.cartera_abierta. Sin ese permiso el tablero sigue mostrando el AGREGADO
 * (para eso está cxp.dashboard) pero no ofrece el enlace factura por factura, que es lo que el
 * permiso protege; el backend lo rechaza igual si alguien arma la URL a mano.
 */
function irACartera(vencimiento: string, puede: boolean): string | undefined {
  return puede ? `/cxp/bandeja?fase=abi&vencimiento=${encodeURIComponent(vencimiento)}` : undefined;
}

export function DashboardCxpPage() {
  const { periodo } = usePeriodoActivo();
  const q = useDashboardCxp(periodo);
  const actualizado = q.dataUpdatedAt ? new Date(q.dataUpdatedAt) : null;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Dashboard CxP"
        description={`Cartera al día de hoy · movimiento de ${etiquetaPeriodo(periodo)}`}
        actions={
          <div className="flex items-center gap-3">
            {actualizado && (
              <span className="text-xs tabular-nums text-content-muted">
                actualizado{" "}
                {actualizado.toLocaleTimeString("es-CR", { hour: "2-digit", minute: "2-digit", hour12: false })}
              </span>
            )}
            <Button variant="secondary" onClick={() => void q.refetch()} disabled={q.isFetching}>
              {q.isFetching ? "Actualizando…" : "Actualizar"}
            </Button>
          </div>
        }
      />

      {/* Un refetch fallido no borra un tablero que ya tenemos: se avisa y se deja lo último
          bueno en pantalla, que para decidir pagos es mejor que quedarse sin nada. */}
      {q.isError && q.data && (
        <Aviso tono="warn" icono="⚠️">
          No se pudo refrescar ({mensajeError(q.error)}). Estás viendo los últimos datos buenos.
        </Aviso>
      )}
      {q.isPending ? (
        <LoadingState label="Cargando el tablero" />
      ) : q.isError && !q.data ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />
      ) : q.data ? (
        <div className={cn(q.isPlaceholderData && "opacity-50 transition-opacity")}>
          <Contenido data={q.data} periodo={periodo} />
        </div>
      ) : null}
    </div>
  );
}

function Contenido({ data, periodo }: { data: DashboardCxp; periodo: string }) {
  const c = data.cartera;
  const vacio = data.total_documentos === 0;
  // El drill-down a la cartera completa (factura por factura) exige su propio permiso.
  const verCartera = useTienePermiso()("cxp.cartera_abierta");

  if (vacio) {
    return (
      <Card>
        <CardContent className="py-5 text-sm text-content-muted">
          {data.alcance_limitado ? (
            <>
              No hay facturas asignadas a <b>tu área</b>. La empresa puede tener otras: este tablero
              muestra solo tu alcance, igual que tu Bandeja.
            </>
          ) : (
            <>
              Todavía no hay facturas de proveedor en esta empresa. Empezá por{" "}
              <Link to="/cxp/importar" className="font-medium text-accent underline">
                Importar facturación
              </Link>{" "}
              y el tablero se llena solo.
            </>
          )}
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {data.alcance_limitado && (
        <Aviso tono="info" icono="🔐">
          Estás viendo <b>solo las facturas de tu área</b>, igual que en tu Bandeja. Los totales no son
          los de la empresa completa.
        </Aviso>
      )}

      {/* 1. La plata: cartera y vencimientos (a hoy, no del período) */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[10.5px] font-bold uppercase tracking-wider text-content-muted">Cartera</span>
        <Badge tone="accent">al {formatFecha(data.hoy)} · no depende del período</Badge>
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Kpi
          label="Por pagar · cartera abierta"
          valor={formatMoneda(c.abierta.monto, "CRC")}
          detalle={`${c.abierta.cantidad.toLocaleString("es-CR")} facturas · ${pct(c.vencido.monto, c.abierta.monto)} ya vencido`}
          destacado
        />
        <Kpi
          label="Vencido"
          valor={formatMoneda(c.vencido.monto, "CRC")}
          detalle={
            c.vencido.cantidad > 0
              ? `${c.vencido.cantidad.toLocaleString("es-CR")} facturas · la más vieja, ${c.dias_mas_antigua} días`
              : "nada vencido"
          }
          tono="negativo"
          to={irACartera("vencido", verCartera)}
        />
        <Kpi
          label="Vence esta semana"
          valor={formatMoneda(c.vence_semana.monto, "CRC")}
          detalle={`${c.vence_semana.cantidad} facturas · hoy y los próximos 7 días`}
          tono="pendiente"
          to={irACartera("s7", verCartera)}
        />
        <Kpi
          label="Rebotadas por el banco"
          valor={formatMoneda(c.rebotadas.monto, "CRC")}
          detalle={
            c.rebotadas.cantidad > 0
              ? `${c.rebotadas.cantidad} facturas · hay que reintentarlas`
              : "sin rebotes pendientes"
          }
          tono={c.rebotadas.cantidad > 0 ? "negativo" : "neutral"}
          to={irABandeja("bco")}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Aging cartera={c} verCartera={verCartera} />
        <Concentracion cartera={c} />
      </div>

      {/* 2. Cola de trabajo: cada paso del flujo con su propio número */}
      <Cola cola={data.cola ?? []} />

      {/* 3. Movimiento del período */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <MovimientoCard movimiento={data.movimiento} periodo={periodo} />
        <SerieCard movimiento={data.movimiento} />
        <CicloCard movimiento={data.movimiento} />
      </div>

      {/* 4. Universo completo por estado + trabas */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <EstadosCard data={data} />
        <TrabasCard cartera={c} proveedores={data.proveedores_activos} alcanceLimitado={data.alcance_limitado} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Piezas
// ---------------------------------------------------------------------------

function pct(parte: string, total: string): string {
  const t = toNumber(total);
  if (t === 0) return "0 %";
  return `${Math.round((toNumber(parte) / t) * 100)} %`;
}

/** Porcentaje con dos decimales en formato de Costa Rica (coma decimal). */
function pctExacto(parte: number, total: number): string {
  if (total === 0) return "0,00 %";
  return `${((parte / total) * 100).toLocaleString("es-CR", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} %`;
}

/** Mes abreviado de un período YYYY-MM (etiqueta del eje de la serie). */
const MES_CORTO = ["Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Set", "Oct", "Nov", "Dic"];
function mesCorto(periodo: string): string {
  const mes = Number(periodo.slice(5, 7));
  return MES_CORTO[mes - 1] ?? periodo.slice(5);
}

/** Días del ciclo con coma decimal (el backend los manda como decimal en string). */
function diasLegibles(valor: string): string {
  const n = toNumber(valor);
  return n.toLocaleString("es-CR", { minimumFractionDigits: 1, maximumFractionDigits: 1 });
}

function Aviso({
  tono,
  icono,
  children,
}: {
  tono: "info" | "warn" | "legal";
  icono: string;
  children: React.ReactNode;
}) {
  const estilo =
    tono === "warn"
      ? "border-pendiente/40 bg-pendiente/10"
      : tono === "legal"
        ? "border-brand-gold/40 bg-brand-gold/10"
        : "border-accent/30 bg-accent/5";
  return (
    <div className={cn("flex gap-2 rounded-xl border px-3 py-2.5 text-xs leading-relaxed", estilo)}>
      <span aria-hidden>{icono}</span>
      <span>{children}</span>
    </div>
  );
}

interface KpiProps {
  label: string;
  valor: string;
  detalle: string;
  tono?: "neutral" | "negativo" | "pendiente" | "positivo";
  destacado?: boolean;
  to?: string;
}

function Kpi({ label, valor, detalle, tono = "neutral", destacado, to }: KpiProps) {
  const color =
    tono === "negativo"
      ? "text-negativo"
      : tono === "pendiente"
        ? "text-pendiente"
        : tono === "positivo"
          ? "text-positivo"
          : destacado
            ? "text-accent"
            : "text-content";
  const cuerpo = (
    <Card
      className={cn(
        "h-full",
        destacado && "border-accent/60 bg-accent/5",
        to && "transition-colors hover:border-accent",
      )}
    >
      <CardContent className="py-4">
        <p className="flex items-center gap-1 text-xs uppercase tracking-wide text-content-muted">
          {label}
          {to && <span aria-hidden className="ml-auto text-content-muted">→</span>}
        </p>
        <p className={cn("mt-1 text-2xl font-semibold tabular-nums", color)}>{valor}</p>
        <p className="mt-2 text-xs text-content-muted">{detalle}</p>
      </CardContent>
    </Card>
  );
  return to ? (
    <Link to={to} className="block">
      {cuerpo}
    </Link>
  ) : (
    cuerpo
  );
}

function Aging({ cartera, verCartera }: { cartera: CarteraCxp; verCartera: boolean }) {
  const tramos = cartera.tramos ?? [];
  const max = Math.max(...tramos.map((t) => toNumber(t.monto)), 1);
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <h3 className="font-semibold">Antigüedad de la cartera por pagar</h3>
        <p className="text-xs text-content-muted">
          Monto <b>neto</b> (lo que sale del banco). Clic en un tramo para ver esas facturas.
        </p>
        <div className="mt-3 flex flex-col gap-2">
          {tramos.map((t) => (
            <TramoFila key={t.clave} tramo={t} max={max} verCartera={verCartera} />
          ))}
        </div>
        {cartera.prioridad_aa.cantidad > 0 && (
          <div className="mt-3">
            <Aviso tono="warn" icono="🚩">
              <b>Prioridad AA (se paga sí o sí):</b> {cartera.prioridad_aa.cantidad}{" "}
              {cartera.prioridad_aa.cantidad === 1 ? "factura" : "facturas"} por{" "}
              {formatMoneda(cartera.prioridad_aa.monto, "CRC")}
              {cartera.aa_vencidas > 0 &&
                (cartera.prioridad_aa.cantidad === 1 ? (
                  <>
                    , y <b>ya está vencida</b>
                  </>
                ) : (
                  <>
                    , de las cuales <b>{cartera.aa_vencidas} ya están vencidas</b>
                  </>
                ))}
              .
            </Aviso>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function TramoFila({ tramo, max, verCartera }: { tramo: TramoVencimiento; max: number; verCartera: boolean }) {
  // Con respaldo por si el backend agrega un tramo: mejor una fila sin decorar que un
  // tablero en blanco por un índice inexistente.
  const et = ETIQUETA_TRAMO[tramo.clave] ?? { titulo: tramo.clave, nota: "" };
  const color = COLOR_TRAMO[tramo.clave] ?? "bg-content-muted/40";
  const ancho = max > 0 ? (toNumber(tramo.monto) / max) * 100 : 0;
  const to = irACartera(tramo.clave, verCartera);
  const clases = cn(
    "grid grid-cols-[7.5rem_1fr_8rem] items-center gap-3 rounded-lg px-1 py-0.5",
    to && "hover:bg-surface-muted",
  );
  const cuerpo = (
    <>
      <span className="text-xs">
        <b className="block font-semibold">{et.titulo}</b>
        <span className="text-[10.5px] text-content-muted">{et.nota}</span>
      </span>
      <span className="h-5 overflow-hidden rounded-md bg-surface-muted">
        <span
          className={cn("block h-full rounded-md", color)}
          style={{ width: `${Math.max(ancho, tramo.cantidad > 0 ? 1.5 : 0)}%` }}
        />
      </span>
      <span className="text-right text-xs">
        <b className="block font-semibold tabular-nums">{formatMoneda(tramo.monto, "CRC")}</b>
        <span className="text-[10.5px] tabular-nums text-content-muted">
          {tramo.cantidad.toLocaleString("es-CR")} facturas
        </span>
      </span>
    </>
  );
  // Sin permiso a la cartera abierta la fila se ve igual pero no navega: el tramo (agregado)
  // es parte del tablero; la lista de facturas que lo compone, no.
  return to ? (
    <Link to={to} className={clases}>
      {cuerpo}
    </Link>
  ) : (
    <div className={clases}>{cuerpo}</div>
  );
}

function Concentracion({ cartera }: { cartera: CarteraCxp }) {
  const top = cartera.top_proveedores ?? [];
  const suma = top.reduce((a, p) => a + toNumber(p.monto), 0);
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <h3 className="font-semibold">Concentración: a quién le debemos</h3>
        <p className="text-xs text-content-muted">
          {top.length > 0
            ? `Los ${top.length} proveedores más grandes concentran el ${pct(String(suma), cartera.abierta.monto)} de la cartera abierta.`
            : "Sin cartera abierta."}
        </p>
        <div className="mt-2 flex flex-col">
          {top.map((p) => (
            <div
              key={p.nombre}
              className="flex items-center justify-between gap-3 border-b border-dashed border-border py-1.5 text-xs last:border-0"
            >
              <span className="min-w-0 flex-1 truncate" title={p.nombre}>
                {p.nombre}
                {p.vencidos > 0 && (
                  <Badge tone="negativo" className="ml-2">
                    {p.vencidos} de {p.cantidad} vencidas
                  </Badge>
                )}
              </span>
              <b className="shrink-0 tabular-nums">{formatMoneda(p.monto, "CRC")}</b>
            </div>
          ))}
        </div>
        <p className="mt-3 border-t border-border pt-2 text-[11px] text-content-muted">
          Cartera bruta {formatMoneda(cartera.bruto, "CRC")} − retención{" "}
          {formatMoneda(cartera.retencion, "CRC")} − anticipos aplicados{" "}
          {formatMoneda(cartera.anticipos, "CRC")} = <b>{formatMoneda(cartera.abierta.monto, "CRC")}</b> netos a pagar.
        </p>
      </CardContent>
    </Card>
  );
}

function Cola({ cola }: { cola: FaseBandeja[] }) {
  const porFase = new Map(cola.map((f) => [f.fase, f]));
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <h3 className="font-semibold">Cola de trabajo</h3>
        <p className="text-xs text-content-muted">
          Cada paso del flujo con su propio número, con la misma definición y el mismo monto que las
          pestañas de la Bandeja (el <b>total de la factura</b>, no el neto). Clic para ver los documentos.
        </p>
        <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
          {FASES_COLA.map((fase, i) => {
            const f = porFase.get(fase);
            const cantidad = f?.cantidad ?? 0;
            return (
              <Link
                key={fase}
                to={irABandeja(fase)}
                className={cn(
                  "relative min-w-[8.5rem] flex-1 rounded-xl border px-3 py-2.5 transition-colors hover:border-accent",
                  cantidad === 0 ? "border-dashed border-border bg-surface-muted" : "border-border bg-surface-raised",
                )}
              >
                <p className="text-[10.5px] font-bold uppercase tracking-wide text-content-muted">
                  {ETIQUETA_FASE[fase]}
                </p>
                <p className="mt-1 text-xl font-semibold tabular-nums">{cantidad.toLocaleString("es-CR")}</p>
                <p className="text-[11px] tabular-nums text-content-muted">
                  {formatMoneda(f?.monto ?? "0", "CRC")}
                </p>
                {i < FASES_COLA.length - 1 && (
                  <span aria-hidden className="absolute -right-[9px] top-1/2 hidden -translate-y-1/2 text-content-muted sm:block">
                    →
                  </span>
                )}
              </Link>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}

function MovimientoCard({ movimiento, periodo }: { movimiento: MovimientoCxp; periodo: string }) {
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <h3 className="font-semibold">Movimiento de {etiquetaPeriodo(periodo)}</h3>
        <p className="text-xs text-content-muted">Este bloque sí obedece al selector de período de la barra.</p>
        <div className="mt-3 grid grid-cols-2 gap-3">
          <Mini label="Recibidas" valor={movimiento.recibidas.cantidad} monto={movimiento.recibidas.monto} />
          <Mini
            label="Pagadas"
            valor={movimiento.pagadas.cantidad}
            monto={movimiento.pagadas.monto}
            tono="positivo"
          />
        </div>
        <p className="mt-3 text-[11px] text-content-muted">
          «Recibidas» por fecha de emisión de la factura; «pagadas» por la <b>fecha real del pago</b> (el
          evento de auditoría), no por la última vez que alguien tocó el documento.
        </p>
        {movimiento.pagadas_sin_evento > 0 && (
          <p className="text-[11px] text-content-muted">
            Aparte: {movimiento.pagadas_sin_evento} facturas pagadas <b>en toda la historia</b> (no de este
            mes) no tienen evento de pago, así que no se pueden fechar ni entran en el ciclo.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function Mini({
  label,
  valor,
  monto,
  tono = "neutral",
}: {
  label: string;
  valor: number;
  monto: string;
  tono?: "neutral" | "positivo";
}) {
  return (
    <div className="rounded-lg border border-border px-3 py-2">
      <p className="text-[10.5px] font-bold uppercase tracking-wide text-content-muted">{label}</p>
      <p
        className={cn(
          "mt-0.5 text-lg font-semibold tabular-nums",
          tono === "positivo" ? "text-positivo" : "text-content",
        )}
      >
        {valor.toLocaleString("es-CR")}
      </p>
      <p className="text-[11px] tabular-nums text-content-muted">{formatMoneda(monto, "CRC")}</p>
    </div>
  );
}

function SerieCard({ movimiento }: { movimiento: MovimientoCxp }) {
  const serie = movimiento.serie ?? [];
  const max = Math.max(...serie.map((p) => p.cantidad), 1);
  const { setPeriodo } = usePeriodoActivo();
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <h3 className="font-semibold">Facturas recibidas por mes</h3>
        <p className="text-xs text-content-muted">
          Por fecha de emisión. Clic en una barra para cambiar el período.
        </p>
        <div className="mt-3 flex h-28 items-end gap-1.5">
          {serie.map((p) => (
            <button
              key={p.periodo}
              type="button"
              onClick={() => setPeriodo(p.periodo)}
              title={`${etiquetaPeriodo(p.periodo)}: ${p.cantidad} facturas por ${formatMoneda(p.monto, "CRC")}`}
              className="flex flex-1 flex-col items-center justify-end gap-1"
            >
              <span
                className={cn(
                  "w-3/4 rounded-t-md transition-opacity",
                  p.en_curso ? "bg-brand-gold" : "bg-accent/40 hover:bg-accent/70",
                )}
                style={{ height: `${Math.max((p.cantidad / max) * 100, 2)}%` }}
              />
              <span className="text-[10px] text-content-muted">{mesCorto(p.periodo)}</span>
              <span className="text-[10px] font-semibold tabular-nums text-content-muted">{p.cantidad}</span>
            </button>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function CicloCard({ movimiento }: { movimiento: MovimientoCxp }) {
  const dias = toNumber(movimiento.ciclo_dias);
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <h3 className="font-semibold">Tiempo del ciclo</h3>
        <p className="text-xs text-content-muted">
          De la emisión de la factura al pago efectivo, sobre lo pagado en el período.
        </p>
        {movimiento.pagadas.cantidad === 0 ? (
          <p className="mt-4 text-sm text-content-muted">
            No hubo pagos en este período, así que no hay ciclo que medir.
          </p>
        ) : (
          <>
            <p className="mt-3 text-3xl font-semibold tabular-nums">
              {diasLegibles(movimiento.ciclo_dias)}{" "}
              <span className="text-sm font-medium text-content-muted">días</span>
            </p>
            <p className="text-xs text-content-muted">
              promedio de las {movimiento.pagadas.cantidad} facturas pagadas en el período
            </p>
            {dias > 30 && (
              <div className="mt-3">
                <Aviso tono="warn" icono="⏱">
                  Se mide desde la <b>emisión</b> de la factura. El indicador anterior medía desde la carga
                  al sistema y mostraba un proceso mucho más corto del real.
                </Aviso>
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function EstadosCard({ data }: { data: DashboardCxp }) {
  const lista = data.por_estado ?? [];
  const porEstado = new Map(lista.map((c) => [c.estado, c]));
  const total = data.total_documentos;
  const fila = (e: EstadoDocumento) => {
    const c: ConteoEstado | undefined = porEstado.get(e);
    const cantidad = c?.cantidad ?? 0;
    return (
      <TR key={e}>
        <TD>
          <Badge tone={TONO_ESTADO[e]}>{ETIQUETA_ESTADO[e]}</Badge>
        </TD>
        <TD className="text-right tabular-nums">{cantidad.toLocaleString("es-CR")}</TD>
        <TD className="text-right tabular-nums">{formatMoneda(c?.monto ?? "0", "CRC")}</TD>
        <TD className="text-right tabular-nums text-content-muted">{pctExacto(cantidad, total)}</TD>
      </TR>
    );
  };
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <h3 className="font-semibold">Todos los documentos, sin que ninguno desaparezca</h3>
        <p className="text-xs text-content-muted">
          Los estados de cierre van aparte pero <b>visibles</b>: antes sumaban al total sin dibujarse y
          achicaban todos los porcentajes.
        </p>
        <TableContainer className="mt-2">
          <Table>
            <THead>
              <TR>
                <TH>Estado</TH>
                <TH className="text-right">Documentos</TH>
                <TH className="text-right">Monto</TH>
                <TH className="text-right">% del total</TH>
              </TR>
            </THead>
            <TBody>
              {FLUJO_ESTADOS.map(fila)}
              {/* REBOTADA va con el flujo, no con el cierre: el banco la devolvió, se sigue
                  debiendo y cuenta en la cartera abierta (igual que en la Bandeja, donde vive
                  en «En banco»). Ponerla abajo la hacía parecer un caso cerrado. */}
              {fila("REBOTADA")}
              <TR>
                <TD colSpan={4} className="bg-surface-muted text-[10.5px] font-bold uppercase tracking-wide text-content-muted">
                  Cierre (ya no se deben)
                </TD>
              </TR>
              {ESTADOS_CIERRE.map(fila)}
              <TR className="border-t-2 border-border bg-surface-muted font-semibold">
                <TD>Total</TD>
                <TD className="text-right tabular-nums">{total.toLocaleString("es-CR")}</TD>
                <TD className="text-right tabular-nums">{formatMoneda(data.total_monto, "CRC")}</TD>
                <TD className="text-right tabular-nums">100,00 %</TD>
              </TR>
            </TBody>
          </Table>
        </TableContainer>
      </CardContent>
    </Card>
  );
}

function TrabasCard({
  cartera,
  proveedores,
  alcanceLimitado,
}: {
  cartera: CarteraCxp;
  proveedores: number;
  alcanceLimitado: boolean;
}) {
  const traba = (c: Cubo, total: number) => (total > 0 ? `${Math.round((c.cantidad / total) * 100)} %` : "0 %");
  return (
    <Card>
      <CardContent className="flex flex-col gap-1 py-4">
        <h3 className="font-semibold">Lo que traba la cola</h3>
        <p className="text-xs text-content-muted">
          Por qué la cartera no avanza, y qué hay que hacer primero para poder trabajarla.
        </p>
        <div className="mt-3 grid grid-cols-2 gap-3">
          {/* Con alcance por área el conteo sería 0 por construcción (una factura sin área no
              cae en ningún departamento visible): mostrarlo sería un cero engañoso. */}
          {!alcanceLimitado && (
            <div className="rounded-lg border border-border px-3 py-2">
              <p className="text-[10.5px] font-bold uppercase tracking-wide text-content-muted">Sin departamento</p>
              <p className="mt-0.5 text-lg font-semibold tabular-nums text-pendiente">
                {cartera.sin_departamento.cantidad.toLocaleString("es-CR")}
              </p>
              <p className="text-[11px] text-content-muted">
                {traba(cartera.sin_departamento, cartera.abierta.cantidad)} · nadie las puede validar
              </p>
            </div>
          )}
          <div className="rounded-lg border border-border px-3 py-2">
            <p className="text-[10.5px] font-bold uppercase tracking-wide text-content-muted">Sin clasificar</p>
            <p className="mt-0.5 text-lg font-semibold tabular-nums text-pendiente">
              {cartera.sin_clasificar.cantidad.toLocaleString("es-CR")}
            </p>
            <p className="text-[11px] text-content-muted">
              {traba(cartera.sin_clasificar, cartera.abierta.cantidad)} · sin concepto de gasto
            </p>
          </div>
        </div>
        <p className="mt-3 border-t border-border pt-2 text-[11px] text-content-muted">
          {proveedores.toLocaleString("es-CR")} proveedores activos en el maestro.
        </p>
        <div className="mt-2 flex flex-wrap gap-2">
          <Link to={irABandeja("rec")}>
            <Button size="sm" variant="secondary">
              Asignar área y clasificar →
            </Button>
          </Link>
          <Link to="/cxp/departamentos">
            <Button size="sm" variant="secondary">
              Departamentos →
            </Button>
          </Link>
        </div>
      </CardContent>
    </Card>
  );
}
