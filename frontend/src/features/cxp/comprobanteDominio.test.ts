import { describe, it, expect } from "vitest";
import {
  ETIQUETA_CATEGORIA_ENVIO,
  etiquetaIntento,
  fraseEnvioOK,
  frasePersonas,
  notaArchivo,
  tonoResultado,
} from "@/features/cxp/comprobanteDominio";
import { PERMISO_COMPROBANTE } from "@/features/cxp/dominio";
import type { CategoriaErrorEnvio } from "@/api/cxp";

/**
 * Las ocho categorías del contrato tienen que tener etiqueta. Si mañana el backend agrega una,
 * TypeScript rompe el Record —que es justo lo que se quiere—, pero esta prueba también avisa si
 * alguien la agrega con la etiqueta vacía, que se vería como una columna en blanco.
 */
describe("ETIQUETA_CATEGORIA_ENVIO", () => {
  const conError: CategoriaErrorEnvio[] = [
    "AUTENTICACION_RECHAZADA",
    "HOST_INALCANZABLE",
    "RELAY_DENEGADO",
    "TLS_FALLIDO",
    "TIEMPO_AGOTADO",
    "CORREO_NO_CONFIGURADO",
    "SECRETO_ILEGIBLE",
    "OTRO",
  ];

  it("toda categoría de error tiene etiqueta legible", () => {
    for (const c of conError) expect(ETIQUETA_CATEGORIA_ENVIO[c]).not.toBe("");
  });

  it("la categoría vacía (= salió bien) no pinta nada", () => {
    expect(ETIQUETA_CATEGORIA_ENVIO[""]).toBe("");
  });

  it("ninguna etiqueta filtra datos del servidor de correo", () => {
    // La bitácora la leen siete roles; el host, el usuario del buzón y el código SMTP viven en
    // `probado_error`, detrás de admin.correo. Nada de eso puede colarse en una etiqueta.
    for (const c of conError) {
      expect(ETIQUETA_CATEGORIA_ENVIO[c]).not.toMatch(/smtp|@|\d{3}/i);
    }
  });
});

describe("frasePersonas", () => {
  it("nombra al destinatario y a la copia", () => {
    expect(frasePersonas("prov@x.com", "jefe@vdp.com")).toBe(
      "a prov@x.com, con copia oculta a jefe@vdp.com",
    );
  });

  it("copia vacía se EXPLICA, no se esconde: no es un error", () => {
    // 44 de las 62 facturas pagadas de VDP no tienen aprobador con correo. Un hueco sin texto se
    // lee como «algo salió mal» y hace que alguien vaya a buscar un problema que no existe.
    const frase = frasePersonas("prov@x.com", "");
    expect(frase).toContain("sin copia");
    expect(frase).toContain("aprobador");
  });
});

describe("etiquetaIntento / fraseEnvioOK", () => {
  it("distingue el envío del reenvío", () => {
    expect(etiquetaIntento(false)).toBe("Envío");
    expect(etiquetaIntento(true)).toBe("Reenvío");
  });

  it("el toast dice a quién se mandó, sin volver a preguntar", () => {
    const frase = fraseEnvioOK({ destinatario: "prov@x.com", copia: "jefe@vdp.com", reenvio: true });
    expect(frase).toContain("Reenvío");
    expect(frase).toContain("prov@x.com");
    expect(frase).toContain("jefe@vdp.com");
  });
});

describe("tonoResultado", () => {
  it("usa los tonos semánticos del Badge", () => {
    expect(tonoResultado("OK")).toBe("positivo");
    expect(tonoResultado("ERROR")).toBe("negativo");
  });
});

/**
 * Reemplazar el adjunto deja los envíos viejos con `mismo_archivo:false`. Sin decirlo, la bitácora
 * afirmaría que al proveedor le llegó el PDF que hay hoy en el expediente, y no es verdad.
 */
describe("notaArchivo", () => {
  it("avisa cuando lo que se mandó ya no es el adjunto de hoy", () => {
    expect(notaArchivo({ archivo: "pago.pdf", mismo_archivo: true })).toBe("pago.pdf");
    expect(notaArchivo({ archivo: "pago.pdf", mismo_archivo: false })).toContain("se reemplazó");
  });
});

/**
 * La desalineación que arregla esta entrega: las dos pantallas gateaban adjuntar/enviar con el
 * permiso de PAGAR (`cxp.tesoreria`), pero el backend exige `cxp.comprobante`. El Auxiliar
 * Financiero tiene `cxp.comprobante` y NO tiene tesorería: los botones no le aparecían.
 */
describe("permiso del comprobante", () => {
  it("es el mismo código que exige el router del backend", () => {
    expect(PERMISO_COMPROBANTE).toBe("cxp.comprobante");
  });
});
