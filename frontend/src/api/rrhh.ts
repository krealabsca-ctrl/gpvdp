/**
 * Cliente tipado del módulo RRHH / Nómina (Fase 3 — Etapa 1: fundamentos).
 *
 * Montos y porcentajes viajan como STRING decimal (nunca float); el formateo
 * vive en lib/format.ts. Guardarraíl CCSS: los conceptos `de_sistema` están
 * bloqueados por ley (el backend rechaza editarlos).
 */

import { apiFetch } from "@/api/client";

// ---------------------------------------------------------------------------
// Tipos
// ---------------------------------------------------------------------------

export type Jornada = "MENSUAL" | "QUINCENAL" | "SEMANAL" | "HORAS";
export type TipoConcepto = "INGRESO" | "DEDUCCION";

export interface Empleado {
  id: string;
  nombre: string;
  tipo_identificacion: "CEDULA" | "DIMEX" | "PASAPORTE";
  identificacion: string;
  email: string;
  telefono: string;
  iban: string;
  departamento_id: string;
  departamento_nombre: string;
  puesto: string;
  fecha_ingreso: string;
  fecha_salida: string;
  salario_base: string;
  jornada: Jornada;
  /** Créditos fiscales por familia (Renta 45333-H). */
  hijos: number;
  conyuge: boolean;
  activo: boolean;
  deducciones_activas: number;
}

export interface EmpleadoInput {
  nombre: string;
  tipo_identificacion?: string;
  identificacion: string;
  email?: string;
  telefono?: string;
  iban?: string;
  departamento_id?: string;
  puesto?: string;
  fecha_ingreso: string;
  salario_base: string;
  jornada?: string;
  hijos?: number;
  conyuge?: boolean;
}

export interface Carga {
  codigo: string;
  nombre: string;
  tipo: "OBRERO" | "PATRONAL";
  pct: string;
}

export interface TramoRenta {
  hasta: string | null;
  pct: string;
}

export interface RentaConfig {
  tramos: TramoRenta[];
  credito_hijo: string;
  credito_conyuge: string;
}

export interface ParametrosNomina {
  id: string;
  anio: number;
  cargas: Carga[];
  renta: RentaConfig;
  ins_riesgos_pct: string;
  aplica_ina: boolean;
  adelanto_pct: string;
  adelanto_base: "SALARIO_BASE" | "BRUTO";
  redondeo: "COLON" | "CENTIMO";
  provision_base: "REMUNERACION_TOTAL" | "SALARIO_BASE";
  /** Provisiones informativas de la corrida (maqueta: 8.33 / 4.16 / 1.50). */
  aguinaldo_pct: string;
  vacaciones_pct: string;
  cesantia_pct: string;
  /** EMPRESA = guardados; DEFAULT = legales de referencia CR 2026, aún sin guardar. */
  origen: "EMPRESA" | "DEFAULT";
}

export interface ParametrosInput {
  cargas: Carga[];
  renta: RentaConfig;
  ins_riesgos_pct: string;
  aplica_ina: boolean;
  adelanto_pct: string;
  adelanto_base: string;
  redondeo: string;
  provision_base: string;
  aguinaldo_pct: string;
  vacaciones_pct: string;
  cesantia_pct: string;
}

export interface ConceptoNomina {
  id: string;
  nombre: string;
  tipo: TipoConcepto;
  afecta_ccss: boolean;
  afecta_renta: boolean;
  afecta_aguinaldo: boolean;
  base_legal: string;
  /** Bloqueado por ley (salario, extras, comisiones, bonos habituales…). */
  de_sistema: boolean;
  /** Se captura en HORAS y el motor deriva el monto (horas × valor hora × factor, art. 139 CT). */
  por_horas: boolean;
  activo: boolean;
}

export interface ConceptoInput {
  nombre: string;
  tipo: TipoConcepto;
  afecta_ccss: boolean;
  afecta_renta: boolean;
  afecta_aguinaldo: boolean;
  base_legal?: string;
}

/** Frecuencia de cobro de una deducción a lo largo del mes. */
export type FrecuenciaDeduccion = "AMBAS" | "PRIMERA" | "SEGUNDA" | "MENSUAL";

export const ETIQUETA_FRECUENCIA: Record<FrecuenciaDeduccion, string> = {
  AMBAS: "Cada quincena",
  PRIMERA: "Solo 1ª quincena",
  SEGUNDA: "Solo 2ª quincena",
  MENSUAL: "Una vez al mes",
};

export interface DeduccionEmpleado {
  id: string;
  empleado_id: string;
  concepto_id: string;
  concepto_nombre: string;
  etiqueta: string;
  cuota: string;
  saldo_total: string;
  saldo_restante: string;
  prioridad: number;
  frecuencia: FrecuenciaDeduccion;
  activo: boolean;
}

export interface DeduccionInput {
  concepto_id: string;
  etiqueta: string;
  cuota: string;
  saldo_total?: string;
  prioridad?: number;
  frecuencia?: FrecuenciaDeduccion;
}

export interface FiltrosEmpleados {
  q?: string;
  estado?: "activo" | "inactivo" | "";
}

// ---- Corrida quincenal (Etapa 2) ----

export type TipoCorrida = "ADELANTO" | "LIQUIDACION";
export type EstadoCorrida = "BORRADOR" | "APROBADA" | "PAGADA" | "ANULADA";

export interface Corrida {
  id: string;
  anio: number;
  mes: number;
  tipo: TipoCorrida;
  estado: EstadoCorrida;
  fecha_pago: string;
  empleados: number;
  total_bruto: string;
  total_ccss_obrero: string;
  total_renta: string;
  total_deducciones: string;
  total_adelanto: string;
  total_neto: string;
  total_patronal: string;
  total_provisiones: string;
  creado_en: string;
  aprobado_en: string;
  pagado_en: string;
}

export interface DetalleLinea {
  tipo: "INGRESO" | "CCSS" | "RENTA" | "DEDUCCION" | "ADELANTO" | "PATRONAL" | "PROVISION";
  nombre: string;
  monto: string;
  deduccion_id?: string;
}

/** Tratamiento aplicado a la colilla según la jornada del empleado. */
export type TratamientoLinea = "QUINCENA_1" | "QUINCENA_2" | "ADELANTO" | "MENSUAL";

export const ETIQUETA_TRATAMIENTO: Record<TratamientoLinea, string> = {
  QUINCENA_1: "Pago quincenal",
  QUINCENA_2: "Pago quincenal",
  ADELANTO: "Adelanto",
  MENSUAL: "Liquidación mensual",
};

export interface LineaCorrida {
  id: string;
  tratamiento: TratamientoLinea;
  empleado_id: string;
  nombre: string;
  identificacion: string;
  iban: string;
  departamento: string;
  puesto: string;
  salario_base: string;
  hijos: number;
  conyuge: boolean;
  bruto: string;
  base_ccss: string;
  base_renta: string;
  ccss_obrero: string;
  renta: string;
  deducciones: string;
  adelanto: string;
  neto: string;
  patronal: string;
  prov_aguinaldo: string;
  prov_vacaciones: string;
  prov_cesantia: string;
  detalle: DetalleLinea[];
}

export interface NovedadCorrida {
  empleado_id: string;
  concepto_id: string;
  concepto_nombre: string;
  monto: string;
  /** Horas cuando la novedad se registró por horas (extra); "0" si el monto es directo. */
  cantidad: string;
}

export interface CorridaDetalle extends Corrida {
  lineas: LineaCorrida[];
  novedades: NovedadCorrida[];
}

export interface CorridaInput {
  anio: number;
  mes: number;
  tipo: TipoCorrida;
  fecha_pago: string;
}

export interface NovedadInput {
  empleado_id: string;
  concepto_id: string;
  monto: string;
  /** Horas trabajadas. Con cantidad, el monto lo calcula el sistema (art. 139 CT). */
  cantidad?: string;
}

// ---- Finiquito / liquidación de cese (Etapa 3) ----

export type MotivoCese = "DESPIDO_RESPONSABILIDAD" | "RENUNCIA" | "FIN_CONTRATO" | "MUTUO_ACUERDO";
export type EstadoFiniquito = "BORRADOR" | "APROBADO" | "PAGADO" | "ANULADO";

export interface Finiquito {
  id: string;
  empleado_id: string;
  empleado_nombre: string;
  identificacion: string;
  fecha_ingreso: string;
  motivo: MotivoCese;
  fecha_salida: string;
  estado: EstadoFiniquito;
  dias_vacaciones: string;
  salario_promedio: string;
  salario_diario: string;
  anios_servicio: number;
  preaviso: string;
  cesantia: string;
  vacaciones: string;
  aguinaldo: string;
  /** Porción AFECTA (vacaciones pendientes) y sus retenciones; el resto es exento. */
  base_ccss: string;
  ccss_obrero: string;
  renta: string;
  /** Carga patronal sobre las vacaciones del cese: no se le resta, va a la planilla CCSS. */
  patronal: string;
  descuentos: string;
  total: string;
  detalle: DetalleLinea[];
  /** Acumulado provisionado (corridas pagadas) para el comparativo de la maqueta. */
  provisionado: string;
  /** true = los días de vacaciones los digitó RRHH; false = vienen del saldo calculado. */
  dias_vacaciones_manual: boolean;
  creado_en: string;
  aprobado_en: string;
  pagado_en: string;
}

export interface FiniquitoInput {
  empleado_id?: string;
  motivo: MotivoCese;
  fecha_salida: string;
  dias_vacaciones?: string;
}

// ---- Incapacidades y vacaciones (Etapa 4) ----

export type EntidadIncapacidad = "CCSS" | "INS";

export interface Incapacidad {
  id: string;
  empleado_id: string;
  empleado_nombre: string;
  entidad: EntidadIncapacidad;
  fecha_inicio: string;
  fecha_fin: string;
  dias: number;
  boleta: string;
  observaciones: string;
  anulada: boolean;
  /** Explicación de quién paga qué días (la calcula el backend). */
  subsidio: string;
  dias_empresa: string;
  creado_en: string;
}

export interface IncapacidadInput {
  empleado_id: string;
  entidad: EntidadIncapacidad;
  fecha_inicio: string;
  dias: number;
  boleta?: string;
  observaciones?: string;
}

/** Reporte del envío masivo de boletas: a quién le llegó y a quién no (y por qué). */
export interface ResultadoEnvioBoletas {
  enviados: number;
  /** Colaboradores sin correo en su ficha: no se les pudo avisar. */
  sin_correo: string[];
  /** El correo existe pero el envío falló. */
  fallidos: string[];
}

export interface Vacacion {
  id: string;
  empleado_id: string;
  empleado_nombre: string;
  fecha_inicio: string;
  dias: string;
  observaciones: string;
  anulada: boolean;
  creado_en: string;
}

export interface VacacionInput {
  empleado_id: string;
  fecha_inicio: string;
  dias: string;
  observaciones?: string;
}

export interface SaldoVacaciones {
  empleado_id: string;
  nombre: string;
  identificacion: string;
  fecha_ingreso: string;
  meses_servicio: number;
  acumulado: string;
  disfrutado: string;
  pendiente: string;
}

/** Descarga con el nombre autoritativo que puso el servidor. */
export interface Descarga {
  blob: Blob;
  filename: string;
}

export interface ProvisionEmpleado {
  empleado_id: string;
  nombre: string;
  identificacion: string;
  meses: number;
  aguinaldo: string;
  vacaciones: string;
  cesantia: string;
  total: string;
}

// ---- Dashboard de RRHH (Etapa 5) ----

export type EstadoPaso = "SIN_CREAR" | "BORRADOR" | "APROBADA" | "PAGADA" | "PENDIENTE" | "LISTA";

export interface DashboardPaso {
  estado: EstadoPaso;
  corrida_id: string;
  etiqueta: string;
}

export interface DashboardMes {
  anio: number;
  mes: number;
  etiqueta: string;
  costo: string;
  en_curso: boolean;
}

export interface DashboardAlerta {
  tono: "WARN" | "INFO" | "LEGAL";
  icono: string;
  texto: string;
}

export interface DashboardRRHH {
  anio: number;
  mes: number;
  empleados: number;
  empleados_pagados: number;
  /** Costo real = bruto devengado + cargas patronales + provisiones (sin finiquitos). */
  costo_real: string;
  bruto: string;
  patronal: string;
  base_ccss: string;
  patronal_pct: string;
  provisiones: string;
  prov_aguinaldo: string;
  prov_vacaciones: string;
  prov_cesantia: string;
  neto: string;
  neto_liquidacion: string;
  /** Colones de costo real por cada ₡100 de salario bruto. */
  costo_por_100: string;
  tendencia: DashboardMes[];
  ciclo: { adelanto: DashboardPaso; liquidacion: DashboardPaso; planilla: DashboardPaso };
  finiquitos: { cantidad: number; total: string; patronal: string; pendientes_pago: number };
  headcount: { departamento: string; empleados: number }[];
  alertas: DashboardAlerta[];
}

// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------

export const rrhhApi = {
  // --- Dashboard ---
  dashboard(anio: number, mes: number): Promise<DashboardRRHH> {
    return apiFetch<DashboardRRHH>("/rrhh/dashboard", {
      method: "GET",
      query: { anio: String(anio), mes: String(mes) },
    });
  },

  // --- Empleados ---
  empleados(filtros: FiltrosEmpleados): Promise<Empleado[]> {
    return apiFetch<{ items: Empleado[] }>("/rrhh/empleados", {
      method: "GET",
      query: { ...filtros },
    }).then((r) => r.items ?? []);
  },
  empleado(id: string): Promise<Empleado> {
    return apiFetch<Empleado>(`/rrhh/empleados/${id}`, { method: "GET" });
  },
  crearEmpleado(input: EmpleadoInput): Promise<Empleado> {
    return apiFetch<Empleado>("/rrhh/empleados", { method: "POST", json: input });
  },
  actualizarEmpleado(id: string, input: EmpleadoInput): Promise<Empleado> {
    return apiFetch<Empleado>(`/rrhh/empleados/${id}`, { method: "PATCH", json: input });
  },
  desactivarEmpleado(id: string, fechaSalida?: string): Promise<void> {
    return apiFetch<void>(`/rrhh/empleados/${id}/desactivar`, {
      method: "POST",
      json: fechaSalida ? { fecha_salida: fechaSalida } : {},
    });
  },

  // --- Parámetros del año ---
  parametros(anio: number): Promise<ParametrosNomina> {
    return apiFetch<ParametrosNomina>(`/rrhh/parametros/${anio}`, { method: "GET" });
  },
  guardarParametros(anio: number, input: ParametrosInput): Promise<ParametrosNomina> {
    return apiFetch<ParametrosNomina>(`/rrhh/parametros/${anio}`, { method: "PUT", json: input });
  },

  // --- Conceptos ---
  conceptos(): Promise<ConceptoNomina[]> {
    return apiFetch<{ items: ConceptoNomina[] }>("/rrhh/conceptos", { method: "GET" }).then(
      (r) => r.items ?? [],
    );
  },
  crearConcepto(input: ConceptoInput): Promise<ConceptoNomina> {
    return apiFetch<ConceptoNomina>("/rrhh/conceptos", { method: "POST", json: input });
  },
  actualizarConcepto(id: string, input: ConceptoInput): Promise<ConceptoNomina> {
    return apiFetch<ConceptoNomina>(`/rrhh/conceptos/${id}`, { method: "PATCH", json: input });
  },
  desactivarConcepto(id: string): Promise<void> {
    return apiFetch<void>(`/rrhh/conceptos/${id}/desactivar`, { method: "POST" });
  },

  // --- Deducciones recurrentes ---
  deducciones(empleadoId: string): Promise<DeduccionEmpleado[]> {
    return apiFetch<{ items: DeduccionEmpleado[] }>(`/rrhh/empleados/${empleadoId}/deducciones`, {
      method: "GET",
    }).then((r) => r.items ?? []);
  },
  crearDeduccion(empleadoId: string, input: DeduccionInput): Promise<DeduccionEmpleado> {
    return apiFetch<DeduccionEmpleado>(`/rrhh/empleados/${empleadoId}/deducciones`, {
      method: "POST",
      json: input,
    });
  },
  actualizarDeduccion(empleadoId: string, id: string, input: DeduccionInput): Promise<DeduccionEmpleado> {
    return apiFetch<DeduccionEmpleado>(`/rrhh/empleados/${empleadoId}/deducciones/${id}`, {
      method: "PATCH",
      json: input,
    });
  },
  desactivarDeduccion(empleadoId: string, id: string): Promise<void> {
    return apiFetch<void>(`/rrhh/empleados/${empleadoId}/deducciones/${id}/desactivar`, {
      method: "POST",
    });
  },

  // --- Corrida quincenal ---
  corridas(anio: number): Promise<Corrida[]> {
    return apiFetch<{ items: Corrida[] }>("/rrhh/corridas", {
      method: "GET",
      query: { anio: String(anio) },
    }).then((r) => r.items ?? []);
  },
  corrida(id: string): Promise<CorridaDetalle> {
    return apiFetch<CorridaDetalle>(`/rrhh/corridas/${id}`, { method: "GET" });
  },
  crearCorrida(input: CorridaInput): Promise<CorridaDetalle> {
    return apiFetch<CorridaDetalle>("/rrhh/corridas", { method: "POST", json: input });
  },
  guardarNovedades(id: string, novedades: NovedadInput[]): Promise<CorridaDetalle> {
    return apiFetch<CorridaDetalle>(`/rrhh/corridas/${id}/novedades`, {
      method: "PUT",
      json: { novedades },
    });
  },
  recalcularCorrida(id: string): Promise<CorridaDetalle> {
    return apiFetch<CorridaDetalle>(`/rrhh/corridas/${id}/recalcular`, { method: "POST" });
  },
  aprobarCorrida(id: string): Promise<Corrida> {
    return apiFetch<Corrida>(`/rrhh/corridas/${id}/aprobar`, { method: "POST" });
  },
  pagarCorrida(id: string): Promise<Corrida> {
    return apiFetch<Corrida>(`/rrhh/corridas/${id}/pagar`, { method: "POST" });
  },
  anularCorrida(id: string): Promise<Corrida> {
    return apiFetch<Corrida>(`/rrhh/corridas/${id}/anular`, { method: "POST" });
  },

  // --- Exportaciones de la corrida (.xlsx) ---
  // El nombre lo pone el backend: lleva el tipo de corrida y el consecutivo de bitácora
  // con el que se concilia el pago. El cliente NO lo reinventa.
  archivoPagoXLSX(corridaId: string): Promise<Descarga> {
    return apiFetch<Descarga>(`/rrhh/corridas/${corridaId}/archivo-pago`, {
      method: "GET",
      blobConNombre: true,
    });
  },
  planillaCCSSXLSX(corridaId: string): Promise<Descarga> {
    return apiFetch<Descarga>(`/rrhh/corridas/${corridaId}/planilla-ccss`, {
      method: "GET",
      blobConNombre: true,
    });
  },

  // --- Finiquitos ---
  finiquitos(): Promise<Finiquito[]> {
    return apiFetch<{ items: Finiquito[] }>("/rrhh/finiquitos", { method: "GET" }).then((r) => r.items ?? []);
  },
  finiquito(id: string): Promise<Finiquito> {
    return apiFetch<Finiquito>(`/rrhh/finiquitos/${id}`, { method: "GET" });
  },
  crearFiniquito(input: FiniquitoInput): Promise<Finiquito> {
    return apiFetch<Finiquito>("/rrhh/finiquitos", { method: "POST", json: input });
  },
  actualizarFiniquito(id: string, input: FiniquitoInput): Promise<Finiquito> {
    return apiFetch<Finiquito>(`/rrhh/finiquitos/${id}`, { method: "PATCH", json: input });
  },
  aprobarFiniquito(id: string): Promise<Finiquito> {
    return apiFetch<Finiquito>(`/rrhh/finiquitos/${id}/aprobar`, { method: "POST" });
  },
  pagarFiniquito(id: string): Promise<Finiquito> {
    return apiFetch<Finiquito>(`/rrhh/finiquitos/${id}/pagar`, { method: "POST" });
  },
  anularFiniquito(id: string): Promise<Finiquito> {
    return apiFetch<Finiquito>(`/rrhh/finiquitos/${id}/anular`, { method: "POST" });
  },

  // --- Incapacidades y vacaciones ---
  incapacidades(anio: number, mes: number): Promise<Incapacidad[]> {
    return apiFetch<{ items: Incapacidad[] }>("/rrhh/incapacidades", {
      method: "GET",
      query: { anio: String(anio), mes: String(mes) },
    }).then((r) => r.items ?? []);
  },
  registrarIncapacidad(input: IncapacidadInput): Promise<Incapacidad> {
    return apiFetch<Incapacidad>("/rrhh/incapacidades", { method: "POST", json: input });
  },
  anularIncapacidad(id: string): Promise<void> {
    return apiFetch<void>(`/rrhh/incapacidades/${id}/anular`, { method: "POST" });
  },
  saldosVacaciones(anio: number): Promise<SaldoVacaciones[]> {
    return apiFetch<{ items: SaldoVacaciones[] }>("/rrhh/vacaciones/saldos", {
      method: "GET",
      query: { anio: String(anio) },
    }).then((r) => r.items ?? []);
  },
  vacaciones(empleadoId?: string): Promise<Vacacion[]> {
    return apiFetch<{ items: Vacacion[] }>("/rrhh/vacaciones", {
      method: "GET",
      query: empleadoId ? { empleado_id: empleadoId } : undefined,
    }).then((r) => r.items ?? []);
  },
  registrarVacacion(input: VacacionInput): Promise<Vacacion> {
    return apiFetch<Vacacion>("/rrhh/vacaciones", { method: "POST", json: input });
  },
  anularVacacion(id: string): Promise<void> {
    return apiFetch<void>(`/rrhh/vacaciones/${id}/anular`, { method: "POST" });
  },

  // --- Notificaciones al colaborador (texto en Configuración → Notificaciones) ---
  enviarBoletas(corridaId: string): Promise<ResultadoEnvioBoletas> {
    return apiFetch<ResultadoEnvioBoletas>(`/rrhh/corridas/${corridaId}/boletas`, { method: "POST" });
  },
  avisarVacaciones(vacacionId: string): Promise<{ enviado: boolean }> {
    return apiFetch<{ enviado: boolean }>(`/rrhh/vacaciones/${vacacionId}/aviso`, { method: "POST" });
  },

  // --- Reportes ---
  provisiones(anio: number): Promise<ProvisionEmpleado[]> {
    return apiFetch<{ items: ProvisionEmpleado[] }>("/rrhh/reportes/provisiones", {
      method: "GET",
      query: { anio: String(anio) },
    }).then((r) => r.items ?? []);
  },
};
