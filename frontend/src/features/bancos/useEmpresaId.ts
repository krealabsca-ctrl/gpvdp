/**
 * Devuelve el id de la empresa activa para namespacing de query keys.
 * Dentro del AppShell protegido la empresa activa SIEMPRE existe (el guard
 * redirige a /seleccionar-empresa si no), por eso lanza si falta: es un
 * invariante del árbol de rutas, no un caso de UI.
 */

import { useAuth } from "@/features/auth/AuthContext";

export function useEmpresaId(): string {
  const { empresaActiva } = useAuth();
  if (!empresaActiva) {
    throw new Error("No hay empresa activa; ruta protegida usada fuera de contexto.");
  }
  return empresaActiva.id;
}
