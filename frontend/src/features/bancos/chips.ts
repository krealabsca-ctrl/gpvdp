/**
 * Mapeo de estados de dominio a tono + etiqueta de <Badge>.
 * Centralizado para que todas las pantallas muestren los mismos colores.
 */

import type { BadgeTone } from "@/components/ui";
import type { EstadoClasificacion, EstadoDuplicado } from "@/api/bancos";

export function chipEstadoClasificacion(
  estado: EstadoClasificacion,
): { tone: BadgeTone; label: string } {
  switch (estado) {
    case "REVISADO":
      return { tone: "positivo", label: "Revisado" };
    case "AUTO":
      return { tone: "accent", label: "Auto" };
    case "NO_IDENTIFICADO":
    default:
      return { tone: "pendiente", label: "No identificado" };
  }
}

export function chipEstadoDuplicado(
  estado: EstadoDuplicado,
): { tone: BadgeTone; label: string } {
  switch (estado) {
    case "NUEVO":
      return { tone: "positivo", label: "Nuevo" };
    case "REIMPORTACION":
      return { tone: "accent", label: "Reimportación" };
    case "DUPLICADO_REAL":
    default:
      return { tone: "negativo", label: "Duplicado real" };
  }
}
