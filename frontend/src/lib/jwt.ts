/**
 * Decodificación *sin verificación* del payload de un JWT.
 * Solo para leer claims en el cliente (p. ej. empresa_id) y mostrar UI.
 * La verificación real la hace SIEMPRE el backend; nunca confiar en esto
 * para decisiones de seguridad.
 */

export interface JwtClaims {
  /** Empresa a la que está scopeado el token (null si aún no se seleccionó). */
  empresa_id?: string;
  sub?: string;
  exp?: number;
  [key: string]: unknown;
}

/** Decodifica base64url -> string, compatible con navegador. */
function base64UrlDecode(segment: string): string {
  const base64 = segment.replace(/-/g, "+").replace(/_/g, "/");
  const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), "=");
  const binary = atob(padded);
  // Reconstruir UTF-8 correctamente (nombres/acentos).
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

export function decodeJwt(token: string | null): JwtClaims | null {
  if (!token) return null;
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  const payload = parts[1];
  if (!payload) return null;
  try {
    return JSON.parse(base64UrlDecode(payload)) as JwtClaims;
  } catch {
    return null;
  }
}

/** Devuelve el empresa_id del claim del access_token, o null. */
export function getEmpresaIdFromToken(token: string | null): string | null {
  const claims = decodeJwt(token);
  const empresaId = claims?.empresa_id;
  return typeof empresaId === "string" && empresaId.length > 0 ? empresaId : null;
}
