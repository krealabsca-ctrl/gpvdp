import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * Los permisos del usuario, y la cascada que ya no esperan (10-nov-2026).
 *
 * Esto era el segundo escalón de tres: los permisos esperaban a `GET /me` para saber la empresa, y
 * la página esperaba a los permisos para montarse. Tres viajes al servidor EN FILA antes de pedir
 * el primer dato; medido en la Bandeja con la red en cero, 211 ms de pura espera.
 *
 * Lo que se protege acá es que no vuelvan a encadenarse:
 *   1. La consulta arranca con el token SOLO, sin que nadie haya llamado a /me.
 *   2. Los permisos recordados sirven en el PRIMER render, así el guard no bloquea.
 *   3. Al cerrar sesión se borran, o el siguiente que entre ve el menú del anterior.
 */

vi.mock("@/api/rbac", () => ({
  rbacApi: { misPermisos: vi.fn() },
}));

// `useAuth` solo se usa para provocar un render cuando cambia la sesión; acá se finge en el estado
// en que peor estaba antes: TODAVÍA CARGANDO /me, sin empresa resuelta. Si la consulta arranca
// igual, es que de verdad dejó de depender de él.
vi.mock("@/features/auth/AuthContext", () => ({
  useAuth: () => ({ status: "loading", empresaActiva: null }),
}));

import { rbacApi } from "@/api/rbac";
import { olvidarPermisos } from "@/lib/permisosCache";
import { usePermisos, usePuedeVer } from "@/features/auth/permisos";

const EMPRESA = "427f8851-1e94-4c13-abbd-3f7bb9f456e9";

/** Un access_token con el claim `empresa_id`, como el que guarda el navegador tras elegir empresa. */
function tokenCon(empresaId: string | null): string {
  const payload = empresaId ? { empresa_id: empresaId, sub: "u1" } : { sub: "u1" };
  const base64url = (s: string) =>
    btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `h.${base64url(JSON.stringify(payload))}.firma`;
}

function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

const RESPUESTA = { rol: "ADMIN", es_admin: true, permisos: ["bancos.ver"] };

describe("usePermisos", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.mocked(rbacApi.misPermisos).mockResolvedValue(RESPUESTA);
  });

  it("arranca con el token, sin esperar a que /me responda", async () => {
    localStorage.setItem("gpvdp.access_token", tokenCon(EMPRESA));

    const { result } = renderHook(() => usePermisos(), { wrapper: ({ children }) => envolver(children) });

    // `useAuth` está fingido en "loading": si la consulta dependiera de él, no saldría nunca.
    await waitFor(() => expect(rbacApi.misPermisos).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.data).toEqual(RESPUESTA));
  });

  it("no pide permisos si el token todavía no tiene empresa", async () => {
    // Sesión recién iniciada, antes de elegir empresa: pedirlos daría 403.
    localStorage.setItem("gpvdp.access_token", tokenCon(null));

    renderHook(() => usePermisos(), { wrapper: ({ children }) => envolver(children) });

    await new Promise((r) => setTimeout(r, 30));
    expect(rbacApi.misPermisos).not.toHaveBeenCalled();
  });

  it("usa los permisos recordados en el PRIMER render", () => {
    localStorage.setItem("gpvdp.access_token", tokenCon(EMPRESA));
    localStorage.setItem(`gpvdp.permisos.${EMPRESA}`, JSON.stringify(RESPUESTA));

    const { result } = renderHook(() => usePuedeVer("bancos.ver"), {
      wrapper: ({ children }) => envolver(children),
    });

    // Sin esperar nada: si esto fuera `loading`, el guard mostraría el spinner y la página no se
    // montaría — que es exactamente el tercer escalón de la cascada.
    expect(result.current.loading).toBe(false);
    expect(result.current.permitido).toBe(true);
  });

  it("guarda lo que responde el servidor para la próxima vez", async () => {
    localStorage.setItem("gpvdp.access_token", tokenCon(EMPRESA));

    renderHook(() => usePermisos(), { wrapper: ({ children }) => envolver(children) });

    await waitFor(() =>
      expect(localStorage.getItem(`gpvdp.permisos.${EMPRESA}`)).toBe(JSON.stringify(RESPUESTA)),
    );
  });

  it("ignora un recuerdo con forma inválida en vez de reventar", () => {
    localStorage.setItem("gpvdp.access_token", tokenCon(EMPRESA));
    // Basura de otra versión: sin la comprobación de forma, `permisos.includes(...)` tiraría la
    // aplicación entera en el primer render.
    localStorage.setItem(`gpvdp.permisos.${EMPRESA}`, JSON.stringify({ permisos: "todos" }));

    const { result } = renderHook(() => usePuedeVer("bancos.ver"), {
      wrapper: ({ children }) => envolver(children),
    });

    expect(result.current.permitido).toBe(false); // todavía no sabe: espera al servidor
    expect(() => result.current.loading).not.toThrow();
  });

  it("olvidarPermisos borra los de todas las empresas", () => {
    localStorage.setItem(`gpvdp.permisos.${EMPRESA}`, JSON.stringify(RESPUESTA));
    localStorage.setItem("gpvdp.permisos.otra-empresa", JSON.stringify(RESPUESTA));
    localStorage.setItem("gpvdp.access_token", "no-se-toca");

    olvidarPermisos();

    expect(localStorage.getItem(`gpvdp.permisos.${EMPRESA}`)).toBeNull();
    expect(localStorage.getItem("gpvdp.permisos.otra-empresa")).toBeNull();
    // El token NO es asunto de esta función: lo limpia `tokenStore.clear()`.
    expect(localStorage.getItem("gpvdp.access_token")).toBe("no-se-toca");
  });
});
