import { describe, expect, it } from "vitest";
import { calcularCuadre, lineaVacia, queLeFalta } from "@/features/conta/cuadre";
import type { NuevaLineaAsiento } from "@/api/conta";

/**
 * El cuadre del asiento mientras se escribe.
 *
 * La base rechaza todo asiento descuadrado y esa es la que manda. Esto prueba otra cosa: que
 * quien carga el asiento vea la diferencia ANTES de guardar, y que el botón no se habilite sobre
 * algo que el servidor va a rechazar.
 */

function l(cuenta: string, debe: string, haber: string): NuevaLineaAsiento {
  return { cuenta_id: cuenta, debe, haber, detalle: "" };
}

describe("calcularCuadre", () => {
  it("cuadra cuando el debe y el haber dan lo mismo", () => {
    const c = calcularCuadre([l("c1", "1000", ""), l("c2", "", "1000")]);
    expect(c.cuadra).toBe(true);
    expect(c.diferencia).toBe(0);
    expect(c.debeTexto).toBe("1000.00");
  });

  it("dice de cuánto es la diferencia y para qué lado", () => {
    const c = calcularCuadre([l("c1", "1000", ""), l("c2", "", "900")]);
    expect(c.cuadra).toBe(false);
    expect(c.diferencia).toBe(10000); // céntimos: sobran 100,00 al debe
    expect(c.diferenciaTexto).toBe("100.00");
  });

  // LA RAZÓN DE CONTAR EN CÉNTIMOS. En coma flotante 0.1 + 0.2 da 0.30000000000000004, así que
  // este asiento marcaría descuadre sin estarlo y nadie entendería por qué.
  it("cuadra con decimales que en coma flotante no cuadrarían", () => {
    const c = calcularCuadre([l("c1", "0,10", ""), l("c2", "0,20", ""), l("c3", "", "0,30")]);
    expect(c.cuadra).toBe(true);
    expect(c.diferencia).toBe(0);
  });

  // Un formulario en blanco cuadra en cero, que no es lo mismo que cuadrar. Sin esta condición
  // el botón de guardar se habilitaría sobre nada.
  it("un asiento vacío no cuadra", () => {
    const c = calcularCuadre([lineaVacia(), lineaVacia()]);
    expect(c.cuadra).toBe(false);
    expect(c.conMonto).toBe(0);
  });

  it("una sola línea con monto no cuadra aunque la suma dé cero", () => {
    expect(calcularCuadre([l("c1", "0", ""), l("c2", "", "0")]).cuadra).toBe(false);
  });

  it("varias líneas contra una sí cuadran", () => {
    // El pago de una factura con retención: un debe contra dos haberes.
    const c = calcularCuadre([l("c1", "1000", ""), l("c2", "", "980"), l("c3", "", "20")]);
    expect(c.cuadra).toBe(true);
  });
});

describe("queLeFalta", () => {
  const ok = [l("c1", "1000", ""), l("c2", "", "1000")];

  it("no dice nada cuando está listo", () => {
    expect(queLeFalta("2026-10-27", "compra de papelería", ok, calcularCuadre(ok))).toBe("");
  });

  // El orden importa: UNA cosa a la vez, la primera que falta. Una lista de cinco problemas
  // sobre un formulario a medio llenar no se lee.
  it("pide la fecha antes que nada", () => {
    expect(queLeFalta("", "", [], calcularCuadre([]))).toMatch(/fecha/i);
  });

  it("pide la glosa y dice para qué sirve", () => {
    expect(queLeFalta("2026-10-27", "   ", ok, calcularCuadre(ok))).toMatch(/glosa/i);
  });

  it("avisa de una línea con monto y sin cuenta", () => {
    const lineas = [l("", "1000", ""), l("c2", "", "1000")];
    expect(queLeFalta("2026-10-27", "compra", lineas, calcularCuadre(lineas))).toMatch(/sin cuenta/i);
  });

  it("avisa de una línea que va al debe y al haber", () => {
    const lineas = [l("c1", "500", "500"), l("c2", "", "1000")];
    expect(queLeFalta("2026-10-27", "compra", lineas, calcularCuadre(lineas))).toMatch(/no a los dos/i);
  });

  // El mensaje tiene que decir CUÁNTO y para qué lado: «no cuadra» a secas obliga a sumar a mano.
  it("dice cuánto sobra y de qué lado", () => {
    const lineas = [l("c1", "1000", ""), l("c2", "", "900")];
    const msg = queLeFalta("2026-10-27", "compra", lineas, calcularCuadre(lineas));
    expect(msg).toContain("100.00");
    expect(msg).toMatch(/al debe/);
  });

  it("y al revés cuando sobra del otro lado", () => {
    const lineas = [l("c1", "900", ""), l("c2", "", "1000")];
    expect(queLeFalta("2026-10-27", "compra", lineas, calcularCuadre(lineas))).toMatch(/al haber/);
  });
});
