/**
 * Calendario de flujo del período (Fase B): el mes como grilla, cada día
 * coloreado por su neto (créditos − débitos, traslados excluidos). El detalle
 * (créditos, débitos, neto, movimientos) sale al pasar el cursor.
 */

import { cn } from "@/lib/cn";
import { formatMoneda, partesPeriodo, toNumber } from "@/lib/format";
import type { DiaCalendario } from "@/api/bancos";
import { useChartColors } from "@/features/bancos/components/chartColors";

const DOW = ["L", "K", "M", "J", "V", "S", "D"];

export function CalendarioFlujo({ periodo, dias }: { periodo: string; dias: DiaCalendario[] }) {
  const c = useChartColors();
  const { anio, mes } = partesPeriodo(periodo);
  const porDia = new Map(dias.map((d) => [Number(d.fecha.slice(8)), d]));
  const netoMax = Math.max(...dias.map((d) => Math.abs(toNumber(d.neto))), 1);
  const nDias = new Date(anio, mes, 0).getDate();
  const offset = (new Date(anio, mes - 1, 1).getDay() + 6) % 7; // lunes = 0

  const celdas: Array<{ dia: number; dato?: DiaCalendario } | null> = [];
  for (let i = 0; i < offset; i++) celdas.push(null);
  for (let d = 1; d <= nDias; d++) celdas.push({ dia: d, dato: porDia.get(d) });

  return (
    <div>
      <div className="grid grid-cols-7 gap-1">
        {DOW.map((d) => (
          <div key={d} className="pb-0.5 text-center text-[10px] font-bold uppercase tracking-wider text-content-muted">
            {d}
          </div>
        ))}
        {celdas.map((celda, i) => {
          if (!celda) return <div key={`v${i}`} />;
          if (!celda.dato) {
            return (
              <div key={celda.dia} className="min-h-[52px] rounded-lg border border-dashed border-border p-1.5">
                <span className="text-[10.5px] text-content-muted">{celda.dia}</span>
              </div>
            );
          }
          const neto = toNumber(celda.dato.neto);
          const intensidad = Math.min(0.85, 0.12 + (0.73 * Math.abs(neto)) / netoMax);
          const color = neto >= 0 ? c.ingreso : c.gasto;
          const fuerte = intensidad > 0.5;
          const titulo = [
            `${String(celda.dia).padStart(2, "0")}/${String(mes).padStart(2, "0")}/${anio}`,
            `Créditos: ${formatMoneda(celda.dato.creditos)}`,
            `Débitos: ${formatMoneda(celda.dato.debitos)}`,
            `Neto: ${formatMoneda(celda.dato.neto)}`,
            `Movimientos: ${celda.dato.movimientos}`,
          ].join("\n");
          return (
            <div
              key={celda.dia}
              title={titulo}
              className="relative min-h-[52px] rounded-lg border border-border p-1.5 outline outline-2 outline-transparent transition-[outline-color] hover:outline-brand-gold"
              style={{ backgroundColor: `color-mix(in srgb, ${color} ${Math.round(intensidad * 100)}%, transparent)` }}
            >
              <span className={cn("text-[10.5px] font-bold", fuerte ? "text-white" : "text-content-muted")}>
                {celda.dia}
              </span>
              <span
                className="absolute inset-x-1.5 bottom-1 text-[10.5px] font-semibold tabular-nums"
                style={{ color: fuerte ? "#fff" : color }}
              >
                {neto >= 0 ? "+" : "−"}
                {(Math.abs(neto) / 1e6).toLocaleString("es-CR", { minimumFractionDigits: 1, maximumFractionDigits: 1 })} M
              </span>
            </div>
          );
        })}
      </div>
      <div className="mt-2.5 flex items-center gap-2 text-[10.5px] text-content-muted">
        <span>Sale</span>
        <div className="flex h-2 flex-1 overflow-hidden rounded-full">
          {[-1, -0.6, -0.25, 0, 0.25, 0.6, 1].map((t) => (
            <i
              key={t}
              className="flex-1"
              style={{
                backgroundColor: `color-mix(in srgb, ${t < 0 ? c.gasto : c.ingreso} ${Math.round(Math.abs(t) * 80 + 8)}%, transparent)`,
              }}
            />
          ))}
        </div>
        <span>Entra</span>
      </div>
    </div>
  );
}
