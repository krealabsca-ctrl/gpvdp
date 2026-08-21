/**
 * Estado de AUTENTICACIÓN de la app (tokens, usuario, empresas, empresa activa).
 *
 * Decisión: el estado de auth NO es "estado de servidor" reutilizable como los
 * datos de Bancos, así que vive en un Context de React + localStorage
 * (los datos de servidor van SOLO en TanStack Query).
 *
 * Flujo:
 *  - login(): guarda tokens/usuario/empresas; si hay 1 empresa auto-selecciona.
 *  - selectEmpresa(): pide token scopeado y lo guarda.
 *  - bootstrap al montar: si hay access_token, GET /me para rehidratar.
 *  - logout(): limpia todo.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import { authApi } from "@/api/auth";
import { setOnSessionExpired } from "@/api/client";
import { tokenStore } from "@/lib/tokenStore";
import { getEmpresaIdFromToken } from "@/lib/jwt";
import type { EmpresaActiva, EmpresaMembresia, Usuario } from "@/api/types";

export type AuthStatus = "loading" | "authenticated" | "unauthenticated";

export interface AuthState {
  status: AuthStatus;
  user: Usuario | null;
  empresas: EmpresaMembresia[];
  empresaActiva: EmpresaActiva | null;
  /** Rol del usuario en la empresa activa. */
  rol: string | null;
  /** true cuando hay sesión pero falta elegir empresa. */
  necesitaSeleccionEmpresa: boolean;
  /** true cuando el usuario debe cambiar su contraseña (temporal / primer ingreso). */
  debeCambiarPassword: boolean;
}

export interface AuthContextValue extends AuthState {
  login: (email: string, password: string) => Promise<void>;
  selectEmpresa: (empresaId: string) => Promise<void>;
  logout: () => void;
  /** Marca la contraseña como ya cambiada (tras el cambio exitoso). */
  marcarPasswordCambiada: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

/** Resuelve la empresa activa a partir del claim del token + la lista de empresas. */
function resolveEmpresaActiva(
  empresas: EmpresaMembresia[],
): EmpresaActiva | null {
  const empresaId = getEmpresaIdFromToken(tokenStore.getAccess());
  if (!empresaId) return null;
  const match = empresas.find((e) => e.id === empresaId);
  return match ? { id: match.id, nombre: match.nombre } : null;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [state, setState] = useState<AuthState>({
    status: "loading",
    user: null,
    empresas: [],
    empresaActiva: null,
    rol: null,
    necesitaSeleccionEmpresa: false,
    debeCambiarPassword: false,
  });

  // Evita doble bootstrap en StrictMode (dev monta dos veces).
  const didBootstrap = useRef(false);

  const setUnauthenticated = useCallback(() => {
    tokenStore.clear();
    queryClient.clear();
    setState({
      status: "unauthenticated",
      user: null,
      empresas: [],
      empresaActiva: null,
      rol: null,
      necesitaSeleccionEmpresa: false,
      debeCambiarPassword: false,
    });
  }, [queryClient]);

  const logout = useCallback(() => {
    setUnauthenticated();
  }, [setUnauthenticated]);

  // Cuando el client detecta refresh fallido, forzamos logout.
  useEffect(() => {
    setOnSessionExpired(() => setUnauthenticated());
    return () => setOnSessionExpired(null);
  }, [setUnauthenticated]);

  // Bootstrap al recargar: si hay token, rehidratar con GET /me.
  useEffect(() => {
    if (didBootstrap.current) return;
    didBootstrap.current = true;

    if (!tokenStore.hasSession()) {
      setState((s) => ({ ...s, status: "unauthenticated" }));
      return;
    }

    // NO se cancela esta petición en el cleanup: con React.StrictMode el efecto
    // se monta, se limpia y se remonta; como `didBootstrap` evita un segundo
    // fetch, cancelar el primero dejaría el estado atascado en "loading" para
    // siempre (spinner infinito al recargar con sesión). El GET /me es
    // idempotente, así que se deja resolver y aplicar su resultado.
    (async () => {
      try {
        const me = await authApi.me();
        const empresaActiva = me.empresa_activa ?? resolveEmpresaActiva(me.empresas);
        setState({
          status: "authenticated",
          user: me.user,
          empresas: me.empresas,
          empresaActiva,
          rol: me.rol,
          necesitaSeleccionEmpresa: empresaActiva === null,
          debeCambiarPassword: me.debe_cambiar_password ?? false,
        });
      } catch {
        // El client ya intentó refresh; si llega aquí (ApiError o error de red),
        // la sesión no sirve: tratamos como no autenticado y a login.
        setUnauthenticated();
      }
    })();
  }, [setUnauthenticated]);

  const login = useCallback(
    async (email: string, password: string) => {
      const res = await authApi.login(email, password);
      tokenStore.setTokens(res.access_token, res.refresh_token);

      // Auto-selección si solo hay una empresa.
      if (res.empresas.length === 1) {
        const unica = res.empresas[0]!;
        const sel = await authApi.selectEmpresa(unica.id);
        tokenStore.setTokens(sel.access_token);
        setState({
          status: "authenticated",
          user: res.user,
          empresas: res.empresas,
          empresaActiva: { id: unica.id, nombre: unica.nombre },
          rol: unica.rol,
          necesitaSeleccionEmpresa: false,
          debeCambiarPassword: res.debe_cambiar_password ?? false,
        });
        return;
      }

      // Varias empresas: entrar en estado "hay que elegir".
      setState({
        status: "authenticated",
        user: res.user,
        empresas: res.empresas,
        empresaActiva: null,
        rol: null,
        necesitaSeleccionEmpresa: true,
        debeCambiarPassword: res.debe_cambiar_password ?? false,
      });
    },
    [],
  );

  const selectEmpresa = useCallback(
    async (empresaId: string) => {
      const sel = await authApi.selectEmpresa(empresaId);
      tokenStore.setTokens(sel.access_token);
      // Cambiar de empresa segrega caché: limpiamos datos de servidor.
      queryClient.clear();

      setState((s) => {
        const membresia = s.empresas.find((e) => e.id === empresaId) ?? null;
        return {
          ...s,
          status: "authenticated",
          empresaActiva: membresia ? { id: membresia.id, nombre: membresia.nombre } : null,
          rol: membresia?.rol ?? null,
          necesitaSeleccionEmpresa: membresia === null,
        };
      });
    },
    [queryClient],
  );

  const marcarPasswordCambiada = useCallback(() => {
    setState((s) => ({ ...s, debeCambiarPassword: false }));
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({ ...state, login, selectEmpresa, logout, marcarPasswordCambiada }),
    [state, login, selectEmpresa, logout, marcarPasswordCambiada],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/** Hook de acceso al contexto de auth. Lanza si se usa fuera del provider. */
// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth debe usarse dentro de <AuthProvider>");
  return ctx;
}
