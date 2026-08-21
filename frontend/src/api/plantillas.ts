/**
 * Plantillas de correo de las notificaciones (comprobante al proveedor, boleta de pago,
 * vacaciones). El TEXTO es configuración por empresa; el sistema solo llena las variables.
 */

import { apiFetch } from "@/api/client";

export interface VariablePlantilla {
  nombre: string;
  descripcion: string;
  /** Valor de muestra que se usa en la vista previa. */
  ejemplo: string;
}

export interface PlantillaVigente {
  clave: string;
  asunto: string;
  cuerpo: string;
  /** false = rige el texto de fábrica (la empresa no lo ha cambiado). */
  personalizada: boolean;
  actualizado_en: string;
  actualizado_por: string;
}

export interface TipoNotificacion {
  clave: string;
  nombre: string;
  descripcion: string;
  modulo: string;
  variables: VariablePlantilla[];
  asunto_default: string;
  cuerpo_default: string;
  vigente: PlantillaVigente;
}

export interface VistaPreviaCorreo {
  asunto: string;
  cuerpo: string;
  /** Variables del texto que el sistema no sabe llenar (no deja guardar con estas). */
  desconocidas: string[];
}

export const plantillasApi = {
  listar(): Promise<TipoNotificacion[]> {
    return apiFetch<TipoNotificacion[]>("/plantillas", { method: "GET" });
  },
  guardar(clave: string, asunto: string, cuerpo: string): Promise<{ guardada: boolean }> {
    return apiFetch<{ guardada: boolean }>(`/plantillas/${clave}`, {
      method: "PUT",
      json: { asunto, cuerpo },
    });
  },
  restablecer(clave: string): Promise<{ restablecida: boolean }> {
    return apiFetch<{ restablecida: boolean }>(`/plantillas/${clave}`, { method: "DELETE" });
  },
  vistaPrevia(clave: string, asunto: string, cuerpo: string): Promise<VistaPreviaCorreo> {
    return apiFetch<VistaPreviaCorreo>(`/plantillas/${clave}/vista-previa`, {
      method: "POST",
      json: { asunto, cuerpo },
    });
  },
};
