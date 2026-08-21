/**
 * Cliente HTTP tipado del ERP GPVDP.
 *
 * Responsabilidades:
 *  - Base URL = VITE_API_URL + "/v1".
 *  - Inyecta Authorization: Bearer <access_token>.
 *  - Parsea { code, message } del backend en un ApiError.
 *  - Maneja 401: intenta UN refresh (una sola vez) y reintenta la petición.
 *    Si el refresh falla, limpia la sesión y notifica (para redirigir a login).
 *
 * La empresa activa NO se envía por header ni body: viaja dentro del
 * access_token (claim empresa_id). Para cambiar de empresa se pide token nuevo.
 */

import type { ApiErrorBody, RefreshResponse } from "@/api/types";
import { tokenStore } from "@/lib/tokenStore";
import { getEmpresaIdFromToken } from "@/lib/jwt";

/**
 * Base de la API. Sin `VITE_API_URL` (o con la variable vacía) se usa el MISMO origen desde el
 * que se abrió la página: así el sistema funciona igual por localhost, por IP de la red o por
 * nombre de máquina, sin que nadie tenga que configurar una dirección. El reverse proxy —el dev
 * server en desarrollo— reenvía /v1 al backend.
 *
 * Tiene que quedar ABSOLUTA, no una ruta relativa como "/v1": `buildUrl` construye la dirección
 * con `new URL()`, y ese constructor exige una base absoluta. Con "/v1" lanzaba
 * «TypeError: Failed to construct 'URL': Invalid URL» en TODAS las peticiones, y como no es un
 * ApiError la pantalla lo mostraba como «No se pudo conectar con el servidor» — un error de red
 * que no existía. `fetch` sí acepta rutas relativas, y por eso probarlo con fetch no lo detecta.
 */
const ORIGEN_ACTUAL = typeof window !== "undefined" ? window.location.origin : "http://localhost:8080";
const RAW_BASE = (import.meta.env.VITE_API_URL as string | undefined)?.trim() || ORIGEN_ACTUAL;
export const API_BASE = `${RAW_BASE.replace(/\/+$/, "")}/v1`;

/** Error tipado que expone el { code, message } del backend + status HTTP. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  /** Cuerpo JSON crudo del error (puede traer detalle extra, p. ej. no_identificados). */
  readonly body: Record<string, unknown> | null;

  constructor(status: number, code: string, message: string, body: Record<string, unknown> | null = null) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.body = body;
  }

  /** true si es un error de negocio (422). */
  get isBusinessRule(): boolean {
    return this.status === 422;
  }
  /** true si es falta de permiso en la empresa (403). */
  get isForbidden(): boolean {
    return this.status === 403;
  }
  /** true si es conflicto/duplicado (409). */
  get isConflict(): boolean {
    return this.status === 409;
  }
}

/**
 * Suscripción para cuando la sesión expira sin remedio (refresh falló).
 * El AuthProvider se suscribe para limpiar estado y mandar a /login.
 */
type SessionExpiredHandler = () => void;
let onSessionExpired: SessionExpiredHandler | null = null;
export function setOnSessionExpired(handler: SessionExpiredHandler | null): void {
  onSessionExpired = handler;
}

export interface RequestOptions extends Omit<RequestInit, "body"> {
  /** Cuerpo JSON (se serializa automáticamente). Para FormData usar `raw`. */
  json?: unknown;
  /** Cuerpo crudo (FormData, Blob, etc.), sin serializar. */
  raw?: BodyInit;
  /** Query params tipados. */
  /**
   * Params de la URL. Un arreglo se manda como parámetro REPETIDO (?p=a&p=b), que es la forma
   * estándar y la que el backend interpreta (también acepta la separada por comas). Un arreglo
   * vacío no escribe nada: «sin restricción» y «lista vacía» no son lo mismo.
   */
  query?: Record<string, string | number | boolean | string[] | undefined | null>;
  /** Si true, NO adjunta el Bearer (para login/refresh públicos). */
  skipAuth?: boolean;
  /** Si true, devuelve la respuesta como Blob (para descargas: CSV, binarios). */
  blob?: boolean;
  /**
   * Si true, devuelve `{ blob, filename }` con el nombre que puso el servidor en
   * Content-Disposition (para descargas cuyo nombre es autoritativo: lleva consecutivo
   * de bitácora y no debe reinventarse en el cliente).
   */
  blobConNombre?: boolean;
  /** Uso interno: evita bucle de refresh. */
  _isRetry?: boolean;
}

/** Construye la URL con query params, omitiendo valores nulos/undefined. */
function buildUrl(path: string, query?: RequestOptions["query"]): string {
  const url = new URL(`${API_BASE}${path.startsWith("/") ? path : `/${path}`}`);
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (Array.isArray(value)) {
        for (const v of value) {
          if (v !== "") url.searchParams.append(key, v);
        }
      } else if (value !== undefined && value !== null) {
        url.searchParams.set(key, String(value));
      }
    }
  }
  return url.toString();
}

/**
 * Intenta refrescar el access_token una sola vez.
 * Se serializa con una promesa compartida para evitar múltiples refresh
 * concurrentes cuando varias peticiones reciben 401 a la vez.
 */
let refreshPromise: Promise<boolean> | null = null;

async function tryRefresh(): Promise<boolean> {
  if (refreshPromise) return refreshPromise;

  refreshPromise = (async () => {
    const refresh = tokenStore.getRefresh();
    if (!refresh) return false;

    // Conserva el scope de empresa: si el token actual estaba scopeado a una
    // empresa, se re-scopea al refrescar (el backend re-verifica la pertenencia).
    const empresaId = getEmpresaIdFromToken(tokenStore.getAccess());

    try {
      const res = await fetch(buildUrl("/auth/refresh"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(
          empresaId ? { refresh_token: refresh, empresa_id: empresaId } : { refresh_token: refresh },
        ),
      });
      if (!res.ok) return false;
      const data = (await res.json()) as RefreshResponse;
      tokenStore.setTokens(data.access_token, data.refresh_token);
      return true;
    } catch {
      return false;
    } finally {
      // Se libera al terminar (el valor ya está resuelto para los que esperaban).
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

/** Parsea el cuerpo de error del backend a un ApiError legible. */
async function toApiError(res: Response): Promise<ApiError> {
  let code = "ERROR";
  let message = res.statusText || "Error de red";
  let body: Record<string, unknown> | null = null;
  try {
    const parsed = (await res.json()) as Partial<ApiErrorBody> & Record<string, unknown>;
    if (parsed && typeof parsed === "object") body = parsed;
    if (parsed && typeof parsed.code === "string") code = parsed.code;
    if (parsed && typeof parsed.message === "string") message = parsed.message;
  } catch {
    /* respuesta sin cuerpo JSON: se conservan los valores por defecto */
  }
  return new ApiError(res.status, code, message, body);
}

/**
 * Función central de fetch tipado.
 * @typeParam T tipo esperado del JSON de respuesta.
 */
export async function apiFetch<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { json, raw, query, skipAuth, blob, blobConNombre, _isRetry, headers, ...rest } = options;

  const finalHeaders = new Headers(headers);
  let body: BodyInit | undefined;

  if (raw !== undefined) {
    body = raw; // p. ej. FormData: dejar que el navegador ponga el Content-Type.
  } else if (json !== undefined) {
    finalHeaders.set("Content-Type", "application/json");
    body = JSON.stringify(json);
  }

  if (!skipAuth) {
    const access = tokenStore.getAccess();
    if (access) finalHeaders.set("Authorization", `Bearer ${access}`);
  }

  const res = await fetch(buildUrl(path, query), { ...rest, headers: finalHeaders, body });

  // 401 -> intentar refresh una única vez y reintentar.
  if (res.status === 401 && !skipAuth && !_isRetry) {
    const refreshed = await tryRefresh();
    if (refreshed) {
      return apiFetch<T>(path, { ...options, _isRetry: true });
    }
    // Refresh falló: sesión muerta.
    tokenStore.clear();
    onSessionExpired?.();
    throw await toApiError(res);
  }

  if (!res.ok) {
    throw await toApiError(res);
  }

  // 204 / sin cuerpo.
  if (res.status === 204) return undefined as T;
  // Descarga con el nombre autoritativo del servidor (Content-Disposition).
  if (blobConNombre) {
    const disposition = res.headers.get("Content-Disposition") ?? "";
    const match = /filename="?([^";]+)"?/i.exec(disposition);
    return { blob: await res.blob(), filename: match?.[1] ?? "" } as T;
  }
  // Descargas (CSV, binarios): devolver el Blob crudo sin intentar parsear JSON.
  if (blob) return (await res.blob()) as T;
  const text = await res.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}
