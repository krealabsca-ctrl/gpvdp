import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * Lo que todo documento que alguien FIRMA tiene que cumplir, revisado leyendo el fuente.
 *
 * Son tres hoy —acta de conciliación, finiquito, relación de pagos— y comparten molde: HTML en una
 * ventana que la persona guarda como PDF. Esta prueba se descubre sola los archivos, así que un
 * cuarto documento nace cumpliendo o la prueba lo nombra.
 *
 * Las dos reglas salen de defectos reales, no de teoría:
 *
 *  · SIN `background` declarado el fondo lo pone el sistema, y en un equipo con tema oscuro el
 *    documento sale NEGRO con el texto gris encima. Al imprimir saldría blanco igual, así que
 *    ninguna prueba de contenido lo ve: solo se nota abriendo la ventana, que es exactamente el
 *    momento en que alguien lo revisa antes de mandarlo a firmar.
 *  · UN `</style>` que faltaba dejó la relación de pagos COMPLETAMENTE EN BLANCO: sin ese cierre el
 *    parser no sale del <head> y se traga el cuerpo entero. Compilaba, no lanzaba y las pruebas de
 *    contenido seguían verdes.
 */

const RAIZ = "src/features";

/** Todos los componentes de documento imprimible, descubiertos recorriendo features/. */
function imprimibles(dir = RAIZ): string[] {
  const out: string[] = [];
  for (const entrada of readdirSync(dir, { withFileTypes: true })) {
    const ruta = join(dir, entrada.name);
    if (entrada.isDirectory()) out.push(...imprimibles(ruta));
    else if (/Imprimible\.tsx$/.test(entrada.name)) out.push(ruta);
  }
  return out;
}

describe("los documentos imprimibles", () => {
  const archivos = imprimibles();

  // Si el descubrimiento se rompe (alguien mueve o renombra), la prueba pasaría revisando nada.
  it("se encuentran los documentos del sistema", () => {
    expect(archivos.length, "no se encontró ningún *Imprimible.tsx: el descubrimiento dejó de funcionar").toBeGreaterThanOrEqual(3);
  });

  it.each(archivos)("%s declara papel blanco y esquema claro", (archivo) => {
    const src = readFileSync(archivo, "utf8");
    expect(src, `${archivo}: sin «background» el documento sale NEGRO en un equipo con tema oscuro`).toMatch(
      /background:\s*#(FFF|fff)/,
    );
    expect(src, `${archivo}: falta «color-scheme: light», el navegador puede reinterpretar los colores`).toMatch(
      /color-scheme:\s*light/,
    );
  });

  it.each(archivos)("%s cierra todas sus etiquetas <style>", (archivo) => {
    const src = readFileSync(archivo, "utf8");
    const abren = (src.match(/<style>/g) ?? []).length;
    const cierran = (src.match(/<\/style>/g) ?? []).length;
    expect(abren, `${archivo}: no tiene ninguna etiqueta <style>`).toBeGreaterThan(0);
    expect(cierran, `${archivo}: ${abren} <style> y ${cierran} </style> — sin el cierre el documento sale EN BLANCO`).toBe(abren);
  });
});
