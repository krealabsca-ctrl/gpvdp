/**
 * EBITDA mensual (Fase B): barras que divergen del cero — verde positivo,
 * terracota negativo. Clic en una barra → selecciona ese período.
 */

import { useMemo } from "react";
import {
  Bar,
  BarChart,
  Cell,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { formatMoneda, toNumber } from "@/lib/format";
import type { SerieMensualPunto } from "@/api/bancos";
import { etiquetaMesCorto, useChartColors } from "@/features/bancos/components/chartColors";

interface BarraEbitda {
  periodo: string;
  label: string;
  ebitda: number;
}

export function EbitdaChart({
  serie,
  periodoActivo,
  onSelect,
}: {
  serie: SerieMensualPunto[];
  periodoActivo: string;
  onSelect: (periodo: string) => void;
}) {
  const c = useChartColors();
  const data = useMemo<BarraEbitda[]>(
    () =>
      serie.map((p) => ({
        periodo: p.periodo,
        label: etiquetaMesCorto(p.periodo),
        ebitda: toNumber(p.ebitda),
      })),
    [serie],
  );

  return (
    <div style={{ width: "100%", height: 260 }}>
      <ResponsiveContainer>
        <BarChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 4 }}>
          <XAxis
            dataKey="label"
            tick={{ fontSize: 10, fill: c.tick }}
            interval="preserveStartEnd"
            axisLine={{ stroke: c.grid }}
            tickLine={false}
          />
          <YAxis
            tick={{ fontSize: 10.5, fill: c.tick }}
            tickFormatter={(v: number) => `${Math.round(v / 1e6)} M`}
            axisLine={false}
            tickLine={false}
            width={44}
          />
          <ReferenceLine y={0} stroke={c.tick} />
          <Tooltip content={<TooltipEbitda />} cursor={{ fill: "transparent" }} />
          <Bar
            dataKey="ebitda"
            radius={[3, 3, 0, 0]}
            onClick={(entry) => {
              const p = (entry as { payload?: BarraEbitda }).payload;
              if (p) onSelect(p.periodo);
            }}
            className="cursor-pointer"
          >
            {data.map((d) => (
              <Cell
                key={d.periodo}
                fill={d.ebitda >= 0 ? c.ingreso : c.gasto}
                opacity={d.periodo === periodoActivo ? 1 : 0.55}
              />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

function TooltipEbitda({
  active,
  payload,
}: {
  active?: boolean;
  payload?: ReadonlyArray<{ payload: BarraEbitda }>;
}) {
  const p = payload?.[0]?.payload;
  if (!active || !p) return null;
  return (
    <div className="rounded-lg border border-border bg-surface-raised px-3 py-2 text-xs shadow-lifted">
      <p className="mb-1 font-bold">{p.label}</p>
      <p className="flex justify-between gap-6">
        <span className="text-content-muted">EBITDA</span>
        <span className="font-semibold tabular-nums">{formatMoneda(String(p.ebitda))}</span>
      </p>
    </div>
  );
}
