/**
 * Reglas de la pantalla de Correo saliente que NO son de presentación.
 *
 * Están fuera del componente para poder probarlas: la de abajo (`exigeReescribirPassword`) es una
 * defensa de seguridad copiada del servidor, y una defensa que solo existe adentro de un JSX no se
 * puede verificar.
 */

import type { CorreoSaliente, OrigenCorreo, SeguridadSMTP } from "@/api/correo";

/** Las tres seguridades que acepta el servidor, con el puerto que usa cada una en la práctica. */
export const SEGURIDADES: { value: SeguridadSMTP; label: string }[] = [
  { value: "STARTTLS", label: "STARTTLS (lo normal: puerto 587)" },
  { value: "TLS", label: "TLS directo (puerto 465)" },
  { value: "NINGUNO", label: "Ninguna (sin cifrar — solo para pruebas)" },
];

/** Lo que el formulario manda al servidor, antes de decidir qué pasa con la contraseña. */
export interface FormularioCorreo {
  host: string;
  puerto: number;
  seguridad: SeguridadSMTP;
  usuario: string;
  remitente: string;
  remitente_nombre: string;
  activo: boolean;
}

/**
 * ¿Hay que volver a escribir la contraseña para poder guardar?
 *
 * El servidor responde 422 «cambió el servidor; hay que escribir de nuevo la contraseña del buzón»
 * cuando cambia host, puerto, usuario O seguridad y no viene contraseña nueva. **No es un capricho
 * de formulario**: la contraseña guardada está atada a (empresa, host, puerto, usuario), y sin esa
 * regla quien pueda editar esta pantalla se lleva la credencial sin verla nunca —apuntando el host
 * a su propio servidor, dejando el campo vacío («ausente = se conserva») y apretando «probar»—.
 *
 * Acá se calcula lo mismo para pedirla ANTES, no para reintentar: el 422 no se puede evitar
 * reintentando, y la pantalla que reintenta sola es la que deja al usuario dándole a un botón que
 * no va a funcionar nunca.
 *
 * Solo aplica si HAY contraseña guardada: sin nada guardado no hay nada que proteger.
 * La comparación del host ignora mayúsculas, igual que `strings.EqualFold` del servidor.
 */
export function exigeReescribirPassword(
  guardado: Pick<CorreoSaliente, "tiene_password" | "host" | "puerto" | "usuario" | "seguridad">,
  form: Pick<FormularioCorreo, "host" | "puerto" | "usuario" | "seguridad">,
): boolean {
  if (!guardado.tiene_password) return false;
  return (
    guardado.host.trim().toLowerCase() !== form.host.trim().toLowerCase() ||
    guardado.puerto !== form.puerto ||
    guardado.usuario.trim() !== form.usuario.trim() ||
    guardado.seguridad !== form.seguridad
  );
}

/**
 * Frase de una línea para «desde dónde sale hoy el correo de esta empresa».
 *
 * `configurado: false` NO es «los correos no salen»: mientras exista el buzón del grupo, salen por
 * ahí. Decir «sin configurar» a secas sería mentir con buena intención.
 */
export function textoOrigen(origen: OrigenCorreo, remitenteVigente: string): string {
  switch (origen) {
    case "EMPRESA":
      return `Los correos de esta empresa salen desde ${remitenteVigente || "su propio buzón"}.`;
    case "GLOBAL":
      return `Esta empresa todavía manda desde el buzón del grupo (${remitenteVigente || "el del servidor"}). Configurá el suyo para que el proveedor vea quién le escribe.`;
    default:
      return "No hay correo saliente: hoy esta empresa NO puede enviar comprobantes ni notificaciones.";
  }
}

/** Estado de la última prueba. «Nunca se probó» y «falló» no son lo mismo. */
export type EstadoPrueba = "nunca" | "ok" | "error";

export function estadoUltimaPrueba(
  cfg: Pick<CorreoSaliente, "probado_en" | "probado_error">,
): EstadoPrueba {
  if (cfg.probado_error) return "error";
  if (cfg.probado_en) return "ok";
  return "nunca";
}

/**
 * Qué hace el formulario con la contraseña al guardar. Son los TRES estados del contrato, y el
 * formulario los tiene explícitos por una razón: el GET no devuelve la contraseña, así que «el
 * campo quedó vacío» no puede significar «borrala». Si lo significara, cualquiera que entre a
 * corregir el nombre del remitente dejaría el buzón sin autenticación sin enterarse.
 *
 *  · `conservar` → la clave `password` NO viaja en el JSON (se queda la guardada).
 *  · `escribir`  → viaja con valor: se cifra y reemplaza.
 *  · `quitar`    → viaja como `""`: se borra (el buzón queda sin autenticación).
 */
export type ModoPassword = "conservar" | "escribir" | "quitar";

/**
 * El valor de `password` que viaja en el PUT. `undefined` es significativo: `JSON.stringify`
 * omite las propiedades `undefined`, y AUSENTE es justamente «conservar la guardada».
 */
export function passwordParaGuardar(modo: ModoPassword, valor: string): string | undefined {
  switch (modo) {
    case "escribir":
      return valor;
    case "quitar":
      return "";
    default:
      return undefined;
  }
}

/** Validación mínima de dirección: lo mismo que mira el servidor antes de responder 400. */
const RE_CORREO = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function esCorreoValido(valor: string): boolean {
  return RE_CORREO.test(valor.trim());
}

/**
 * Por qué NO se puede guardar todavía. Lista vacía = se puede.
 *
 * Es ESPEJO de las reglas del servidor (400 de validación y los 422 de regla de negocio), no una
 * capa de reglas propias: la idea es decirlas antes de que el usuario escriba una contraseña y se
 * la rebote el servidor. El backend las reverifica igual.
 */
export function problemasParaGuardar(
  guardado: Pick<
    CorreoSaliente,
    "tiene_password" | "host" | "puerto" | "usuario" | "seguridad" | "cifrado_disponible"
  >,
  form: FormularioCorreo,
  modo: ModoPassword,
  password: string,
): string[] {
  const problemas: string[] = [];

  if (!form.host.trim()) {
    problemas.push("Escribí el servidor de correo (por ejemplo, smtp.office365.com).");
  }
  if (!Number.isInteger(form.puerto) || form.puerto < 1 || form.puerto > 65535) {
    problemas.push("El puerto tiene que ser un número entre 1 y 65535.");
  }
  if (!esCorreoValido(form.remitente)) {
    problemas.push("El remitente tiene que ser una dirección de correo (alguien@empresa.com).");
  }

  // Pidió reemplazarla y dejó el campo vacío. Mensaje propio porque la salida es otra: cancelar
  // el reemplazo deja la guardada intacta.
  const reemplazoVacio = modo === "escribir" && !password && guardado.tiene_password;
  if (reemplazoVacio) {
    problemas.push("Escribí la contraseña nueva, o cancelá el reemplazo para dejar la guardada.");
  }
  if (modo === "escribir" && password && !guardado.cifrado_disponible) {
    problemas.push(
      "El servidor no tiene la clave de cifrado (CIFRADO_SECRET), así que no puede guardar contraseñas. Pedí que la configuren antes de escribirla.",
    );
  }
  // La defensa del cambio de servidor solo corre con la contraseña AUSENTE del JSON, que es
  // exactamente cuando el servidor la aplica (`in.Password == nil`). Mandarla vacía es un borrado
  // explícito y no se lleva ninguna credencial a ningún lado, así que ahí no aplica.
  if (modo === "conservar" && exigeReescribirPassword(guardado, form)) {
    problemas.push(
      "Cambió el servidor del buzón (host, puerto, usuario o seguridad): hay que escribir de nuevo la contraseña.",
    );
  }
  // Un buzón con usuario y sin contraseña no autentica: el servidor responde 422 y no guarda nada.
  // `quedaráSinPassword` es lo que el servidor va a terminar guardando, con los tres modos.
  const quedaraSinPassword =
    modo === "quitar" ||
    (modo === "escribir" && !password) ||
    (modo === "conservar" && !guardado.tiene_password);
  if (!reemplazoVacio && form.usuario.trim() && quedaraSinPassword) {
    problemas.push(
      "El buzón tiene usuario, así que hace falta su contraseña. Si el servidor no pide autenticación, dejá el usuario vacío.",
    );
  }

  return problemas;
}
