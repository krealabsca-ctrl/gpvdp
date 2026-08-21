import { Navigate, Outlet } from "react-router-dom";
import { useAuth } from "@/features/auth/AuthContext";
import { Spinner } from "@/components/ui";

/**
 * Guard inverso: rutas solo para NO autenticados (p. ej. /login).
 * Si ya hay sesión, redirige al destino adecuado.
 */
export function PublicOnlyRoute() {
  const { status, necesitaSeleccionEmpresa } = useAuth();

  if (status === "loading") {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner size="lg" label="Cargando sesión" />
      </div>
    );
  }

  if (status === "authenticated") {
    return <Navigate to={necesitaSeleccionEmpresa ? "/seleccionar-empresa" : "/"} replace />;
  }

  return <Outlet />;
}
