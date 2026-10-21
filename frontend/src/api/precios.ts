/**
 * Cliente de la Calculadora de Precios.
 *
 * El costo NO viene como un campo del insumo: el servidor lo deriva del historial a la fecha
 * que se consulta. Por eso `costo_vigente` puede ser null —el insumo existe pero nadie le
 * capturó costo todavía—, y eso es distinto de que valga cero. El 97,4 % del catálogo de costeo
 * actual está en ese estado, así que la distinción es la función, no un detalle.
 */

import { apiFetch } from "@/api/client";

export interface Insumo {
  id: string;
  codigo: string;
  nombre: string;
  unidad: string;
  tipo_costo: "VARIABLE" | "FIJO";
  base_iva: "SIN_IVA" | "CON_IVA";
  proveedor_id: string | null;
  proveedor: string;
  cabys: string | null;
  notas: string;
  activo: boolean;
  /** null = sin costo capturado. NO es cero. */
  costo_vigente: string | null;
  vigente_desde: string | null;
  moneda: string;
  /** Lo que de verdad se pagó la última vez, en subtotal (sin IVA). */
  ultima_compra_cxp: string | null;
  ultima_compra_en: string | null;
}

export interface CapturaCosto {
  costo: string;
  moneda: string;
  vigente_desde: string;
  origen: string;
  nota: string;
  capturado_por: string;
}

export interface PlanPrecio {
  id: string;
  codigo: string;
  nombre: string;
  familia: string;
  modalidad: "DIRECTO" | "SUSCRIPCION";
  precio_venta: string | null;
  cuota_mensual: string | null;
  moneda: string;
  beneficiarios: number | null;
  permanencia_meses: number | null;
  notas: string;
  activo: boolean;
}

export interface LineaReceta {
  id: string;
  insumo_id: string;
  codigo: string;
  nombre: string;
  unidad: string;
  tipo_costo: "VARIABLE" | "FIJO";
  cantidad: string;
  ordinal: number;
  nota: string;
  costo_unitario: string | null;
  extendido: string | null;
}

export interface Costeo {
  plan: PlanPrecio;
  fecha: string;
  lineas: LineaReceta[];
  costo_variable: string;
  costo_fijo: string;
  costo_total: string;
  precio_referencia: string | null;
  /** null cuando la cobertura no es del 100 %: un margen parcial sale MÁS ALTO que el real. */
  margen: string | null;
  margen_pct: string | null;
  /** Se publica aunque falte un costo FIJO; no si falta uno VARIABLE. */
  contribucion_marginal: string | null;
  piso_de_precio: string;
  lineas_total: number;
  lineas_con_costo: number;
  lineas_sin_costo: number;
  cobertura_pct: string;
  insumos_sin_costo: string[];
}

export interface RespuestaInsumos {
  insumos: Insumo[];
  total: number;
  sin_costo: number;
}

export async function listarInsumos(params: {
  fecha?: string;
  busca?: string;
  tipo?: string;
  sinCosto?: boolean;
}): Promise<RespuestaInsumos> {
  const q = new URLSearchParams();
  if (params.fecha) q.set("fecha", params.fecha);
  if (params.busca) q.set("busca", params.busca);
  if (params.tipo) q.set("tipo", params.tipo);
  if (params.sinCosto) q.set("sin_costo", "true");
  const s = q.toString();
  return apiFetch<RespuestaInsumos>(`/precios/insumos${s ? `?${s}` : ""}`);
}

export async function crearInsumo(body: Partial<Insumo>): Promise<{ id: string }> {
  return apiFetch<{ id: string }>("/precios/insumos", { method: "POST", json: body });
}

export async function actualizarInsumo(id: string, body: Partial<Insumo>): Promise<void> {
  await apiFetch(`/precios/insumos/${id}`, { method: "PATCH", json: body });
}

export async function eliminarInsumo(id: string): Promise<void> {
  await apiFetch(`/precios/insumos/${id}`, { method: "DELETE" });
}

export async function capturarCosto(
  insumoID: string,
  body: { costo: string; moneda?: string; vigente_desde: string; origen?: string; nota?: string },
): Promise<void> {
  await apiFetch(`/precios/insumos/${insumoID}/costos`, { method: "POST", json: body });
}

export async function historialDeCosto(insumoID: string): Promise<{ capturas: CapturaCosto[] }> {
  return apiFetch<{ capturas: CapturaCosto[] }>(`/precios/insumos/${insumoID}/costos`);
}

export async function listarPlanes(): Promise<{ planes: PlanPrecio[] }> {
  return apiFetch<{ planes: PlanPrecio[] }>("/precios/planes");
}

export async function crearPlan(body: Partial<PlanPrecio>): Promise<{ id: string }> {
  return apiFetch<{ id: string }>("/precios/planes", { method: "POST", json: body });
}

export async function actualizarPlan(id: string, body: Partial<PlanPrecio>): Promise<void> {
  await apiFetch(`/precios/planes/${id}`, { method: "PATCH", json: body });
}

export async function costear(planID: string, fecha?: string): Promise<Costeo> {
  const q = fecha ? `?fecha=${fecha}` : "";
  return apiFetch<Costeo>(`/precios/planes/${planID}/costeo${q}`);
}

export async function agregarALaReceta(
  planID: string,
  body: { insumo_id: string; cantidad: string; nota?: string },
): Promise<void> {
  await apiFetch(`/precios/planes/${planID}/receta`, { method: "POST", json: body });
}

export async function cambiarCantidad(lineaID: string, cantidad: string): Promise<void> {
  await apiFetch(`/precios/receta/${lineaID}`, { method: "PATCH", json: { cantidad } });
}

export async function quitarDeLaReceta(lineaID: string): Promise<void> {
  await apiFetch(`/precios/receta/${lineaID}`, { method: "DELETE" });
}

// ── Importador del catálogo ─────────────────────────────────────────────────────────────────
//
// Tres llamadas y dos pasos: analizar y planear NO escriben nada, importar sí. La confirmación
// lleva el número de insumos que el plan prometió, para que escribir cientos de filas no dependa
// de que la pantalla y el servidor estén viendo lo mismo.

export interface GrupoDeInsumo {
  nombre_base: string;
  lineas: number;
  sedes: string[];
  con_costo: number;
  costos_distintos: number;
  costo_minimo: string | null;
  costo_maximo: string | null;
}

export interface FilaImportada {
  fila: number;
  nombre_crudo: string;
  nombre_base: string;
  sede: string;
  costo: string | null;
  motivo: string;
}

export interface AnalisisImportacion {
  hoja: string;
  hojas_del_libro: string[];
  columna_nombre: string;
  columna_costo: string;
  lineas_totales: number;
  lineas_con_costo: number;
  lineas_sin_costo: number;
  insumos_si_se_unifica: number;
  insumos_si_no_se_unifica: number;
  sedes_detectadas: string[];
  grupos_con_costo_igual: number;
  grupos_con_costo_distinto: number;
  conflictos: GrupoDeInsumo[];
  muestra: FilaImportada[];
}

export interface InsumoAImportar {
  codigo: string;
  nombre: string;
  sede: string;
  costo: string | null;
}

export interface PlanImportacion {
  total: number;
  con_costo: number;
  sin_costo: number;
  muestra: InsumoAImportar[];
  confirmar_con: number;
}

export interface ResultadoImportacion {
  insumos_creados: number;
  insumos_actualizados: number;
  costos_capturados: number;
  sin_costo: number;
}

function formDatos(archivo: File, extra: Record<string, string>): FormData {
  const fd = new FormData();
  fd.append("archivo", archivo);
  for (const [k, v] of Object.entries(extra)) {
    if (v !== "") fd.append(k, v);
  }
  return fd;
}

export async function analizarImportacion(
  archivo: File,
  opts: { hoja?: string; columnaNombre?: string; columnaCosto?: string } = {},
): Promise<AnalisisImportacion> {
  return apiFetch<AnalisisImportacion>("/precios/importar/analizar", {
    method: "POST",
    raw: formDatos(archivo, {
      hoja: opts.hoja ?? "",
      columna_nombre: opts.columnaNombre ?? "",
      columna_costo: opts.columnaCosto ?? "",
    }),
  });
}

export async function planearImportacion(
  archivo: File,
  opts: {
    hoja?: string;
    columnaNombre?: string;
    columnaCosto?: string;
    unificarSedes: boolean;
    sedes?: string[];
  },
): Promise<PlanImportacion> {
  const fd = formDatos(archivo, {
    hoja: opts.hoja ?? "",
    columna_nombre: opts.columnaNombre ?? "",
    columna_costo: opts.columnaCosto ?? "",
    unificar_sedes: String(opts.unificarSedes),
  });
  for (const s of opts.sedes ?? []) fd.append("sedes", s);
  return apiFetch<PlanImportacion>("/precios/importar/plan", { method: "POST", raw: fd });
}

export async function importarCatalogo(
  archivo: File,
  opts: {
    hoja?: string;
    columnaNombre?: string;
    columnaCosto?: string;
    unificarSedes: boolean;
    vigenteDesde: string;
    confirmacion: number;
    sedes?: string[];
  },
): Promise<ResultadoImportacion> {
  const fd = formDatos(archivo, {
    hoja: opts.hoja ?? "",
    columna_nombre: opts.columnaNombre ?? "",
    columna_costo: opts.columnaCosto ?? "",
    unificar_sedes: String(opts.unificarSedes),
    vigente_desde: opts.vigenteDesde,
    confirmacion: String(opts.confirmacion),
  });
  for (const s of opts.sedes ?? []) fd.append("sedes", s);
  return apiFetch<ResultadoImportacion>("/precios/importar", { method: "POST", raw: fd });
}

// ── Costeo por actividad (ABC) ──────────────────────────────────────────────────────────────
//
// El gasto sale de Bancos, no se teclea. Un reparto cuyos porcentajes no suman 100 % NO se usa:
// el servidor lo informa aparte y su gasto queda sin absorber, en vez de inflar o desinflar el
// costo de todo lo que se vende sin que nada lo delate.

export interface Driver {
  id: string;
  codigo: string;
  nombre: string;
  unidad: string;
  descripcion: string;
  activo: boolean;
  gastos: number;
  planes: number;
  /** null = no se cargó la medición del período; sin ella no hay costo unitario. */
  base_del_periodo: string | null;
}

export interface LineaDeReparto {
  id: string;
  driver_id: string;
  driver: string;
  porcentaje: string;
  monto: string;
}

export interface RepartoDeGasto {
  clasificacion_id: string;
  clasificacion: string;
  concepto: string;
  gasto_del_periodo: string;
  lineas: LineaDeReparto[];
  suma_pct: string;
  valido: boolean;
  motivo: string;
}

export interface BolsaDeDriver {
  driver_id: string;
  driver: string;
  unidad: string;
  gasto: string;
  base: string | null;
  costo_unitario: string | null;
  motivo: string;
}

export interface RepartoDelPeriodo {
  desde: string;
  hasta: string;
  gasto_total: string;
  gasto_repartido: string;
  gasto_sin_asignar: string;
  gasto_en_repartos_invalidos: string;
  bolsas: BolsaDeDriver[];
  problemas: RepartoDeGasto[];
}

export interface ConsumoDeDriver {
  driver_id: string;
  driver: string;
  unidad: string;
  consumo: string;
  costo_unitario: string | null;
  absorbido: string | null;
  motivo: string;
}

export interface AbsorcionDelPlan {
  plan_id: string;
  plan: string;
  lineas: ConsumoDeDriver[];
  total: string;
  completo: boolean;
  drivers_sin_base: string[];
}

export async function listarDrivers(periodo?: string): Promise<{ drivers: Driver[] }> {
  const q = periodo ? `?periodo=${periodo}` : "";
  return apiFetch<{ drivers: Driver[] }>(`/precios/drivers${q}`);
}

export async function crearDriver(body: {
  codigo: string;
  nombre: string;
  unidad: string;
  descripcion?: string;
}): Promise<{ id: string }> {
  return apiFetch<{ id: string }>("/precios/drivers", { method: "POST", json: body });
}

export async function eliminarDriver(id: string): Promise<void> {
  await apiFetch(`/precios/drivers/${id}`, { method: "DELETE" });
}

export async function fijarBase(
  driverID: string,
  body: { periodo: string; cantidad: string; nota?: string },
): Promise<void> {
  await apiFetch(`/precios/drivers/${driverID}/base`, { method: "POST", json: body });
}

export async function listarRepartos(
  desde: string,
  hasta: string,
): Promise<{ repartos: RepartoDeGasto[]; con_problema: number }> {
  return apiFetch<{ repartos: RepartoDeGasto[]; con_problema: number }>(
    `/precios/repartos?desde=${desde}&hasta=${hasta}`,
  );
}

export async function fijarReparto(body: {
  clasificacion_id: string;
  driver_id: string;
  porcentaje: string;
  vigente_desde: string;
}): Promise<void> {
  await apiFetch("/precios/repartos", { method: "POST", json: body });
}

export async function quitarReparto(id: string): Promise<void> {
  await apiFetch(`/precios/repartos/${id}`, { method: "DELETE" });
}

export async function repartir(desde: string, hasta: string): Promise<RepartoDelPeriodo> {
  return apiFetch<RepartoDelPeriodo>(`/precios/repartir?desde=${desde}&hasta=${hasta}`);
}

export async function consumosDelPlan(planID: string): Promise<{ consumos: ConsumoDeDriver[] }> {
  return apiFetch<{ consumos: ConsumoDeDriver[] }>(`/precios/planes/${planID}/drivers`);
}

export async function fijarConsumo(
  planID: string,
  body: { driver_id: string; consumo: string; nota?: string },
): Promise<void> {
  await apiFetch(`/precios/planes/${planID}/drivers`, { method: "POST", json: body });
}

export async function quitarConsumo(planID: string, driverID: string): Promise<void> {
  await apiFetch(`/precios/planes/${planID}/drivers/${driverID}`, { method: "DELETE" });
}

export async function absorcionDelPlan(
  planID: string,
  desde: string,
  hasta: string,
): Promise<AbsorcionDelPlan> {
  return apiFetch<AbsorcionDelPlan>(
    `/precios/planes/${planID}/absorcion?desde=${desde}&hasta=${hasta}`,
  );
}

// ── Políticas de comisión ───────────────────────────────────────────────────────────────────
//
// La comisión NO cuesta lo que recibe el vendedor: en nómina el concepto «Comisiones» afecta
// CCSS, renta y aguinaldo, así que el costo lleva las cargas patronales. El factor lo pone el
// servidor desde los parámetros de nómina; la pantalla no lo teclea ni lo supone.

export interface PoliticaComision {
  id: string;
  plan_id: string | null;
  plan: string;
  vendedor_id: string | null;
  vendedor: string;
  canal: string;
  tipo: "PORCENTAJE" | "MONTO_FIJO";
  base: "PRECIO" | "MARGEN" | "PRIMERA_CUOTA";
  valor: string;
  vigente_desde: string;
  vigente_hasta: string | null;
  notas: string;
  activo: boolean;
  /** Cuanto más alta, más manda al resolver. La calcula el servidor. */
  especificidad: number;
}

export interface Vendedor {
  id: string;
  nombre: string;
  puesto: string;
}

export interface CostoDeComision {
  politica_id: string;
  tipo: string;
  base: string;
  valor: string;
  monto_base: string;
  bruto: string;
  /** null = no se pudo obtener el factor de nómina; el costo total queda incompleto. */
  carga_patronal: string | null;
  factor_carga: string | null;
  costo_total: string | null;
  motivo: string;
}

export interface RespuestaComision {
  comision: CostoDeComision | null;
  motivo?: string;
}

export async function listarPoliticas(): Promise<{ politicas: PoliticaComision[] }> {
  return apiFetch<{ politicas: PoliticaComision[] }>("/precios/comisiones");
}

export async function listarVendedores(): Promise<{ vendedores: Vendedor[] }> {
  return apiFetch<{ vendedores: Vendedor[] }>("/precios/vendedores");
}

export async function crearPolitica(body: {
  plan_id?: string | null;
  vendedor_id?: string | null;
  canal?: string;
  tipo: string;
  base: string;
  valor: string;
  vigente_desde: string;
  vigente_hasta?: string | null;
  notas?: string;
}): Promise<{ id: string }> {
  return apiFetch<{ id: string }>("/precios/comisiones", { method: "POST", json: body });
}

export async function actualizarPolitica(
  id: string,
  body: Partial<PoliticaComision>,
): Promise<void> {
  await apiFetch(`/precios/comisiones/${id}`, { method: "PATCH", json: body });
}

export async function eliminarPolitica(id: string): Promise<void> {
  await apiFetch(`/precios/comisiones/${id}`, { method: "DELETE" });
}

export async function comisionDelPlan(
  planID: string,
  opts: { vendedor?: string; fecha?: string } = {},
): Promise<RespuestaComision> {
  const q = new URLSearchParams();
  if (opts.vendedor) q.set("vendedor", opts.vendedor);
  if (opts.fecha) q.set("fecha", opts.fecha);
  const s = q.toString();
  return apiFetch<RespuestaComision>(`/precios/planes/${planID}/comision${s ? `?${s}` : ""}`);
}

// ── Simulador ───────────────────────────────────────────────────────────────────────────────
//
// El cálculo vive en el SERVIDOR, no acá. Las reglas del costeo —qué se publica y qué no cuando
// falta un costo, cómo se trata la frecuencia en suscripción— ya están allá, y duplicarlas en el
// cliente daría dos verdades que se separan al primer cambio.

export interface EscenarioSimulacion {
  precio_venta?: string | null;
  cuota_mensual?: string | null;
  permanencia_meses?: number | null;
  /** Mueve TODO el costo de la receta: «si los insumos suben 12 %». */
  ajuste_costos_pct?: string | null;
  /** Probabilidad anual de uso. Solo aplica a suscripción. */
  frecuencia_anual_pct?: string | null;
  sin_comision?: boolean;
  sin_indirecto?: boolean;
}

export interface ResultadoSimulacion {
  plan: string;
  modalidad: string;
  ingreso: string;
  ingreso_detalle: string;
  costo_servicio: string;
  costo_esperado: string;
  indirecto: string;
  comision: string;
  costo_total: string;
  margen: string;
  margen_pct: string;
  /** Cuánta siniestralidad aguanta antes de perder. null en planes directos. */
  frecuencia_equilibrio: string | null;
  frecuencia_usada: string | null;
  /** false = algo faltó; el número se muestra pero no se puede leer como completo. */
  confiable: boolean;
  faltantes: string[];
}

export async function simular(
  planID: string,
  body: {
    fecha?: string;
    desde?: string;
    hasta?: string;
    vendedor?: string;
    escenario: EscenarioSimulacion;
  },
): Promise<ResultadoSimulacion> {
  return apiFetch<ResultadoSimulacion>(`/precios/planes/${planID}/simular`, {
    method: "POST",
    json: body,
  });
}
