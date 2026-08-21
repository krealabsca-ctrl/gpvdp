/**
 * Sidebar CONTEXTUAL (nivel 2): muestra las páginas del módulo activo (según la
 * ruta). El módulo se elige en el riel; aquí se navega entre sus pantallas.
 * Iconos Lucide + indicador activo en barra lateral, en el acento de la empresa.
 */

import { NavLink, useLocation } from "react-router-dom";
import { cn } from "@/lib/cn";
import { moduloActivo, permisoDePagina } from "@/app/nav";
import { useTienePermiso } from "@/features/auth/permisos";

export function Sidebar() {
  const { pathname } = useLocation();
  const modulo = moduloActivo(pathname);
  const ModuloIcon = modulo.icon;
  const tienePermiso = useTienePermiso();
  // Oculta las páginas cuyo permiso (propio o heredado del módulo) el usuario no tiene.
  const paginas = modulo.pages.filter((p) => {
    const permiso = permisoDePagina(modulo, p);
    return !permiso || tienePermiso(permiso);
  });

  return (
    <aside className="hidden w-60 shrink-0 border-r border-border bg-surface-raised md:block">
      <div className="flex items-center gap-2.5 px-4 pb-1 pt-4">
        <ModuloIcon className="h-[18px] w-[18px] text-accent" aria-hidden />
        <span className="text-sm font-semibold tracking-tight text-content">{modulo.label}</span>
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
