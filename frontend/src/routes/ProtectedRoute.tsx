import { Navigate, Outlet, useLocation } from "react-router-dom";
import { useAuth } from "@/features/auth/AuthContext";
import { Spinner } from "@/components/ui";

/**
 * Guard de rutas autenticadas.
 *  - status "loading": pantalla de carga (bootstrap con GET /me en curso).
 *  - sin sesión -> /login.
 *  - con sesión pero sin empresa activa -> /seleccionar-empresa.
 *  - todo OK -> renderiza las rutas hijas.
 */
export function ProtectedRoute() {
  const { status, necesitaSeleccionEmpresa, debeCambiarPassword } = useAuth();
  const location = useLocation();

  if (status === "loading") {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner size="lg" label="Cargando sesión" />
      </div>
    );
  }

  if (status === "unauthenticated") {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  // Contraseña temporal / primer ingreso: se fuerza el cambio antes de usar la app.
  if (debeCambiarPassword && location.pathname !== "/cambiar-password") {
    return <Navigate to="/cambiar-password" replace />;
  }

  if (necesitaSeleccionEmpresa && !debeCambiarPassword) {
    return <Navigate to="/seleccionar-empresa" replace />;
  }

  return <Outlet />;
}
