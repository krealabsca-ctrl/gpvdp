/**
 * Hooks de tesorería (Tanda 1): saldo diario, checklist de carga y conciliación mensual.
 *
 * Invalidación: capturar un saldo mueve el cuadre del día Y el acta del mes (el saldo de
 * cierre del mes es un saldo diario), así que ambas cachés se refrescan juntas.
 */

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/api/queryKeys";
import { bancosApi, type PartidaInput, type SaldoInput } from "@/api/bancos";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

/** Refresca todo lo que depende de un saldo capturado o congelado. */
function invalidarTesoreria(qc: QueryClient, empresaId: string): void {
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.tesoreriaRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.conciliacionRaiz(empresaId) });
}

export function useTesoreria(fecha: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.tesoreria(empresaId, fecha),
    queryFn: () => bancosApi.tesoreria(fecha || undefined),
    staleTime: 30_000,
  });
}

export function useGuardarSaldos() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ fecha, saldos }: { fecha: string; saldos: SaldoInput[] }) =>
      bancosApi.guardarSaldos(fecha, saldos),
    onSuccess: () => invalidarTesoreria(qc, empresaId),
  });
}

export function useRevisarSaldos() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ fecha, congelar, motivo }: { fecha: string; congelar: boolean; motivo?: string }) =>
      bancosApi.revisarSaldos(fecha, congelar, motivo),
    onSuccess: () => invalidarTesoreria(qc, empresaId),
  });
}

export function useCargaPeriodo(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.carga(empresaId, periodo),
    queryFn: () => bancosApi.carga(periodo || undefined),
    staleTime: 60_000,
  });
}

export function useConciliacion(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.conciliacion(empresaId, periodo),
    queryFn: () => bancosApi.conciliacion(periodo),
    staleTime: 30_000,
  });
}

export function useRegistrarPartida() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: PartidaInput) => bancosApi.registrarPartida(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conciliacionRaiz(empresaId) });
    },
  });
}

export function useAnularPartida() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bancosApi.anularPartida(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conciliacionRaiz(empresaId) });
    },
  });
}

export function useFirmarActa() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ cuentaId, periodo }: { cuentaId: string; periodo: string }) =>
      bancosApi.firmarActa(cuentaId, periodo),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conciliacionRaiz(empresaId) });
      // Firmar habilita el cierre del período: el estado del período también cambia.
      void qc.invalidateQueries({ queryKey: ["bancos", "periodo", empresaId] });
    },
  });
}
