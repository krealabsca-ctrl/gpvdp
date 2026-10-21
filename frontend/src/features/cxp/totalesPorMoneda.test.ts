import { describe, expect, it } from "vitest";
import { totalesPorMoneda } from "./dominio";

// EL PROBLEMA (Director Financiero, 16-oct-2026): «las macros de dólares, a pesar de estar en
// dólares, muestran el tema en colones».
//
// La barra del corte y el encabezado del lote sumaban `total_crc` y lo rotulaban ₡ siempre, así
// que un corte de USD 384,20 se anunciaba como «₡174 015,71» mientras la fila de abajo, en la
// misma pantalla, decía «USD 384,20». El archivo que se sube al banco va en dólares: el número que
// se lee antes de subirlo tiene que ir en dólares también.

describe("totalesPorMoneda", () => {
  it("un corte en dólares se anuncia en dólares, con el equivalente al lado", () => {
    const [usd, ...resto] = totalesPorMoneda([
      { moneda: "USD", total: "384.20", total_crc: "174015.71" },
    ]);
    expect(resto).toHaveLength(0);
    expect(usd!.moneda).toBe("USD");
    // El titular es el monto en dólares, no el de colones.
    expect(usd!.total).toBeCloseTo(384.2, 2);
    // Y el equivalente sigue disponible: hace falta para cuadrar contra el presupuesto.
    expect(usd!.totalCRC).toBeCloseTo(174015.71, 2);
  });

  it("un corte mixto NO suma peras con manzanas", () => {
    const grupos = totalesPorMoneda([
      { moneda: "CRC", total: "100000", total_crc: "100000" },
      { moneda: "USD", total: "200", total_crc: "90000" },
      { moneda: "USD", total: "100", total_crc: "45000" },
    ]);
    expect(grupos).toHaveLength(2);
    // Colones primero: es la moneda de casi toda la operación.
    expect(grupos[0]!.moneda).toBe("CRC");
    expect(grupos[0]!.total).toBeCloseTo(100000, 2);
    expect(grupos[1]!.moneda).toBe("USD");
    expect(grupos[1]!.total).toBeCloseTo(300, 2);
    expect(grupos[1]!.totalCRC).toBeCloseTo(135000, 2);
  });

  it("una factura sin moneda declarada cuenta como colones", () => {
    // Así nacieron las 4.542 que vinieron por Excel. Tratarlas como desconocidas las dejaría
    // fuera de los dos grupos y el total anunciado sería menor que el que se paga.
    const grupos = totalesPorMoneda([
      { total: "50000", total_crc: "50000" },
      { moneda: "  ", total: "25000", total_crc: "25000" },
      { moneda: "crc", total: "25000", total_crc: "25000" },
    ]);
    expect(grupos).toHaveLength(1);
    expect(grupos[0]!.moneda).toBe("CRC");
    expect(grupos[0]!.total).toBeCloseTo(100000, 2);
  });

  it("sin facturas no inventa un grupo", () => {
    // «No hay nada que pagar» y «hay un total en cero» son cosas distintas.
    expect(totalesPorMoneda([])).toHaveLength(0);
  });

  it("minúsculas y espacios no son otra moneda", () => {
    const grupos = totalesPorMoneda([
      { moneda: "usd", total: "100", total_crc: "45000" },
      { moneda: " USD ", total: "100", total_crc: "45000" },
    ]);
    expect(grupos).toHaveLength(1);
    expect(grupos[0]!.total).toBeCloseTo(200, 2);
  });
});
