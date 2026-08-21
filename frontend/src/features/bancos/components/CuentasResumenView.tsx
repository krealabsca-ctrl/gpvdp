/**
 * Desglose por cuenta (Fase B): créditos y débitos del período por cuenta
 * bancaria, como pares de barras comparables entre sí.
 */

import { formatMoneda, toNumber } from "@/lib/format";
import type { CuentaResumen } from "@/api/bancos";
import { useChartColors } from "@/features/bancos/components/chartColors";

export function CuentasResumenView({ cuentas }: { cuentas: CuentaResumen[] }) {
  const c = useChartColors();
  const max = Math.max(...cuentas.flatMap((x) => [toNumber(x.creditos), toNumber(x.debitos)]), 1);

  return (
    <div className="flex flex-col gap-4">
      {cuentas.map((cta) => (
        <div key={cta.cuenta_id}>
          <div className="mb-1.5 flex items-baseline justify-between text-[12.5px]">
            <span className="font-semibold">
              {cta.banco}
              {cta.alias && <span className="font-normal text-content-muted"> · {cta.alias}</span>}
            </span>
            <span className="text-[11px] tabular-nums text-content-muted">
              {cta.movimientos.toLocaleString("es-CR")} movs
            </span>
          </div>
          <div className="grid gap-1">
            <BarraCuenta valor={cta.creditos} max={max} color={c.ingreso} etiqueta="Créditos" />
            <BarraCuenta valor={cta.debitos} max={max} color={c.gasto} etiqueta="Débitos" />
          </div>
        </div>
      ))}
    </div>
  );
}

function BarraCuenta({
  valor,
  max,
  color,
  etiqueta,
}: {
  valor: string;
  max: number;
  color: string;
  etiqueta: string;
}) {
  const n = toNumber(valor);
  return (
    <div className="grid grid-cols-[1fr_auto] items-center gap-2" title={`${etiqueta}: ${formatMoneda(valor)}`}>
      <div className="h-3 overflow-hidden rounded bg-surface-muted">
        <div
          className="h-full rounded-r"
          style={{ width: `${Math.max(0.5, (n / max) * 100)}%`, backgroundColor: color }}
        />
      </div>
      <span className="min-w-[86px] text-right text-[11px] tabular-nums text-content-muted">
        {formatMoneda(valor)}
      </span>
    </div>
  );
}
