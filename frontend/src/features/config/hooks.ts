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
import { sistemaApi, type FiltrosIncidentes } from "@/api/sistema";
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

// ---------------------------------------------------------------------------
// Los errores del sistema (mig 0107)
// ---------------------------------------------------------------------------

/**
 * Los errores internos registrados.
 *
 * `staleTime` en cero y sin refresco automático: esta pantalla se abre cuando alguien llama
 * diciendo «me dio error», y lo que busca acaba de pasar hace treinta segundos. Un dato cacheado
 * acá haría que la referencia recién emitida «no exista».
 */
export function useIncidentes(filtros: FiltrosIncidentes = {}) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.config.incidentes(empresaId, filtros),
    queryFn: () => sistemaApi.incidentes(filtros),
    staleTime: 0,
  });
}

/** Cuántos van y cuándo fue el último. Contesta «¿esto está pasando ahora?». */
export function useResumenIncidentes() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.config.resumenIncidentes(empresaId),
    queryFn: () => sistemaApi.resumen(),
    staleTime: 0,
  });
}
