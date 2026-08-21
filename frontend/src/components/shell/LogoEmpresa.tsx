/**
 * Marca de cada empresa del grupo: el LOGOTIPO OFICIAL (archivo en public/marcas/)
 * más su color y su descriptor, leídos del propio logotipo.
 *
 * Reglas:
 *  - El logo se muestra tal cual viene el archivo. Acá NO se dibuja ningún logotipo
 *    aproximado: si el archivo falta, se cae a la inicial de la empresa en su color.
 *  - Se pinta sobre un recuadro BLANCO (el área de respeto de la marca), así se ve
 *    igual en tema claro y oscuro y ningún fondo altera los colores del logo.
 *  - Ver public/marcas/LEEME.txt para los nombres de archivo esperados.
 */

import { useState } from "react";
import { cn } from "@/lib/cn";

export interface MarcaEmpresa {
  /** Slug del archivo en public/marcas/ y del tema por empresa. */
  slug: string;
  /** Color de marca leído del logotipo (para bordes, franjas y el «Entrar →»). */
  color: string;
  /** Variante clara del mismo color, para que contraste en tema oscuro. */
  colorOscuro: string;
  /** Bajada del propio logotipo (no inventada). */
  descriptor: string;
}

/**
 * Marca de una empresa a partir de su nombre en la base de datos.
 * Al agregar una empresa nueva hay que sumarla acá (y su archivo en public/marcas/).
 */
export function marcaDeEmpresa(nombre: string | null | undefined): MarcaEmpresa {
  const n = (nombre ?? "").toLowerCase();
  if (n.includes("coopeprofa") || n.includes("protección") || n.includes("proteccion")) {
    // Logotipo: abrazo en turquesa + azul marino, con la figura de la familia. Se toma el
    // TURQUESA como color de la empresa: el azul marino lo comparte con las otras dos, el
    // turquesa es lo único suyo.
    return {
      slug: "coopeprofa",
      color: "#128BA8",
      colorOscuro: "#4FC3DE",
      descriptor: "Cooperativa de Protección Familiar",
    };
  }
  if (n.includes("memorial")) {
    // Logotipo: la misma paloma de Valle de Paz, con la bajada «by Valle de Paz».
    return {
      slug: "memorial-pets",
      color: "#17395F",
      colorOscuro: "#8FB4DE",
      descriptor: "by Valle de Paz",
    };
  }
  return {
    slug: "valle-de-paz",
    color: "#17395F",
    colorOscuro: "#8FB4DE",
    descriptor: "Camposanto · Funeraria · Crematorio",
  };
}

/** Inicial de la empresa (respaldo cuando todavía no está el archivo del logo). */
function inicial(nombre: string): string {
  return (nombre.trim()[0] ?? "?").toUpperCase();
}

interface LogoEmpresaProps {
  nombre: string;
  marca: MarcaEmpresa;
  /** Alto del recuadro blanco en px. */
  alto?: number;
  className?: string;
}

/**
 * Recuadro blanco con el logotipo oficial de la empresa. Prueba .svg, luego .png,
 * y si ninguno existe muestra la inicial en el color de marca.
 */
export function LogoEmpresa({ nombre, marca, alto = 84, className }: LogoEmpresaProps) {
  // 0 = .svg · 1 = .png · 2 = sin archivo (respaldo con la inicial).
  const [intento, setIntento] = useState(0);
  const fuente = intento === 0 ? `/marcas/${marca.slug}.svg` : `/marcas/${marca.slug}.png`;

  return (
    <span
      className={cn(
        "flex w-full items-center justify-center overflow-hidden rounded-xl border border-black/5 bg-white px-4",
        className,
      )}
      style={{ height: alto }}
    >
      {intento < 2 ? (
        <img
          src={fuente}
          alt={`Logotipo de ${nombre}`}
          className="max-h-full max-w-full object-contain"
          onError={() => setIntento((i) => i + 1)}
        />
      ) : (
        <span
          aria-hidden="true"
          className="text-3xl font-bold tracking-tight"
          style={{ color: marca.color }}
          title="Falta el archivo del logotipo en public/marcas/"
        >
          {inicial(nombre)}
        </span>
      )}
    </span>
  );
}
