/**
 * Hooks de los ratios del ciclo de efectivo. Solo leen: este módulo no escribe nada.
 */

import { useQuery } from "@tanstack/react-query";

import { queryKeys } from "@/api/queryKeys";
import { getCicloDeEfectivo } from "@/api/ratios";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

/** DSO, DPO y el ciclo de la ventana pedida. */
export function useCicloDeEfectivo(meses: number) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.ratios.ciclo(empresaId, meses),
    queryFn: () => getCicloDeEfectivo(meses),
  });
}
