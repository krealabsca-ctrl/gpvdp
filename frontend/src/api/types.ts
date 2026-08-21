/**
 * Tipos del contrato AUTH/CORE de la Fase 0.
 * Escritos a mano porque estos endpoints NO están en openapi-bancos.yaml todavía.
 * Los tipos del módulo Bancos SÍ se generan (src/api/generated/bancos.ts).
 */

export interface Usuario {
  id: string;
  nombre: string;
  email: string;
}

export interface EmpresaMembresia {
  id: string;
  nombre: string;
  /** Rol del usuario en esa empresa (p. ej. "ADMIN", "CONTADOR"). */
  rol: string;
}

export interface EmpresaActiva {
  id: string;
  nombre: string;
}

/** Respuesta de POST /v1/auth/login. El access_token aún NO trae empresa. */
export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  user: Usuario;
  empresas: EmpresaMembresia[];
  debe_cambiar_password?: boolean;
}

/** Respuesta de POST /v1/auth/refresh. */
export interface RefreshResponse {
  access_token: string;
  refresh_token: string;
}

/** Respuesta de POST /v1/auth/select-empresa. Token YA scopeado a la empresa. */
export interface SelectEmpresaResponse {
  access_token: string;
}

/** Respuesta de GET /v1/me. */
export interface MeResponse {
  user: Usuario;
  empresas: EmpresaMembresia[];
  empresa_activa: EmpresaActiva | null;
  rol: string | null;
  debe_cambiar_password?: boolean;
}

/** Forma del error del backend: { code, message }. */
export interface ApiErrorBody {
  code: string;
  message: string;
}
