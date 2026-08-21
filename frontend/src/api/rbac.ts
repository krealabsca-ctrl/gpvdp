/**
 * Cliente tipado del módulo RBAC (matriz permiso × rol × empresa).
 * Mirror manual del backend (igual que auth.ts/bancos.ts). El token lleva el rol;
 * el backend resuelve permisos por request, así que la matriz aplica casi en vivo.
 */

import { apiFetch } from "@/api/client";

export interface PermisoDef {
  Codigo: string;
  Modulo: string;
  Nombre: string;
  Descripcion: string;
  Critico: boolean;
}

export interface RolItem {
  id: string;
  codigo: string;
  nombre: string;
  es_base: boolean;
  es_admin: boolean;
}

export interface MatrizGrant {
  rol_codigo: string;
  permiso_codigo: string;
}

export interface MisPermisos {
  rol: string;
  es_admin: boolean;
  permisos: string[];
}

export interface UsuarioAdmin {
  id: string;
  nombre: string;
  email: string;
  activo: boolean;
  debe_cambiar_password: boolean;
  rol_codigo: string;
  rol_nombre: string;
}

export interface CrearUsuarioInput {
  nombre: string;
  email: string;
  password: string;
  rol_codigo: string;
}

export const rbacApi = {
  /** Permisos efectivos del usuario en la empresa activa (para ocultar acciones). */
  misPermisos(): Promise<MisPermisos> {
    return apiFetch<MisPermisos>("/rbac/mis-permisos", { method: "GET" });
  },
  /** Catálogo completo de permisos (para la matriz). */
  catalogo(): Promise<{ permisos: PermisoDef[] }> {
    return apiFetch<{ permisos: PermisoDef[] }>("/rbac/permisos", { method: "GET" });
  },
  roles(): Promise<RolItem[]> {
    return apiFetch<RolItem[]>("/rbac/roles", { method: "GET" });
  },
  matriz(): Promise<MatrizGrant[]> {
    return apiFetch<MatrizGrant[]>("/rbac/matriz", { method: "GET" });
  },
  setPermisos(rolCodigo: string, permisos: string[]): Promise<void> {
    return apiFetch<void>(`/rbac/roles/${rolCodigo}/permisos`, { method: "PUT", json: { permisos } });
  },
  crearRol(nombre: string): Promise<RolItem> {
    return apiFetch<RolItem>("/rbac/roles", { method: "POST", json: { nombre } });
  },

  // --- Usuarios (Administración, por empresa activa) ---
  usuarios(): Promise<UsuarioAdmin[]> {
    return apiFetch<UsuarioAdmin[]>("/rbac/usuarios", { method: "GET" });
  },
  crearUsuario(input: CrearUsuarioInput): Promise<{ ok: boolean; nuevo: boolean }> {
    return apiFetch<{ ok: boolean; nuevo: boolean }>("/rbac/usuarios", { method: "POST", json: input });
  },
  actualizarUsuario(id: string, input: { nombre: string; activo: boolean; rol_codigo?: string }): Promise<void> {
    return apiFetch<void>(`/rbac/usuarios/${id}`, { method: "PATCH", json: input });
  },
  resetPassword(id: string, password: string): Promise<void> {
    return apiFetch<void>(`/rbac/usuarios/${id}/reset-password`, { method: "POST", json: { password } });
  },
  quitarAcceso(id: string): Promise<void> {
    return apiFetch<void>(`/rbac/usuarios/${id}/acceso`, { method: "DELETE" });
  },
  aplicarPermisosFaltantes(): Promise<{ ok: boolean; agregados: number }> {
    return apiFetch<{ ok: boolean; agregados: number }>("/rbac/permisos/aplicar-faltantes", { method: "POST" });
  },
};
