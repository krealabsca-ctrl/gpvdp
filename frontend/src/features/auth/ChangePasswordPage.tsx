/**
 * Cambio de contraseña obligatorio (contraseña temporal / primer ingreso).
 * Se llega aquí cuando el backend marca debe_cambiar_password. Al cambiarla,
 * se limpia la bandera y se continúa (a elegir empresa o a la app).
 */

import { useState, type FormEvent } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { Button, Input, useToast } from "@/components/ui";
import { authApi } from "@/api/auth";
import { mensajeError } from "@/lib/apiError";
import { useAuth } from "@/features/auth/AuthContext";

export function ChangePasswordPage() {
  const { status, debeCambiarPassword, necesitaSeleccionEmpresa, marcarPasswordCambiada } = useAuth();
  const navigate = useNavigate();
  const toast = useToast();
  const [actual, setActual] = useState("");
  const [nueva, setNueva] = useState("");
  const [repetir, setRepetir] = useState("");
  const [guardando, setGuardando] = useState(false);

  if (status === "unauthenticated") return <Navigate to="/login" replace />;
  // Si ya no debe cambiarla, no tiene nada que hacer acá.
  if (status === "authenticated" && !debeCambiarPassword) {
    return <Navigate to={necesitaSeleccionEmpresa ? "/seleccionar-empresa" : "/"} replace />;
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (nueva.length < 8) return toast.error("La nueva contraseña debe tener al menos 8 caracteres.");
    if (nueva !== repetir) return toast.error("Las contraseñas no coinciden.");
    setGuardando(true);
    try {
      await authApi.cambiarPassword(actual, nueva);
      marcarPasswordCambiada();
      toast.success("Contraseña actualizada.");
      navigate(necesitaSeleccionEmpresa ? "/seleccionar-empresa" : "/", { replace: true });
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setGuardando(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface p-4">
      <div className="w-full max-w-sm rounded-2xl border border-border bg-surface-raised p-6 shadow-card">
        <h1 className="text-lg font-semibold text-content">Cambiá tu contraseña</h1>
        <p className="mt-1 text-sm text-content-muted">
          Estás usando una contraseña temporal. Definí una nueva para continuar.
        </p>
        <form onSubmit={onSubmit} className="mt-5 flex flex-col gap-3">
          <Input
            label="Contraseña actual (temporal)"
            type="password"
            value={actual}
            onChange={(e) => setActual(e.target.value)}
            autoComplete="current-password"
            autoFocus
          />
          <Input
            label="Nueva contraseña"
            type="password"
            value={nueva}
            onChange={(e) => setNueva(e.target.value)}
            placeholder="Mínimo 8 caracteres"
            autoComplete="new-password"
          />
          <Input
            label="Repetir nueva contraseña"
            type="password"
            value={repetir}
            onChange={(e) => setRepetir(e.target.value)}
            autoComplete="new-password"
          />
          <Button type="submit" loading={guardando} disabled={!actual || !nueva || !repetir} className="mt-1">
            Cambiar contraseña
          </Button>
        </form>
      </div>
    </div>
  );
}
