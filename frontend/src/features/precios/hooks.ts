/**
 * Datos de la Calculadora de Precios (TanStack Query).
 *
 * ── La regla de invalidación de este módulo ──────────────────────────────────
 *
 * Capturar un costo invalida TAMBIÉN el costeo de todos los planes, no solo el catálogo. Un
 * insumo está en la receta de varios planes a la vez: si se captura el costo del cofre y el
 * costeo de Oro se queda con el número viejo, la pantalla muestra un margen que ya no existe y
 * nadie tiene forma de saber cuál de los dos es el bueno.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { simular } from "@/api/precios";
import {
  comisionDelPlan,
  crearPolitica,
  eliminarPolitica,
  listarPoliticas,
  listarVendedores,
} from "@/api/precios";
import {
  absorcionDelPlan,
  crearDriver,
  eliminarDriver,
  fijarBase,
  fijarConsumo,
  fijarReparto,
  listarDrivers,
  listarRepartos,
  quitarConsumo,
  quitarReparto,
  repartir,
} from "@/api/precios";
import {
  actualizarInsumo,
  actualizarPlan,
  agregarALaReceta,
  cambiarCantidad,
  capturarCosto,
  costear,
  crearInsumo,
  crearPlan,
  eliminarInsumo,
  historialDeCosto,
  listarInsumos,
  listarPlanes,
  quitarDeLaReceta,
} from "@/api/precios";
import type { Insumo, PlanPrecio } from "@/api/precios";
import { queryKeys } from "@/api/queryKeys";
import { useAuth } from "@/features/auth/AuthContext";

/** La empresa activa. Va en cada clave: sin ella, cambiar de empresa mostraría datos de la otra. */
function useEmpresaId(): string {
  const { empresaActiva } = useAuth();
  return empresaActiva?.id ?? "sin-empresa";
}

/** Invalida todo lo que un cambio de costo puede haber movido. Ver cabecera. */
function useInvalidarCosteo() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return () => {
    const k = queryKeys.precios;
    for (const raiz of [
      k.insumosRaiz(empresaId),
      k.historialRaiz(empresaId),
      k.costeoRaiz(empresaId),
      k.planesRaiz(empresaId),
    ]) {
      void qc.invalidateQueries({ queryKey: raiz });
    }
  };
}

export interface FiltroInsumos {
  fecha?: string;
  busca?: string;
  tipo?: string;
  sinCosto?: boolean;
}

export function useInsumos(filtro: FiltroInsumos) {
  const empresaId = useEmpresaId();
  const clave = JSON.stringify(filtro);
  return useQuery({
    queryKey: queryKeys.precios.insumos(empresaId, clave),
    queryFn: () => listarInsumos(filtro),
  });
}

export function useHistorialDeCosto(insumoId: string | null) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.historial(empresaId, insumoId ?? "ninguno"),
    queryFn: () => historialDeCosto(insumoId as string),
    enabled: Boolean(insumoId),
  });
}

export function usePlanes() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.planes(empresaId),
    queryFn: listarPlanes,
  });
}

export function useCosteo(planId: string | null, fecha: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.costeo(empresaId, planId ?? "ninguno", fecha),
    queryFn: () => costear(planId as string, fecha),
    enabled: Boolean(planId),
  });
}

export function useCrearInsumo() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: (body: Partial<Insumo>) => crearInsumo(body),
    onSuccess: invalidar,
  });
}

export function useActualizarInsumo() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<Insumo> }) => actualizarInsumo(id, body),
    onSuccess: invalidar,
  });
}

export function useEliminarInsumo() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: (id: string) => eliminarInsumo(id),
    onSuccess: invalidar,
  });
}

export function useCapturarCosto() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: ({
      insumoId,
      costo,
      vigenteDesde,
      nota,
    }: {
      insumoId: string;
      costo: string;
      vigenteDesde: string;
      nota?: string;
    }) => capturarCosto(insumoId, { costo, vigente_desde: vigenteDesde, nota }),
    onSuccess: invalidar,
  });
}

export function useCrearPlan() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: (body: Partial<PlanPrecio>) => crearPlan(body),
    onSuccess: invalidar,
  });
}

export function useActualizarPlan() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<PlanPrecio> }) => actualizarPlan(id, body),
    onSuccess: invalidar,
  });
}

export function useAgregarALaReceta() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: ({
      planId,
      insumoId,
      cantidad,
    }: {
      planId: string;
      insumoId: string;
      cantidad: string;
    }) => agregarALaReceta(planId, { insumo_id: insumoId, cantidad }),
    onSuccess: invalidar,
  });
}

export function useCambiarCantidad() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: ({ lineaId, cantidad }: { lineaId: string; cantidad: string }) =>
      cambiarCantidad(lineaId, cantidad),
    onSuccess: invalidar,
  });
}

export function useQuitarDeLaReceta() {
  const invalidar = useInvalidarCosteo();
  return useMutation({
    mutationFn: (lineaId: string) => quitarDeLaReceta(lineaId),
    onSuccess: invalidar,
  });
}

// ── Costeo por actividad ────────────────────────────────────────────────────────────────────
//
// Cambiar un reparto, una base o un driver invalida TAMBIÉN el costeo de los planes: el
// indirecto que absorbe cada uno sale de ahí, y dejarlo con el número viejo mostraría un margen
// que ya no existe.

function useInvalidarABC() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return () => {
    const k = queryKeys.precios;
    for (const raiz of [
      k.driversRaiz(empresaId),
      k.repartoRaiz(empresaId),
      k.absorcionRaiz(empresaId),
      k.costeoRaiz(empresaId),
    ]) {
      void qc.invalidateQueries({ queryKey: raiz });
    }
  };
}

export function useDrivers(periodo = "") {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.drivers(empresaId, periodo),
    queryFn: () => listarDrivers(periodo || undefined),
  });
}

export function useReparto(desde: string, hasta: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.reparto(empresaId, desde, hasta),
    queryFn: () => repartir(desde, hasta),
    enabled: Boolean(desde && hasta),
  });
}

export function useRepartosDeGasto(desde: string, hasta: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: [...queryKeys.precios.reparto(empresaId, desde, hasta), "detalle"] as const,
    queryFn: () => listarRepartos(desde, hasta),
    enabled: Boolean(desde && hasta),
  });
}

export function useAbsorcion(planId: string | null, desde: string, hasta: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.absorcion(empresaId, planId ?? "ninguno", desde, hasta),
    queryFn: () => absorcionDelPlan(planId as string, desde, hasta),
    enabled: Boolean(planId && desde && hasta),
  });
}

export function useCrearDriver() {
  const invalidar = useInvalidarABC();
  return useMutation({
    mutationFn: (body: { codigo: string; nombre: string; unidad: string; descripcion?: string }) =>
      crearDriver(body),
    onSuccess: invalidar,
  });
}

export function useEliminarDriver() {
  const invalidar = useInvalidarABC();
  return useMutation({ mutationFn: (id: string) => eliminarDriver(id), onSuccess: invalidar });
}

export function useFijarBase() {
  const invalidar = useInvalidarABC();
  return useMutation({
    mutationFn: ({
      driverId,
      periodo,
      cantidad,
      nota,
    }: {
      driverId: string;
      periodo: string;
      cantidad: string;
      nota?: string;
    }) => fijarBase(driverId, { periodo, cantidad, nota }),
    onSuccess: invalidar,
  });
}

export function useFijarReparto() {
  const invalidar = useInvalidarABC();
  return useMutation({
    mutationFn: (body: {
      clasificacion_id: string;
      driver_id: string;
      porcentaje: string;
      vigente_desde: string;
    }) => fijarReparto(body),
    onSuccess: invalidar,
  });
}

export function useQuitarReparto() {
  const invalidar = useInvalidarABC();
  return useMutation({ mutationFn: (id: string) => quitarReparto(id), onSuccess: invalidar });
}

export function useFijarConsumo() {
  const invalidar = useInvalidarABC();
  return useMutation({
    mutationFn: ({
      planId,
      driverId,
      consumo,
    }: {
      planId: string;
      driverId: string;
      consumo: string;
    }) => fijarConsumo(planId, { driver_id: driverId, consumo }),
    onSuccess: invalidar,
  });
}

export function useQuitarConsumo() {
  const invalidar = useInvalidarABC();
  return useMutation({
    mutationFn: ({ planId, driverId }: { planId: string; driverId: string }) =>
      quitarConsumo(planId, driverId),
    onSuccess: invalidar,
  });
}

// ── Políticas de comisión ───────────────────────────────────────────────────────────────────
//
// Cambiar una política invalida también el costeo: la comisión es parte del costo del plan, y
// dejarlo con el número viejo mostraría un margen que ya no existe.

function useInvalidarComisiones() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return () => {
    const k = queryKeys.precios;
    for (const raiz of [k.comisionesRaiz(empresaId), k.costeoRaiz(empresaId)]) {
      void qc.invalidateQueries({ queryKey: raiz });
    }
  };
}

export function usePoliticas() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.comisiones(empresaId),
    queryFn: listarPoliticas,
  });
}

export function useVendedores() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.vendedores(empresaId),
    queryFn: listarVendedores,
  });
}

export function useComisionDelPlan(planId: string | null, vendedor: string, fecha: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.precios.comisionDelPlan(empresaId, planId ?? "ninguno", vendedor, fecha),
    queryFn: () => comisionDelPlan(planId as string, { vendedor, fecha }),
    enabled: Boolean(planId),
  });
}

export function useCrearPolitica() {
  const invalidar = useInvalidarComisiones();
  return useMutation({
    mutationFn: (body: Parameters<typeof crearPolitica>[0]) => crearPolitica(body),
    onSuccess: invalidar,
  });
}

export function useEliminarPolitica() {
  const invalidar = useInvalidarComisiones();
  return useMutation({ mutationFn: (id: string) => eliminarPolitica(id), onSuccess: invalidar });
}

/** El simulador no escribe nada, así que es una mutación solo por llevar cuerpo en el POST. */
export function useSimular() {
  return useMutation({
    mutationFn: ({ planId, body }: { planId: string; body: Parameters<typeof simular>[1] }) =>
      simular(planId, body),
  });
}
