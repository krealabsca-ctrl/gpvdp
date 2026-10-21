import { describe, it, expect } from "vitest";
import {
  MODULES,
  migasDeRuta,
  paginaActiva,
  permisoDePagina,
  permisoDeRuta,
  todasLasPaginas,
} from "@/app/nav";

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

  it("separa la bandeja de recepción de la configuración de buzones", () => {
    // Son dos permisos DISTINTOS a propósito: ver lo que llegó es trabajo de operación, mientras
    // dar de alta un buzón crea una CREDENCIAL y decide de qué empresa son las facturas que
    // entran. Si las dos rutas resolvieran al mismo permiso, cualquiera que revisa la cola de
    // errores podría emitir credenciales.
    expect(permisoDeRuta("/cxp/recepcion")).toBe("cxp.recepcion");
    expect(permisoDeRuta("/cxp/fuentes")).toBe("cxp.fuentes");
    // Y ninguna de las dos se come la de importar, que comparte el módulo.
    expect(permisoDeRuta("/cxp/importar")).toBe("cxp.importar");
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

  it("separa el correo saliente del resto de Configuración", () => {
    // El sobre y la carta son dos permisos DISTINTOS: en Notificaciones se edita TEXTO, mientras
    // que en Correo saliente se guarda la CREDENCIAL del correo corporativo de la empresa. Si las
    // dos rutas resolvieran al mismo permiso, quien redacta los correos podría cambiar el buzón
    // desde el que salen. Y ninguna de las dos puede heredar `admin.roles` del módulo, que abriría
    // la pantalla a quien administra usuarios.
    expect(permisoDeRuta("/correo-saliente")).toBe("admin.correo");
    expect(permisoDeRuta("/notificaciones")).toBe("admin.plantillas");
    expect(permisoDeRuta("/usuarios")).toBe("admin.roles");
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

describe("migasDeRuta", () => {
  it("ninguna página del registro se queda SIN migas", () => {
    // La versión anterior enumeraba 14 rutas a mano y dejaba sin migas a Inventario, RRHH, CxC,
    // Grupo y Configuración. Con la sidebar colapsable las migas pasan a ser la única orientación,
    // así que una página sin ellas deja al usuario sin saber dónde está.
    const sinMigas: string[] = [];
    for (const m of MODULES.filter((x) => x.disponible)) {
      for (const p of m.pages) {
        if (p.to === "#") continue;
        const migas = migasDeRuta(p.to);
        if (!migas.modulo || !migas.pagina) sinMigas.push(`${m.label} › ${p.label} (${p.to})`);
      }
    }
    expect(sinMigas, `estas páginas no tienen migas: ${sinMigas.join(", ")}`).toEqual([]);
  });

  it("la miga nombra la página MÁS específica, no la que es prefijo", () => {
    // /precios/insumos no puede mostrar «Costeo del plan»: sería decir que estás en otro lado.
    //
    // Hoy los 16 pares padre/hija del registro tienen al padre marcado `end: true`, así que es ESE
    // flag el que descarta al padre y no la comparación de longitud. La longitud sigue en el
    // código como defensa para cuando aparezca una ruta anidada sin `end`; mutarla no pone roja
    // esta prueba, mutar el manejo de `end` sí.
    for (const m of MODULES.filter((x) => x.disponible)) {
      for (const hija of m.pages) {
        if (hija.to === "/" || hija.to === "#") continue;
        const padre = m.pages.find((p) => p !== hija && p.to !== "/" && hija.to.startsWith(p.to));
        if (!padre) continue;
        expect(
          migasDeRuta(hija.to).pagina,
          `«${hija.to}» muestra la miga de «${padre.to}»`,
        ).toBe(hija.label);
      }
    }
  });
});

describe("auto-colapso de la sidebar", () => {
  it("las pantallas marcadas anchoCompleto son páginas reales del registro", () => {
    // La marca vive en el registro para que no haya una lista de rutas aparte que se desincronice.
    const anchas = MODULES.filter((x) => x.disponible)
      .flatMap((m) => m.pages.filter((p) => p.anchoCompleto).map((p) => `${m.label} › ${p.label}`));
    expect(anchas.length, "nadie marcó pantallas anchas: el auto-colapso no haría nada").toBeGreaterThan(0);
  });

  it("la marca se resuelve por ruta, también en las rutas anidadas", () => {
    // paginaActiva es lo que consulta la Sidebar; si no resolviera la ruta exacta, el auto-colapso
    // se dispararía en la pantalla equivocada.
    for (const m of MODULES.filter((x) => x.disponible)) {
      for (const p of m.pages.filter((x) => x.anchoCompleto)) {
        expect(paginaActiva(p.to).page?.anchoCompleto, `«${p.to}» perdió su marca`).toBe(true);
      }
    }
  });
});
