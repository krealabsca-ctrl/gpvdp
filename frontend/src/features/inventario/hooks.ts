/**
 * Datos del módulo de Inventario (TanStack Query).
 *
 * ── La regla de invalidación de este módulo ──────────────────────────────────
 *
 * Toda operación que mueva inventario invalida `existencias`, `unidades`, `movimientos`,
 * `reposicion` y `rotacion` de una sola vez, con `invalidarMovimiento()`. No es exceso: una entrada
 * cambia lo que hay, lo que se puede pedir y la rotación del artículo, y dejar cualquiera de esas
 * pantallas con el número viejo hace pensar que la operación no se guardó.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { candidatasDeConciliacion, consignacionApi, conteoApi, inventarioApi } from "@/api/inventario";
import type {
  AjusteNuevo,
  ArticuloNuevo,
  EntradaNueva,
  ServicioNuevo,
  TrasladoNuevo,
} from "@/api/inventario";
import { queryKeys } from "@/api/queryKeys";
import { useAuth } from "@/features/auth/AuthContext";

/** La empresa activa. Va en cada clave: sin ella, cambiar de empresa mostraría datos de la otra. */
function useEmpresaId(): string {
  const { empresaActiva } = useAuth();
  return empresaActiva?.id ?? "sin-empresa";
}

/** Invalida todo lo que un movimiento de inventario puede haber cambiado. */
function useInvalidarMovimiento() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return () => {
    const k = queryKeys.inventario;
    for (const raiz of [
      k.existenciasRaiz(empresaId),
      k.unidadesRaiz(empresaId),
      k.movimientosRaiz(empresaId),
      k.reposicionRaiz(empresaId),
      k.rotacionRaiz(empresaId),
      k.serviciosRaiz(empresaId),
      k.trasladosRaiz(empresaId),
      // La consignación también: registrar un servicio con un cofre del proveedor mete esa unidad
      // en la cola de «lo que hay que facturarle». Sin invalidarla, la cola diría que no hay nada.
      k.consignacionRaiz(empresaId),
    ]) {
      void qc.invalidateQueries({ queryKey: raiz });
    }
  };
}

// ── Catálogo ────────────────────────────────────────────────────────────────

export function useCategorias(incluirInactivas = false) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.categorias(empresaId, incluirInactivas),
    queryFn: () => inventarioApi.categorias(incluirInactivas),
    staleTime: 5 * 60_000,
  });
}

export function useCrearCategoria() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return useMutation({
    mutationFn: (v: { nombre: string; padre_id?: string }) =>
      inventarioApi.crearCategoria(v.nombre, v.padre_id ?? ""),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.categoriasRaiz(empresaId) });
    },
  });
}

export function useActualizarCategoria() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return useMutation({
    mutationFn: (v: { id: string; nombre: string; activo: boolean }) =>
      inventarioApi.actualizarCategoria(v.id, v.nombre, v.activo),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.categoriasRaiz(empresaId) });
      // El nombre de la categoría se muestra en las existencias y en la rotación.
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.existenciasRaiz(empresaId) });
    },
  });
}

export function useArticulos(f: {
  categoria_id?: string;
  modo_control?: string;
  q?: string;
  incluir_inactivos?: boolean;
} = {}) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.articulos(empresaId, JSON.stringify(f)),
    queryFn: () => inventarioApi.articulos(f),
  });
}

export function useCrearArticulo() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return useMutation({
    mutationFn: (a: ArticuloNuevo) => inventarioApi.crearArticulo(a),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.articulosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.categoriasRaiz(empresaId) });
    },
  });
}

export function useActualizarArticulo() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return useMutation({
    mutationFn: (v: { id: string; articulo: ArticuloNuevo }) =>
      inventarioApi.actualizarArticulo(v.id, v.articulo),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.articulosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.existenciasRaiz(empresaId) });
    },
  });
}

export function useNiveles(articuloId: string, habilitado = true) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.niveles(empresaId, articuloId),
    queryFn: () => inventarioApi.niveles(articuloId),
    enabled: habilitado && articuloId !== "",
  });
}

export function useFijarNivel() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return useMutation({
    mutationFn: (v: { articuloId: string; sedeId: string; minimo: number; maximo: number }) =>
      inventarioApi.fijarNivel(v.articuloId, v.sedeId, v.minimo, v.maximo),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.nivelesRaiz(empresaId) });
      // El mínimo es lo que decide el semáforo y la reposición: las dos cambian.
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.existenciasRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.inventario.reposicionRaiz(empresaId) });
    },
  });
}

// ── Existencias ─────────────────────────────────────────────────────────────

export function useExistencias(f: {
  sede_id?: string;
  categoria_id?: string;
  modo_control?: string;
  q?: string;
  solo_bajo_minimo?: boolean;
  solo_consignadas?: boolean;
} = {}) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.existencias(empresaId, JSON.stringify(f)),
    queryFn: () => inventarioApi.existencias(f),
  });
}

export function useUnidades(
  f: {
    articulo_id?: string;
    sede_id?: string;
    estado?: string;
    q?: string;
    quietas_desde_dias?: number;
    limite?: number;
    consignada?: boolean;
  } = {},
  habilitado = true,
) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.unidades(empresaId, JSON.stringify(f)),
    queryFn: () => inventarioApi.unidades(f),
    enabled: habilitado,
  });
}

export function useMovimientos(
  f: {
    articulo_id?: string;
    unidad_id?: string;
    sede_id?: string;
    tipo?: string;
    desde?: string;
    hasta?: string;
    limite?: number;
  } = {},
) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.movimientos(empresaId, JSON.stringify(f)),
    queryFn: () => inventarioApi.movimientos(f),
  });
}

// ── Operaciones ─────────────────────────────────────────────────────────────

export function useRegistrarEntrada() {
  const invalidar = useInvalidarMovimiento();
  return useMutation({
    mutationFn: (e: EntradaNueva) => inventarioApi.registrarEntrada(e),
    onSuccess: invalidar,
  });
}

export function useServicios(desde = "", hasta = "") {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.servicios(empresaId, desde, hasta),
    queryFn: () => inventarioApi.servicios(desde, hasta),
  });
}

/** El servicio prestado: es lo que descarga el inventario. */
export function useRegistrarServicio() {
  const invalidar = useInvalidarMovimiento();
  return useMutation({
    mutationFn: (s: ServicioNuevo) => inventarioApi.registrarServicio(s),
    onSuccess: invalidar,
  });
}

export function useRegistrarAjuste() {
  const invalidar = useInvalidarMovimiento();
  return useMutation({
    mutationFn: (a: AjusteNuevo) => inventarioApi.registrarAjuste(a),
    onSuccess: invalidar,
  });
}

// ── Traslados ───────────────────────────────────────────────────────────────

export function useTraslados(estado = "") {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.traslados(empresaId, estado),
    queryFn: () => inventarioApi.traslados(estado),
  });
}

export function useCrearTraslado() {
  const invalidar = useInvalidarMovimiento();
  return useMutation({
    mutationFn: (t: TrasladoNuevo) => inventarioApi.crearTraslado(t),
    onSuccess: invalidar,
  });
}

export function useRecibirTraslado() {
  const invalidar = useInvalidarMovimiento();
  return useMutation({
    mutationFn: (v: { id: string; fecha?: string }) =>
      inventarioApi.recibirTraslado(v.id, v.fecha ?? ""),
    onSuccess: invalidar,
  });
}

// ── Análisis ────────────────────────────────────────────────────────────────

export function useReposicion(sedeId = "", semanas = 8) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.reposicion(empresaId, sedeId, semanas),
    queryFn: () => inventarioApi.reposicion(sedeId, semanas),
  });
}

export function useRotacion(desde = "", hasta = "") {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.rotacion(empresaId, desde, hasta),
    queryFn: () => inventarioApi.rotacion(desde, hasta),
  });
}

// ── Conteo cíclico (Fase 2) ─────────────────────────────────────────────────

export function usePlanConteo() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.planConteo(empresaId),
    queryFn: () => conteoApi.plan(),
  });
}

export function useConteos(estado = "", sedeId = "") {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.conteos(empresaId, estado, sedeId),
    queryFn: () => conteoApi.lista(estado, sedeId),
  });
}

export function useHojaConteo(id: string, habilitado = true) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.conteo(empresaId, id),
    queryFn: () => conteoApi.hoja(id),
    enabled: habilitado && id !== "",
  });
}

/** Invalida las hojas y el plan. No toca existencias: abrir o contar todavía no mueve nada. */
function useInvalidarConteo() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return () => {
    void qc.invalidateQueries({ queryKey: queryKeys.inventario.conteosRaiz(empresaId) });
    void qc.invalidateQueries({ queryKey: queryKeys.inventario.conteoRaiz(empresaId) });
    void qc.invalidateQueries({ queryKey: queryKeys.inventario.planConteo(empresaId) });
  };
}

export function useAbrirConteo() {
  const invalidar = useInvalidarConteo();
  return useMutation({
    mutationFn: (v: { sede_id: string; categoria_id?: string; fecha?: string; nota?: string }) =>
      conteoApi.abrir(v),
    onSuccess: invalidar,
  });
}

export function useGuardarLineaConteo() {
  const invalidar = useInvalidarConteo();
  return useMutation({
    mutationFn: (v: { conteoId: string; lineaId: string; cantidad: number; motivo?: string }) =>
      conteoApi.guardarLinea(v.conteoId, v.lineaId, v.cantidad, v.motivo ?? ""),
    onSuccess: invalidar,
  });
}

/**
 * Cerrar la hoja SÍ mueve el inventario: genera los ajustes de las diferencias explicadas. Por eso
 * invalida también las existencias y todo lo que depende de ellas.
 */
export function useCerrarConteo() {
  const invalidarConteo = useInvalidarConteo();
  const invalidarMovimiento = useInvalidarMovimiento();
  return useMutation({
    mutationFn: (v: { id: string; fecha?: string }) => conteoApi.cerrar(v.id, v.fecha ?? ""),
    onSuccess: () => {
      invalidarConteo();
      invalidarMovimiento();
    },
  });
}

export function useAnularConteo() {
  const invalidar = useInvalidarConteo();
  return useMutation({
    mutationFn: (v: { id: string; motivo: string }) => conteoApi.anular(v.id, v.motivo),
    onSuccess: invalidar,
  });
}

// ── Consignación (Fase 3) ───────────────────────────────────────────────────

export function useConsignacion(f: { proveedor_id?: string; estado?: string; situacion?: string } = {}) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.consignacion(empresaId, f),
    queryFn: () => consignacionApi.cola(f),
  });
}

/**
 * Facturar y conciliar CREAN o ANULAN documentos en CxP, así que invalidan las raíces de CxP además
 * de las de inventario. Sin eso, la bandeja de CxP seguiría mostrando la cartera vieja: una
 * invalidación que no invalida no falla, miente.
 */
function useInvalidarConsignacion() {
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  return () => {
    void qc.invalidateQueries({ queryKey: queryKeys.inventario.consignacionRaiz(empresaId) });
    void qc.invalidateQueries({ queryKey: queryKeys.inventario.unidadesRaiz(empresaId) });
    void qc.invalidateQueries({ queryKey: ["cxp"] });
  };
}

export function useFacturarConsignada() {
  const invalidar = useInvalidarConsignacion();
  return useMutation({
    mutationFn: (v: { unidadId: string; confirmar?: boolean }) =>
      consignacionApi.facturar(v.unidadId, v.confirmar ?? false),
    onSuccess: invalidar,
  });
}

export function useConciliarConsignada() {
  const invalidar = useInvalidarConsignacion();
  return useMutation({
    mutationFn: (v: { unidadId: string; documentoId: string }) =>
      consignacionApi.conciliar(v.unidadId, v.documentoId),
    onSuccess: invalidar,
  });
}

/**
 * Las facturas que sirven para conciliar una unidad. Se pide SOLO cuando el diálogo está abierto
 * (`habilitado`): traerlas por cada fila de la tabla serían tantas consultas como unidades.
 */
export function useCandidatasConciliacion(unidadId: string, habilitado: boolean) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.inventario.candidatasConciliacion(empresaId, unidadId),
    queryFn: () => candidatasDeConciliacion(unidadId),
    enabled: habilitado && unidadId !== "",
  });
}
