/**
 * Hooks de la vista consolidada del grupo.
 *
 * Solo lectura: no hay mutaciones ni, por lo tanto, invalidaciones. Si algún día este módulo
 * escribiera algo, dejaría de ser una vista.
 */

import { useQuery } from "@tanstack/react-query";
import { queryKeys } from "@/api/queryKeys";
import { grupoApi } from "@/api/grupo";
import { esStatus } from "@/lib/apiError";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

/** Consolidado del período. `placeholderData` deja el número anterior a la vista al cambiar de mes. */
export function useResumenGrupo(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.grupo.resumen(empresaId, periodo),
    queryFn: () => grupoApi.resumen(periodo),
    placeholderData: (prev) => prev,
    // El proyecto reintenta una vez por defecto, pero un 403 no cambia por volver a pedirlo: solo
    // gasta un pedido y deja a la persona mirando el spinner antes de leer por qué no tiene acceso.
    retry: (intentos, error) => !esStatus(error, 403) && intentos < 1,
  });
}
