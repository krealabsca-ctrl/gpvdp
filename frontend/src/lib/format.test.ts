import { describe, expect, it } from "vitest";

import { aFechaHora, formatMoneda, montoACentimos, montoLegible, montoParaApi } from "./format";

/**
 * EL BUG DE LA CAPTURA DE SALDOS (10 de setiembre de 2026).
 *
 * La pantalla de saldos diarios tenía su propio normalizador —`texto.replace(/[^\d.,-]/g,"")`
 * seguido de borrar las comas— en vez de usar estos helpers. Con eso, copiar el número TAL COMO LA
 * APP LO MUESTRA mandaba a la base un monto cien veces mayor, sin un solo error a la vista:
 *
 *   «152 345 000,50»  ->  15234500050   (₡15.234.500.050 en vez de ₡152.345.000,50)
 *   «1.234,56»        ->  1.23456       (₡1,23 en vez de ₡1.234,56)
 *
 * Estos casos son la red para que ningún formato que una persona escriba de verdad se malinterprete.
 */
describe("montos que escribe una persona", () => {
  const casos: { entrada: string; api: string; porque: string }[] = [
    { entrada: "152345000", api: "152345000.00", porque: "sin separadores" },
    { entrada: "152.345.000", api: "152345000.00", porque: "miles con punto (como se escribe en CR)" },
    { entrada: "152,345,000", api: "152345000.00", porque: "miles con coma (como se escribe en EEUU)" },
    {
      entrada: "152 345 000,50",
      api: "152345000.50",
      porque: "EXACTAMENTE como lo muestra la app: el caso que mandaba 100 veces de más",
    },
    { entrada: "1.234,56", api: "1234.56", porque: "miles con punto y decimal con coma" },
    { entrada: "1,234.56", api: "1234.56", porque: "miles con coma y decimal con punto" },
    { entrada: "1234.56", api: "1234.56", porque: "decimal plano" },
    {
      entrada: "480.000",
      api: "480000.00",
      porque: "tres dígitos después del separador = miles, no decimales",
    },
    { entrada: "480.00", api: "480.00", porque: "dos dígitos después del separador = decimales" },
    { entrada: "480.0", api: "480.00", porque: "un dígito después del separador = decimales" },
    { entrada: "₡ 45 200,00", api: "45200.00", porque: "con símbolo de moneda pegado" },
    { entrada: "0", api: "0.00", porque: "cero es un saldo válido" },
    { entrada: "", api: "", porque: "vacío queda vacío: no es lo mismo que cero" },
    { entrada: "   ", api: "", porque: "solo espacios es vacío" },
  ];

  it.each(casos)("«$entrada» -> $api ($porque)", ({ entrada, api }) => {
    expect(montoParaApi(entrada)).toBe(api);
  });

  it("lo que se reformatea en pantalla se puede volver a parsear sin perder el monto", () => {
    // Es el ciclo real: se teclea, el campo se reescribe legible al salir, y eso es lo que viaja.
    for (const { entrada, api } of casos) {
      if (api === "") continue;
      const legible = montoLegible(entrada);
      expect(montoParaApi(legible)).toBe(api);
    }
  });

  it("trabaja en céntimos enteros, sin arrastre de punto flotante", () => {
    expect(montoACentimos("0.07")).toBe(7);
    expect(montoACentimos("1234.56")).toBe(123456);
    // 8.16 * 100 en coma flotante da 815.9999...; en céntimos tiene que dar 816 exacto.
    expect(montoACentimos("8.16")).toBe(816);
  });
});

describe("formatMoneda", () => {
  it("distingue las dos monedas del sistema", () => {
    const crc = formatMoneda("152345000.50", "CRC");
    const usd = formatMoneda("152345000.50", "USD");
    expect(crc).not.toBe(usd);
    // Lo que importa es que la moneda esté a la vista: sin marca, ₡152 millones y USD 152
    // millones se leen igual. En es-CR el colón sale como «₡» y el dólar como «USD» (no «$»).
    expect(crc).toMatch(/₡/);
    expect(usd).toMatch(/USD/);
  });

  it("un monto nulo o vacío no rompe la pantalla", () => {
    expect(() => formatMoneda(null)).not.toThrow();
    expect(() => formatMoneda(undefined)).not.toThrow();
    expect(() => formatMoneda("")).not.toThrow();
  });
});

/**
 * EL DESFASE SIN MINUTOS (10 de setiembre de 2026).
 *
 * El backend formatea 17 endpoints con `to_char(..., 'YYYY-MM-DD"T"HH24:MI:SSOF')`, y el patrón
 * `OF` de Postgres emite el desfase SIN minutos cuando son cero: «+00», «-06». Eso no es ISO 8601
 * válido y `new Date()` devuelve NaN, así que la pantalla mostraba «nunca» sobre un dato que sí
 * existía —el síntoma peor posible: no es un error, es un dato que se lee como ausente—.
 *
 * También cubre el desfase con minutos y la «Z», para que arreglar esto no rompa lo que ya andaba.
 */
describe("aFechaHora", () => {
  it("parsea el desfase sin minutos que emite Postgres", () => {
    expect(aFechaHora("2026-09-10T18:50:23+00")?.toISOString()).toBe("2026-09-10T18:50:23.000Z");
    expect(aFechaHora("2026-09-10T18:50:23-06")?.toISOString()).toBe("2026-09-11T00:50:23.000Z");
  });

  it("sigue parseando el ISO completo y la Z", () => {
    expect(aFechaHora("2026-09-10T18:50:23+00:00")?.toISOString()).toBe("2026-09-10T18:50:23.000Z");
    expect(aFechaHora("2026-09-10T18:50:23-06:00")?.toISOString()).toBe("2026-09-11T00:50:23.000Z");
    expect(aFechaHora("2026-09-10T18:50:23Z")?.toISOString()).toBe("2026-09-10T18:50:23.000Z");
  });

  it("devuelve null —y no una fecha inventada— con lo que no se entiende", () => {
    // Importa que sea null y no new Date(0): un cero se leería como 1970 y parecería un dato real.
    for (const x of ["", "   ", "no es fecha", null, undefined]) {
      expect(aFechaHora(x)).toBeNull();
    }
  });
});
