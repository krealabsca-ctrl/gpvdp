/**
 * Logotipo de marca de Finance Group VDP.
 *
 * Marca (SVG, sin assets externos): insignia verde con un "valle" y un sol
 * dorado — evoca Valle de Paz (jardines/memorial) y unifica al grupo. Usa los
 * tokens de color (fill-accent / fill-accent-fg / fill-brand-gold), así que se
 * adapta a claro/oscuro y a un futuro color por empresa sin tocar el SVG.
 *
 * Si más adelante hay un logo oficial en archivo, se reemplaza <BrandMark/> por
 * un <img src="/logo.svg"> y listo.
 */

import { cn } from "@/lib/cn";

interface BrandLogoProps {
  className?: string;
  /** Muestra el subtítulo con las empresas del grupo (para login/selector). */
  tagline?: boolean;
  /** Tamaño de la insignia en px (por defecto 32). */
  size?: number;
}

export function BrandLogo({ className, tagline = false, size = 32 }: BrandLogoProps) {
  return (
    <span className={cn("flex items-center gap-2.5", className)}>
      <BrandMark size={size} />
      <span className="flex flex-col leading-tight">
        <span className="text-base font-semibold tracking-tight text-content">Finance Group VDP</span>
        {tagline && (
          <span className="text-[11px] font-medium uppercase tracking-wider text-content-muted">
            Valle de Paz · Coopeprofa · Memorial Pets
          </span>
        )}
      </span>
    </span>
  );
}

function BrandMark({ size }: { size: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      aria-hidden="true"
      className="shrink-0"
    >
      <rect x="0.5" y="0.5" width="31" height="31" rx="8" className="fill-accent" />
      {/* Sol dorado */}
      <circle cx="22" cy="9.5" r="2.4" className="fill-brand-gold" />
      {/* Valle / picos */}
      <path
        d="M5 22 L12 12.5 L16.5 18 L21 11 L27 22 Z"
        className="fill-accent-fg"
        opacity="0.95"
      />
    </svg>
  );
}
