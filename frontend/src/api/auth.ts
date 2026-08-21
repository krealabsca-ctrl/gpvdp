/**
 * Cliente tipado de los endpoints AUTH/CORE de la Fase 0.
 * Estos endpoints NO están en openapi-bancos.yaml, por eso los tipos van a mano
 * (ver src/api/types.ts) en lugar de generados.
 */

import { apiFetch } from "@/api/client";
import type {
  LoginResponse,
  MeResponse,
  EmpresaMembresia,
  SelectEmpresaResponse,
} from "@/api/types";

export const authApi = {
  /** POST /v1/auth/login — público. El access_token aún NO trae empresa. */
  login(email: string, password: string): Promise<LoginResponse> {
    return apiFetch<LoginResponse>("/auth/login", {
      method: "POST",
      json: { email, password },
      skipAuth: true,
    });
  },

  /**
   * POST /v1/auth/select-empresa — requiere Bearer.
   * Devuelve un access_token YA scopeado a la empresa elegida.
   */
  selectEmpresa(empresaId: string): Promise<SelectEmpresaResponse> {
    return apiFetch<SelectEmpresaResponse>("/auth/select-empresa", {
      method: "POST",
      json: { empresa_id: empresaId },
    });
  },

  /** GET /v1/me — rehidratación de sesión al recargar. */
  me(): Promise<MeResponse> {
    return apiFetch<MeResponse>("/me", { method: "GET" });
  },

  /** GET /v1/empresas — empresas a las que el usuario tiene acceso. */
  empresas(): Promise<EmpresaMembresia[]> {
    return apiFetch<EmpresaMembresia[]>("/empresas", { method: "GET" });
  },

  /** POST /v1/auth/cambiar-password — cambia la contraseña del usuario autenticado. */
  cambiarPassword(actual: string, nueva: string): Promise<{ ok: boolean }> {
    return apiFetch<{ ok: boolean }>("/auth/cambiar-password", {
      method: "POST",
      json: { actual, nueva },
    });
  },

  /** GET /v1/healthz — chequeo de salud (público). */
  healthz(): Promise<{ status: string }> {
    return apiFetch<{ status: string }>("/healthz", { method: "GET", skipAuth: true });
  },
};
