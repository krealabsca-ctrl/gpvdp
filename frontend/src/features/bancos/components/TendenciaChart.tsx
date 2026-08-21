/**
 * Tendencia 12 meses (Fase B): líneas de Ingresos y Gastos (traslados excluidos).
 * Clic en un mes → selecciona ese período y todo el tablero se enfoca en él.
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
import type { SerieMensualPunto } from "@/api/bancos";
import { etiquetaMesCorto, useChartColors } from "@/features/bancos/components/chartColors";

interface PuntoChart {
  periodo: string;
  label: string;
  ingresos: number;
  gastos: number;
  ebitda: number;
}

export function TendenciaChart({
  serie,
  periodoActivo,
  onSelect,
}: {
  serie: SerieMensualPunto[];
  periodoActivo: string;
  onSelect: (periodo: string) => void;
}) {
  const c = useChartColors();
  const data = useMemo<PuntoChart[]>(
    () =>
      serie.map((p) => ({
        periodo: p.periodo,
        label: etiquetaMesCorto(p.periodo),
        ingresos: toNumber(p.ingresos),
        gastos: toNumber(p.gastos),
        ebitda: toNumber(p.ebitda),
      })),
    [serie],
  );
  const labelActivo = data.find((d) => d.periodo === periodoActivo)?.label;

  return (
    <div style={{ width: "100%", height: 260 }} className="cursor-pointer">
      <ResponsiveContainer>
        <LineChart
          data={data}
          margin={{ top: 8, right: 12, bottom: 0, left: 4 }}
          onClick={(st) => {
            const label = st && "activeLabel" in st ? st.activeLabel : undefined;
            const p = data.find((d) => d.label === label);
            if (p) onSelect(p.periodo);
          }}
        >
          <CartesianGrid stroke={c.grid} vertical={false} />
          <XAxis
            dataKey="label"
            tick={{ fontSize: 10.5, fill: c.tick }}
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
          {labelActivo !== undefined && (
            <ReferenceLine x={labelActivo} stroke={c.oro} strokeDasharray="4 3" strokeWidth={2} />
          )}
          <Tooltip content={<TooltipTendencia />} cursor={{ stroke: c.tick, strokeDasharray: "3 3" }} />
          <Line
            type="monotone"
            dataKey="ingresos"
            name="Ingresos"
            stroke={c.ingreso}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4.5, strokeWidth: 2 }}
          />
          <Line
            type="monotone"
            dataKey="gastos"
            name="Gastos"
            stroke={c.gasto}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4.5, strokeWidth: 2 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function TooltipTendencia({
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
      <p className="mb-1 font-bold">{p.label}</p>
      <Fila etiqueta="Ingresos" valor={p.ingresos} />
      <Fila etiqueta="Gastos" valor={p.gastos} />
      <Fila etiqueta="EBITDA" valor={p.ebitda} />
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
