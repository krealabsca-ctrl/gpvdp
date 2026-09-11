/**
 * Día a día de las partidas seleccionadas (/analisis).
 *
 * Contesta CUÁNDO se movió la plata: «¿este gasto fue de golpe o repartido?». El juicio de
 * anomalía se queda en la vista mensual, porque el gasto diario es a saltos —un día se paga y diez
 * no pasa nada— y un promedio diario marcaría casi todos los días de pago.
 *
 * Se dibujan BARRAS y no una línea: una línea entre dos días con movimiento sugiere que hubo gasto
 * en los días del medio, y no lo hubo. La barra dice lo que pasó ese día y nada más.
 */

import { useMemo } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { formatMoneda, formatMonto } from "@/lib/format";
import { useChartColors } from "@/features/bancos/components/chartColors";
import type { SerieDiariaPartida } from "@/api/bancos";

/**
 * Colores de las series. Los dos primeros son los validados de la paleta (ingreso/gasto); el resto
 * se agrega para poder comparar varias partidas. Con muchas curvas el color deja de distinguir,
 * y por eso el backend acota a 12 partidas.
 */
const EXTRA = ["#7B6CD9", "#B0894A", "#3C8DBC", "#B34F7A", "#5E8C3A", "#8A6F4E", "#4C6EA8", "#9B5DA8"];

/** "2026-08-28" → "28 ago". El año se omite: el rango ya está rotulado en el encabezado. */
const MES_CORTO = ["ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"];
function etiquetaDia(iso: string): string {
  const [, m, d] = iso.split("-");
  if (!m || !d) return iso;
  return `${Number(d)} ${MES_CORTO[Number(m) - 1] ?? ""}`;
}

interface FilaChart {
  fecha: string;
  label: string;
  [serie: string]: string | number;
}

export function DiarioPartidasChart({ partidas }: { partidas: SerieDiariaPartida[] }) {
  const c = useChartColors();

  const colores = useMemo(
    () => [c.gasto, c.ingreso, ...EXTRA],
    [c.gasto, c.ingreso],
  );

  /**
   * Una fila por DÍA con movimiento, con una columna por partida. Los días sin movimiento no se
   * rellenan con cero: el eje muestra solo lo que pasó, que es más corto y más honesto que 90
   * barras en el suelo con tres picos.
   */
  const data = useMemo<FilaChart[]>(() => {
    const porFecha = new Map<string, FilaChart>();
    for (const p of partidas) {
      for (const d of p.dias) {
        let fila = porFecha.get(d.fecha);
        if (!fila) {
          fila = { fecha: d.fecha, label: etiquetaDia(d.fecha) };
          porFecha.set(d.fecha, fila);
        }
        fila[p.clasificacion] = Number(d.monto);
      }
    }
    return [...porFecha.values()].sort((a, b) => a.fecha.localeCompare(b.fecha));
  }, [partidas]);

  if (data.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-content-muted">
        Estas partidas no tuvieron movimiento en el rango elegido.
      </p>
    );
  }

  return (
    <div style={{ width: "100%", height: 300 }}>
      <ResponsiveContainer>
        <BarChart data={data} margin={{ top: 8, right: 12, bottom: 0, left: 4 }}>
          <CartesianGrid stroke={c.grid} vertical={false} />
          <XAxis
            dataKey="label"
            tick={{ fill: c.tick, fontSize: 11 }}
            stroke={c.grid}
            interval="preserveStartEnd"
            minTickGap={16}
          />
          <YAxis
            tick={{ fill: c.tick, fontSize: 11 }}
            stroke={c.grid}
            width={78}
            tickFormatter={(v: number) => formatMonto(String(v))}
          />
          <Tooltip
            formatter={(v, nombre) => [formatMoneda(String(v ?? 0), "CRC"), String(nombre ?? "")]}
            // El tooltip muestra la FECHA COMPLETA aunque el eje diga «28 ago»: en un rango de
            // varios meses el día suelto no dice de qué mes es.
            labelFormatter={(l) => data.find((d) => d.label === l)?.fecha ?? String(l ?? "")}
            contentStyle={{ fontSize: 12 }}
          />
          {partidas.length > 1 && <Legend wrapperStyle={{ fontSize: 12 }} />}
          {partidas.map((p, i) => (
            <Bar
              key={p.clasificacion_id}
              dataKey={p.clasificacion}
              fill={colores[i % colores.length]}
              // Apiladas: lo que interesa es cuánto salió ESE día en total y de qué partidas, no
              // comparar dos barras vecinas de la misma fecha.
              stackId="dia"
              maxBarSize={40}
            />
          ))}
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
