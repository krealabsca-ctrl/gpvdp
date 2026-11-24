import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";

import { buscarGuias, GUIAS } from "@/features/ayuda/guias";
import { AYUDA_POR_PANTALLA, ayudaDe } from "@/features/ayuda/pantallas";

/**
 * El centinela del manual.
 *
 * EL MODO DE FALLA QUE VIGILA: alguien renombra una ruta —ya pasó con `/tesoreria`, que se llamó
 * así un día— y la ayuda de esa pantalla deja de aparecer. Nada falla, nada avisa: simplemente el
 * botón «?» desaparece y nadie lo nota, porque quien renombra la ruta no es quien lee la ayuda.
 *
 * Por eso esta prueba LEE EL ROUTER, no una lista a mano. Es el mismo criterio que la prueba de
 * permisos del backend: una lista paralela se desincroniza, un centinela que lee el fuente no.
 */

const ROUTER = readFileSync("src/routes/router.tsx", "utf8");

/** Las rutas declaradas en el router, tal como se escriben: `{ path: "cxp/bandeja", …` */
const RUTAS_DEL_ROUTER = new Set(
  [...ROUTER.matchAll(/path:\s*"([^"]+)"/g)].map((m) => "/" + m[1]!.replace(/^\//, "")),
);

describe("el manual no se queda huérfano", () => {
  // Control: si el patrón dejara de reconocer las rutas, las dos pruebas de abajo pasarían sin
  // mirar nada.
  it("el centinela está leyendo el router de verdad", () => {
    expect(RUTAS_DEL_ROUTER.size).toBeGreaterThan(50);
    expect(RUTAS_DEL_ROUTER.has("/cxp/bandeja")).toBe(true);
  });

  it("toda pantalla con ayuda apunta a una ruta que existe", () => {
    const huerfanas = AYUDA_POR_PANTALLA.filter((a) => !RUTAS_DEL_ROUTER.has(a.ruta)).map((a) => a.ruta);
    expect(huerfanas, "la ayuda de estas pantallas no se va a mostrar nunca").toEqual([]);
  });

  it("toda ruta nombrada por una guía existe", () => {
    const huerfanas = GUIAS.flatMap((g) => g.rutas).filter((r) => !RUTAS_DEL_ROUTER.has(r));
    expect(huerfanas, "estas rutas del manual ya no existen").toEqual([]);
  });

  // Un enlace «Ver la guía completa» que no lleva a ninguna parte es peor que no tenerlo: el
  // usuario hace clic esperando más y recibe una pantalla que no habla de lo suyo.
  it("toda guía referida desde una pantalla existe", () => {
    const ids = new Set(GUIAS.map((g) => g.id));
    const rotas = AYUDA_POR_PANTALLA.filter((a) => a.guia && !ids.has(a.guia)).map((a) => a.guia);
    expect(rotas, "estos enlaces a la guía completa no llevan a ninguna parte").toEqual([]);
  });

  it("no hay dos pantallas con la misma ruta", () => {
    const vistas = new Set<string>();
    const repetidas: string[] = [];
    for (const a of AYUDA_POR_PANTALLA) {
      if (vistas.has(a.ruta)) repetidas.push(a.ruta);
      vistas.add(a.ruta);
    }
    // `ayudaDe` devuelve la PRIMERA: la segunda nunca se vería y nadie sabría por qué.
    expect(repetidas).toEqual([]);
  });

  it("no hay dos guías con el mismo id", () => {
    expect(new Set(GUIAS.map((g) => g.id)).size).toBe(GUIAS.length);
  });
});

describe("la ayuda llega donde se la busca", () => {
  it("las pantallas del cierre de mes tienen la suya", () => {
    // Son las cuatro que el manual identifica como el trabajo que se repite todos los meses. Si
    // alguna se queda sin ayuda, la capacitación vuelve a ser en persona justo donde más duele.
    for (const ruta of ["/importador", "/clasificar", "/saldos-diarios", "/cxp/arqueo"]) {
      expect(ayudaDe(ruta), `falta la ayuda de ${ruta}`).toBeDefined();
    }
  });

  it("una pantalla sin ayuda escrita no devuelve nada", () => {
    // El botón «?» se dibuja solo si hay algo. Un panel vacío enseña a no hacer clic.
    expect(ayudaDe("/una-pantalla-que-no-existe")).toBeUndefined();
  });

  it("cada ayuda de pantalla dice qué es y tiene al menos dos puntos", () => {
    for (const a of AYUDA_POR_PANTALLA) {
      expect(a.queEs.length, `${a.ruta}: sin «qué es» el panel no contesta la pregunta`).toBeGreaterThan(20);
      expect(a.puntos.length, `${a.ruta}: con un solo punto no hacía falta un panel`).toBeGreaterThanOrEqual(2);
      expect(a.puntos.length, `${a.ruta}: más de cuatro puntos no se leen`).toBeLessThanOrEqual(4);
    }
  });
});

describe("el buscador del manual", () => {
  it("encuentra por una palabra que está en el CUERPO, no en el título", () => {
    // Es el caso real: nadie busca «Pagarle a un proveedor», busca «macro» o «rebotada».
    expect(buscarGuias("macro").map((g) => g.id)).toContain("pagar-proveedor");
    expect(buscarGuias("rebotada").map((g) => g.id)).toContain("cuando-algo-no-sale-derecho");
    // Y dentro de una tabla, que es donde está la mitad del contenido útil.
    expect(buscarGuias("caja chica").map((g) => g.id)).toContain("cuando-algo-no-sale-derecho");
  });

  it("no distingue mayúsculas ni espacios de más", () => {
    expect(buscarGuias("  MACRO ").map((g) => g.id)).toContain("pagar-proveedor");
  });

  it("sin texto devuelve todo", () => {
    expect(buscarGuias("")).toHaveLength(GUIAS.length);
  });

  it("lo que no está devuelve vacío, no todo", () => {
    expect(buscarGuias("criptomonedas")).toEqual([]);
  });
});
