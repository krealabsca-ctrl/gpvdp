import { describe, expect, it } from "vitest";
import { seVaciaLaPestanaDeBanco } from "./dominio";

/**
 * EL DEFECTO (Director Financiero, 19-oct-2026): «en el flujo bancos no están apareciendo las
 * macros que hemos pagado en producción, como si desaparecen o se ocultan».
 *
 * La pestaña «En banco» mostraba «En banco: 0 · ₡0,00 — Nada pendiente en esta fase ✓», y el
 * histórico de cortes —con la macro de cada uno— vive DENTRO de esa pestaña. Como el estado vacío
 * reemplazaba la pestaña entera, marcar la última factura como pagada dejaba 64 cortes de
 * producción sin ninguna puerta en la aplicación.
 *
 * Y pasa justo cuando más se necesita: la macro se vuelve a bajar DESPUÉS de pagar, para respaldar
 * o para rehacer un envío al banco.
 */

describe("seVaciaLaPestanaDeBanco", () => {
  it("con cortes y sin pendientes, la pestaña NO se vacía: ahí están las macros", () => {
    expect(seVaciaLaPestanaDeBanco({ fase: "bco", items: 0, lotes: 36, hayFiltros: false })).toBe(false);
  });

  it("sin cortes y sin pendientes sí se vacía: no hay nada que mostrar", () => {
    expect(seVaciaLaPestanaDeBanco({ fase: "bco", items: 0, lotes: 0, hayFiltros: false })).toBe(true);
  });

  it("con FILTROS que no dan resultados, gana «sin resultados»", () => {
    // La causa del vacío es el filtro, no el estado del trabajo: mostrar el histórico ahí haría
    // creer que el filtro no se aplicó.
    expect(seVaciaLaPestanaDeBanco({ fase: "bco", items: 0, lotes: 36, hayFiltros: true })).toBe(true);
  });

  it("las demás pestañas no cambian de comportamiento", () => {
    // Solo «En banco» tiene contenido propio además de la lista. En las otras, cero facturas es
    // cero y punto.
    for (const fase of ["rec", "val", "apr", "pag", "pgd", "arc"]) {
      expect(seVaciaLaPestanaDeBanco({ fase, items: 0, lotes: 36, hayFiltros: false })).toBe(true);
    }
  });

  it("con pendientes nunca se vacía, haya cortes o no", () => {
    expect(seVaciaLaPestanaDeBanco({ fase: "bco", items: 3, lotes: 0, hayFiltros: false })).toBe(false);
    expect(seVaciaLaPestanaDeBanco({ fase: "pgd", items: 5, lotes: 0, hayFiltros: true })).toBe(false);
  });
});
