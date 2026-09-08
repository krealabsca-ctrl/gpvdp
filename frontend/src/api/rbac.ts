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

/**
 * Un rol a medida que existe en otra empresa y se puede traer a la activa.
 *
 * Un rol a medida pertenece a su empresa y sus permisos se guardan por empresa, así que «el mismo
 * rol en dos empresas» son dos roles con el mismo código. Traerlo copia los permisos de una sola vez
 * en lugar de volver a marcarlos a mano, que es donde empiezan a diferenciarse sin que nadie lo note.
 */
export interface RolTraible {
  codigo: string;
  nombre: string;
  empresa_id: string;
  empresa_nombre: string;
  cuantos_permisos: number;
}

export interface UsuarioAdmin {
  id: string;
  nombre: string;
  email: string;
  activo: boolean;
  debe_cambiar_password: boolean;
  rol_codigo: string;
  rol_nombre: string;
  /**
   * Las OTRAS empresas del grupo a las que este usuario entra, separadas por « · ».
   * Vacío = solo la empresa activa. El acceso a cada empresa es una vinculación aparte, no un
   * permiso: para agregar una hay que dar de alta el mismo correo estando en esa empresa.
   */
  otras_empresas: string;
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
  /** Los roles a medida que existen en OTRAS empresas del usuario y todavía no en la activa. */
  rolesTraibles(): Promise<RolTraible[]> {
    return apiFetch<RolTraible[]>("/rbac/roles/traibles", { method: "GET" });
  },
  /** Copia acá un rol a medida de otra empresa, con sus permisos. */
  traerRol(empresaOrigenId: string, codigo: string): Promise<{ rol: RolItem; permisos_copiados: number }> {
    return apiFetch<{ rol: RolItem; permisos_copiados: number }>("/rbac/roles/traer", {
      method: "POST",
      json: { empresa_origen_id: empresaOrigenId, codigo },
    });
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
