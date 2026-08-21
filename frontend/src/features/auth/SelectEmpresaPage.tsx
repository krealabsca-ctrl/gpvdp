/**
 * Selector de empresa — «mosaico de marca» (propuesta aprobada por el usuario).
 *
 * Las empresas se eligen por su LOGOTIPO, no por una línea de texto: cada tarjeta
 * lleva la marca oficial sobre su recuadro blanco y una franja con el color de esa
 * empresa. Es la primera pantalla del día, así que también resuelve tres cosas que
 * antes faltaban: se puede salir de la sesión, se recuerda la última empresa usada
 * (queda con el foco, para entrar con un Enter) y se puede elegir con el teclado.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { Navigate } from "react-router-dom";
import { useAuth } from "@/features/auth/AuthContext";
import { ApiError } from "@/api/client";
import { Spinner } from "@/components/ui";
import { ThemeToggle } from "@/components/ThemeToggle";
import { BrandLogo } from "@/components/shell/BrandLogo";
import { LogoEmpresa, marcaDeEmpresa } from "@/components/shell/LogoEmpresa";
import { cn } from "@/lib/cn";

/** Clave de la última empresa usada (para dejarla enfocada al volver). */
const CLAVE_ULTIMA = "gpvdp.ultima-empresa";

function mensajeError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.isForbidden) return "No tenés acceso a esa empresa.";
    return err.message || "No se pudo seleccionar la empresa.";
  }
  return "No se pudo conectar con el servidor. Intentá de nuevo.";
}

export function SelectEmpresaPage() {
  const { status, empresas, necesitaSeleccionEmpresa, selectEmpresa, logout, user } = useAuth();
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});

  const ultimaId = useMemo(() => {
    if (typeof window === "undefined") return null;
    return window.localStorage.getItem(CLAVE_ULTIMA);
  }, []);

  // La última empresa usada arranca con el foco: entrar es un Enter.
  useEffect(() => {
    if (!empresas.length) return;
    const objetivo = (ultimaId && refs.current[ultimaId]) || refs.current[empresas[0]?.id ?? ""];
    objetivo?.focus({ preventScroll: true });
  }, [empresas, ultimaId]);

  // Atajos 1-9: elegir directo con el número de la tarjeta.
  useEffect(() => {
    function onKey(ev: KeyboardEvent) {
      if (pendingId || ev.metaKey || ev.ctrlKey || ev.altKey) return;
      const n = Number(ev.key);
      if (!Number.isInteger(n) || n < 1 || n > empresas.length) return;
      const emp = empresas[n - 1];
      if (emp) void elegir(emp.id);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [empresas, pendingId]);

  if (status === "unauthenticated") return <Navigate to="/login" replace />;
  if (status === "authenticated" && !necesitaSeleccionEmpresa) return <Navigate to="/" replace />;

  async function elegir(empresaId: string) {
    setError(null);
    setPendingId(empresaId);
    try {
      window.localStorage.setItem(CLAVE_ULTIMA, empresaId);
      await selectEmpresa(empresaId);
      // Al setear empresa activa, el guard redirige al dashboard.
    } catch (err) {
      setError(mensajeError(err));
      setPendingId(null);
    }
  }

  return (
    <div className="flex min-h-screen flex-col bg-surface">
      <header className="flex items-center gap-3 px-6 py-4">
        <BrandLogo size={30} />
        <div className="ml-auto flex items-center gap-2">
          {user?.email && (
            <span className="hidden text-xs text-content-muted sm:inline">{user.email}</span>
          )}
          <button
            type="button"
            onClick={() => void logout()}
            className={cn(
              "rounded-lg border border-border px-3 py-1.5 text-xs font-semibold text-content-muted",
              "transition-colors hover:border-accent hover:text-accent",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent",
            )}
          >
            Salir
          </button>
          <ThemeToggle />
        </div>
      </header>

      <main className="mx-auto flex w-full max-w-4xl flex-1 flex-col justify-center px-5 py-8">
        <div className="text-center">
          <h1 className="text-2xl font-semibold tracking-tight text-content sm:text-[26px]">
            ¿Con cuál empresa vas a trabajar?
          </h1>
          <p className="mx-auto mt-2 max-w-xl text-sm text-content-muted">
            {user ? `Hola, ${user.nombre}. ` : ""}
            Todo lo que abrás adentro pertenece a una sola empresa; podés cambiarla después desde la
            barra superior.
          </p>
        </div>

        {error && (
          <p
            role="alert"
            className="mx-auto mt-5 rounded-lg border border-negativo/40 bg-negativo/5 px-4 py-2.5 text-sm text-negativo"
          >
            {error}
          </p>
        )}

        {empresas.length === 0 ? (
          <p className="mt-8 text-center text-sm text-content-muted">
            Tu usuario todavía no tiene ninguna empresa asignada. Pedile a un administrador que te dé
            acceso.
          </p>
        ) : (
          <ul
            className={cn(
              "mt-8 grid gap-4",
              empresas.length >= 3 ? "sm:grid-cols-2 lg:grid-cols-3" : "sm:grid-cols-2",
            )}
          >
            {empresas.map((empresa, i) => {
              const marca = marcaDeEmpresa(empresa.nombre);
              const entrando = pendingId === empresa.id;
              const otraEnCurso = pendingId !== null && !entrando;
              return (
                <li key={empresa.id}>
                  <button
                    ref={(el) => {
                      refs.current[empresa.id] = el;
                    }}
                    type="button"
                    onClick={() => void elegir(empresa.id)}
                    disabled={pendingId !== null}
                    aria-busy={entrando || undefined}
                    aria-label={`Entrar a ${empresa.nombre} como ${empresa.rol}`}
                    style={{ ["--marca" as string]: marca.color }}
                    className={cn(
                      "group relative flex h-full w-full flex-col items-stretch gap-3 overflow-hidden",
                      "rounded-2xl border border-border bg-surface-raised p-4 pb-3.5 text-left shadow-card",
                      "transition-all duration-200",
                      "hover:-translate-y-1 hover:border-[--marca] hover:shadow-lifted",
                      "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-surface",
                      entrando && "-translate-y-1 border-[--marca] shadow-lifted",
                      otraEnCurso && "pointer-events-none scale-[0.99] opacity-40 saturate-50",
                      pendingId !== null && "cursor-not-allowed",
                    )}
                  >
                    {/* Franja con el color de la marca: crece al acercarse. */}
                    <span
                      aria-hidden="true"
                      className={cn(
                        "absolute inset-x-0 top-0 h-[3px] origin-left bg-[--marca] transition-transform duration-300",
                        entrando ? "scale-x-100" : "scale-x-0 group-hover:scale-x-100",
                      )}
                    />
                    <span
                      aria-hidden="true"
                      className="absolute right-3 top-3 rounded-md border border-border border-b-2 bg-surface px-1.5 text-[10.5px] font-bold text-content-muted"
                    >
                      {i + 1}
                    </span>

                    <LogoEmpresa nombre={empresa.nombre} marca={marca} alto={88} className="mt-1.5" />

                    <span className="flex flex-1 flex-col gap-1">
                      <span className="text-[15px] font-semibold leading-tight tracking-tight text-content">
                        {empresa.nombre}
                      </span>
                      <span className="text-[11.5px] leading-snug text-content-muted">
                        {marca.descriptor}
                      </span>
                    </span>

                    <span className="flex items-center gap-2">
                      <span className="rounded-full border border-border px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider text-content-muted">
                        {empresa.rol}
                      </span>
                      {ultimaId === empresa.id && (
                        <span className="rounded-full border border-brand-gold/50 bg-brand-gold/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider text-brand-gold">
                          Última vez
                        </span>
                      )}
                    </span>

                    <span className="mt-1 flex items-center gap-2 text-[13px] font-bold text-[--marca]">
                      {entrando ? (
                        <>
                          <Spinner size="sm" label="Entrando" />
                          Entrando…
                        </>
                      ) : (
                        <>
                          Entrar
                          <span
                            aria-hidden="true"
                            className="transition-transform duration-200 group-hover:translate-x-1"
                          >
                            →
                          </span>
                        </>
                      )}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}

        {empresas.length > 1 && (
          <p className="mt-7 text-center text-[11.5px] text-content-muted">
            También podés elegir con las teclas{" "}
            <Tecla>1</Tecla>–<Tecla>{String(empresas.length)}</Tecla>, o moverte con{" "}
            <Tecla>Tab</Tecla> y entrar con <Tecla>↵</Tecla>.
          </p>
        )}
      </main>
    </div>
  );
}

function Tecla({ children }: { children: React.ReactNode }) {
  return (
    <span className="rounded border border-border border-b-2 bg-surface-raised px-1.5 py-0.5 text-[10.5px] font-bold text-content-muted">
      {children}
    </span>
  );
}
