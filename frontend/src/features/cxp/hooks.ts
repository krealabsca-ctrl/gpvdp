/**
 * Hooks de TanStack Query del módulo CxP.
 *
 * Reglas (skill gpvdp-data-layer):
 *  - Estado de servidor SOLO aquí (Query), namespaced por empresa activa.
 *  - Invalidación explícita tras cada mutación (listados + detalle afectado).
 */

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/api/queryKeys";
import {
  cxpApi,
  type AccionMasiva,
  type DepartamentoInput,
  type DocumentoInput,
  type FiltrosDocumentos,
  type FilaIBAN,
  type FondoInput,
  type ValeInput,
  type FiltrosProveedores,
  type ProveedorInput,
  type TipoFactura,
} from "@/api/cxp";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

// ---------------------------------------------------------------------------
// Proveedores
// ---------------------------------------------------------------------------

export function useProveedores(filtros: FiltrosProveedores, page = 1, pageSize = 50) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.proveedores(empresaId, { ...filtros, page, pageSize }),
    queryFn: () => cxpApi.proveedores(filtros, page, pageSize),
    placeholderData: (prev) => prev, // búsqueda/paginación/filtros sin parpadeo
  });
}

/** Lista COMPLETA de proveedores (para poblar selects sin tope de página). */
export function useTodosProveedores() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.proveedoresTodos(empresaId),
    queryFn: () => cxpApi.todosProveedores(),
    staleTime: 60_000,
  });
}

export function useProveedor(id: string | undefined) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.proveedor(empresaId, id ?? ""),
    queryFn: () => cxpApi.proveedor(id!),
    enabled: !!id,
  });
}

export function useCrearProveedor() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ProveedorInput) => cxpApi.crearProveedor(input),
    onSuccess: () => {
      invalidarProveedores(qc, empresaId);
    },
  });
}

export function useActualizarProveedor() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; input: ProveedorInput }) =>
      cxpApi.actualizarProveedor(vars.id, vars.input),
    onSuccess: (_data, vars) => {
      invalidarProveedores(qc, empresaId);
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.proveedor(empresaId, vars.id) });
    },
  });
}

export function useDesactivarProveedor() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => cxpApi.desactivarProveedor(id),
    onSuccess: () => {
      invalidarProveedores(qc, empresaId);
    },
  });
}

// ---------------------------------------------------------------------------
// Departamentos (catálogo)
// ---------------------------------------------------------------------------

export function useDepartamentos(soloActivos = false) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.departamentos(empresaId, soloActivos),
    queryFn: () => cxpApi.departamentos(soloActivos),
    staleTime: 60_000,
  });
}

function invalidarDepartamentos(qc: QueryClient, empresaId: string) {
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.departamentos(empresaId, true) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.departamentos(empresaId, false) });
}

export function useCrearDepartamento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: DepartamentoInput) => cxpApi.crearDepartamento(input),
    onSuccess: () => invalidarDepartamentos(qc, empresaId),
  });
}

export function useActualizarDepartamento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; input: DepartamentoInput }) =>
      cxpApi.actualizarDepartamento(vars.id, vars.input),
    onSuccess: () => invalidarDepartamentos(qc, empresaId),
  });
}

export function useDesactivarDepartamento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => cxpApi.desactivarDepartamento(id),
    onSuccess: () => invalidarDepartamentos(qc, empresaId),
  });
}

// ---------------------------------------------------------------------------
// Documentos
// ---------------------------------------------------------------------------

export function useDocumentos(filtros: FiltrosDocumentos) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.documentos(empresaId, filtros),
    queryFn: () => cxpApi.documentos(filtros),
    placeholderData: (prev) => prev,
  });
}

export function useDocumento(id: string | undefined) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.documento(empresaId, id ?? ""),
    queryFn: () => cxpApi.documento(id!),
    enabled: !!id,
  });
}

export function useCrearDocumento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: DocumentoInput) => cxpApi.crearDocumento(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.documentosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.dashboardRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.bandeja(empresaId) });
      // Un anticipo/abono recién creado debe aparecer en la billetera («en trámite»).
      void qc.invalidateQueries({ queryKey: ["cxp", "anticipos-empresa", empresaId] });
      void qc.invalidateQueries({ queryKey: ["cxp", "anticipos-disponibles", empresaId] });
    },
  });
}

// ---------------------------------------------------------------------------
// Transiciones de estado (revisar / aprobar / pagar / conciliar). Cada una
// afecta el detalle del documento y todos los listados.
// ---------------------------------------------------------------------------

function useTransicionDocumento(fn: (id: string) => Promise<unknown>) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => fn(id),
    onSuccess: (_data, id) => invalidarDocumento(qc, empresaId, id),
  });
}

export function useRevisarDocumento() {
  return useTransicionDocumento((id) => cxpApi.revisar(id));
}
export function useAprobarDocumento() {
  return useTransicionDocumento((id) => cxpApi.aprobar(id));
}
export function usePagarDocumento() {
  return useTransicionDocumento((id) => cxpApi.pagar(id));
}
export function useConciliarDocumento() {
  return useTransicionDocumento((id) => cxpApi.conciliar(id));
}

export function useProgramarDocumento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; fecha: string }) => cxpApi.programar(vars.id, vars.fecha),
    onSuccess: (_data, vars) => invalidarDocumento(qc, empresaId, vars.id),
  });
}

/**
 * Transición masiva del flujo (revisar/aprobar/programar/pagar/conciliar en lote).
 * Invalida los listados, el tablero, la Bandeja, los lotes y el detalle/historial de cada
 * documento tocado (si no, al abrir uno se ve el estado viejo).
 */
export function useTransicionMasiva() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { accion: AccionMasiva; ids: string[]; fecha?: string; nota?: string }) =>
      cxpApi.transicionMasiva(vars.accion, vars.ids, vars.fecha, vars.nota),
    onSuccess: (_data, vars) => {
      invalidarDocs(qc, empresaId, vars.ids);
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.lotes(empresaId) });
    },
  });
}

// ---------------------------------------------------------------------------
// Validación por departamento (asignar depto, validar, devolver)
// ---------------------------------------------------------------------------

/**
 * Invalida lo que cambia al mover documentos desde la Bandeja. Ojo: "documentos" (listados)
 * NO es prefijo de "documento" (detalle) ni de "historial", así que ambos van explícitos —
 * si no, al abrir el documento se veía el estado anterior y la línea de tiempo sin el
 * evento nuevo, que es literalmente «hago el seguimiento y no cambia».
 */
function invalidarDocs(qc: QueryClient, empresaId: string, ids: string[] = []) {
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.documentosRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.dashboardRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.bandeja(empresaId) });
  for (const id of ids) {
    void qc.invalidateQueries({ queryKey: queryKeys.cxp.documento(empresaId, id) });
    void qc.invalidateQueries({ queryKey: queryKeys.cxp.historial(empresaId, id) });
  }
}

export function useAsignarDepartamentoDoc() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; departamentoId: string }) =>
      cxpApi.asignarDepartamentoDoc(vars.id, vars.departamentoId),
    onSuccess: (_data, vars) => invalidarDocs(qc, empresaId, [vars.id]),
  });
}

export function useValidarDepto() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; respaldo: string; nota?: string }) =>
      cxpApi.validarDepto(vars.id, vars.respaldo, vars.nota),
    onSuccess: (_data, vars) => invalidarDocs(qc, empresaId, [vars.id]),
  });
}

export function useDevolverDoc() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; nota?: string }) => cxpApi.devolverDoc(vars.id, vars.nota),
    onSuccess: (_data, vars) => invalidarDocs(qc, empresaId, [vars.id]),
  });
}

export function useValidarEscalado() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; motivo: string; respaldo?: string }) =>
      cxpApi.validarEscalado(vars.id, vars.motivo, vars.respaldo),
    onSuccess: (_data, vars) => invalidarDocs(qc, empresaId, [vars.id]),
  });
}

// ---------------------------------------------------------------------------
// Cuentas IBAN de proveedores (carga masiva)
// ---------------------------------------------------------------------------

/** Los proveedores a los que todavía no se les puede transferir. */
export function useProveedoresSinIBAN() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.sinIBAN(empresaId),
    queryFn: () => cxpApi.proveedoresSinIBAN(),
    staleTime: 30_000,
  });
}

/** Previsualizar: dice qué va a pasar con cada fila SIN escribir nada. */
export function usePrevisualizarIBAN() {
  return useMutation({ mutationFn: (filas: FilaIBAN[]) => cxpApi.previsualizarIBAN(filas) });
}

/**
 * Cargar las cuentas. Invalida la lista de faltantes Y los proveedores: el IBAN se muestra en
 * la ficha, así que dejarla con el dato viejo haría dudar de si la carga funcionó.
 */
export function useCargarIBAN() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (filas: FilaIBAN[]) => cxpApi.cargarIBAN(filas),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.sinIBAN(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.proveedores(empresaId) });
    },
  });
}

// ---------------------------------------------------------------------------
// Umbrales de la validación por riesgo
// ---------------------------------------------------------------------------

export function useParametrosValidacion() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.parametrosValidacion(empresaId),
    queryFn: () => cxpApi.parametrosValidacion(),
    staleTime: 60_000,
  });
}

/**
 * Guardar un umbral NO recalcula las facturas ya revisadas: la regla se aplica de aquí en
 * adelante. Por eso solo se invalida el propio cuadro de parámetros y no los listados.
 */
export function useGuardarParametroValidacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { clave: string; valor: string }) =>
      cxpApi.guardarParametroValidacion(vars.clave, vars.valor),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: queryKeys.cxp.parametrosValidacion(empresaId) }),
  });
}

// ---------------------------------------------------------------------------
// Facturas «de Contabilidad» (no requieren validación de área)
// ---------------------------------------------------------------------------

export function useMarcasContabilidad() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.marcasContabilidad(empresaId),
    queryFn: () => cxpApi.marcasContabilidad(),
    staleTime: 60_000,
  });
}

/** Marca UNA factura (tres estados: true / false / null = heredar). */
export function useMarcarDocContabilidad() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; valor: boolean | null; motivo?: string }) =>
      cxpApi.marcarDocContabilidad(vars.id, vars.valor, vars.motivo),
    onSuccess: (_data, vars) => invalidarDocs(qc, empresaId, [vars.id]),
  });
}

export function useAprobarContabilidad() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => cxpApi.aprobarContabilidad(id),
    onSuccess: (_data, id) => invalidarDocumento(qc, empresaId, id),
  });
}

/**
 * Marcar un proveedor o un rubro cambia la marca EFECTIVA de todas sus facturas abiertas, así que
 * hay que invalidar los listados y los detalles, no solo el cuadro de marcas. Si solo se
 * invalidara el cuadro, la Bandeja seguiría mostrando el estado viejo.
 */
function invalidarMarcas(qc: QueryClient, empresaId: string): void {
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.marcasContabilidad(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.documentosRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.bandeja(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.proveedoresRaiz(empresaId) });
  // Los detalles abiertos: la clave `documento` no cuelga de `documentos`, así que no la alcanza
  // la invalidación de arriba.
  void qc.invalidateQueries({ queryKey: ["cxp", "documento", empresaId] });
}

export function useMarcarProveedorContabilidad() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; valor: boolean }) =>
      cxpApi.marcarProveedorContabilidad(vars.id, vars.valor),
    onSuccess: () => invalidarMarcas(qc, empresaId),
  });
}

export function useMarcarConceptoContabilidad() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; valor: boolean }) =>
      cxpApi.marcarConceptoContabilidad(vars.id, vars.valor),
    onSuccess: () => invalidarMarcas(qc, empresaId),
  });
}

export function useMarcarClasificacionContabilidad() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; valor: boolean }) =>
      cxpApi.marcarClasificacionContabilidad(vars.id, vars.valor),
    onSuccess: () => invalidarMarcas(qc, empresaId),
  });
}

// ---------------------------------------------------------------------------
// Anticipos (netting)
// ---------------------------------------------------------------------------

export function useAnticiposDisponibles(proveedorId: string | undefined) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: ["cxp", "anticipos-disponibles", empresaId, proveedorId ?? ""],
    queryFn: () => cxpApi.anticiposDisponibles(proveedorId!),
    enabled: !!proveedorId,
  });
}

export function useAnticiposEmpresa() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: ["cxp", "anticipos-empresa", empresaId],
    queryFn: () => cxpApi.anticiposEmpresa(),
    staleTime: 30_000,
  });
}

export function useAplicacionesDocumento(id: string | undefined) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: ["cxp", "aplicaciones", empresaId, id ?? ""],
    queryFn: () => cxpApi.aplicacionesDocumento(id!),
    enabled: !!id,
  });
}

function invalidarAnticipos(qc: QueryClient, empresaId: string, facturaId: string) {
  invalidarDocs(qc, empresaId);
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.documento(empresaId, facturaId) });
  void qc.invalidateQueries({ queryKey: ["cxp", "aplicaciones", empresaId, facturaId] });
  void qc.invalidateQueries({ queryKey: ["cxp", "anticipos-disponibles", empresaId] });
  void qc.invalidateQueries({ queryKey: ["cxp", "anticipos-empresa", empresaId] });
}

export function useAplicarAnticipo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; anticipoId: string; monto: string }) =>
      cxpApi.aplicarAnticipo(vars.id, vars.anticipoId, vars.monto),
    onSuccess: (_d, vars) => invalidarAnticipos(qc, empresaId, vars.id),
  });
}

export function useAplicarAnticiposLote() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; aplicaciones: { anticipo_id: string; monto: string }[] }) =>
      cxpApi.aplicarAnticiposLote(vars.id, vars.aplicaciones),
    onSuccess: (_d, vars) => invalidarAnticipos(qc, empresaId, vars.id),
  });
}

export function useReversarAnticipo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; aplicacionId: string }) => cxpApi.reversarAnticipo(vars.id, vars.aplicacionId),
    onSuccess: (_d, vars) => invalidarAnticipos(qc, empresaId, vars.id),
  });
}

// ---------------------------------------------------------------------------
// Caja chica (fondo fijo)
// ---------------------------------------------------------------------------

function invalidarCajas(qc: QueryClient, empresaId: string) {
  void qc.invalidateQueries({ queryKey: ["cxp", "cajas", empresaId] });
  void qc.invalidateQueries({ queryKey: ["cxp", "vales", empresaId] });
}

export function useFondosCaja() {
  const empresaId = useEmpresaId();
  return useQuery({ queryKey: ["cxp", "cajas", empresaId], queryFn: () => cxpApi.fondosCaja() });
}

export function useValesCaja(fondoId: string | undefined) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: ["cxp", "vales", empresaId, fondoId ?? ""],
    queryFn: () => cxpApi.valesCaja(fondoId!),
    enabled: !!fondoId,
  });
}

export function useCrearFondo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: FondoInput) => cxpApi.crearFondo(input),
    onSuccess: () => invalidarCajas(qc, empresaId),
  });
}

export function useActualizarFondo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; input: FondoInput }) => cxpApi.actualizarFondo(vars.id, vars.input),
    onSuccess: () => invalidarCajas(qc, empresaId),
  });
}

export function useDesactivarFondo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => cxpApi.desactivarFondo(id),
    onSuccess: () => invalidarCajas(qc, empresaId),
  });
}

export function useCrearVale() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { fondoId: string; input: ValeInput }) => cxpApi.crearVale(vars.fondoId, vars.input),
    onSuccess: () => invalidarCajas(qc, empresaId),
  });
}

export function useAnularVale() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { fondoId: string; valeId: string }) => cxpApi.anularVale(vars.fondoId, vars.valeId),
    onSuccess: () => invalidarCajas(qc, empresaId),
  });
}

export function useGenerarReposicion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (fondoId: string) => cxpApi.generarReposicion(fondoId),
    onSuccess: () => {
      invalidarCajas(qc, empresaId);
      invalidarDocs(qc, empresaId); // la reposición entra a la Bandeja como documento nuevo
    },
  });
}

// ---------------------------------------------------------------------------
// Validadores de departamento
// ---------------------------------------------------------------------------

export function useValidadores(departamentoId: string | undefined) {
  return useQuery({
    queryKey: ["cxp", "validadores", departamentoId ?? ""],
    queryFn: () => cxpApi.validadores(departamentoId!),
    enabled: !!departamentoId,
  });
}

export function useUsuariosEmpresa(enabled = true) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: ["cxp", "usuarios", empresaId],
    queryFn: () => cxpApi.usuariosEmpresa(),
    enabled,
    staleTime: 60_000,
  });
}

export function useAsignarValidador() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { departamentoId: string; usuarioId: string; rol: "TITULAR" | "SUPLENTE" }) =>
      cxpApi.asignarValidador(vars.departamentoId, vars.usuarioId, vars.rol),
    onSuccess: (_d, vars) =>
      void qc.invalidateQueries({ queryKey: ["cxp", "validadores", vars.departamentoId] }),
  });
}

export function useQuitarValidador() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { departamentoId: string; usuarioId: string }) =>
      cxpApi.quitarValidador(vars.departamentoId, vars.usuarioId),
    onSuccess: (_d, vars) =>
      void qc.invalidateQueries({ queryKey: ["cxp", "validadores", vars.departamentoId] }),
  });
}

// ---------------------------------------------------------------------------
// Lotes de pago (corte)
// ---------------------------------------------------------------------------

export function useLotes() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.lotes(empresaId),
    queryFn: () => cxpApi.lotes(),
    staleTime: 15_000,
  });
}

export function useCrearLote() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { fechaCorte: string; ids: string[] }) => cxpApi.crearLote(vars.fechaCorte, vars.ids),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.lotes(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.documentosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.dashboardRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.bandeja(empresaId) });
    },
  });
}

// ---------------------------------------------------------------------------
// Comprobante de pago
// ---------------------------------------------------------------------------

export function useAdjuntarComprobante() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; archivo: File }) => cxpApi.adjuntarComprobante(vars.id, vars.archivo),
    onSuccess: (_d, vars) => invalidarDocumento(qc, empresaId, vars.id),
  });
}

export function useEnviarComprobante() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => cxpApi.enviarComprobante(id),
    onSuccess: (_d, id) => invalidarDocumento(qc, empresaId, id),
  });
}

// ---------------------------------------------------------------------------
// Importador de facturación (Excel)
// ---------------------------------------------------------------------------

/** Sube el Excel y trae el preview (sin crear nada). */
export function usePrevisualizarImportacion() {
  return useMutation({
    mutationFn: (archivo: File) => cxpApi.previsualizarImportacion(archivo),
  });
}

/**
 * Confirma la importación; invalida documentos, proveedores (pudo crear ambos), la Bandeja
 * y el tablero. El tablero va EXPLÍCITO: antes llegaba de rebote por invalidarProveedores,
 * así que un cambio ahí lo habría dejado sin refrescar sin que nada lo delatara.
 */
export function useConfirmarImportacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (archivo: File) => cxpApi.confirmarImportacion(archivo),
    onSuccess: () => {
      invalidarDocs(qc, empresaId);
      invalidarProveedores(qc, empresaId);
    },
  });
}

// ---------------------------------------------------------------------------
// Conciliación por huella (Bancos↔CxP)
// ---------------------------------------------------------------------------

export function useConciliarMatch() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { descripcion: string; monto?: string; fecha?: string }) =>
      cxpApi.conciliarMatch(vars.descripcion, vars.monto, vars.fecha),
    onSuccess: (res) => {
      if (!res.conciliado) return;
      // Último paso del seguimiento (pagado → conciliado): antes era la única transición
      // que NO refrescaba el tablero ni la Bandeja, y quedaban con datos viejos.
      if (res.documento) {
        invalidarDocumento(qc, empresaId, res.documento.id);
        return;
      }
      invalidarDocs(qc, empresaId);
    },
  });
}

// ---------------------------------------------------------------------------
// Clasificación de gasto (individual y en lote)
// ---------------------------------------------------------------------------

export function useClasificarDocumento() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; conceptoId: string; clasificacionId: string; subclasificacionId?: string }) =>
      cxpApi.clasificar(vars.id, vars.conceptoId, vars.clasificacionId, vars.subclasificacionId ?? ""),
    onSuccess: (_data, vars) => invalidarDocumento(qc, empresaId, vars.id),
  });
}

export function useClasificarMasivo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: {
      ids: string[];
      conceptoId: string;
      clasificacionId: string;
      subclasificacionId?: string;
    }) => cxpApi.clasificarMasivo(vars.ids, vars.conceptoId, vars.clasificacionId, vars.subclasificacionId ?? ""),
    onSuccess: (_data, vars) => invalidarDocs(qc, empresaId, vars.ids),
  });
}

/** Subclasificaciones (3er nivel) de una clasificación. */
export function useSubclasificaciones(clasificacionId: string | undefined) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.subclasificaciones(empresaId, clasificacionId ?? ""),
    queryFn: () => cxpApi.subclasificaciones(clasificacionId),
    enabled: !!clasificacionId,
    staleTime: 60_000,
  });
}

export function useCrearSubclasificacion() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { clasificacionId: string; nombre: string }) =>
      cxpApi.crearSubclasificacion(vars.clasificacionId, vars.nombre),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.subclasificacionesRaiz(empresaId) });
    },
  });
}

export function usePrioridadMasiva() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { ids: string[]; prioridad: "" | "A" | "AA" }) =>
      cxpApi.prioridadMasiva(vars.ids, vars.prioridad),
    // La prioridad AA sale en el tablero (compromiso ineludible del corte): refrescarlo.
    onSuccess: (_data, vars) => invalidarDocs(qc, empresaId, vars.ids),
  });
}

/** Categorías frecuentes de un proveedor (para el clasificador). */
export function useGastosProveedor(proveedorId: string | undefined) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: ["cxp", "gastos-proveedor", empresaId, proveedorId ?? ""] as const,
    queryFn: () => cxpApi.gastosProveedor(proveedorId!),
    enabled: !!proveedorId,
    staleTime: 30_000,
  });
}

export function useTipoMasivo() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { ids: string[]; tipo: TipoFactura }) => cxpApi.tipoMasivo(vars.ids, vars.tipo),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.documentosRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.dashboardRaiz(empresaId) });
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.bandeja(empresaId) });
    },
  });
}

// ---------------------------------------------------------------------------
// Trazabilidad y dashboard ejecutivo
// ---------------------------------------------------------------------------

export function useHistorialDocumento(id: string | undefined) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.historial(empresaId, id ?? ""),
    queryFn: () => cxpApi.historial(id!),
    enabled: !!id,
  });
}

/**
 * Tablero de CxP del período indicado (el del selector global de la barra). El período va
 * en la clave: cambiarlo dispara el refetch, que antes no ocurría nunca.
 *
 * Frescura: se marca viejo a los 15 s y se refresca al volver a la pestaña, porque el
 * proceso lo mueven varias personas a la vez y el tablero decide pagos.
 */
export function useDashboardCxp(periodo: string) {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.dashboard(empresaId, periodo),
    queryFn: () => cxpApi.dashboard(periodo),
    staleTime: 15_000,
    refetchOnWindowFocus: true,
    placeholderData: (prev) => prev,
  });
}

/** Resumen por fase de la Bandeja (pestañas con conteo + monto). */
export function useBandeja() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.bandeja(empresaId),
    queryFn: () => cxpApi.bandeja(),
    staleTime: 10_000,
  });
}

/** Todas las subclasificaciones de la empresa (para armar rutas completas del gasto). */
export function useSubclasificacionesTodas() {
  const empresaId = useEmpresaId();
  return useQuery({
    queryKey: queryKeys.cxp.subclasificaciones(empresaId, "todas"),
    queryFn: () => cxpApi.subclasificaciones(),
    staleTime: 60_000,
  });
}

function invalidarProveedores(qc: QueryClient, empresaId: string): void {
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.proveedoresRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.proveedoresTodos(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.dashboardRaiz(empresaId) });
}

function invalidarDocumento(qc: QueryClient, empresaId: string, id: string): void {
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.documentosRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.documento(empresaId, id) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.historial(empresaId, id) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.dashboardRaiz(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.bandeja(empresaId) });
  void qc.invalidateQueries({ queryKey: queryKeys.cxp.lotes(empresaId) });
}
