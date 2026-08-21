/**
 * Pantalla 3 — Dashboard (/), renovado en la Fase B (análisis visual).
 * KPIs con comparativo mensual + tendencia 12 meses (clic en un mes enfoca todo
 * el tablero), EBITDA mensual, calendario de flujo diario, desglose por cuenta
 * y cuadre por concepto con drill-down a movimientos.
 * Botón Cerrar período (bloqueante: 409 -> muestra No-identificados pendientes).
 */

import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  LoadingState,
  PageHeader,
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { etiquetaPeriodo, formatMoneda, formatMonto, toNumber } from "@/lib/format";
import { mensajeError, noIdentificadosDe } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import {
  useCalendarioDiario,
  useCerrarPeriodo,
  useCuadreArbol,
  useDashboard,
  usePeriodo,
  useResumenPorCuenta,
  useSerieMensual,
} from "@/features/bancos/hooks";
import { CuadreArbolView } from "@/features/bancos/components/CuadreArbol";
import { TendenciaChart } from "@/features/bancos/components/TendenciaChart";
import { EbitdaChart } from "@/features/bancos/components/EbitdaChart";
import { CalendarioFlujo } from "@/features/bancos/components/CalendarioFlujo";
import { CuentasResumenView } from "@/features/bancos/components/CuentasResumenView";
import { useChartColors } from "@/features/bancos/components/chartColors";
import type { DashboardData } from "@/api/bancos";

export function DashboardPage() {
  const toast = useToast();
  const { periodo, setPeriodo } = usePeriodoActivo();
  const colores = useChartColors();

  const dashboardQuery = useDashboard(periodo);
  const serieQuery = useSerieMensual(periodo);
  const calendarioQuery = useCalendarioDiario(periodo);
  const cuentasQuery = useResumenPorCuenta(periodo);
  const cuadreArbolQuery = useCuadreArbol(periodo);
  const periodoQuery = usePeriodo(periodo);
  const cerrar = useCerrarPeriodo();
  const [confirmarCierre, setConfirmarCierre] = useState(false);

  function cerrarPeriodo() {
    cerrar.mutate(periodo, {
      onSuccess: () => {
        toast.success(`Período ${etiquetaPeriodo(periodo)} cerrado.`);
        setConfirmarCierre(false);
      },
      onError: (err) => {
        const pendientes = noIdentificadosDe(err);
        if (pendientes !== null) {
          toast.error(
            `No se puede cerrar: quedan ${pendientes} movimiento(s) No-identificado(s) por clasificar.`,
          );
        } else {
          toast.error(mensajeError(err));
        }
        setConfirmarCierre(false);
      },
    });
  }

  const cerrado = periodoQuery.data?.cerrado ?? false;
  const noIdentificados = dashboardQuery.data?.no_identificados ?? 0;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Dashboard"
        description="El período de un vistazo: tendencia, flujo diario, cuentas y cuadre con drill-down."
        actions={
          <Button
            onClick={() => setConfirmarCierre(true)}
            disabled={cerrado}
            title={cerrado ? "El período ya está cerrado" : "Cerrar el período"}
          >
            {cerrado ? "Período cerrado" : "Cerrar período"}
          </Button>
        }
      />

      {confirmarCierre && (
        <ConfirmDialog
          titulo={`Cerrar ${etiquetaPeriodo(periodo)}`}
          descripcion="Vas a cerrar el período contable. Es una acción de alto impacto."
          impacto={[
            "Exige que el 100% de los movimientos estén clasificados (si quedan «No identificado», el cierre se rechaza).",
            "Reabrir un período cerrado requiere un rol autorizado y queda en la auditoría.",
            noIdentificados > 0
              ? `Ahora mismo hay ${noIdentificados.toLocaleString("es-CR")} movimiento(s) sin clasificar.`
              : "Todos los movimientos del período están clasificados.",
          ]}
          textoConfirmar="Cerrar período"
          pendiente={cerrar.isPending}
          onConfirmar={cerrarPeriodo}
          onCancelar={() => setConfirmarCierre(false)}
        />
      )}

      {/* Alerta de cierre → Bandeja de clasificación (Fase A) */}
      {noIdentificados > 0 && (
        <div className="flex flex-wrap items-center gap-2.5 rounded-xl border border-brand-gold/60 bg-brand-gold/10 px-4 py-2.5 text-sm">
          <span aria-hidden>⚠️</span>
          <span>
            <strong>{noIdentificados.toLocaleString("es-CR")} movimiento(s) sin clasificar</strong> en{" "}
            {etiquetaPeriodo(periodo)} — el cierre exige el 100% clasificado.
          </span>
          <Link to="/clasificar" className="ml-auto font-bold text-accent hover:underline">
            Ir a Clasificar →
          </Link>
        </div>
      )}

      {/* KPIs */}
      {dashboardQuery.isPending ? (
        <LoadingState label="Cargando KPIs" />
      ) : dashboardQuery.isError ? (
        <ErrorState message={mensajeError(dashboardQuery.error)} onRetry={() => dashboardQuery.refetch()} />
      ) : (
        <KpiGrid data={dashboardQuery.data} />
      )}

      {/* Tendencia + EBITDA */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[2fr_1fr]">
        <Card>
          <CardHeader className="flex flex-wrap items-baseline justify-between gap-3">
            <div>
              <CardTitle>Tendencia 12 meses</CardTitle>
              <p className="mt-0.5 text-xs text-content-muted">
                Ingresos y gastos por mes, traslados excluidos · clic en un mes para enfocarlo
              </p>
            </div>
            <div className="flex gap-3 text-xs text-content-muted">
              <span className="flex items-center gap-1.5">
                <i className="h-2.5 w-2.5 rounded-sm" style={{ backgroundColor: colores.ingreso }} />
                Ingresos
              </span>
              <span className="flex items-center gap-1.5">
                <i className="h-2.5 w-2.5 rounded-sm" style={{ backgroundColor: colores.gasto }} />
                Gastos
              </span>
            </div>
          </CardHeader>
          <CardContent>
            {serieQuery.isPending ? (
              <LoadingState label="Cargando tendencia" />
            ) : serieQuery.isError ? (
              <ErrorState message={mensajeError(serieQuery.error)} onRetry={() => serieQuery.refetch()} />
            ) : (
              <TendenciaChart serie={serieQuery.data} periodoActivo={periodo} onSelect={setPeriodo} />
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>EBITDA mensual</CardTitle>
            <p className="mt-0.5 text-xs text-content-muted">Ingresos − gastos, por mes</p>
          </CardHeader>
          <CardContent>
            {serieQuery.isPending ? (
              <LoadingState label="Cargando EBITDA" />
            ) : serieQuery.isError ? (
              <ErrorState message={mensajeError(serieQuery.error)} onRetry={() => serieQuery.refetch()} />
            ) : (
              <EbitdaChart serie={serieQuery.data} periodoActivo={periodo} onSelect={setPeriodo} />
            )}
          </CardContent>
        </Card>
      </div>

      {/* Calendario + Por cuenta */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Calendario · {etiquetaPeriodo(periodo)}</CardTitle>
            <p className="mt-0.5 text-xs text-content-muted">
              Flujo neto por día (créditos − débitos), traslados excluidos
            </p>
          </CardHeader>
          <CardContent>
            {calendarioQuery.isPending ? (
              <LoadingState label="Cargando calendario" />
            ) : calendarioQuery.isError ? (
              <ErrorState
                message={mensajeError(calendarioQuery.error)}
                onRetry={() => calendarioQuery.refetch()}
              />
            ) : calendarioQuery.data.length === 0 ? (
              <EmptyState message="Sin movimientos en este período." />
            ) : (
              <CalendarioFlujo periodo={periodo} dias={calendarioQuery.data} />
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-wrap items-baseline justify-between gap-3">
            <div>
              <CardTitle>Por cuenta</CardTitle>
              <p className="mt-0.5 text-xs text-content-muted">
                Créditos y débitos del período por cuenta bancaria
              </p>
            </div>
            <div className="flex gap-3 text-xs text-content-muted">
              <span className="flex items-center gap-1.5">
                <i className="h-2.5 w-2.5 rounded-sm" style={{ backgroundColor: colores.ingreso }} />
                Créditos
              </span>
              <span className="flex items-center gap-1.5">
                <i className="h-2.5 w-2.5 rounded-sm" style={{ backgroundColor: colores.gasto }} />
                Débitos
              </span>
            </div>
          </CardHeader>
          <CardContent>
            {cuentasQuery.isPending ? (
              <LoadingState label="Cargando cuentas" />
            ) : cuentasQuery.isError ? (
              <ErrorState message={mensajeError(cuentasQuery.error)} onRetry={() => cuentasQuery.refetch()} />
            ) : cuentasQuery.data.length === 0 ? (
              <EmptyState message="Sin movimientos en este período." />
            ) : (
              <CuentasResumenView cuentas={cuentasQuery.data} />
            )}
          </CardContent>
        </Card>
      </div>

      {/* Cuadre por concepto con drill-down */}
      <Card>
        <CardHeader>
          <CardTitle>Cuadre por concepto — {etiquetaPeriodo(periodo)}</CardTitle>
          <p className="mt-0.5 text-xs text-content-muted">
            Clic en un concepto para abrirlo; clic en una clasificación para ver sus movimientos.
          </p>
        </CardHeader>
        <CardContent>
          {cuadreArbolQuery.isPending ? (
            <LoadingState label="Cargando cuadre" />
          ) : cuadreArbolQuery.isError ? (
            <ErrorState
              message={mensajeError(cuadreArbolQuery.error)}
              onRetry={() => cuadreArbolQuery.refetch()}
            />
          ) : !cuadreArbolQuery.data || cuadreArbolQuery.data.conceptos.length === 0 ? (
            <EmptyState message="Sin movimientos en este período." />
          ) : (
            <CuadreArbolView data={cuadreArbolQuery.data} periodo={periodo} />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function KpiGrid({ data }: { data: DashboardData }) {
  const cmp = data.comparativo;
  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <KpiCard
          label="Ingresos"
          valor={data.ingresos}
          anterior={cmp.ingresos}
          moneda
          buenoSiSube
        />
        <KpiCard label="Gastos" valor={data.gastos} anterior={cmp.gastos} moneda buenoSiSube={false} />
        <KpiCard label="EBITDA" valor={data.ebitda} anterior={cmp.ebitda} moneda buenoSiSube />
        <NoIdentificadosCard cantidad={data.no_identificados} />
      </div>
      <AvisoFueraDelEbitda data={data} />
    </div>
  );
}

/**
 * Qué quedó FUERA del EBITDA y por qué.
 *
 * Los KPIs cuentan SOLO los conceptos declarados como ingreso o gasto. Eso es lo correcto —antes
 * sumaban cualquier crédito y cualquier débito, y el ahorro, las reservas y los aportes entre
 * empresas inflaban el número—, pero un total que omite algo sin decirlo miente por silencio. Acá
 * se dice cuánto quedó afuera y cuántos conceptos falta declarar, con el enlace para hacerlo.
 */
function AvisoFueraDelEbitda({ data }: { data: DashboardData }) {
  const monto = toNumber(data.fuera_del_ebitda);
  if (monto <= 0 && data.conceptos_sin_declarar === 0) return null;
  return (
    <div className="rounded-xl border border-pendiente/40 bg-pendiente/10 px-4 py-3 text-sm">
      <p className="text-content">
        <b>₡{formatMonto(data.fuera_del_ebitda)}</b> en {data.movs_fuera_del_ebitda.toLocaleString("es-CR")}{" "}
        movimiento{data.movs_fuera_del_ebitda === 1 ? "" : "s"} <b>no entra al EBITDA</b>: su concepto
        está en «no entra» o todavía no está clasificado.
      </p>
      {data.conceptos_sin_declarar > 0 && (
        <p className="mt-1 text-content-muted">
          Hay <b>{data.conceptos_sin_declarar}</b> concepto{data.conceptos_sin_declarar === 1 ? "" : "s"} en
          uso sin declarar si suma a ingresos o a gastos.{" "}
          <Link to="/catalogo" className="text-accent underline">
            Declararlo en el Catálogo › Conceptos
          </Link>
          .
        </p>
      )}
    </div>
  );
}

interface KpiCardProps {
  label: string;
  valor: string;
  anterior: string;
  moneda: boolean;
  /** true si un aumento es "positivo" (ingresos/EBITDA); false para gastos. */
  buenoSiSube: boolean;
}

function KpiCard({ label, valor, anterior, moneda, buenoSiSube }: KpiCardProps) {
  const actualN = toNumber(valor);
  const anteriorN = toNumber(anterior);
  const delta = actualN - anteriorN;
  const pct = anteriorN !== 0 ? (delta / Math.abs(anteriorN)) * 100 : null;
  const sube = delta > 0;
  const bueno = delta === 0 ? null : sube === buenoSiSube;

  return (
    <Card>
      <CardContent className="py-4">
        <p className="text-xs uppercase tracking-wide text-content-muted">{label}</p>
        <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
          {moneda ? formatMoneda(valor) : formatMonto(valor)}
        </p>
        <p className="mt-2 flex items-center gap-1.5 text-sm">
          <span
            className={cn(
              "font-medium tabular-nums",
              bueno === null ? "text-content-muted" : bueno ? "text-positivo" : "text-negativo",
            )}
          >
            {delta === 0 ? "±0" : `${sube ? "▲" : "▼"} ${formatMonto(String(Math.abs(delta)))}`}
          </span>
          {pct !== null && (
            <span className="text-content-muted tabular-nums">
              ({sube ? "+" : ""}
              {pct.toFixed(1)}%)
            </span>
          )}
          <span className="text-content-muted">vs mes anterior</span>
        </p>
      </CardContent>
    </Card>
  );
}

function NoIdentificadosCard({ cantidad }: { cantidad: number }) {
  return (
    <Card>
      <CardContent className="py-4">
        <p className="text-xs uppercase tracking-wide text-content-muted">No identificados</p>
        <p
          className={cn(
            "mt-1 text-2xl font-semibold tabular-nums",
            cantidad > 0 ? "text-pendiente" : "text-positivo",
          )}
        >
          {cantidad}
        </p>
        <p className="mt-2 text-sm">
          {cantidad > 0 ? (
            <Badge tone="pendiente">Bloquea el cierre</Badge>
          ) : (
            <Badge tone="positivo">Listo para cerrar</Badge>
          )}
        </p>
      </CardContent>
    </Card>
  );
}
