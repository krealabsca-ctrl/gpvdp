import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";

/**
 * Paginador de listas largas.
 *
 * Antes las pantallas traían las primeras N filas y avisaban «Mostrando 200 de 4.471 — afiná los
 * filtros para ver el resto». Era honesto pero era un callejón sin salida: no había manera de ver
 * el resto, y con 4.471 facturas abiertas eso significa que 4.271 no existían para quien mira.
 *
 * Tres tamaños (50 · 100 · 200) porque el tamaño útil depende del monitor y de la tarea: revisar
 * de a poco en una laptop, o barrer el corte completo en la pantalla grande de tesorería.
 */
export interface PaginadorProps {
  /** Total de registros que calzan con el filtro (lo dice el backend, no se estima). */
  total: number;
  /** Página actual, empezando en 1. */
  pagina: number;
  porPagina: number;
  /** Cuántas filas llegaron de verdad en esta página. */
  enPantalla: number;
  onPagina: (p: number) => void;
  onPorPagina: (n: number) => void;
  /** Sustantivo en plural para el conteo ("facturas", "proveedores"…). */
  etiqueta?: string;
  className?: string;
}

export const TAMANOS_PAGINA = [50, 100, 200] as const;

/**
 * Aritmética del paginador, aparte del dibujo para poder probarla.
 *
 * `hasta` sale de lo que REALMENTE llegó (`enPantalla`), no de `pagina * porPagina`: en la última
 * página el cálculo teórico miente hacia arriba, y un conteo que miente en una pantalla de plata
 * no sirve para nada.
 */
export function rangoVisible(
  total: number,
  pagina: number,
  porPagina: number,
  enPantalla: number,
): { desde: number; hasta: number; totalPaginas: number } {
  const totalPaginas = Math.max(1, Math.ceil(total / porPagina));
  if (enPantalla <= 0) return { desde: 0, hasta: 0, totalPaginas };
  const desde = (pagina - 1) * porPagina + 1;
  return { desde, hasta: desde + enPantalla - 1, totalPaginas };
}

export function Paginador({
  total,
  pagina,
  porPagina,
  enPantalla,
  onPagina,
  onPorPagina,
  etiqueta = "registros",
  className,
}: PaginadorProps) {
  const { desde, hasta, totalPaginas } = rangoVisible(total, pagina, porPagina, enPantalla);
  const nf = new Intl.NumberFormat("es-CR");

  // Página vacía: el filtro se achicó debajo de los pies y quedamos parados en el aire. Se dice y
  // se ofrece la salida, en vez de mostrar una tabla en blanco que se lee como «no hay nada».
  if (enPantalla <= 0 && total > 0) {
    return (
      <div className={cn("flex flex-wrap items-center gap-3 text-xs text-content-muted", className)}>
        <span>
          La página {nf.format(pagina)} quedó vacía: hay {nf.format(total)} {etiqueta} en{" "}
          {nf.format(totalPaginas)} {totalPaginas === 1 ? "página" : "páginas"}.
        </span>
        <Button size="sm" variant="secondary" onClick={() => onPagina(1)}>
          Ir a la primera
        </Button>
      </div>
    );
  }

  const primera = pagina <= 1;
  const ultima = pagina >= totalPaginas;

  return (
    <div className={cn("flex flex-wrap items-center justify-between gap-3", className)}>
      <p className="text-xs tabular-nums text-content-muted">
        {total > enPantalla
          ? `Mostrando ${nf.format(desde)}–${nf.format(hasta)} de ${nf.format(total)} ${etiqueta}`
          : `${nf.format(total)} ${etiqueta}`}
      </p>

      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1.5 text-xs text-content-muted">
          Por página
          <select
            aria-label="Filas por página"
            className="h-8 rounded-lg border border-border bg-surface-raised px-2 text-xs tabular-nums text-content shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            value={porPagina}
            onChange={(e) => {
              onPorPagina(Number(e.target.value));
              onPagina(1);
            }}
          >
            {TAMANOS_PAGINA.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>

        {totalPaginas > 1 && (
          <div className="flex items-center gap-1">
            <Button size="sm" variant="ghost" disabled={primera} onClick={() => onPagina(1)} title="Primera página">
              ««
            </Button>
            <Button size="sm" variant="secondary" disabled={primera} onClick={() => onPagina(pagina - 1)}>
              ‹ Anterior
            </Button>
            <span className="px-2 text-xs tabular-nums text-content-muted">
              Página {nf.format(pagina)} de {nf.format(totalPaginas)}
            </span>
            <Button size="sm" variant="secondary" disabled={ultima} onClick={() => onPagina(pagina + 1)}>
              Siguiente ›
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={ultima}
              onClick={() => onPagina(totalPaginas)}
              title="Última página"
            >
              »»
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
