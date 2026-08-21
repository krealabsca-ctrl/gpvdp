import type { ReactNode } from "react";
import { Spinner } from "@/components/ui/Spinner";

/** Estado de carga centrado para bloques de contenido. */
export function LoadingState({ label = "Cargando" }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-3 py-12 text-sm text-content-muted">
      <Spinner size="md" label={label} />
      <span>{label}…</span>
    </div>
  );
}

/** Estado de error legible, con acción opcional de reintento. */
export function ErrorState({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-negativo/30 bg-negativo/5 py-10 text-center">
      <p className="max-w-md px-4 text-sm text-negativo">{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="rounded-md border border-border px-3 py-1.5 text-sm text-content hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          Reintentar
        </button>
      )}
    </div>
  );
}

/** Estado vacío con mensaje y acción opcional. */
export function EmptyState({ message, children }: { message: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-12 text-center text-sm text-content-muted">
      <p>{message}</p>
      {children}
    </div>
  );
}
