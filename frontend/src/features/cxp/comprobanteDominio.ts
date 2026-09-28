/**
 * Reglas de presentación de la bitácora de envíos del comprobante (migración 0084) que NO son
 * JSX, para poder probarlas.
 *
 * Lo que hay que entender de este archivo:
 *
 *  · `error` que viene del servidor YA es una frase para el operador de CxP: no trae el host, ni
 *    el usuario del buzón, ni el código SMTP. Acá NO se la reescribe ni se le agrega detalle —el
 *    detalle técnico vive en `probado_error` del correo saliente, detrás de `admin.correo`, que
 *    hoy tiene un solo rol mientras esta bitácora la ven siete—. Lo único que se agrega es una
 *    ETIQUETA CORTA de la categoría, para poder leer la columna de un vistazo.
 *  · La pareja (`resultado`, `error_categoria`) la garantiza un CHECK de la base: categoría vacía
 *    si y solo si salió bien. Por eso `ETIQUETA_CATEGORIA_ENVIO[""]` es "" y no «desconocido».
 */

import type { BadgeTone } from "@/components/ui";
import type { CategoriaErrorEnvio, EnvioComprobante } from "@/api/cxp";

/** Etiqueta corta de por qué no salió. Vacía cuando salió bien (no hay categoría). */
export const ETIQUETA_CATEGORIA_ENVIO: Record<CategoriaErrorEnvio, string> = {
  "": "",
  AUTENTICACION_RECHAZADA: "Credencial rechazada",
  HOST_INALCANZABLE: "Sin conexión",
  RELAY_DENEGADO: "Destino rechazado",
  TLS_FALLIDO: "Conexión segura",
  TIEMPO_AGOTADO: "Sin respuesta",
  CORREO_NO_CONFIGURADO: "Correo sin configurar",
  SECRETO_ILEGIBLE: "Contraseña ilegible",
  OTRO: "Otro fallo",
};

/** Tono del badge del resultado. Solo hay dos resultados posibles. */
export function tonoResultado(resultado: EnvioComprobante["resultado"]): BadgeTone {
  return resultado === "OK" ? "positivo" : "negativo";
}

/**
 * «Envío» o «Reenvío». El backend ya lo derivó (el primer OK es el envío; los siguientes, reenvíos),
 * así que acá solo se nombra: recalcularlo en pantalla sería tener dos verdades.
 */
export function etiquetaIntento(reenvio: boolean): string {
  return reenvio ? "Reenvío" : "Envío";
}

/**
 * «a X, con copia oculta a Y» — la misma frase para el toast del envío y para la bitácora, así no
 * pueden divergir.
 *
 * `copia` vacía NO es un error: significa que la factura no tiene aprobador con correo, y el envío
 * salió igual. Decirlo evita que alguien lea el hueco como «falló algo».
 */
export function frasePersonas(destinatario: string, copia: string): string {
  if (!copia) return `a ${destinatario} (sin copia: la factura no tiene aprobador con correo)`;
  return `a ${destinatario}, con copia oculta a ${copia}`;
}

/** Frase completa del toast de un envío que salió bien. */
export function fraseEnvioOK(r: {
  destinatario: string;
  copia: string;
  reenvio: boolean;
}): string {
  return `${etiquetaIntento(r.reenvio)} hecho: se mandó ${frasePersonas(r.destinatario, r.copia)}.`;
}

/**
 * Nota de una fila cuyo PDF ya no es el que está adjunto hoy.
 *
 * Vale la pena que se lea: si alguien reemplazó el comprobante, lo que recibió el proveedor en esa
 * fila NO es lo que hay ahora en el expediente, y esa es la pregunta que se va a hacer quien abra
 * la bitácora buscando qué se mandó.
 */
export function notaArchivo(e: Pick<EnvioComprobante, "archivo" | "mismo_archivo">): string {
  if (e.mismo_archivo) return e.archivo;
  return `${e.archivo} · el adjunto se reemplazó después`;
}
