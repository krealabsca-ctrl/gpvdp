/**
 * Guard de ruta por PERMISO (defensa en profundidad; el backend ya deniega con 403).
 * Espera a que los permisos carguen (spinner) antes de decidir, para no montar la
 * página y disparar los 403 (p. ej. el Dashboard de Bancos sin bancos.ver). Si el
 * usuario no puede ver la ruta, lo lleva a la primera ruta accesible; si no tiene
 * ninguna, muestra un aviso de "sin acceso".
 */

import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { Spinner } from "@/components/ui";
import { permisoDeRuta, primeraRutaAccesible } from "@/app/nav";
import { usePuedeVer, useTienePermiso } from "@/features/auth/permisos";

export function PermisoGate({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  const permiso = permisoDeRuta(pathname);
  const { loading, permitido } = usePuedeVer(permiso);
  const tienePermiso = useTienePermiso();

  if (loading) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Spinner size="lg" label="Verificando permisos" />
      </div>
    );
  }

  if (!permitido) {
    const destino = primeraRutaAccesible(tienePermiso);
    if (destino !== pathname) return <Navigate to={destino} replace />;
    return (
      <div className="mx-auto max-w-md py-16 text-center">
        <h1 className="text-lg font-semibold text-content">Sin acceso</h1>
        <p className="mt-2 text-sm text-content-muted">
          No tenés permiso para ver esta sección en esta empresa. Pedile a un administrador que te
          asigne el permiso correspondiente en Configuración → Seguridad.
        </p>
      </div>
    );
  }

  return <>{children}</>;
}
