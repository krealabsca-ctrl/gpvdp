import { describe, expect, it } from "vitest";
import { fechaISOValida, revisarRango } from "@/features/bancos/rangoFechas";

/**
 * EL FILTRO DE FECHAS DE «MI PARTIDA» (21 de setiembre de 2026).
 *
 * Lo que se prueba acá no es cosmético. Cada caso es un valor que la pantalla llegó a mandar de
 * verdad y que terminó en 500 «error interno» o —peor— en 200 con la lista equivocada.
 */
describe("fechaISOValida", () => {
  it("acepta una fecha ISO que existe", () => {
    expect(fechaISOValida("2026-09-01")).toBe(true);
    expect(fechaISOValida("2024-02-29")).toBe(true); // bisiesto
  });

  // El año a medio teclear del selector de Chrome: rellena con ceros a la izquierda y dispara
  // onChange en cada tecla. «0000-09-01» reventaba Postgres; «0202-09-01» devolvía TODO el
  // histórico en silencio, que es el caso peligroso porque no hay error que lo delate.
  it("rechaza el año a medio escribir que emite el navegador", () => {
    expect(fechaISOValida("0000-09-01")).toBe(false);
    expect(fechaISOValida("0002-09-01")).toBe(false);
    expect(fechaISOValida("0020-09-01")).toBe(false);
    expect(fechaISOValida("0202-09-01")).toBe(false);
  });

  it("rechaza el día que no existe en el calendario", () => {
    expect(fechaISOValida("2026-02-31")).toBe(false);
    expect(fechaISOValida("2026-13-01")).toBe(false);
    expect(fechaISOValida("2026-00-10")).toBe(false);
    expect(fechaISOValida("2025-02-29")).toBe(false); // 2025 no es bisiesto
  });

  // Postgres SÍ entiende estos, y los entiende distinto: con DateStyle=MDY «01/09/2026» es el 9 de
  // enero. Se aceptaban en silencio y el filtro medía otro rango.
  it("rechaza lo que no es ISO aunque Postgres lo entienda", () => {
    expect(fechaISOValida("01/09/2026")).toBe(false);
    expect(fechaISOValida("2026-09")).toBe(false);
    expect(fechaISOValida("2026")).toBe(false);
    expect(fechaISOValida("abc")).toBe(false);
  });
});

describe("revisarRango", () => {
  it("sin fechas no filtra y no se queja", () => {
    expect(revisarRango("", "")).toEqual({
      desde: undefined,
      hasta: undefined,
      errorDesde: "",
      errorHasta: "",
      hayError: false,
    });
  });

  it("deja pasar el rango bien escrito", () => {
    const r = revisarRango("2026-09-01", "2026-09-30");
    expect(r.hayError).toBe(false);
    expect(r.desde).toBe("2026-09-01");
    expect(r.hasta).toBe("2026-09-30");
  });

  it("acepta una sola punta del rango", () => {
    expect(revisarRango("2026-09-01", "")).toMatchObject({
      desde: "2026-09-01",
      hasta: undefined,
      hayError: false,
    });
    expect(revisarRango("", "2026-09-30")).toMatchObject({
      desde: undefined,
      hasta: "2026-09-30",
      hayError: false,
    });
  });

  // La regla que importa: si UNA fecha está mal, no se manda NINGUNA. Aplicar media fecha devuelve
  // una lista que no es la que el usuario pidió, y acá una lista es una afirmación sobre el dinero.
  it("con una fecha mala no manda ninguna de las dos", () => {
    const r = revisarRango("0002-09-01", "2026-09-30");
    expect(r.hayError).toBe(true);
    expect(r.desde).toBeUndefined();
    expect(r.hasta).toBeUndefined();
    expect(r.errorDesde).toContain("año");
    expect(r.errorHasta).toBe("");
  });

  it("marca el campo que está mal, no el otro", () => {
    const r = revisarRango("2026-09-01", "2026-02-31");
    expect(r.hayError).toBe(true);
    expect(r.errorDesde).toBe("");
    expect(r.errorHasta).not.toBe("");
  });

  // Antes esto salía 200 con cero filas y la pantalla lo explicaba como «ningún movimiento de tu
  // partida coincide», o sea: «no entró la plata». Son dos cosas MUY distintas.
  it("avisa el rango invertido en vez de devolver una lista vacía", () => {
    const r = revisarRango("2026-09-30", "2026-09-01");
    expect(r.hayError).toBe(true);
    expect(r.errorHasta).toContain("anterior");
    expect(r.desde).toBeUndefined();
    expect(r.hasta).toBeUndefined();
  });

  it("el mismo día en las dos puntas es un rango válido", () => {
    expect(revisarRango("2026-09-11", "2026-09-11").hayError).toBe(false);
  });
});
