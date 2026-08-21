/**
 * Estado de servidor de CxC. Toda clave lleva la empresa activa: cambiar de empresa no
 * puede mostrar la cartera de otra.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  arreglosApi,
  asociacionesApi,
  cobrosApi,
  colaApi,
  configCxcApi,
  cxcApi,
  notasApi,
  planillasApi,
  preventivoApi,
  suspensionApi,
  type NuevoArreglo,
  type NuevaNota,
  type PlanillaDetalle,
  type CambioTramo,
  type FiltrosCartera,
  type FiltrosCobros,
  type FiltrosCola,
  type NuevaGestion,
  type NuevoCobro,
} from "@/api/cxc";
import { queryKeys } from "@/api/queryKeys";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

export function useCatalogosCxc() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.catalogos(empresaId),
    queryFn: () => cxcApi.catalogos(),
    // Los catálogos cambian poco y los usan todas las pantallas del módulo.
    staleTime: 5 * 60 * 1000,
  });
}

export function useCartera(filtros: FiltrosCartera) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.contratos(empresaId, filtros),
    queryFn: () => cxcApi.contratos(filtros),
    placeholderData: (prev) => prev, // filtrar y paginar sin parpadeo
  });
}

export function useContrato(numero: string, soloAbiertos = false) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.contrato(empresaId, numero, soloAbiertos),
    queryFn: () => cxcApi.contrato(numero, soloAbiertos),
    enabled: numero !== "",
  });
}

/** El plan del generador: cuántos cargos crearía. `desde` vacío = no se consulta. */
export function usePlanCargos(desde: string, hasta: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.planCargos(empresaId, desde, hasta),
    queryFn: () => cxcApi.planCargos(desde, hasta),
    enabled: desde !== "",
    // Si el rango es inválido el servidor responde 422: no se reintenta.
    retry: false,
  });
}

export function usePrevisualizarContratos() {
  return useMutation({ mutationFn: (archivo: File) => cxcApi.previsualizarContratos(archivo) });
}

export function useConfirmarContratos() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ archivo, importacionId }: { archivo: File; importacionId: string }) =>
      cxcApi.confirmarContratos(archivo, importacionId),
    // Importar cambia la cartera Y los conteos de los catálogos (sedes y asociaciones
    // nuevas): se invalida el módulo completo.
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

export function useGenerarCargos() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ desde, hasta, total }: { desde: string; hasta: string; total: number }) =>
      cxcApi.generarCargos(desde, hasta, total),
    // Los cargos son la fuente del saldo derivado: al crearlos, toda la cartera cambia.
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

// ───────────────────────── Cobros (fase 2) ─────────────────────────

export function useCobros(filtros: FiltrosCobros) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.cobros(empresaId, filtros),
    queryFn: () => cobrosApi.listar(filtros),
    placeholderData: (prev) => prev,
  });
}

/**
 * Las tres mutaciones de cobros invalidan el módulo COMPLETO, no solo la lista: aplicar,
 * reversar o identificar cambia el saldo de los cargos y por lo tanto la cartera entera.
 * Invalidar de menos dejaría la pantalla mostrando una deuda que ya se pagó.
 */
function useMutacionDeCobro<TVars, TData>(fn: (v: TVars) => Promise<TData>) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

export function useRegistrarCobro() {
  return useMutacionDeCobro((cobro: NuevoCobro) => cobrosApi.registrar(cobro));
}

export function useReversarCobro() {
  return useMutacionDeCobro(({ id, motivo }: { id: string; motivo: string }) => cobrosApi.reversar(id, motivo));
}

export function useIdentificarCobro() {
  return useMutacionDeCobro(({ id, contrato }: { id: string; contrato: string }) =>
    cobrosApi.identificar(id, contrato),
  );
}

export function usePrevisualizarCobros() {
  return useMutation({ mutationFn: (archivo: File) => cobrosApi.previsualizar(archivo) });
}

export function useConfirmarCobros() {
  return useMutacionDeCobro(({ archivo, importacionId }: { archivo: File; importacionId: string }) =>
    cobrosApi.confirmar(archivo, importacionId),
  );
}

/** Panorama del canal de asociaciones en un período. Se calcula vivo en el servidor. */
export function usePanoramaAsociaciones(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.asociaciones(empresaId, periodo),
    queryFn: () => asociacionesApi.panorama(periodo),
    placeholderData: (prev) => prev,
  });
}

// ───────────── Gestión de cobro (fase 3): la cola y lo que se hizo ─────────────

export function useCola(filtros: FiltrosCola) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.cola(empresaId, filtros),
    queryFn: () => colaApi.listar(filtros),
    placeholderData: (prev) => prev, // filtrar y paginar sin parpadeo
  });
}

export function useCatalogosGestion() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.catalogosGestion(empresaId),
    queryFn: () => colaApi.catalogosGestion(),
    staleTime: 5 * 60 * 1000,
  });
}

export function useGestiones(numero: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.gestiones(empresaId, numero),
    queryFn: () => colaApi.gestiones(numero),
    enabled: numero !== "",
  });
}

/**
 * Registrar una gestión mueve la cola: el contrato sale de «sin gestionar», aparece su
 * última gestión y, si prometió, su promesa. Se invalida el módulo completo por lo mismo
 * que los cobros: invalidar de menos deja la pantalla mintiendo.
 */
export function useRegistrarGestion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (g: NuevaGestion) => colaApi.registrar(g),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

// ─────────── Configuración del módulo (parámetros, tramos, sedes, frontera) ───────────

export function useConfigCxc() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.config(empresaId),
    queryFn: () => configCxcApi.cargar(),
  });
}

/**
 * Todo cambio de configuración invalida el módulo COMPLETO: la probabilidad de un tramo y
 * el factor de una forma de pago son los dos multiplicadores del valor esperado, así que
 * cambiarlos REORDENA la cola de cobro. Dejar la cola cacheada mostraría un orden que ya
 * no corresponde a los parámetros vigentes.
 */
function useMutacionDeConfig<TVars, TData>(fn: (v: TVars) => Promise<TData>) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

export function useGuardarParametrosCxc() {
  return useMutacionDeConfig((valores: Record<string, string>) => configCxcApi.guardarParametros(valores));
}

export function useActualizarTramo() {
  return useMutacionDeConfig(({ codigo, cambio }: { codigo: string; cambio: CambioTramo }) =>
    configCxcApi.actualizarTramo(codigo, cambio),
  );
}

export function useActualizarFormaPago() {
  return useMutacionDeConfig(({ id, factor }: { id: string; factor: string }) =>
    configCxcApi.actualizarFormaPago(id, factor),
  );
}

export function useCrearSede() {
  return useMutacionDeConfig(({ nombre, plaza }: { nombre: string; plaza?: string }) =>
    configCxcApi.crearSede(nombre, plaza),
  );
}

export function useActualizarSede() {
  return useMutacionDeConfig(({ id, cambio }: { id: string; cambio: { nombre?: string; activa?: boolean } }) =>
    configCxcApi.actualizarSede(id, cambio),
  );
}

export function useAsignarSedes() {
  return useMutacionDeConfig(({ usuarioId, sedeIds }: { usuarioId: string; sedeIds: string[] }) =>
    configCxcApi.asignarSedes(usuarioId, sedeIds),
  );
}

// ─────────── Planillas de asociación: conciliación contra el depósito ───────────

export function usePlanilla(asociacionId: string, periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.planilla(empresaId, asociacionId, periodo),
    queryFn: () => planillasApi.ficha(asociacionId, periodo),
    enabled: asociacionId !== "",
  });
}

/** Los candidatos salen de Bancos: se recalculan cuando cambia lo vinculado. */
export function useCandidatosDeposito(planillaId: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.candidatos(empresaId, planillaId),
    queryFn: () => planillasApi.candidatos(planillaId),
    enabled: planillaId !== "",
  });
}

/**
 * Vincular o desvincular cambia el panorama, la ficha y los candidatos. Se invalida el
 * módulo entero: el estado de la planilla lo derivan tres consultas distintas y ninguna
 * puede quedar mostrando el estado anterior.
 */
function useMutacionDePlanilla<TVars>(fn: (v: TVars) => Promise<PlanillaDetalle>) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

export function useAbrirPlanilla() {
  return useMutacionDePlanilla(
    ({ asociacionId, periodo, referencia, nota }: { asociacionId: string; periodo: string; referencia: string; nota?: string }) =>
      planillasApi.abrir(asociacionId, periodo, referencia, nota),
  );
}

export function useVincularDeposito() {
  return useMutacionDePlanilla(({ planillaId, movimientoId }: { planillaId: string; movimientoId: string }) =>
    planillasApi.vincular(planillaId, movimientoId),
  );
}

export function useDesvincularDeposito() {
  return useMutacionDePlanilla(({ planillaId, movimientoId }: { planillaId: string; movimientoId: string }) =>
    planillasApi.desvincular(planillaId, movimientoId),
  );
}

// ─────────── Notas de crédito ───────────

export function useNotasCredito(filtros: { contrato?: string; incluir_anuladas?: boolean }) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.notas(empresaId, filtros),
    queryFn: () => notasApi.listar(filtros),
  });
}

/**
 * Emitir o anular una nota cambia el SALDO de los cargos, así que mueve la cartera, la cola,
 * el valor esperado y el aging. Se invalida el módulo completo: es el mismo criterio que los
 * cobros, porque una nota es exactamente eso salvo que no entró plata.
 */
export function useEmitirNota() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (nota: NuevaNota) => notasApi.emitir(nota),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

export function useAnularNota() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, motivo }: { id: string; motivo: string }) => notasApi.anular(id, motivo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

// ─────────── Suspensión por mora (18 meses o su equivalencia) ───────────

export function useEstadoSuspension(numero: string, activo = true) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.suspension(empresaId, numero),
    queryFn: () => suspensionApi.estado(numero),
    enabled: activo && numero !== "",
  });
}

/**
 * Suspender o reactivar cambia el ESTADO del contrato, y ese estado se ve en la cartera, en la
 * cola y en la ficha. Se invalida el módulo completo, igual que con los cobros y las notas.
 */
export function useSuspender() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ numero, motivo }: { numero: string; motivo: string }) =>
      suspensionApi.suspender(numero, motivo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

export function useReactivar() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ numero, motivo }: { numero: string; motivo: string }) =>
      suspensionApi.reactivar(numero, motivo),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

// ─────────── Arreglos de pago ───────────

export function useArreglos(filtros: {
  contrato?: string;
  estado?: string;
  excepciones?: boolean;
  page?: number;
  page_size?: number;
}) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.arreglos(empresaId, filtros),
    queryFn: () => arreglosApi.listar(filtros),
  });
}

/**
 * Pactar, quebrar o anular un arreglo no cambia ningún saldo —el arreglo no toca los cargos—,
 * pero sí cambia lo que la cola muestra y lo que el gestor debe hacer hoy: un arreglo al día es
 * razón para NO llamar, y uno quebrado manda el contrato a cartera morosa. Por eso se invalida
 * el módulo, no solo la lista de arreglos.
 */
function useMutacionDeArreglo<TVars, TData>(fn: (v: TVars) => Promise<TData>) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.cxc.raiz(empresaId) }),
  });
}

export function usePactarArreglo() {
  return useMutacionDeArreglo((a: NuevoArreglo) => arreglosApi.pactar(a));
}

export function useQuebrarArreglo() {
  return useMutacionDeArreglo(({ id, motivo }: { id: string; motivo: string }) =>
    arreglosApi.quebrar(id, motivo),
  );
}

export function useAnularArreglo() {
  return useMutacionDeArreglo(({ id, motivo }: { id: string; motivo: string }) =>
    arreglosApi.anular(id, motivo),
  );
}

// ─────────── Contacto preventivo ───────────

export function usePreventivo(filtros: {
  sede_id?: string;
  motivo?: string;
  q?: string;
  page?: number;
  page_size?: number;
}) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxc.preventivo(empresaId, filtros),
    queryFn: () => preventivoApi.listar(filtros),
  });
}
