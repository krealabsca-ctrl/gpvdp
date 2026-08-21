/**
 * Riel de módulos (nivel 1 de navegación). Un icono por módulo del ERP; el
 * módulo activo se resalta con barra lateral. Los módulos aún no construidos se
 * muestran atenuados ("Próximamente"). Escala a 20+ módulos (el riel scrollea).
 */

import { useLocation, useNavigate } from "react-router-dom";
import { useToast } from "@/components/ui";
import { cn } from "@/lib/cn";
import { MODULES, moduloActivo, permisoDePagina } from "@/app/nav";
import { useTienePermiso } from "@/features/auth/permisos";

export function ModuleRail() {
  const navigate = useNavigate();
  const toast = useToast();
  const { pathname } = useLocation();
  const activo = moduloActivo(pathname);
  const tienePermiso = useTienePermiso();

  // Un módulo disponible se muestra solo si el usuario puede ver al menos una de sus
  // páginas (permiso efectivo). Los módulos del roadmap (disponible:false) se ven atenuados.
  const visibles = MODULES.filter(
    (m) => !m.disponible || m.pages.some((p) => {
      const permiso = permisoDePagina(m, p);
      return !permiso || tienePermiso(permiso);
    }),
  );

  return (
    <nav
      aria-label="Módulos"
      className="hidden w-16 shrink-0 flex-col items-center gap-1 overflow-y-auto border-r border-border bg-surface-raised py-3 md:flex"
    >
      {visibles.map((m, i) => {
        const Icon = m.icon;
        const isActive = m.disponible && m.id === activo.id;
        // Separador antes del PRIMER módulo del roadmap: sin él, un módulo terminado que quede
        // rodeado de grises se lee como si tampoco funcionara. Se deriva de `disponible` y no
        // de un índice fijo, así que se mantiene solo cuando un módulo del roadmap se termina.
        const abreRoadmap = !m.disponible && (i === 0 || visibles[i - 1]?.disponible === true);
        return (
          // `display: contents` para que el botón siga participando del flex del riel y el
          // `gap-1` no cambie por envolverlo.
          <div key={m.id} className="contents">
            {abreRoadmap && (
              <span
                aria-hidden
                title="De aquí abajo: módulos del roadmap, todavía sin construir"
                className="my-2 h-px w-7 shrink-0 bg-border"
              />
            )}
          <button
            type="button"
            title={m.disponible ? m.label : `${m.label} · Próximamente`}
            aria-label={m.label}
            aria-current={isActive ? "page" : undefined}
            onClick={() => (m.disponible ? navigate(m.to) : toast.info(`${m.label}: próximamente.`))}
            className={cn(
              "relative flex h-11 w-11 items-center justify-center rounded-xl transition-colors",
              isActive
                ? "bg-accent/10 text-accent"
                : m.disponible
                  ? "text-content-muted hover:bg-surface-muted hover:text-content"
                  : "text-content-muted/40 hover:bg-surface-muted/50",
            )}
          >
            {isActive && (
              <span aria-hidden className="absolute inset-y-2 left-0 w-1 rounded-r-full bg-accent" />
            )}
            <Icon className="h-5 w-5" aria-hidden />
          </button>
          </div>
        );
      })}
    </nav>
  );
}
