/**
 * Hooks del flujo de caja a 13 semanas.
 *
 * Guardar la política invalida la RAÍZ y no solo la política: el tope semanal es lo que produce
 * la curva, así que cambiarlo cambia las trece semanas. Invalidar solo la política dejaría la
 * pantalla mostrando el gráfico viejo al lado del tope nuevo.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { queryKeys } from "@/api/queryKeys";
import { getFlujoCaja, getPoliticaCaja, putPoliticaCaja, type PoliticaCaja } from "@/api/flujocaja";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

/** La proyección de una moneda. */
export function useFlujoCaja(moneda: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.flujoCaja.flujo(empresaId, moneda),
    queryFn: () => getFlujoCaja(moneda),
  });
}

/** La política vigente de esa moneda. */
export function usePoliticaCaja(moneda: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.flujoCaja.politica(empresaId, moneda),
    queryFn: () => getPoliticaCaja(moneda),
  });
}

/** Guarda la política y recalcula la curva. */
export function useGuardarPolitica() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: PoliticaCaja) => putPoliticaCaja(p),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.flujoCaja.raiz(empresaId) }),
  });
}
