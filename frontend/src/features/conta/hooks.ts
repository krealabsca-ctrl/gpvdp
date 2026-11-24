/**
 * Hooks del módulo Contabilidad (Fase 1: el plan de cuentas).
 *
 * Reglas del proyecto: el estado de servidor vive solo acá, con la clave namespaced por empresa
 * activa, e invalidación explícita tras cada mutación.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/api/queryKeys";
import {
  contaApi,
  type CuentaContable,
  type DecisionDeMapeo,
  type FiltroDiario,
  type NuevoAsiento,
} from "@/api/conta";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

/** El plan como árbol. Es la vista principal. */
export function usePlanDeCuentas(soloActivas = false) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.plan(empresaId, soloActivas),
    queryFn: () => contaApi.arbol(soloActivas),
  });
}

export function useCrearCuenta() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (c: Partial<CuentaContable>) => contaApi.crear(c),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

export function useActualizarCuenta() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; cambios: Partial<CuentaContable> }) =>
      contaApi.actualizar(v.id, v.cambios),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

export function useBorrarCuenta() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => contaApi.borrar(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

/**
 * Previsualiza el archivo. Mutación y no query: es una acción sobre un archivo que la persona
 * acaba de elegir, y cachearla mostraría el resultado del archivo anterior.
 */
export function usePrevisualizarPlan() {
  return useMutation({ mutationFn: (archivo: File) => contaApi.previsualizar(archivo) });
}

export function useImportarPlan() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { archivo: File; destino: "grupo" | "empresa" }) =>
      contaApi.importar(v.archivo, v.destino),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

// ── Fase 2 · el mapeo partida → cuenta (mig 0110) ────────────────────────────────────────────

/**
 * Las cuentas a las que se PUEDE mapear: activas y de movimiento.
 *
 * El filtro se hace acá y no en el servidor porque es la misma lista de siempre y el servidor ya
 * la sirve plana; lo que cambia es para qué se usa. Ofrecer una cuenta de agrupación en el
 * selector sería ofrecer algo que la base va a rechazar.
 */
export function useCuentasDeMovimiento() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.cuentasPlanas(empresaId),
    queryFn: () => contaApi.planas(true),
    select: (d) => d.items.filter((c) => c.acepta_movimiento),
  });
}

/** La lista del mapeo con su cobertura. */
export function useMapeoContable(pendientes: boolean, conMovimientos: boolean) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.mapeo(empresaId, pendientes, conMovimientos),
    queryFn: () => contaApi.mapeo({ pendientes, conMovimientos }),
  });
}

/**
 * Guarda una decisión o un lote.
 *
 * Invalida la RAÍZ de contabilidad, no la clave exacta: la pantalla cambia de filtro mientras
 * trabaja («solo pendientes» ↔ todas), y refrescar únicamente la combinación activa dejaría las
 * otras mostrando la cobertura vieja.
 */
export function useDecidirMapeo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (decisiones: DecisionDeMapeo[]) => contaApi.decidir(decisiones),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

export function useDeshacerMapeo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (clasificacionId: string) => contaApi.deshacer(clasificacionId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

/**
 * Sube el .xlsx del mapeo lleno (12-nov-2026).
 *
 * Va acá y no suelto en el panel por la misma razón que los dos de arriba: así la pantalla
 * depende de un hook del módulo que las pruebas pueden mockear, en vez de arrastrar la sesión y
 * el cliente de consultas a cada prueba que la monte.
 */
export function useImportarMapeoXLSX() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (archivo: File) => contaApi.importarMapeoXLSX(archivo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

// ── Fase 3A · la contrapartida bancaria y los libros (mig 0111) ──────────────────────────────

/** Las cuentas bancarias con su cuenta contable y la sugerencia por IBAN. */
export function useBancosContables() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.bancos(empresaId),
    queryFn: () => contaApi.bancos(),
  });
}

export function useEnlazarBanco() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { cuentaBancariaId: string; cuentaId: string }) =>
      contaApi.enlazarBanco(v.cuentaBancariaId, v.cuentaId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

export function useDesenlazarBanco() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (cuentaBancariaId: string) => contaApi.desenlazarBanco(cuentaBancariaId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

export function useAceptarSugerencias() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => contaApi.aceptarSugerencias(),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

/** El libro diario. */
export function useDiario(f: FiltroDiario) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.diario(empresaId, {
      desde: f.desde ?? "", hasta: f.hasta ?? "", estado: f.estado ?? "", origen: f.origen ?? "",
    }),
    queryFn: () => contaApi.diario(f),
  });
}

/** Un asiento con sus líneas. `enabled` evita pedirlo mientras no hay ninguno abierto. */
export function useAsiento(id: string | null) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.asiento(empresaId, id ?? ""),
    queryFn: () => contaApi.asiento(id as string),
    enabled: !!id,
  });
}

/** El mayor de una cuenta. Sin cuenta elegida no se pide nada. */
export function useMayor(cuentaId: string, desde: string, hasta: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.mayor(empresaId, cuentaId, desde, hasta),
    queryFn: () => contaApi.mayor(cuentaId, desde, hasta),
    enabled: !!cuentaId,
  });
}

export function useCrearAsiento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (a: NuevoAsiento) => contaApi.crearAsiento(a),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

// ── Fase 3B · generar asientos desde el banco (mig 0112) ─────────────────────────────────────

/** Qué pasaría al generar el período. No escribe nada. */
export function usePrevisualizarGeneracion(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.generacion(empresaId, periodo),
    queryFn: () => contaApi.previsualizarGeneracion(periodo),
    enabled: !!periodo,
  });
}

export function useGenerarAsientos() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (periodo: string) => contaApi.generar(periodo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

export function useRevertirGeneracion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { periodo: string; motivo: string }) =>
      contaApi.revertirGeneracion(v.periodo, v.motivo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

// ── Fase 4 · balance, apertura y cierre de período (mig 0113) ────────────────────────────────

export function useBalance(desde: string, hasta: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.balance(empresaId, desde, hasta),
    queryFn: () => contaApi.balance(desde, hasta),
    enabled: !!desde && !!hasta,
  });
}

export function usePeriodosContables() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.conta.periodos(empresaId),
    queryFn: () => contaApi.periodos(),
  });
}

export function useCerrarPeriodo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (periodo: string) => contaApi.cerrarPeriodo(periodo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

export function useReabrirPeriodo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { periodo: string; motivo: string }) =>
      contaApi.reabrirPeriodo(v.periodo, v.motivo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

/** Mutación y no query: es una acción sobre un archivo que la persona acaba de elegir. */
export function usePrevisualizarApertura() {
  return useMutation({
    mutationFn: (v: { periodo: string; archivo: File }) =>
      contaApi.previsualizarApertura(v.periodo, v.archivo),
  });
}

export function useCargarApertura() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { periodo: string; archivo: File }) =>
      contaApi.cargarApertura(v.periodo, v.archivo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}

export function useAnularAsiento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; motivo: string }) => contaApi.anularAsiento(v.id, v.motivo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.conta.raiz(empresaId) }),
  });
}
