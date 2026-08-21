/**
 * RRHH — Dashboard (/rrhh). Pantalla aprobada de la maqueta: el COSTO REAL de la planilla
 * del mes (bruto + cargas patronales + provisiones), la tendencia, a dónde va cada colón,
 * qué falta del ciclo quincenal, el headcount y las alertas antes de pagar.
 *
 * Todo sale de datos reales (corridas vivas del mes, borrador incluido: la pantalla sirve
 * para decidir ANTES de aprobar). Guardarraíl: no hay ningún indicador de "ahorro por no
 * reportar"; la única alerta legal verifica que cada concepto excluido de CCSS tenga base.
 */

import { useMemo } from "react";
import { Link } from "react-router-dom";
import { Bar, BarChart, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import {
  Badge,
  Button,
  Card,
  CardContent,
  ErrorState,
  LoadingState,
  PageHeader,
  type BadgeTone,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { DondeSeRegistra } from "@/features/rrhh/components/DondeSeRegistra";
import { formatMoneda, partesPeriodo, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useChartColors } from "@/features/bancos/components/chartColors";
import { useDashboardRRHH } from "@/features/rrhh/hooks";
import type { DashboardAlerta, DashboardMes, DashboardPaso, DashboardRRHH } from "@/api/rrhh";

const MESES = [
  "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
  "Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre",
];

/** Nombre del mes (1-12) sin depender del índice: TS estricto no confía en el arreglo. */
function nombreMes(mes: number): string {
  return MESES[mes - 1] ?? String(mes);
}

/** Porcentaje con dos decimales (la carga patronal real es 26,83%: redondear a 27% engaña). */
function pctLegible(valor: string): string {
  const n = Number(valor);
  if (!Number.isFinite(n)) return "—";
  return `${n.toLocaleString("es-CR", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} %`;
}

const TONO_PASO: Record<string, BadgeTone> = {
  SIN_CREAR: "neutral",
  PENDIENTE: "neutral",
  BORRADOR: "pendiente",
  APROBADA: "accent",
  PAGADA: "positivo",
  LISTA: "positivo",
};

export function DashboardRRHHPage() {
  // El período es contexto ambiente del ERP (un único selector en la barra superior):
  // el tablero sigue ese mes en vez de duplicar el control.
  const { periodo } = usePeriodoActivo();
  const { anio, mes } = partesPeriodo(periodo);
  const q = useDashboardRRHH(anio, mes);

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Dashboard de Recursos Humanos"
        description={`${nombreMes(mes)} ${anio} · el costo real de planilla y qué falta del ciclo quincenal`}
      />

      {q.isPending ? (
        <LoadingState label="Cargando el resumen del mes" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : (
        <Resumen d={q.data} />
      )}
    </div>
  );
}

function Resumen({ d }: { d: DashboardRRHH }) {
  const sinCorrida = toNumber(d.costo_real) === 0 && d.finiquitos.cantidad === 0;
  return (
    <div className="flex flex-col gap-4">
      {sinCorrida && (
        <Card>
          <CardContent className="py-4 text-sm text-content-muted">
            Todavía no hay corridas de {nombreMes(d.mes)} {d.anio}. Abrí la corrida del mes en{" "}
            <Link to="/rrhh/corridas" className="font-medium text-accent underline">
              Corridas
            </Link>{" "}
            y este tablero se llena solo.
          </CardContent>
        </Card>
      )}

      {/* KPIs de la maqueta: costo real destacado + neto, cargas y provisiones. */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Kpi
          label={`Costo real de planilla · ${nombreMes(d.mes).slice(0, 3)}`}
          valor={d.costo_real}
          detalle="bruto + cargas patronales + provisiones"
          destacado
        />
        <Kpi
          label="Neto a pagar · liquidación"
          valor={d.neto_liquidacion}
          detalle={`${d.empleados_pagados} ${d.empleados_pagados === 1 ? "empleado" : "empleados"} en la corrida · ${formatMoneda(d.neto)} en el mes (las dos quincenas)`}
        />
        <Kpi
          label="Cargas patronales"
          valor={d.patronal}
          detalle={`${pctLegible(d.patronal_pct)} sobre la base contributiva (${formatMoneda(d.base_ccss)})`}
        />
        <Kpi
          label="Provisiones del mes"
          valor={d.provisiones}
          detalle={`aguinaldo ${formatMoneda(d.prov_aguinaldo)} · cesantía ${formatMoneda(d.prov_cesantia)} · vacaciones ${formatMoneda(d.prov_vacaciones)}`}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardContent className="flex flex-col gap-1 py-4">
            <h3 className="font-semibold">Costo de planilla — tendencia</h3>
            <p className="text-xs text-content-muted">
              Costo real de los últimos 6 meses; el mes seleccionado va en dorado.
            </p>
            <TendenciaCosto puntos={d.tendencia} />
          </CardContent>
        </Card>

        <Card>
          <CardContent className="flex flex-col gap-1 py-4">
            <h3 className="font-semibold">Composición del costo · {nombreMes(d.mes)}</h3>
            <p className="text-xs text-content-muted">A dónde va cada colón del costo real de planilla.</p>
            <Composicion d={d} />
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card>
          <CardContent className="flex flex-col gap-3 py-4">
            <div>
              <h3 className="font-semibold">Estado del ciclo del mes</h3>
              <p className="text-xs text-content-muted">
                {nombreMes(d.mes)} {d.anio}
              </p>
            </div>
            <div className="flex flex-col gap-2 text-sm">
              <Paso etiqueta="1ª quincena · día 15" paso={d.ciclo.adelanto} />
              <Paso etiqueta="2ª quincena · día 30" paso={d.ciclo.liquidacion} />
              <Paso etiqueta="Planilla CCSS (SICERE)" paso={d.ciclo.planilla} />
            </div>
            <Link to="/rrhh/corridas" className="self-start">
              <Button size="sm">
                {d.ciclo.liquidacion.estado === "BORRADOR" ? "Ir a aprobar la corrida →" : "Ir a las corridas →"}
              </Button>
            </Link>
            {d.finiquitos.cantidad > 0 && (
              <p className="border-t border-border pt-2 text-xs text-content-muted">
                {d.finiquitos.cantidad} {d.finiquitos.cantidad === 1 ? "finiquito" : "finiquitos"} con salida en el
                mes por {formatMoneda(d.finiquitos.total)} (más {formatMoneda(d.finiquitos.patronal)} de carga
                patronal): entran en el archivo SINPE de la liquidación y en la planilla CCSS.
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardContent className="flex flex-col gap-2 py-4">
            <div>
              <h3 className="font-semibold">Headcount por departamento</h3>
              <p className="text-xs text-content-muted">
                {d.empleados} {d.empleados === 1 ? "empleado activo" : "empleados activos"}
              </p>
            </div>
            {d.headcount.length === 0 ? (
              <p className="text-sm text-content-muted">Sin empleados activos.</p>
            ) : (
              <div className="flex flex-col">
                {d.headcount.map((h) => (
                  <div
                    key={h.departamento}
                    className="flex items-center justify-between border-b border-dashed border-border py-1.5 text-sm last:border-0"
                  >
                    <span>{h.departamento}</span>
                    <b className="tabular-nums">{h.empleados}</b>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardContent className="flex flex-col gap-2 py-4">
            <div>
              <h3 className="font-semibold">Alertas</h3>
              <p className="text-xs text-content-muted">Lo que conviene resolver antes de pagar</p>
            </div>
            <div className="flex flex-col gap-2">
              {d.alertas.map((a, i) => (
                <Alerta key={`${a.tono}-${i}`} alerta={a} />
              ))}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function Kpi({
  label,
  valor,
  detalle,
  destacado = false,
}: {
  label: string;
  valor: string;
  detalle: string;
  destacado?: boolean;
}) {
  return (
    <Card className={cn(destacado && "border-accent/60 bg-accent/5")}>
      <CardContent className="py-4">
        <p className="text-xs uppercase tracking-wide text-content-muted">{label}</p>
        <p className={cn("mt-1 text-2xl font-semibold tabular-nums", destacado && "text-accent")}>
          {formatMoneda(valor)}
        </p>
        <p className="mt-2 text-xs text-content-muted">{detalle}</p>
      </CardContent>
    </Card>
  );
}

function Paso({ etiqueta, paso }: { etiqueta: string; paso: DashboardPaso }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span>{etiqueta}</span>
      <Badge tone={TONO_PASO[paso.estado] ?? "neutral"}>{paso.etiqueta}</Badge>
    </div>
  );
}

function Alerta({ alerta }: { alerta: DashboardAlerta }) {
  const estilo =
    alerta.tono === "WARN"
      ? "border-pendiente/40 bg-pendiente/10"
      : alerta.tono === "LEGAL"
        ? "border-accent/40 bg-accent/5"
        : "border-border bg-surface-muted";
  return (
    <div className={cn("flex gap-2 rounded-lg border px-3 py-2 text-xs", estilo)}>
      <span aria-hidden>{alerta.icono}</span>
      <span>{alerta.texto}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tendencia (barras, 6 meses) y composición (barra apilada)
// ---------------------------------------------------------------------------

interface PuntoTendencia {
  label: string;
  costo: number;
  enCurso: boolean;
}

function TendenciaCosto({ puntos }: { puntos: DashboardMes[] }) {
  const c = useChartColors();
  const data = useMemo<PuntoTendencia[]>(
    () => puntos.map((p) => ({ label: p.etiqueta, costo: toNumber(p.costo), enCurso: p.en_curso })),
    [puntos],
  );
  if (data.every((p) => p.costo === 0)) {
    return (
      <p className="py-8 text-center text-sm text-content-muted">
        Sin corridas en los últimos 6 meses: no hay tendencia que mostrar todavía.
      </p>
    );
  }
  return (
    <div style={{ width: "100%", height: 220 }} className="mt-2">
      <ResponsiveContainer>
        <BarChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 4 }}>
          <XAxis
            dataKey="label"
            tick={{ fontSize: 10.5, fill: c.tick }}
            axisLine={{ stroke: c.grid }}
            tickLine={false}
          />
          <YAxis
            tick={{ fontSize: 10.5, fill: c.tick }}
            tickFormatter={(v: number) => `${(v / 1e6).toFixed(1)} M`}
            axisLine={false}
            tickLine={false}
            width={48}
          />
          <Tooltip content={<TooltipCosto />} cursor={{ fill: c.grid, fillOpacity: 0.35 }} />
          <Bar dataKey="costo" name="Costo real" radius={[4, 4, 0, 0]}>
            {data.map((p) => (
              <Cell key={p.label} fill={p.enCurso ? c.oro : c.ingreso} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

function TooltipCosto({
  active,
  payload,
}: {
  active?: boolean;
  payload?: ReadonlyArray<{ payload: PuntoTendencia }>;
}) {
  const p = payload?.[0]?.payload;
  if (!active || !p) return null;
  return (
    <div className="rounded-lg border border-border bg-surface-raised px-3 py-2 text-xs shadow-lifted">
      <p className="mb-1 font-bold">{p.label}</p>
      <p className="flex justify-between gap-6">
        <span className="text-content-muted">Costo real</span>
        <span className="font-semibold tabular-nums">{formatMoneda(String(p.costo))}</span>
      </p>
    </div>
  );
}

function Composicion({ d }: { d: DashboardRRHH }) {
  const bruto = toNumber(d.bruto);
  const patronal = toNumber(d.patronal);
  const provisiones = toNumber(d.provisiones);
  const total = bruto + patronal + provisiones;
  if (total === 0) {
    return (
      <p className="py-8 text-center text-sm text-content-muted">
        Sin corrida del mes: todavía no hay costo que descomponer.
      </p>
    );
  }
  const pct = (v: number) => `${((v / total) * 100).toFixed(1)}%`;
  return (
    <div className="mt-3 flex flex-col gap-3">
      <div className="flex h-3.5 w-full overflow-hidden rounded-full bg-surface-muted">
        <span style={{ width: pct(bruto) }} className="bg-accent" title={`Bruto ${pct(bruto)}`} />
        <span style={{ width: pct(patronal) }} className="bg-brand-gold" title={`Cargas patronales ${pct(patronal)}`} />
        <span
          style={{ width: pct(provisiones) }}
          className="bg-accent/45"
          title={`Provisiones ${pct(provisiones)}`}
        />
      </div>
      <div className="flex flex-col gap-1.5 text-xs">
        <Leyenda color="bg-accent" etiqueta="Bruto pagado" valor={d.bruto} pct={pct(bruto)} />
        <Leyenda color="bg-brand-gold" etiqueta="Cargas patronales" valor={d.patronal} pct={pct(patronal)} />
        <Leyenda color="bg-accent/45" etiqueta="Provisiones" valor={d.provisiones} pct={pct(provisiones)} />
      </div>
      {bruto > 0 && (
        <p className="border-t border-border pt-2 text-xs text-content-muted">
          Por cada ₡100 de salario bruto, la empresa desembolsa ≈{" "}
          <b className="text-content">₡{d.costo_por_100}</b> de costo real.
        </p>
      )}
    </div>
  );
}

function Leyenda({
  color,
  etiqueta,
  valor,
  pct,
}: {
  color: string;
  etiqueta: string;
  valor: string;
  pct: string;
}) {
  return (
    <div className="flex items-center gap-2">
      <i className={cn("h-2.5 w-2.5 shrink-0 rounded-sm", color)} aria-hidden />
      <span className="text-content-muted">{etiqueta}</span>
      <span className="ml-auto font-semibold tabular-nums">{formatMoneda(valor)}</span>
      <span className="w-12 text-right tabular-nums text-content-muted">{pct}</span>
      {/* El mapa del módulo: la pregunta «¿dónde registro las horas extra?» se contesta acá. */}
      <DondeSeRegistra />
    </div>
  );
}
