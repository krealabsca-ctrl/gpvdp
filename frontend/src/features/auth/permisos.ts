/**
 * Permisos efectivos del usuario en la empresa activa (RBAC).
 * Estado de servidor → TanStack Query, keyed por empresa (como el resto de la app).
 * `useTienePermiso()` devuelve un predicado para ocultar acciones que el usuario
 * no puede ejecutar (el backend igual las deniega; esto es solo UX).
 *
 * ESTE ARCHIVO ERA EL SEGUNDO ESCALÓN DE UNA CASCADA DE TRES (10-nov-2026). Los permisos esperaban
 * a que `GET /me` respondiera para saber en qué empresa estaba el usuario, y la página esperaba a
 * los permisos para montarse: tres viajes al servidor EN FILA antes de pedir el primer dato.
 * Medido en la Bandeja con la red en cero: 211 ms de pura espera. En el servidor de verdad, cada
 * escalón cuesta un viaje completo, y por eso se sentía sobre todo al refrescar.
 *
 * Los dos escalones se quitaron sin tocar el backend:
 *   1. La empresa sale del TOKEN (ya venía ahí), así que esta consulta arranca a la vez que /me.
 *   2. Los permisos se recuerdan entre recargas, así que la página se monta de una y revalida
 *      mientras tanto.
 */

import { useQuery } from "@tanstack/react-query";
import { rbacApi } from "@/api/rbac";
import { useAuth } from "@/features/auth/AuthContext";
import { getEmpresaIdFromToken } from "@/lib/jwt";
import { leerPermisosGuardados, guardarPermisos } from "@/lib/permisosCache";
import { tokenStore } from "@/lib/tokenStore";

export function usePermisos() {
  // `useAuth` NO se usa para decidir la empresa —ese era el escalón de la cascada— pero sí para
  // provocar un render cuando la sesión cambia: al elegir otra empresa, el token nuevo ya está
  // guardado y este render lo vuelve a leer con la clave correcta.
  useAuth();
  // El claim `empresa_id` del propio token: ya estaba ahí desde que se eligió la empresa, así que
  // no hay nada que esperar. Se reusa `getEmpresaIdFromToken` (lib/jwt) en vez de decodificar acá:
  // esa versión arregla el relleno de base64 y los acentos, y dos decodificadores del mismo token
  // se separan en cuanto alguien toque uno.
  const empresaId = getEmpresaIdFromToken(tokenStore.getAccess());
  const cacheados = leerPermisosGuardados(empresaId);

  return useQuery({
    queryKey: ["rbac", "mis-permisos", empresaId ?? "none"],
    queryFn: async () => {
      const data = await rbacApi.misPermisos();
      guardarPermisos(empresaId, data);
      return data;
    },
    // Arranca en cuanto el token trae empresa, sin esperar a /me. Un token sin empresa es una
    // sesión que todavía no eligió una: pedir los permisos ahí daría 403.
    enabled: empresaId !== null,
    staleTime: 60_000,
    initialData: cacheados,
    // Los datos sembrados nacen VIEJOS (updatedAt = 0) para que se revaliden de inmediato. Sin
    // esto, `staleTime` los daría por frescos un minuto y un permiso recién quitado tardaría ese
    // minuto en notarse.
    initialDataUpdatedAt: cacheados ? 0 : undefined,
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
 *
 * Con los permisos recordados de la sesión anterior, `data` ya existe en el primer render y esto
 * deja de bloquear: la página se monta de una y pide sus datos mientras la revalidación viaja.
 */
export function usePuedeVer(permiso?: string): { loading: boolean; permitido: boolean } {
  const { data, isPending } = usePermisos();
  if (!permiso) return { loading: false, permitido: true };
  if (!data) return { loading: isPending, permitido: false };
  return { loading: false, permitido: data.es_admin || data.permisos.includes(permiso) };
}

