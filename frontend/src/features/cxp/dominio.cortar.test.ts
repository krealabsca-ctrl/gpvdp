import { describe, expect, it } from "vitest";

import { sePuedeCortar } from "./dominio";

/**
 * El 25 de setiembre de 2026, en la pestaña «Por pagar», se marcaron tres facturas APROBADAS y el
 * corte se abortó entero: las tres estaban bloqueadas para pago (viáticos ya desembolsados). El
 * servidor hizo lo correcto —no crear un lote vacío— pero la pantalla nunca debió ofrecerlas.
 *
 * Estas pruebas fijan la regla de la pantalla. La verificación de verdad sigue estando en el
 * servidor (`ProgramarAprobados` filtra `AND NOT bloqueado_para_pago`); acá se prueba que el
 * tilde no se ofrezca, que es lo que le hace perder el corte a la persona.
 */
describe("sePuedeCortar", () => {
  it("una factura bloqueada para pago no se puede cortar, aunque esté aprobada", () => {
    expect(sePuedeCortar({ bloqueado_para_pago: true })).toBe(false);
  });

  it("una factura normal sí", () => {
    expect(sePuedeCortar({ bloqueado_para_pago: false })).toBe(true);
  });

  it("sin el campo se deja cortar: la decide el servidor, no la pantalla", () => {
    // Un backend viejo no manda el campo. El default tiene que ser PERMITIR y dejar que el
    // servidor rechace con su motivo: si acá se negara por defecto, una versión desalineada
    // dejaría la pestaña entera sin poder cortar nada y sin decir por qué.
    expect(sePuedeCortar({})).toBe(true);
  });

  it("filtrar una lista deja afuera solo las bloqueadas", () => {
    const items = [
      { id: "a", bloqueado_para_pago: false },
      { id: "b", bloqueado_para_pago: true },
      { id: "c", bloqueado_para_pago: false },
    ];
    expect(items.filter(sePuedeCortar).map((d) => d.id)).toEqual(["a", "c"]);
  });
});
