import { Select } from "@/components/ui";
import { etiquetaPeriodo, periodosRecientes } from "@/lib/format";

interface PeriodoSelectorProps {
  value: string;
  onChange: (periodo: string) => void;
  label?: string;
  id?: string;
}

/** Selector de período YYYY-MM (últimos 18 meses), con etiqueta legible. */
export function PeriodoSelector({
  value,
  onChange,
  label = "Período",
  id,
}: PeriodoSelectorProps) {
  const options = periodosRecientes().map((p) => ({ value: p, label: etiquetaPeriodo(p) }));
  return (
    <Select
      id={id}
      label={label}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      options={options}
      className="min-w-44"
    />
  );
}
