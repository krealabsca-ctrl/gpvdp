/**
 * Permisos efectivos del usuario en la empresa activa (RBAC).
 * Estado de servidor → TanStack Query, keyed por empresa (como el resto de la app).
 * `useTienePermiso()` devuelve un predicado para ocultar acciones que el usuario
 * no puede ejecutar (el backend igual las deniega; esto es solo UX).
 */

import { useQuery } from "@tanstack/react-query";
import { rbacApi } from "@/api/rbac";
import { useAuth } from "@/features/auth/AuthContext";

export function usePermisos() {
  const { empresaActiva, status } = useAuth();
  return useQuery({
    queryKey: ["rbac", "mis-permisos", empresaActiva?.id ?? "none"],
    queryFn: () => rbacApi.misPermisos(),
    enabled: status === "authenticated" && !!empresaActiva,
    staleTime: 60_000,
  });
}

/**
 * Predicado de permiso. Mientras carga (o si falla), devuelve `true` para NO
 * ocultar de más — la autorización real la impone el backend (deny-by-default).
 */
export function useTienePermiso(): (permiso: string) => boolean {
  const { data } = usePermisos();
  return (permiso: string) => {
    if (!data) return true; // aún cargando: no parpadear ocultando
    return data.es_admin || data.permisos.includes(permiso);
  };
}

/**
 * Variante para GUARDS de ruta: espera a que los permisos carguen antes de decidir
 * (no devuelve true optimista como useTienePermiso, que dejaría montar la página y
 * disparar los 403). `loading` mientras resuelve; `permitido` una vez cargado.
 */
export function usePuedeVer(permiso?: string): { loading: boolean; permitido: boolean } {
  const { data, isPending } = usePermisos();
  if (!permiso) return { loading: false, permitido: true };
  if (!data) return { loading: isPending, permitido: false };
  return { loading: false, permitido: data.es_admin || data.permisos.includes(permiso) };
}
