import { describe, expect, it } from "vitest";
import { TAMANOS_PAGINA, rangoVisible } from "@/components/ui/Paginador";

/**
 * EL CONTEO DEL PAGINADOR (11 de setiembre de 2026).
 *
 * El riesgo acá no es que el componente se vea feo: es que el encabezado MIENTA. Si dice
 * «Mostrando 4.401–4.500 de 4.471» nadie vuelve a creerle a ninguna otra cifra de la pantalla.
 */
describe("rangoVisible", () => {
  it("cuenta desde 1 en la primera página", () => {
    expect(rangoVisible(4471, 1, 100, 100)).toEqual({ desde: 1, hasta: 100, totalPaginas: 45 });
  });

  it("corre el rango en las páginas del medio", () => {
    expect(rangoVisible(4471, 3, 100, 100)).toEqual({ desde: 201, hasta: 300, totalPaginas: 45 });
  });

  // La trampa: la última página casi nunca viene llena. 45 × 100 = 4.500, pero solo hay 4.471.
  // Si el «hasta» se calculara como pagina × porPagina, diría 4.500 de 4.471.
  it("no infla la última página: usa lo que llegó, no lo que cabría", () => {
    expect(rangoVisible(4471, 45, 100, 71)).toEqual({ desde: 4401, hasta: 4471, totalPaginas: 45 });
  });

  it("una sola página cuando todo cabe", () => {
    expect(rangoVisible(12, 1, 50, 12)).toEqual({ desde: 1, hasta: 12, totalPaginas: 1 });
  });

  // Con 4.471 y 200 por página son 23 páginas (22 llenas + 71 sueltas), no 22.
  it("redondea las páginas hacia arriba, nunca deja un resto afuera", () => {
    expect(rangoVisible(4471, 1, 200, 200).totalPaginas).toBe(23);
    expect(rangoVisible(4471, 1, 50, 50).totalPaginas).toBe(90);
    expect(rangoVisible(201, 1, 200, 200).totalPaginas).toBe(2);
  });

  // Quedar parado en una página que ya no existe (el filtro se achicó bajo los pies): el rango
  // se apaga en 0–0 para que el componente muestre la salida en vez de un conteo inventado.
  it("página vacía: rango en cero, pero el total sigue siendo el real", () => {
    expect(rangoVisible(30, 7, 100, 0)).toEqual({ desde: 0, hasta: 0, totalPaginas: 1 });
  });

  it("sin registros nunca reporta cero páginas (no existe la página 0 de 0)", () => {
    expect(rangoVisible(0, 1, 100, 0)).toEqual({ desde: 0, hasta: 0, totalPaginas: 1 });
  });

  it("los tamaños ofrecidos son los tres que pidió el Director Financiero", () => {
    expect([...TAMANOS_PAGINA]).toEqual([50, 100, 200]);
  });
});
