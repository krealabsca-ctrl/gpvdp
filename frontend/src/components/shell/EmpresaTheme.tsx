/**
 * Aplica el acento de marca de la EMPRESA activa como atributo `data-empresa`
 * en <html>. Las variables CSS de acento (ver src/index.css) se sobreescriben
 * por empresa, así que la UI y el logotipo (fill-accent) se re-tintan al cambiar
 * de empresa en el selector. No renderiza nada.
 */

import { useEffect } from "react";
import { useAuth } from "@/features/auth/AuthContext";

/** Deriva el slug de tema desde el nombre de la empresa (robusto a mayúsculas). */
function empresaSlug(nombre: string | null | undefined): string {
  const n = (nombre ?? "").toLowerCase();
  if (n.includes("coopeprofa")) return "coopeprofa";
  if (n.includes("memorial")) return "memorialpets";
  // Valle de Paz y el grupo por defecto usan el verde base.
  return "vdp";
}

export function EmpresaTheme() {
  const { empresaActiva } = useAuth();
  useEffect(() => {
    document.documentElement.dataset.empresa = empresaSlug(empresaActiva?.nombre);
  }, [empresaActiva?.nombre]);
  return null;
}
