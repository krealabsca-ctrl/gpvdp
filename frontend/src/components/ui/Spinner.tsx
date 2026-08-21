import { cn } from "@/lib/cn";

export interface SpinnerProps {
  size?: "sm" | "md" | "lg";
  className?: string;
  /** Etiqueta accesible; por defecto "Cargando". */
  label?: string;
}

const sizes = {
  sm: "h-4 w-4 border-2",
  md: "h-6 w-6 border-2",
  lg: "h-10 w-10 border-[3px]",
} as const;

/** Indicador de carga circular, accesible. */
export function Spinner({ size = "md", className, label = "Cargando" }: SpinnerProps) {
  return (
    <span
      role="status"
      aria-live="polite"
      className={cn(
        "inline-block animate-spin rounded-full border-current border-t-transparent text-accent",
        sizes[size],
        className,
      )}
    >
      <span className="sr-only">{label}</span>
    </span>
  );
}
