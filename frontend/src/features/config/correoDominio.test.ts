import { describe, it, expect } from "vitest";
import {
  estadoUltimaPrueba,
  exigeReescribirPassword,
  passwordParaGuardar,
  problemasParaGuardar,
  textoOrigen,
  type FormularioCorreo,
} from "@/features/config/correoDominio";
import type { CorreoSaliente } from "@/api/correo";

/** Una configuración guardada completa, para ir cambiándole una cosa por prueba. */
const GUARDADO: CorreoSaliente = {
  configurado: true,
  host: "smtp.office365.com",
  puerto: 587,
  seguridad: "STARTTLS",
  usuario: "cxp@valledepazcr.com",
  tiene_password: true,
  remitente: "cxp@valledepazcr.com",
  remitente_nombre: "Valle de Paz — Cuentas por pagar",
  activo: true,
  probado_en: null,
  probado_error: null,
  actualizado_por: "Administrador GPVDP",
  actualizado_en: null,
  origen_vigente: "EMPRESA",
  remitente_vigente: "cxp@valledepazcr.com",
  cifrado_disponible: true,
};

const FORM: FormularioCorreo = {
  host: GUARDADO.host,
  puerto: GUARDADO.puerto,
  seguridad: GUARDADO.seguridad,
  usuario: GUARDADO.usuario,
  remitente: GUARDADO.remitente,
  remitente_nombre: GUARDADO.remitente_nombre,
  activo: GUARDADO.activo,
};

/**
 * EL ATAQUE QUE ESTA REGLA DETIENE: quien pueda editar la pantalla apunta el host a un servidor
 * suyo, deja la contraseña «como está» (ausente = se conserva) y le da a Probar. El servidor le
 * presentaría la credencial del correo corporativo a un destino elegido por el atacante.
 *
 * El backend lo corta con un 422; acá se calcula lo MISMO para pedir la contraseña antes, porque
 * ese 422 no se arregla reintentando.
 */
describe("exigeReescribirPassword", () => {
  it("exige reescribirla si cambia el host", () => {
    expect(exigeReescribirPassword(GUARDADO, { ...FORM, host: "smtp.atacante.com" })).toBe(true);
  });

  it("exige reescribirla si cambia el puerto, el usuario o la seguridad", () => {
    expect(exigeReescribirPassword(GUARDADO, { ...FORM, puerto: 465 })).toBe(true);
    expect(exigeReescribirPassword(GUARDADO, { ...FORM, usuario: "otro@valledepazcr.com" })).toBe(true);
    expect(exigeReescribirPassword(GUARDADO, { ...FORM, seguridad: "TLS" })).toBe(true);
  });

  it("no la exige si solo cambia lo que no la ata (nombre del remitente, encendido)", () => {
    // `problemasParaGuardar` es el que recibe el formulario entero: acá se ve que tocar el nombre
    // del remitente o apagar el buzón NO obliga a reescribir la contraseña.
    expect(problemasParaGuardar(GUARDADO, { ...FORM, remitente_nombre: "VDP" }, "conservar", "")).toEqual([]);
    expect(problemasParaGuardar(GUARDADO, { ...FORM, activo: false }, "conservar", "")).toEqual([]);
  });

  it("ignora mayúsculas y espacios del host, igual que EqualFold del servidor", () => {
    expect(exigeReescribirPassword(GUARDADO, { ...FORM, host: " SMTP.Office365.COM " })).toBe(false);
  });

  it("no exige nada si no hay contraseña guardada: no hay credencial que proteger", () => {
    const sinPass = { ...GUARDADO, tiene_password: false };
    expect(exigeReescribirPassword(sinPass, { ...FORM, host: "smtp.atacante.com" })).toBe(false);
  });
});

/**
 * Los tres estados del contrato. El importante es `conservar` → `undefined`: `JSON.stringify`
 * omite las propiedades `undefined`, y AUSENTE es lo que el servidor lee como «dejá la guardada».
 * Si esto devolviera "" en vez de `undefined`, entrar a corregir el nombre del remitente borraría
 * la credencial del buzón.
 */
describe("passwordParaGuardar", () => {
  it("conservar NO manda la clave (ausente = se queda la guardada)", () => {
    expect(passwordParaGuardar("conservar", "")).toBeUndefined();
    expect(JSON.stringify({ password: passwordParaGuardar("conservar", "") })).toBe("{}");
  });

  it("quitar manda cadena vacía (borrar)", () => {
    expect(passwordParaGuardar("quitar", "")).toBe("");
  });

  it("escribir manda el valor tal cual", () => {
    expect(passwordParaGuardar("escribir", "hunter2")).toBe("hunter2");
  });
});

describe("problemasParaGuardar", () => {
  it("deja guardar lo que ya estaba, sin tocar la contraseña", () => {
    expect(problemasParaGuardar(GUARDADO, FORM, "conservar", "")).toEqual([]);
  });

  it("deja configurar una empresa que nunca tuvo buzón", () => {
    // La respuesta REAL de una empresa sin configuración: todo vacío y `tiene_password:false`.
    const sinConfigurar = {
      tiene_password: false,
      host: "",
      puerto: 587,
      usuario: "",
      seguridad: "STARTTLS" as const,
      cifrado_disponible: true,
    };
    expect(problemasParaGuardar(sinConfigurar, FORM, "escribir", "hunter2")).toEqual([]);
  });

  it("pide la contraseña cuando cambió el servidor", () => {
    const problemas = problemasParaGuardar(
      GUARDADO,
      { ...FORM, host: "smtp.atacante.com" },
      "conservar",
      "",
    );
    expect(problemas.some((p) => p.includes("Cambió el servidor"))).toBe(true);
  });

  it("acepta el cambio de servidor si viene contraseña nueva", () => {
    expect(
      problemasParaGuardar(GUARDADO, { ...FORM, host: "smtp.gmail.com" }, "escribir", "nueva"),
    ).toEqual([]);
  });

  it("no deja «reemplazar» con el campo vacío (eso borraría la guardada sin querer)", () => {
    const problemas = problemasParaGuardar(GUARDADO, FORM, "escribir", "");
    expect(problemas.some((p) => p.includes("Escribí la contraseña nueva"))).toBe(true);
  });

  it("avisa ANTES de escribirla cuando el servidor no puede cifrar", () => {
    const sinCifrado = { ...GUARDADO, cifrado_disponible: false };
    const problemas = problemasParaGuardar(sinCifrado, FORM, "escribir", "hunter2");
    expect(problemas.some((p) => p.includes("CIFRADO_SECRET"))).toBe(true);
  });

  it("valida host, puerto y remitente como el servidor", () => {
    // Se compara por TEXTO y no por cantidad: tocar el host o el puerto además dispara la defensa
    // del cambio de servidor, así que esos casos traen dos avisos y los dos son correctos.
    const con = (p: string[], texto: string) => p.some((x) => x.includes(texto));
    expect(con(problemasParaGuardar(GUARDADO, { ...FORM, host: "  " }, "conservar", ""), "servidor de correo")).toBe(true);
    expect(con(problemasParaGuardar(GUARDADO, { ...FORM, puerto: 0 }, "conservar", ""), "entre 1 y 65535")).toBe(true);
    expect(con(problemasParaGuardar(GUARDADO, { ...FORM, puerto: 70000 }, "conservar", ""), "entre 1 y 65535")).toBe(true);
    expect(problemasParaGuardar(GUARDADO, { ...FORM, remitente: "no-es-correo" }, "conservar", "")).toEqual([
      "El remitente tiene que ser una dirección de correo (alguien@empresa.com).",
    ]);
  });

  it("no deja un buzón con usuario y sin contraseña (el servidor responde 422)", () => {
    const sinPass = { ...GUARDADO, tiene_password: false };
    expect(problemasParaGuardar(sinPass, FORM, "conservar", "")).toHaveLength(1);
    // Quitarla explícitamente tiene el mismo problema mientras quede el usuario.
    expect(problemasParaGuardar(GUARDADO, FORM, "quitar", "")).toHaveLength(1);
    // Sin usuario, un buzón sin autenticación es válido. Y borrar NO dispara la defensa del
    // cambio de servidor, porque el servidor solo la aplica con la contraseña AUSENTE del JSON:
    // mandar "" es un borrado explícito, no un intento de llevarse la credencial a otro host.
    expect(problemasParaGuardar(GUARDADO, { ...FORM, usuario: "" }, "quitar", "")).toEqual([]);
  });
});

/**
 * `configurado:false` NO es «los correos no salen»: mientras exista el buzón del grupo, salen por
 * ahí. Es la diferencia entre avisar de algo que hay que arreglar y mentir con buena intención.
 */
describe("textoOrigen", () => {
  it("distingue las TRES situaciones, no dos", () => {
    expect(textoOrigen("EMPRESA", "cxp@vdp.com")).toContain("cxp@vdp.com");
    expect(textoOrigen("GLOBAL", "erp@grupo.com")).toContain("buzón del grupo");
    expect(textoOrigen("NINGUNO", "")).toContain("NO puede enviar");
  });
});

/** «Nunca se probó» y «la última prueba falló» no son lo mismo, y los dos campos vienen null. */
describe("estadoUltimaPrueba", () => {
  it("nunca / ok / error", () => {
    expect(estadoUltimaPrueba({ probado_en: null, probado_error: null })).toBe("nunca");
    expect(estadoUltimaPrueba({ probado_en: "2026-09-17T22:20:16Z", probado_error: null })).toBe("ok");
    expect(
      estadoUltimaPrueba({ probado_en: "2026-09-17T22:20:16Z", probado_error: "535 …" }),
    ).toBe("error");
  });
});
