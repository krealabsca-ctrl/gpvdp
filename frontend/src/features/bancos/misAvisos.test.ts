import { describe, expect, it, vi } from "vitest";
import {
  MAX_VISTOS,
  claveVistos,
  escribirVistos,
  estadoDeAviso,
  idsResueltos,
  leerVistos,
  respuestasNuevas,
  tituloAvisoResuelto,
  unirVistos,
} from "@/features/bancos/misAvisos";

/**
 * LA RESPUESTA A LOS AVISOS (22-set-2026).
 *
 * Antes la respuesta no se veía en ningún lado y el pie prometía lo contrario. El caso que más
 * importa es el más común: la corrección fue pasar el movimiento a OTRA partida, así que sale del
 * alcance y la fila desaparece. «Mis avisos» tiene que leerse como «lo movieron», no como un error.
 */

describe("tituloAvisoResuelto (la fila)", () => {
  it("dice cómo se cerró", () => {
    expect(tituloAvisoResuelto({ resolucion: "RECLASIFICADO" })).toBe("Corregido");
    expect(tituloAvisoResuelto({ resolucion: "SIN_CAMBIO" })).toBe("Revisado, sin cambio");
  });
});

describe("estadoDeAviso («Mis avisos»)", () => {
  it("abierto: en revisión", () => {
    const e = estadoDeAviso({ estado: "EN_REVISION", resolucion: "", es_faltante: false });
    expect(e).toEqual({
      etiqueta: "En revisión",
      tono: "pendiente",
      explicacion: "Todavía no lo respondió quien clasifica.",
    });
  });

  // El aviso cuyo movimiento ya no está en la partida: se lee como lo que es.
  it("reclasificado: explica que salió de la partida y que no se perdió", () => {
    const e = estadoDeAviso({ estado: "RESUELTO", resolucion: "RECLASIFICADO", es_faltante: false });
    expect(e.etiqueta).toBe("Corregido");
    expect(e.tono).toBe("positivo");
    expect(e.explicacion).toMatch(/quedó en la partida que corresponde/);
    expect(e.explicacion).toMatch(/no se perdió/);
  });

  it("faltante reclasificado: lo encontraron", () => {
    const e = estadoDeAviso({ estado: "RESUELTO", resolucion: "RECLASIFICADO", es_faltante: true });
    expect(e.explicacion).toBe("Quien clasifica encontró el movimiento y le corrigió la partida.");
  });

  it("sin cambio: la partida estaba bien", () => {
    expect(estadoDeAviso({ estado: "RESUELTO", resolucion: "SIN_CAMBIO", es_faltante: false })).toEqual({
      etiqueta: "Sin cambio",
      tono: "neutral",
      explicacion: "Quien clasifica lo revisó y dejó la partida como estaba.",
    });
    expect(estadoDeAviso({ estado: "RESUELTO", resolucion: "SIN_CAMBIO", es_faltante: true }).explicacion).toBe(
      "Quien clasifica lo revisó y no cambió nada.",
    );
  });

  // Ningún estado de un aviso es un error: pintarlo en rojo diría «algo falló».
  it("ningún estado se pinta como error", () => {
    for (const estado of ["EN_REVISION", "RESUELTO"] as const) {
      for (const resolucion of ["", "RECLASIFICADO", "SIN_CAMBIO"] as const) {
        for (const es_faltante of [true, false]) {
          expect(estadoDeAviso({ estado, resolucion, es_faltante }).tono).not.toBe("negativo");
        }
      }
    }
  });
});

describe("«Respuesta nueva»: lo visto en este navegador", () => {
  function almacen(inicial: Record<string, string> = {}) {
    const datos = new Map(Object.entries(inicial));
    return {
      getItem: vi.fn((k: string) => datos.get(k) ?? null),
      setItem: vi.fn((k: string, v: string) => {
        datos.set(k, v);
      }),
      datos,
    };
  }

  it("la clave es por empresa y por usuario", () => {
    expect(claveVistos("e1", "u1")).not.toBe(claveVistos("e1", "u2"));
    expect(claveVistos("e1", "u1")).not.toBe(claveVistos("e2", "u1"));
  });

  it("sin almacenamiento o sin clave no se puede saber (null), no «todo es nuevo»", () => {
    expect(leerVistos(null, "k")).toBeNull();
    expect(leerVistos(almacen(), null)).toBeNull();
    expect(respuestasNuevas([{ id: "a", estado: "RESUELTO" }], null).size).toBe(0);
  });

  it("si el navegador no deja leer, tampoco se puede saber", () => {
    const roto = {
      getItem: () => {
        throw new Error("SecurityError");
      },
    };
    expect(leerVistos(roto, "k")).toBeNull();
  });

  it("primera vez: conjunto vacío; lo guardado se relee", () => {
    const s = almacen();
    expect(leerVistos(s, "k")).toEqual(new Set());
    escribirVistos(s, "k", new Set(["a", "b"]));
    expect(leerVistos(s, "k")).toEqual(new Set(["a", "b"]));
  });

  // Un valor corrupto no puede apagar la marca para siempre: cuenta como vacío y se reescribe.
  it("un valor guardado que no se entiende cuenta como vacío", () => {
    expect(leerVistos(almacen({ k: "{no es json" }), "k")).toEqual(new Set());
    expect(leerVistos(almacen({ k: '{"a":1}' }), "k")).toEqual(new Set());
    expect(leerVistos(almacen({ k: '["a", 3, "b"]' }), "k")).toEqual(new Set(["a", "b"]));
  });

  it("escribir nunca revienta la pantalla y se queda con los últimos", () => {
    const lleno = {
      setItem: () => {
        throw new Error("QuotaExceededError");
      },
    };
    expect(() => escribirVistos(lleno, "k", new Set(["a"]))).not.toThrow();

    const s = almacen();
    const muchos = new Set(Array.from({ length: MAX_VISTOS + 5 }, (_, i) => `id${i}`));
    escribirVistos(s, "k", muchos);
    const guardados = JSON.parse(s.datos.get("k") ?? "[]") as string[];
    expect(guardados).toHaveLength(MAX_VISTOS);
    expect(guardados[0]).toBe("id5");
    expect(guardados[guardados.length - 1]).toBe(`id${MAX_VISTOS + 4}`);
  });

  it("solo son nuevas las RESUELTAS que no se vieron", () => {
    const items = [
      { id: "abierto", estado: "EN_REVISION" as const },
      { id: "visto", estado: "RESUELTO" as const },
      { id: "nuevo", estado: "RESUELTO" as const },
    ];
    expect([...respuestasNuevas(items, new Set(["visto"]))]).toEqual(["nuevo"]);
    expect(idsResueltos(items)).toEqual(["visto", "nuevo"]);
  });

  // Devolver el MISMO conjunto cuando no hay nada nuevo es lo que evita el bucle efecto→estado.
  it("unirVistos no cambia la referencia si no hay nada nuevo", () => {
    const prev = new Set(["a", "b"]);
    expect(unirVistos(prev, ["a"])).toBe(prev);
    expect(unirVistos(prev, [])).toBe(prev);
    const unidos = unirVistos(prev, ["c"]);
    expect(unidos).not.toBe(prev);
    expect(unidos).toEqual(new Set(["a", "b", "c"]));
    expect(unirVistos(null, ["a"])).toBeNull();
  });
});
