/**
 * Hooks de TanStack Query por recurso del módulo Bancos.
 *
 * Reglas (skill gpvdp-data-layer):
 *  - Estado de servidor SOLO aquí (Query), nunca en Context/estado local.
 *  - Query keys namespaced por empresa activa (queryKeys.bancos.*).
 *  - Invalidación explícita tras mutaciones (movimientos, cuadre, dashboard,
 *    propuestas, tipo de cambio, período — según lo que cambie).
 */

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/api/queryKeys";
import {
  bancosApi,
  type AmbitoCatalogo,
  type CambioDeCuenta,
  type ClasificacionInput,
  type ConceptoInput,
  type NaturalezaConcepto,
  type CotizacionInput,
  type CuentaInput,
  type AgruparResumen,
  type FiltrosMovimientos,
  type MetodoProyeccion,
  type ReglaClasificacionInput,
  type ReglaUpdateInput,
} from "@/api/bancos";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { partesPeriodo } from "@/lib/format";

// ---------------------------------------------------------------------------
// Catálogos (cuentas / conceptos / clasificaciones). Cambian poco -> más stale.
// ---------------------------------------------------------------------------

// `incluirInactivas` solo lo pide el catálogo (para poder reactivar); el importador no.
export function useCuentas(incluirInactivas = false) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.cuentas(empresaId, incluirInactivas),
    queryFn: () => bancosApi.cuentas(incluirInactivas),
    staleTime: 5 * 60_000,
  });
}

export function useConceptos(ambito?: AmbitoCatalogo) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.conceptos(empresaId, ambito),
    queryFn: () => bancosApi.conceptos(ambito),
    staleTime: 5 * 60_000,
  });
}

// --- Administración de bancos y cuentas (catálogo) ---

export function useBancosCatalogo(incluirInactivos = false) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.bancosCatalogo(empresaId, incluirInactivos),
    queryFn: () => bancosApi.bancos(incluirInactivos),
    staleTime: 5 * 60_000,
  });
}

export function useCrearBanco() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (nombre: string) => bancosApi.crearBanco(nombre),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.bancosCatalogo(empresaId) });
    },
  });
}

export function useRenombrarBanco() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; nombre: string }) => bancosApi.renombrarBanco(vars.id, vars.nombre),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.bancosCatalogo(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.cuentas(empresaId) });
    },
  });
}

export function useCrearCuenta() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CuentaInput) => bancosApi.crearCuenta(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.cuentas(empresaId) });
    },
  });
}

export function useClasificaciones(ambito?: AmbitoCatalogo) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.clasificaciones(empresaId, ambito),
    queryFn: () => bancosApi.clasificaciones(ambito),
    staleTime: 5 * 60_000,
  });
}

/** Reglas del motor de clasificación (ordenadas por prioridad desc). */
export function useReglas() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.reglas(empresaId),
    queryFn: () => bancosApi.reglas(),
    staleTime: 60_000,
  });
}

export function useCrearConcepto() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ConceptoInput) => bancosApi.crearConcepto(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conceptosRaiz(empresaId) });
    },
  });
}

/**
 * Declara si el concepto suma a ingresos, a gastos, o no entra al EBITDA.
 *
 * Mueve el EBITDA de TODOS los períodos, así que hay que invalidar el dashboard, la tendencia, el
 * calendario, el cuadre y las proyecciones — no solo el catálogo. Sin eso el usuario declara la
 * naturaleza, vuelve al dashboard y sigue viendo el número viejo. La serie y el calendario cuelgan
 * del prefijo ["bancos","dashboard",empresaId], así que esa clave los alcanza a los tres.
 */
export function useCambiarNaturaleza() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; naturaleza: NaturalezaConcepto }) =>
      bancosApi.cambiarNaturaleza(vars.id, vars.naturaleza),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conceptosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.clasificacionesRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: ["bancos", "dashboard", empresaId] });
      void qc.invalidateQueries({ queryKey: ["bancos", "cuadre", empresaId] });
      void qc.invalidateQueries({ queryKey: ["bancos", "proyeccion", empresaId] });
    },
  });
}

/** Muestra u oculta un concepto (y sus clasificaciones) en el clasificador de CxP. */
export function useCambiarVisibilidadCxP() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; visible: boolean }) =>
      bancosApi.cambiarVisibilidadCxP(vars.id, vars.visible),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conceptosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.clasificacionesRaiz(empresaId) });
    },
  });
}

export function useCrearClasificacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ClasificacionInput) => bancosApi.crearClasificacion(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.clasificacionesRaiz(empresaId) });
    },
  });
}

// --- Administración del catálogo (renombrar / eliminar) ---

function invalidarCatalogo(qc: QueryClient, empresaId: string): void {
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.conceptosRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.clasificacionesRaiz(empresaId) });
  // Los nombres aparecen en movimientos, cuadre y dashboard.
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.movimientosRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: ["bancos", "cuadre", empresaId] });
  void qc.invalidateQueries({ queryKey: ["bancos", "dashboard", empresaId] });
}

export function useRenombrarConcepto() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; nombre: string }) => bancosApi.renombrarConcepto(vars.id, vars.nombre),
    onSuccess: () => invalidarCatalogo(qc, empresaId),
  });
}

export function useEliminarConcepto() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bancosApi.eliminarConcepto(id),
    onSuccess: () => invalidarCatalogo(qc, empresaId),
  });
}

export function useRenombrarClasificacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; nombre: string }) =>
      bancosApi.renombrarClasificacion(vars.id, vars.nombre),
    onSuccess: () => invalidarCatalogo(qc, empresaId),
  });
}

export function useReasignarConceptoClasificacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; conceptoId: string }) =>
      bancosApi.reasignarConceptoClasificacion(vars.id, vars.conceptoId),
    onSuccess: () => invalidarCatalogo(qc, empresaId),
  });
}

export function useEliminarClasificacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bancosApi.eliminarClasificacion(id),
    onSuccess: () => invalidarCatalogo(qc, empresaId),
  });
}

// ---------------------------------------------------------------------------
// Movimientos
// ---------------------------------------------------------------------------

export function useMovimientos(filtros: FiltrosMovimientos) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.movimientos(empresaId, filtros),
    queryFn: () => bancosApi.movimientos(filtros),
    placeholderData: (prev) => prev, // paginación/filtrado sin parpadeo (keepPreviousData v5)
  });
}

/**
 * Resumen (cuántos y cuánto) de la selección activa de la hoja de trabajo.
 * Recibe los MISMOS filtros que useMovimientos: es el encabezado de esa misma lista.
 */
export function useResumenSeleccion(filtros: FiltrosMovimientos, agrupar: AgruparResumen) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.resumenSeleccion(empresaId, agrupar, filtros),
    queryFn: () => bancosApi.resumenSeleccion(filtros, agrupar),
    placeholderData: (prev) => prev, // al cambiar un filtro, no parpadea el encabezado
  });
}

/** Clasifica UN movimiento. Invalida movimientos + cuadre + dashboard. */
export function useClasificarMovimiento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { movimientoId: string; conceptoId: string; clasificacionId: string }) =>
      bancosApi.clasificar(vars.movimientoId, vars.conceptoId, vars.clasificacionId),
    onSuccess: () => invalidarClasificacion(qc, empresaId),
  });
}

// ---------------------------------------------------------------------------
// Reglas (clasificación por bloque)
// ---------------------------------------------------------------------------

export function useCrearRegla() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (regla: ReglaClasificacionInput) => bancosApi.crearRegla(regla),
    onSuccess: () => invalidarClasificacion(qc, empresaId),
  });
}

/**
 * Importa el diccionario del catálogo. Con aplicar=false solo previsualiza (no invalida nada);
 * al aplicar cambia catálogo Y clasificación (las reglas nuevas retro-aplican), así que se
 * refresca todo eso.
 */
export function useImportarDiccionario() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ archivo, aplicar }: { archivo: File; aplicar: boolean }) =>
      bancosApi.importarDiccionario(archivo, aplicar),
    onSuccess: (plan) => {
      if (!plan.aplicado) return;
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.conceptosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.clasificacionesRaiz(empresaId) });
      invalidarClasificacion(qc, empresaId);
    },
  });
}

/**
 * Patrones de los movimientos sin clasificar. Comparte prefijo de caché con el resumen de
 * clasificación, así que crear una regla (que invalida la clasificación) también los refresca.
 */
export function usePatrones(periodo?: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.patrones(empresaId, periodo),
    queryFn: () => bancosApi.patrones(periodo),
    staleTime: 60_000,
  });
}

/** Edita prioridad / activo / palabras clave de una regla del motor. */
export function useActualizarRegla() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; input: ReglaUpdateInput }) =>
      bancosApi.actualizarRegla(vars.id, vars.input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.reglas(empresaId) });
    },
  });
}

export function useEliminarRegla() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bancosApi.eliminarRegla(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.reglas(empresaId) });
    },
  });
}

/** Clasifica un bloque de movimientos seleccionados en un solo request. */
export function useClasificarMasivo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { movimientoIds: string[]; conceptoId: string; clasificacionId: string }) =>
      bancosApi.clasificarMasivo(vars.movimientoIds, vars.conceptoId, vars.clasificacionId),
    onSuccess: () => invalidarClasificacion(qc, empresaId),
  });
}

/** Conteos por estado para el KPI de auto-clasificación (meta ≥90%). */
export function useResumenClasificacion(periodo?: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.resumenClasif(empresaId, periodo),
    queryFn: () => bancosApi.resumenClasificacion(periodo),
  });
}

// ---------------------------------------------------------------------------
// Importaciones
// ---------------------------------------------------------------------------

export function useImportar() {
  return useMutation({
    mutationFn: (vars: { cuentaId: string; archivo: File }) =>
      bancosApi.importar(vars.cuentaId, vars.archivo),
  });
}

export function useConfirmarImportacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { importacionId: string; excluir: string[] }) =>
      bancosApi.confirmar(vars.importacionId, vars.excluir),
    onSuccess: () => {
      // Insertar movimientos afecta listados, cuadre y dashboard de la empresa.
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.movimientosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: ["bancos", "cuadre", empresaId] });
      void qc.invalidateQueries({ queryKey: ["bancos", "dashboard", empresaId] });
    },
  });
}

// ---------------------------------------------------------------------------
// Cuadre / Dashboard
// ---------------------------------------------------------------------------

export function useCuadre(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.cuadre(empresaId, periodo),
    queryFn: () => bancosApi.cuadre(periodo),
  });
}

/** Cuadre jerárquico (Tipo → Concepto → Clasificación) del período. */
export function useCuadreArbol(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.cuadreArbol(empresaId, periodo),
    queryFn: () => bancosApi.cuadreArbol(periodo),
  });
}

export function useDashboard(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.dashboard(empresaId, periodo),
    queryFn: () => bancosApi.dashboard(periodo),
  });
}

// --- Análisis visual (Fase B) ---

/** Tendencia de los últimos 12 meses hasta el período activo (sin huecos). */
export function useSerieMensual(hasta: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.serieMensual(empresaId, hasta),
    queryFn: () => bancosApi.serieMensual(hasta, 12),
  });
}

/**
 * Traer la clasificación hecha en Excel. Una sola mutación para previsualizar y para aplicar: el
 * plan que se ve en pantalla es el mismo que se ejecuta.
 *
 * Al aplicar invalida los movimientos, el cuadre y el tablero de la empresa: la clasificación mueve
 * el EBITDA y dejar la pantalla con el número viejo haría dudar de si el archivo sirvió.
 */
export function useClasificarDesdeExcel() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return useMutation({
    mutationFn: (v: {
      archivo: File;
      cuentaBancariaId?: string;
      reemplazar?: boolean;
      aplicar?: boolean;
    }) => bancosApi.clasificarDesdeExcel(v.archivo, v),
    onSuccess: (plan) => {
      if (!plan.aplicado || plan.clasificados === 0) return;
      qc.invalidateQueries({ queryKey: queryKeys.bancos.movimientosRaiz(empresaId) });
      qc.invalidateQueries({ queryKey: ["bancos", "cuadre", empresaId] });
      qc.invalidateQueries({ queryKey: ["bancos", "dashboard", empresaId] });
      qc.invalidateQueries({ queryKey: queryKeys.bancos.resumenClasifRaiz(empresaId) });
    },
  });
}

/**
 * Análisis de partidas en el tiempo. El rango va explícito en la clave: pedir otro semestre
 * es otra consulta, no la misma con otros datos.
 */
export function useAnalisisPartidas(desde: string, hasta: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.analisisPartidas(empresaId, desde, hasta),
    queryFn: () => bancosApi.analisisPartidas(desde, hasta),
  });
}

/** Totales por día del período (calendario de flujo). */
export function useCalendarioDiario(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.calendario(empresaId, periodo),
    queryFn: () => bancosApi.calendarioDiario(periodo),
  });
}

/** Créditos/débitos del período por cuenta bancaria. */
export function useResumenPorCuenta(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.cuentasResumen(empresaId, periodo),
    queryFn: () => bancosApi.resumenPorCuenta(periodo),
  });
}

// --- Proyecciones (Fase C) ---

/** Escenario de proyección calculado en el backend (no persiste). */
export function useProyeccion(periodo: string, metodo: MetodoProyeccion, metaPct: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.proyeccion(empresaId, periodo, metodo, metaPct),
    queryFn: () => bancosApi.proyeccion(periodo, metodo, metaPct),
    placeholderData: (prev) => prev, // cambiar método/meta sin parpadeo
  });
}

export function useGuardarEscenario() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { periodo: string; metodo: MetodoProyeccion; metaPct: string }) =>
      bancosApi.guardarEscenario(vars.periodo, vars.metodo, vars.metaPct),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.escenariosRaiz(empresaId) });
    },
  });
}

/** Escenarios guardados (con el real del período para medir precisión). */
export function useEscenarios(periodo?: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.escenarios(empresaId, periodo),
    queryFn: () => bancosApi.escenarios(periodo),
  });
}

// ---------------------------------------------------------------------------
// Período (estado + cierre)
// ---------------------------------------------------------------------------

export function usePeriodo(periodo: string) {
  const empresaId = useEmpresaId();
  const { anio, mes } = partesPeriodo(periodo);
  return useQuery({
    queryKey: queryKeys.bancos.periodo(empresaId, anio, mes),
    queryFn: () => bancosApi.periodo(anio, mes),
  });
}

export function useCerrarPeriodo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (periodo: string) => {
      const { anio, mes } = partesPeriodo(periodo);
      return bancosApi.cerrarPeriodo(anio, mes);
    },
    onSuccess: (_data, periodo) => {
      const { anio, mes } = partesPeriodo(periodo);
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.periodo(empresaId, anio, mes) });
      void qc.invalidateQueries({ queryKey: ["bancos", "dashboard", empresaId] });
    },
  });
}

// ---------------------------------------------------------------------------
// Traslados
// ---------------------------------------------------------------------------

export function usePropuestasTraslado(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.propuestas(empresaId, periodo),
    queryFn: () => bancosApi.propuestasTraslado(periodo),
  });
}

export function useEmparejarTraslado() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { debitoId: string; creditoId: string }) =>
      bancosApi.emparejar(vars.debitoId, vars.creditoId),
    onSuccess: () => invalidarTraslados(qc, empresaId),
  });
}

export function useDesemparejarTraslado() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (movimientoId: string) => bancosApi.desemparejar(movimientoId),
    onSuccess: () => invalidarTraslados(qc, empresaId),
  });
}

// ---------------------------------------------------------------------------
// Tipo de cambio
// ---------------------------------------------------------------------------

export function useTipoCambio(periodo: string) {
  const empresaId = useEmpresaId();
  const { anio, mes } = partesPeriodo(periodo);
  return useQuery({
    queryKey: queryKeys.bancos.tipoCambio(empresaId, anio, mes),
    queryFn: () => bancosApi.tipoCambio(anio, mes),
  });
}

export function useRegistrarCotizacion(periodo: string) {
  const empresaId = useEmpresaId();
  const { anio, mes } = partesPeriodo(periodo);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (cotizacion: CotizacionInput) => bancosApi.registrarCotizacion(cotizacion),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.tipoCambio(empresaId, anio, mes) });
    },
  });
}

/** Parámetros de negocio de la empresa (tolerancia de traslado, etc.). */
export function useParametros() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.parametros(empresaId),
    queryFn: () => bancosApi.parametros(),
    staleTime: 5 * 60_000,
  });
}

export function useActualizarTolerancia() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (toleranciaPct: string) => bancosApi.actualizarTolerancia(toleranciaPct),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.parametros(empresaId) });
      // La tolerancia cambia qué pares de traslado se proponen.
      void qc.invalidateQueries({ queryKey: ["bancos", "propuestas", empresaId] });
    },
  });
}

/** Último resultado de sincronización con el BCCR (o sin sincronizar). */
export function useUltimoSyncBCCR() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.bancos.ultimoSync(empresaId),
    queryFn: () => bancosApi.ultimoSyncBCCR(),
  });
}

export function useSincronizarBCCR(periodo: string) {
  const empresaId = useEmpresaId();
  const { anio, mes } = partesPeriodo(periodo);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (fecha?: string) => bancosApi.sincronizarBCCR(fecha),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.ultimoSync(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.tipoCambio(empresaId, anio, mes) });
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.movimientosRaiz(empresaId) });
    },
  });
}

export function useCongelarTipoCambio(periodo: string) {
  const empresaId = useEmpresaId();
  const { anio, mes } = partesPeriodo(periodo);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => bancosApi.congelarTipoCambio(anio, mes),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.tipoCambio(empresaId, anio, mes) });
      // Congelar TC recalcula montos en CRC -> refrescar listados/cuadre/dashboard.
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.movimientosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: ["bancos", "cuadre", empresaId] });
      void qc.invalidateQueries({ queryKey: ["bancos", "dashboard", empresaId] });
    },
  });
}

// ---------------------------------------------------------------------------
// Invalidaciones compartidas
// ---------------------------------------------------------------------------

/** Clasificar (individual, masivo o por regla) afecta listados, cuadre, dashboard y KPI. */
function invalidarClasificacion(qc: QueryClient, empresaId: string): void {
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.movimientosRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: ["bancos", "cuadre", empresaId] });
  void qc.invalidateQueries({ queryKey: ["bancos", "dashboard", empresaId] });
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.resumenClasifRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.reglas(empresaId) }); // aciertos
}

/** Emparejar/desemparejar afecta propuestas, movimientos y dashboard (EBITDA). */
function invalidarTraslados(qc: QueryClient, empresaId: string): void {
  void qc.invalidateQueries({ queryKey: ["bancos", "propuestas", empresaId] });
  void qc.invalidateQueries({ queryKey: queryKeys.bancos.movimientosRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: ["bancos", "dashboard", empresaId] });
}

// ─────────── Corregir lo que se creó mal: bancos, cuentas y fusión del catálogo ───────────

/** Invalida las dos listas del catálogo de cuentas (con y sin inactivas). */
function invalidarCuentas(qc: QueryClient, empresaId: string): void {
  void qc.invalidateQueries({ queryKey: ["bancos", "cuentas", empresaId] });
  void qc.invalidateQueries({ queryKey: ["bancos", "catalogo-bancos", empresaId] });
}

export function useEliminarBanco() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bancosApi.eliminarBanco(id),
    onSuccess: () => invalidarCuentas(qc, empresaId),
  });
}

export function useCambiarActivoBanco() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; activo: boolean }) => bancosApi.cambiarActivoBanco(vars.id, vars.activo),
    onSuccess: () => invalidarCuentas(qc, empresaId),
  });
}

export function useEliminarCuenta() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bancosApi.eliminarCuenta(id),
    onSuccess: () => invalidarCuentas(qc, empresaId),
  });
}

export function useCambiarActivoCuenta() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; activo: boolean }) => bancosApi.cambiarActivoCuenta(vars.id, vars.activo),
    onSuccess: () => invalidarCuentas(qc, empresaId),
  });
}

/** Alias y banco siempre; moneda e IBAN solo si la cuenta no tiene movimientos. */
export function useActualizarCuenta() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; cambio: CambioDeCuenta }) =>
      bancosApi.actualizarCuenta(vars.id, vars.cambio),
    onSuccess: () => invalidarCuentas(qc, empresaId),
  });
}

/** Qué cuelga de la cuenta. Se pide al abrir la fila, para avisar antes de que choque. */
export function useUsoDeCuenta(cuentaId: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: ["bancos", "cuenta-uso", empresaId, cuentaId],
    queryFn: () => bancosApi.usoDeCuenta(cuentaId),
    enabled: cuentaId !== "",
  });
}

/**
 * Fusionar mueve movimientos, reglas, documentos de CxP, vales de caja chica y gastos de
 * proveedor. Toca TRES módulos, así que se invalida la caché completa: una invalidación
 * parcial dejaría alguna pantalla mostrando un concepto que ya no existe.
 */
export function useFusionarConcepto() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; destinoId: string }) =>
      bancosApi.fusionarConcepto(vars.id, vars.destinoId),
    onSuccess: () => void qc.invalidateQueries(),
  });
}

export function useFusionarClasificacion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; destinoId: string; confirmarCambioDeConcepto?: boolean }) =>
      bancosApi.fusionarClasificacion(vars.id, vars.destinoId, vars.confirmarCambioDeConcepto ?? false),
    onSuccess: () => void qc.invalidateQueries(),
  });
}
