/**
 * Sidebar CONTEXTUAL (nivel 2): muestra las páginas del módulo activo (según la
 * ruta). El módulo se elige en el riel; aquí se navega entre sus pantallas.
 * Iconos Lucide + indicador activo en barra lateral, en el acento de la empresa.
 *
 * ── COLAPSABLE ──────────────────────────────────────────────────────────────────────────────
 *
 * Ocupa 240 px fijos y en la operativa diaria estorba, sobre todo en las pantallas de tabla
 * ancha. Se colapsa con el botón del borde o con Ctrl/⌘+B, y la decisión se recuerda por usuario.
 * Colapsada desaparece del todo: el riel de iconos (64 px) sigue permitiendo cambiar de módulo, y
 * las migas de la barra superior dicen dónde se está parado.
 *
 * En las pantallas marcadas `anchoCompleto` se PROPONE colapsarla la primera vez. Apenas el
 * usuario decide algo —abrir o cerrar—, su decisión manda y la propuesta deja de aplicarse.
 */

import { useEffect } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";

import { cn } from "@/lib/cn";
import { moduloActivo, paginaActiva, permisoDePagina } from "@/app/nav";
import { useTienePermiso } from "@/features/auth/permisos";

import type { EstadoSidebar } from "./useSidebarColapsada";

export function Sidebar({ estado }: { estado: EstadoSidebar }) {
  const { pathname } = useLocation();
  const modulo = moduloActivo(pathname);
  const ModuloIcon = modulo.icon;
  const tienePermiso = useTienePermiso();
  const { colapsada, sinDecidir, alternar, proponer } = estado;

  // Auto-colapso: solo PROPONE, y solo mientras el usuario no haya decidido nunca.
  const { page } = paginaActiva(pathname);
  const esAncha = Boolean(page?.anchoCompleto);
  useEffect(() => {
    if (sinDecidir) proponer(esAncha);
  }, [esAncha, sinDecidir, proponer, pathname]);

  // Oculta las páginas cuyo permiso (propio o heredado del módulo) el usuario no tiene.
  const paginas = modulo.pages.filter((p) => {
    const permiso = permisoDePagina(modulo, p);
    return !permiso || tienePermiso(permiso);
  });

  // Colapsada: no queda un panel angosto, se va del todo. Dejar una tira de iconos duplicaría el
  // riel, que ya está al lado y hace exactamente eso.
  if (colapsada) {
    return (
      <button
        type="button"
        onClick={alternar}
        title="Mostrar el menú del módulo (Ctrl+B)"
        aria-label="Mostrar el menú del módulo"
        aria-expanded={false}
        className="hidden w-6 shrink-0 items-center justify-center border-r border-border bg-surface-raised text-content-muted transition-colors hover:bg-surface-muted hover:text-content focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent md:flex"
      >
        <PanelLeftOpen className="h-4 w-4" aria-hidden />
      </button>
    );
  }

  return (
    <aside className="hidden w-60 shrink-0 border-r border-border bg-surface-raised md:block">
      <div className="flex items-center gap-2.5 px-4 pb-1 pt-4">
        <ModuloIcon className="h-[18px] w-[18px] shrink-0 text-accent" aria-hidden />
        <span className="flex-1 truncate text-sm font-semibold tracking-tight text-content">
          {modulo.label}
        </span>
        <button
          type="button"
          onClick={alternar}
          title="Ocultar el menú del módulo (Ctrl+B)"
          aria-label="Ocultar el menú del módulo"
          aria-expanded
          className="rounded p-1 text-content-muted transition-colors hover:bg-surface-muted hover:text-content focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          <PanelLeftClose className="h-4 w-4" aria-hidden />
        </button>
      </div>
      <nav aria-label={modulo.label} className="flex flex-col gap-0.5 p-3 pt-1">
        {paginas.map((page) => {
          const Icon = page.icon;
          return (
            <NavLink
              key={`${page.label}-${page.to}`}
              to={page.to}
              end={page.end}
              className={({ isActive }) =>
                cn(
                  "group relative flex items-center gap-3 rounded-lg px-3 py-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent",
                  isActive ? "bg-accent/10 text-accent" : "text-content hover:bg-surface-muted",
                )
              }
            >
              {({ isActive }) => (
                <>
                  {isActive && (
                    <span aria-hidden className="absolute inset-y-1.5 left-0 w-1 rounded-r-full bg-accent" />
                  )}
                  <Icon
                    aria-hidden
                    className={cn(
                      "h-[18px] w-[18px] shrink-0",
                      isActive ? "text-accent" : "text-content-muted group-hover:text-content",
                    )}
                  />
                  <span className="flex min-w-0 flex-col leading-tight">
                    <span className={cn("truncate text-sm font-medium", isActive ? "text-accent" : "text-content")}>
                      {page.label}
                    </span>
                    <span className="truncate text-[11px] text-content-muted">{page.descripcion}</span>
                  </span>
                </>
              )}
            </NavLink>
          );
        })}
      </nav>
    </aside>
  );
}
