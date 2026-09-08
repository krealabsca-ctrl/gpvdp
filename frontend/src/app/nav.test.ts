import { describe, it, expect } from "vitest";
import { MODULES, permisoDePagina, permisoDeRuta, todasLasPaginas } from "@/app/nav";

/**
 * El registro de navegación ES el gate: `PermisoGate` decide qué puede abrir cada usuario
 * llamando a `permisoDeRuta`. Una entrada mal escrita no falla ruidosamente —abre una pantalla a
 * quien no debería, o le cierra la puerta a quien sí—, así que las rutas con permiso propio se
 * verifican acá.
 */
describe("permisoDeRuta", () => {
  it("gatea el catálogo de gasto de CxP con su propio permiso", () => {
    // La segunda puerta al catálogo: Contabilidad abre rubros sin `bancos.catalogo`.
    expect(permisoDeRuta("/cxp/catalogo")).toBe("cxp.catalogo");
  });

  it("gatea la consulta por segmento con su propio permiso", () => {
    // La pantalla del equipo de una partida: es lo ÚNICO que abre ese rol del módulo Bancos.
    expect(permisoDeRuta("/mi-segmento")).toBe("bancos.ver_mi_segmento");
  });

  it("no se come rutas vecinas que comparten prefijo", () => {
    expect(permisoDeRuta("/cxp/cajas")).toBe("cxp.caja_ver");
    expect(permisoDeRuta("/cxp/departamentos")).toBe("cxp.departamentos");
    expect(permisoDeRuta("/cxp/contabilidad")).toBe("cxp.marcar_contabilidad");
    expect(permisoDeRuta("/cxp/validacion")).toBe("cxp.parametros");
  });

  it("con rutas anidadas gana la MÁS ESPECÍFICA, no la primera del registro", () => {
    // `/inventario` es prefijo de `/inventario/entradas`. Resolviendo por la primera coincidencia,
    // la pantalla de entradas heredaba `inventario.ver`: cualquiera que viera existencias la abría,
    // y recién ahí chocaba con los 403 del servidor.
    expect(permisoDeRuta("/inventario")).toBe("inventario.ver");
    expect(permisoDeRuta("/inventario/entradas")).toBe("inventario.entrada");
  });

  it("cae al permiso del módulo cuando la página no declara uno", () => {
    expect(permisoDeRuta("/cxp/bandeja")).toBe("cxp.ver");
  });

  it("toda página anidada resuelve a SU permiso y no al de su ruta padre", () => {
    // Barrido de todo el registro: por cada par de rutas donde una es prefijo de la otra, la más
    // larga tiene que quedarse con su propio permiso. Es el caso que hay que sostener cada vez que
    // alguien agregue una subpágina.
    for (const m of MODULES.filter((x) => x.disponible)) {
      for (const hija of m.pages) {
        if (hija.to === "/" || hija.to === "#") continue;
        const padre = m.pages.find((p) => p !== hija && p.to !== "/" && hija.to.startsWith(p.to));
        if (!padre) continue;
        expect(
          permisoDeRuta(hija.to),
          `«${hija.to}» está heredando el permiso de «${padre.to}»`,
        ).toBe(permisoDePagina(m, hija));
      }
    }
  });
});

describe("todasLasPaginas", () => {
  it("expone el catálogo de gasto con su permiso efectivo (command palette)", () => {
    const pagina = todasLasPaginas().find((p) => p.to === "/cxp/catalogo");
    expect(pagina?.permisoEfectivo).toBe("cxp.catalogo");
  });
});
