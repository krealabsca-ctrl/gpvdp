/**
 * Hooks de TanStack Query del módulo Configuración.
 *
 * Reglas (skill gpvdp-data-layer): el estado de servidor vive SOLO acá, con la clave namespaced
 * por empresa activa (`queryKeys.config.*`, que lleva la empresa SEGUNDA para que la raíz
 * invalide por prefijo).
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/api/queryKeys";
import { correoApi, type CorreoSalienteInput } from "@/api/correo";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

/** Desde qué buzón manda la empresa activa. Nunca trae la contraseña: solo `tiene_password`. */
export function useCorreoSaliente() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.config.correoSaliente(empresaId),
    queryFn: () => correoApi.obtener(),
    staleTime: 30_000,
  });
}

export function useGuardarCorreoSaliente() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CorreoSalienteInput) => correoApi.guardar(input),
    onSuccess: () =>
      void qc.invalidateQueries({ queryKey: queryKeys.config.correoSaliente(empresaId) }),
  });
}

/**
 * Prueba LO GUARDADO. Invalida la configuración aunque la prueba falle: el resultado se guarda en
 * `probado_en` / `probado_error`, así que después de un fallo la pantalla tiene que releerlo —si
 * no, sigue mostrando «la última prueba salió bien», que es justo lo contrario de lo que pasó.
 */
export function useProbarCorreoSaliente() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => correoApi.probar(),
    onSettled: () =>
      void qc.invalidateQueries({ queryKey: queryKeys.config.correoSaliente(empresaId) }),
  });
}
