/**
 * Une clases condicionales (evita depender de clsx/tailwind-merge en Fase 0).
 * Filtra falsy y colapsa espacios.
 */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ").replace(/\s+/g, " ").trim();
}
