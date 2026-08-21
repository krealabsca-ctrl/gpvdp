/**
 * Sistema de toasts mínimo (sin librería externa: Node no está disponible para
 * instalar deps). Provee un contexto con `toast.success/error/info` y renderiza
 * las notificaciones en una región aria-live para accesibilidad.
 *
 * Uso:
 *   const toast = useToast();
 *   toast.success("Importación confirmada: 42 insertados");
 *   toast.error(apiError.message);
 */

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { cn } from "@/lib/cn";

type ToastKind = "success" | "error" | "info";

interface ToastItem {
  id: number;
  kind: ToastKind;
  message: string;
}

interface ToastApi {
  success: (message: string) => void;
  error: (message: string) => void;
  info: (message: string) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

const DURACION_MS = 5000;

const kindStyles: Record<ToastKind, string> = {
  success: "border-positivo/40 bg-surface-raised text-content",
  error: "border-negativo/50 bg-surface-raised text-content",
  info: "border-border bg-surface-raised text-content",
};

const kindDot: Record<ToastKind, string> = {
  success: "bg-positivo",
  error: "bg-negativo",
  info: "bg-accent",
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const nextId = useRef(1);

  const remove = useCallback((id: number) => {
    setItems((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const push = useCallback(
    (kind: ToastKind, message: string) => {
      const id = nextId.current++;
      setItems((prev) => [...prev, { id, kind, message }]);
      window.setTimeout(() => remove(id), DURACION_MS);
    },
    [remove],
  );

  const api = useMemo<ToastApi>(
    () => ({
      success: (m) => push("success", m),
      error: (m) => push("error", m),
      info: (m) => push("info", m),
    }),
    [push],
  );

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div
        aria-live="polite"
        aria-atomic="false"
        className="pointer-events-none fixed bottom-4 right-4 z-50 flex w-full max-w-sm flex-col gap-2"
      >
        {items.map((t) => (
          <div
            key={t.id}
            role="status"
            className={cn(
              "pointer-events-auto flex items-start gap-3 rounded-xl border px-4 py-3 text-sm shadow-lifted",
              kindStyles[t.kind],
            )}
          >
            <span className={cn("mt-1.5 h-2 w-2 shrink-0 rounded-full", kindDot[t.kind])} />
            <p className="flex-1">{t.message}</p>
            <button
              type="button"
              onClick={() => remove(t.id)}
              aria-label="Cerrar notificación"
              className="shrink-0 rounded text-content-muted hover:text-content focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast debe usarse dentro de <ToastProvider>");
  return ctx;
}
