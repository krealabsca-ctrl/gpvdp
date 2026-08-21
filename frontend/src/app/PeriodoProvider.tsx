/**
 * Contexto de PERÍODO ACTIVO (global), al nivel de la empresa activa.
 *
 * El período es contexto ambiente (como la empresa): un único selector en la
 * navbar y todas las pantallas lo comparten. Se persiste en localStorage para
 * sobrevivir recargas. Formato "YYYY-MM".
 */

import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import { periodoActual } from "@/lib/format";

interface PeriodoContextValue {
  periodo: string;
  setPeriodo: (p: string) => void;
}

const PeriodoContext = createContext<PeriodoContextValue | null>(null);
const KEY = "gpvdp.periodo";

export function PeriodoProvider({ children }: { children: ReactNode }) {
  const [periodo, setPeriodoState] = useState<string>(() => {
    if (typeof window !== "undefined") {
      const saved = window.localStorage.getItem(KEY);
      if (saved && /^\d{4}-\d{2}$/.test(saved)) return saved;
    }
    return periodoActual();
  });

  const setPeriodo = useCallback((p: string) => {
    setPeriodoState(p);
    if (typeof window !== "undefined") window.localStorage.setItem(KEY, p);
  }, []);

  return (
    <PeriodoContext.Provider value={{ periodo, setPeriodo }}>{children}</PeriodoContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function usePeriodoActivo(): PeriodoContextValue {
  const ctx = useContext(PeriodoContext);
  if (!ctx) throw new Error("usePeriodoActivo debe usarse dentro de <PeriodoProvider>");
  return ctx;
}
