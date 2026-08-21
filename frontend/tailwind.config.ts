import type { Config } from "tailwindcss";

/**
 * Estética SOBRIA financiera:
 * - Neutros zinc/slate + un único acento (teal petróleo).
 * - Colores mapeados a variables CSS (ver src/index.css) para soportar
 *   modo claro/oscuro con la estrategia `class`.
 * - Fuente principal deliberadamente NO-Inter para evitar el "look de IA".
 */
const config: Config = {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        // Tipografía premium: Inter (variable, self-host offline) con fallback al
        // stack del sistema. Alta legibilidad para cifras y tablas densas.
        sans: [
          "Inter Variable",
          "Inter",
          "ui-sans-serif",
          "system-ui",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "Roboto",
          "Helvetica Neue",
          "Arial",
          "sans-serif",
        ],
        // Números en tablas/cifras: monoespaciada tabular.
        mono: [
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Consolas",
          "Liberation Mono",
          "monospace",
        ],
      },
      colors: {
        // Tokens semánticos -> variables CSS con canal RGB (permite /opacidad).
        surface: "rgb(var(--surface) / <alpha-value>)",
        "surface-muted": "rgb(var(--surface-muted) / <alpha-value>)",
        "surface-raised": "rgb(var(--surface-raised) / <alpha-value>)",
        border: "rgb(var(--border) / <alpha-value>)",
        content: "rgb(var(--content) / <alpha-value>)",
        "content-muted": "rgb(var(--content-muted) / <alpha-value>)",
        accent: {
          DEFAULT: "rgb(var(--accent) / <alpha-value>)",
          hover: "rgb(var(--accent-hover) / <alpha-value>)",
          fg: "rgb(var(--accent-fg) / <alpha-value>)",
        },
        // Dorado sobrio de marca (detalles del logotipo / realces).
        "brand-gold": "rgb(var(--brand-gold) / <alpha-value>)",
        // Estados para tablas/cifras futuras del módulo Bancos.
        positivo: "rgb(var(--positivo) / <alpha-value>)",
        negativo: "rgb(var(--negativo) / <alpha-value>)",
        pendiente: "rgb(var(--pendiente) / <alpha-value>)",
      },
      fontFeatureSettings: {
        tabular: '"tnum" 1, "lnum" 1',
      },
      // Sombras suaves (premium): elevación sutil, nunca duras. Tintadas slate.
      boxShadow: {
        card: "0 1px 2px 0 rgb(15 23 42 / 0.04), 0 1px 3px 0 rgb(15 23 42 / 0.06)",
        soft: "0 2px 8px -2px rgb(15 23 42 / 0.08)",
        lifted: "0 10px 30px -8px rgb(15 23 42 / 0.15)",
      },
    },
  },
  plugins: [],
};

export default config;
