/**
 * Traduce un error de `apiFetch` (ApiError o error de red) a un mensaje de UI
 * claro en español, respetando el mapeo de estados del proyecto:
 *   403 -> sin permiso en esta empresa
 *   409 -> conflicto (duplicado / período con No-identificados)
 *   422 -> regla de negocio (ej. formato de archivo no reconocido)
 *
 * Cuando el backend manda { code, message }, se prioriza su `message`.
 */

import { ApiError } from "@/api/client";

export function mensajeError(err: unknown): string {
  if (err instanceof ApiError) {
    // El backend suele dar un message legible; si no, mapear por status.
    if (err.message && err.message !== err.name) {
      return err.message;
    }
    switch (err.status) {
      case 403:
        return "No tenés permiso para esta acción en esta empresa.";
      case 404:
        return "No se encontró el recurso solicitado.";
      case 409:
        return "Conflicto: el recurso ya existe o el estado no lo permite.";
      case 422:
        return "No se pudo procesar: revisá los datos (regla de negocio).";
      default:
        return "Ocurrió un error al comunicarse con el servidor.";
    }
  }
  if (err instanceof Error && err.message) return err.message;
  return "Ocurrió un error inesperado.";
}

/** true si el error es un ApiError con el status indicado. */
export function esStatus(err: unknown, status: number): boolean {
  return err instanceof ApiError && err.status === status;
}

/**
 * Extrae el detalle `no_identificados` de un 409 de cierre de período.
 * El backend adjunta la cuenta de No-identificados en el cuerpo del error;
 * puede venir en la raíz o anidado bajo `detail`.
 */
export function noIdentificadosDe(err: unknown): number | null {
  if (!(err instanceof ApiError) || !err.body) return null;
  const raiz = err.body["no_identificados"];
  if (typeof raiz === "number") return raiz;
  const detail = err.body["detail"];
  if (detail && typeof detail === "object") {
    const anidado = (detail as Record<string, unknown>)["no_identificados"];
    if (typeof anidado === "number") return anidado;
  }
  return null;
}
