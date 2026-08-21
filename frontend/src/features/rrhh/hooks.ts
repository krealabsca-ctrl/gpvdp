/**
 * Hooks de TanStack Query del módulo RRHH / Nómina.
 *
 * Reglas (skill gpvdp-data-layer): estado de servidor SOLO aquí, namespaced por
 * empresa activa; invalidación explícita tras cada mutación.
 */

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/api/queryKeys";
import {
  rrhhApi,
  type ConceptoInput,
  type CorridaInput,
  type DeduccionInput,
  type EmpleadoInput,
  type FiltrosEmpleados,
  type FiniquitoInput,
  type IncapacidadInput,
  type NovedadInput,
  type ParametrosInput,
  type VacacionInput,
} from "@/api/rrhh";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

// ---------------------------------------------------------------------------
// Dashboard (Etapa 5)
// ---------------------------------------------------------------------------

/** Resumen del mes: costo real, ciclo y alertas. Se refresca al volver a la pestaña. */
export function useDashboardRRHH(anio: number, mes: number) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.dashboard(empresaId, anio, mes),
    queryFn: () => rrhhApi.dashboard(anio, mes),
    placeholderData: (prev) => prev,
  });
}

// ---------------------------------------------------------------------------
// Empleados
// ---------------------------------------------------------------------------

export function useEmpleados(filtros: FiltrosEmpleados) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.empleados(empresaId, filtros),
    queryFn: () => rrhhApi.empleados(filtros),
    placeholderData: (prev) => prev,
  });
}

function invalidarEmpleados(qc: QueryClient, empresaId: string) {
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.empleadosRaiz(empresaId) });
}

export function useCrearEmpleado() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: EmpleadoInput) => rrhhApi.crearEmpleado(input),
    onSuccess: () => invalidarEmpleados(qc, empresaId),
  });
}

export function useActualizarEmpleado() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; input: EmpleadoInput }) =>
      rrhhApi.actualizarEmpleado(vars.id, vars.input),
    onSuccess: (_data, vars) => {
      invalidarEmpleados(qc, empresaId);
      void qc.invalidateQueries({ queryKey: queryKeys.rrhh.empleado(empresaId, vars.id) });
    },
  });
}

export function useDesactivarEmpleado() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; fechaSalida?: string }) =>
      rrhhApi.desactivarEmpleado(vars.id, vars.fechaSalida),
    onSuccess: () => invalidarEmpleados(qc, empresaId),
  });
}

// ---------------------------------------------------------------------------
// Parámetros del año
// ---------------------------------------------------------------------------

export function useParametrosNomina(anio: number) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.parametros(empresaId, anio),
    queryFn: () => rrhhApi.parametros(anio),
  });
}

export function useGuardarParametros() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { anio: number; input: ParametrosInput }) =>
      rrhhApi.guardarParametros(vars.anio, vars.input),
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: queryKeys.rrhh.parametros(empresaId, vars.anio) });
    },
  });
}

// ---------------------------------------------------------------------------
// Conceptos
// ---------------------------------------------------------------------------

export function useConceptosNomina() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.conceptos(empresaId),
    queryFn: () => rrhhApi.conceptos(),
    staleTime: 60_000,
  });
}

function invalidarConceptos(qc: QueryClient, empresaId: string) {
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.conceptos(empresaId) });
}

export function useCrearConcepto() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ConceptoInput) => rrhhApi.crearConcepto(input),
    onSuccess: () => invalidarConceptos(qc, empresaId),
  });
}

export function useActualizarConcepto() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; input: ConceptoInput }) =>
      rrhhApi.actualizarConcepto(vars.id, vars.input),
    onSuccess: () => invalidarConceptos(qc, empresaId),
  });
}

export function useDesactivarConcepto() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.desactivarConcepto(id),
    onSuccess: () => invalidarConceptos(qc, empresaId),
  });
}

// ---------------------------------------------------------------------------
// Deducciones recurrentes
// ---------------------------------------------------------------------------

export function useDeducciones(empleadoId: string | null) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.deducciones(empresaId, empleadoId ?? ""),
    queryFn: () => rrhhApi.deducciones(empleadoId!),
    enabled: !!empleadoId,
  });
}

function invalidarDeducciones(qc: QueryClient, empresaId: string, empleadoId: string) {
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.deducciones(empresaId, empleadoId) });
  // El conteo de deducciones activas vive en la fila del empleado.
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.empleadosRaiz(empresaId) });
}

export function useCrearDeduccion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { empleadoId: string; input: DeduccionInput }) =>
      rrhhApi.crearDeduccion(vars.empleadoId, vars.input),
    onSuccess: (_data, vars) => invalidarDeducciones(qc, empresaId, vars.empleadoId),
  });
}

export function useActualizarDeduccion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { empleadoId: string; id: string; input: DeduccionInput }) =>
      rrhhApi.actualizarDeduccion(vars.empleadoId, vars.id, vars.input),
    onSuccess: (_data, vars) => invalidarDeducciones(qc, empresaId, vars.empleadoId),
  });
}

export function useDesactivarDeduccion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { empleadoId: string; id: string }) =>
      rrhhApi.desactivarDeduccion(vars.empleadoId, vars.id),
    onSuccess: (_data, vars) => invalidarDeducciones(qc, empresaId, vars.empleadoId),
  });
}

// ---------------------------------------------------------------------------
// Corrida quincenal
// ---------------------------------------------------------------------------

export function useCorridas(anio: number) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.corridas(empresaId, anio),
    queryFn: () => rrhhApi.corridas(anio),
  });
}

export function useCorrida(id: string | null) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.corrida(empresaId, id ?? ""),
    queryFn: () => rrhhApi.corrida(id!),
    enabled: !!id,
  });
}

function invalidarCorridas(qc: QueryClient, empresaId: string, id?: string) {
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.corridasRaiz(empresaId) });
  if (id) void qc.invalidateQueries({ queryKey: queryKeys.rrhh.corrida(empresaId, id) });
}

export function useCrearCorrida() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CorridaInput) => rrhhApi.crearCorrida(input),
    onSuccess: () => invalidarCorridas(qc, empresaId),
  });
}

export function useGuardarNovedades() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; novedades: NovedadInput[] }) =>
      rrhhApi.guardarNovedades(vars.id, vars.novedades),
    onSuccess: (_data, vars) => invalidarCorridas(qc, empresaId, vars.id),
  });
}

export function useRecalcularCorrida() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.recalcularCorrida(id),
    onSuccess: (_data, id) => invalidarCorridas(qc, empresaId, id),
  });
}

export function useAprobarCorrida() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.aprobarCorrida(id),
    onSuccess: (_data, id) => invalidarCorridas(qc, empresaId, id),
  });
}

export function usePagarCorrida() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.pagarCorrida(id),
    onSuccess: (_data, id) => {
      invalidarCorridas(qc, empresaId, id);
      // El pago descuenta saldos de deducciones: refrescar empleados y sus deducciones.
      void qc.invalidateQueries({ queryKey: queryKeys.rrhh.empleadosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: ["rrhh", "deducciones", empresaId] });
    },
  });
}

export function useAnularCorrida() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.anularCorrida(id),
    onSuccess: (_data, id) => invalidarCorridas(qc, empresaId, id),
  });
}

// ---------------------------------------------------------------------------
// Finiquitos y reportes (Etapa 3)
// ---------------------------------------------------------------------------

export function useFiniquitos() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.finiquitos(empresaId),
    queryFn: () => rrhhApi.finiquitos(),
  });
}

export function useFiniquito(id: string | null) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.finiquito(empresaId, id ?? ""),
    queryFn: () => rrhhApi.finiquito(id!),
    enabled: !!id,
  });
}

function invalidarFiniquitos(qc: QueryClient, empresaId: string, id?: string) {
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.finiquitos(empresaId) });
  if (id) void qc.invalidateQueries({ queryKey: queryKeys.rrhh.finiquito(empresaId, id) });
}

export function useCrearFiniquito() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: FiniquitoInput) => rrhhApi.crearFiniquito(input),
    onSuccess: () => invalidarFiniquitos(qc, empresaId),
  });
}

export function useActualizarFiniquito() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; input: FiniquitoInput }) =>
      rrhhApi.actualizarFiniquito(vars.id, vars.input),
    onSuccess: (_data, vars) => invalidarFiniquitos(qc, empresaId, vars.id),
  });
}

export function useAprobarFiniquito() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.aprobarFiniquito(id),
    onSuccess: (_data, id) => invalidarFiniquitos(qc, empresaId, id),
  });
}

export function usePagarFiniquito() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.pagarFiniquito(id),
    onSuccess: (_data, id) => {
      invalidarFiniquitos(qc, empresaId, id);
      // El pago da de baja la ficha y cierra sus deducciones.
      void qc.invalidateQueries({ queryKey: queryKeys.rrhh.empleadosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: ["rrhh", "deducciones", empresaId] });
    },
  });
}

export function useAnularFiniquito() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.anularFiniquito(id),
    onSuccess: (_data, id) => invalidarFiniquitos(qc, empresaId, id),
  });
}

export function useProvisiones(anio: number) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.provisiones(empresaId, anio),
    queryFn: () => rrhhApi.provisiones(anio),
  });
}

// ---------------------------------------------------------------------------
// Incapacidades y vacaciones (Etapa 4)
// ---------------------------------------------------------------------------

export function useIncapacidades(anio: number, mes: number) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.incapacidades(empresaId, anio, mes),
    queryFn: () => rrhhApi.incapacidades(anio, mes),
  });
}

/** Tras tocar una ausencia hay que refrescar también las corridas: alimentan el cálculo. */
function invalidarAusencias(qc: QueryClient, empresaId: string) {
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.incapacidadesRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.saldosVacacionesRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.vacacionesRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.rrhh.corridasRaiz(empresaId) });
}

export function useRegistrarIncapacidad() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: IncapacidadInput) => rrhhApi.registrarIncapacidad(input),
    onSuccess: () => invalidarAusencias(qc, empresaId),
  });
}

export function useAnularIncapacidad() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.anularIncapacidad(id),
    onSuccess: () => invalidarAusencias(qc, empresaId),
  });
}

export function useSaldosVacaciones(anio: number) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.saldosVacaciones(empresaId, anio),
    queryFn: () => rrhhApi.saldosVacaciones(anio),
  });
}

export function useVacaciones(empleadoId?: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.rrhh.vacaciones(empresaId, empleadoId),
    queryFn: () => rrhhApi.vacaciones(empleadoId),
  });
}

export function useRegistrarVacacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: VacacionInput) => rrhhApi.registrarVacacion(input),
    onSuccess: () => invalidarAusencias(qc, empresaId),
  });
}

export function useAnularVacacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rrhhApi.anularVacacion(id),
    onSuccess: () => invalidarAusencias(qc, empresaId),
  });
}

/**
 * Notificaciones al colaborador. No invalidan nada: enviar un correo no cambia ningún dato del
 * ERP (el resultado del envío se muestra en el momento).
 */
export function useEnviarBoletas() {
  return useMutation({ mutationFn: (corridaId: string) => rrhhApi.enviarBoletas(corridaId) });
}

export function useAvisarVacaciones() {
  return useMutation({ mutationFn: (vacacionId: string) => rrhhApi.avisarVacaciones(vacacionId) });
}
