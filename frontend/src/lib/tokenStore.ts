/**
 * Almacén de tokens en localStorage.
 * El estado de auth (tokens) puede vivir aquí + en el AuthContext.
 * El cliente HTTP (api/client.ts) lee/escribe estos tokens directamente para
 * poder inyectar el Bearer y hacer el refresh sin depender de React.
 *
 * NOTA: guardar el access_token en localStorage es una decisión pragmática de
 * Fase 0. La empresa activa NO se guarda aparte: va DENTRO del access_token
 * (claim empresa_id). Ver TODO de Fase 1 sobre endurecer esto.
 */

const ACCESS_KEY = "gpvdp.access_token";
const REFRESH_KEY = "gpvdp.refresh_token";

export const tokenStore = {
  getAccess(): string | null {
    return localStorage.getItem(ACCESS_KEY);
  },
  getRefresh(): string | null {
    return localStorage.getItem(REFRESH_KEY);
  },
  setTokens(access: string, refresh?: string): void {
    localStorage.setItem(ACCESS_KEY, access);
    if (refresh !== undefined) {
      localStorage.setItem(REFRESH_KEY, refresh);
    }
  },
  clear(): void {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
  },
  hasSession(): boolean {
    return Boolean(localStorage.getItem(ACCESS_KEY));
  },
};
