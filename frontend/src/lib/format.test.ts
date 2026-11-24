import { describe, expect, it } from "vitest";

import {
  aFechaHora,
  finDeMes,
  formatMoneda,
  formatTipoCambio,
  montoACentimos,
  montoLegible,
  montoParaApi,
  periodosDelSelector,
} from "./format";

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

/**
 * `finDeMes` convierte un mes en un rango cerrado: lo usa el salto de «está en otro mes» de «Mi
 * partida» (2-oct-2026).
 *
 * El filtro de MONTO de esa misma pantalla NO trae helper nuevo: usa `montoParaApi`, el de arriba.
 * Inventar un normalizador por pantalla es exactamente lo que causó el bug de los saldos.
 */
describe("finDeMes", () => {
  it("meses de 31, de 30 y febrero", () => {
    expect(finDeMes("2026-01")).toBe("2026-01-31");
    expect(finDeMes("2026-04")).toBe("2026-04-30");
    expect(finDeMes("2026-02")).toBe("2026-02-28");
  });

  it("un año bisiesto da 29", () => {
    // Si alguien lo reescribiera con una tabla de 30/31, este caso se pierde.
    expect(finDeMes("2028-02")).toBe("2028-02-29");
  });

  it("no se corre un día por la zona horaria", () => {
    // Armado con toISOString() daría el día anterior en Costa Rica (UTC−6), y el rango dejaría
    // afuera justo el último día del mes al que se saltó.
    expect(finDeMes("2026-12")).toBe("2026-12-31");
  });
});

describe("formatTipoCambio", () => {
  /**
   * Los dos extremos tienen razones distintas y las dos salen de la operación real.
   *
   * ARRIBA: el TC del mes es el PROMEDIO de tres cotizaciones y casi nunca es exacto. Las de
   * octubre 2026 dan (460,73 + 453,25 + 456,04) / 3 = 456,673333…, y recortar eso descuadra cada
   * monto en dólares del mes contra el estado de cuenta: es el defecto que motivó la mig 0119.
   *
   * ABAJO: la base devuelve el valor sin ceros de relleno, así que un TC de 452,60 llega como
   * «452.6». Un tipo de cambio con un solo decimal se lee como un error de carga.
   */
  it("conserva hasta seis decimales cuando el promedio los tiene", () => {
    expect(formatTipoCambio("456.673333")).toBe("456,673333");
  });

  it("rellena hasta dos decimales cuando el valor viene corto", () => {
    expect(formatTipoCambio("452.6")).toBe("452,60");
    expect(formatTipoCambio("500")).toBe("500,00");
  });

  it("deja los decimales intermedios como están", () => {
    expect(formatTipoCambio("460.73")).toBe("460,73");
    expect(formatTipoCambio("454.8167")).toBe("454,8167");
  });

  it("no inventa un cero cuando no hay tipo de cambio", () => {
    // Un mes sin congelar NO tiene TC; mostrar «0,00» diría que el dólar no vale nada.
    expect(formatTipoCambio(null)).toBe("—");
    expect(formatTipoCambio("")).toBe("—");
    expect(formatTipoCambio(undefined)).toBe("—");
  });
});

/**
 * La lista del selector de período (16-nov-2026).
 *
 * EL DEFECTO QUE ESTO FIJA: la lista se armaba contando 18 meses hacia atrás desde el reloj, sin
 * mirar la base. A la empresa más antigua del grupo —21 meses cargados desde enero 2025— le
 * escondía enero a mayo 2025, y a las otras dos —3 meses cargados— les ofrecía 15 meses vacíos.
 */
describe("periodosDelSelector", () => {
  // EL CASO REPORTADO: 21 meses cargados tienen que ser alcanzables.
  it("llega hasta el primer mes cargado aunque pase de 18 meses", () => {
    const lista = periodosDelSelector("2025-01", "2026-09", "2026-11");

    // Enero 2025 a noviembre 2026 son 23 meses, no 18.
    expect(lista).toHaveLength(23);
    expect(lista[0]).toBe("2026-11"); // el más reciente primero
    expect(lista[lista.length - 1]).toBe("2025-01");
    // Los cinco que el tope de 18 escondía.
    for (const m of ["2025-01", "2025-02", "2025-03", "2025-04", "2025-05"]) {
      expect(lista).toContain(m);
    }
  });

  // Y EL CASO INVERSO: no ofrecer meses anteriores al primer dato.
  it("no ofrece meses anteriores al primer mes cargado", () => {
    const lista = periodosDelSelector("2026-07", "2026-09", "2026-11");

    // Julio a noviembre 2026: cinco meses, no dieciocho.
    expect(lista).toEqual(["2026-11", "2026-10", "2026-09", "2026-08", "2026-07"]);
  });

  // LLEGA HASTA HOY AUNQUE NO HAYA DATO: el mes en curso es el que más se mira, y casi nunca
  // está cargado todavía.
  it("incluye el mes de hoy aunque no tenga movimiento", () => {
    const lista = periodosDelSelector("2026-07", "2026-09", "2026-11");
    expect(lista).toContain("2026-11");
    expect(lista).toContain("2026-10");
  });

  // Y SE ESTIRA HACIA ADELANTE si alguien cargó un mes posterior a hoy.
  it("llega hasta el último mes cargado si es posterior a hoy", () => {
    const lista = periodosDelSelector("2026-07", "2026-12", "2026-11");
    expect(lista[0]).toBe("2026-12");
  });

  // SIN CALENDARIO NO SE ROMPE: empresa nueva o respuesta en camino → el respaldo de siempre.
  it("cae al respaldo cuando la empresa no tiene nada cargado", () => {
    expect(periodosDelSelector("", "", "2026-11")).toHaveLength(18);
  });

  // UN RANGO IMPOSIBLE cae al respaldo en vez de truncarse en silencio.
  it("cae al respaldo con un rango absurdo", () => {
    // Un movimiento con año 1900 por un error de carga: 126 años son 1 512 meses.
    expect(periodosDelSelector("1900-01", "1900-02", "2026-11")).toHaveLength(18);
    // Y un primero posterior al fin —datos incoherentes— tampoco arma una lista al revés.
    expect(periodosDelSelector("2027-05", "2026-01", "2026-11")).toHaveLength(18);
  });

  // UN SOLO MES CARGADO que además es hoy: una lista de uno, no una vacía.
  it("devuelve un mes cuando ese es todo el calendario", () => {
    expect(periodosDelSelector("2026-11", "2026-11", "2026-11")).toEqual(["2026-11"]);
  });

  // CRUZA EL AÑO sin saltarse diciembre ni repetir enero.
  it("cruza el cambio de año", () => {
    const lista = periodosDelSelector("2025-11", "2026-02", "2026-02");
    expect(lista).toEqual(["2026-02", "2026-01", "2025-12", "2025-11"]);
  });
});
