/**
 * Dominio del módulo CxP en el frontend: flujo de estados, etiquetas y tonos de
 * badge, y la matriz de aprobación por monto.
 *
 * IMPORTANTE: la matriz de aprobación y los roles habilitados son ESPEJO de
 * `backend/internal/cxp/aprobacion.go` y `internal/server/router.go`. El backend
 * es la autoridad (aquí solo se muestra/gatea la UI); si cambian allá, actualizar
 * aquí. No inventar reglas nuevas.
 */

import type { BadgeTone } from "@/components/ui";
import type { EstadoDocumento } from "@/api/cxp";

/** Orden lineal del flujo del documento CxP. */
export const FLUJO_ESTADOS: EstadoDocumento[] = [
  "RECIBIDO",
  "REVISADO",
  "VALIDADO_DEPTO",
  "APROBADO",
  "PROGRAMADO",
  "PAGADO",
  "CONCILIADO",
];

export const ETIQUETA_ESTADO: Record<EstadoDocumento, string> = {
  RECIBIDO: "Recibido",
  REVISADO: "Por validar (área)",
  VALIDADO_DEPTO: "Validado por el área",
  APROBADO: "Aprobado",
  PROGRAMADO: "Programado",
  PAGADO: "Pagado",
  CONCILIADO: "Conciliado",
  DENEGADO: "Denegado",
  ANULADO: "Anulado",
  LIQUIDADA: "Liquidada (sin pago)",
  REBOTADA: "Rebotada",
};

export const TONO_ESTADO: Record<EstadoDocumento, BadgeTone> = {
  RECIBIDO: "neutral",
  REVISADO: "pendiente",
  VALIDADO_DEPTO: "accent",
  APROBADO: "accent",
  PROGRAMADO: "pendiente",
  PAGADO: "accent",
  CONCILIADO: "positivo",
  DENEGADO: "negativo",
  ANULADO: "negativo",
  LIQUIDADA: "neutral",
  REBOTADA: "negativo",
};

/** Estados fuera del flujo lineal de pago (se muestran como pestañas aparte en el Flujo). */
export const ESTADOS_TERMINALES: EstadoDocumento[] = ["REBOTADA", "DENEGADO", "ANULADO", "LIQUIDADA"];

/** Etiquetas de los tipos de factura. */
export const ETIQUETA_TIPO: Record<string, string> = {
  CXP: "CxP",
  ANTICIPO: "Anticipo",
  VIATICOS: "Viáticos",
  REINTEGRO: "Reintegro",
  INTERNO: "Interno",
};

export const TIPOS_FACTURA = ["CXP", "ANTICIPO", "VIATICOS", "REINTEGRO", "INTERNO"] as const;

/**
 * Vía expresa (espejo de esViaExpresa del backend): documentos internos que gestiona
 * Contabilidad — anticipos, reintegros de caja chica y documentos internos. No requieren
 * validación de área: se aprueban directo con la matriz de firmas.
 */
export function esViaExpresa(tipo: string): boolean {
  return tipo === "ANTICIPO" || tipo === "REINTEGRO" || tipo === "INTERNO";
}

// Umbrales (CRC) — espejo de aprobacion.go.
export const UMBRAL_UN_APROBADOR = 1_000_000;
export const UMBRAL_DOS_APROBADOR = 5_000_000;

/** Cuántas firmas exige el monto y si una debe ser de Gerencia (espejo de backend). */
export function requisitoAprobacion(totalCRC: number): {
  requeridos: number;
  requiereGerencia: boolean;
} {
  if (totalCRC <= UMBRAL_UN_APROBADOR) return { requeridos: 1, requiereGerencia: false };
  if (totalCRC <= UMBRAL_DOS_APROBADOR) return { requeridos: 2, requiereGerencia: false };
  return { requeridos: 2, requiereGerencia: true };
}

/** Texto legible de la regla de aprobación para un monto. */
export function textoRequisitoAprobacion(totalCRC: number): string {
  const { requeridos, requiereGerencia } = requisitoAprobacion(totalCRC);
  if (requiereGerencia) {
    return "2 aprobaciones, una de ellas de Gerencia General (mancomunada).";
  }
  return requeridos === 1 ? "1 aprobación." : "2 aprobaciones.";
}

/** Etiquetas legibles de las acciones de auditoría (línea de tiempo). */
export const ETIQUETA_ACCION: Record<string, string> = {
  CREAR_DOCUMENTO: "Documento creado",
  CLASIFICAR_DOCUMENTO: "Clasificado (gasto)",
  TIPO_DOCUMENTO: "Tipo asignado",
  ASIGNAR_DEPARTAMENTO: "Departamento asignado",
  REVISAR_DOCUMENTO: "Revisado",
  VALIDAR_DEPTO: "Validado por el área (+ respaldo)",
  VALIDAR_ESCALADO: "Validado por escalamiento (Dirección)",
  DEVOLVER_DOCUMENTO: "Devuelto a Contabilidad",
  APROBAR_DOCUMENTO: "Aprobación registrada",
  // Facturas «de Contabilidad»: la excepción tiene que leerse en el histórico, no salir en crudo.
  MARCAR_CONTABILIDAD: "Marca de Contabilidad cambiada",
  APROBAR_DOCUMENTO_CONTABILIDAD: "Aprobada como de Contabilidad (sin validación de área)",
  MARCAR_PROVEEDOR_CONTABILIDAD: "Proveedor marcado como de Contabilidad",
  DESMARCAR_PROVEEDOR_CONTABILIDAD: "Proveedor ya no es de Contabilidad",
  MARCAR_CONCEPTO_CONTABILIDAD: "Concepto marcado como de Contabilidad",
  DESMARCAR_CONCEPTO_CONTABILIDAD: "Concepto ya no es de Contabilidad",
  MARCAR_CLASIFICACION_CONTABILIDAD: "Clasificación marcada como de Contabilidad",
  DESMARCAR_CLASIFICACION_CONTABILIDAD: "Clasificación ya no es de Contabilidad",
  PROGRAMAR_DOCUMENTO: "Pago programado",
  PAGAR_DOCUMENTO: "Marcado como pagado",
  CONCILIAR_DOCUMENTO: "Conciliado",
  CONCILIAR_AUTO: "Conciliado (automático por banco)",
  DENEGAR_DOCUMENTO: "Denegado",
  ANULAR_DOCUMENTO: "Anulado",
  LIQUIDAR_DOCUMENTO: "Liquidado (sin pago)",
  REBOTAR_DOCUMENTO: "Rebotado (banco)",
  REINTENTAR_DOCUMENTO: "Reintentar (a programado)",
  CREAR_LOTE_PAGO: "Incluido en lote de pago",
  APLICAR_ANTICIPO: "Anticipo aplicado",
  REVERSAR_ANTICIPO: "Aplicación de anticipo reversada",
};

/** Nombre legible de una acción de auditoría (fallback: la acción cruda). */
export function etiquetaAccion(accion: string): string {
  return ETIQUETA_ACCION[accion] ?? accion;
}

// Roles habilitados por acción — espejo de router.go / masivo.go.
export type AccionDocumento =
  | "revisar"
  | "validar"
  | "escalar"
  | "devolver"
  | "aprobar"
  | "programar"
  | "pagar"
  | "conciliar"
  | "denegar"
  | "anular"
  | "liquidar"
  | "rebotar"
  | "reintentar";

// Permiso RBAC requerido por cada acción — ESPEJO EXACTO de los gates de router.go.
// Se usa con los permisos EFECTIVOS del usuario (no con el rol), para que funcione también
// con roles a medida (p. ej. un "Validador de área"). El backend reverifica igual.
const PERMISO_POR_ACCION: Record<AccionDocumento, string> = {
  revisar: "cxp.revisar",
  validar: "cxp.validar_depto",
  escalar: "cxp.validar_escalado",
  devolver: "cxp.validar_depto", // el validador que puede validar también puede devolver
  aprobar: "cxp.aprobar",
  programar: "cxp.tesoreria",
  pagar: "cxp.tesoreria",
  conciliar: "cxp.tesoreria",
  denegar: "cxp.revisar", // denegar/anular/liquidar viajan por transición (cxp.revisar)
  anular: "cxp.revisar",
  liquidar: "cxp.revisar",
  rebotar: "cxp.tesoreria",
  reintentar: "cxp.tesoreria",
};

/** true si el usuario (por sus permisos efectivos) puede ejecutar la acción. */
export function puedeAccion(tienePermiso: (permiso: string) => boolean, accion: AccionDocumento): boolean {
  return tienePermiso(PERMISO_POR_ACCION[accion]);
}

/** Acción de transición disponible desde un estado (null si es final o no aplica). */
export function accionSiguiente(
  estado: EstadoDocumento,
): { accion: AccionDocumento; label: string } | null {
  switch (estado) {
    case "RECIBIDO":
      return { accion: "revisar", label: "Revisar" };
    case "REVISADO":
      return { accion: "validar", label: "Validar (área)" };
    case "VALIDADO_DEPTO":
      return { accion: "aprobar", label: "Registrar aprobación" };
    case "APROBADO":
      return { accion: "programar", label: "Programar pago" };
    case "PROGRAMADO":
      return { accion: "pagar", label: "Marcar pagado" };
    case "PAGADO":
      return { accion: "conciliar", label: "Conciliar" };
    default:
      return null;
  }
}
