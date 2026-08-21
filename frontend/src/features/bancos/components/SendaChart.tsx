/**
 * Senda de cierre del mes (Fase C): ingresos acumulados reales (línea sólida),
 * proyección al cierre (punteada) y senda lineal hacia la meta (dorada).
 */

import { useMemo } from "react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { formatMoneda, toNumber } from "@/lib/format";
import type { ProyeccionResultado } from "@/api/bancos";
import { useChartColors } from "@/features/bancos/components/chartColors";

interface PuntoChart {
  dia: number;
  real: number | null;
  proyeccion: number | null;
  meta: number | null;
}

export function SendaChart({ resultado }: { resultado: ProyeccionResultado }) {
  const c = useChartColors();
  const meta = toNumber(resultado.meta_monto);

  const data = useMemo<PuntoChart[]>(() => {
    const porDiaReal = new Map(resultado.senda_real.map((p) => [p.dia, toNumber(p.acumulado)]));
    const porDiaProy = new Map(resultado.senda_proyeccion.map((p) => [p.dia, toNumber(p.acumulado)]));
    const out: PuntoChart[] = [];
    for (let d = 1; d <= resultado.dias_mes; d++) {
      out.push({
        dia: d,
        real: porDiaReal.get(d) ?? null,
        proyeccion: porDiaProy.get(d) ?? null,
        meta: meta > 0 ? (meta * d) / resultado.dias_mes : null,
      });
    }
    return out;
  }, [resultado, meta]);

  return (
    <div style={{ width: "100%", height: 300 }}>
      <ResponsiveContainer>
        <LineChart data={data} margin={{ top: 10, right: 70, bottom: 0, left: 4 }}>
          <CartesianGrid stroke={c.grid} vertical={false} />
          <XAxis
            dataKey="dia"
            tick={{ fontSize: 10.5, fill: c.tick }}
            ticks={[1, 5, 10, 15, 20, 25, resultado.dias_mes]}
            axisLine={{ stroke: c.grid }}
            tickLine={false}
          />
          <YAxis
            tick={{ fontSize: 10.5, fill: c.tick }}
            tickFormatter={(v: number) => `${Math.round(v / 1e6)} M`}
            axisLine={false}
            tickLine={false}
            width={46}
          />
          <ReferenceLine
            x={resultado.dia_calculo}
            stroke={c.tick}
            strokeDasharray="2 3"
            label={{ value: "hoy", position: "top", fontSize: 9.5, fill: c.tick }}
          />
          <Tooltip content={<TooltipSenda />} cursor={{ stroke: c.tick, strokeDasharray: "3 3" }} />
          {meta > 0 && (
            <Line type="linear" dataKey="meta" name="Meta" stroke={c.oro} strokeWidth={2} dot={false} />
          )}
          <Line
            type="monotone"
            dataKey="proyeccion"
            name="Proyección"
            stroke={c.ingreso}
            strokeWidth={2}
            strokeDasharray="5 4"
            dot={false}
            activeDot={{ r: 4, strokeWidth: 2 }}
          />
          <Line
            type="monotone"
            dataKey="real"
            name="Real"
            stroke={c.ingreso}
            strokeWidth={2.5}
            dot={false}
            activeDot={{ r: 4.5, strokeWidth: 2 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function TooltipSenda({
  active,
  payload,
}: {
  active?: boolean;
  payload?: ReadonlyArray<{ payload: PuntoChart }>;
}) {
  const p = payload?.[0]?.payload;
  if (!active || !p) return null;
  return (
    <div className="rounded-lg border border-border bg-surface-raised px-3 py-2 text-xs shadow-lifted">
      <p className="mb-1 font-bold">Día {p.dia}</p>
      {p.real !== null && <Fila etiqueta="Acumulado real" valor={p.real} />}
      {p.real === null && p.proyeccion !== null && <Fila etiqueta="Proyección" valor={p.proyeccion} />}
      {p.meta !== null && <Fila etiqueta="Senda de la meta" valor={p.meta} />}
    </div>
  );
}

function Fila({ etiqueta, valor }: { etiqueta: string; valor: number }) {
  return (
    <p className="flex justify-between gap-6">
      <span className="text-content-muted">{etiqueta}</span>
      <span className="font-semibold tabular-nums">{formatMoneda(String(valor))}</span>
    </p>
  );
}
