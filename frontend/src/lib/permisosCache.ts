/**
 * Lo último que el servidor dijo sobre los permisos del usuario, por empresa.
 *
 * EXISTE PARA QUE LA PANTALLA NO SE QUEDE EN BLANCO AL REFRESCAR. El guard de ruta esperaba a que
 * llegaran los permisos para montar la página, y la página esperaba a montarse para pedir sus
 * datos: tres viajes al servidor en fila. Con los permisos recordados, el guard decide en el primer
 * render y los datos salen de una, mientras la revalidación viaja por detrás.
 *
 * NO ES LA AUTORIDAD. Quien autoriza es el backend, en cada petición. Si a alguien le quitan un
 * permiso, lo peor que pasa es que vea su menú de siempre durante el instante que tarda la
 * respuesta fresca, y el servidor le niegue la acción igual.
 *
 * Vive en `lib/` y no junto al hook a propósito: `AuthContext` tiene que poder borrarlo al cerrar
 * sesión, y el hook de permisos usa `AuthContext`. Ponerlo en el mismo archivo que el hook ataba
 * los dos módulos en un ciclo.
 */

export interface PermisosGuardados {
  rol: string;
  es_admin: boolean;
  permisos: string[];
}

const PREFIJO = "gpvdp.permisos.";

export function leerPermisosGuardados(empresaId: string | null): PermisosGuardados | undefined {
  if (!empresaId) return undefined;
  try {
    const crudo = localStorage.getItem(PREFIJO + empresaId);
    if (!crudo) return undefined;
    const data = JSON.parse(crudo) as PermisosGuardados;
    // Se comprueba la FORMA, no solo que haya algo: con basura de otra versión guardada acá,
    // `permisos.includes(...)` reventaría la aplicación entera en el primer render.
    if (!Array.isArray(data.permisos) || typeof data.es_admin !== "boolean") return undefined;
    return data;
  } catch {
    return undefined;
  }
}

export function guardarPermisos(empresaId: string | null, data: PermisosGuardados): void {
  if (!empresaId) return;
  try {
    localStorage.setItem(PREFIJO + empresaId, JSON.stringify(data));
  } catch {
    // Sin espacio, o en una ventana privada: se pierde el arranque instantáneo y nada más.
  }
}

/** Borra los permisos recordados de TODAS las empresas. Se llama al cerrar sesión. */
export function olvidarPermisos(): void {
  try {
    for (const clave of Object.keys(localStorage)) {
      if (clave.startsWith(PREFIJO)) localStorage.removeItem(clave);
    }
  } catch {
    // Nada que hacer: el siguiente inicio de sesión los sobrescribe por empresa.
  }
}
