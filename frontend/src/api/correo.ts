/**
 * Correo saliente por empresa — DESDE QUÉ BUZÓN manda cada empresa (migración 0084).
 *
 * El TEXTO de los correos vive en `@/api/plantillas` y ya era configurable por empresa; esto es el
 * sobre, no la carta.
 *
 * LO QUE NO ESTÁ EN ESTE ARCHIVO, Y ES A PROPÓSITO: **no existe un campo `password` en la
 * respuesta**. El servidor no la devuelve —ni vacía ni enmascarada—, así que tampoco se declara
 * acá: lo que no está en el tipo nadie lo puede pintar por error. Lo único que se sabe de la
 * contraseña guardada es `tiene_password`.
 */

import { apiFetch } from "@/api/client";

/** Cómo se cifra el canal con el servidor de correo. */
export type SeguridadSMTP = "NINGUNO" | "STARTTLS" | "TLS";

/**
 * De dónde sale HOY el correo de la empresa:
 *  · EMPRESA — la configuración de esta pantalla.
 *  · GLOBAL  — las variables SMTP_* del servidor (el buzón del grupo).
 *  · NINGUNO — no hay ni lo uno ni lo otro: de verdad no sale nada.
 */
export type OrigenCorreo = "EMPRESA" | "GLOBAL" | "NINGUNO";

export interface CorreoSaliente {
  /** Hay fila propia de esta empresa. `false` NO significa «los correos no salen»: ver `origen_vigente`. */
  configurado: boolean;
  host: string;
  puerto: number;
  seguridad: SeguridadSMTP;
  usuario: string;
  /** Hay una contraseña guardada. Apagar la configuración NO la destruye. */
  tiene_password: boolean;
  remitente: string;
  remitente_nombre: string;
  activo: boolean;
  /**
   * Última prueba. `probado_en` lleno + `probado_error` null = salió bien. Los dos null = nunca se
   * probó, que no es lo mismo. `probado_error` es el ÚNICO lugar con detalle técnico del fallo
   * (categoría, código SMTP y servidor), y se lee con `admin.correo`.
   */
  probado_en: string | null;
  probado_error: string | null;
  actualizado_por: string;
  actualizado_en: string | null;
  origen_vigente: OrigenCorreo;
  /** La dirección desde la que salen hoy los correos, venga de la empresa o del global. */
  remitente_vigente: string;
  /** El servidor tiene la clave de cifrado. En `false`, guardar una contraseña responde 422. */
  cifrado_disponible: boolean;
}

export interface CorreoSalienteInput {
  host: string;
  puerto: number;
  seguridad: SeguridadSMTP;
  usuario: string;
  /**
   * TRES estados, porque el GET no puede devolverla y el formulario no la puede round-tripear:
   *   `undefined` → conservar la guardada
   *   `""`        → borrarla (el buzón queda sin autenticación)
   *   con valor   → cifrarla y reemplazar
   * `JSON.stringify` omite las propiedades `undefined`, así que «conservar» viaja como ausencia.
   */
  password?: string;
  remitente: string;
  remitente_nombre: string;
  activo: boolean;
}

/** Resultado de la prueba. El destino NO se elige: va al correo del usuario autenticado. */
export interface PruebaCorreo {
  ok: boolean;
  servidor: string;
  remitente: string;
  origen: "EMPRESA" | "GLOBAL";
  enviado_a: string;
  probado_en: string;
  descripcion: string;
}

export const correoApi = {
  /** La configuración de la empresa activa (que sale del token, nunca de la query). */
  obtener(): Promise<CorreoSaliente> {
    return apiFetch<CorreoSaliente>("/correo-saliente", { method: "GET" });
  },
  guardar(input: CorreoSalienteInput): Promise<{ guardada: boolean; tiene_password: boolean }> {
    return apiFetch("/correo-saliente", { method: "PUT", json: input });
  },
  /**
   * Prueba LO GUARDADO (no lo del formulario): el flujo de la pantalla es guardar → probar.
   * Sin cuerpo y sin destinatario a propósito — un endpoint autenticado que manda a una dirección
   * arbitraria desde el dominio de la empresa es correo que pasa SPF y DKIM.
   */
  probar(): Promise<PruebaCorreo> {
    return apiFetch<PruebaCorreo>("/correo-saliente/probar", { method: "POST" });
  },
};
