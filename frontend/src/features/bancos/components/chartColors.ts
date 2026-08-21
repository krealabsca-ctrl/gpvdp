/**
 * Paleta de series para los gráficos del módulo Bancos, VALIDADA con el
 * procedimiento de data-viz (banda de luminosidad OKLCH, piso de croma,
 * separación CVD protan/deutan/tritan y contraste ≥3:1 contra la superficie):
 *  - light: #0B7E53 / #B3542E sobre #F7F8F5 → ALL CHECKS PASS
 *  - dark:  #35A578 / #CC6647 sobre #171A18 → ALL CHECKS PASS
 * El dorado es realce de meta/selección (no es una serie).
 */

import { useEffect, useState } from "react";

export interface ChartColors {
  ingreso: string;
  gasto: string;
  oro: string;
  grid: string;
  tick: string;
}

export const CHART_COLORS: Record<"light" | "dark", ChartColors> = {
  light: { ingreso: "#0B7E53", gasto: "#B3542E", oro: "#B0894A", grid: "#E4E8E1", tick: "#8A958D" },
  dark: { ingreso: "#35A578", gasto: "#CC6647", oro: "#C9A468", grid: "#2A302B", tick: "#77817A" },
};

/** Colores de gráfico del tema vigente; reacciona al toggle claro/oscuro. */
export function useChartColors(): ChartColors {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    const obs = new MutationObserver(() =>
      setDark(document.documentElement.classList.contains("dark")),
    );
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    return () => obs.disconnect();
  }, []);
  return CHART_COLORS[dark ? "dark" : "light"];
}

const MES_CORTO = ["Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic"];

/** "2026-07" → "Jul 26" (etiqueta corta para ejes). */
export function etiquetaMesCorto(periodo: string): string {
  const [y, m] = periodo.split("-");
  return `${MES_CORTO[Number(m) - 1] ?? m} ${(y ?? "").slice(2)}`;
}
