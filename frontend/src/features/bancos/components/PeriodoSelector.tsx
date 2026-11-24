import { Select } from "@/components/ui";
import { etiquetaPeriodo, mesDeHoy, periodosDelSelector } from "@/lib/format";
import { usePeriodosDeLaEmpresa } from "@/features/bancos/hooks";

interface PeriodoSelectorProps {
  value: string;
  onChange: (periodo: string) => void;
  label?: string;
  id?: string;
}

/** Selector de período YYYY-MM, con etiqueta legible. Los meses salen del calendario real de la empresa. */
export function PeriodoSelector({
  value,
  onChange,
  label = "Período",
  id,
}: PeriodoSelectorProps) {
  const cal = usePeriodosDeLaEmpresa();
  const options = periodosDelSelector(
    cal.data?.primero ?? "",
    cal.data?.ultimo ?? "",
    mesDeHoy(),
  ).map((p) => ({ value: p, label: etiquetaPeriodo(p) }));
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
