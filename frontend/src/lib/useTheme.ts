/**
 * Hook de tema claro/oscuro con estrategia `class` de Tailwind.
 * Persiste la preferencia en localStorage (clave gpvdp.theme).
 * El tema inicial ya se aplica en index.html para evitar parpadeo.
 */
import { useCallback, useEffect, useState } from "react";

type Theme = "light" | "dark";
const THEME_KEY = "gpvdp.theme";

function currentTheme(): Theme {
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(() => currentTheme());

  const applyTheme = useCallback((next: Theme) => {
    const root = document.documentElement;
    root.classList.toggle("dark", next === "dark");
    localStorage.setItem(THEME_KEY, next);
    setThemeState(next);
  }, []);

  const toggle = useCallback(() => {
    applyTheme(currentTheme() === "dark" ? "light" : "dark");
  }, [applyTheme]);

  // Sincroniza si otra pestaña cambia el tema.
  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === THEME_KEY && (e.newValue === "light" || e.newValue === "dark")) {
        applyTheme(e.newValue);
      }
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, [applyTheme]);

  return { theme, toggle, setTheme: applyTheme };
}
