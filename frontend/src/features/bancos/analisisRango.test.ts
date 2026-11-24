import { describe, expect, it } from "vitest";
import {
  calcularConcentracion,
  calcularPuente,
  desdeDelTramo,
  mesesEntre,
  partidasQueExplican,
  problemaDelRango,
  restarMeses,
} from "@/features/bancos/analisisRango";
import type { TendenciaPartida } from "@/api/bancos";

/**
 * El rango del análisis y la concentración.
 *
 * Son las dos piezas donde un error no se ve: un rango corrido un mes y una concentración mal
 * acumulada producen tablas que parecen correctas y se llevan a una reunión.
 */

describe("desdeDelTramo", () => {
  it("el trimestre son tres meses contando el final", () => {
    // Marzo hacia atrás: enero, febrero, marzo. No «marzo menos tres».
    expect(desdeDelTramo(3, "2026-03", "")).toBe("2026-01");
    expect(mesesEntre(desdeDelTramo(3, "2026-03", ""), "2026-03")).toBe(3);
  });

  it("el trimestre cruza el fin de año sin saltarse un mes", () => {
    // De enero hacia atrás: noviembre, diciembre, enero.
    expect(desdeDelTramo(3, "2026-01", "")).toBe("2025-11");
    expect(mesesEntre("2025-11", "2026-01")).toBe(3);
  });

  // EL AÑO EN CURSO NO SON DOCE MESES HACIA ATRÁS. Es de enero al mes elegido, y confundirlos
  // daría un «acumulado del año» que arranca en el año pasado.
  it("el año en curso arranca en enero del año del mes final", () => {
    expect(desdeDelTramo("YTD", "2026-09", "")).toBe("2026-01");
    expect(desdeDelTramo("YTD", "2026-01", "")).toBe("2026-01");
    expect(desdeDelTramo(12, "2026-09", "")).toBe("2025-10"); // doce meses sí retrocede de año
  });

  it("el año en curso en enero es un solo mes", () => {
    expect(mesesEntre(desdeDelTramo("YTD", "2026-01", ""), "2026-01")).toBe(1);
  });

  it("el personalizado respeta lo que se eligió", () => {
    expect(desdeDelTramo("LIBRE", "2026-09", "2025-04")).toBe("2025-04");
  });

  // El tramo se conserva al mover el mes final: con `desde` guardado suelto, pasar de marzo a
  // septiembre dejaría nueve meses mientras el botón sigue diciendo «Trimestre».
  it("mover el mes final conserva el largo del tramo", () => {
    expect(mesesEntre(desdeDelTramo(3, "2026-03", ""), "2026-03")).toBe(3);
    expect(mesesEntre(desdeDelTramo(3, "2026-09", ""), "2026-09")).toBe(3);
  });
});

describe("restarMeses", () => {
  it("retrocede dentro del año y cruzando el año", () => {
    expect(restarMeses("2026-09", 0)).toBe("2026-09");
    expect(restarMeses("2026-09", 5)).toBe("2026-04");
    expect(restarMeses("2026-02", 3)).toBe("2025-11");
    expect(restarMeses("2026-01", 1)).toBe("2025-12");
    expect(restarMeses("2026-01", 13)).toBe("2024-12");
  });
});

describe("mesesEntre", () => {
  it("cuenta meses de calendario con las dos puntas", () => {
    expect(mesesEntre("2026-09", "2026-09")).toBe(1);
    expect(mesesEntre("2026-01", "2026-03")).toBe(3);
    // 24 meses exactos: el tope del endpoint tiene que entrar, no quedar en 25.
    expect(mesesEntre("2024-10", "2026-09")).toBe(24);
  });
});

describe("problemaDelRango", () => {
  it("acepta un rango válido", () => {
    expect(problemaDelRango("2026-01", "2026-03")).toBe("");
    expect(problemaDelRango("2024-10", "2026-09")).toBe(""); // 24 justos
  });

  it("rechaza el rango invertido", () => {
    expect(problemaDelRango("2026-09", "2026-01")).toMatch(/posterior/i);
  });

  it("rechaza pasarse del tope y dice de cuánto es", () => {
    const msg = problemaDelRango("2024-09", "2026-09"); // 25
    expect(msg).toContain("25");
    expect(msg).toContain("24");
  });
});

function partida(nombre: string, total: string): TendenciaPartida {
  return {
    concepto_id: "co1",
    concepto: "Gastos",
    clasificacion_id: nombre,
    clasificacion: nombre,
    naturaleza: "GASTO",
    naturaleza_declarada: true,
    serie: [],
    total,
    promedio: "0",
    ultimo: "0",
    desvio_pct: "0",
    meses_con_dato: 3,
    confiable: true,
    mediana: "0",
    desvio_mediana_pct: "0",
    meses_con_gasto: 3,
    recurrencia: "FIJO",
  };
}

describe("calcularConcentracion", () => {
  // EL CASO REAL, con la forma de Memorial Pets: Planilla se lleva casi un tercio y la cola es
  // larga. La tabla principal ordena por desvío, así que esto es lo único que muestra el peso.
  const reales = [
    partida("Gas", "18926162"),
    partida("Planilla", "89399096"),
    partida("Mercadeo", "16194236"),
    partida("Operaciones Bancarias", "31184785"),
    partida("CCSS", "20415638"),
  ];

  it("ordena de mayor a menor aunque lleguen desordenadas", () => {
    const c = calcularConcentracion(reales);
    expect(c.map((f) => f.p.clasificacion)).toEqual([
      "Planilla",
      "Operaciones Bancarias",
      "CCSS",
      "Gas",
      "Mercadeo",
    ]);
  });

  it("el acumulado sube y cierra en 100 %", () => {
    const c = calcularConcentracion(reales);
    for (let i = 1; i < c.length; i++) {
      expect(c[i]!.pctAcum).toBeGreaterThanOrEqual(c[i - 1]!.pctAcum);
    }
    expect(c[c.length - 1]!.pctAcum).toBeCloseTo(100, 5);
  });

  it("el porcentaje de cada una es su peso real", () => {
    const c = calcularConcentracion(reales);
    // 89.399.096 de 176.119.917 = 50,8 %
    expect(c[0]!.pct).toBeCloseTo(50.8, 1);
  });

  // Sin partidas, o con todo en cero, no se divide entre cero ni se inventa un 100 %.
  it("aguanta la lista vacía y los totales en cero", () => {
    expect(calcularConcentracion([])).toEqual([]);
    const c = calcularConcentracion([partida("Nada", "0"), partida("Tampoco", "0")]);
    expect(c.every((f) => f.pct === 0 && f.pctAcum === 0)).toBe(true);
  });
});

describe("calcularPuente", () => {
  function conAnterior(nombre: string, total: string, anterior: string, nueva = false) {
    const p = partida(nombre, total);
    p.total_anterior = anterior;
    p.sin_base_anterior = nueva;
    return p;
  }

  // EL CASO QUE SE LLEVA A UNA REUNIÓN: «el gasto subió ₡12 M — ₡8 M son planilla».
  it("ordena por el aporte en plata, no por el porcentaje", () => {
    const pu = calcularPuente([
      conAnterior("Planilla", "20000000", "12000000"), // +8 M
      conAnterior("Café", "30000", "10000"), // +20 mil pero +200 %
      conAnterior("Gas", "5000000", "2000000"), // +3 M
    ]);
    expect(pu.tramos.map((t) => t.clasificacion)).toEqual(["Planilla", "Gas", "Café"]);
    expect(pu.cambio).toBe(11020000);
  });

  // Lo que más BAJÓ importa tanto como lo que más subió: ordenar por el número con signo
  // dejaría las caídas al final, fuera de la lista corta.
  it("una caída grande entra antes que una subida chica", () => {
    const pu = calcularPuente([
      conAnterior("Urnas", "1000000", "9000000"), // −8 M
      conAnterior("Gas", "3000000", "2000000"), // +1 M
    ]);
    expect(pu.tramos[0]!.clasificacion).toBe("Urnas");
    expect(pu.tramos[0]!.aporte).toBe(-8000000);
  });

  // Si las subidas y las bajadas casi se cancelan, el cambio neto es chico: dividir por el neto
  // daría porcentajes de miles por ciento. Se divide por su valor absoluto.
  it("no explota cuando el cambio neto es casi cero", () => {
    const pu = calcularPuente([
      conAnterior("Sube", "5000000", "1000000"),
      conAnterior("Baja", "1000000", "4999000"),
    ]);
    expect(pu.cambio).toBe(1000);
    expect(Number.isFinite(pu.tramos[0]!.pctDelCambio)).toBe(true);
  });

  it("con cambio cero no divide entre cero", () => {
    const pu = calcularPuente([conAnterior("Igual", "1000", "1000")]);
    expect(pu.cambio).toBe(0);
    expect(pu.tramos).toHaveLength(0); // aporte 0: no es un tramo
  });

  // Una partida nueva se marca: su aporte entero es «apareció», no «creció».
  it("marca las partidas que no existían antes", () => {
    const pu = calcularPuente([conAnterior("Mercadeo", "5000000", "0", true)]);
    expect(pu.tramos[0]!.esNueva).toBe(true);
    expect(pu.tramos[0]!.aporte).toBe(5000000);
  });

  // La lista corta no esconde el resto: lo suma y dice cuántas quedaron fuera.
  it("acumula en el resto lo que no entra en la lista corta", () => {
    const muchas = Array.from({ length: 12 }, (_, i) =>
      conAnterior(`P${i}`, String(1000 - i * 10), "0", true),
    );
    const pu = calcularPuente(muchas, 8);
    expect(pu.tramos).toHaveLength(8);
    expect(pu.restoPartidas).toBe(4);
    // Las cuatro más chicas: 920+910+900+890
    expect(pu.resto).toBe(3620);
  });

  it("la lista vacía no rompe", () => {
    const pu = calcularPuente([]);
    expect(pu.cambio).toBe(0);
    expect(pu.tramos).toEqual([]);
  });
});

describe("partidasQueExplican", () => {
  it("cuenta cuántas llegan al 80 %", () => {
    // 50 + 30 = 80 con dos; la tercera ya está de más.
    const c = calcularConcentracion([
      partida("A", "50"),
      partida("B", "30"),
      partida("C", "20"),
    ]);
    expect(partidasQueExplican(c)).toBe(2);
  });

  it("si una sola se lleva más del 80 %, es una", () => {
    const c = calcularConcentracion([partida("A", "90"), partida("B", "10")]);
    expect(partidasQueExplican(c)).toBe(1);
  });

  // Con todo en cero ninguna «explica» nada: devolver 0 haría que la frase diga «0 de 5
  // partidas explican el 80 %», que es falso y además raro.
  it("con totales en cero devuelve todas", () => {
    const c = calcularConcentracion([partida("A", "0"), partida("B", "0")]);
    expect(partidasQueExplican(c)).toBe(2);
  });

  it("la lista vacía no rompe", () => {
    expect(partidasQueExplican([])).toBe(0);
  });
});
