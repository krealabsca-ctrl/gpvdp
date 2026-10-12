import { describe, expect, it } from "vitest";

import { montoEnLetras } from "./ReporteLoteImprimible";

/**
 * El monto en letras del documento que firma Gerencia General.
 *
 * Es lo que hace verificable el papel de un vistazo: un cero de más en la cifra se nota al leer la
 * línea. Por eso tiene prueba y no se escribió «a ojo» — una cifra mal escrita en un documento
 * firmado es peor que no tenerla, y de ahí también el corte de arriba (ver el último caso).
 */
describe("montoEnLetras", () => {
  it("escribe los veintis juntos y con tilde, que es donde una regla genérica falla", () => {
    expect(montoEnLetras("22")).toBe("veintidós colones con 00/100");
    expect(montoEnLetras("26")).toBe("veintiséis colones con 00/100");
    expect(montoEnLetras("23")).toBe("veintitrés colones con 00/100");
    // Y 21 apocopa: «veintiún colones», nunca «veintiuno colones».
    expect(montoEnLetras("21")).toBe("veintiún colones con 00/100");
  });

  it("apocopa el uno cuando acompaña, pero no cuando es el monto entero", () => {
    expect(montoEnLetras("1")).toBe("un colón con 00/100");
    expect(montoEnLetras("31")).toBe("treinta y un colones con 00/100");
    expect(montoEnLetras("1000")).toBe("mil colones con 00/100");
    expect(montoEnLetras("21000")).toBe("veintiún mil colones con 00/100");
  });

  it("distingue cien de ciento", () => {
    expect(montoEnLetras("100")).toBe("cien colones con 00/100");
    expect(montoEnLetras("101")).toBe("ciento un colones con 00/100");
    expect(montoEnLetras("115")).toBe("ciento quince colones con 00/100");
  });

  it("maneja el singular y el plural del millón", () => {
    expect(montoEnLetras("1000000")).toBe("un millón colones con 00/100");
    expect(montoEnLetras("2000000")).toBe("dos millones colones con 00/100");
  });

  it("escribe los céntimos en quebrado, como un documento de pago en CR", () => {
    expect(montoEnLetras("26515.00")).toBe("veintiséis mil quinientos quince colones con 00/100");
    expect(montoEnLetras("26515.50")).toBe("veintiséis mil quinientos quince colones con 50/100");
    // Los céntimos nunca pierden el cero de la izquierda: «con 05/100», no «con 5/100».
    expect(montoEnLetras("10.05")).toBe("diez colones con 05/100");
  });

  it("escribe un monto real de un corte", () => {
    // El total del lote 29 de la base de pruebas.
    expect(montoEnLetras("124300.00")).toBe(
      "ciento veinticuatro mil trescientos colones con 00/100",
    );
  });

  it("cero no se rompe", () => {
    expect(montoEnLetras("0")).toBe("cero colones con 00/100");
    expect(montoEnLetras("0.75")).toBe("cero colones con 75/100");
  });

  // El corte es deliberado: antes que imprimir una cifra dudosa en un papel firmado, el documento
  // omite la línea y deja sola la cifra en números.
  it("devuelve vacío cuando no puede garantizar la escritura", () => {
    expect(montoEnLetras("1000000000")).toBe("");
    expect(montoEnLetras("-5")).toBe("");
    expect(montoEnLetras("")).toBe("");
    expect(montoEnLetras("no es un número")).toBe("");
  });
});
