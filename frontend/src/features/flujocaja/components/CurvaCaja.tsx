/**
 * La curva del saldo de caja a 13 semanas.
 *
 * SE GRAFICA EL SALDO, NO EL NETO. Es toda la diferencia: un mes puede cerrar positivo y aun así
 * dejar la cuenta en rojo el día 12, y lo que decide si se puede pagar la planilla es el saldo en
 * una fecha. El neto ya lo contaba el flujo proyectado mensual que existía antes.
 *
 * La línea del mínimo operativo va dibujada porque el cruce es el evento que importa: sin ella,
 * una curva que baja se ve igual que una curva que baja DEMASIADO.
 */

import { useMemo } from "react";
import {
  Area,
  CartesianGrid,
  ComposedChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { formatMoneda, toNumber } from "@/lib/format";
import { useChartColors } from "@/features/bancos/components/chartColors";
import type { FlujoSemanal } from "@/api/flujocaja";

/** "2026-11-02" → "2 nov". El eje lleva trece etiquetas y no caben completas. */
function diaCorto(iso: string): string {
  const [, m, d] = iso.split("-");
  const MES = ["", "ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"];
  return `${Number(d)} ${MES[Number(m)] ?? ""}`;
}

interface Punto {
  semana: number;
  etiqueta: string;
  saldo: number;
  entradas: number;
  salidas: number;
}

export function CurvaCaja({ flujo }: { flujo: FlujoSemanal }) {
  const c = useChartColors();
  const minimo = toNumber(flujo.politica.minimo_operativo);

  const datos = useMemo<Punto[]>(
    () =>
      flujo.semanas.map((s) => ({
        semana: s.numero,
        etiqueta: diaCorto(s.desde),
        saldo: toNumber(s.saldo_final),
        entradas: toNumber(s.entradas),
        salidas: toNumber(s.salidas),
      })),
    [flujo.semanas],
  );

  return (
    <div className="h-72 w-full">
      <ResponsiveContainer width="100%" height="100%">
        <ComposedChart data={datos} margin={{ top: 8, right: 8, bottom: 4, left: 8 }}>
          <CartesianGrid stroke={c.grid} strokeDasharray="3 3" vertical={false} />
          <XAxis
            dataKey="etiqueta"
            tick={{ fill: c.tick, fontSize: 11 }}
            axisLine={{ stroke: c.grid }}
            tickLine={false}
          />
          <YAxis
            tick={{ fill: c.tick, fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            width={78}
            tickFormatter={(v: number) => `${Math.round(v / 1_000_000)}M`}
          />
          <Tooltip
            contentStyle={{
              background: "var(--color-surface-raised)",
              border: "1px solid var(--color-border)",
              borderRadius: 10,
              fontSize: 12,
            }}
            labelFormatter={(l, p) => {
              const n = (p as unknown as readonly { payload?: Punto }[] | undefined)?.[0]?.payload
                ?.semana;
              return n ? `Semana ${n} · ${String(l)}` : String(l ?? "");
            }}
            formatter={(v, name) => [formatMoneda(String(v ?? 0)), String(name ?? "")]}
          />

          {/* UNA SOLA SERIE sobre `saldo`. Con un Area y un Line compartiendo dataKey, Recharts
              dibujaba los puntos y se comía la línea: quedaba una nube de marcas sin curva. El
              Area ya trae su propio trazo, así que el Line sobraba. */}
          <Area
            type="monotone"
            dataKey="saldo"
            name="Saldo"
            stroke={c.ingreso}
            strokeWidth={2}
            fill={c.ingreso}
            fillOpacity={0.14}
            dot={{ r: 2.5, fill: c.ingreso, strokeWidth: 0 }}
            activeDot={{ r: 4 }}
            isAnimationActive={false}
          />

          {/* El cero siempre: cruzar hacia abajo es un sobregiro, no un número más. */}
          <ReferenceLine y={0} stroke={c.gasto} strokeWidth={1.5} />
          {minimo > 0 && (
            <ReferenceLine
              y={minimo}
              stroke={c.oro}
              strokeDasharray="5 4"
              strokeWidth={1.5}
              label={{
                value: "mínimo operativo",
                position: "insideTopLeft",
                fill: c.oro,
                fontSize: 10,
              }}
            />
          )}
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  );
}
