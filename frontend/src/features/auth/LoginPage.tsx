import { useState, type FormEvent } from "react";
import { useAuth } from "@/features/auth/AuthContext";
import { ApiError } from "@/api/client";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
} from "@/components/ui";
import { ThemeToggle } from "@/components/ThemeToggle";
import { BrandLogo } from "@/components/shell/BrandLogo";

/** Traduce un error del backend a un mensaje de UI en español. */
function mensajeError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401) return "Credenciales inválidas. Revisá tu correo y contraseña.";
    if (err.status === 422) return err.message || "Datos inválidos.";
    return err.message || "No se pudo iniciar sesión.";
  }
  return "No se pudo conectar con el servidor. Intentá de nuevo.";
}

export function LoginPage() {
  const { login } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await login(email, password);
      // La navegación la resuelven los guards al cambiar el estado de auth.
    } catch (err) {
      setError(mensajeError(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen flex-col bg-surface">
      <header className="flex items-center justify-end px-6 py-4">
        <ThemeToggle />
      </header>

      <main className="flex flex-1 items-center justify-center px-4 pb-16">
        <div className="w-full max-w-sm">
          <div className="mb-6 flex justify-center">
            <BrandLogo tagline />
          </div>
          <Card className="w-full">
          <CardHeader>
            <CardTitle>Iniciar sesión</CardTitle>
            <CardDescription>Finance Group VDP — sistema de registro financiero</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="flex flex-col gap-4" noValidate>
              <Input
                label="Correo electrónico"
                type="email"
                name="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="tu@empresa.com"
              />
              <Input
                label="Contraseña"
                type="password"
                name="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
              />

              {error && (
                <p role="alert" className="text-sm text-negativo">
                  {error}
                </p>
              )}

              <Button type="submit" loading={loading} className="mt-2 w-full">
                Entrar
              </Button>
            </form>
          </CardContent>
          </Card>
        </div>
      </main>
    </div>
  );
}
