import type { ReactNode } from "react";

import { AyudaDeEstaPantalla } from "@/features/ayuda/AyudaDeEstaPantalla";

interface PageHeaderProps {
  title: string;
  description?: string;
  /** Acciones a la derecha (botones, selectores). */
  actions?: ReactNode;
}

/**
 * Cabecera consistente de página: título + ayuda + descripción + acciones.
 *
 * El botón de ayuda va ACÁ y no en cada pantalla (22-oct-2026, punto 3 de la revisión comercial):
 * las 68 pantallas del sistema ya usan este componente, así que la ayuda contextual aparece en
 * todas sin editar ninguna. Se dibuja solo donde hay ayuda escrita, así que acá no hay nada que
 * decidir: ver features/ayuda/pantallas.ts.
 *
 * Pegado al título y no en `actions`: es sobre QUÉ es esta pantalla, no una acción más.
 */
export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div>
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-semibold tracking-tight text-content">{title}</h1>
          <AyudaDeEstaPantalla />
        </div>
        {description && <p className="mt-1 text-sm text-content-muted">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}
