import type { HTMLAttributes } from "react";
import { cn } from "@/lib/cn";

export type BadgeTone = "neutral" | "accent" | "positivo" | "negativo" | "pendiente";

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone;
}

const tones: Record<BadgeTone, string> = {
  neutral: "bg-surface-muted text-content-muted",
  accent: "bg-accent/10 text-accent",
  positivo: "bg-positivo/10 text-positivo",
  negativo: "bg-negativo/10 text-negativo",
  pendiente: "bg-pendiente/10 text-pendiente",
};

/** Chip/etiqueta compacta para estados (duplicado, clasificación, traslado). */
export function Badge({ className, tone = "neutral", ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium uppercase tracking-wide",
        tones[tone],
        className,
      )}
      {...props}
    />
  );
}
